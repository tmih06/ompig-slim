package webtools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeProvider is a recorded candidate: it returns a fixed response, error, or
// blocks until its context ends, and records how often it ran.
type fakeProvider struct {
	id        string
	label     string
	available bool
	explicit  bool
	response  SearchResponse
	err       error
	block     bool
	calls     int
	syntax    QuerySyntax
}

func (f *fakeProvider) ID() string                  { return f.id }
func (f *fakeProvider) Label() string               { return f.label }
func (f *fakeProvider) IsAvailable() bool           { return f.available }
func (f *fakeProvider) IsExplicitlyAvailable() bool { return f.explicit }
func (f *fakeProvider) Syntax() QuerySyntax {
	if f.syntax == (QuerySyntax{}) {
		return GoogleQuerySyntax
	}
	return f.syntax
}

func (f *fakeProvider) Search(ctx context.Context, _ SearchParams) (SearchResponse, error) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return SearchResponse{}, ctx.Err()
	}
	return f.response, f.err
}

func testParams(query string) SearchParams {
	return SearchParams{
		Query:   query,
		Parsed:  ParseSearchQuery(query),
		Limit:   5,
		Timeout: 2 * time.Second,
	}
}

func TestRunChainFallsBackAndNamesTheFailure(t *testing.T) {
	t.Parallel()
	first := &fakeProvider{id: "duckduckgo", label: "DuckDuckGo", available: true, explicit: true,
		err: &SearchError{Provider: "duckduckgo", Message: "bot-detection challenge", Status: 429}}
	second := &fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Sources: []SearchSource{{Title: "Go blog", URL: "https://go.dev/blog/context"}}}}

	result, err := runChain(context.Background(), []SearchProvider{first, second}, []string{"duckduckgo", "parallel"}, testParams("context cancellation"))
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if result.IsError {
		t.Fatalf("result reported failure: %s", result.Text)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("calls = %d, %d", first.calls, second.calls)
	}
	if !strings.Contains(result.Text, "answered by Parallel after DuckDuckGo failed (bot-detection challenge)") {
		t.Fatalf("fallback note missing:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "[1] Go blog\n    https://go.dev/blog/context") {
		t.Fatalf("cited source missing:\n%s", result.Text)
	}
	if len(result.Details.Failures) != 1 || result.Details.Failures[0].Provider != "duckduckgo" || result.Details.Failures[0].Status != 429 {
		t.Fatalf("failures = %+v", result.Details.Failures)
	}
	if result.Details.Response.Provider != "parallel" {
		t.Fatalf("response provider = %q", result.Details.Response.Provider)
	}
}

func TestRunChainAllFail(t *testing.T) {
	t.Parallel()
	a := &fakeProvider{id: "duckduckgo", label: "DuckDuckGo", available: true, explicit: true,
		err: errors.New("bot wall")}
	b := &fakeProvider{id: "startpage", label: "Startpage", available: true, explicit: true,
		err: errors.New("captcha challenge")}

	result, err := runChain(context.Background(), []SearchProvider{a, b}, []string{"duckduckgo", "startpage"}, testParams("context"))
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError")
	}
	if want := "Error: All web search providers failed: duckduckgo: bot wall; startpage: captcha challenge"; result.Text != want {
		t.Fatalf("text = %q, want %q", result.Text, want)
	}
	if len(result.Details.Failures) != 2 {
		t.Fatalf("ordered failures = %+v", result.Details.Failures)
	}
	if result.Details.Failures[0].Provider != "duckduckgo" || result.Details.Failures[1].Provider != "startpage" {
		t.Fatalf("failure order = %+v", result.Details.Failures)
	}
	if result.Details.Response.Provider != "startpage" {
		t.Fatalf("failed provider = %q", result.Details.Response.Provider)
	}
}

func TestRunChainSourceLessAnswerIsFailure(t *testing.T) {
	t.Parallel()
	// Prose without citations may have come from a model's weights, so it is a
	// failure (status 204), not a search result.
	a := &fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Answer: "Contexts cancel downward."}}
	b := &fakeProvider{id: "firecrawl", label: "Firecrawl", available: true, explicit: true,
		response: SearchResponse{Provider: "firecrawl", Sources: []SearchSource{{Title: "Docs", URL: "https://go.dev/doc/context"}}}}

	result, err := runChain(context.Background(), []SearchProvider{a, b}, []string{"parallel", "firecrawl"}, testParams("context"))
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if result.IsError || result.Details.Response.Provider != "firecrawl" {
		t.Fatalf("served = %q, error %v", result.Details.Response.Provider, result.IsError)
	}
	if result.Details.Failures[0].Status != 204 {
		t.Fatalf("sourceless status = %d", result.Details.Failures[0].Status)
	}
	if !strings.Contains(result.Details.Failures[0].Reason, "returned no sources") {
		t.Fatalf("reason = %q", result.Details.Failures[0].Reason)
	}
}

func TestRunChainCancellationIsNotAFallbackFailure(t *testing.T) {
	t.Parallel()
	blocked := &fakeProvider{id: "duckduckgo", label: "DuckDuckGo", available: true, explicit: true, block: true}
	second := &fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Sources: []SearchSource{{URL: "https://x.dev"}}}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := runChain(ctx, []SearchProvider{blocked, second}, []string{"duckduckgo", "parallel"}, testParams("context"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if second.calls != 0 {
		t.Fatalf("the chain advanced after cancellation (%d calls)", second.calls)
	}
}

func TestRunChainTimeoutAdvancesToNextProvider(t *testing.T) {
	t.Parallel()
	slow := &fakeProvider{id: "duckduckgo", label: "DuckDuckGo", available: true, explicit: true, block: true}
	second := &fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Sources: []SearchSource{{URL: "https://x.dev"}}}}

	params := testParams("context")
	params.Timeout = 20 * time.Millisecond
	result, err := runChain(context.Background(), []SearchProvider{slow, second}, []string{"duckduckgo", "parallel"}, params)
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %s", result.Text)
	}
	if !strings.Contains(result.Details.Failures[0].Reason, "timed out") {
		t.Fatalf("reason = %q", result.Details.Failures[0].Reason)
	}
}

func TestRunChainUnavailableExplicitProviderIsRecorded(t *testing.T) {
	t.Parallel()
	unavailable := &fakeProvider{id: "exa", label: "Exa", available: false, explicit: false}
	answer := &fakeProvider{id: "firecrawl", label: "Firecrawl", available: true, explicit: true,
		response: SearchResponse{Provider: "firecrawl", Sources: []SearchSource{{URL: "https://x.dev"}}}}

	result, err := runChain(context.Background(), []SearchProvider{unavailable, answer}, []string{"exa", "firecrawl"}, testParams("context"))
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if unavailable.calls != 0 {
		t.Fatal("an unavailable provider was called")
	}
	if len(result.Details.Failures) != 1 || result.Details.Failures[0].Reason != "unavailable" {
		t.Fatalf("failures = %+v", result.Details.Failures)
	}
}

func TestResolveChain(t *testing.T) {
	t.Parallel()
	defaultChain, err := ResolveChain(nil)
	if err != nil {
		t.Fatalf("default chain: %v", err)
	}
	if want := []string{"duckduckgo", "startpage", "parallel", "exa", "firecrawl"}; !equalStrings(defaultChain, want) {
		t.Fatalf("default chain = %v, want %v", defaultChain, want)
	}

	configured, err := ResolveChain([]string{"Firecrawl", " exa "})
	if err != nil {
		t.Fatalf("configured chain: %v", err)
	}
	if want := []string{"firecrawl", "exa"}; !equalStrings(configured, want) {
		t.Fatalf("configured chain = %v, want %v", configured, want)
	}

	if _, err := ResolveChain([]string{"duckduckgo", "brave"}); err == nil {
		t.Fatal("an unsupported provider name must be a configuration error")
	} else if !strings.Contains(err.Error(), "brave") || !strings.Contains(err.Error(), "supported") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunChainAppliesRecencyWhenTheEngineDoesNot(t *testing.T) {
	t.Parallel()
	recent := time.Now().AddDate(0, 0, -2).Format(time.RFC3339)
	old := "2019-01-02T00:00:00Z"
	provider := &fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Sources: []SearchSource{
			{Title: "Recent", URL: "https://x.dev/recent", PublishedDate: recent},
			{Title: "Old", URL: "https://x.dev/old", PublishedDate: old},
		}}}

	params := testParams("context")
	params.Recency = "week"
	result, err := runChain(context.Background(), []SearchProvider{provider}, []string{"parallel"}, params)
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if len(result.Details.Response.Sources) != 1 || result.Details.Response.Sources[0].Title != "Recent" {
		t.Fatalf("sources = %+v", result.Details.Response.Sources)
	}

	// A window that would empty the set is relaxed and reported.
	params.Recency = "day"
	result, err = runChain(context.Background(), []SearchProvider{provider}, []string{"parallel"}, params)
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if len(result.Details.Response.Sources) != 2 {
		t.Fatalf("relaxed sources = %+v", result.Details.Response.Sources)
	}
	if !strings.Contains(result.Text, "the recency filter was relaxed") {
		t.Fatalf("relaxation note missing:\n%s", result.Text)
	}
}

func TestRunChainReportsUnverifiableWindowAndLanguage(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{id: "exa", label: "Exa", available: true, explicit: true, syntax: exaSyntax,
		response: SearchResponse{Provider: "exa", Sources: []SearchSource{
			{Title: "Undated", URL: "https://x.dev/a"},
			{Title: "Recent", URL: "https://x.dev/b", PublishedDate: time.Now().AddDate(0, 0, -1).Format(time.RFC3339)},
		}}}

	params := testParams("context lang:fr")
	params.Recency = "week"
	result, err := runChain(context.Background(), []SearchProvider{provider}, []string{"exa"}, params)
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if len(result.Details.Response.Sources) != 2 {
		t.Fatalf("an undated source must not be dropped: %+v", result.Details.Response.Sources)
	}
	if !strings.Contains(result.Text, "reports no publish date for 1 of 2 results") {
		t.Fatalf("unverifiable-window note absent:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "does not filter by language, so `lang:fr` was not applied") {
		t.Fatalf("language note absent:\n%s", result.Text)
	}
}

func TestRunChainDoesNotInjectRecencyIntoMCPQueries(t *testing.T) {
	t.Parallel()
	// A provider that cannot filter by time must not have its query rewritten;
	// recency reaches it only through the parameter.
	recording := &recordingProvider{fakeProvider: fakeProvider{id: "parallel", label: "Parallel", available: true, explicit: true,
		response: SearchResponse{Provider: "parallel", Sources: []SearchSource{{URL: "https://x.dev"}}}}}
	params := testParams("golang context")
	params.Recency = "week"
	if _, err := runChain(context.Background(), []SearchProvider{recording}, []string{"parallel"}, params); err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if recording.params.Recency != "week" {
		t.Fatalf("recency = %q", recording.params.Recency)
	}
}

func TestRunChainReportsUnenforceableInTextOperator(t *testing.T) {
	t.Parallel()
	provider := &fakeProvider{id: "exa", label: "Exa", available: true, explicit: true, syntax: exaSyntax,
		response: SearchResponse{Provider: "exa", Sources: []SearchSource{{URL: "https://x.dev"}}}}
	result, err := runChain(context.Background(), []SearchProvider{provider}, []string{"exa"}, testParams(`context intext:deadline`))
	if err != nil {
		t.Fatalf("runChain: %v", err)
	}
	if !strings.Contains(result.Text, "cannot search page bodies") {
		t.Fatalf("missing-constraint note absent:\n%s", result.Text)
	}
}

func TestFormatSearchTextCapsSnippetsAndQueries(t *testing.T) {
	t.Parallel()
	snippet := strings.Repeat("é", 400)
	queries := []string{strings.Repeat("q", 200), "second", "third", "fourth"}
	text := FormatSearchText(SearchResponse{
		Provider:      "parallel",
		Sources:       []SearchSource{{Title: "T", URL: "https://x.dev", Snippet: snippet}},
		SearchQueries: queries,
	}, nil)

	if !strings.Contains(text, strings.Repeat("é", MaxSnippetRunes)) {
		t.Fatal("snippet was not capped at the rune limit")
	}
	if strings.Contains(text, strings.Repeat("é", MaxSnippetRunes+1)) {
		t.Fatal("snippet exceeded the rune limit")
	}
	if strings.Contains(text, "fourth") {
		t.Fatal("more than three search queries were listed")
	}
	if strings.Contains(text, strings.Repeat("q", MaxSearchQueryRunes+1)) {
		t.Fatal("a search query exceeded the rune limit")
	}
	if !strings.Contains(text, "Search queries: 4") {
		t.Fatalf("query count missing:\n%s", text)
	}
}

func TestClampHelpers(t *testing.T) {
	t.Parallel()
	if got := ClampLimit(0); got != DefaultSearchResults {
		t.Fatalf("ClampLimit(0) = %d", got)
	}
	if got := ClampLimit(999); got != MaxSearchResults {
		t.Fatalf("ClampLimit(999) = %d", got)
	}
	if got := ClampLimit(3); got != 3 {
		t.Fatalf("ClampLimit(3) = %d", got)
	}
	if got := ClampTimeout(0); got != DefaultSearchTimeout {
		t.Fatalf("ClampTimeout(0) = %v", got)
	}
	if got := ClampTimeout(10 * time.Minute); got != MaxSearchTimeout {
		t.Fatalf("ClampTimeout(10m) = %v", got)
	}
}

// recordingProvider captures the params the chain passed down.
type recordingProvider struct {
	fakeProvider
	params SearchParams
}

func (r *recordingProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	r.params = params
	return r.fakeProvider.Search(ctx, params)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
