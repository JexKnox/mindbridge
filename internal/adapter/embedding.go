package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/metrics"
)

// NewEmbedder 按 cfg.Provider 构造一个 eino embedding.Embedder(带 API 延迟/错误埋点)。
func NewEmbedder(ctx context.Context, cfg config.ModelConfig) (embedding.Embedder, error) {
	var (
		emb embedding.Embedder
		err error
	)
	switch cfg.Provider {
	case "openai":
		emb, err = newOpenAIEmbedder(ctx, cfg)
	default:
		return nil, fmt.Errorf("adapter: unsupported embedding provider %q", cfg.Provider)
	}
	if err != nil {
		return nil, err
	}
	return &timedEmbedder{inner: emb}, nil
}

// timedEmbedder 给 embedding API 调用加延迟/错误指标。
// 包在缓存装饰器内侧:缓存命中不算一次 API 调用。
type timedEmbedder struct {
	inner embedding.Embedder
}

func (t *timedEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	start := time.Now()
	vecs, err := t.inner.EmbedStrings(ctx, texts, opts...)
	metrics.ObserveModel("embedding", start, err)
	return vecs, err
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
