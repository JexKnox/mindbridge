package server

import (
	"context"
	"fmt"
	"io"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/JexKnox/mindbridge/internal/kb"
)

// agentChat 是 agent 链路入口:检索→决策→调工具→生成,流式 SSE。
// 与 /api/kb/:id/chat 的固定 RAG 路径并列,不复用 rag.Service。
//
// 请求体:{"query": string, "kb_id"?: string}。kb_id 可选:绑了就把 kb_search 工具交给 agent。
//
// SSE 事件:
//
//	data: <token>          逐 token(无 event 行)
//	event: tool_call       {name, args}
//	event: tool_result     {name, result|error}
//	event: done            结束
//	event: error           出错
func (h *handlers) agentChat(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Query string `json:"query"`
		KbID  string `json:"kb_id"`
	}
	if err := c.Bind(&body); err != nil {
		writeErr(c, 400, "invalid body: "+err.Error())
		return
	}
	if body.Query == "" {
		writeErr(c, 400, "query is required")
		return
	}

	// kb_id 可选:有则取 KB(404 同现有 chat handler),无则 nil。
	var k *kb.KB
	if body.KbID != "" {
		var err error
		k, err = h.deps.KB.Get(ctx, body.KbID)
		if err != nil {
			writeErr(c, 404, "kb not found")
			return
		}
	}

	// SSE 响应头(与 chat_handler 一致)。
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")

	pr, pw := io.Pipe()
	c.SetBodyStream(pr, -1)

	go func() {
		defer pw.Close()
		bgctx := context.Background()
		onEvent := func(event string, data string) {
			if event == "" {
				fmt.Fprintf(pw, "data: %s\n\n", data)
				return
			}
			fmt.Fprintf(pw, "event: %s\ndata: %s\n\n", event, data)
		}
		err := h.deps.Agent.Answer(bgctx, body.Query, h.deps.Cfg.Models.Chat, h.deps.Cfg.Models.Embedding, k, onEvent)
		if err != nil {
			fmt.Fprintf(pw, "event: error\ndata: %s\n\n", err.Error())
		}
	}()
}
