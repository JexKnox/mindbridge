package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"

	"github.com/google/uuid"
)

// qdrantIndexer 实现 eino 的 indexer.Indexer:把文档内容向量化后写入 Qdrant。
type qdrantIndexer struct {
	qdrant     *QdrantClient
	embedder   embedding.Embedder
	collection string
}

// NewIndexer 构造一个面向指定 collection 的 eino Indexer。
// embedder 必须与建库时绑定的 embedding 模型一致(维度匹配)。
func NewIndexer(q *QdrantClient, emb embedding.Embedder, collection string) indexer.Indexer {
	return &qdrantIndexer{qdrant: q, embedder: emb, collection: collection}
}

// Store 对 docs 内容批量向量化并写入 Qdrant,返回生成的点 ID。
func (i *qdrantIndexer) Store(ctx context.Context, docs []*schema.Document, _ ...indexer.Option) ([]string, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	texts := make([]string, len(docs))
	for j, d := range docs {
		texts[j] = d.Content
	}
	vecs, err := i.embedder.EmbedStrings(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("indexer: embed docs: %w", err)
	}
	if len(vecs) != len(docs) {
		return nil, fmt.Errorf("indexer: embedding count mismatch: got %d want %d", len(vecs), len(docs))
	}

	points := make([]Point, len(docs))
	ids := make([]string, len(docs))
	for j, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		ids[j] = id
		// payload 同时存原始内容,便于检索后直接拼 prompt;source_file/chunk_index 由 pipeline 写入 MetaData。
		payload := map[string]any{"content": d.Content}
		for k, v := range d.MetaData {
			payload[k] = v
		}
		points[j] = Point{ID: id, Vector: toFloat32(vecs[j]), Payload: payload}
	}
	if err := i.qdrant.UpsertPoints(ctx, i.collection, points); err != nil {
		return nil, fmt.Errorf("indexer: upsert: %w", err)
	}
	return ids, nil
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for j, x := range v {
		out[j] = float32(x)
	}
	return out
}
