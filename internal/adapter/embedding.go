package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/embedding"

	"github.com/JexKnox/mindbridge/internal/config"
)

// NewEmbedder 按 cfg.Provider 构造一个 eino embedding.Embedder。
func NewEmbedder(ctx context.Context, cfg config.ModelConfig) (embedding.Embedder, error) {
	switch cfg.Provider {
	case "openai":
		return newOpenAIEmbedder(ctx, cfg)
	default:
		return nil, fmt.Errorf("adapter: unsupported embedding provider %q", cfg.Provider)
	}
}

// EmbedDim 探测 embedding 模型输出的向量维度。
// 用于建库时把维度绑定到 KB 元信息与 Qdrant collection(维度一旦确定不可改)。
func EmbedDim(ctx context.Context, emb embedding.Embedder) (int, error) {
	vecs, err := emb.EmbedStrings(ctx, []string{"mindbridge dimension probe"})
	if err != nil {
		return 0, fmt.Errorf("probe embedding dim: %w", err)
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return 0, fmt.Errorf("probe embedding dim: empty vector returned")
	}
	return len(vecs[0]), nil
}
