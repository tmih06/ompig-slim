package webtools

// Search chain: candidate resolution and sequential fallback.
//
// Ports packages/coding-agent/src/web/search/index.ts (executeSearch). The chain
// is sequential and first-usable-wins: fanning out would spend quota on every
// engine to answer one question.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultSearchTimeout bounds one provider transport.
	DefaultSearchTimeout = 60 * time.Second
	// MaxSearchTimeout caps the configurable per-provider timeout.
	MaxSearchTimeout = 300 * time.Second
	// DefaultSearchResults is the result count when the caller sets none.
	DefaultSearchResults = 10
	// MaxSearchResults caps the per-engine result count.
	MaxSearchResults = 20
	// MaxSnippetRunes is the model-visible snippet and cited-text cap.
	MaxSnippetRunes = 240
	// MaxSearchQueryRunes is the cap for one listed search query.
	MaxSearchQueryRunes = 120
	// MaxListedSearchQueries is how many provider search queries the text lists.
	MaxListedSearchQueries = 3
	// MaxFailureReasonRunes caps a quoted failure reason.
	MaxFailureReasonRunes = 240
)

// SearchFailureDetail is one provider that failed before another answered, or
// before the search gave up.
type SearchFailureDetail struct {
	// Provider is the chain id (duckduckgo, …), or "none" when none ran.
	Provider string `json:"provider"`
	Label    string `json:"label"`
	// Reason is the flattened, capped reason a fallback note quotes.
	Reason string `json:"reason"`
	// Status is an HTTP-style status; 204 marks a source-less answer.
	Status int `json:"status,omitempty"`
}

// SearchDetails is the unified structured result of one web_search call.
type SearchDetails struct {
	// Query is the raw query the model sent.
	Query string `json:"query"`
	// Parsed is the structured view of Query.
	Parsed *StructuredQuery `json:"parsedQuery,omitempty"`
	// Chain is the resolved candidate order this call walked.
	Chain []string `json:"chain"`
	// Response is the winning response, or an empty one on total failure.
	Response SearchResponse `json:"response"`
	// Error is the failure message when every candidate failed.
	Error string `json:"error,omitempty"`
	// Failures are the providers that failed, in chain order.
	Failures []SearchFailureDetail `json:"failures,omitempty"`
	// Notes are the fallback and relaxed-constraint notes the text leads with.
	Notes []string `json:"notes,omitempty"`
}

// SearchResult is a finished call: the model-visible text plus its details.
type SearchResult struct {
	Text    string
	Details SearchDetails
	IsError bool
}

// SearchOptions are the inputs of one RunSearch call.
type SearchOptions struct {
	Query      string
	Limit      int
	Recency    string
	NumResults int
	// Timeout bounds one provider transport; zero means DefaultSearchTimeout.
	Timeout time.Duration
	// Chain is the resolved provider order; empty means DefaultChain().
	Chain []string
	// Endpoints overrides a provider's transport URL by chain id.
	Endpoints map[string]string
}

// ResolveChain applies the configured chain, wholly replacing the default. A
// nil/empty configuration selects DefaultChain. Every entry of a configured
// chain must name a supported provider: a typo is a configuration error, not a
// silently skipped candidate.
func ResolveChain(configured []string) ([]string, error) {
	if len(configured) == 0 {
		return DefaultChain(), nil
	}
	chain := make([]string, 0, len(configured))
	for _, entry := range configured {
		id := strings.ToLower(strings.TrimSpace(entry))
		if id == "" {
			return nil, errors.New("webSearch.chain contains an empty provider name")
		}
		if _, ok := providerMeta[id]; !ok {
			return nil, fmt.Errorf("unsupported web search provider %q in webSearch.chain (supported: %s)",
				entry, strings.Join(supportedProviderOrder, ", "))
		}
		chain = append(chain, id)
	}
	return chain, nil
}

// ClampTimeout bounds a configured per-provider timeout to [1s, MaxSearchTimeout],
// returning the default for a missing or non-positive value.
func ClampTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultSearchTimeout
	}
	if d < time.Second {
		return time.Second
	}
	if d > MaxSearchTimeout {
		return MaxSearchTimeout
	}
	return d
}

// ClampLimit bounds a requested result count to [1, MaxSearchResults].
func ClampLimit(limit int) int { return clampNumResults(limit, DefaultSearchResults, MaxSearchResults) }

// RunSearch walks the chain and returns the first usable response. A returned
// error is a configuration or cancellation problem; every runtime failure is a
// SearchResult with IsError set, so the transcript keeps the detail.
func RunSearch(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	chain, err := ResolveChain(opts.Chain)
	if err != nil {
		return SearchResult{}, err
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return SearchResult{}, errors.New("query must not be empty")
	}
	limit := ClampLimit(opts.Limit)
	recency := strings.ToLower(strings.TrimSpace(opts.Recency))
	if recency != "" && !validRecency(recency) {
		return SearchResult{}, fmt.Errorf("unsupported recency %q (use day, week, month or year)", opts.Recency)
	}
	parsed := ParseSearchQuery(query)

	params := SearchParams{
		Query:      query,
		Parsed:     parsed,
		Limit:      limit,
		Recency:    recency,
		Timeout:    ClampTimeout(opts.Timeout),
		NumResults: limit,
		Endpoints:  opts.Endpoints,
	}
	if opts.NumResults > 0 {
		params.NumResults = clampNumResults(opts.NumResults, limit, MaxSearchResults)
	}

	providers := make([]SearchProvider, 0, len(chain))
	for _, id := range chain {
		provider, buildErr := newProvider(id, opts.Endpoints[id])
		if buildErr != nil {
			return SearchResult{}, buildErr
		}
		providers = append(providers, provider)
	}
	return runChain(ctx, providers, chain, params)
}

// runChain walks the already-built candidates. It is the testable core of
// RunSearch: the fallback, failure and cancellation rules live here, so they can
// be exercised with recorded providers instead of network fixtures.
func runChain(ctx context.Context, providers []SearchProvider, chain []string, params SearchParams) (SearchResult, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	details := SearchDetails{Query: params.Query, Parsed: parsed, Chain: chain}
	if len(providers) == 0 {
		// ResolveChain never returns an empty chain, so this is a programming
		// error rather than a user configuration one.
		message := "No web search provider is configured."
		details.Error = message
		details.Response = SearchResponse{Provider: "none", Sources: []SearchSource{}}
		return SearchResult{Text: "Error: " + message, Details: details, IsError: true}, nil
	}
	failures := make([]SearchFailureDetail, 0, len(providers))

	for _, provider := range providers {
		if err := ctx.Err(); err != nil {
			return SearchResult{}, err
		}
		// Every chain entry is explicit (a configured chain replaces the default
		// wholesale), so a provider that reports itself usable only on explicit
		// selection still runs; one that refuses both is recorded, not skipped.
		if !provider.IsAvailable() && !provider.IsExplicitlyAvailable() {
			failures = append(failures, SearchFailureDetail{
				Provider: provider.ID(),
				Label:    provider.Label(),
				Reason:   "unavailable",
			})
			continue
		}

		response, searchErr := searchOnce(ctx, provider, params)
		if searchErr != nil {
			// Cancellation is never a provider failure: report it as such
			// instead of letting the loop fall through to "all providers failed".
			if ctxErr := ctx.Err(); ctxErr != nil {
				return SearchResult{}, ctxErr
			}
			if errors.Is(searchErr, context.Canceled) {
				return SearchResult{}, searchErr
			}
			if errors.Is(searchErr, context.DeadlineExceeded) {
				failures = append(failures, SearchFailureDetail{
					Provider: provider.ID(),
					Label:    provider.Label(),
					Reason:   "timed out after " + params.Timeout.String(),
				})
				continue
			}
			failures = append(failures, failureOf(provider, searchErr))
			continue
		}

		notes := make([]string, 0, 4)
		final := response
		if parsed.HasConstraints && len(response.Sources) > 0 {
			filtered := ApplyQueryConstraints(response.Sources, parsed)
			if len(filtered.Sources) != len(response.Sources) {
				final.Sources = filtered.Sources
			}
			for _, label := range filtered.Dropped {
				notes = append(notes, fmt.Sprintf("no results matched `%s`; the constraint was relaxed", label))
			}
		}
		if !hasRenderableSearchContent(final) {
			failures = append(failures, failureOf(provider, &SearchError{
				Provider: provider.ID(),
				Message:  provider.Label() + " returned no sources; an answer without citations is not a search result",
				Status:   204,
			}))
			continue
		}

		// A provider whose engine does not enforce the relative window natively
		// has it applied here, leniently: a window that would empty the result
		// set is relaxed rather than returning nothing, and dates the pipeline
		// cannot check are reported instead of being claimed as restricted.
		if params.Recency != "" && !filtersRecencyNatively(provider) {
			windowed, undated, relaxedWindow := applyRecencyWindow(final.Sources, params.Recency, time.Now())
			final.Sources = windowed
			if relaxedWindow {
				notes = append(notes, fmt.Sprintf("no results fell within the last %s; the recency filter was relaxed", params.Recency))
			}
			if undated > 0 && !relaxedWindow {
				notes = append(notes, fmt.Sprintf("%s reports no publish date for %d of %d results, so the last %s window could not be verified for them",
					provider.Label(), undated, len(windowed), params.Recency))
			}
		}
		notes = append(notes, missingConstraintNotes(provider, parsed)...)
		if fallback := fallbackNotes(failures, provider.Label()); fallback != "" {
			notes = append([]string{fallback}, notes...)
		}
		details.Response = final
		details.Failures = nonEmptyFailures(failures)
		details.Notes = notes
		return SearchResult{Text: FormatSearchText(final, notes), Details: details}, nil
	}

	details.Failures = failures
	var message string
	switch len(failures) {
	case 0:
		message = "Unknown web search failure"
	case 1:
		message = failures[0].Label + ": " + failures[0].Reason
	default:
		parts := make([]string, len(failures))
		for i, failure := range failures {
			parts[i] = failure.Provider + ": " + failure.Reason
		}
		message = "All web search providers failed: " + strings.Join(parts, "; ")
	}
	details.Error = message
	details.Response = SearchResponse{Provider: failedProvider(failures), Sources: []SearchSource{}}
	return SearchResult{Text: "Error: " + message, Details: details, IsError: true}, nil
}

// searchOnce runs one provider under its own timeout so a stalled transport
// cannot consume the next candidate's budget.
func searchOnce(ctx context.Context, provider SearchProvider, params SearchParams) (SearchResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, params.Timeout)
	defer cancel()
	return provider.Search(attemptCtx, params)
}

// SearchError is a provider runtime failure with an optional HTTP-style status.
type SearchError struct {
	Provider string
	Message  string
	Status   int
}

func (e *SearchError) Error() string { return e.Message }

// failureOf normalizes a provider error into a failure detail.
func failureOf(provider SearchProvider, err error) SearchFailureDetail {
	reason := normalizeSearchText(err.Error())
	reason = strings.TrimSuffix(reason, ".")
	detail := SearchFailureDetail{
		Provider: provider.ID(),
		Label:    provider.Label(),
		Reason:   truncateRunes(reason, MaxFailureReasonRunes),
	}
	var searchErr *SearchError
	if errors.As(err, &searchErr) {
		detail.Status = searchErr.Status
	}
	return detail
}

func nonEmptyFailures(failures []SearchFailureDetail) []SearchFailureDetail {
	if len(failures) == 0 {
		return nil
	}
	out := make([]SearchFailureDetail, len(failures))
	copy(out, failures)
	return out
}

func failedProvider(failures []SearchFailureDetail) string {
	if len(failures) == 0 {
		return "none"
	}
	return failures[len(failures)-1].Provider
}

// fallbackNotes names the candidates that failed before served answered.
func fallbackNotes(failed []SearchFailureDetail, servedLabel string) string {
	if len(failed) == 0 {
		return ""
	}
	parts := make([]string, len(failed))
	for i, failure := range failed {
		parts[i] = fmt.Sprintf("%s failed (%s)", failure.Label, failure.Reason)
	}
	return fmt.Sprintf("answered by %s after %s", servedLabel, strings.Join(parts, " and "))
}

// recencyFiltering is the optional capability a provider implements when its
// engine enforces the relative recency window itself. A provider without it, or
// answering false, has the window applied to its sources by the pipeline.
type recencyFiltering interface {
	FiltersRecency() bool
}

// filtersRecencyNatively reports whether the provider's engine applies the
// window itself.
func filtersRecencyNatively(provider SearchProvider) bool {
	filters, ok := provider.(recencyFiltering)
	return ok && filters.FiltersRecency()
}

// applyRecencyWindow keeps the sources published inside the window. It reports
// how many kept sources carry no resolvable date (the window cannot be verified
// for those) and whether the window had to be relaxed to keep the result set
// non-empty.
func applyRecencyWindow(sources []SearchSource, recency string, now time.Time) (kept []SearchSource, undated int, relaxed bool) {
	if len(sources) == 0 {
		return sources, 0, false
	}
	days, ok := recencyDays[recency]
	if !ok {
		return sources, 0, false
	}
	cutoff := now.AddDate(0, 0, -days).UnixMilli()
	kept = make([]SearchSource, 0, len(sources))
	for _, source := range sources {
		millis, dated := sourceTime(source, now)
		if !dated {
			undated++
			kept = append(kept, source)
			continue
		}
		if millis >= cutoff {
			kept = append(kept, source)
		}
	}
	if len(kept) == 0 {
		return sources, undated, true
	}
	return kept, undated, false
}

// missingConstraintNotes names the parts of a request the serving provider
// could not express and the pipeline could not enforce. Filetype, site, URL,
// title and date filters are post-filtered, so a missing operator there narrows
// results anyway; the page-body operators are the ones with no enforcement path,
// because a snippet is only a fragment of the page.
func missingConstraintNotes(provider SearchProvider, parsed *StructuredQuery) []string {
	notes := make([]string, 0, 2)
	if len(parsed.InText) > 0 || len(parsed.ExcludedInText) > 0 {
		if syntax := provider.Syntax(); !syntax.InText {
			values := append(append([]string(nil), parsed.InText...), parsed.ExcludedInText...)
			notes = append(notes, fmt.Sprintf("%s cannot search page bodies, so `intext:%s` was not applied",
				provider.Label(), strings.Join(values, "` and `intext:")))
		}
	}
	// A lang: locale is not post-filterable: a source's language is not in the
	// result shape, so an engine that ignores it leaves the request unmet.
	if parsed.Lang != "" && !filtersLanguageNatively(provider) {
		notes = append(notes, fmt.Sprintf("%s does not filter by language, so `lang:%s` was not applied",
			provider.Label(), parsed.Lang))
	}
	return notes
}

// languageFiltering is the optional capability a provider implements when its
// engine applies a lang: locale to the request.
type languageFiltering interface {
	FiltersLanguage() bool
}

// filtersLanguageNatively reports whether the provider's engine applies the
// lang: locale itself.
func filtersLanguageNatively(provider SearchProvider) bool {
	filters, ok := provider.(languageFiltering)
	return ok && filters.FiltersLanguage()
}

// hasRenderableSearchContent reports a response a search call may return. Every
// provider in this package is a pure search engine, so prose without sources is a
// failure, status 204: a host can silently drop the search tool and answer from
// weights, and that must not be presented as a search result.
func hasRenderableSearchContent(response SearchResponse) bool {
	return len(response.Sources) > 0 || len(response.Citations) > 0
}

// FormatSearchText renders a response for the model: notes first (fallback and
// relaxed constraints), then the answer, then numbered cited sources.
func FormatSearchText(response SearchResponse, notes []string) string {
	parts := make([]string, 0, 8)
	for _, note := range notes {
		if strings.TrimSpace(note) == "" {
			continue
		}
		parts = append(parts, "Note: "+note)
	}
	if answer := strings.TrimSpace(response.Answer); answer != "" {
		parts = append(parts, answer)
		if len(response.Sources) > 0 {
			parts = append(parts, "", "## Sources", countLabel(len(response.Sources), "source"))
		}
	}
	for i, src := range response.Sources {
		title := strings.TrimSpace(src.Title)
		if title == "" {
			title = src.URL
		}
		age := ""
		if src.AgeSeconds != nil {
			age = " (" + formatAge(*src.AgeSeconds) + ")"
		} else if src.PublishedDate != "" {
			age = " (" + src.PublishedDate + ")"
		}
		parts = append(parts, fmt.Sprintf("[%d] %s%s", i+1, title, age), "    "+src.URL)
		if snippet := truncateRunes(normalizeSearchText(src.Snippet), MaxSnippetRunes); snippet != "" {
			parts = append(parts, "    "+snippet)
		}
	}
	if len(response.Citations) > 0 {
		parts = append(parts, "", "## Citations", countLabel(len(response.Citations), "citation"))
		for i, citation := range response.Citations {
			title := strings.TrimSpace(citation.Title)
			if title == "" {
				title = citation.URL
			}
			parts = append(parts, fmt.Sprintf("[%d] %s", i+1, title), "    "+citation.URL)
			if text := truncateRunes(normalizeSearchText(citation.CitedText), MaxSnippetRunes); text != "" {
				parts = append(parts, "    "+text)
			}
		}
	}
	if len(response.RelatedQuestions) > 0 {
		parts = append(parts, "", "## Related", countLabel(len(response.RelatedQuestions), "question"))
		for _, question := range response.RelatedQuestions {
			parts = append(parts, "- "+question)
		}
	}
	if len(response.SearchQueries) > 0 {
		parts = append(parts, fmt.Sprintf("Search queries: %d", len(response.SearchQueries)))
		listed := response.SearchQueries
		if len(listed) > MaxListedSearchQueries {
			listed = listed[:MaxListedSearchQueries]
		}
		for _, q := range listed {
			parts = append(parts, "- "+truncateRunes(normalizeSearchText(q), MaxSearchQueryRunes))
		}
	}
	return strings.Join(parts, "\n")
}

func countLabel(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(count) + " " + unit + "s"
}

// formatAge renders an age in seconds the way the tool card does (3d, 2h, 12m).
func formatAge(seconds int) string {
	switch {
	case seconds < 60:
		return "just now"
	case seconds < 3600:
		return strconv.Itoa(seconds/60) + "m ago"
	case seconds < 86400:
		return strconv.Itoa(seconds/3600) + "h ago"
	case seconds < 86400*30:
		return strconv.Itoa(seconds/86400) + "d ago"
	case seconds < 86400*365:
		return strconv.Itoa(seconds/(86400*30)) + "mo ago"
	default:
		return strconv.Itoa(seconds/(86400*365)) + "y ago"
	}
}
