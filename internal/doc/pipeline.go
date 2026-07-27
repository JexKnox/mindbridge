package doc

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"

	"github.com/JexKnox/mindbridge/internal/kb"
)

// Pipeline 编排单次文档摄取:读取文件 → 切分 → (Indexer 内部向量化) → 写入 Qdrant。
// 业务层只依赖 eino 的 indexer.Indexer 接口,不感知具体向量库。
type Pipeline struct {
	splitter *Splitter
}

func NewPipeline(cfg SplitterConfig) *Pipeline {
	return &Pipeline{splitter: NewSplitter(cfg)}
}

// Ingest 读入文件内容并索引到 KB 对应的 collection。
// markdown 走结构化切分(按标题分节、代码/表格整块保留),其余格式解析为纯文本后常规切分。
// idx 必须已绑定到 kb.Collection;同一 KB 强制使用同一 embedding 模型(维度匹配),
// 由调用方(kb.Manager 建库时绑定)保证。
func (p *Pipeline) Ingest(ctx context.Context, k *kb.KB, idx indexer.Indexer, fileName string, r io.Reader) (int, error) {
	source := filepath.Base(fileName)
	ext := strings.ToLower(filepath.Ext(fileName))

	var docs []*schema.Document
	switch ext {
	case ".md", ".markdown":
		raw, err := io.ReadAll(r)
		if err != nil {
			return 0, fmt.Errorf("doc: read %s: %w", fileName, err)
		}
		docs = p.splitter.SplitMarkdown(string(raw), source)
	default:
		text, err := Load(fileName, r)
		if err != nil {
			return 0, fmt.Errorf("doc: load: %w", err)
		}
		docs = p.splitter.Split(text, source)
	}
	if len(docs) == 0 {
		return 0, nil
	}

	// 审计字段:每个 chunk 记录所用 embedding 模型,便于后续迁移/排查维度不一致。
	for _, d := range docs {
		d.MetaData["embedding_model"] = k.EmbeddingModel
	}

	if _, err := idx.Store(ctx, docs); err != nil {
		return 0, fmt.Errorf("doc: store: %w", err)
	}
	return len(docs), nil
}
