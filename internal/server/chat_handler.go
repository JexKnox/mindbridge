package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/rag"
	"github.com/JexKnox/mindbridge/internal/trace"
)

// chat 在指定 KB 上检索 + 流式生成。
// 响应为 SSE:逐 token 的 data 事件 + 末尾 event:citations 携带来源。
//
// Hertz 流式:用 io.Pipe + SetBodyStream(bodySize=-1) 触发 chunked 编码,
// handler 立即返回,goroutine 向 pipe 写 SSE 帧;Hertz 在 handler 返回后
// 从 pipe 读取并实时下发。context 用 Background 以免被 handler 返回取消
// (代价:客户端中途断连不会中断生成,后续可优化)。
func (h *handlers) chat(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	k, err := h.deps.KB.Get(ctx, id)
	if err != nil {
		writeErr(c, 404, "kb not found")
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	if err := c.Bind(&body); err != nil {
		writeErr(c, 400, "invalid body: "+err.Error())
		return
	}
	if body.Query == "" {
		writeErr(c, 400, "query is required")
		return
	}

	embCfg := h.deps.Cfg.Models.Embedding
	embCfg.Model = k.EmbeddingModel
	emb, err := adapter.NewEmbedder(ctx, embCfg)
	if err != nil {
		writeErr(c, 500, "build embedder: "+err.Error())
		return
	}
	// 查询向量缓存:相同问题(含改写)不重复调 embedding API。
	emb = adapter.NewCachedEmbedder(emb, embCfg.Model, h.deps.Caches.Embedding)
	ragCfg := h.deps.Cfg.RAG
	ret := adapter.NewRetriever(h.deps.Qdrant, emb, k.Collection, ragCfg.RetrieveTopK, ragCfg.HybridEnabled())
	chat, err := adapter.NewChatModel(ctx, h.deps.Cfg.Models.Chat)
	if err != nil {
		writeErr(c, 500, "build chat model: "+err.Error())
		return
	}

	// 检索结果缓存 key 前缀:collection + KB 版本号 + 检索配置。
	// 文档变更时版本自增,旧缓存自然失效;调参后前缀变化,同样不会命中旧结果。
	cacheKeyPrefix := fmt.Sprintf("ret:%s:v%d:k%d-r%d-h%v-rw%d",
		k.Collection, h.deps.Caches.KBVersion.Get(k.ID),
		ragCfg.RetrieveTopK, ragCfg.RerankTopK, ragCfg.HybridEnabled(), ragCfg.RewriteN)

	opts := rag.Options{
		RerankTopK:     ragCfg.RerankTopK,
		Refuse:         ragCfg.RefuseThreshold,
		RewriteCache:   h.deps.Caches.Rewrite,
		RetrievalCache: h.deps.Caches.Retrieval,
		CacheKeyPrefix: cacheKeyPrefix,
	}
	// cross-encoder 重排:配置了 reranker 模型才启用,失败时服务内部自动降级。
	if h.deps.Cfg.Models.Reranker.Model != "" {
		opts.Reranker = adapter.NewHTTPReranker(h.deps.Cfg.Models.Reranker, ragCfg.RerankTopK)
	}
	// query 改写:配置了改写小模型才启用;构建失败不阻断主链路。
	if h.deps.Cfg.Models.Rewrite.Model != "" {
		if rw, err := adapter.NewChatModel(ctx, h.deps.Cfg.Models.Rewrite); err == nil {
			opts.RewriteModel = rw
			opts.RewriteN = ragCfg.RewriteN
		}
	}
	svc := rag.NewService(ret, chat, opts)

	// SSE 响应头。
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	pr, pw := io.Pipe()
	c.SetBodyStream(pr, -1) // -1:读到 EOF,自动 chunked

	go func() {
		defer pw.Close()
		ctx := context.Background()
		start := time.Now()
		onToken := func(token string) {
			fmt.Fprintf(pw, "data: %s\n\n", token)
		}
		res, err := svc.Answer(ctx, body.Query, onToken)
		if err != nil {
			fmt.Fprintf(pw, "event: error\ndata: %s\n\n", err.Error())
			return
		}
		citsJSON, err := json.Marshal(res.Citations)
		if err == nil {
			fmt.Fprintf(pw, "event: citations\ndata: %s\n\n", citsJSON)
		}

		// 问答留痕:先把 trace_id 推给前端(反馈接口引用),再异步落库。
		traceID := uuid.NewString()
		fmt.Fprintf(pw, "event: trace\ndata: {\"trace_id\":%q}\n\n", traceID)
		go func() {
			tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			t := &trace.Trace{
				ID:         traceID,
				KBID:       k.ID,
				Query:      body.Query,
				Answer:     res.Answer,
				Citations:  citsJSON,
				Contexts:   res.Contexts,
				TopScore:   res.TopScore,
				Refused:    res.Refused,
				DurationMS: time.Since(start).Milliseconds(),
			}
			if err := h.deps.Traces.Insert(tctx, t); err != nil {
				h.deps.Logger.Warn("insert chat trace failed", zap.Error(err))
			}
		}()
	}()
}
