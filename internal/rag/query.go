// Package rag 实现在线查询路径:Retriever → [可选 Reranker] → ChatModel → 流式响应。
package rag

import "context"

// QueryRewriter 负责把用户原始 query 改写成更适合检索的形式(指代消解、多意图拆分等)。
// 本期为占位:直接返回原 query。后续接入 HyDE / LLM 改写时在此实现,上层无感。
type QueryRewriter struct{}

func NewQueryRewriter() *QueryRewriter { return &QueryRewriter{} }

// Rewrite 当前为直通;留 TODO 接 HyDE。
func (q *QueryRewriter) Rewrite(ctx context.Context, query string) (string, error) {
	return query, nil
}
