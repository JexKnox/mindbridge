package doc

import (
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// segment 是 markdown 一节内的连续片段。atomic 片段(代码块/表格)切分时整块保留,
// 避免代码或表格被从中间截断导致语义破碎。
type segment struct {
	text   string
	atomic bool
}

// section 是按标题层级切出的一节:标题路径(面包屑)+ 若干片段。
type section struct {
	path     []string // 各级标题,如 ["部署指南", "Docker 部署"]
	segments []segment
}

var headingRe = regexp.MustCompile("^(#{1,6})\\s+(.*)$")

// SplitMarkdown 对 markdown 做结构化切分:
//   - 按标题(#..######)分节,每个 chunk 内容前缀标题路径,保留上下文归属;
//   - 围栏代码块(``` / ~~~)与表格(连续 | 行)整块保留,不从中间切断;
//   - 节内普通文本沿用 Splitter 的递归切分与重叠策略。
//
// 每个 chunk 的 Meta 带 source_file / chunk_index(与普通切分一致)及 section(标题路径)。
func (s *Splitter) SplitMarkdown(raw, sourceFile string) []*schema.Document {
	sections := parseMarkdownSections(raw)

	var docs []*schema.Document
	chunkIdx := 0
	for _, sec := range sections {
		prefix := ""
		secPath := strings.Join(sec.path, " > ")
		if secPath != "" {
			prefix = "【" + secPath + "】\n"
		}
		for _, chunk := range s.chunkSegments(sec.segments) {
			chunk = strings.TrimSpace(chunk)
			if chunk == "" {
				continue
			}
			meta := map[string]any{
				"source_file": sourceFile,
				"chunk_index": chunkIdx,
			}
			if secPath != "" {
				meta["section"] = secPath
			}
			docs = append(docs, &schema.Document{
				Content:  prefix + chunk,
				MetaData: meta,
			})
			chunkIdx++
		}
	}
	return docs
}

// parseMarkdownSections 按行扫描 markdown,切出带标题路径的节,
// 并把围栏代码块 / 表格标记为 atomic 片段。
func parseMarkdownSections(raw string) []section {
	lines := strings.Split(raw, "\n")

	var sections []section
	cur := section{}
	var textBuf []string // 普通文本行缓冲
	var atomBuf []string // 当前 atomic 片段(代码/表格)行缓冲
	fence := ""          // 非空表示身处围栏代码块中,值为围栏标记(``` 或 ~~~)

	flushText := func() {
		if t := strings.TrimSpace(strings.Join(textBuf, "\n")); t != "" {
			cur.segments = append(cur.segments, segment{text: t})
		}
		textBuf = textBuf[:0]
	}
	flushAtom := func() {
		if t := strings.TrimSpace(strings.Join(atomBuf, "\n")); t != "" {
			cur.segments = append(cur.segments, segment{text: t, atomic: true})
		}
		atomBuf = atomBuf[:0]
	}
	flushSection := func() {
		flushText()
		if len(cur.segments) > 0 {
			sections = append(sections, cur)
		}
	}

	// headings[i] 为第 i+1 级标题文本;进入更深标题时保留祖先,同级/更浅时清空后代。
	var headings [6]string

	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 围栏代码块:开闭之间(含围栏行)全部进 atomic 缓冲。
		if fence != "" {
			atomBuf = append(atomBuf, line)
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
				flushAtom()
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inTable {
				inTable = false
				flushAtom()
			}
			flushText()
			fence = trimmed[:3]
			atomBuf = append(atomBuf, line)
			continue
		}

		// 表格:连续以 | 开头的行合成一个 atomic 片段。
		if strings.HasPrefix(trimmed, "|") {
			if !inTable {
				flushText()
				inTable = true
			}
			atomBuf = append(atomBuf, line)
			continue
		}
		if inTable {
			inTable = false
			flushAtom()
		}

		// 标题:结束当前节,更新标题路径。
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flushSection()
			level := len(m[1])
			headings[level-1] = strings.TrimSpace(m[2])
			for i := level; i < len(headings); i++ {
				headings[i] = ""
			}
			var path []string
			for i := 0; i < level; i++ {
				if headings[i] != "" {
					path = append(path, headings[i])
				}
			}
			cur = section{path: path}
			continue
		}

		textBuf = append(textBuf, line)
	}
	if fence != "" || inTable {
		flushAtom()
	}
	flushSection()

	return sections
}

// chunkSegments 把一节内的片段拼成 chunk:
// 普通文本按 Splitter 常规策略切;atomic 片段不切,能与相邻 chunk 合并则合并,否则独立成块。
func (s *Splitter) chunkSegments(segs []segment) []string {
	var chunks []string
	for _, seg := range segs {
		if seg.atomic {
			// 能塞进上一个 chunk 就合并(小代码块跟随上下文),否则独立成块。
			if n := len(chunks); n > 0 && len(chunks[n-1])+len(seg.text)+1 <= s.cfg.ChunkSize {
				chunks[n-1] += "\n" + seg.text
			} else {
				chunks = append(chunks, seg.text)
			}
			continue
		}
		parts := s.recursiveSplit(seg.text, s.cfg.Separators)
		chunks = append(chunks, s.mergeWithOverlap(parts)...)
	}
	return chunks
}
