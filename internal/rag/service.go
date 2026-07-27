package rag

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/JexKnox/mindbridge/internal/cache"
	"github.com/JexKnox/mindbridge/internal/metrics"
)

// Citation 是返回给前端的来源引用。
type Citation struct {
	SourceFile string  `json:"source_file"`
	ChunkIndex int     `json:"chunk_index"`
	Score      float64 `json:"score"`
}

// Result 是一次问答的完整结果,除引用外还携带留痕(trace)所需信息:
// 回答全文、注入 prompt 的证据 chunk(LLM Judge 评 faithfulness 用)、拒答标志。
type Result struct {
	Citations []Citation
	Contexts  []string
	Answer    string
	Refused   bool
	TopScore  float64
}

// Reranker 对召回文档做 cross-encoder 重排。实现见 adapter.HTTPReranker。
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error)
}

// Options 汇总 Service 的可调参数。Reranker / 改写模型 / 缓存为 nil 时对应环节自动跳过。
type Options struct {
	RewriteModel model.ChatModel // query 改写用小模型,nil 禁用改写
	RewriteN     int             // 改写 query 数(不含原 query)
	Reranker     Reranker        // nil 时退化为按融合序截断
	RerankTopK   int             // 重排/截断后保留的文档数
	Refuse       float64         // top1 余弦分低于此值则提示模型拒答

	// RewriteCache 缓存改写结果(同一问题改写基本稳定)。
	RewriteCache *cache.LRU[[]string]
	// RetrievalCache 缓存「改写→召回→重排」后的最终文档列表,命中时整段跳过。
	// CacheKeyPrefix 由调用方拼好,须含 kb collection、KB 版本号与检索配置,
	// 保证文档变更(版本自增)或调参后旧缓存不再命中。
	RetrievalCache *cache.LRU[cache.RetrievalEntry]
	CacheKeyPrefix string
}

// Service 编排检索 + 生成。检索范围由传入的 retriever 决定(已绑定到 kb_id 对应 collection),
// 杜绝跨库串数据。
type Service struct {
	retriever  retriever.Retriever
	chat       model.ChatModel
	rewriter   *QueryRewriter
	reranker   Reranker
	rerankTopK int
	refuse     float64

	retrievalCache *cache.LRU[cache.RetrievalEntry]
	cacheKeyPrefix string
}

func NewService(r retriever.Retriever, chat model.ChatModel, opts Options) *Service {
	return &Service{
		retriever:      r,
		chat:           chat,
		rewriter:       NewQueryRewriter(opts.RewriteModel, opts.RewriteN, opts.RewriteCache),
		reranker:       opts.Reranker,
		rerankTopK:     opts.RerankTopK,
		refuse:         opts.Refuse,
		retrievalCache: opts.RetrievalCache,
		cacheKeyPrefix: opts.CacheKeyPrefix,
	}
}

// Answer 流式生成回答。onToken 用于逐 token 回推前端(SSE);
// 返回 Result(引用 + 回答全文 + 证据 chunk + 拒答标志),供 handler 回推引用并写问答留痕。
func (s *Service) Answer(ctx context.Context, query string, onToken func(string)) (*Result, error) {
	docs, maxScore, err := s.retrieveWithCache(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieve: %w", err)
	}

	res := &Result{
		Citations: make([]Citation, 0, len(docs)),
		Contexts:  make([]string, 0, len(docs)),
		TopScore:  maxScore,
	}
	for _, d := range docs {
		res.Citations = append(res.Citations, Citation{
			SourceFile: asString(d.MetaData["source_file"]),
			ChunkIndex: asInt(d.MetaData["chunk_index"]),
			Score:      asFloat(d.MetaData["score"]),
		})
		res.Contexts = append(res.Contexts, d.Content)
	}

	// 拒答:top1 相关度过低时,不注入检索内容,提示模型基于自身说明拒答。
	res.Refused = len(docs) == 0 || maxScore < s.refuse
	var sys string
	if res.Refused {
		sys = "你是一个严谨的问答助手。知识库中没有与问题相关的信息。" +
			"请明确告知用户:当前知识库中未找到相关内容,不要编造答案。"
	} else {
		sys = "你是一个严谨的问答助手,只能基于下方【参考资料】回答用户问题。" +
			"回答时在对应事实后用 [1][2] 形式标注引用编号;若资料不足以回答,请说明。\n\n" +
			"【参考资料】\n" + renderContext(docs)
	}

	messages := []*schema.Message{
		schema.SystemMessage(sys),
		schema.UserMessage(query),
	}

	// 生成层指标:TTFT / 完整生成时长 / 输出字符量 / 拒答 / 引用数 / 流错误。
	genStart := time.Now()
	var ttft time.Duration
	var streamErr error
	var answer strings.Builder
	defer func() {
		metrics.ObserveAnswer(res.Refused, len(res.Citations), ttft, time.Since(genStart),
			len([]rune(answer.String())), streamErr)
	}()

	stream, err := s.chat.Stream(ctx, messages)
	if err != nil {
		streamErr = err
		return nil, fmt.Errorf("rag: chat stream: %w", err)
	}
	defer stream.Close()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			streamErr = err
			res.Answer = answer.String()
			return res, fmt.Errorf("rag: recv stream: %w", err)
		}
		if msg.Content != "" {
			if ttft == 0 {
				ttft = time.Since(genStart)
			}
			answer.WriteString(msg.Content)
			onToken(msg.Content)
		}
	}
	res.Answer = answer.String()
	return res, nil
}

// retrieveWithCache 执行「改写 → 多路召回 → 重排/截断」,可选结果缓存。
// 缓存命中时整段跳过(省 1+N 次 embedding、N 次向量检索、1 次改写和 1 次重排调用);
// 返回的 maxScore 为重排前的最高余弦分,供拒答阈值判断。
func (s *Service) retrieveWithCache(ctx context.Context, query string) ([]*schema.Document, float64, error) {
	var cacheKey string
	if s.retrievalCache != nil {
		cacheKey = s.cacheKeyPrefix + ":" + cache.Hash(query)
		ent, ok := s.retrievalCache.Get(cacheKey)
		metrics.Cache("retrieval", ok)
		if ok {
			return ent.Docs, ent.MaxScore, nil
		}
	}

	// 多路改写并行召回:原 query + 改写 query 各自检索,去重后 RRF 合并。
	queries := s.rewriter.Rewrite(ctx, query)
	docs, err := s.retrieveAll(ctx, queries)
	if err != nil {
		return nil, 0, err
	}

	// 拒答判断基于重排前的最高余弦分:重排分数量纲不同,且重排只影响排序不改相关性事实。
	maxScore := topScore(docs)
	metrics.ObserveRetrieval(len(docs), maxScore)

	if s.reranker != nil && len(docs) > 0 {
		preTop1 := docs[0].ID
		reranked, err := s.reranker.Rerank(ctx, query, docs)
		if err == nil {
			docs = reranked
			// rerank 前后 top1 变化率:过高说明召回排序差,持续为 0 说明重排冗余。
			metrics.ObserveRerank(len(docs) > 0 && docs[0].ID != preTop1)
		}
		// 重排失败静默降级为融合序截断,不阻断回答。
	}
	if s.rerankTopK > 0 && len(docs) > s.rerankTopK {
		docs = docs[:s.rerankTopK]
	}

	if s.retrievalCache != nil {
		s.retrievalCache.Set(cacheKey, cache.RetrievalEntry{Docs: docs, MaxScore: maxScore})
	}
	return docs, maxScore, nil
}

// retrieveAll 对每个 query 召回一次,跨 query 去重后按 RRF 融合排序。
// 文档 MetaData["score"] 保留各路中的最大余弦分,供拒答阈值与引用展示使用。
func (s *Service) retrieveAll(ctx context.Context, queries []string) ([]*schema.Document, error) {
	if len(queries) == 1 {
		return s.retriever.Retrieve(ctx, queries[0])
	}

	const rrfK = 60
	type entry struct {
		doc *schema.Document
		rrf float64
	}
	byID := map[string]*entry{}
	var firstErr error
	ok := false
	for _, q := range queries {
		docs, err := s.retriever.Retrieve(ctx, q)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue // 单路失败不阻断:其余 query 的召回仍可用
		}
		ok = true
		for rank, d := range docs {
			key := d.ID
			if key == "" {
				key = asString(d.MetaData["source_file"]) + "#" + fmt.Sprint(asInt(d.MetaData["chunk_index"]))
			}
			score := 1.0 / float64(rrfK+rank+1)
			if e, exists := byID[key]; exists {
				e.rrf += score
				if asFloat(d.MetaData["score"]) > asFloat(e.doc.MetaData["score"]) {
					e.doc.MetaData["score"] = d.MetaData["score"]
				}
				continue
			}
			byID[key] = &entry{doc: d, rrf: score}
		}
	}
	if !ok {
		return nil, firstErr
	}

	entries := make([]*entry, 0, len(byID))
	for _, e := range byID {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rrf > entries[j].rrf })
	out := make([]*schema.Document, len(entries))
	for i, e := range entries {
		out[i] = e.doc
	}
	return out, nil
}

// renderContext 把召回 chunk 渲染成带编号的 prompt 片段。
func renderContext(docs []*schema.Document) string {
	var b strings.Builder
	for i, d := range docs {
		fmt.Fprintf(&b, "[%d] %s\n\n", i+1, d.Content)
	}
	return b.String()
}

// topScore 返回文档中最高的向量余弦分。
func topScore(docs []*schema.Document) float64 {
	max := 0.0
	for _, d := range docs {
		if s := asFloat(d.MetaData["score"]); s > max {
			max = s
		}
	}
	return max
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}
