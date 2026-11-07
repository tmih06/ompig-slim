package webtools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
)

// zipFixture builds a ZIP archive.
func zipFixture(t testing.TB, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// tarFixture builds a tar archive.
func tarFixture(t testing.TB, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// sqliteFixture builds a SQLite database file with one table.
func sqliteFixture(t testing.TB) []byte {
	t.Helper()
	path := t.TempDir() + "/fixture.db"
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('one')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// docxFixture builds a minimal DOCX package.
func docxFixture(t testing.TB, paragraphs ...string) []byte {
	t.Helper()
	var document bytes.Buffer
	document.WriteString(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, paragraph := range paragraphs {
		document.WriteString(`<w:p><w:r><w:t>` + paragraph + `</w:t></w:r></w:p>`)
	}
	document.WriteString(`</w:body></w:document>`)
	return zipFixture(t, map[string]string{"word/document.xml": document.String()})
}

// xlsxFixture builds a minimal XLSX package.
func xlsxFixture(t testing.TB) []byte {
	t.Helper()
	shared := `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Name</t></si><si><t>Alpha</t></si></sst>`
	sheet := `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
		`<row><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row>` +
		`<row><c><v>1</v></c><c><v>2</v></c></row>` +
		`</sheetData></worksheet>`
	return zipFixture(t, map[string]string{"xl/sharedStrings.xml": shared, "xl/worksheets/sheet1.xml": sheet})
}

// pdfFixture builds a one-page PDF with an uncompressed text stream.
func pdfFixture() []byte {
	content := "BT /F1 12 Tf 72 712 Td (PDF_TEXT_PROOF: the fifth line of a local report.) Tj ET"
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n")
	buffer.WriteString("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n")
	buffer.WriteString("3 0 obj<</Type/Page/Parent 2 0 R/Contents 4 0 R>>endobj\n")
	buffer.WriteString("4 0 obj<</Length " + itoa(len(content)) + ">>stream\n" + content + "\nendstream endobj\n")
	buffer.WriteString("trailer<</Root 1 0 R>>\n%%EOF\n")
	return buffer.Bytes()
}

// itoa is strconv.Itoa for the fixture's string concatenations.
func itoa(value int) string {
	data, _ := json.Marshal(value)
	return string(data)
}

// gzipFixture gzip-compresses a payload.
func gzipFixture(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
