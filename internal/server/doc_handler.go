package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/JexKnox/mindbridge/internal/adapter"
)

// uploadDocument 接收 multipart 文件并摄取到指定 KB。
// 路由 /api/kb/{id}/documents;每个 KB 用其绑定的 embedding 模型建索引器,
// 全局配置提供 base_url/api_key,模型名取自 KB 元信息(保证同库同模型)。
func (h *handlers) uploadDocument(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	k, err := h.deps.KB.Get(ctx, id)
	if err != nil {
		writeErr(c, 404, "kb not found")
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		writeErr(c, 400, "missing 'file' field: "+err.Error())
		return
	}
	file, err := header.Open()
	if err != nil {
		writeErr(c, 400, "open uploaded file: "+err.Error())
		return
	}
	defer file.Close()

	// 同一文件重新上传:先按 source_file 删旧 chunk,避免幽灵数据。
	_ = h.deps.Qdrant.DeleteByPayload(ctx, k.Collection, "source_file", header.Filename)

	embCfg := h.deps.Cfg.Models.Embedding
	embCfg.Model = k.EmbeddingModel
	emb, err := adapter.NewEmbedder(ctx, embCfg)
	if err != nil {
		writeErr(c, 500, "build embedder: "+err.Error())
		return
	}
	idx := adapter.NewIndexer(h.deps.Qdrant, emb, k.Collection)

	n, err := h.deps.Pipeline.Ingest(ctx, k, idx, header.Filename, file)
	if err != nil {
		writeErr(c, 500, err.Error())
		return
	}
	c.JSON(200, utils.H{
		"kb_id":      k.ID,
		"file":       header.Filename,
		"chunks":     n,
		"collection": k.Collection,
	})
}
