package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"

	"github.com/google/uuid"
)

// IndexerConfig 控制批量写入行为。
type IndexerConfig struct {
	EmbedBatch  int // 单次 embedding 请求的文本数,默认 64(过大易触发 provider 限流/超长)
	UpsertBatch int // 单次 Qdrant upsert 的点数,默认 128
	MaxRetries  int // 单批失败重试次数(指数退避),默认 3
}

func (c *IndexerConfig) applyDefaults() {
	if c.EmbedBatch <= 0 {
		c.EmbedBatch = 64
	}
	if c.UpsertBatch <= 0 {
		c.UpsertBatch = 128
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
}

// qdrantIndexer 实现 eino 的 indexer.Indexer:把文档内容向量化后写入 Qdrant。
// 同时生成 bm25 稀疏向量(关键词召回用);embedding 与 upsert 均分批 + 重试。
type qdrantIndexer struct {
	qdrant     *QdrantClient
	embedder   embedding.Embedder
	collection string
	cfg        IndexerConfig
}

// NewIndexer 构造一个面向指定 collection 的 eino Indexer。
// embedder 必须与建库时绑定的 embedding 模型一致(维度匹配)。
func NewIndexer(q *QdrantClient, emb embedding.Embedder, collection string, cfg IndexerConfig) indexer.Indexer {
	cfg.applyDefaults()
	return &qdrantIndexer{qdrant: q, embedder: emb, collection: collection, cfg: cfg}
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

	// embedding 分批 + 重试:provider 侧常见限流,一次性全量请求易失败。
	vecs := make([][]float64, 0, len(docs))
	for start := 0; start < len(texts); start += i.cfg.EmbedBatch {
		end := min(start+i.cfg.EmbedBatch, len(texts))
		batch, err := withRetry(ctx, i.cfg.MaxRetries, func() ([][]float64, error) {
			return i.embedder.EmbedStrings(ctx, texts[start:end])
		})
		if err != nil {
			return nil, fmt.Errorf("indexer: embed docs [%d:%d]: %w", start, end, err)
		}
		if len(batch) != end-start {
			return nil, fmt.Errorf("indexer: embedding count mismatch: got %d want %d", len(batch), end-start)
		}
		vecs = append(vecs, batch...)
	}

	points := make([]Point, len(docs))
	ids := make([]string, len(docs))
	for j, d := range docs {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		ids[j] = id
		// payload 同时存原始内容,便于检索后直接拼 prompt;
		// source_file/chunk_index/embedding_model 等由 pipeline 写入 MetaData。
		payload := map[string]any{"content": d.Content}
		for k, v := range d.MetaData {
			payload[k] = v
		}
		points[j] = Point{
			ID:      id,
			Vector:  toFloat32(vecs[j]),
			Sparse:  BuildSparseVector(d.Content),
			Payload: payload,
		}
	}

	// upsert 分批 + 重试:Qdrant 对单请求体积有限制,大文档一次写入易超限。
	for start := 0; start < len(points); start += i.cfg.UpsertBatch {
		end := min(start+i.cfg.UpsertBatch, len(points))
		if _, err := withRetry(ctx, i.cfg.MaxRetries, func() (struct{}, error) {
			return struct{}{}, i.qdrant.UpsertPoints(ctx, i.collection, points[start:end])
		}); err != nil {
			return nil, fmt.Errorf("indexer: upsert [%d:%d]: %w", start, end, err)
		}
	}
	return ids, nil
}

// withRetry 以指数退避(1s/2s/4s…)重试 fn,直到成功、耗尽次数或 ctx 取消。
func withRetry[T any](ctx context.Context, maxRetries int, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Second << (attempt - 1)
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(backoff):
			}
		}
		v, err := fn()
		if err == nil {
			return v, nil
		}
		lastErr = err
	}
	return zero, fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func toFloat32(v []float64) []float32 {
	out := make([]float32, len(v))
	for j, x := range v {
		out[j] = float32(x)
	}
	return out
}
