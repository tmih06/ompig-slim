package webtools

import (
	"strings"
	"testing"
)

func TestConvertDOCX(t *testing.T) {
	text, err := convertOffice(docxMediaType, docxFixture(t, "First paragraph of the report.", "Second paragraph."))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "First paragraph of the report.") || !strings.Contains(text, "Second paragraph.") {
		t.Fatalf("docx text = %q", text)
	}
}

func TestConvertXLSX(t *testing.T) {
	text, err := convertOffice(xlsxMediaType, xlsxFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "| Name | Alpha |") || !strings.Contains(text, "| 1 | 2 |") {
		t.Fatalf("xlsx text = %q", text)
	}
}

func TestConvertPDF(t *testing.T) {
	text, notes, err := convertPDF(pdfFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "PDF_TEXT_PROOF") {
		t.Fatalf("pdf text = %q", text)
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "layout") {
		t.Fatalf("pdf notes = %v", notes)
	}
}

func TestConvertPDFWithoutTextReportsFailure(t *testing.T) {
	if _, _, err := convertPDF([]byte("%PDF-1.4\n%%EOF\n")); err == nil {
		t.Fatal("a PDF with no text must report a failure, not return binary")
	}
}

func TestConvertEPUB(t *testing.T) {
	container := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	opf := `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="c1" href="chapter1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`
	chapter := `<html><body><h1>EPUB chapter</h1><p>EPUB_TEXT_PROOF: chapter body with enough words to be substantial.</p></body></html>`
	body := zipFixture(t, map[string]string{
		"META-INF/container.xml": container,
		"OEBPS/content.opf":      opf,
		"OEBPS/chapter1.xhtml":   chapter,
	})
	text, err := convertEPUB(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "EPUB_TEXT_PROOF") {
		t.Fatalf("epub text = %q", text)
	}
}

func TestConvertGzipTar(t *testing.T) {
	body := gzipFixture(t, tarFixture(t, map[string]string{"inside.txt": "payload"}))
	text, method, err := listGzip(body)
	if err != nil {
		t.Fatal(err)
	}
	if method != "tar-listing" || !strings.Contains(text, "inside.txt") {
		t.Fatalf("gzip tar: %q %q", method, text)
	}
}

func TestConvertGzipText(t *testing.T) {
	body := gzipFixture(t, []byte("COMPRESSED_TEXT_PROOF\n"))
	text, method, err := listGzip(body)
	if err != nil {
		t.Fatal(err)
	}
	if method != "gzip-text" || !strings.Contains(text, "COMPRESSED_TEXT_PROOF") {
		t.Fatalf("gzip text: %q %q", method, text)
	}
}

func TestSniffMedia(t *testing.T) {
	tests := []struct {
		body   []byte
		target string
		want   string
	}{
		{[]byte("%PDF-1.7\n"), "", "application/pdf"},
		{[]byte("SQLite format 3\x00rest"), "", sqliteMediaType},
		{[]byte("\x1f\x8b\x08"), "", "application/gzip"},
		{[]byte("PK\x03\x04rest"), "https://x/a.docx", docxMediaType},
		{[]byte("PK\x03\x04rest"), "https://x/a.zip", "application/zip"},
		{[]byte(`{"a":1}`), "https://x/noext", "application/json"},
		{[]byte(`<rss version="2.0">`), "https://x/f", "application/rss+xml"},
		{[]byte("<!DOCTYPE html><html></html>"), "https://x/f", "text/html"},
	}
	for _, test := range tests {
		if got := sniffMedia(test.body, test.target); got != test.want {
			t.Fatalf("sniff(%q, %q) = %q, want %q", test.body, test.target, got, test.want)
		}
	}
}

func TestClassifyMediaKeepsPlainText(t *testing.T) {
	if got := classifyMedia("text/plain", "", []byte("plain words")); got != "text/plain" {
		t.Fatalf("plain text reclassified as %q", got)
	}
	if got := classifyMedia("application/octet-stream", "https://x/a.pdf", []byte("%PDF-1.4")); got != "application/pdf" {
		t.Fatalf("octet-stream pdf classified as %q", got)
	}
}
