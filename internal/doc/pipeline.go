package doc

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/cloudwego/eino/components/indexer"

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
// idx 必须已绑定到 kb.Collection;同一 KB 强制使用同一 embedding 模型(维度匹配),
// 由调用方(kb.Manager 建库时绑定)保证。
func (p *Pipeline) Ingest(ctx context.Context, kb *kb.KB, idx indexer.Indexer, fileName string, r io.Reader) (int, error) {
	text, err := Load(fileName, r)
	if err != nil {
		return 0, fmt.Errorf("doc: load: %w", err)
	}
	source := filepath.Base(fileName)
	docs := p.splitter.Split(text, source)
	if len(docs) == 0 {
		return 0, nil
	}
	if _, err := idx.Store(ctx, docs); err != nil {
		return 0, fmt.Errorf("doc: store: %w", err)
	}
	return len(docs), nil
}
