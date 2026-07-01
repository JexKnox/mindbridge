package rag

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// Citation 是返回给前端的来源引用。
type Citation struct {
	SourceFile string  `json:"source_file"`
	ChunkIndex int     `json:"chunk_index"`
	Score      float64 `json:"score"`
}

// Service 编排检索 + 生成。检索范围由传入的 retriever 决定(已绑定到 kb_id 对应 collection),
// 杜绝跨库串数据。
type Service struct {
	retriever  retriever.Retriever
	chat       model.ChatModel
	rewriter   *QueryRewriter
	rerankTopK int             // rerank 后保留数(本期未实现 reranker,直接截断)
	refuse     float64         // top1 score 低于此值则提示模型拒答
}

func NewService(r retriever.Retriever, chat model.ChatModel, rerankTopK int, refuseThreshold float64) *Service {
	return &Service{
		retriever:  r,
		chat:       chat,
		rewriter:   NewQueryRewriter(),
		rerankTopK: rerankTopK,
		refuse:     refuseThreshold,
	}
}

// Answer 流式生成回答。onToken 用于逐 token 回推前端(SSE);返回完整引用列表。
func (s *Service) Answer(ctx context.Context, query string, onToken func(string)) ([]Citation, error) {
	q, err := s.rewriter.Rewrite(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rag: rewrite query: %w", err)
	}

	docs, err := s.retriever.Retrieve(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("rag: retrieve: %w", err)
	}
	// 截断到 rerankTopK(真正的 reranker 为后续阶段;此处占位截断)。
	if s.rerankTopK > 0 && len(docs) > s.rerankTopK {
		docs = docs[:s.rerankTopK]
	}

	cits := make([]Citation, 0, len(docs))
	for _, d := range docs {
		cits = append(cits, Citation{
			SourceFile: asString(d.MetaData["source_file"]),
			ChunkIndex: asInt(d.MetaData["chunk_index"]),
			Score:      asFloat(d.MetaData["score"]),
		})
	}

	// 拒答:top1 相关度过低时,不注入检索内容,提示模型基于自身说明拒答。
	var sys string
	if len(docs) == 0 || topScore(cits) < s.refuse {
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

	stream, err := s.chat.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("rag: chat stream: %w", err)
	}
	defer stream.Close()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return cits, fmt.Errorf("rag: recv stream: %w", err)
		}
		if msg.Content != "" {
			onToken(msg.Content)
		}
	}
	return cits, nil
}

// renderContext 把召回 chunk 渲染成带编号的 prompt 片段。
func renderContext(docs []*schema.Document) string {
	var b strings.Builder
	for i, d := range docs {
		fmt.Fprintf(&b, "[%d] %s\n\n", i+1, d.Content)
	}
	return b.String()
}

func topScore(cits []Citation) float64 {
	if len(cits) == 0 {
		return 0
	}
	return cits[0].Score
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
