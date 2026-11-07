package webtools

// Exa keyless search over its public MCP endpoint.
//
// Ports packages/coding-agent/src/web/search/providers/exa.ts. The public MCP at
// mcp.exa.ai/mcp exposes `web_search_exa` with arguments
// `{query, objective, numResults}`; it answers with a text block per result in
// the shape:
//
//	Title: <title>
//	URL: <url>
//	Published: <iso>
//	Author: <name|N/A>
//	Highlights:
//	<text>
//
// with results separated by a `---` line. Verified against the live tools/list and
// one tools/call at the commit this ports; both shapes (JSON result and SSE
// transport) are handled by the shared MCP reader.

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	exaMCPURL        = "https://mcp.exa.ai/mcp?tools=web_search_exa"
	exaTool          = "web_search_exa"
	exaResults       = 20
	exaDefaultN      = 10
	exaMaxSnippetLen = 500
)

// exaSyntax is what Exa's neural search parses: it prefers natural language, so
// only quoted phrases are re-emitted.
var exaSyntax = QuerySyntax{Phrases: true}

// exaProvider calls the keyless Exa MCP.
type exaProvider struct {
	endpoint string
}

func newExa(endpoint string) SearchProvider {
	if endpoint == "" {
		endpoint = exaMCPURL
	}
	return &exaProvider{endpoint: endpoint}
}

func (p *exaProvider) ID() string                  { return "exa" }
func (p *exaProvider) Label() string               { return providerMeta["exa"] }
func (p *exaProvider) IsAvailable() bool           { return true }
func (p *exaProvider) IsExplicitlyAvailable() bool { return true }
func (p *exaProvider) Syntax() QuerySyntax         { return exaSyntax }

// FiltersRecency reports that the engine does not enforce the window: the keyless
// MCP treats `after:` as a ranking hint, so the pipeline applies the window.
func (p *exaProvider) FiltersRecency() bool { return false }

func (p *exaProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	want := clampNumResults(params.NumResultsOrDefault(), exaDefaultN, exaResults)

	// Domain and date directives move into the query text: the keyless MCP tool
	// takes one query string, and Exa reads the Google-style operators it accepts.
	// recency is not one of them: the keyless MCP treats `after:` as a ranking
	// hint rather than a filter, so the window is applied to the returned sources
	// by the chain instead of being approximated in the query.
	query := params.Query
	if parsed.HasDirectives {
		syntax := exaSyntax
		syntax.Site = true
		syntax.Negation = true
		syntax.DateRange = true
		query = FormatQuery(parsed, syntax)
	}

	args := map[string]any{
		"query":      query,
		"objective":  query,
		"numResults": want,
	}

	client := NewHTTPClient()
	defer client.CloseIdleConnections()
	result, err := callMCPToolWithHeaders(ctx, client, p.endpoint, p.ID(), exaTool, args, exaHeaders())
	if err != nil {
		return SearchResponse{}, explainMCPFailure(err, p.ID())
	}

	sources := make([]SearchSource, 0, want)
	if payload, ok := mcpJSONPayload(result); ok {
		if parsedSources, ok := parseExaJSONResults(payload); ok {
			sources = parsedSources
		}
	}
	if len(sources) == 0 {
		sources = parseExaTextResults(mcpToolText(result))
	}
	if len(sources) > want {
		sources = sources[:want]
	}
	return SearchResponse{Provider: p.ID(), Sources: sources, AuthMode: "keyless"}, nil
}

// exaHeaders sends the provenance header the public MCP expects.
func exaHeaders() map[string][]string {
	return map[string][]string{"X-Exa-Source": {"ompig-slim"}}
}

// parseExaJSONResults reads an Exa REST-shaped payload:
// {"results": [{title,url,author,publishedDate,text,summary,highlights}]}.
func parseExaJSONResults(payload json.RawMessage) ([]SearchSource, bool) {
	var envelope struct {
		RequestID string `json:"requestId"`
		Results   []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Author        string   `json:"author"`
			PublishedDate string   `json:"publishedDate"`
			Text          string   `json:"text"`
			Summary       string   `json:"summary"`
			Highlights    []string `json:"highlights"`
		} `json:"results"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Results == nil {
		return nil, false
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
		snippet := item.Summary
		if strings.TrimSpace(snippet) == "" {
			snippet = item.Text
		}
		if strings.TrimSpace(snippet) == "" {
			snippet = strings.Join(item.Highlights, " ")
		}
		sources = append(sources, SearchSource{
			Title:         title,
			URL:           target,
			Snippet:       truncateRunes(snippet, exaMaxSnippetLen),
			PublishedDate: normalizeSearchText(item.PublishedDate),
			AgeSeconds:    dateToAgeSeconds(item.PublishedDate, now),
			Author:        normalizeSearchText(item.Author),
		})
	}
	return sources, true
}

// parseExaTextResults splits the MCP text payload into one source per
// "Title:" section.
func parseExaTextResults(text string) []SearchSource {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	sections := splitExaSections(normalized)
	now := time.Now()
	sources := make([]SearchSource, 0, len(sections))
	for _, section := range sections {
		title := exaSectionField(section, "Title")
		target := normalizeHTTPURL(exaSectionField(section, "URL"))
		if target == "" {
			continue
		}
		if title == "" {
			title = target
		}
		result := SearchSource{
			Title:         title,
			URL:           target,
			PublishedDate: exaFieldValue(section, "Published"),
			Author:        exaFieldValue(section, "Author"),
			Snippet:       truncateRunes(exaSectionText(section), exaMaxSnippetLen),
		}
		result.AgeSeconds = dateToAgeSeconds(result.PublishedDate, now)
		sources = append(sources, result)
	}
	return sources
}

// splitExaSections cuts the payload at each new "Title:" block: a blank line
// followed by a line starting with "Title:".
func splitExaSections(text string) []string {
	lines := strings.Split(text, "\n")
	sections := make([][]string, 0, 8)
	var current []string
	for i, line := range lines {
		startsSection := strings.HasPrefix(strings.TrimSpace(line), "Title:") &&
			(i == 0 || strings.TrimSpace(lines[i-1]) == "")
		if startsSection && current != nil {
			sections = append(sections, current)
			current = nil
		}
		current = append(current, line)
	}
	if current != nil {
		sections = append(sections, current)
	}
	out := make([]string, 0, len(sections))
	for _, lines := range sections {
		section := strings.TrimSpace(strings.Join(lines, "\n"))
		if strings.HasPrefix(section, "Title:") {
			out = append(out, section)
		}
	}
	return out
}

// exaSectionField reads a `Label: value` line from a section header. The value
// stops at the first line that does not continue it, so a highlight body never
// looks like a field.
func exaSectionField(section, label string) string {
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		value, ok := strings.CutPrefix(trimmed, label+":")
		if !ok {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}

// exaFieldValue reads a header field, treating Exa's "N/A" placeholder as absent.
func exaFieldValue(section, label string) string {
	value := normalizeSearchText(exaSectionField(section, label))
	if value == "N/A" || value == "n/a" {
		return ""
	}
	return value
}

// exaSectionText is the highlight body: everything after the "Highlights:" line,
// with the "..." elision markers removed.
func exaSectionText(section string) string {
	lines := strings.Split(section, "\n")
	start := -1
	for i, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "Highlights:") {
			start = i + 1
			break
		}
	}
	if start < 0 || start >= len(lines) {
		return ""
	}
	parts := make([]string, 0, len(lines)-start)
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "..." || trimmed == "---" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

var _ SearchProvider = (*exaProvider)(nil)
