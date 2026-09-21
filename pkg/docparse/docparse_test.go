package docparse

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDocx(t *testing.T, paras []string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><w:document xmlns:w="http://x"><w:body>`)
	for _, p := range paras {
		b.WriteString(`<w:p><w:r><w:t>` + p + `</w:t></w:r></w:p>`)
	}
	b.WriteString(`</w:body></w:document>`)
	f.Write([]byte(b.String()))
	w.Close()
	path := filepath.Join(t.TempDir(), "cv.docx")
	os.WriteFile(path, buf.Bytes(), 0o600)
	return path
}

func TestDocxNative(t *testing.T) {
	text, format, err := ParseFile(writeDocx(t, []string{"Jane Doe", "Senior Go Developer"}))
	if err != nil || format != "docx" {
		t.Fatalf("docx failed: %s %v", format, err)
	}
	if !strings.Contains(text, "Jane Doe") || !strings.Contains(text, "Senior Go") {
		t.Fatalf("lost content: %q", text)
	}
}

func TestTxtCsvHtml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600)
	if text, _, err := ParseFile(filepath.Join(dir, "a.txt")); err != nil || text != "hello" {
		t.Fatal("txt failed")
	}
	os.WriteFile(filepath.Join(dir, "a.csv"), []byte("name,skill\njane,go"), 0o600)
	if text, f, err := ParseFile(filepath.Join(dir, "a.csv")); err != nil || f != "csv" || !strings.Contains(text, "jane | go") {
		t.Fatalf("csv failed: %q %v", text, err)
	}
	os.WriteFile(filepath.Join(dir, "a.html"), []byte("<h1>Hi</h1><p>there</p>"), 0o600)
	if text, _, err := ParseFile(filepath.Join(dir, "a.html")); err != nil || !strings.Contains(text, "Hi there") {
		t.Fatalf("html failed: %q", text)
	}
}

func TestRefusals(t *testing.T) {
	if _, _, err := ParseFile("/proc/self/cmdline"); err == nil {
		t.Fatal("proc must refuse")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.xlsx"), []byte("x"), 0o600)
	if _, _, err := ParseFile(filepath.Join(dir, "a.xlsx")); err == nil {
		t.Fatal("xlsx must refuse honestly")
	}
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte("   "), 0o600)
	if _, _, err := ParseFile(filepath.Join(dir, "empty.txt")); err == nil {
		t.Fatal("empty must refuse")
	}
	if _, _, err := ParseBytes([]byte("x"), "f.pptx"); err == nil {
		t.Fatal("pptx must refuse")
	}
	if _, _, err := ParseBytes([]byte("not a zip"), "f.docx"); err == nil {
		t.Fatal("garbage docx must error")
	}
}
