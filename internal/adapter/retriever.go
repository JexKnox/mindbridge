package adapter

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/JexKnox/mindbridge/internal/metrics"
)

// rrfK 是 RRF 融合的平滑常数(业界通用取 60)。
const rrfK = 60

// qdrantRetriever 实现 eino 的 retriever.Retriever。
// hybrid=true 时做混合检索:向量召回(语义)+ 稀疏 BM25 召回(关键词),RRF 融合排序;
// 稀疏召回失败(如旧 collection 未配置稀疏向量)时自动降级为纯向量召回。
type qdrantRetriever struct {
	qdrant     *QdrantClient
	embedder   embedding.Embedder
	collection string
	topK       int
	hybrid     bool
}

// NewRetriever 构造一个面向指定 collection 的 eino Retriever。
func NewRetriever(q *QdrantClient, emb embedding.Embedder, collection string, topK int, hybrid bool) retriever.Retriever {
	return &qdrantRetriever{qdrant: q, embedder: emb, collection: collection, topK: topK, hybrid: hybrid}
}

// Retrieve 召回 topK 文档。返回文档 MetaData["score"] 为向量余弦分
// (仅被关键词召回的文档为 0),排序为 RRF 融合序。
func (r *qdrantRetriever) Retrieve(ctx context.Context, query string, _ ...retriever.Option) ([]*schema.Document, error) {
	vecs, err := r.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("retriever: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("retriever: empty query embedding")
	}
	dense, err := r.qdrant.SearchPoints(ctx, r.collection, toFloat32(vecs[0]), r.topK, nil)
	if err != nil {
		return nil, fmt.Errorf("retriever: search: %w", err)
	}

	var sparse []ScoredPoint
	if r.hybrid {
		if sv := BuildSparseVector(query); sv != nil {
			// 失败(旧 collection 无稀疏向量等)不致命,静默降级纯向量。
			sparse, _ = r.qdrant.QuerySparse(ctx, r.collection, sv, r.topK)
		}
	}

	hits := fuseRRF(dense, sparse, r.topK)

	// 检索层指标:各通道候选数与融合后数量,用于观察某一路召回异常(如稀疏路始终为 0)。
	metrics.ObserveCandidates("dense", len(dense))
	if r.hybrid {
		metrics.ObserveCandidates("sparse", len(sparse))
	}
	metrics.ObserveCandidates("fused", len(hits))

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

// fuseRRF 对两路召回做 Reciprocal Rank Fusion,返回按融合分排序的前 topK。
// 返回点的 Score 为向量余弦分(便于上层做拒答阈值判断与引用展示);
// 仅出现在关键词路的点 Score 为 0。
func fuseRRF(dense, sparse []ScoredPoint, topK int) []ScoredPoint {
	if len(sparse) == 0 {
		return dense
	}
	type entry struct {
		point ScoredPoint
		rrf   float64
	}
	byID := map[string]*entry{}
	for rank, p := range dense {
		byID[p.ID] = &entry{point: p, rrf: 1.0 / float64(rrfK+rank+1)}
	}
	for rank, p := range sparse {
		if e, ok := byID[p.ID]; ok {
			e.rrf += 1.0 / float64(rrfK+rank+1)
			continue
		}
		p.Score = 0 // 稀疏分数与余弦不同量纲,不透出
		byID[p.ID] = &entry{point: p, rrf: 1.0 / float64(rrfK+rank+1)}
	}
	entries := make([]*entry, 0, len(byID))
	for _, e := range byID {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rrf > entries[j].rrf })
	if len(entries) > topK {
		entries = entries[:topK]
	}
	out := make([]ScoredPoint, len(entries))
	for i, e := range entries {
		out[i] = e.point
	}
	return out
}
