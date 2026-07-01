package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// qdrantRetriever 实现 eino 的 retriever.Retriever:把 query 向量化后在 Qdrant 检索 topK。
type qdrantRetriever struct {
	qdrant     *QdrantClient
	embedder   embedding.Embedder
	collection string
	topK       int
}

// NewRetriever 构造一个面向指定 collection 的 eino Retriever。
// topK 控制向量召回数量;reranker / BM25 混合检索为后续阶段,届时在更上层编排,本接口不变。
func NewRetriever(q *QdrantClient, emb embedding.Embedder, collection string, topK int) retriever.Retriever {
	return &qdrantRetriever{qdrant: q, embedder: emb, collection: collection, topK: topK}
}

// Retrieve 把 query 向量化并检索,返回带 score/source_file 的文档。
func (r *qdrantRetriever) Retrieve(ctx context.Context, query string, _ ...retriever.Option) ([]*schema.Document, error) {
	vecs, err := r.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("retriever: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("retriever: empty query embedding")
	}
	hits, err := r.qdrant.SearchPoints(ctx, r.collection, toFloat32(vecs[0]), r.topK, nil)
	if err != nil {
		return nil, fmt.Errorf("retriever: search: %w", err)
	}
	docs := make([]*schema.Document, 0, len(hits))
	for _, h := range hits {
		meta := map[string]any{}
		for k, v := range h.Payload {
			if k == "content" {
				continue
			}
			meta[k] = v
		}
		meta["score"] = h.Score
		content, _ := h.Payload["content"].(string)
		docs = append(docs, &schema.Document{
			ID:       h.ID,
			Content:  content,
			MetaData: meta,
		})
	}
	return docs, nil
}
