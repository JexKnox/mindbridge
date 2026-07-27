package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/metrics"
)

// HTTPReranker 调用 OpenAI 风格的 /rerank API 做 cross-encoder 重排。
// 请求/响应格式与 Jina / Cohere / SiliconFlow / HuggingFace TEI 等主流服务兼容:
//
//	POST {base_url}/rerank {"model","query","documents":[...],"top_n":N}
//	→ {"results":[{"index":0,"relevance_score":0.98},...]}
type HTTPReranker struct {
	endpoint string
	apiKey   string
	model    string
	topN     int
	http     *http.Client
}

// NewHTTPReranker 构造 reranker。cfg.BaseURL 末尾没有 /rerank 时自动拼接。
func NewHTTPReranker(cfg config.ModelConfig, topN int) *HTTPReranker {
	endpoint := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/rerank") {
		endpoint += "/rerank"
	}
	return &HTTPReranker{
		endpoint: endpoint,
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		topN:     topN,
		http:     &http.Client{},
	}
}

// Rerank 对 docs 按与 query 的相关度重排,返回前 topN 个。
// 重排相关度写入 MetaData["rerank_score"];MetaData["score"](向量余弦)保留,
// 拒答阈值仍基于余弦分判断,量纲不混用。
func (r *HTTPReranker) Rerank(ctx context.Context, query string, docs []*schema.Document) (ranked []*schema.Document, err error) {
	if len(docs) == 0 {
		return docs, nil
	}
	defer func(start time.Time) { metrics.ObserveModel("rerank", start, err) }(time.Now())
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	body := map[string]any{
		"model":     r.model,
		"query":     query,
		"documents": texts,
		"top_n":     r.topN,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reranker: request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reranker: status %d: %s", resp.StatusCode, string(respBody))
	}
	var out struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("reranker: decode response: %w", err)
	}

	sort.Slice(out.Results, func(i, j int) bool {
		return out.Results[i].RelevanceScore > out.Results[j].RelevanceScore
	})
	reranked := make([]*schema.Document, 0, len(out.Results))
	for _, res := range out.Results {
		if res.Index < 0 || res.Index >= len(docs) {
			continue
		}
		d := docs[res.Index]
		d.MetaData["rerank_score"] = res.RelevanceScore
		reranked = append(reranked, d)
		if r.topN > 0 && len(reranked) >= r.topN {
			break
		}
	}
	return reranked, nil
}
