package webtools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ddgFixture is a DuckDuckGo results page: two organic results (one behind the
// uddg= redirect wrapper), one sponsored y.js redirect, a snippet with a
// date-shaped value that must NOT be read as a publish date, and the next-page
// form.
const ddgFixture = `<!DOCTYPE html>
<html><body>
<div class="result results_links results_links_deep web-result">
  <div class="links_main links_deep result__body">
    <h2 class="result__title">
      <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2Fblog%2Fcontext&amp;rut=abc">Go Concurrency Patterns: <b>Context</b></a>
    </h2>
    <a class="result__snippet" href="https://go.dev/blog/context">The context package carries deadlines, cancellation signals, and request-scoped values.</a>
    <div class="result__extras">
      <div class="result__extras__url">
        <a class="result__url" href="https://go.dev/blog/context">go.dev/blog/context</a>
        <span class="result__timestamp">&nbsp; &nbsp; 2024-07-29T20:19:00.0000000</span>
      </div>
    </div>
  </div>
</div>
<div class="result results_links results_links_deep web-result">
  <div class="links_main links_deep result__body">
    <h2 class="result__title">
      <a rel="nofollow" class="result__a" href="https://pkg.go.dev/context">context package - Go Packages</a>
    </h2>
    <div class="result__snippet">Package context defines the Context type. Updated 2019-01-01 per the changelog.</div>
  </div>
</div>
<div class="result result--ad">
  <a class="result__a" href="//duckduckgo.com/y.js?ad_provider=bing">Sponsored placement</a>
</div>
<form class="nav-link" action="/html/" method="post">
  <input type="hidden" name="q" value="golang context" />
  <input type="hidden" name="s" value="20" />
  <input type="hidden" name="vqd" value="4-1234" />
</form>
</body></html>`

func TestDuckDuckGoParsesResults(t *testing.T) {
	t.Parallel()
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, string(body))
		writer.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(writer, ddgFixture)
	}))
	defer server.Close()

	provider, err := newProvider("duckduckgo", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{
		Query:   `golang context site:go.dev`,
		Parsed:  ParseSearchQuery(`golang context site:go.dev`),
		Recency: "week",
		Limit:   2,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %+v", result.Sources)
	}

	first := result.Sources[0]
	if first.URL != "https://go.dev/blog/context" {
		t.Fatalf("uddg redirect was not unwrapped: %q", first.URL)
	}
	if !strings.Contains(first.Title, "Go Concurrency Patterns") {
		t.Fatalf("title = %q", first.Title)
	}
	if first.PublishedDate != "2024-07-29T20:19:00.0000000" {
		t.Fatalf("publishedDate = %q", first.PublishedDate)
	}
	if first.AgeSeconds == nil {
		t.Fatal("ageSeconds was not derived from the publish date")
	}
	// The date-shaped text inside the snippet is not publication metadata.
	if result.Sources[1].PublishedDate != "" {
		t.Fatalf("snippet date leaked into publishedDate: %q", result.Sources[1].PublishedDate)
	}

	if len(bodies) != 1 {
		t.Fatalf("requests = %d", len(bodies))
	}
	form := bodies[0]
	if !strings.Contains(form, "q=golang+context+site%3Ago.dev") {
		t.Fatalf("query was not sent as a form: %s", form)
	}
	if !strings.Contains(form, "df=w") {
		t.Fatalf("recency was not mapped to df: %s", form)
	}
}

func TestDuckDuckGoBotWallIsDetected(t *testing.T) {
	t.Parallel()
	// DDG serves the challenge with 200 as often as 202; the marker is the signal.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(writer, `<html><body><div class="anomaly-modal__title">Unfortunately, bots use DuckDuckGo too.</div></body></html>`)
	}))
	defer server.Close()

	provider, err := newProvider("duckduckgo", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	_, err = provider.Search(context.Background(), SearchParams{Query: "golang", Limit: 5, Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("a bot wall must fail the provider, not return zero sources")
	}
	var searchErr *SearchError
	if !asSearchError(err, &searchErr) {
		t.Fatalf("error type = %T", err)
	}
	if searchErr.Status != http.StatusTooManyRequests {
		t.Fatalf("status = %d", searchErr.Status)
	}
	if !strings.Contains(searchErr.Message, "bot-detection challenge") {
		t.Fatalf("message = %q", searchErr.Message)
	}
}

func TestDuckDuckGoFollowsContinuationForm(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		body, _ := io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "text/html")
		if requests == 1 {
			if strings.Contains(string(body), "vqd=") {
				t.Errorf("the first request must be the initial form, not the continuation: %s", body)
			}
			_, _ = io.WriteString(writer, ddgFixture)
			return
		}
		if !strings.Contains(string(body), "vqd=4-1234") || !strings.Contains(string(body), "s=20") {
			t.Errorf("continuation fields missing: %s", body)
		}
		_, _ = io.WriteString(writer, `<div class="result"><a class="result__a" href="https://go.dev/doc/faq">FAQ</a></div>`)
	}))
	defer server.Close()

	provider, err := newProvider("duckduckgo", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{Query: "golang", Limit: 10, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want the continuation fetch", requests)
	}
	if len(result.Sources) != 3 {
		t.Fatalf("sources = %+v", result.Sources)
	}
}

func TestDuckDuckGoUnwrapsRedirectShapes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2Fblog%2Fcontext&rut=x": "https://go.dev/blog/context",
		"https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa%3Fb%3D1": "https://example.com/a?b=1",
		"//example.com/protocol-relative":                                      "https://example.com/protocol-relative",
		"https://example.com/absolute":                                         "https://example.com/absolute",
		"//duckduckgo.com/y.js?ad_provider=bing":                               "",
		"javascript:alert(1)":                                                  "",
	}
	for href, want := range cases {
		if got := unwrapDDGURL(href); got != want {
			t.Fatalf("unwrapDDGURL(%q) = %q, want %q", href, got, want)
		}
	}
}

// startpageHomeFixture serves the tokenized search form.
const startpageHomeFixture = `<html><body>
<form method="POST" action="/sp/search" id="search">
  <input type="hidden" name="sc" value="TOKEN123" />
  <input type="hidden" name="language" value="english" />
  <input type="text" name="query" value="" />
</form>
</body></html>`

// startpageResultsFixture holds two organic hits, a honeypot row and a sponsored
// row outside div.result.
const startpageResultsFixture = `<html><body>
<div class="result"><a class="result-link" href="https://go.dev/blog/context"><h2 class="wgl-title">Go Concurrency Patterns: Context</h2></a>
  <p class="description">The context package carries deadlines and cancellation signals.</p></div>
<div class="result"><a class="result-link" href="https://pkg.go.dev/context"><h3>context package</h3></a>
  <p class="description">Package context defines the Context type.</p></div>
<div class="a-bg-result"><a class="result-link" href="https://spam.example">honeypot</a></div>
<div class="sponsored"><a class="result-link" href="https://ads.example">sponsored</a></div>
</body></html>`

func TestStartpageTokenHandshake(t *testing.T) {
	t.Parallel()
	var sawHome, sawPost bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.Method == http.MethodGet {
			sawHome = true
			_, _ = io.WriteString(writer, startpageHomeFixture)
			return
		}
		body, _ := io.ReadAll(request.Body)
		form := string(body)
		if !strings.Contains(form, "sc=TOKEN123") || !strings.Contains(form, "query=golang+context") || !strings.Contains(form, "with_date=m") {
			t.Errorf("POST form = %s", form)
		}
		sawPost = true
		_, _ = io.WriteString(writer, startpageResultsFixture)
	}))
	defer server.Close()

	provider, err := newProvider("startpage", server.URL+"/sp/search")
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{
		Query:   "golang context",
		Parsed:  ParseSearchQuery("golang context"),
		Recency: "month",
		Limit:   10,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !sawHome || !sawPost {
		t.Fatalf("handshake incomplete: home %v post %v", sawHome, sawPost)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %+v", result.Sources)
	}
	if result.Sources[0].URL != "https://go.dev/blog/context" || !strings.Contains(result.Sources[0].Snippet, "cancellation") {
		t.Fatalf("first source = %+v", result.Sources[0])
	}
}

func TestStartpageChallengeIsDetected(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(writer, startpageHomeFixture)
			return
		}
		// The Gatsby CAPTCHA shell names its page components in the chunk map.
		_, _ = io.WriteString(writer, `<html><body><script>component---src-pages-captcha-js</script></body></html>`)
	}))
	defer server.Close()

	provider, err := newProvider("startpage", server.URL+"/sp/search")
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	_, err = provider.Search(context.Background(), SearchParams{Query: "golang", Limit: 5, Timeout: 2 * time.Second})
	var searchErr *SearchError
	if !asSearchError(err, &searchErr) {
		t.Fatalf("error = %v", err)
	}
	if searchErr.Status != http.StatusTooManyRequests || !strings.Contains(searchErr.Message, "CAPTCHA") {
		t.Fatalf("error = %+v", searchErr)
	}
}

func TestStartpageFallsBackToTokenlessGet(t *testing.T) {
	t.Parallel()
	var sawGetSearch bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		if request.URL.Path == "/" {
			// No token in the form: the handshake cannot run.
			_, _ = io.WriteString(writer, `<html><body><form action="/sp/search"><input type="text" name="query"></form></body></html>`)
			return
		}
		if request.Method == http.MethodGet && strings.Contains(request.URL.RawQuery, "query=golang") {
			sawGetSearch = true
			_, _ = io.WriteString(writer, startpageResultsFixture)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	provider, err := newProvider("startpage", server.URL+"/sp/search")
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{Query: "golang", Limit: 10, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !sawGetSearch {
		t.Fatal("the provider did not degrade to a tokenless GET")
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %+v", result.Sources)
	}
}

func TestFirecrawlParsesWebGroupAndPostsLimit(t *testing.T) {
	t.Parallel()
	var posted struct {
		Query   string              `json:"query"`
		Limit   int                 `json:"limit"`
		Sources []map[string]string `json:"sources"`
		TBS     string              `json:"tbs"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s", request.Method)
		}
		if request.Header.Get("Authorization") != "" {
			t.Error("keyless firecrawl sent an Authorization header")
		}
		body, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(body, &posted); err != nil {
			t.Errorf("body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"success":true,"id":"fc-1","data":{"web":[{"url":"https://go.dev/blog/context","title":"Go Concurrency Patterns","description":"Contexts carry cancellation."}]}}`)
	}))
	defer server.Close()

	provider, err := newProvider("firecrawl", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{
		Query:   "golang context",
		Parsed:  ParseSearchQuery("golang context"),
		Recency: "week",
		Limit:   3,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if posted.Query != "golang context" || posted.Limit != 3 || posted.TBS != "qdr:w" {
		t.Fatalf("posted = %+v", posted)
	}
	if len(posted.Sources) != 1 || posted.Sources[0]["type"] != "web" {
		t.Fatalf("sources = %+v", posted.Sources)
	}
	if len(result.Sources) != 1 || result.RequestID != "fc-1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestFirecrawlFlatDataAndExplicitDates(t *testing.T) {
	t.Parallel()
	var posted struct {
		Query string `json:"query"`
		TBS   string `json:"tbs"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &posted)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"success":true,"data":[{"url":"https://example.com/a","title":"A","snippet":"flat array"}]}`)
	}))
	defer server.Close()

	provider, err := newProvider("firecrawl", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	query := `golang site:go.dev after:2024-01-02 before:2024-06-01`
	result, err := provider.Search(context.Background(), SearchParams{
		Query:   query,
		Parsed:  ParseSearchQuery(query),
		Limit:   5,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if posted.TBS != "cdr:1,cd_min:01/02/2024,cd_max:06/01/2024" {
		t.Fatalf("tbs = %q", posted.TBS)
	}
	if strings.Contains(posted.Query, "after:") || strings.Contains(posted.Query, "before:") {
		t.Fatalf("absolute dates were not stripped from the query: %q", posted.Query)
	}
	if !strings.Contains(posted.Query, "site:go.dev") {
		t.Fatalf("site: was stripped too: %q", posted.Query)
	}
	if len(result.Sources) != 1 || result.Sources[0].Snippet != "flat array" {
		t.Fatalf("sources = %+v", result.Sources)
	}
}

func TestFirecrawlErrorEnvelope(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"success":false,"error":"Rate limit exceeded"}`)
	}))
	defer server.Close()

	provider, err := newProvider("firecrawl", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	_, err = provider.Search(context.Background(), SearchParams{Query: "x", Limit: 1, Timeout: 2 * time.Second})
	var searchErr *SearchError
	if !asSearchError(err, &searchErr) || searchErr.Message != "Rate limit exceeded" {
		t.Fatalf("error = %v", err)
	}
}

// parallelPayload is the envelope Parallel's keyless MCP text block carries.
const parallelPayload = `{"search_id":"search_abc","results":[{"url":"https://go.dev/blog/context","title":"Go Concurrency Patterns: Context","publish_date":"2014-07-29T00:00:00Z","excerpts":["Contexts carry deadlines.","A Context may be canceled."]}]}`

func TestParallelMCPJSONResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Errorf("Accept = %q", request.Header.Get("Accept"))
		}
		var rpc struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		body, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(body, &rpc); err != nil {
			t.Errorf("body: %v", err)
		}
		if rpc.Method != "tools/call" || rpc.Params.Name != "web_search" {
			t.Errorf("request = %s", body)
		}
		if _, ok := rpc.Params.Arguments["objective"]; !ok {
			t.Errorf("objective missing: %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"jsonrpc":"2.0","id":`+string(rpc.ID)+`,"result":{"content":[{"type":"text","text":`+jsonString(parallelPayload)+`}]}}`)
	}))
	defer server.Close()

	provider, err := newProvider("parallel", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{Query: "golang context", Limit: 5, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Sources) != 1 {
		t.Fatalf("sources = %+v", result.Sources)
	}
	source := result.Sources[0]
	if source.URL != "https://go.dev/blog/context" || !strings.Contains(source.Snippet, "Contexts carry deadlines.") {
		t.Fatalf("source = %+v", source)
	}
	if source.AgeSeconds == nil {
		t.Fatal("ageSeconds was not derived")
	}
	if result.RequestID != "search_abc" {
		t.Fatalf("requestId = %q", result.RequestID)
	}
}

func TestParallelDoesNotRewriteQueryForRecency(t *testing.T) {
	t.Parallel()
	var arguments map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpc struct {
			ID     json.RawMessage `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &rpc)
		arguments = rpc.Params.Arguments
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"jsonrpc":"2.0","id":`+string(rpc.ID)+`,"result":{"content":[{"type":"text","text":`+jsonString(parallelPayload)+`}]}}`)
	}))
	defer server.Close()

	provider, err := newProvider("parallel", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := provider.Search(context.Background(), SearchParams{
		Query:   "golang context",
		Parsed:  ParseSearchQuery("golang context"),
		Recency: "week",
		Limit:   5,
		Timeout: 2 * time.Second,
	}); err != nil {
		t.Fatalf("search: %v", err)
	}
	// recency is a pure time filter: this engine ignores it, so it must not be
	// approximated by rewriting the query.
	queries, _ := arguments["search_queries"].([]any)
	if len(queries) != 1 {
		t.Fatalf("search_queries = %v", arguments["search_queries"])
	}
	if got, _ := queries[0].(string); strings.Contains(got, "after:") {
		t.Fatalf("the recency window was injected into the query: %q", got)
	}
}

func TestParallelMCPSseResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpc struct {
			ID json.RawMessage `json:"id"`
		}
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &rpc)
		writer.Header().Set("Content-Type", "text/event-stream")
		// A notification first, then the matching response, as a real MCP server
		// may interleave them.
		_, _ = io.WriteString(writer, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
		_, _ = io.WriteString(writer, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"other\",\"result\":{\"content\":[]}}\n\n")
		_, _ = io.WriteString(writer, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":"+string(rpc.ID)+`,"result":{"structuredContent":`+parallelPayload+`}}`+"\n\n")
	}))
	defer server.Close()

	provider, err := newProvider("parallel", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{Query: "golang context", Limit: 5, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Sources) != 1 || result.Sources[0].URL != "https://go.dev/blog/context" {
		t.Fatalf("sources = %+v", result.Sources)
	}
}

func TestParallelMCPErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body   string
		status int
		want   string
	}{
		"rate limited": {body: `{"error":"slow down"}`, status: http.StatusTooManyRequests, want: "MCP rate limit reached"},
		"jsonrpc error": {
			body:   `{"jsonrpc":"2.0","id":ID,"error":{"code":-32000,"message":"quota exhausted"}}`,
			status: http.StatusOK, want: "quota exhausted",
		},
		"tool error": {
			body:   `{"jsonrpc":"2.0","id":ID,"result":{"isError":true,"content":[{"type":"text","text":"search unavailable"}]}}`,
			status: http.StatusOK, want: "search unavailable",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var rpc struct {
					ID json.RawMessage `json:"id"`
				}
				body, _ := io.ReadAll(request.Body)
				_ = json.Unmarshal(body, &rpc)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(tc.status)
				_, _ = io.WriteString(writer, strings.ReplaceAll(tc.body, "ID", string(rpc.ID)))
			}))
			defer server.Close()

			provider, err := newProvider("parallel", server.URL)
			if err != nil {
				t.Fatalf("provider: %v", err)
			}
			_, err = provider.Search(context.Background(), SearchParams{Query: "x", Limit: 1, Timeout: 2 * time.Second})
			if err == nil {
				t.Fatal("expected a provider failure")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// exaTextPayload is the text block Exa's keyless MCP returns, verified live: one
// Title/URL/Published/Author/Highlights block per result, separated by ---.
const exaTextPayload = `Title: Go Concurrency Patterns: Context - The Go Programming Language
URL: https://go.dev/blog/context
Published: 2014-07-29T00:00:00.000Z
Author: Sameer Ajmani
Highlights:
Its methods are safe for simultaneous use by multiple goroutines.

...

Done()

---

Title: context package - context - Go Packages
URL: https://pkg.go.dev/context
Published: 2026-10-20T00:00:00.000Z
Author: N/A
Highlights:
Package context defines the Context type, which carries deadlines.
`

func TestExaTextPayloadParsing(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpc struct {
			ID     json.RawMessage `json:"id"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		body, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(body, &rpc); err != nil {
			t.Errorf("body: %v", err)
		}
		if rpc.Params.Name != "web_search_exa" {
			t.Errorf("tool = %q", rpc.Params.Name)
		}
		if _, ok := rpc.Params.Arguments["objective"]; !ok {
			t.Errorf("objective missing: %s", body)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":"+string(rpc.ID)+`,"result":{"content":[{"type":"text","text":`+jsonString(exaTextPayload)+`}]}}`+"\n\n")
	}))
	defer server.Close()

	provider, err := newProvider("exa", server.URL)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	result, err := provider.Search(context.Background(), SearchParams{Query: "golang context", Limit: 10, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %+v", result.Sources)
	}
	first := result.Sources[0]
	if first.Title != "Go Concurrency Patterns: Context - The Go Programming Language" || first.URL != "https://go.dev/blog/context" {
		t.Fatalf("first = %+v", first)
	}
	if first.Author != "Sameer Ajmani" || first.PublishedDate != "2014-07-29T00:00:00.000Z" {
		t.Fatalf("metadata = %+v", first)
	}
	if !strings.Contains(first.Snippet, "simultaneous use") {
		t.Fatalf("snippet = %q", first.Snippet)
	}
	// "N/A" is not an author.
	if result.Sources[1].Author != "" {
		t.Fatalf("author = %q", result.Sources[1].Author)
	}
}

func TestExaJSONPayloadParsing(t *testing.T) {
	t.Parallel()
	payload := `{"results":[{"title":"Docs","url":"https://go.dev/doc/context","author":"Go team","publishedDate":"2024-01-02T00:00:00Z","summary":"Contexts are canceled downward.","highlights":["h1"]}]}`
	sources, ok := parseExaJSONResults([]byte(payload))
	if !ok || len(sources) != 1 {
		t.Fatalf("sources = %+v ok %v", sources, ok)
	}
	if sources[0].Snippet != "Contexts are canceled downward." {
		t.Fatalf("snippet = %q", sources[0].Snippet)
	}
}

func TestNewProviderRejectsUnknownID(t *testing.T) {
	t.Parallel()
	if _, err := newProvider("brave", ""); err == nil {
		t.Fatal("an unsupported provider must be an error")
	}
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
