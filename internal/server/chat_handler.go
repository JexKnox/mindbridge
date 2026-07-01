package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/rag"
)

// chat 在指定 KB 上检索 + 流式生成。
// 响应为 SSE:逐 token 的 data 事件 + 末尾 event:citations 携带来源。
func (h *handlers) chat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	k, err := h.deps.KB.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "kb not found")
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if body.Query == "" {
		writeErr(w, http.StatusBadRequest, "query is required")
		return
	}

	embCfg := h.deps.Cfg.Models.Embedding
	embCfg.Model = k.EmbeddingModel
	emb, err := adapter.NewEmbedder(r.Context(), embCfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "build embedder: "+err.Error())
		return
	}
	ret := adapter.NewRetriever(h.deps.Qdrant, emb, k.Collection, h.deps.Cfg.RAG.RetrieveTopK)
	chat, err := adapter.NewChatModel(r.Context(), h.deps.Cfg.Models.Chat)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "build chat model: "+err.Error())
		return
	}
	svc := rag.NewService(ret, chat, h.deps.Cfg.RAG.RerankTopK, h.deps.Cfg.RAG.RefuseThreshold)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	cits, err := svc.Answer(r.Context(), body.Query, func(token string) {
		fmt.Fprintf(w, "data: %s\n\n", token)
		flusher.Flush()
	})
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	if citsJSON, err := json.Marshal(cits); err == nil {
		fmt.Fprintf(w, "event: citations\ndata: %s\n\n", citsJSON)
	}
	flusher.Flush()
}
