// Package kb 是"知识库"领域:知识库是头等概念,任何检索/写入都必须绑定 kb_id。
// 元信息(名称、collection 名、embedding 模型与维度)存于 Postgres;
// 向量数据存于 Qdrant 的 kb_{id} collection,实现库间隔离。
package kb

import "time"

// KB 是知识库元信息。
type KB struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Collection     string    `json:"collection"`      // 形如 kb_{id}
	EmbeddingModel string    `json:"embedding_model"` // 绑定,同一 KB 内不可变
	EmbeddingDim   int       `json:"embedding_dim"`   // 由 embedding 模型探测得出
	CreatedAt      time.Time `json:"created_at"`
}

// CreateRequest 是建库请求。
type CreateRequest struct {
	Name           string `json:"name"`
	EmbeddingModel string `json:"embedding_model"`
	// 以下允许建库时单独指定 embedding 的连接信息(可与 chat 不同 Provider/端点)。
	EmbeddingBaseURL string `json:"embedding_base_url"`
	EmbeddingAPIKey  string `json:"embedding_api_key"`
}
