package webtools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// serve starts a local server for the test and returns its base URL.
func serve(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

// reply writes a body with a content type.
func reply(w http.ResponseWriter, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	_, _ = io.WriteString(w, body)
}

const articleHTML = `<html><head><title>Local fixture</title></head><body>
<nav><a href="/a">Home</a><a href="/b">Docs</a></nav>
<article><h1>Free web tools proof</h1>
<p>The compiled harness fetched this article through the native reader. Local fixture evidence proves readable content reaches the assistant without a browser, API key, or paid service.</p>
<p>FETCH_NATIVE_PROOF: the river flows north. This second substantial paragraph distinguishes the article from a short navigation page and exercises the quality gate.</p>
</article><script>UNSAFE_SCRIPT_PROOF</script></body></html>`

func TestFetchHTMLRendersMarkdown(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, "text/html; charset=utf-8", articleHTML)
	})
	result, err := Fetch(context.Background(), base+"/article", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "html-native" {
		t.Fatalf("method = %q, want html-native", result.Method)
	}
	if result.Title != "Local fixture" {
		t.Fatalf("title = %q", result.Title)
	}
	for _, want := range []string{"# Free web tools proof", "FETCH_NATIVE_PROOF", "Local fixture"} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("missing %q in:\n%s", want, result.Text)
		}
	}
	for _, unwanted := range []string{"UNSAFE_SCRIPT_PROOF", "<nav>"} {
		if strings.Contains(result.Text, unwanted) {
			t.Fatalf("chrome leaked into the rendering: %q", unwanted)
		}
	}
	if result.TotalLines == 0 || result.StartLine != 1 {
		t.Fatalf("line metadata: %d %d", result.StartLine, result.TotalLines)
	}
}

func TestFetchRawSkipsShaping(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, "text/html", articleHTML)
	})
	result, err := Fetch(context.Background(), base+"/article:raw", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "html-raw" {
		t.Fatalf("method = %q, want html-raw", result.Method)
	}
	if result.Text != articleHTML {
		t.Fatalf("raw body changed:\n%s", result.Text)
	}
}

func TestFetchLineSelectionAndOffset(t *testing.T) {
	body := strings.Join([]string{"line1", "line2", "line3", "line4", "line5"}, "\n")
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, "text/plain; charset=utf-8", body)
	})
	result, err := Fetch(context.Background(), base+"/x", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != body {
		t.Fatalf("plain text changed: %q", result.Text)
	}

	ranged, err := Fetch(context.Background(), base+"/x:2-4", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ranged.Text != "line2\nline3\nline4" {
		t.Fatalf("line range = %q", ranged.Text)
	}
	if ranged.StartLine != 2 || ranged.EndLine != 4 || ranged.TotalLines != 5 {
		t.Fatalf("range metadata: %+v", ranged)
	}

	offset, err := Fetch(context.Background(), base+"/x", FetchOptions{Offset: 2, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if offset.Text != "line2\nline3" || offset.StartLine != 2 || offset.EndLine != 3 {
		t.Fatalf("offset window = %q (%d-%d)", offset.Text, offset.StartLine, offset.EndLine)
	}
}

func TestFetchJSONAndFeed(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data.json":
			reply(w, "application/json", `{"b":1,"a":[1,2]}`)
		case "/feed.xml":
			reply(w, "application/rss+xml", `<?xml version="1.0"?><rss version="2.0"><channel><title>Fixture feed</title>
<item><title>First post</title><link>https://example.com/first</link><pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate><description>FEED_SNIPPET_PROOF</description></item>
</channel></rss>`)
		case "/atom.xml":
			reply(w, "application/atom+xml", `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Atom feed</title>
<entry><title>Entry one</title><link href="https://example.com/one"/><updated>2024-01-01T00:00:00Z</updated><summary>ATOM_SNIPPET_PROOF</summary></entry></feed>`)
		default:
			http.NotFound(w, r)
		}
	})

	jsonResult, err := Fetch(context.Background(), base+"/data.json", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if jsonResult.Method != "json" {
		t.Fatalf("json method = %q", jsonResult.Method)
	}
	if !strings.Contains(jsonResult.Text, "\n  \"b\": 1,") {
		t.Fatalf("json was not pretty-printed:\n%s", jsonResult.Text)
	}

	feed, err := Fetch(context.Background(), base+"/feed.xml", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if feed.Method != "feed" || !strings.Contains(feed.Text, "Fixture feed") ||
		!strings.Contains(feed.Text, "[First post](https://example.com/first)") ||
		!strings.Contains(feed.Text, "FEED_SNIPPET_PROOF") {
		t.Fatalf("rss rendering:\n%s", feed.Text)
	}

	atom, err := Fetch(context.Background(), base+"/atom.xml", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(atom.Text, "ATOM_SNIPPET_PROOF") {
		t.Fatalf("atom rendering:\n%s", atom.Text)
	}
}

func TestParseTargetNormalization(t *testing.T) {
	tests := []struct {
		input     string
		url       string
		raw       bool
		lineRange LineRange
		wantErr   bool
	}{
		{input: "https://example.com/page", url: "https://example.com/page"},
		{input: "www.example.com/page", url: "https://www.example.com/page"},
		{input: "example.com:8080/page:raw", url: "https://example.com:8080/page", raw: true},
		{input: "https://example.com/a:50-80", url: "https://example.com/a", lineRange: LineRange{Start: 50, End: 80}},
		{input: "https://example.com/a:12-", url: "https://example.com/a", lineRange: LineRange{Start: 12}},
		{input: "https://example.com/:50-80", url: "https://example.com/", lineRange: LineRange{Start: 50, End: 80}},
		{input: "https://example.com:8443/a:raw", url: "https://example.com:8443/a", raw: true},
		{input: "https://example.com:8443", url: "https://example.com:8443", lineRange: LineRange{}},
		{input: "ftp://example.com/x", wantErr: true},
		{input: "javascript:alert(1)", wantErr: true},
		{input: "/etc/passwd", wantErr: true},
		{input: "", wantErr: true},
		{input: "https://example.com/a:9-3", wantErr: true},
	}
	for _, test := range tests {
		target, err := ParseTarget(test.input)
		if test.wantErr {
			if err == nil {
				t.Fatalf("%q: expected an error, got %+v", test.input, target)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", test.input, err)
		}
		if target.URL != test.url || target.Raw != test.raw || target.Range != test.lineRange {
			t.Fatalf("%q => %+v", test.input, target)
		}
	}
}

func TestFetchQualityGateRejectsNavigation(t *testing.T) {
	navigation := `<html><head><title>Menu</title></head><body><nav><a href="/a">A</a><a href="/b">B</a></nav><div><ul>` +
		strings.Repeat(`<li><a href="/x">Short</a></li>`, 20) + `</ul></div></body></html>`
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			reply(w, "text/html", navigation)
			return
		}
		http.NotFound(w, r)
	})
	result, err := Fetch(context.Background(), base+"/page", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if qualityOK(RenderHTML([]byte(navigation), base)) {
		t.Fatal("the quality gate accepted a link farm")
	}
	if result.Text == "" {
		t.Fatal("a failed quality gate must still return the crude rendering")
	}
	if !hasNote(result.Notes, "looked like navigation") {
		t.Fatalf("notes = %v", result.Notes)
	}
}

func TestFetchMarkdownAlternateAndRecovery(t *testing.T) {
	body := strings.Repeat("Markdown alternate paragraph with enough words to pass the gate.\n\n", 4)
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			reply(w, "text/html", `<html><head><title>Alt</title><link rel="alternate" type="text/markdown" href="/page.md"></head>
<body><div><ul>`+strings.Repeat(`<li><a href="/x">Short</a></li>`, 20)+`</ul></div></body></html>`)
		case "/page.md":
			reply(w, "text/markdown", body)
		default:
			http.NotFound(w, r)
		}
	})
	result, err := Fetch(context.Background(), base+"/page", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "markdown-alternate" {
		t.Fatalf("method = %q, notes %v", result.Method, result.Notes)
	}
	if !strings.Contains(result.Text, "Markdown alternate paragraph") {
		t.Fatalf("alternate body missing:\n%s", result.Text)
	}
}

func TestFetchBodyCapNotes(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, "text/plain", strings.Repeat("x", 4096))
	})
	result, err := Fetch(context.Background(), base+"/big.txt", FetchOptions{MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Fatal("expected truncation")
	}
	if !hasNote(result.Notes, "truncated at 100B") {
		t.Fatalf("notes = %v", result.Notes)
	}
	if len(result.Text) != 100 {
		t.Fatalf("cap = %d bytes", len(result.Text))
	}
}

func TestFetchCancellation(t *testing.T) {
	started := make(chan struct{})
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	_, err := Fetch(ctx, base+"/stall", FetchOptions{Timeout: 10 * time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became %v", err)
	}
}

func TestFetchContentTypes(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.md":
			reply(w, "text/markdown", "# Heading\n\nBody with words.\n")
		case "/a.txt":
			reply(w, "text/plain; charset=utf-8", "plain body\n")
		case "/a.bin":
			reply(w, "application/x-unknown-binary", "\x00\x01\x02binary")
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nrest"))
		case "/a.svg":
			reply(w, "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"><text>svgtxt</text></svg>`)
		case "/unlabelled":
			_, _ = w.Write([]byte(`{"k": "v"}`))
		default:
			http.NotFound(w, r)
		}
	})

	markdown, err := Fetch(context.Background(), base+"/a.md", FetchOptions{})
	if err != nil || markdown.Method != "markdown" || !strings.Contains(markdown.Text, "# Heading") {
		t.Fatalf("markdown: %+v %v", markdown, err)
	}
	text, err := Fetch(context.Background(), base+"/a.txt", FetchOptions{})
	if err != nil || text.Method != "text" || text.Text != "plain body\n" {
		t.Fatalf("text: %+v %v", text, err)
	}
	if _, err := Fetch(context.Background(), base+"/a.bin", FetchOptions{}); err == nil {
		t.Fatal("an unconvertible binary must report unsupported conversion")
	}
	image, err := Fetch(context.Background(), base+"/a.png", FetchOptions{})
	if err != nil || image.Image == nil || image.Image.MIME != "image/png" {
		t.Fatalf("image: %+v %v", image, err)
	}
	svg, err := Fetch(context.Background(), base+"/a.svg", FetchOptions{})
	if err != nil || svg.Method != "svg-source" || !strings.Contains(svg.Text, "svgtxt") {
		t.Fatalf("svg: %+v %v", svg, err)
	}
	sniffed, err := Fetch(context.Background(), base+"/unlabelled", FetchOptions{})
	if err != nil || sniffed.Method != "json" {
		t.Fatalf("sniffed json: %+v %v", sniffed, err)
	}
}

func TestFetchHTTPErrorIsReported(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := Fetch(context.Background(), base+"/missing", FetchOptions{}); err == nil {
		t.Fatal("expected an error for 404")
	}
}

func TestFetchRejectsUnsupportedReader(t *testing.T) {
	if _, err := Fetch(context.Background(), "https://example.com/", FetchOptions{Reader: "trafilatura"}); err == nil {
		t.Fatal("expected a configuration error for an unsupported reader")
	}
}

func TestRenderHTMLTableAndPre(t *testing.T) {
	document := `<html><body><table><tr><th>Name</th><th>Value</th></tr><tr><td>a</td><td>1</td></tr></table>
<pre>code  line
  indented</pre><blockquote>quoted words here</blockquote><p>See <a href="/next">next page</a>.</p></body></html>`
	rendered := RenderHTML([]byte(document), "https://example.com/base")
	for _, want := range []string{"| Name | Value |", "| a | 1 |", "code  line", "> quoted words here", "[next page](https://example.com/next)"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in:\n%s", want, rendered)
		}
	}
}

func TestHTMLTitleFallsBackToHeading(t *testing.T) {
	node, err := html.Parse(strings.NewReader(`<html><body><h1>Heading title</h1><p>text</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := HTMLTitle(node); got != "Heading title" {
		t.Fatalf("title = %q", got)
	}
}

func TestDecodeCharset(t *testing.T) {
	if got := decodeBody([]byte("caf\xe9"), "text/plain; charset=iso-8859-1", "text/plain"); got != "café" {
		t.Fatalf("latin1 = %q", got)
	}
	if got := decodeBody([]byte("smart \x93quotes\x94"), "text/plain; charset=windows-1252", "text/plain"); got != "smart “quotes”" {
		t.Fatalf("cp1252 = %q", got)
	}
	utf16 := []byte{0xff, 0xfe, 'h', 0, 'i', 0}
	if got := decodeBody(utf16, "text/plain", "text/plain"); got != "hi" {
		t.Fatalf("utf16 = %q", got)
	}
}

func TestArchiveAndSQLiteAndNotebook(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipFixture(t, map[string]string{"docs/readme.txt": "hello", "docs/": ""}))
		case "/a.tar":
			w.Header().Set("Content-Type", "application/x-tar")
			_, _ = w.Write(tarFixture(t, map[string]string{"one.txt": "1"}))
		case "/a.ipynb":
			reply(w, "application/x-ipynb+json", `{"cells":[{"cell_type":"markdown","source":["# Notebook heading\n","Second line"]},{"cell_type":"code","source":"print(1)","outputs":[{"output_type":"stream","text":["1\n"]}]}]}`)
		case "/a.db":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(sqliteFixture(t))
		default:
			http.NotFound(w, r)
		}
	})

	zipResult, err := Fetch(context.Background(), base+"/a.zip", FetchOptions{})
	if err != nil || !strings.Contains(zipResult.Text, "docs/readme.txt") || !strings.Contains(zipResult.Text, "ZIP archive with 2 entries") {
		t.Fatalf("zip: %+v %v", zipResult, err)
	}
	tarResult, err := Fetch(context.Background(), base+"/a.tar", FetchOptions{})
	if err != nil || !strings.Contains(tarResult.Text, "one.txt") {
		t.Fatalf("tar: %+v %v", tarResult, err)
	}
	notebook, err := Fetch(context.Background(), base+"/a.ipynb", FetchOptions{})
	if err != nil || notebook.Method != "notebook" || !strings.Contains(notebook.Text, "# Notebook heading") || !strings.Contains(notebook.Text, "print(1)") {
		t.Fatalf("notebook: %+v %v", notebook, err)
	}
	sqliteResult, err := Fetch(context.Background(), base+"/a.db", FetchOptions{})
	if err != nil || sqliteResult.Method != "sqlite" || !strings.Contains(sqliteResult.Text, "items") {
		t.Fatalf("sqlite: %+v %v", sqliteResult, err)
	}
}

func hasNote(notes []string, want string) bool {
	for _, note := range notes {
		if strings.Contains(note, want) {
			return true
		}
	}
	return false
}

// TestFetchRawSkipsEveryTextShaping pins the rule that :raw only skips shaping:
// binary branches still run, and no textual branch reshapes the body.
func TestFetchRawSkipsEveryTextShaping(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data.json":
			reply(w, "application/json", `{"b":1,"a":2}`)
		case "/feed.xml":
			reply(w, "application/rss+xml", `<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title>
<item><title>Post</title><link>https://example.com/p</link><description>SUMMARY</description></item></channel></rss>`)
		case "/a.ipynb":
			reply(w, "application/x-ipynb+json", `{"cells":[{"cell_type":"markdown","source":"# Cell"}]}`)
		case "/report.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdfFixture())
		default:
			http.NotFound(w, r)
		}
	})

	jsonRaw, err := Fetch(context.Background(), base+"/data.json:raw", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if jsonRaw.Text != `{"b":1,"a":2}` {
		t.Fatalf("raw json was reshaped: %q", jsonRaw.Text)
	}

	feedRaw, err := Fetch(context.Background(), base+"/feed.xml:raw", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(feedRaw.Text, `<?xml`) {
		t.Fatalf("raw feed was reshaped: %q", feedRaw.Text)
	}

	notebookRaw, err := Fetch(context.Background(), base+"/a.ipynb:raw", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if notebookRaw.Text != `{"cells":[{"cell_type":"markdown","source":"# Cell"}]}` {
		t.Fatalf("raw notebook was reshaped: %q", notebookRaw.Text)
	}

	// Binary branches still run under :raw: a PDF is converted, not echoed.
	pdfRaw, err := Fetch(context.Background(), base+"/report.pdf:raw", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pdfRaw.Method != "pdf" {
		t.Fatalf("raw pdf method = %q", pdfRaw.Method)
	}
}

func TestFetchCancellationDuringFallback(t *testing.T) {
	background := make(chan struct{})
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			reply(w, "text/html", `<html><head><link rel="alternate" type="text/markdown" href="/page.md"></head><body><nav><a href="/x">x</a></nav></body></html>`)
		case "/page.md":
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(background)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-background
		cancel()
	}()
	if _, err := Fetch(ctx, base+"/page", FetchOptions{Timeout: 20 * time.Second}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fallback reported %v", err)
	}
}

func TestFetchNavigationPageYieldsRawHTML(t *testing.T) {
	page := `<html><head><title>Index</title></head><body><ul><li><a href="/x">Go</a></li><li><a href="/y">Hi</a></li></ul></body></html>`
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, "text/html", page)
	})
	result, err := Fetch(context.Background(), base+"/index", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "html-raw" {
		t.Fatalf("method = %q (notes %v), want the raw fallback for a short navigation page", result.Method, result.Notes)
	}
	if !strings.Contains(result.Text, "<html>") {
		t.Fatalf("raw HTML missing:\n%s", result.Text)
	}
	if !hasNote(result.Notes, "found no article text") {
		t.Fatalf("notes = %v", result.Notes)
	}
}

// TestConvertFeedLinkSpellings covers RSS text links, Atom href links and RDF
// root items.
func TestConvertFeedLinkSpellings(t *testing.T) {
	rss := `<?xml version="1.0"?><rss version="2.0"><channel><title>RSS</title>
<item><title>Rss post</title><link>https://example.com/rss</link></item></channel></rss>`
	atom := `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Atom</title>
<entry><title>Atom post</title><link rel="self" href="https://example.com/self"/><link rel="alternate" href="https://example.com/atom"/></entry></feed>`
	rdf := `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/">
<channel><title>RDF</title></channel><item><title>Rdf post</title><link>https://example.com/rdf</link></item></rdf:RDF>`

	for _, test := range []struct{ body, want string }{
		{rss, "[Rss post](https://example.com/rss)"},
		{atom, "[Atom post](https://example.com/atom)"},
		{rdf, "[Rdf post](https://example.com/rdf)"},
	} {
		text, err := convertFeed([]byte(test.body))
		if err != nil {
			t.Fatalf("%v: %v", test.body, err)
		}
		if !strings.Contains(text, test.want) {
			t.Fatalf("missing %q in:\n%s", test.want, text)
		}
	}
}
