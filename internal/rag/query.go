// Package rag 实现在线查询路径:Query 改写 → Retriever(混合检索)→ [可选 Reranker] → ChatModel → 流式响应。
package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// QueryRewriter 把用户原始 query 改写成多个更适合检索的形式(指代消解、多角度表述),
// 多路并行召回后由上层去重合并。chat 为 nil 时直通(仅返回原 query)。
// 改写建议用便宜的小模型:失败时静默降级,不影响主链路。
type QueryRewriter struct {
	chat model.ChatModel
	n    int // 改写出的 query 数(不含原 query)
}

// NewQueryRewriter 构造改写器。chat 为 nil 或 n<=0 时退化为直通。
func NewQueryRewriter(chat model.ChatModel, n int) *QueryRewriter {
	return &QueryRewriter{chat: chat, n: n}
}

const rewritePrompt = `你是检索查询改写助手。把用户问题改写成 %d 个语义等价但表述不同的检索查询,便于向量检索召回更多相关文档。
要求:
- 每行一个查询,不要编号、不要解释、不要其它输出
- 保留问题中的关键实体与术语,可补充同义词或展开缩写
- 使用与原问题相同的语言`

// Rewrite 返回检索用的 query 列表,原 query 恒为第一个。
// LLM 调用失败不阻断主链路:降级为仅原 query。
func (q *QueryRewriter) Rewrite(ctx context.Context, query string) []string {
	queries := []string{query}
	if q.chat == nil || q.n <= 0 {
		return queries
	}

	msgs := []*schema.Message{
		schema.SystemMessage(fmt.Sprintf(rewritePrompt, q.n)),
		schema.UserMessage(query),
	}
	resp, err := q.chat.Generate(ctx, msgs)
	if err != nil {
		return queries
	}

	seen := map[string]bool{query: true}
	for _, line := range strings.Split(resp.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		queries = append(queries, line)
		if len(queries) > q.n {
			break
		}
	}
	return queries
}
