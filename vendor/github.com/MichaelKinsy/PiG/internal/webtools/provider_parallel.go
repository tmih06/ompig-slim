package webtools

// Parallel keyless search over its public MCP endpoint.
//
// Ports packages/coding-agent/src/web/search/providers/parallel.ts. The public
// MCP at search.parallel.ai/mcp exposes one tool, `web_search`, taking
// `objective` + `search_queries`; its result is a `search_id` with `results[]`
// carrying url/title/publish_date/excerpts. 429 means the shared endpoint is
// rate limited. Verified against the live tools/list at the commit this ports.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	parallelMCPURL   = "https://search.parallel.ai/mcp"
	parallelTool     = "web_search"
	parallelResults  = 20
	parallelDefaultN = 10
)

// parallelQuerySyntax is what Parallel's natural-language objective parses.
var parallelQuerySyntax = QuerySyntax{Phrases: true, Negation: true, Or: true}

// parallelMCPSyntax is what its MCP search_queries accept: the search operators
// plus date bounds.
var parallelMCPSyntax = QuerySyntax{Phrases: true, Negation: true, Or: true, Site: true, DateRange: true}

// parallelProvider calls the keyless Parallel MCP.
type parallelProvider struct {
	endpoint string
}

func newParallel(endpoint string) SearchProvider {
	if endpoint == "" {
		endpoint = parallelMCPURL
	}
	return &parallelProvider{endpoint: endpoint}
}

func (p *parallelProvider) ID() string                  { return "parallel" }
func (p *parallelProvider) Label() string               { return providerMeta["parallel"] }
func (p *parallelProvider) IsAvailable() bool           { return true }
func (p *parallelProvider) IsExplicitlyAvailable() bool { return true }
func (p *parallelProvider) Syntax() QuerySyntax         { return parallelQuerySyntax }

func (p *parallelProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	want := clampNumResults(params.NumResultsOrDefault(), parallelDefaultN, parallelResults)

	objective := params.Query
	searchQuery := params.Query
	if parsed.HasDirectives {
		objective = FormatQuery(parsed, parallelQuerySyntax)
		searchQuery = FormatQuery(parsed, parallelMCPSyntax)
	}
	// recency is a pure time filter, and this engine does not honour one: the
	// public MCP accepts an `after:` operator but live results ignore it. The
	// window is therefore applied to the returned sources by the chain, not
	// approximated by rewriting the query. An explicit user `after:` directive
	// stays, because that is the caller's own constraint, not an inferred one.

	args := map[string]any{
		"objective":      objective,
		"search_queries": []string{searchQuery},
	}

	client := NewHTTPClient()
	defer client.CloseIdleConnections()
	result, err := callMCPTool(ctx, client, p.endpoint, p.ID(), parallelTool, args)
	if err != nil {
		return SearchResponse{}, explainMCPFailure(err, p.ID())
	}

	payload, ok := mcpJSONPayload(result)
	if !ok {
		return SearchResponse{}, &SearchError{Provider: p.ID(), Message: "Parallel MCP returned no search payload"}
	}
	sources, requestID, err := parseParallelPayload(payload)
	if err != nil {
		return SearchResponse{}, &SearchError{Provider: p.ID(), Message: err.Error()}
	}
	if len(sources) > want {
		sources = sources[:want]
	}
	return SearchResponse{
		Provider:  p.ID(),
		Sources:   sources,
		RequestID: requestID,
		AuthMode:  "keyless",
	}, nil
}

// parseParallelPayload reads Parallel's search envelope:
// {"search_id": "...", "results": [{url,title,publish_date,excerpts}]}.
func parseParallelPayload(payload json.RawMessage) ([]SearchSource, string, error) {
	var envelope struct {
		SearchID string `json:"search_id"`
		Results  []struct {
			URL         string   `json:"url"`
			Title       string   `json:"title"`
			PublishDate string   `json:"publish_date"`
			Excerpts    []string `json:"excerpts"`
		} `json:"results"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, "", &SearchError{Provider: "parallel", Message: "Parallel search payload was not understood"}
	}
	now := time.Now()
	sources := make([]SearchSource, 0, len(envelope.Results))
	for _, item := range envelope.Results {
		target := normalizeHTTPURL(item.URL)
		if target == "" {
			continue
		}
		title := normalizeSearchText(item.Title)
		if title == "" {
			title = target
		}
		snippet := ""
		excerpts := make([]string, 0, len(item.Excerpts))
		for _, excerpt := range item.Excerpts {
			if trimmed := strings.TrimSpace(excerpt); trimmed != "" {
				excerpts = append(excerpts, trimmed)
			}
		}
		if len(excerpts) > 0 {
			snippet = strings.Join(excerpts, "\n\n")
		}
		sources = append(sources, SearchSource{
			Title:         title,
			URL:           target,
			Snippet:       snippet,
			PublishedDate: normalizeSearchText(item.PublishDate),
			AgeSeconds:    dateToAgeSeconds(item.PublishDate, now),
		})
	}
	return sources, envelope.SearchID, nil
}

// explainMCPFailure maps a JSON-RPC transport error to the chain's message shape,
// keeping a rate-limit hint actionable.
func explainMCPFailure(err error, provider string) error {
	var searchErr *SearchError
	if !errors.As(err, &searchErr) {
		return err
	}
	if searchErr.Status == http.StatusTooManyRequests {
		searchErr.Message = provider + ": MCP rate limit reached (429) on the shared keyless endpoint; " +
			"try again later or reorder webSearch.chain"
	}
	return searchErr
}

// FiltersRecency reports that the engine does not enforce the window: the public
// MCP accepts an `after:` operator but live results ignore it, so the pipeline
// applies the window to the returned sources.
func (p *parallelProvider) FiltersRecency() bool { return false }

var _ SearchProvider = (*parallelProvider)(nil)
