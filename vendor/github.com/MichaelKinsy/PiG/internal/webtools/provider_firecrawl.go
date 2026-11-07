package webtools

// Firecrawl keyless search.
//
// Ports packages/coding-agent/src/web/search/providers/firecrawl.ts. Firecrawl's
// search endpoint answers without an Authorization header, so it is the cheapest
// JSON provider in the chain. Search is SERP-backed, so the query keeps Google
// operators; absolute before:/after: bounds move to the native `tbs` parameter
// and are stripped from the query string.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	firecrawlSearchURL = "https://api.firecrawl.dev/v2/search"
	firecrawlResults   = 20
	firecrawlDefaultN  = 10
)

// recencyToFirecrawlTBS maps a recency window to Google's `qdr` tbs value.
var recencyToFirecrawlTBS = map[string]string{"day": "qdr:d", "week": "qdr:w", "month": "qdr:m", "year": "qdr:y"}

// firecrawlProvider calls api.firecrawl.dev/v2/search without credentials.
type firecrawlProvider struct {
	endpoint string
}

func newFirecrawl(endpoint string) SearchProvider {
	if endpoint == "" {
		endpoint = firecrawlSearchURL
	}
	return &firecrawlProvider{endpoint: endpoint}
}

func (p *firecrawlProvider) ID() string                  { return "firecrawl" }
func (p *firecrawlProvider) Label() string               { return providerMeta["firecrawl"] }
func (p *firecrawlProvider) IsAvailable() bool           { return true }
func (p *firecrawlProvider) IsExplicitlyAvailable() bool { return true }
func (p *firecrawlProvider) Syntax() QuerySyntax         { return GoogleQuerySyntax }

func (p *firecrawlProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	want := clampNumResults(params.NumResultsOrDefault(), firecrawlDefaultN, firecrawlResults)

	query := params.Query
	tbs := ""
	if parsed.HasDirectives {
		switch {
		case parsed.After != "" || parsed.Before != "":
			tbs = firecrawlDateTBS(parsed.After, parsed.Before)
			syntax := GoogleQuerySyntax
			syntax.DateRange = false
			query = FormatQuery(parsed, syntax)
		default:
			query = FormatQuery(parsed, GoogleQuerySyntax)
		}
	}
	if tbs == "" {
		tbs = recencyToFirecrawlTBS[params.Recency]
	}

	request := map[string]any{
		"query":   query,
		"limit":   want,
		"sources": []map[string]any{{"type": "web"}},
	}
	if tbs != "" {
		request["tbs"] = tbs
	}
	body, err := json.Marshal(request)
	if err != nil {
		return SearchResponse{}, &SearchError{Provider: p.ID(), Message: "Firecrawl request could not be encoded"}
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")

	client := NewHTTPClient()
	defer client.CloseIdleConnections()
	response, err := Load(ctx, client, http.MethodPost, p.endpoint, body, headers, DefaultMaxBytes)
	if err != nil {
		if ctx.Err() != nil {
			return SearchResponse{}, ctx.Err()
		}
		return SearchResponse{}, &SearchError{
			Provider: p.ID(),
			Message:  "Firecrawl " + classifyProviderMessage(p.ID(), err),
			Status:   response.StatusCode,
		}
	}

	var payload firecrawlResponse
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return SearchResponse{}, &SearchError{Provider: p.ID(), Message: "Firecrawl returned invalid JSON"}
	}
	if payload.Success != nil && !*payload.Success {
		message := normalizeSearchText(derefString(payload.Error))
		if message == "" {
			message = "Firecrawl request failed"
		}
		return SearchResponse{}, &SearchError{Provider: p.ID(), Message: message}
	}

	results := payload.webResults()
	sources := make([]SearchSource, 0, len(results))
	for _, result := range results {
		target := normalizeHTTPURL(derefString(result.URL))
		if target == "" {
			continue
		}
		title := normalizeSearchText(derefString(result.Title))
		if title == "" {
			title = target
		}
		snippet := derefString(result.Description)
		if strings.TrimSpace(snippet) == "" {
			snippet = derefString(result.Snippet)
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = derefString(result.Markdown)
		}
		sources = append(sources, SearchSource{Title: title, URL: target, Snippet: snippet})
		if len(sources) >= want {
			break
		}
	}
	return SearchResponse{
		Provider:  p.ID(),
		Sources:   sources,
		RequestID: derefString(payload.ID),
		AuthMode:  "keyless",
	}, nil
}

// firecrawlDateTBS builds Google's custom date range tbs value (MM/DD/YYYY).
func firecrawlDateTBS(after, before string) string {
	parts := []string{"cdr:1"}
	if after != "" {
		parts = append(parts, "cd_min:"+googleDate(after))
	}
	if before != "" {
		parts = append(parts, "cd_max:"+googleDate(before))
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, ",")
}

// googleDate converts ISO YYYY-MM-DD to Google's MM/DD/YYYY.
func googleDate(iso string) string {
	parts := strings.Split(iso, "-")
	if len(parts) != 3 {
		return iso
	}
	return parts[1] + "/" + parts[2] + "/" + parts[0]
}

// firecrawlResponse is the v2 search envelope. `data` is either a flat array or
// an object keyed by source type; both shapes are accepted.
type firecrawlResponse struct {
	Success *bool           `json:"success"`
	Error   *string         `json:"error"`
	ID      *string         `json:"id"`
	Data    json.RawMessage `json:"data"`
	Results []firecrawlItem `json:"results"`
}

// firecrawlItem is one result row.
type firecrawlItem struct {
	Title       *string `json:"title"`
	URL         *string `json:"url"`
	Description *string `json:"description"`
	Snippet     *string `json:"snippet"`
	Markdown    *string `json:"markdown"`
}

// webResults returns the web rows of the envelope, whatever shape `data` took.
func (r firecrawlResponse) webResults() []firecrawlItem {
	flat := make([]firecrawlItem, 0, 8)
	for _, raw := range []json.RawMessage{r.Data} {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" || trimmed == "null" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			var items []firecrawlItem
			if json.Unmarshal(raw, &items) == nil {
				flat = append(flat, items...)
			}
			continue
		}
		var grouped struct {
			Web []firecrawlItem `json:"web"`
		}
		if json.Unmarshal(raw, &grouped) == nil {
			flat = append(flat, grouped.Web...)
		}
	}
	if len(flat) == 0 {
		flat = append(flat, r.Results...)
	}
	return flat
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// FiltersRecency reports that the engine applies the recency window natively.
func (p *firecrawlProvider) FiltersRecency() bool { return true }

var _ SearchProvider = (*firecrawlProvider)(nil)
