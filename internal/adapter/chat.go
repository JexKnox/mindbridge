package adapter

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"

	"github.com/JexKnox/mindbridge/internal/config"
)

// NewChatModel 按 cfg.Provider 构造一个 eino model.ChatModel。
// 当前仅实现 "openai"(OpenAI 兼容:火山 Ark / DeepSeek / 通义 / 智谱等)。
// 新增 Provider = 在此分支加 case + 引入对应 eino-ext 适配,业务代码不变。
func NewChatModel(ctx context.Context, cfg config.ModelConfig) (model.ChatModel, error) {
	switch cfg.Provider {
	case "openai":
		return newOpenAIChatModel(ctx, cfg)
	default:
		return nil, fmt.Errorf("adapter: unsupported chat provider %q", cfg.Provider)
	}
}
