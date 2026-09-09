package rag_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"buddy/server/internal/rag"
)

func TestExtractText_PlainTextAndMarkdown(t *testing.T) {
	txt := "Lesson 1: Introduction to Lathe Machining by Dr. Tissa Jinasena."
	res, err := rag.ExtractText("lecture.txt", []byte(txt))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != txt {
		t.Errorf("expected %s, got %s", txt, res)
	}

	md := "# Chapter 4\n\n### Mindful Engineering\nPrecision requires inner calm."
	resMd, err := rag.ExtractText("notes.md", []byte(md))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resMd != md {
		t.Errorf("expected %s, got %s", md, resMd)
	}
}

func TestExtractText_DocxSimulation(t *testing.T) {
	// Create minimal valid in-memory DOCX zip
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Principles of Practical Engineering and Mindfulness.</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:t>Published by Jinasena Training Foundation.</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`

	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(docXML)); err != nil {
		t.Fatalf("failed to write docxml: %v", err)
	}
	_ = zw.Close()

	extracted, err := rag.ExtractText("curriculum.docx", buf.Bytes())
	if err != nil {
		t.Fatalf("failed to extract docx text: %v", err)
	}

	if !strings.Contains(extracted, "Principles of Practical Engineering and Mindfulness") {
		t.Errorf("expected extracted text to contain first paragraph, got: %s", extracted)
	}
	if !strings.Contains(extracted, "Published by Jinasena Training Foundation") {
		t.Errorf("expected extracted text to contain second paragraph, got: %s", extracted)
	}
}
