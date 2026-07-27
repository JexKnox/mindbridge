// Package agent 实现 agentic RAG 链路:检索→agent 决策→调工具→生成。
// 与 internal/rag(固定"检索→生成")并列,互不影响。
//
// agent 每次请求动态组装工具:[可选] KB 检索工具 + 内置 skill 工具 + MCP 工具,
// 交给 eino ReAct agent 自主决策何时调用哪个工具,流式输出回答。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/JexKnox/mindbridge/internal/adapter"
	"github.com/JexKnox/mindbridge/internal/agent/skills"
	"github.com/JexKnox/mindbridge/internal/cache"
	"github.com/JexKnox/mindbridge/internal/config"
	"github.com/JexKnox/mindbridge/internal/kb"
	mindmcp "github.com/JexKnox/mindbridge/internal/mcp"
)

// EventSink 由 handler 注入:agent 把 (event, data) 写成 SSE 帧。
// event=="" 表示默认 data 事件(逐 token);其他为具名事件(tool_call/tool_result/done/error)。
type EventSink func(event string, data string)

// Service 编排 agent 链路。与 rag.Service 并列,不复用其检索逻辑——
// 这里检索是 agent 自主调用的工具,而非固定前置步骤。
type Service struct {
	topK    int
	skills  *skills.Skills
	mcpRepo *mindmcp.Repo
	qdrant  *adapter.QdrantClient
	logger  *zap.Logger
	caches  *cache.RAGCaches
}

// NewService 装配 agent 服务。chatCfg/embCfg 在每次请求时由 handler 传入(与 rag 路径一致)。
// caches 可为 nil(不启用查询向量缓存)。
func NewService(
	topK int,
	sk *skills.Skills,
	mcpRepo *mindmcp.Repo,
	qdrant *adapter.QdrantClient,
	logger *zap.Logger,
	caches *cache.RAGCaches,
) *Service {
	return &Service{
		topK:    topK,
		skills:  sk,
		mcpRepo: mcpRepo,
		qdrant:  qdrant,
		logger:  logger,
		caches:  caches,
	}
}

// Answer 在给定 KB(可空)上跑 agent。onEvent 推送 token / 工具调用 / 结束事件。
func (s *Service) Answer(
	ctx context.Context,
	query string,
	chatCfg config.ModelConfig,
	embCfg config.ModelConfig,
	k *kb.KB,
	onEvent EventSink,
) error {
	// 1. 构造 chat model,断言其支持工具调用。
	chatModel, err := adapter.NewChatModel(ctx, chatCfg)
	if err != nil {
		return fmt.Errorf("agent: build chat model: %w", err)
	}
	tcc, ok := chatModel.(model.ToolCallingChatModel)
	if !ok {
		return fmt.Errorf("agent: chat model %T 未实现 ToolCallingChatModel,无法做工具调用", chatModel)
	}

	// 2. 组装工具列表 + MCP 连接 closer。
	tools, closers, err := s.assembleTools(ctx, embCfg, k)
	if err != nil {
		return fmt.Errorf("agent: assemble tools: %w", err)
	}
	defer func() {
		for _, cl := range closers {
			_ = cl()
		}
	}()

	// 3. 包装工具:在每次调用前后发 tool_call/tool_result 事件。
	wrapped := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		it, ok := t.(tool.InvokableTool)
		if !ok {
			wrapped = append(wrapped, t) // 非 InvokableTool 直接透传(暂不发事件)
			continue
		}
		wrapped = append(wrapped, wrapToolWithEvents(it, onEvent))
	}

	// 4. 构造 ReAct agent。
	ag, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: tcc,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: wrapped},
		MaxStep:          24,
	})
	if err != nil {
		return fmt.Errorf("agent: new react agent: %w", err)
	}

	// 5. 流式生成。
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt(k)),
		schema.UserMessage(query),
	}
	stream, err := ag.Stream(ctx, messages)
	if err != nil {
		return fmt.Errorf("agent: stream: %w", err)
	}
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("agent: recv stream: %w", err)
		}
		if msg.Content != "" {
			onEvent("", msg.Content)
		}
	}
	onEvent("done", "")
	return nil
}

// assembleTools 组装本轮可用工具:KB 检索(若绑 KB)+ skills + MCP。
func (s *Service) assembleTools(ctx context.Context, embCfg config.ModelConfig, k *kb.KB) ([]tool.BaseTool, []func() error, error) {
	var tools []tool.BaseTool
	var closers []func() error

	// KB 检索工具。
	if k != nil {
		kbTool, err := s.newKBSearchTool(ctx, embCfg, k)
		if err != nil {
			s.logger.Warn("agent: build kb_search tool failed", zap.Error(err))
		} else {
			tools = append(tools, kbTool)
		}
	}

	// 内置 skill 工具。
	if s.skills != nil {
		for _, t := range s.skills.List() {
			tools = append(tools, t)
		}
	}

	// MCP 工具:遍历已注册 server,逐个连接拉取。单个失败不阻断整体。
	servers, err := s.mcpRepo.List(ctx)
	if err != nil {
		s.logger.Warn("agent: list mcp servers failed", zap.Error(err))
	} else {
		for _, srv := range servers {
			mcpTools, closeFn, err := adapter.NewMCPTools(ctx, srv)
			if err != nil {
				s.logger.Warn("agent: connect mcp server failed",
					zap.String("server", srv.Name), zap.Error(err))
				continue
			}
			tools = append(tools, mcpTools...)
			closers = append(closers, closeFn)
		}
	}

	return tools, closers, nil
}

// newKBSearchTool 把"检索指定 KB"包成一个 eino InvokableTool。
// agent 自主决定是否调用、用什么 query 检索。
func (s *Service) newKBSearchTool(ctx context.Context, embCfg config.ModelConfig, k *kb.KB) (tool.InvokableTool, error) {
	embCfg = withModel(embCfg, k.EmbeddingModel)
	emb, err := adapter.NewEmbedder(ctx, embCfg)
	if err != nil {
		return nil, fmt.Errorf("build embedder: %w", err)
	}
	if s.caches != nil {
		emb = adapter.NewCachedEmbedder(emb, embCfg.Model, s.caches.Embedding)
	}
	ret := adapter.NewRetriever(s.qdrant, emb, k.Collection, s.topK, true)

	type kbSearchInput struct {
		Query string `json:"query" jsonschema:"description=用于在知识库中做语义检索的自然语言查询"`
	}
	return utils.InferTool(
		"kb_search",
		fmt.Sprintf("在知识库「%s」中检索与查询语义相关的片段并返回带编号的文本。回答需依赖知识库内容时调用。", k.Name),
		func(ctx context.Context, in kbSearchInput) (string, error) {
			docs, err := ret.Retrieve(ctx, in.Query)
			if err != nil {
				return "", fmt.Errorf("retrieve: %w", err)
			}
			if len(docs) == 0 {
				return "未检索到相关内容。", nil
			}
			var b strings.Builder
			for i, d := range docs {
				fmt.Fprintf(&b, "[%d] %s\n\n", i+1, d.Content)
			}
			return strings.TrimSpace(b.String()), nil
		},
	)
}

func systemPrompt(k *kb.KB) string {
	var b strings.Builder
	b.WriteString("你是一个能调用工具的智能助手。请按需调用工具获取信息,再作答。\n")
	b.WriteString("原则:\n")
	b.WriteString("- 涉及实时信息(时间、网页内容等)时调用对应工具,不要凭记忆猜测。\n")
	b.WriteString("- 需要精确计算时使用 calculator。\n")
	if k != nil {
		fmt.Fprintf(&b, "- 用户已绑定知识库「%s」,回答该库相关问题时调用 kb_search 检索,并基于检索结果作答;若检索结果不足以回答请如实说明。\n", k.Name)
	} else {
		b.WriteString("- 当前未绑定知识库,无法检索私有文档。\n")
	}
	b.WriteString("- 工具结果为原始数据,请据此组织自然语言回答,不要直接复读。\n")
	return b.String()
}

// eventTool 包装 InvokableTool,在调用前后发 SSE 事件。
type eventTool struct {
	tool.InvokableTool
	name string
	emit EventSink
}

func wrapToolWithEvents(inner tool.InvokableTool, emit EventSink) *eventTool {
	info, err := inner.Info(context.Background())
	name := ""
	if err == nil && info != nil {
		name = info.Name
	}
	return &eventTool{InvokableTool: inner, name: name, emit: emit}
}

// InvokableRun 在调用前发 tool_call,调用后发 tool_result。
func (t *eventTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	t.emit("tool_call", mustJSON(map[string]any{"name": t.name, "args": args}))
	out, err := t.InvokableTool.InvokableRun(ctx, args, opts...)
	if err != nil {
		t.emit("tool_result", mustJSON(map[string]any{"name": t.name, "error": err.Error()}))
		return out, err
	}
	t.emit("tool_result", mustJSON(map[string]any{"name": t.name, "result": truncate(out, 2000)}))
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func mustJSON(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// withModel 返回一份 ModelConfig 副本,Model 字段替换为指定模型。
// 建库时 KB 绑定了 embedding 模型名,检索须用同名模型(维度才能对齐)。
func withModel(cfg config.ModelConfig, m string) config.ModelConfig {
	cfg.Model = m
	return cfg
}
