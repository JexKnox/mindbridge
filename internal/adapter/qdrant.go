// Package adapter 是"通用"硬约束的落点:所有 Provider / 向量库的具体实现都集中在此包。
// 业务层(internal/kb、internal/doc、internal/rag)只依赖 eino 的接口,
// 切换 Provider 或向量库时改动不应溢出本包。
//
// Qdrant 当前通过 REST API 直接调用(稳定、零额外依赖),实现 eino 的
// indexer.Indexer / retriever.Retriever 接口。后续若改用 eino-ext qdrant,
// 只需替换本包内的 indexer/retriever 实现,业务代码无感。
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// QdrantClient 是 Qdrant REST API 的薄封装。
type QdrantClient struct {
	base   string // 形如 http://localhost:6333
	apiKey string
	http   *http.Client
}

// NewQdrantClient 构造一个 Qdrant REST 客户端。
func NewQdrantClient(host string, port int, apiKey string) *QdrantClient {
	return &QdrantClient{
		base:   fmt.Sprintf("http://%s:%d", host, port),
		apiKey: apiKey,
		http:   &http.Client{},
	}
}

// sparseVectorName 是关键词召回用的稀疏向量名(BM25 风格,IDF 由 Qdrant 计算)。
const sparseVectorName = "bm25"

// CreateCollection 创建一个使用未命名向量(默认向量)、Cosine 距离的 collection,
// 并附带一个 IDF 加权的稀疏向量(bm25)用于关键词召回。
// 同一 KB 强制使用同一 embedding 模型;维度一旦确定不可随意更改(变更需重建)。
func (c *QdrantClient) CreateCollection(ctx context.Context, name string, dim int) error {
	body := map[string]any{
		"vectors": map[string]any{
			"size":     dim,
			"distance": "Cosine",
		},
		"sparse_vectors": map[string]any{
			sparseVectorName: map[string]any{"modifier": "idf"},
		},
		"on_disk_payload": true,
	}
	_, err := c.do(ctx, http.MethodPut, "/collections/"+name+"?timeout=60", body)
	return err
}

// CollectionExists 返回 collection 是否存在。
func (c *QdrantClient) CollectionExists(ctx context.Context, name string) (bool, error) {
	resp, err := c.doRaw(ctx, http.MethodGet, "/collections/"+name, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("qdrant: unexpected status %d checking collection %s", resp.StatusCode, name)
}

// EnsureCollection 幂等创建 collection 并为常用 payload 字段建索引。
func (c *QdrantClient) EnsureCollection(ctx context.Context, name string, dim int) error {
	exists, err := c.CollectionExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		if err := c.CreateCollection(ctx, name, dim); err != nil {
			return err
		}
	}
	// 为 source_file 建 keyword 索引,便于按文件删除/过滤。
	for _, field := range []string{"source_file", "kb_id"} {
		if err := c.CreatePayloadIndex(ctx, name, field); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSparseVectors 为已有 collection 补建 bm25 稀疏向量配置(幂等)。
// 旧版建的 collection 没有稀疏向量,补建后新写入的点才带关键词索引;
// 存量点需重新上传文档才能被关键词召回。
func (c *QdrantClient) EnsureSparseVectors(ctx context.Context, name string) error {
	raw, err := c.do(ctx, http.MethodGet, "/collections/"+name, nil)
	if err != nil {
		return err
	}
	var info struct {
		Result struct {
			Config struct {
				Params struct {
					SparseVectors map[string]any `json:"sparse_vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return fmt.Errorf("qdrant: decode collection info: %w", err)
	}
	if _, ok := info.Result.Config.Params.SparseVectors[sparseVectorName]; ok {
		return nil
	}
	body := map[string]any{
		"sparse_vectors": map[string]any{
			sparseVectorName: map[string]any{"modifier": "idf"},
		},
	}
	_, err = c.do(ctx, http.MethodPatch, "/collections/"+name+"?timeout=60", body)
	return err
}

// CreatePayloadIndex 为 payload 字段建索引(keyword 类型)。
func (c *QdrantClient) CreatePayloadIndex(ctx context.Context, collection, field string) error {
	body := map[string]any{
		"field_name":   field,
		"field_schema": "keyword",
	}
	_, err := c.do(ctx, http.MethodPut, "/collections/"+collection+"/index?wait=true", body)
	return err
}

// Point 是待写入 Qdrant 的点。Sparse 可选(nil 表示仅稠密向量)。
type Point struct {
	ID      string
	Vector  []float32
	Sparse  *SparseVector
	Payload map[string]any
}

// UpsertPoints 批量写入点。带稀疏向量的点用命名向量格式(未命名稠密向量的名字是 "")。
func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []Point) error {
	raw := make([]map[string]any, len(points))
	for i, p := range points {
		var vector any = p.Vector
		if p.Sparse != nil {
			vector = map[string]any{
				"":               p.Vector,
				sparseVectorName: p.Sparse,
			}
		}
		raw[i] = map[string]any{
			"id":      p.ID,
			"vector":  vector,
			"payload": p.Payload,
		}
	}
	body := map[string]any{"points": raw}
	_, err := c.do(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", body)
	return err
}

// ScoredPoint 是检索返回的命中点。
type ScoredPoint struct {
	ID      string         `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

// SearchPoints 用向量检索 topK 个最近邻(可选 payload 过滤)。
// filter 为 nil 表示无过滤;否则形如 {"must":[{"key":"k","match":{"value":"v"}}]}。
func (c *QdrantClient) SearchPoints(ctx context.Context, collection string, vector []float32, topK int, filter map[string]any) ([]ScoredPoint, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
		"with_vector":  false,
	}
	if filter != nil {
		body["filter"] = filter
	}
	raw, err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result []ScoredPoint `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("qdrant: decode search result: %w", err)
	}
	return out.Result, nil
}

// QuerySparse 用稀疏向量(关键词)检索 topK。要求 collection 已配置 bm25 稀疏向量;
// 旧 collection 未配置时 Qdrant 返回错误,调用方应降级为纯向量召回。
func (c *QdrantClient) QuerySparse(ctx context.Context, collection string, sv *SparseVector, topK int) ([]ScoredPoint, error) {
	body := map[string]any{
		"query":        sv,
		"using":        sparseVectorName,
		"limit":        topK,
		"with_payload": true,
	}
	raw, err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/query", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result struct {
			Points []ScoredPoint `json:"points"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("qdrant: decode query result: %w", err)
	}
	return out.Result.Points, nil
}

// DeleteByPayload 删除 payload[key]==value 的所有点(用于文档更新时清旧 chunk)。
func (c *QdrantClient) DeleteByPayload(ctx context.Context, collection, key, value string) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": key, "match": map[string]any{"value": value}},
			},
		},
	}
	_, err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", body)
	return err
}

// do 发起带 JSON body 的请求,返回响应 body(状态码非 2xx 视为错误)。
func (c *QdrantClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	resp, err := c.doRaw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qdrant %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	return raw, nil
}

func (c *QdrantClient) doRaw(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Api-Key", c.apiKey)
	}
	return c.http.Do(req)
}
