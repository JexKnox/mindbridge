package adapter

import (
	"context"

	"github.com/cloudwego/eino/components/embedding"

	"github.com/JexKnox/mindbridge/internal/cache"
)

// cachedEmbedder 装饰 embedding.Embedder,按「模型名 + 文本哈希」缓存向量。
// 只应包在查询侧(retriever):查询重复率高、命中即省一次 API 调用;
// 文档摄取侧不要包(文档内容基本不重复,只会挤占缓存)。
type cachedEmbedder struct {
	inner embedding.Embedder
	model string
	cache *cache.LRU[[]float64]
}

// NewCachedEmbedder 包装 emb,命中缓存的文本跳过远程调用。c 为 nil 时原样返回 emb。
func NewCachedEmbedder(emb embedding.Embedder, model string, c *cache.LRU[[]float64]) embedding.Embedder {
	if c == nil {
		return emb
	}
	return &cachedEmbedder{inner: emb, model: model, cache: c}
}

func (e *cachedEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	out := make([][]float64, len(texts))
	var missTexts []string
	var missIdx []int
	for i, t := range texts {
		if v, ok := e.cache.Get(e.key(t)); ok {
			out[i] = v
			continue
		}
		missTexts = append(missTexts, t)
		missIdx = append(missIdx, i)
	}
	if len(missTexts) == 0 {
		return out, nil
	}
	vecs, err := e.inner.EmbedStrings(ctx, missTexts, opts...)
	if err != nil {
		return nil, err
	}
	if len(vecs) != len(missTexts) {
		// 数量不符时不缓存,直接透传错误语义给上层(与 indexer 的校验一致)。
		return e.inner.EmbedStrings(ctx, texts, opts...)
	}
	for j, v := range vecs {
		out[missIdx[j]] = v
		e.cache.Set(e.key(missTexts[j]), v)
	}
	return out, nil
}

func (e *cachedEmbedder) key(text string) string {
	return "emb:" + e.model + ":" + cache.Hash(text)
}
