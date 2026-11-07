package webtools

// Search provider contract and the unified search response.
//
// Ports packages/coding-agent/src/web/search/providers/base.ts and
// packages/tui/src/tools/web-search-types.ts. Credentials are not modelled: every
// provider in this file is reachable without an API key.

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SearchSource is one result. Snippet and PublishedDate are best-effort; the
// model-visible snippet is capped by FormatSearchText, not here.
type SearchSource struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Snippet       string `json:"snippet,omitempty"`
	PublishedDate string `json:"publishedDate,omitempty"`
	AgeSeconds    *int   `json:"ageSeconds,omitempty"`
	Author        string `json:"author,omitempty"`
}

// SearchCitation is a provider-supplied citation with context. Grounded
// providers emit these; the free providers here do not.
type SearchCitation struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	CitedText string `json:"citedText,omitempty"`
}

// SearchUsage is the token usage a grounded provider reports.
type SearchUsage struct {
	InputTokens    int `json:"inputTokens,omitempty"`
	OutputTokens   int `json:"outputTokens,omitempty"`
	SearchRequests int `json:"searchRequests,omitempty"`
}

// SearchResponse is the unified result both the text and the Details carry.
//
// A response with neither Sources nor Citations is a failure, not an answer: a
// provider that returns prose without citations may have answered from weights.
type SearchResponse struct {
	Provider         string           `json:"provider"`
	Answer           string           `json:"answer,omitempty"`
	Sources          []SearchSource   `json:"sources"`
	Citations        []SearchCitation `json:"citations,omitempty"`
	SearchQueries    []string         `json:"searchQueries,omitempty"`
	RelatedQuestions []string         `json:"relatedQuestions,omitempty"`
	Usage            *SearchUsage     `json:"usage,omitempty"`
	Model            string           `json:"model,omitempty"`
	RequestID        string           `json:"requestId,omitempty"`
	AuthMode         string           `json:"authMode,omitempty"`
}

// SearchParams are the per-call inputs every provider receives.
type SearchParams struct {
	// Query is the raw user query.
	Query string
	// Parsed is the structured view; the pipeline always sets it, providers may
	// parse themselves when called directly.
	Parsed *StructuredQuery
	// Limit is the caller's result cap (1..20).
	Limit int
	// Recency is a pure time filter: "", "day", "week", "month" or "year".
	Recency string
	// Timeout bounds one provider transport.
	Timeout time.Duration
	// NumResults is provider-native breadth; falls back to Limit.
	NumResults int
	// Endpoints overrides a provider's transport URL (webSearch.endpoints).
	Endpoints map[string]string
}

// NumResultsOrDefault is the effective result count: NumResults, else Limit.
func (p SearchParams) NumResultsOrDefault() int {
	if p.NumResults > 0 {
		return p.NumResults
	}
	return p.Limit
}

// SearchProvider is one sequential-fallback candidate.
type SearchProvider interface {
	// ID is the stable chain name (duckduckgo, parallel, …).
	ID() string
	// Label is the human name the failure notes quote.
	Label() string
	// IsAvailable reports whether the provider can serve a request now. Free
	// providers always can; an implicit candidate answering false is skipped.
	IsAvailable() bool
	// IsExplicitlyAvailable admits a provider a configured chain names even when
	// IsAvailable refuses it (keyless public endpoints stay out of the default).
	IsExplicitlyAvailable() bool
	// Syntax is the query-syntax mask the provider's engine parses.
	Syntax() QuerySyntax
	// Search runs one query. The body must be bounded and must honour ctx.
	Search(ctx context.Context, params SearchParams) (SearchResponse, error)
}

// providerMeta is the label of every provider id. Unsupported ids have no entry.
var providerMeta = map[string]string{
	"duckduckgo": "DuckDuckGo",
	"startpage":  "Startpage",
	"parallel":   "Parallel",
	"exa":        "Exa",
	"firecrawl":  "Firecrawl",
}

// supportedProviderOrder lists the ids a chain may name, in the default order.
var supportedProviderOrder = []string{"duckduckgo", "startpage", "parallel", "exa", "firecrawl"}

// DefaultChain is the credential-free default: the scrapers come before the
// shared MCP endpoints so a fresh install does not open with a rate-limited
// endpoint.
func DefaultChain() []string {
	return append([]string(nil), supportedProviderOrder...)
}

// newProvider builds the provider id, pointed at endpoint when non-empty.
func newProvider(id, endpoint string) (SearchProvider, error) {
	switch id {
	case "duckduckgo":
		return newDuckDuckGo(endpoint), nil
	case "startpage":
		return newStartpage(endpoint), nil
	case "parallel":
		return newParallel(endpoint), nil
	case "exa":
		return newExa(endpoint), nil
	case "firecrawl":
		return newFirecrawl(endpoint), nil
	}
	return nil, fmt.Errorf("unsupported web search provider %q (supported: %s)", id, strings.Join(supportedProviderOrder, ", "))
}

// clampNumResults bounds a result count to [1, max], returning fallback when unset.
func clampNumResults(value, fallback, max int) int {
	if value <= 0 {
		value = fallback
	}
	if value > max {
		value = max
	}
	return value
}

// recencyDays maps a recency window to its length in days.
var recencyDays = map[string]int{"day": 1, "week": 7, "month": 30, "year": 365}

// validRecency reports whether recency is a recognised window.
func validRecency(recency string) bool {
	_, ok := recencyDays[recency]
	return ok
}

// publishedTimeLayouts are the shapes search engines emit for a publish date.
// The fractional-second layout is deliberately last among the ISO forms: Go
// accepts an absent fraction for `.999999999`, which covers DDG's 7-digit form.
var publishedTimeLayouts = []string{
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parsePublishedAt parses an engine-supplied publish timestamp.
func parsePublishedAt(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	for _, layout := range publishedTimeLayouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// dateToAgeSeconds converts a publish date to an age in seconds, or nil when it
// cannot be parsed.
func dateToAgeSeconds(value string, now time.Time) *int {
	parsed, ok := parsePublishedAt(value)
	if !ok {
		return nil
	}
	seconds := int(now.Sub(parsed).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

// normalizeSearchText collapses whitespace, returning "" for blank input.
func normalizeSearchText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// normalizeHTTPURL keeps only absolute http(s) URLs.
func normalizeHTTPURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

// truncateRunes caps a string at n runes, cutting on a rune boundary.
func truncateRunes(value string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range value {
		if count == n {
			return value[:i]
		}
		count++
	}
	return value
}
