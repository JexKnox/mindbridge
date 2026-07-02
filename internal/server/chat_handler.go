package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/rag"
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
	ret := adapter.NewRetriever(h.deps.Qdrant, emb, k.Collection, h.deps.Cfg.RAG.RetrieveTopK)
	chat, err := adapter.NewChatModel(ctx, h.deps.Cfg.Models.Chat)
	if err != nil {
		writeErr(c, 500, "build chat model: "+err.Error())
		return
	}
	svc := rag.NewService(ret, chat, h.deps.Cfg.RAG.RerankTopK, h.deps.Cfg.RAG.RefuseThreshold)

	// SSE 响应头。
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	pr, pw := io.Pipe()
	c.SetBodyStream(pr, -1) // -1:读到 EOF,自动 chunked

	go func() {
		defer pw.Close()
		ctx := context.Background()
		onToken := func(token string) {
			fmt.Fprintf(pw, "data: %s\n\n", token)
		}
		cits, err := svc.Answer(ctx, body.Query, onToken)
		if err != nil {
			fmt.Fprintf(pw, "event: error\ndata: %s\n\n", err.Error())
			return
		}
		if citsJSON, err := json.Marshal(cits); err == nil {
			fmt.Fprintf(pw, "event: citations\ndata: %s\n\n", citsJSON)
		}
	}()
}
