package doc

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestLoadMarkdown(t *testing.T) {
	src := "# Title\n\nSome **bold** text and `code`.\n\n- item a\n- item b\n"
	got, err := Load("readme.md", strings.NewReader(src))
	if err != nil {
		t.Fatalf("Load md: %v", err)
	}
	if !strings.Contains(got, "Title") {
		t.Errorf("expected Title in output, got %q", got)
	}
	if !strings.Contains(got, "bold") {
		t.Errorf("expected bold text extracted, got %q", got)
	}
	if !strings.Contains(got, "item a") {
		t.Errorf("expected list item extracted, got %q", got)
	}
	// 标记符号应被去掉
	if strings.Contains(got, "**") {
		t.Errorf("markdown markers should be stripped, got %q", got)
	}
}

func TestLoadTxt(t *testing.T) {
	got, err := Load("note.txt", strings.NewReader("plain text body"))
	if err != nil {
		t.Fatalf("Load txt: %v", err)
	}
	if got != "plain text body" {
		t.Errorf("txt should pass through, got %q", got)
	}
}

// 构造一个最小 .docx(只含 word/document.xml),验证纯 Go 解析路径。
func TestLoadDocx(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Hello</w:t></w:r></w:p>
    <w:p><w:r><w:t>World</w:t></w:r></w:p>
  </w:body>
</w:document>`

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := f.Write([]byte(xml)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	got, err := Load("doc.docx", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Load docx: %v", err)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Errorf("expected Hello and World extracted, got %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("expected paragraph break between Hello/World, got %q", got)
	}
}
