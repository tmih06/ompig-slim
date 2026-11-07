//go:build !pig_strip_websearch

package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/internal/webtools"
)

// fixture is the local HTTP surface the tool tests drive: a DuckDuckGo bot wall,
// a Startpage results page, and a Firecrawl search endpoint.
type fixture struct {
	server *httptest.Server
	paths  []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fx := &fixture{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fx.paths = append(fx.paths, request.URL.Path)
		switch request.URL.Path {
		case "/ddg":
			writer.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(writer, `<html><body><div class="anomaly-modal">bots</div></body></html>`)
		case "/sp/search":
			// The default-chain test needs Startpage to refuse, so the chain
			// advances to the MCP engine; the success path is covered by the
			// webtools provider tests.
			writer.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(writer, `<html><body><script id="anubis_challenge" type="application/json">{"challenge":{}}</script></body></html>`)
		case "/exa":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"jsonrpc":"2.0","id":"1","result":{"results":[]}}`)
		case "/firecrawl":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"success":true,"id":"fc","data":{"web":[`+
				`{"url":"https://go.dev/blog/context","title":"Go Concurrency Patterns","description":"Contexts carry cancellation signals."},`+
				`{"url":"https://example.com/other","title":"Other","description":"Unrelated."}]}}`)
		case "/mcp":
			var rpc struct {
				ID json.RawMessage `json:"id"`
			}
			body, _ := io.ReadAll(request.Body)
			_ = json.Unmarshal(body, &rpc)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"jsonrpc":"2.0","id":`+string(rpc.ID)+`,"result":{"content":[{"type":"text","text":`+
				jsonQuote(`{"search_id":"p1","results":[{"url":"https://example.org/mcp","title":"MCP hit","excerpts":["from the keyless MCP"]}]}`)+`}]}}`)
		default:
			http.NotFound(writer, request)
		}
	})
	fx.server = httptest.NewServer(handler)
	t.Cleanup(fx.server.Close)
	return fx
}

func jsonQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// settingsFunc serves a fixed webSearch namespace.
func settingsFunc(namespace map[string]any) func() extension.Settings {
	return func() extension.Settings {
		if namespace == nil {
			return extension.Settings{}
		}
		return extension.Settings{"webSearch": namespace}
	}
}

func execute(t *testing.T, definition extension.ToolDefinition, params string) (extension.AgentToolResult, error) {
	t.Helper()
	return definition.Execute(context.Background(), "call-1", json.RawMessage(params), nil)
}

func resultText(t *testing.T, result extension.AgentToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d", len(result.Content))
	}
	text, ok := result.Content[0].(ai.TextContent)
	if !ok {
		t.Fatalf("content type = %T", result.Content[0])
	}
	return text.Text
}

func TestExecuteFallsBackAndReportsStructuredDetails(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"duckduckgo", "firecrawl"},
		"endpoints": map[string]any{"duckduckgo": fx.server.URL + "/ddg", "firecrawl": fx.server.URL + "/firecrawl"},
	})})

	result, err := execute(t, definition, `{"query":"golang context","limit":3}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "answered by Firecrawl after DuckDuckGo failed") {
		t.Fatalf("fallback note missing:\n%s", text)
	}
	if !strings.Contains(text, "https://go.dev/blog/context") {
		t.Fatalf("cited source missing:\n%s", text)
	}

	details, ok := result.Details.(webtools.SearchDetails)
	if !ok {
		t.Fatalf("details type = %T", result.Details)
	}
	if details.Response.Provider != "firecrawl" || len(details.Response.Sources) != 2 {
		t.Fatalf("details response = %+v", details.Response)
	}
	if len(details.Failures) != 1 || details.Failures[0].Provider != "duckduckgo" || details.Failures[0].Status != http.StatusTooManyRequests {
		t.Fatalf("details failures = %+v", details.Failures)
	}
	if len(details.Chain) != 2 || details.Chain[0] != "duckduckgo" {
		t.Fatalf("details chain = %v", details.Chain)
	}
	if details.Parsed == nil || details.Query != "golang context" {
		t.Fatalf("details query = %+v", details)
	}
}

func TestExecuteReportsRelaxedConstraint(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"firecrawl"},
		"endpoints": map[string]any{"firecrawl": fx.server.URL + "/firecrawl"},
	})})

	result, err := execute(t, definition, `{"query":"golang site:example.org"}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Note: no results matched `site:example.org`; the constraint was relaxed") {
		t.Fatalf("relaxation note missing:\n%s", text)
	}
	if !strings.Contains(text, "https://go.dev/blog/context") {
		t.Fatalf("relaxed results were dropped:\n%s", text)
	}
}

func TestExecuteAllProvidersFail(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"duckduckgo", "startpage"},
		"endpoints": map[string]any{"duckduckgo": fx.server.URL + "/ddg", "startpage": fx.server.URL + "/missing"},
	})})

	result, err := execute(t, definition, `{"query":"golang"}`)
	if err != nil {
		t.Fatalf("execute returned a Go error for a runtime failure: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError")
	}
	text := resultText(t, result)
	if !strings.HasPrefix(text, "Error: All web search providers failed: duckduckgo: ") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "; startpage: ") {
		t.Fatalf("ordered failures missing:\n%s", text)
	}
	details, ok := result.Details.(webtools.SearchDetails)
	if !ok || len(details.Failures) != 2 || details.Error == "" {
		t.Fatalf("details = %+v", result.Details)
	}
	if result.Preview == "" {
		t.Fatal("a failed call should carry a collapsed preview")
	}
}

func TestExecuteUnsupportedConfiguredProviderIsAnError(t *testing.T) {
	t.Parallel()
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain": []any{"brave"},
	})})
	_, err := execute(t, definition, `{"query":"golang"}`)
	if err == nil {
		t.Fatal("an unsupported chain name must be a returned error")
	}
	if !strings.Contains(err.Error(), "brave") || !strings.Contains(err.Error(), "supported") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteConfiguredChainReplacesDefault(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	// Only firecrawl is configured; the default chain's other engines must not run.
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"firecrawl"},
		"endpoints": map[string]any{"firecrawl": fx.server.URL + "/firecrawl"},
	})})
	result, err := execute(t, definition, `{"query":"golang"}`)
	if err != nil || result.IsError {
		t.Fatalf("execute: %v %s", err, resultText(t, result))
	}
	for _, path := range fx.paths {
		if path != "/firecrawl" {
			t.Fatalf("an unconfigured provider ran: %v", fx.paths)
		}
	}
}

func TestExecuteDefaultChainWithoutCredentials(t *testing.T) {
	t.Parallel()
	// Every default engine answers at the fixture; the first usable one wins, so
	// the test also pins the default order.
	fx := newFixture(t)
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"endpoints": map[string]any{
			"duckduckgo": fx.server.URL + "/ddg",
			"startpage":  fx.server.URL + "/sp/search",
			"parallel":   fx.server.URL + "/mcp",
			"exa":        fx.server.URL + "/exa",
			"firecrawl":  fx.server.URL + "/firecrawl",
		},
	})})

	result, err := execute(t, definition, `{"query":"golang context"}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("the default chain failed without credentials: %s", resultText(t, result))
	}
	details := result.Details.(webtools.SearchDetails)
	if strings.Join(details.Chain, ",") != "duckduckgo,startpage,parallel,exa,firecrawl" {
		t.Fatalf("chain = %v", details.Chain)
	}
	if details.Response.Provider != "parallel" {
		t.Fatalf("served = %q, want the first usable engine", details.Response.Provider)
	}
	if len(details.Failures) != 2 || details.Failures[0].Provider != "duckduckgo" || details.Failures[1].Provider != "startpage" {
		t.Fatalf("failures = %+v", details.Failures)
	}
}

func TestExecuteMCPProviderThroughEndpointOverride(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"parallel"},
		"endpoints": map[string]any{"parallel": fx.server.URL + "/mcp"},
	})})
	result, err := execute(t, definition, `{"query":"golang context"}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %s", resultText(t, result))
	}
	if !strings.Contains(resultText(t, result), "https://example.org/mcp") {
		t.Fatalf("MCP source missing:\n%s", resultText(t, result))
	}
}

func TestExecuteCancellationIsNotAFailure(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-release
		http.NotFound(writer, request)
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"firecrawl"},
		"endpoints": map[string]any{"firecrawl": server.URL},
	})})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, err := definition.Execute(ctx, "call-1", json.RawMessage(`{"query":"golang"}`), nil)
	if err == nil {
		t.Fatal("cancellation must return the context error, not a fallback failure")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	definition := Definition(tools.ExtendedBuiltinOptions{})
	cases := map[string]string{
		"empty query":      `{"query":"   "}`,
		"fractional limit": `{"query":"x","limit":2.5}`,
		"negative limit":   `{"query":"x","limit":-1}`,
		"bad recency":      `{"query":"x","recency":"decade"}`,
		"malformed json":   `{"query":`,
	}
	for name, params := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := execute(t, definition, params); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

func TestExecuteClampsAbsurdNumbers(t *testing.T) {
	t.Parallel()
	// 1e20 overflows a float-to-int conversion and a duration multiplication;
	// both are capped before either happens.
	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"duckduckgo"},
		"endpoints": map[string]any{"duckduckgo": "http://127.0.0.1:1/ddg"},
	})})
	args, err := parseArguments(json.RawMessage(`{"query":"x","limit":1e20,"num_search_results":1e20,"timeout":1e20,"max_tokens":1e20}`))
	if err != nil {
		t.Fatalf("parseArguments: %v", err)
	}
	if args.limit != webtools.MaxSearchResults || args.numResults != webtools.MaxSearchResults {
		t.Fatalf("limits = %d, %d", args.limit, args.numResults)
	}
	if args.timeoutSeconds != int(webtools.MaxSearchTimeout/time.Second) {
		t.Fatalf("timeoutSeconds = %d", args.timeoutSeconds)
	}
	_ = definition
}

func TestExecuteClampsLimitAndTimeout(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	var posted struct {
		Limit int `json:"limit"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &posted)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"success":true,"data":[]}`)
	}))
	defer server.Close()
	_ = fx

	definition := Definition(tools.ExtendedBuiltinOptions{Settings: settingsFunc(map[string]any{
		"chain":     []any{"firecrawl"},
		"endpoints": map[string]any{"firecrawl": server.URL},
	})})
	// An empty result set is a runtime failure (204), which is fine here: the
	// assertion is about the clamped request the provider sent.
	_, _ = execute(t, definition, `{"query":"x","limit":500,"timeout":99999}`)
	if posted.Limit != 20 {
		t.Fatalf("limit = %d, want the 20-result cap", posted.Limit)
	}
}
