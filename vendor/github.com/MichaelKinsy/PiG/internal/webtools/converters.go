package webtools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite" // read-only access to a fetched SQLite file
)

// writeTemp writes body to a private temporary file and returns its path; SQLite
// cannot read a database from memory.
func writeTemp(body []byte) (string, error) {
	file, err := os.CreateTemp("", "pi-fetch-*.sqlite")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := file.Write(body); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// convertJSON pretty-prints a JSON body, keeping the original text when it is
// large or already formatted.
func convertJSON(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	if len(trimmed) > 2<<20 {
		return string(trimmed)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, trimmed, "", "  "); err != nil {
		return string(trimmed)
	}
	return pretty.String()
}

// ─── feeds ───────────────────────────────────────────────────────────────────

// feedDocument is the union of RSS, Atom and RDF feed shapes.
type feedDocument struct {
	XMLName xml.Name
	Title   string       `xml:"title"`
	Channel *feedChannel `xml:"channel"`
	Entries []feedXML    `xml:"entry"`
	Items   []feedXML    `xml:"item"`
}

// feedChannel is an RSS or RDF channel, which nests its items.
type feedChannel struct {
	Title string    `xml:"title"`
	Items []feedXML `xml:"item"`
}

// feedLink is one <link>: RSS puts the URL in its text, Atom in href.
type feedLink struct {
	Text string `xml:",chardata"`
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// URL returns the link's target, preferring an Atom alternate over any other
// relation.
func (l feedLink) URL() string {
	if value := strings.TrimSpace(l.Href); value != "" {
		return value
	}
	return strings.TrimSpace(l.Text)
}

// feedXML is one item or entry; the fields cover RSS and Atom spellings.
type feedXML struct {
	Title   string     `xml:"title"`
	Links   []feedLink `xml:"link"`
	Updated string     `xml:"updated"`
	PubDate string     `xml:"pubDate"`
	Date    string     `xml:"date"`
	Summary string     `xml:"summary"`
	Desc    string     `xml:"description"`
	Content string     `xml:"encoded"`
	Body    string     `xml:"content"`
}

// URL returns the entry's primary link: an Atom alternate, else the first link
// that carries a target.
func (e feedXML) URL() string {
	for _, link := range e.Links {
		if strings.EqualFold(link.Rel, "alternate") && link.URL() != "" {
			return link.URL()
		}
	}
	for _, link := range e.Links {
		if value := link.URL(); value != "" {
			return value
		}
	}
	return ""
}

// convertFeed renders an RSS or Atom document as a Markdown list of entries.
func convertFeed(body []byte) (string, error) {
	var document feedDocument
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.Strict = false
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("feed could not be parsed: %v", err)
	}
	var entries []feedXML
	if document.Channel != nil {
		entries = append(entries, document.Channel.Items...)
	}
	// Atom entries are top-level; RDF puts its items at the root too.
	entries = append(entries, document.Items...)
	entries = append(entries, document.Entries...)
	title := collapseSpace(document.Title)
	if title == "" && document.Channel != nil {
		title = collapseSpace(document.Channel.Title)
	}
	var builder strings.Builder
	if title != "" {
		builder.WriteString("# " + title + "\n\n")
	}
	if len(entries) == 0 {
		builder.WriteString("(no entries)\n")
		return builder.String(), nil
	}
	for _, entry := range entries {
		entryTitle := collapseSpace(entry.Title)
		if entryTitle == "" {
			entryTitle = "(untitled)"
		}
		link := entry.URL()
		builder.WriteString("- ")
		if link != "" {
			builder.WriteString("[" + entryTitle + "](" + link + ")")
		} else {
			builder.WriteString(entryTitle)
		}
		if date := firstNonEmpty(entry.PubDate, entry.Updated, entry.Date); date != "" {
			builder.WriteString(" (" + collapseSpace(date) + ")")
		}
		builder.WriteByte('\n')
		if summary := firstNonEmpty(entry.Summary, entry.Desc, entry.Content, entry.Body); strings.TrimSpace(summary) != "" {
			text := collapseSpace(stripMarkup(summary))
			if runes := []rune(text); len(runes) > 240 {
				text = string(runes[:240]) + "…"
			}
			if text != "" {
				builder.WriteString("  " + text + "\n")
			}
		}
	}
	return builder.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// stripMarkup drops tags and unescapes entities from an HTML fragment.
func stripMarkup(fragment string) string {
	var builder strings.Builder
	depth := 0
	for _, r := range fragment {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				builder.WriteRune(r)
			}
		}
	}
	return html.UnescapeString(builder.String())
}

// ─── notebooks ───────────────────────────────────────────────────────────────

// notebookFile is the subset of the Jupyter format the converter reads.
type notebookFile struct {
	Cells []notebookCell `json:"cells"`
}

type notebookCell struct {
	CellType string          `json:"cell_type"`
	Source   json.RawMessage `json:"source"`
	Outputs  []notebookOut   `json:"outputs"`
}

type notebookOut struct {
	OutputType string          `json:"output_type"`
	Text       json.RawMessage `json:"text"`
	Data       json.RawMessage `json:"data"`
}

// stringOrLines decodes a nbformat string field, which is a string or an array
// of strings.
func stringOrLines(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single
	}
	var lines []string
	if err := json.Unmarshal(raw, &lines); err == nil {
		return strings.Join(lines, "")
	}
	return ""
}

// convertNotebook renders a Jupyter notebook's cells as Markdown, with code cells
// fenced and their textual outputs kept.
func convertNotebook(body []byte) (string, error) {
	var notebook notebookFile
	if err := json.Unmarshal(body, &notebook); err != nil {
		return "", fmt.Errorf("notebook could not be parsed: %v", err)
	}
	var builder strings.Builder
	for _, cell := range notebook.Cells {
		source := stringOrLines(cell.Source)
		switch cell.CellType {
		case "markdown":
			if strings.TrimSpace(source) != "" {
				builder.WriteString(strings.TrimRight(source, "\n") + "\n\n")
			}
		case "code":
			builder.WriteString("```\n" + strings.TrimRight(source, "\n") + "\n```\n\n")
			for _, output := range cell.Outputs {
				text := stringOrLines(output.Text)
				if text == "" {
					text = notebookDataText(output.Data)
				}
				if strings.TrimSpace(text) == "" {
					continue
				}
				builder.WriteString("```\n" + strings.TrimRight(text, "\n") + "\n```\n\n")
			}
		case "raw":
			if strings.TrimSpace(source) != "" {
				builder.WriteString(strings.TrimRight(source, "\n") + "\n\n")
			}
		}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "(notebook has no textual cells)\n", nil
	}
	return text + "\n", nil
}

// notebookDataText picks the first textual representation of an output bundle.
func notebookDataText(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var bundle map[string]json.RawMessage
	if err := json.Unmarshal(data, &bundle); err != nil {
		return ""
	}
	for _, key := range []string{"text/plain", "text/markdown", "text/html"} {
		if raw, ok := bundle[key]; ok {
			if text := stringOrLines(raw); text != "" {
				return text
			}
		}
	}
	return ""
}

// ─── archives ────────────────────────────────────────────────────────────────

// listZip renders a ZIP archive's root listing and nested archive note.
func listZip(body []byte) string {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "ZIP archive could not be read: " + err.Error() + "\n"
	}
	var builder strings.Builder
	builder.WriteString("ZIP archive with " + strconv.Itoa(len(reader.File)) + " entries:\n")
	writeEntries(&builder, zipEntries(reader.File))
	return builder.String()
}

// zipEntries converts ZIP members to archive entries.
func zipEntries(files []*zip.File) []archiveEntry {
	entries := make([]archiveEntry, 0, len(files))
	for _, file := range files {
		modified := file.Modified
		entries = append(entries, archiveEntry{Name: file.Name, Size: int64(file.UncompressedSize64), Modified: modified, Directory: file.FileInfo().IsDir()})
	}
	return entries
}

// archiveEntry is one member of a listing.
type archiveEntry struct {
	Name      string
	Size      int64
	Modified  time.Time
	Directory bool
}

// writeEntries writes a sorted, bounded listing.
func writeEntries(builder *strings.Builder, entries []archiveEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	const limit = 500
	for index, entry := range entries {
		if index == limit {
			builder.WriteString(fmt.Sprintf("… %d more entries\n", len(entries)-limit))
			return
		}
		name := entry.Name
		if entry.Directory && !strings.HasSuffix(name, "/") {
			name += "/"
		}
		builder.WriteString(name)
		if !entry.Directory {
			builder.WriteString(" (" + formatBytes(entry.Size) + ")")
		}
		if !entry.Modified.IsZero() {
			builder.WriteString(" " + entry.Modified.Format(time.RFC3339))
		}
		builder.WriteByte('\n')
	}
}

// listTar renders a tar archive's listing.
func listTar(body []byte) string {
	reader := tar.NewReader(bytes.NewReader(body))
	var entries []archiveEntry
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(entries) == 0 {
				return "tar archive could not be read: " + err.Error() + "\n"
			}
			entries = append(entries, archiveEntry{Name: "(listing stopped: " + err.Error() + ")"})
			break
		}
		entries = append(entries, archiveEntry{Name: header.Name, Size: header.Size, Modified: header.ModTime, Directory: header.Typeflag == tar.TypeDir})
	}
	var builder strings.Builder
	builder.WriteString("tar archive with " + strconv.Itoa(len(entries)) + " entries:\n")
	writeEntries(&builder, entries)
	return builder.String()
}

// listGzip decompresses a gzip body and lists the tar inside it, or returns the
// text when the payload is textual.
func listGzip(body []byte) (string, string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("gzip body could not be decompressed")
	}
	defer reader.Close()
	// A listing only needs the members, not the payload: bound the read.
	payload, err := io.ReadAll(io.LimitReader(reader, DefaultMaxBytes))
	if err != nil && len(payload) == 0 {
		return "", "", fmt.Errorf("gzip body could not be decompressed")
	}
	if len(payload) > 262 && string(payload[257:262]) == "ustar" {
		return listTar(payload), "tar-listing", nil
	}
	if isTextual(payload) {
		return decodeCharset(payload, ""), "gzip-text", nil
	}
	return "", "", fmt.Errorf("gzip payload is binary and not a tar archive; no local converter for it")
}

// ─── EPUB ────────────────────────────────────────────────────────────────────

// convertEPUB renders an EPUB's chapters in spine order.
func convertEPUB(body []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("EPUB could not be read: %v", err)
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[strings.TrimPrefix(file.Name, "./")] = file
	}
	opfPath := ""
	if container, err := readZipFile(files["META-INF/container.xml"]); err == nil {
		opfPath = firstMatch(string(container), "full-path=\"")
	}
	if opfPath == "" {
		for name := range files {
			if strings.HasSuffix(strings.ToLower(name), ".opf") {
				opfPath = name
				break
			}
		}
	}
	opf, err := readZipFile(files[opfPath])
	if err != nil {
		return "", fmt.Errorf("EPUB package document is missing")
	}
	base := path.Dir(opfPath)
	var pkg struct {
		Manifest []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"manifest>item"`
		Spine []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(opf))
	decoder.Strict = false
	if err := decoder.Decode(&pkg); err != nil {
		return "", fmt.Errorf("EPUB has no readable chapters")
	}
	manifest := make(map[string]string, len(pkg.Manifest))
	for _, item := range pkg.Manifest {
		manifest[item.ID] = item.Href
	}
	var builder strings.Builder
	written := 0
	for _, item := range pkg.Spine {
		href, ok := manifest[item.IDRef]
		if !ok {
			continue
		}
		chapter, err := readZipFile(files[path.Join(base, href)])
		if err != nil {
			continue
		}
		rendered := strings.TrimSpace(renderHTMLDocument(parseHTML(chapter), "file:///"+path.Join(base, href)))
		if rendered == "" {
			continue
		}
		builder.WriteString(rendered + "\n\n")
		written++
	}
	if written == 0 {
		return "", fmt.Errorf("EPUB has no readable chapters")
	}
	return strings.TrimSpace(builder.String()) + "\n", nil
}

// readZipFile returns a member's bytes.
func readZipFile(file *zip.File) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("missing member")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, DefaultMaxBytes))
}

// firstMatch returns the text after the first occurrence of prefix up to the
// next quote.
func firstMatch(text, prefix string) string {
	index := strings.Index(text, prefix)
	if index < 0 {
		return ""
	}
	value := text[index+len(prefix):]
	if end := strings.IndexByte(value, '"'); end >= 0 {
		return value[:end]
	}
	return ""
}

// ─── Office ──────────────────────────────────────────────────────────────────

// convertOffice renders a DOCX, PPTX or XLSX document as Markdown text.
func convertOffice(media string, body []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("Office document could not be read: %v", err)
	}
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[file.Name] = file
	}
	switch media {
	case docxMediaType:
		return officeWord(files)
	case pptxMediaType:
		return officeSlides(files)
	case xlsxMediaType:
		return officeSheets(files)
	}
	return "", fmt.Errorf("unsupported Office media type %q", media)
}

// officeWord renders word/document.xml paragraphs.
func officeWord(files map[string]*zip.File) (string, error) {
	document, err := readZipFile(files["word/document.xml"])
	if err != nil {
		return "", fmt.Errorf("DOCX has no word/document.xml")
	}
	text := xmlParagraphs(document, "p", "t")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("DOCX contains no extractable text")
	}
	return text, nil
}

// officeSlides renders every slide in numeric order.
func officeSlides(files map[string]*zip.File) (string, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("PPTX has no slides")
	}
	sort.Slice(names, func(i, j int) bool { return slideNumber(names[i]) < slideNumber(names[j]) })
	var builder strings.Builder
	for index, name := range names {
		document, err := readZipFile(files[name])
		if err != nil {
			continue
		}
		text := strings.TrimSpace(xmlParagraphs(document, "p", "t"))
		if text == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf("## Slide %d\n\n%s\n\n", index+1, text))
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "", fmt.Errorf("PPTX contains no extractable text")
	}
	return strings.TrimSpace(builder.String()) + "\n", nil
}

// slideNumber extracts the index from ppt/slides/slideN.xml.
func slideNumber(name string) int {
	base := strings.TrimSuffix(path.Base(name), ".xml")
	digits := strings.TrimPrefix(base, "slide")
	value, _ := strconv.Atoi(digits)
	return value
}

// officeSheets renders each worksheet as a Markdown table, resolving shared
// strings.
func officeSheets(files map[string]*zip.File) (string, error) {
	shared := []string{}
	if document, err := readZipFile(files["xl/sharedStrings.xml"]); err == nil {
		shared = sharedStrings(document)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("XLSX has no worksheets")
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		document, err := readZipFile(files[name])
		if err != nil {
			continue
		}
		rows := sheetRows(document, shared)
		if len(rows) == 0 {
			continue
		}
		builder.WriteString("### " + strings.TrimSuffix(path.Base(name), ".xml") + "\n\n")
		builder.WriteString(markdownTable(rows))
		builder.WriteString("\n")
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "", fmt.Errorf("XLSX contains no cell values")
	}
	return strings.TrimSpace(builder.String()) + "\n", nil
}

// sharedStrings collects xl/sharedStrings.xml entries.
func sharedStrings(document []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	decoder.Strict = false
	var values []string
	var current strings.Builder
	inItem, inText := false, false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "si":
				inItem, current = true, strings.Builder{}
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "si":
				values = append(values, current.String())
				inItem = false
			case "t":
				inText = false
			}
		case xml.CharData:
			if inItem && inText {
				current.WriteString(string(element))
			}
		}
	}
	return values
}

// sheetRows returns a worksheet's rows, resolving shared-string indexes.
func sheetRows(document []byte, shared []string) [][]string {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	decoder.Strict = false
	var rows [][]string
	var row []string
	var value strings.Builder
	cellType := ""
	inCell, inValue, inInline := false, false, false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "row":
				row = nil
			case "c":
				inCell = true
				value.Reset()
				cellType = ""
				for _, attr := range element.Attr {
					if attr.Name.Local == "t" {
						cellType = attr.Value
					}
				}
			case "v":
				inValue = true
			case "t":
				inInline = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "row":
				if row != nil {
					rows = append(rows, row)
				}
				row = nil
			case "c":
				text := value.String()
				if cellType == "s" {
					if index, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && index >= 0 && index < len(shared) {
						text = shared[index]
					}
				}
				row = append(row, text)
				inCell = false
			case "v":
				inValue = false
			case "t":
				inInline = false
			}
		case xml.CharData:
			if inCell && (inValue || inInline) {
				value.WriteString(string(element))
			}
		}
	}
	return rows
}

// markdownTable renders rows as a Markdown table.
func markdownTable(rows [][]string) string {
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return ""
	}
	var builder strings.Builder
	write := func(cells []string) {
		builder.WriteString("|")
		for index := range width {
			value := ""
			if index < len(cells) {
				value = escapeTableCell(cells[index])
			}
			builder.WriteString(" " + value + " |")
		}
		builder.WriteString("\n")
	}
	write(rows[0])
	builder.WriteString("|")
	for range width {
		builder.WriteString(" --- |")
	}
	builder.WriteString("\n")
	for _, row := range rows[1:] {
		write(row)
	}
	return builder.String()
}

// escapeTableCell keeps cell text on one line and out of the column separator.
func escapeTableCell(value string) string {
	value = strings.ReplaceAll(collapseSpace(value), "|", "\\|")
	if runes := []rune(value); len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return value
}

// xmlParagraphs extracts text from an Office XML part, starting a new line at
// each paragraph element and keeping the text of each text element.
func xmlParagraphs(document []byte, paragraphTag, textTag string) string {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	decoder.Strict = false
	var builder strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == textTag {
				inText = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case textTag:
				inText = false
			case paragraphTag:
				builder.WriteString("\n\n")
			}
		case xml.CharData:
			if inText {
				builder.WriteString(string(element))
			}
		}
	}
	lines := strings.Split(builder.String(), "\n\n")
	kept := lines[:0]
	for _, line := range lines {
		if text := collapseSpace(line); text != "" {
			kept = append(kept, text)
		}
	}
	return strings.Join(kept, "\n\n")
}

// ─── PDF ─────────────────────────────────────────────────────────────────────

// convertPDF extracts a PDF's text. Only text-showing operators are honoured, so
// layout, tables and images are lost; a document whose fonts use custom encodings
// or that is a scan yields no text and reports that instead of binary noise.
func convertPDF(body []byte) (string, []string, error) {
	text := extractPDFText(body)
	if strings.TrimSpace(text) == "" {
		return "", nil, fmt.Errorf("no extractable text in PDF (scanned images or an unsupported font encoding)")
	}
	if !plausibleText(text) {
		return "", nil, fmt.Errorf("PDF text uses an embedded font encoding that cannot be decoded locally")
	}
	notes := []string{"text extracted from PDF; layout, tables and images are not preserved"}
	return text, notes, nil
}

// extractPDFText walks every content stream and concatenates the text it shows.
func extractPDFText(body []byte) string {
	var builder strings.Builder
	position := 0
	for position < len(body) {
		index := bytes.Index(body[position:], []byte("stream"))
		if index < 0 {
			break
		}
		start := position + index
		position = start + len("stream")
		headerStart := start - 4096
		if headerStart < 0 {
			headerStart = 0
		}
		header := body[headerStart:start]
		dataStart := position
		if dataStart < len(body) && body[dataStart] == '\r' {
			dataStart++
		}
		if dataStart < len(body) && body[dataStart] == '\n' {
			dataStart++
		}
		end := bytes.Index(body[dataStart:], []byte("endstream"))
		if end < 0 {
			break
		}
		raw := body[dataStart : dataStart+end]
		position = dataStart + end + len("endstream")
		decoded := raw
		if bytes.Contains(header, []byte("/FlateDecode")) {
			reader, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			inflated, err := io.ReadAll(io.LimitReader(reader, DefaultMaxBytes))
			reader.Close()
			if err != nil && len(inflated) == 0 {
				continue
			}
			decoded = inflated
		}
		if !bytes.Contains(decoded, []byte("BT")) {
			continue
		}
		builder.WriteString(pdfContentText(decoded))
	}
	return sanitizePDFText(builder.String())
}

// pdfContentText extracts the text one content stream shows.
func pdfContentText(stream []byte) string {
	var out strings.Builder
	var pending strings.Builder
	index := 0
	lastNumber := 0.0
	for index < len(stream) {
		c := stream[index]
		switch {
		case c == '%':
			for index < len(stream) && stream[index] != '\n' && stream[index] != '\r' {
				index++
			}
		case c == '(':
			value, next := pdfLiteralString(stream, index)
			pending.WriteString(value)
			index = next
		case c == '<' && index+1 < len(stream) && stream[index+1] != '<':
			value, next := pdfHexString(stream, index)
			pending.WriteString(value)
			index = next
		case c == '<' && index+1 < len(stream) && stream[index+1] == '<':
			// Nested dictionary inside a content stream: skip it.
			depth, next := 0, index
			for next < len(stream)-1 {
				if stream[next] == '<' && stream[next+1] == '<' {
					depth++
					next += 2
					continue
				}
				if stream[next] == '>' && stream[next+1] == '>' {
					depth--
					next += 2
					if depth == 0 {
						break
					}
					continue
				}
				next++
			}
			index = next
		case c == '/':
			// A name operand; skip it.
			index++
			for index < len(stream) && !pdfDelimiter(stream[index]) {
				index++
			}
		case c == '[' || c == ']':
			index++
		case c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.':
			value, next := pdfNumber(stream, index)
			lastNumber = value
			index = next
			if pending.Len() == 0 && lastNumber <= -100 && out.Len() > 0 && !strings.HasSuffix(out.String(), " ") {
				out.WriteByte(' ')
			}
		case isOperatorByte(c):
			start := index
			for index < len(stream) && isOperatorByte(stream[index]) {
				index++
			}
			operator := string(stream[start:index])
			switch operator {
			case "Tj", "'", "\"":
				out.WriteString(pending.String())
				pending.Reset()
				if operator != "Tj" {
					out.WriteByte('\n')
				}
			case "TJ":
				out.WriteString(pending.String())
				pending.Reset()
			case "Td", "TD", "T*", "ET":
				out.WriteByte('\n')
			case "Tm":
				out.WriteByte('\n')
			case "BT":
				out.WriteByte('\n')
			}
		default:
			index++
		}
	}
	return out.String()
}

func pdfDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isOperatorByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '*' || c == '\'' || c == '"'
}

// pdfNumber parses a number operand.
func pdfNumber(stream []byte, index int) (float64, int) {
	start := index
	for index < len(stream) && (stream[index] >= '0' && stream[index] <= '9' || stream[index] == '-' || stream[index] == '+' || stream[index] == '.') {
		index++
	}
	value, err := strconv.ParseFloat(string(stream[start:index]), 64)
	if err != nil {
		return 0, index
	}
	return value, index
}

// pdfLiteralString decodes a `(...)` string with PDF escapes.
func pdfLiteralString(stream []byte, index int) (string, int) {
	var builder strings.Builder
	depth := 0
	index++ // the opening parenthesis
	for index < len(stream) {
		c := stream[index]
		switch c {
		case '\\':
			index++
			if index >= len(stream) {
				return builder.String(), index
			}
			switch escape := stream[index]; escape {
			case 'n':
				builder.WriteByte('\n')
			case 'r':
				builder.WriteByte('\r')
			case 't':
				builder.WriteByte('\t')
			case 'b':
				builder.WriteByte('\b')
			case 'f':
				builder.WriteByte('\f')
			case '(', ')', '\\':
				builder.WriteByte(escape)
			case '\n', '\r':
				// A line continuation contributes nothing.
			case '0', '1', '2', '3', '4', '5', '6', '7':
				value := 0
				count := 0
				for count < 3 && index < len(stream) && stream[index] >= '0' && stream[index] <= '7' {
					value = value*8 + int(stream[index]-'0')
					index++
					count++
				}
				builder.WriteByte(byte(value))
				continue
			default:
				builder.WriteByte(escape)
			}
			index++
		case '(':
			depth++
			builder.WriteByte(c)
			index++
		case ')':
			if depth == 0 {
				return builder.String(), index + 1
			}
			depth--
			builder.WriteByte(c)
			index++
		default:
			builder.WriteByte(c)
			index++
		}
	}
	return builder.String(), index
}

// pdfHexString decodes a `<...>` hex string to bytes interpreted as Latin-1.
func pdfHexString(stream []byte, index int) (string, int) {
	end := bytes.IndexByte(stream[index:], '>')
	if end < 0 {
		return "", len(stream)
	}
	digits := make([]byte, 0, end)
	for _, b := range stream[index+1 : index+end] {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		digits = append(digits, b)
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	decoded := make([]byte, hex.DecodedLen(len(digits)))
	if _, err := hex.Decode(decoded, digits); err != nil {
		return "", index + end + 1
	}
	// A 2-byte-per-glyph string is font code, not text; keep only Latin-1 ASCII.
	runes := make([]rune, 0, len(decoded))
	for _, b := range decoded {
		if b < 0x80 {
			runes = append(runes, rune(b))
		}
	}
	return string(runes), index + end + 1
}

// sanitizePDFText drops control characters and squashes blank-line runs.
func sanitizePDFText(text string) string {
	var builder strings.Builder
	blank := 0
	for _, line := range strings.Split(text, "\n") {
		cleaned := strings.Map(func(r rune) rune {
			switch {
			case r == '\t' || r == '\n':
				return r
			case r < 0x20 || r == 0x7f:
				return -1
			}
			return r
		}, line)
		cleaned = strings.TrimRight(cleaned, " \t")
		if strings.TrimSpace(cleaned) == "" {
			blank++
			if blank > 1 {
				continue
			}
			builder.WriteByte('\n')
			continue
		}
		blank = 0
		builder.WriteString(strings.TrimSpace(cleaned) + "\n")
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

// plausibleText reports whether extracted text looks like language rather than
// undecoded font codes.
func plausibleText(text string) bool {
	letters, total := 0, 0
	for _, r := range text {
		if r == utf8.RuneError || r == 0xfffd {
			return false
		}
		total++
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsPunct(r) {
			letters++
		}
	}
	return total > 0 && letters*100 >= total*90
}

// ─── SQLite ──────────────────────────────────────────────────────────────────

// convertSQLite lists a SQLite database's tables and their columns. SQLite is not
// queried for row data: the model gets the schema it can then query through code.
func (s *fetchState) convertSQLite() (conversion, error) {
	file, err := writeTemp(s.body)
	if err != nil {
		return conversion{}, fmt.Errorf("SQLite body could not be written for reading")
	}
	defer os.Remove(file)
	db, err := sql.Open("sqlite", "file:"+file+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return conversion{}, fmt.Errorf("SQLite body could not be opened")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.Query(`SELECT type, name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return conversion{}, fmt.Errorf("SQLite schema could not be read")
	}
	type object struct{ kind, name string }
	var objects []object
	for rows.Next() {
		var entry object
		if err := rows.Scan(&entry.kind, &entry.name); err != nil {
			rows.Close()
			return conversion{}, fmt.Errorf("SQLite schema could not be read")
		}
		objects = append(objects, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return conversion{}, fmt.Errorf("SQLite schema could not be read")
	}
	var builder strings.Builder
	builder.WriteString("SQLite database with " + strconv.Itoa(len(objects)) + " tables/views:\n")
	for _, entry := range objects {
		builder.WriteString("\n### " + entry.kind + " " + entry.name + "\n")
		columns, err := db.Query("SELECT name, type FROM pragma_table_info(?)", entry.name)
		if err != nil {
			builder.WriteString("(columns unavailable)\n")
			continue
		}
		builder.WriteString("| column | type |\n| --- | --- |\n")
		for columns.Next() {
			var name, kind string
			if err := columns.Scan(&name, &kind); err != nil {
				break
			}
			builder.WriteString("| " + escapeTableCell(name) + " | " + escapeTableCell(kind) + " |\n")
		}
		columns.Close()
	}
	return conversion{method: "sqlite", text: builder.String()}, nil
}
