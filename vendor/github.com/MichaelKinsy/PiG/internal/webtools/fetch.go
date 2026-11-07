package webtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DefaultFetchTimeout is the whole-call deadline a fetch uses when the caller sets none.
const DefaultFetchTimeout = 30 * time.Second

// Fetch deadlines are clamped to this range.
const (
	MinFetchTimeout = time.Second
	MaxFetchTimeout = 5 * time.Minute
)

// maxFetchBytes bounds the body a caller may ask for: the model only ever sees a
// fraction of it, so a larger cap only risks a slow or memory-hungry read.
const maxFetchBytes int64 = 1 << 30

// minRenderedChars is the smallest rendering a reader accepts: a page that shapes
// to less than this is not an article, whatever its markup claims.
const minRenderedChars = 100

// fetchAccept asks for a Markdown rendering first (Markdown for Agents endpoints
// answer with one), then HTML, then plain text.
const fetchAccept = "text/markdown;q=0.95, text/html;q=0.9, application/xhtml+xml;q=0.85, text/plain;q=0.6, application/json;q=0.6, */*;q=0.5"

// Target is a parsed fetch target: a normalized URL plus the modifiers a caller
// appended to it (`:raw`, `:50-80`).
type Target struct {
	// URL is the absolute http(s) URL to load.
	URL string
	// Raw skips every text-shaping step and returns the response body as text.
	Raw bool
	// Range selects lines of the rendered text; zero means the whole text.
	Range LineRange
}

// LineRange is a 1-based, inclusive line window. A zero Start means "from the
// first line"; a zero End means "to the last line".
type LineRange struct {
	Start int
	End   int
}

func (r LineRange) empty() bool { return r.Start == 0 && r.End == 0 }

// FetchOptions configures one fetch. The zero value selects the defaults.
type FetchOptions struct {
	// MaxBytes caps the response body; 0 selects DefaultMaxBytes.
	MaxBytes int64
	// Reader names the HTML reader: "auto" (default) or "native". Only the native
	// reader ships, so both select it; any other value is a configuration error.
	Reader string
	// Timeout bounds the whole call; 0 selects DefaultFetchTimeout.
	Timeout time.Duration
	// Client is the HTTP client to use; nil creates a request-scoped one whose idle
	// connections are closed before Fetch returns.
	Client *http.Client
	// Offset is the first line (1-based) of the rendered text to return; 0 means the
	// first selected line.
	Offset int
	// Limit is the number of lines to return; 0 means to the end.
	Limit int
}

func (o FetchOptions) withDefaults() (FetchOptions, error) {
	switch o.Reader {
	case "", "auto", "native":
		o.Reader = "native"
	default:
		return FetchOptions{}, fmt.Errorf("webFetch.reader %q is not supported (use \"auto\" or \"native\")", o.Reader)
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxBytes > maxFetchBytes {
		o.MaxBytes = maxFetchBytes
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultFetchTimeout
	}
	if o.Timeout < MinFetchTimeout {
		o.Timeout = MinFetchTimeout
	}
	if o.Timeout > MaxFetchTimeout {
		o.Timeout = MaxFetchTimeout
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	if o.Limit < 0 {
		o.Limit = 0
	}
	return o, nil
}

// ImageData is an image a fetch returns as bytes instead of text.
type ImageData struct {
	MIME string
	Data []byte
}

// FetchResult is one rendered document. Text holds the full rendering with the
// caller's line selection applied; it is never truncated by byte or line count
// here (that is the tool boundary's job).
type FetchResult struct {
	// URL is the final URL after redirects.
	URL string
	// Method names the pipeline step that produced the result, e.g. "html-native",
	// "markdown-alternate", "json", "pdf", "zip-listing".
	Method string
	// MediaType is the content type the pipeline settled on.
	MediaType string
	// Title is the document title when the source has one.
	Title string
	// Status is the HTTP status code.
	Status int
	// Truncated reports that the body hit the byte cap.
	Truncated bool
	// Text is the rendered text with the line selection applied.
	Text string
	// Image is set instead of Text for an image result.
	Image *ImageData
	// Notes explain fallbacks and degradations to the model.
	Notes []string
	// TotalLines is the line count of the full rendering, before selection.
	TotalLines int
	// StartLine and EndLine are the 1-based window of the full rendering that Text
	// holds. Both are 0 for an image result.
	StartLine int
	EndLine   int
}

// ParseTarget splits the selectors a caller appended to a URL and normalizes the
// URL. Selectors are stripped from the right while the remainder stays a valid
// http(s) URL with a path, so a port or a query value is never mistaken for one.
func ParseTarget(input string) (Target, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Target{}, fmt.Errorf("empty URL")
	}
	target := Target{}
	for {
		remainder, selector, ok := splitSelector(raw)
		if !ok {
			break
		}
		if selector == "raw" {
			target.Raw = true
		} else {
			match := lineSelectorPattern.FindStringSubmatch(selector)
			start, err := strconv.Atoi(match[1])
			if err != nil || start < 1 {
				return Target{}, fmt.Errorf("invalid line selector :%s", selector)
			}
			end := 0
			if match[2] != "" {
				if end, err = strconv.Atoi(match[2]); err != nil || end < start {
					return Target{}, fmt.Errorf("invalid line selector :%s", selector)
				}
			}
			target.Range = LineRange{Start: start, End: end}
		}
		raw = remainder
	}
	normalized, err := normalizeURL(raw)
	if err != nil {
		return Target{}, err
	}
	target.URL = normalized
	return target, nil
}

// lineSelectorPattern matches `START` or `START-END` (an empty END means "to the
// end of the document").
var lineSelectorPattern = regexp.MustCompile(`^([0-9]+)(?:-([0-9]*))?$`)

// splitSelector strips one trailing selector from input. It reports false unless
// the remainder is a fetchable URL that has a path, which keeps `host:8080` and
// `?v=1:2` intact.
func splitSelector(input string) (remainder, selector string, ok bool) {
	index := strings.LastIndexByte(input, ':')
	if index <= 0 || index == len(input)-1 {
		return "", "", false
	}
	selector = input[index+1:]
	if selector != "raw" && !lineSelectorPattern.MatchString(selector) {
		return "", "", false
	}
	remainder = input[:index]
	normalized, err := normalizeURL(remainder)
	if err != nil {
		return "", "", false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", "", false
	}
	// A selector needs a path to hang off; an empty one is `host:port`, where the
	// colon is part of the address. "/" is a real path, so `https://x/:50-80` works.
	if selector != "raw" && parsed.Path == "" {
		return "", "", false
	}
	return remainder, selector, true
}

// normalizeURL accepts an absolute http(s) URL, or a bare host such as
// `www.example.com/page`, which is fetched over https.
func normalizeURL(input string) (string, error) {
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return "", fmt.Errorf("empty URL")
	}
	if !strings.Contains(candidate, "://") {
		if !looksLikeHost(candidate) {
			return "", fmt.Errorf("invalid URL %q: provide an absolute http(s) URL", shorten(input))
		}
		candidate = "https://" + candidate
	}
	parsed, err := parseHTTPURL(candidate)
	if err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

// parseHTTPURL validates one candidate URL.
func parseHTTPURL(candidate string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q", shorten(candidate))
	}
	switch scheme := strings.ToLower(parsed.Scheme); scheme {
	case "http", "https":
	case "":
		return nil, fmt.Errorf("invalid URL %q: missing scheme", shorten(candidate))
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q: only http and https are fetched", scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("invalid URL %q: missing host", shorten(candidate))
	}
	if strings.ContainsAny(host, " \t\r\n") || !plausibleHost(host) {
		return nil, fmt.Errorf("invalid URL %q: bad host", shorten(candidate))
	}
	return parsed, nil
}

// looksLikeHost reports whether a scheme-less input names a host and not, say, a
// filesystem path.
func looksLikeHost(input string) bool {
	if strings.ContainsAny(input, " \t\r\n") || strings.HasPrefix(input, "/") {
		return false
	}
	host := input
	if index := strings.IndexAny(host, "/?#"); index >= 0 {
		host = host[:index]
	}
	if slash := strings.LastIndexByte(host, '@'); slash >= 0 {
		host = host[slash+1:]
	}
	if colon := strings.LastIndexByte(host, ':'); colon >= 0 {
		host = host[:colon]
	}
	return plausibleHost(host) && (strings.Contains(host, ".") || host == "localhost" || net.ParseIP(host) != nil)
}

// plausibleHost rejects hosts the URL parser tolerates but no fetch can reach.
func plausibleHost(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == '[' || r == ']' || r == ':':
		default:
			return false
		}
	}
	return true
}

// shorten keeps an error message a single bounded line.
func shorten(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if runes := []rune(value); len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return value
}

// conversion is what one pipeline step produced.
type conversion struct {
	method string
	text   string
	image  *ImageData
	title  string
	notes  []string
}

// fetchState carries one request through the pipeline.
type fetchState struct {
	ctx      context.Context
	client   *http.Client
	options  FetchOptions
	target   Target
	body     []byte
	media    string // settled media type
	declared string // the Content-Type header's base type
	finalURL string
}

// Fetch loads one URL and renders it to text or an image. Rendering failures fall
// back to a cruder representation with a note; only a target that cannot be
// represented at all (an unrenderable binary format) returns an error.
func Fetch(ctx context.Context, target string, options FetchOptions) (FetchResult, error) {
	parsed, err := ParseTarget(target)
	if err != nil {
		return FetchResult{}, err
	}
	options, err = options.withDefaults()
	if err != nil {
		return FetchResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	client := options.Client
	if client == nil {
		client = NewHTTPClient()
		defer client.CloseIdleConnections()
	}
	headers := make(http.Header)
	headers.Set("Accept", fetchAccept)
	response, err := Load(runCtx, client, http.MethodGet, parsed.URL, nil, headers, options.MaxBytes)
	if err != nil {
		return FetchResult{}, err
	}

	state := &fetchState{ctx: runCtx, client: client, options: options, target: parsed, body: response.Body, finalURL: response.URL}
	state.declared = baseMediaType(response.Header.Get("Content-Type"))
	state.media = classifyMedia(state.declared, response.URL, response.Body)

	result := FetchResult{URL: response.URL, Status: response.StatusCode, Truncated: response.Truncated, MediaType: state.media}
	if response.URL != parsed.URL {
		result.Notes = append(result.Notes, "redirected to "+response.URL)
	}
	if response.Truncated {
		result.Notes = append(result.Notes, fmt.Sprintf("response body truncated at %s", formatBytes(options.MaxBytes)))
	}

	converted, err := state.convert()
	if err != nil {
		// A fallback may have been cut short by the deadline; report that, not the
		// failure it caused.
		if ctxErr := runCtx.Err(); ctxErr != nil && ctxErr != context.DeadlineExceeded {
			return FetchResult{}, ctxErr
		}
		return FetchResult{}, err
	}
	// The rendering ladder makes auxiliary requests: never report success for a
	// call the caller has already cancelled.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return FetchResult{}, ctxErr
	}
	result.Method = converted.method
	result.Text = converted.text
	result.Image = converted.image
	result.Title = converted.title
	result.Notes = append(result.Notes, converted.notes...)

	if result.Image == nil {
		selected, start, end, total := selectLines(result.Text, parsed.Range, options.Offset, options.Limit)
		result.Text = selected
		result.StartLine, result.EndLine, result.TotalLines = start, end, total
	}
	return result, nil
}

// convert runs the branch the media type selects. Binary formats are handled
// before the :raw check: raw bytes of a PDF or a spreadsheet are not text, so
// :raw can only mean "body unchanged" for a format whose bytes are already text.
func (s *fetchState) convert() (conversion, error) {
	switch {
	case s.media == "image/svg+xml":
		// SVG is XML text; there is no local rasterizer, so hand over the source.
		return conversion{method: "svg-source", text: s.text(), notes: []string{"SVG source returned as text; no local rasterizer"}}, nil
	case strings.HasPrefix(s.media, "image/"):
		if !supportedImage(s.media) {
			return conversion{}, fmt.Errorf("unsupported image format %q: only png, jpeg, gif, webp and bmp can be attached", s.media)
		}
		return conversion{method: "image", image: &ImageData{MIME: s.media, Data: s.body}}, nil
	case s.media == "application/pdf":
		text, notes, err := convertPDF(s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "pdf", text: text, notes: notes}, nil
	case s.media == docxMediaType, s.media == pptxMediaType, s.media == xlsxMediaType:
		text, err := convertOffice(s.media, s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: officeMethod(s.media), text: text}, nil
	case s.media == epubMediaType:
		text, err := convertEPUB(s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "epub", text: text}, nil
	case s.media == "application/zip":
		return conversion{method: "zip-listing", text: listZip(s.body)}, nil
	case s.media == "application/x-tar":
		return conversion{method: "tar-listing", text: listTar(s.body)}, nil
	case s.media == "application/gzip":
		text, method, err := listGzip(s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: method, text: text}, nil
	case s.media == sqliteMediaType:
		return s.convertSQLite()
	}
	if s.target.Raw {
		method := "raw"
		if s.media == "text/html" || s.media == "application/xhtml+xml" {
			method = "html-raw"
		}
		return conversion{method: method, text: s.text()}, nil
	}
	switch {
	case s.media == notebookMediaType:
		text, err := convertNotebook(s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "notebook", text: text}, nil
	case s.media == "application/json":
		return conversion{method: "json", text: convertJSON(s.body)}, nil
	case isFeedMedia(s.media):
		text, err := convertFeed(s.body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "feed", text: text}, nil
	case s.media == "text/markdown" || s.media == "text/x-markdown":
		return conversion{method: "markdown", text: s.text()}, nil
	case s.media == "text/html" || s.media == "application/xhtml+xml":
		return s.renderHTML()
	case strings.HasPrefix(s.media, "text/"):
		return conversion{method: "text", text: s.text()}, nil
	default:
		if isTextual(s.body) {
			return conversion{method: "text", text: s.text()}, nil
		}
		return conversion{}, fmt.Errorf("unsupported content type %q: no local converter for this binary", s.media)
	}
}

// text decodes the body with the response charset (or the document's own meta
// charset for HTML).
func (s *fetchState) text() string { return decodeBody(s.body, s.declared, s.media) }

// renderHTML runs the HTML fallback ladder: the page's Markdown alternate, the
// native reader, a linked document, llms.txt, then raw HTML. A substantial but
// low-quality rendering is held and returned only if nothing better appears; a
// page whose rendering is not even substantial falls straight through to raw
// HTML, because a navigation shell carries no article text.
func (s *fetchState) renderHTML() (conversion, error) {
	document := parseHTML(s.body)
	title := HTMLTitle(document)
	if href, ok := markdownAlternate(document, s.finalURL); ok {
		markdown, err := s.fetchText(href)
		if err != nil {
			if ctxErr := s.ctx.Err(); ctxErr != nil {
				return conversion{}, ctxErr
			}
		} else if qualityOK(markdown) {
			return conversion{method: "markdown-alternate", text: markdown, title: title,
				notes: []string{"rendered from the page's Markdown alternate"}}, nil
		}
	}
	native := renderHTMLDocument(document, s.finalURL)
	if qualityOK(native) {
		return conversion{method: "html-native", text: native, title: title}, nil
	}
	substantial := len([]rune(strings.TrimSpace(native))) >= minRenderedChars
	if href, ok := documentLink(document, s.finalURL); ok {
		recovered, err := s.fetchDocument(href)
		switch {
		case err != nil && s.ctx.Err() != nil:
			return conversion{}, s.ctx.Err()
		case err == nil:
			recovered.notes = append(recovered.notes, "page text looked like navigation; recovered the article from a linked document")
			if recovered.title == "" {
				recovered.title = title
			}
			return recovered, nil
		}
	}
	for _, candidate := range llmsCandidates(s.finalURL) {
		text, err := s.fetchText(candidate)
		if err != nil {
			if ctxErr := s.ctx.Err(); ctxErr != nil {
				return conversion{}, ctxErr
			}
			continue
		}
		if len([]rune(strings.TrimSpace(text))) < minRenderedChars || !looksLikeLLMsTxt(text) {
			continue
		}
		return conversion{method: "llms-txt", text: text, title: title,
			notes: []string{"page text looked like navigation; used " + candidate}}, nil
	}
	if substantial {
		return conversion{method: "html-native", text: native, title: title,
			notes: []string{"native rendering looked like navigation; check the source URL"}}, nil
	}
	return conversion{method: "html-raw", text: s.text(), title: title,
		notes: []string{"native rendering found no article text; returning raw HTML. The page may need JavaScript or the URL may be an index."}}, nil
}

// fetchText loads an auxiliary text resource (a Markdown alternate, llms.txt).
func (s *fetchState) fetchText(target string) (string, error) {
	headers := make(http.Header)
	headers.Set("Accept", "text/markdown, text/plain;q=0.9, */*;q=0.5")
	response, err := Load(s.ctx, s.client, http.MethodGet, target, nil, headers, s.options.MaxBytes)
	if err != nil {
		return "", err
	}
	media := classifyMedia(baseMediaType(response.Header.Get("Content-Type")), response.URL, response.Body)
	if strings.HasPrefix(media, "image/") || !isTextual(response.Body) {
		return "", fmt.Errorf("auxiliary resource is not text")
	}
	return decodeBody(response.Body, response.Header.Get("Content-Type"), media), nil
}

// fetchDocument renders a document a page links to.
func (s *fetchState) fetchDocument(target string) (conversion, error) {
	headers := make(http.Header)
	response, err := Load(s.ctx, s.client, http.MethodGet, target, nil, headers, s.options.MaxBytes)
	if err != nil {
		return conversion{}, err
	}
	media := classifyMedia(baseMediaType(response.Header.Get("Content-Type")), response.URL, response.Body)
	switch {
	case media == "application/pdf":
		text, notes, err := convertPDF(response.Body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "pdf", text: text, notes: notes}, nil
	case media == docxMediaType:
		text, err := convertOffice(media, response.Body)
		if err != nil {
			return conversion{}, err
		}
		return conversion{method: "docx", text: text}, nil
	}
	return conversion{}, fmt.Errorf("linked resource is not a supported document")
}

// selectLines applies the URL's line range and the caller's offset/limit to the
// full rendering. It returns the selected text plus the 1-based window and the
// total line count.
func selectLines(text string, lineRange LineRange, offset, limit int) (string, int, int, int) {
	trimmed := strings.TrimSuffix(text, "\n")
	lines := strings.Split(trimmed, "\n")
	total := len(lines)
	if total == 1 && lines[0] == "" {
		return "", 0, 0, 0
	}
	// No selection at all returns the rendering verbatim, trailing newline included.
	if lineRange.empty() && offset == 0 && limit == 0 {
		return text, 1, total, total
	}
	start, end := 1, total
	if lineRange.Start > 0 {
		start = lineRange.Start
	}
	if lineRange.End > 0 {
		end = lineRange.End
	}
	if start > total {
		start = total + 1
	}
	if end > total {
		end = total
	}
	if offset > 0 {
		start += offset - 1
	}
	if limit > 0 && start+limit-1 < end {
		end = start + limit - 1
	}
	if start < 1 {
		start = 1
	}
	if end > total {
		end = total
	}
	if start > end {
		return "", start, end, total
	}
	return strings.Join(lines[start-1:end], "\n"), start, end, total
}

// baseMediaType is the lowercased content type without parameters.
func baseMediaType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mediaType)
}

// classifyMedia settles a media type: the header when it names one, otherwise the
// bytes and the URL's extension. A generic header (octet-stream, empty) defers to
// content sniffing so a mislabelled document is still rendered.
func classifyMedia(declared, target string, body []byte) string {
	switch declared {
	case "", "application/octet-stream", "binary/octet-stream", "text/plain":
		if sniffed := sniffMedia(body, target); sniffed != "" {
			// text/plain covers real text too; keep it unless the bytes prove otherwise.
			if declared == "text/plain" && !sniffedOverridesPlain(sniffed) {
				return declared
			}
			return sniffed
		}
		if declared == "" {
			return "text/plain"
		}
		return declared
	}
	return declared
}

// sniffedOverridesPlain reports whether a sniffed type is worth replacing a
// text/plain claim with.
func sniffedOverridesPlain(sniffed string) bool {
	switch sniffed {
	case "application/json", "text/html", "application/xhtml+xml", "application/pdf",
		"application/zip", "application/gzip", "application/x-tar", sqliteMediaType,
		notebookMediaType, docxMediaType, pptxMediaType, xlsxMediaType, epubMediaType:
		return true
	}
	return isFeedMedia(sniffed)
}

// Media type constants of the formats the pipeline converts.
const (
	docxMediaType     = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	pptxMediaType     = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	xlsxMediaType     = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	epubMediaType     = "application/epub+zip"
	sqliteMediaType   = "application/vnd.sqlite3"
	notebookMediaType = "application/x-ipynb+json"
)

// officeMethod maps an Office media type to its pipeline method name.
func officeMethod(media string) string {
	switch media {
	case docxMediaType:
		return "docx"
	case pptxMediaType:
		return "pptx"
	case xlsxMediaType:
		return "xlsx"
	}
	return "office"
}

// isFeedMedia reports the feed syntaxes the feed converter handles.
func isFeedMedia(media string) bool {
	switch media {
	case "application/rss+xml", "application/atom+xml", "application/rdf+xml", "text/xml", "application/xml":
		return true
	}
	return false
}

// sniffMedia identifies a body by its magic bytes, falling back to the URL's
// extension for formats without one (Office and EPUB are all ZIP archives).
func sniffMedia(body []byte, target string) string {
	switch {
	case bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(body, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(body, []byte("GIF87a")), bytes.HasPrefix(body, []byte("GIF89a")):
		return "image/gif"
	case len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP":
		return "image/webp"
	case bytes.HasPrefix(body, []byte("BM")):
		return "image/bmp"
	case bytes.HasPrefix(body, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(body, []byte("SQLite format 3\x00")):
		return sqliteMediaType
	case bytes.HasPrefix(body, []byte{0x1f, 0x8b}):
		return "application/gzip"
	case len(body) > 262 && string(body[257:262]) == "ustar":
		return "application/x-tar"
	case bytes.HasPrefix(body, []byte("PK\x03\x04")):
		return zipKind(target)
	}
	extension := lowerExtension(target)
	switch extension {
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".ipynb":
		return notebookMediaType
	case ".docx":
		return docxMediaType
	case ".pptx":
		return pptxMediaType
	case ".xlsx":
		return xlsxMediaType
	case ".epub":
		return epubMediaType
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gz", ".tgz":
		return "application/gzip"
	case ".rss":
		return "application/rss+xml"
	case ".atom":
		return "application/atom+xml"
	case ".md", ".markdown":
		return "text/markdown"
	}
	text := strings.TrimSpace(string(body))
	switch {
	case strings.HasPrefix(text, "{"), strings.HasPrefix(text, "["):
		if json.Valid(body) {
			return "application/json"
		}
	case strings.HasPrefix(strings.ToLower(text), "<rss"), strings.HasPrefix(strings.ToLower(text), "<feed"),
		strings.Contains(strings.ToLower(string(head([]byte(text), 512))), "<rdf:rdf"):
		return feedKind(text)
	case looksLikeHTML(text):
		return "text/html"
	}
	return ""
}

// zipKind picks the container behind ZIP magic from the URL extension.
func zipKind(target string) string {
	switch lowerExtension(target) {
	case ".docx":
		return docxMediaType
	case ".pptx":
		return pptxMediaType
	case ".xlsx":
		return xlsxMediaType
	case ".epub":
		return epubMediaType
	}
	return "application/zip"
}

// feedKind distinguishes RSS from Atom in a document whose content type is
// generic XML.
func feedKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "<rss"):
		return "application/rss+xml"
	case strings.HasPrefix(lower, "<feed"):
		return "application/atom+xml"
	default:
		return "application/rdf+xml"
	}
}

// supportedImage reports the still-image formats the tool can attach.
func supportedImage(media string) bool {
	switch media {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return true
	}
	return false
}

// isTextual reports whether a body is plausibly text: no NUL bytes and few
// control characters.
func isTextual(body []byte) bool {
	sample := body
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	control := 0
	for _, b := range sample {
		switch {
		case b == 0:
			return false
		case b < 0x09, b > 0x0d && b < 0x20:
			control++
		}
	}
	return control*100 <= len(sample)
}

// decodeBody decodes a body with the charset the header names, or the document's
// own declaration for HTML.
func decodeBody(body []byte, contentType, media string) string {
	charset := ""
	if _, params, err := mime.ParseMediaType(contentType); err == nil {
		charset = strings.ToLower(params["charset"])
	}
	if charset == "" && (media == "text/html" || media == "application/xhtml+xml") {
		charset = metaCharset(body)
	}
	return decodeCharset(body, charset)
}

// decodeCharset converts body to UTF-8. Without x/net/html/charset only the
// encodings vendored with x/text can be decoded exactly; the two single-byte
// encodings that matter in practice are handled here directly.
func decodeCharset(body []byte, charset string) string {
	if len(body) >= 2 {
		if body[0] == 0xfe && body[1] == 0xff {
			return decodeUTF16(body[2:], unicode.BigEndian)
		}
		if body[0] == 0xff && body[1] == 0xfe {
			return decodeUTF16(body[2:], unicode.LittleEndian)
		}
	}
	body = trimUTF8BOM(body)
	switch charset {
	case "utf-16", "utf-16le", "utf-16be":
		endianness := unicode.LittleEndian
		if charset == "utf-16be" {
			endianness = unicode.BigEndian
		}
		return decodeUTF16(body, endianness)
	case "iso-8859-1", "latin1", "latin-1", "us-ascii", "ascii":
		runes := make([]rune, len(body))
		for i, b := range body {
			runes[i] = rune(b)
		}
		return string(runes)
	case "windows-1252", "cp1252":
		var builder strings.Builder
		builder.Grow(len(body))
		for _, b := range body {
			if b >= 0x80 && b <= 0x9f {
				builder.WriteRune(windows1252[b-0x80])
				continue
			}
			builder.WriteRune(rune(b))
		}
		return builder.String()
	}
	if utf8.Valid(body) {
		return string(body)
	}
	return strings.ToValidUTF8(string(body), "\uFFFD")
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[3:]
	}
	return body
}

// decodeUTF16 decodes a BOM-less UTF-16 body.
func decodeUTF16(body []byte, endianness unicode.Endianness) string {
	encoding := unicode.UTF16(endianness, unicode.IgnoreBOM)
	decoded, _, err := transform.String(encoding.NewDecoder(), string(body))
	if err != nil {
		return strings.ToValidUTF8(string(body), "\uFFFD")
	}
	return decoded
}

// windows1252 maps 0x80..0x9f; the rest of the range is identical to Latin-1.
var windows1252 = [32]rune{
	0x20ac, 0x0081, 0x201a, 0x0192, 0x201e, 0x2026, 0x2020, 0x2021,
	0x02c6, 0x2030, 0x0160, 0x2039, 0x0152, 0x008d, 0x017d, 0x008f,
	0x0090, 0x2018, 0x2019, 0x201c, 0x201d, 0x2022, 0x2013, 0x2014,
	0x02dc, 0x2122, 0x0161, 0x203a, 0x0153, 0x009d, 0x017e, 0x0178,
}

// metaCharset reads a `<meta charset>` or `<meta http-equiv="content-type">`
// declaration from the head of an HTML document.
func metaCharset(body []byte) string {
	head := head(body, 2048)
	lower := strings.ToLower(string(head))
	for _, prefix := range []string{"charset=", "charset =\""} {
		index := strings.Index(lower, prefix)
		if index < 0 {
			continue
		}
		value := lower[index+len(prefix):]
		value = strings.TrimLeft(value, "\"' ")
		end := strings.IndexFunc(value, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_')
		})
		if end >= 0 {
			value = value[:end]
		}
		if value != "" {
			return value
		}
	}
	return ""
}

// head returns at most n leading bytes.
func head(body []byte, n int) []byte {
	if len(body) > n {
		return body[:n]
	}
	return body
}

// lowerExtension is the URL path's extension, lowercased.
func lowerExtension(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(path.Ext(parsed.Path))
}

// looksLikeHTML reports a document that starts like HTML markup.
func looksLikeHTML(text string) bool {
	lower := strings.ToLower(text)
	if len(lower) == 0 {
		return false
	}
	for _, prefix := range []string{"<!doctype html", "<html", "<head", "<body"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// formatBytes renders a byte count the way the truncation warnings do.
func formatBytes(bytes int64) string {
	switch {
	case bytes < 1024:
		return strconv.FormatInt(bytes, 10) + "B"
	case bytes < 1024*1024:
		return strconv.FormatFloat(float64(bytes)/1024, 'f', 1, 64) + "KB"
	default:
		return strconv.FormatFloat(float64(bytes)/(1024*1024), 'f', 1, 64) + "MB"
	}
}

// llmsCandidates are the well-known machine-readable summaries of a site.
func llmsCandidates(target string) []string {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return nil
	}
	root := parsed.Scheme + "://" + parsed.Host
	return []string{root + "/llms.txt", root + "/.well-known/llms.txt"}
}

// looksLikeLLMsTxt reports a body shaped like llms.txt: Markdown with a heading.
func looksLikeLLMsTxt(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "# ") {
		return true
	}
	return strings.Contains(trimmed, "\n# ")
}
