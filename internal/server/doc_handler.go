package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/JexKnox/mindbridge/internal/adapter"
)

// uploadDocument 接收 multipart 文件并摄取到指定 KB。
// 路由 /api/kb/{id}/documents;每个 KB 用其绑定的 embedding 模型建索引器,
// 全局配置提供 base_url/api_key,模型名取自 KB 元信息(保证同库同模型)。
func (h *handlers) uploadDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	k, err := h.deps.KB.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "kb not found")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "parse multipart: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()

	// 同一文件重新上传:先按 source_file 删旧 chunk,避免幽灵数据。
	_ = h.deps.Qdrant.DeleteByPayload(r.Context(), k.Collection, "source_file", header.Filename)

	embCfg := h.deps.Cfg.Models.Embedding
	embCfg.Model = k.EmbeddingModel
	emb, err := adapter.NewEmbedder(r.Context(), embCfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "build embedder: "+err.Error())
		return
	}
	idx := adapter.NewIndexer(h.deps.Qdrant, emb, k.Collection)

	n, err := h.deps.Pipeline.Ingest(r.Context(), k, idx, header.Filename, file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kb_id":      k.ID,
		"file":       header.Filename,
		"chunks":     n,
		"collection": k.Collection,
	})
}
