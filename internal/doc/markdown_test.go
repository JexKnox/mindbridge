package doc

import (
	"strings"
	"testing"
)

func TestSplitMarkdownSectionsAndBreadcrumb(t *testing.T) {
	md := "# 部署指南\n\n总体说明。\n\n## Docker 部署\n\n用 docker compose 启动。\n\n## 手动部署\n\n手动编译运行。\n"
	s := NewSplitter(SplitterConfig{ChunkSize: 400})
	docs := s.SplitMarkdown(md, "deploy.md")

	if len(docs) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(docs))
	}
	if !strings.HasPrefix(docs[1].Content, "【部署指南 > Docker 部署】") {
		t.Errorf("chunk missing breadcrumb prefix: %q", docs[1].Content)
	}
	if docs[1].MetaData["section"] != "部署指南 > Docker 部署" {
		t.Errorf("section meta = %v", docs[1].MetaData["section"])
	}
	for i, d := range docs {
		if d.MetaData["source_file"] != "deploy.md" {
			t.Errorf("chunk %d missing source_file", i)
		}
		if d.MetaData["chunk_index"] != i {
			t.Errorf("chunk %d index = %v", i, d.MetaData["chunk_index"])
		}
	}
}

func TestSplitMarkdownKeepsCodeBlockIntact(t *testing.T) {
	var code strings.Builder
	code.WriteString("```go\n")
	for i := 0; i < 30; i++ {
		code.WriteString("fmt.Println(\"line line line line line\")\n")
	}
	code.WriteString("```")
	md := "# 用法\n\n示例代码:\n\n" + code.String() + "\n\n结束说明。\n"

	s := NewSplitter(SplitterConfig{ChunkSize: 100})
	docs := s.SplitMarkdown(md, "usage.md")

	found := false
	for _, d := range docs {
		if strings.Contains(d.Content, "```go") {
			found = true
			// 代码块虽远超 ChunkSize,仍应整块保留在同一 chunk。
			if strings.Count(d.Content, "fmt.Println") != 30 {
				t.Errorf("code block was split: %q", d.Content)
			}
		}
	}
	if !found {
		t.Fatal("code block not found in any chunk")
	}
}

func TestSplitMarkdownKeepsTableIntact(t *testing.T) {
	md := "# 对比\n\n| 项目 | 说明 |\n|---|---|\n| a | 甲 |\n| b | 乙 |\n\n表后文字。\n"
	s := NewSplitter(SplitterConfig{ChunkSize: 400})
	docs := s.SplitMarkdown(md, "cmp.md")

	found := false
	for _, d := range docs {
		if strings.Contains(d.Content, "| a | 甲 |") {
			found = true
			if !strings.Contains(d.Content, "| b | 乙 |") {
				t.Errorf("table was split: %q", d.Content)
			}
		}
	}
	if !found {
		t.Fatal("table not found in any chunk")
	}
}

func TestSplitMarkdownHeadingOnlySections(t *testing.T) {
	// 只有标题没有正文的节不应产出空 chunk。
	md := "# A\n\n## B\n\n正文。\n"
	s := NewSplitter(SplitterConfig{ChunkSize: 400})
	docs := s.SplitMarkdown(md, "x.md")
	if len(docs) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(docs))
	}
	if !strings.HasPrefix(docs[0].Content, "【A > B】") {
		t.Errorf("breadcrumb wrong: %q", docs[0].Content)
	}
}
