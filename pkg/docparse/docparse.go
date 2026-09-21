// Package docparse extracts text from documents for CV import and
// opportunity intake: docx natively (stdlib zip+xml), PDF via a pure-Go
// reader, plus txt/md/html/csv/json. Formats outside Scout's domain
// (spreadsheets, slides, notebooks) are refused with a clear error rather
// than half-parsed. Paths are constrained (no proc/sys/dev, 20MB cap).
package docparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxBytes caps parsed files (Pi SD-card friendly).
const MaxBytes = 20 << 20

// ParseFile detects format from extension and extracts text.
func ParseFile(path string) (string, string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		abs, err := filepath.Abs(clean)
		if err != nil {
			return "", "", err
		}
		clean = abs
	}
	for _, blocked := range []string{"/proc/", "/sys/", "/dev/"} {
		if strings.HasPrefix(clean, blocked) {
			return "", "", fmt.Errorf("refusing blocked path")
		}
	}
	raw, err := readCapped(clean)
	if err != nil {
		return "", "", err
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(clean), "."))
	switch format {
	case "txt", "md", "markdown", "text":
		return string(raw), format, nil
	case "html", "htm":
		return stripHTML(string(raw)), "html", nil
	case "csv":
		return csvText(raw), "csv", nil
	case "json":
		return string(raw), "json", nil
	case "docx":
		text, err := docxText(raw)
		return text, "docx", err
	case "pdf":
		text, err := pdfText(bytes.NewReader(raw), int64(len(raw)))
		return text, "pdf", err
	default:
		if format == "" {
			return string(raw), "txt", nil
		}
		return "", "", fmt.Errorf("unsupported format %q (supported: txt, md, html, csv, json, docx, pdf)", format)
	}
}

// ParseBytes extracts text from in-memory content with a filename hint.
func ParseBytes(raw []byte, filename string) (string, string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", "", fmt.Errorf("file is empty")
	}
	if len(raw) > MaxBytes {
		return "", "", fmt.Errorf("file exceeds %dMB cap", MaxBytes>>20)
	}
	format := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	switch format {
	case "txt", "md", "markdown", "text", "":
		return string(raw), "txt", nil
	case "html", "htm":
		return stripHTML(string(raw)), "html", nil
	case "csv":
		return csvText(raw), "csv", nil
	case "json":
		return string(raw), "json", nil
	case "docx":
		text, err := docxText(raw)
		return text, "docx", err
	case "pdf":
		text, err := pdfText(bytes.NewReader(raw), int64(len(raw)))
		return text, "pdf", err
	default:
		return "", "", fmt.Errorf("unsupported format %q (supported: txt, md, html, csv, json, docx, pdf)", format)
	}
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("file exceeds %dMB cap", MaxBytes>>20)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	return raw, nil
}

// docxText extracts word/document.xml paragraph text.
func docxText(raw []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("not a docx file: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		var doc struct {
			Body struct {
				Paras []struct {
					Runs []struct {
						Text []string `xml:"t"`
					} `xml:"r"`
				} `xml:"p"`
			} `xml:"body"`
		}
		if err := xml.NewDecoder(rc).Decode(&doc); err != nil {
			return "", err
		}
		var b strings.Builder
		for _, p := range doc.Body.Paras {
			var line strings.Builder
			for _, r := range p.Runs {
				for _, t := range r.Text {
					line.WriteString(t)
				}
			}
			if s := strings.TrimSpace(line.String()); s != "" {
				b.WriteString(s + "\n")
			}
		}
		if b.Len() == 0 {
			return "", fmt.Errorf("no text in docx")
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("not a docx file: missing word/document.xml")
}

// pdfText extracts plain text via content-stream parsing.
func pdfText(r *bytes.Reader, size int64) (string, error) {
	text, err := pdf.NewReader(r, size)
	if err != nil {
		return "", fmt.Errorf("not a readable pdf: %w", err)
	}
	n := text.NumPage()
	if n == 0 {
		return "", fmt.Errorf("pdf has no pages")
	}
	if n > 50 {
		n = 50
	}
	var b strings.Builder
	for i := 1; i <= n; i++ {
		p := text.Page(i)
		if p.V.IsNull() {
			continue
		}
		content, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range content {
			var line strings.Builder
			for _, w := range row.Content {
				line.WriteString(w.S + " ")
			}
			if s := strings.TrimSpace(line.String()); s != "" {
				b.WriteString(s + "\n")
			}
		}
	}
	if b.Len() < 10 {
		return "", fmt.Errorf("no extractable text (scanned image? OCR is out of scope)")
	}
	return b.String(), nil
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
			continue
		}
		if s[i] == '>' {
			inTag = false
			b.WriteByte(' ')
			continue
		}
		if !inTag {
			b.WriteByte(s[i])
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func csvText(raw []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(strings.Join(strings.Split(line, ","), " | ") + "\n")
		if b.Len() > 12000 {
			break
		}
	}
	return b.String()
}
