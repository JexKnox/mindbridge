package adapter

import (
	"context"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	openaiemb "github.com/cloudwego/eino-ext/components/embedding/openai"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/JexKnox/mindbridge/internal/config"
)

// newOpenAIChatModel 构造 OpenAI 兼容的 eino ChatModel。
// 通过 BaseURL 切换到火山 Ark / DeepSeek / 通义 / 智谱 / Ollama(OpenAI 兼容端点)等。
func newOpenAIChatModel(ctx context.Context, cfg config.ModelConfig) (model.ChatModel, error) {
	return openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
}

// newOpenAIEmbedder 构造 OpenAI 兼容的 eino Embedder。
func newOpenAIEmbedder(ctx context.Context, cfg config.ModelConfig) (embedding.Embedder, error) {
	return openaiemb.NewEmbedder(ctx, &openaiemb.EmbeddingConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
}
