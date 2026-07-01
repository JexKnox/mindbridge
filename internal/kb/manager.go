package kb

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/config"
)

// Manager 编排"建库":用请求中的 embedding 配置构造 embedder → 探测维度 →
// 建 Qdrant collection(kb_{id})→ 写 KB 元信息。维度一旦绑定不可随意更改。
type Manager struct {
	repo   *Repo
	qdrant *adapter.QdrantClient
	// defaultEmb 是建库时若请求未单独指定 embedding 连接信息时使用的默认配置。
	defaultEmb config.ModelConfig
}

func NewManager(repo *Repo, q *adapter.QdrantClient, defaultEmb config.ModelConfig) *Manager {
	return &Manager{repo: repo, qdrant: q, defaultEmb: defaultEmb}
}

// Create 建立一个新知识库。
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*KB, error) {
	if req.Name == "" || req.EmbeddingModel == "" {
		return nil, fmt.Errorf("kb: name and embedding_model are required")
	}
	embCfg := m.defaultEmb
	embCfg.Model = req.EmbeddingModel
	if req.EmbeddingBaseURL != "" {
		embCfg.BaseURL = req.EmbeddingBaseURL
	}
	if req.EmbeddingAPIKey != "" {
		embCfg.APIKey = req.EmbeddingAPIKey
	}

	emb, err := adapter.NewEmbedder(ctx, embCfg)
	if err != nil {
		return nil, fmt.Errorf("kb: build embedder: %w", err)
	}
	dim, err := adapter.EmbedDim(ctx, emb)
	if err != nil {
		return nil, fmt.Errorf("kb: probe dim: %w", err)
	}

	id := uuid.NewString()
	collection := "kb_" + id
	if err := m.qdrant.EnsureCollection(ctx, collection, dim); err != nil {
		return nil, fmt.Errorf("kb: ensure collection: %w", err)
	}

	kb := &KB{
		ID:             id,
		Name:           req.Name,
		Collection:     collection,
		EmbeddingModel: req.EmbeddingModel,
		EmbeddingDim:   dim,
	}
	if err := m.repo.Create(ctx, kb); err != nil {
		// 元信息写入失败:collection 已建但无元信息,留待后续清理工具处理(本期不实现 DeleteCollection)。
		return nil, err
	}
	return kb, nil
}

// Get 透传到 repo。
func (m *Manager) Get(ctx context.Context, id string) (*KB, error) { return m.repo.Get(ctx, id) }

// List 透传到 repo。
func (m *Manager) List(ctx context.Context) ([]*KB, error) { return m.repo.List(ctx) }
