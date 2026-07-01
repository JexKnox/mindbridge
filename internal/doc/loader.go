package doc

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Load 按文件扩展名解析文档为纯文本。
// 支持 .md(markdown→纯文本)、.docx(Word,纯 Go 解析 zip+XML)、.txt(原样)。
// 其它扩展名按纯文本兜底。解析为纯文本后交给 Splitter 切分。
func Load(fileName string, r io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("loader: read %s: %w", fileName, err)
	}

	switch ext {
	case ".md", ".markdown":
		return markdownToText(raw)
	case ".docx":
		return docxToText(raw)
	default: // .txt 及未知类型按纯文本
		return string(raw), nil
	}
}

// markdownToText 用 goldmark 解析 markdown,抽取纯文本(去掉标记语法)。
// 保留段落换行,代码块内容原样保留(对 RAG 有用)。
func markdownToText(raw []byte) (string, error) {
	md := goldmark.New()
	reader := text.NewReader(raw)
	doc := md.Parser().Parse(reader)

	var b strings.Builder
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindText:
			t := n.(*ast.Text)
			b.Write(t.Segment.Value(raw))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte('\n')
			}
		case ast.KindCodeBlock:
			// 代码块逐行输出,保留可检索的代码内容。
			cb := n.(*ast.CodeBlock)
			for i := 0; i < cb.Lines().Len(); i++ {
				line := cb.Lines().At(i)
				b.Write(line.Value(raw))
			}
		case ast.KindFencedCodeBlock:
			fcb := n.(*ast.FencedCodeBlock)
			for i := 0; i < fcb.Lines().Len(); i++ {
				line := fcb.Lines().At(i)
				b.Write(line.Value(raw))
			}
		case ast.KindParagraph:
			// 段落结束后补换行,便于后续切分按段落识别。
		case ast.KindHeading:
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", fmt.Errorf("loader: markdown walk: %w", err)
	}
	return strings.TrimSpace(b.String()), nil
}

// docxToText 解析 .docx:它是一个 zip,正文在 word/document.xml。
// 遍历 XML,收集 <w:t> 文本,遇到 <w:p> 段落换行。纯 Go,无外部依赖。
func docxToText(raw []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("loader: open docx zip: %w", err)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("loader: docx missing word/document.xml")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("loader: open document.xml: %w", err)
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("loader: parse document.xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		local := se.Name.Local
		switch local {
		case "t": // <w:t> 文本片段
			var s string
			if err := dec.DecodeElement(&s, &se); err != nil {
				return "", fmt.Errorf("loader: decode w:t: %w", err)
			}
			b.WriteString(s)
		case "p": // <w:p> 段落 → 换行
			b.WriteByte('\n')
		case "br", "tab": // <w:br/> <w:tab/>
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String()), nil
}
