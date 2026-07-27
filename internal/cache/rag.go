package cache

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

// RetrievalEntry 是检索+重排结果缓存的值:最终文档列表 + 重排前最高余弦分(拒答判断用)。
type RetrievalEntry struct {
	Docs     []*schema.Document
	MaxScore float64
}

// RAGCaches 汇总 RAG 链路的共享缓存,main 构造一份、各 handler 复用。
type RAGCaches struct {
	// Embedding 缓存查询向量,key = 模型名 + hash(文本)。只用于查询侧,
	// 文档 embedding 不缓存(基本不重复,徒耗内存)。
	Embedding *LRU[[]float64]
	// Rewrite 缓存 query 改写结果,key = 改写数 + hash(原 query)。
	Rewrite *LRU[[]string]
	// Retrieval 缓存重排后的最终召回,key 含 kb 版本号(见 Versions)与检索配置。
	Retrieval *LRU[RetrievalEntry]
	// KBVersion 为每个 KB 维护版本号,文档变更时 Bump 使检索缓存失效。
	KBVersion *Versions
}

// NewRAGCaches 用默认容量/TTL 构造缓存集合:
// embedding 2048 条 × 1h(向量小、命中率高);改写 512 条 × 1h;
// 检索结果 256 条 × 15min(条目大,且知识可能更新,TTL 兜底)。
func NewRAGCaches() *RAGCaches {
	return &RAGCaches{
		Embedding: NewLRU[[]float64](2048, time.Hour),
		Rewrite:   NewLRU[[]string](512, time.Hour),
		Retrieval: NewLRU[RetrievalEntry](256, 15*time.Minute),
		KBVersion: NewVersions(),
	}
}
