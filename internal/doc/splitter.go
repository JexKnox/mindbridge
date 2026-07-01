// Package doc 实现离线索引路径:Loader → Splitter → EmbeddingModel → Indexer。
package doc

import (
	"strings"

	"github.com/cloudwego/eino/schema"
)

// SplitterConfig 控制切分行为。chunk 大小按"近似 token(词/字符混合估)"计算,
// 不同语料最优值差别大,需可配置。
type SplitterConfig struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string // 分隔符优先级,从大到小
}

// Splitter 把一段文本切成带重叠的 chunk,并为每个 chunk 写入元数据。
// 策略:按 separators 优先级递归切,直到块大小 <= ChunkSize;相邻块保留 ChunkOverlap 重叠。
type Splitter struct {
	cfg SplitterConfig
}

func NewSplitter(cfg SplitterConfig) *Splitter {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 400
	}
	if len(cfg.Separators) == 0 {
		cfg.Separators = []string{"\n\n", "\n", " "}
	}
	return &Splitter{cfg: cfg}
}

// Split 对 text 切分,返回带 source_file / chunk_index 元信息的 Documents。
func (s *Splitter) Split(text, sourceFile string) []*schema.Document {
	raw := s.recursiveSplit(text, s.cfg.Separators)
	// 合并过小块 + 加重叠。
	chunks := s.mergeWithOverlap(raw)
	docs := make([]*schema.Document, 0, len(chunks))
	for i, c := range chunks {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		docs = append(docs, &schema.Document{
			Content: c,
			MetaData: map[string]any{
				"source_file": sourceFile,
				"chunk_index": i,
			},
		})
	}
	return docs
}

// recursiveSplit 按分隔符优先级递归切分,直到每块长度 <= ChunkSize 或无更细分隔符。
func (s *Splitter) recursiveSplit(text string, seps []string) []string {
	if len(text) <= s.cfg.ChunkSize {
		return []string{text}
	}
	if len(seps) == 0 {
		// 无分隔符可用,硬切。
		return hardSplit(text, s.cfg.ChunkSize)
	}
	sep := seps[0]
	parts := strings.Split(text, sep)
	var out []string
	for _, p := range parts {
		if len(p) <= s.cfg.ChunkSize {
			out = append(out, p)
			continue
		}
		out = append(out, s.recursiveSplit(p, seps[1:])...)
	}
	return out
}

// mergeWithOverlap 把过小块合并到目标大小,并加入重叠窗口。
func (s *Splitter) mergeWithOverlap(parts []string) []string {
	if len(parts) == 0 {
		return nil
	}
	var chunks []string
	cur := parts[0]
	for _, p := range parts[1:] {
		if len(cur)+len(p) <= s.cfg.ChunkSize {
			cur += p
			continue
		}
		chunks = append(chunks, cur)
		// 重叠:取当前块尾部 overlap 作为下一块开头。
		if s.cfg.ChunkOverlap > 0 && len(cur) > s.cfg.ChunkOverlap {
			cur = cur[len(cur)-s.cfg.ChunkOverlap:] + p
		} else {
			cur = p
		}
	}
	if cur != "" {
		chunks = append(chunks, cur)
	}
	return chunks
}

func hardSplit(text string, size int) []string {
	var out []string
	for i := 0; i < len(text); i += size {
		end := i + size
		if end > len(text) {
			end = len(text)
		}
		out = append(out, text[i:end])
	}
	return out
}
