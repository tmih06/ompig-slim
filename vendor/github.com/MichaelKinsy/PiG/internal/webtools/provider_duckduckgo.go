package webtools

// DuckDuckGo keyless search over the no-JS HTML frontend.
//
// Ports packages/coding-agent/src/web/search/providers/duckduckgo.ts. POST to
// html.duckduckgo.com/html/ returns a static results page; a bot wall is served
// with a 200-or-202 status and an `anomaly-modal` marker, so the body is the
// signal, not the status.

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	duckDuckGoHTMLURL  = "https://html.duckduckgo.com/html/"
	duckDuckGoResults  = 20
	duckDuckGoDefaultN = 10
)

// recencyToDDGDF maps a recency window to DDG's `df` form value.
var recencyToDDGDF = map[string]string{"day": "d", "week": "w", "month": "m", "year": "y"}

// ddgQuerySyntax is what the DDG HTML frontend parses. Date bounds are off on
// purpose: DDG ignores before:/after:, so they are stripped from the query and
// enforced by the pipeline's lenient post-filter.
var ddgQuerySyntax = QuerySyntax{
	Phrases: true, Negation: true, Or: true,
	Site: true, InURL: true, InTitle: true, InText: true, Filetype: true,
}

// ddgKLByLocale is DDG's documented `kl` value for a two-letter locale pair.
// DDG's codes are region-language with provider-specific values, so only the
// mapped ones are used; everything else keeps the default region.
var ddgKLByLocale = map[string]string{
	"ar-es": "xa-ar", "en-gb": "uk-en", "ja-jp": "jp-jp", "ko-kr": "kr-kr",
	"zh-hk": "hk-tzh", "zh-tw": "tw-tzh", "es-us": "ue-es", "es-mx": "mx-es",
	"pt-br": "br-pt", "pt-pt": "pt-pt", "fr-fr": "fr-fr", "de-de": "de-de",
	"it-it": "it-it", "nl-nl": "nl-nl", "pl-pl": "pl-pl", "ru-ru": "ru-ru",
	"sv-se": "se-sv", "da-dk": "dk-da", "fi-fi": "fi-fi", "no-no": "no-no",
	"cs-cz": "cz-cs", "hu-hu": "hu-hu", "tr-tr": "tr-tr", "el-gr": "gr-el",
	"he-il": "il-he", "id-id": "id-id", "th-th": "th-th", "vi-vn": "vn-vi",
	"uk-ua": "ua-uk", "ro-ro": "ro-ro", "en-us": "us-en", "en-au": "au-en",
	"en-ca": "ca-en", "en-in": "in-en", "en-nz": "nz-en", "en-za": "za-en",
	"en-sg": "sg-en", "es-es": "es-es", "es-ar": "ar-es", "es-cl": "cl-es",
	"es-co": "co-es", "es-pe": "pe-es", "es-ve": "ve-es", "de-at": "at-de",
	"de-ch": "ch-de", "fr-be": "be-fr", "fr-ch": "ch-fr", "nl-be": "be-nl",
	"zh-cn": "cn-zh",
}

// localeToKL maps a lang: locale onto a documented DDG `kl` value.
func localeToKL(lang string) string {
	if lang == "" {
		return ""
	}
	locale := strings.ToLower(strings.ReplaceAll(lang, "_", "-"))
	if kl, ok := ddgKLByLocale[locale]; ok {
		return kl
	}
	parts := strings.Split(locale, "-")
	if len(parts) == 2 && len(parts[0]) == 2 && len(parts[1]) == 2 {
		if kl, ok := ddgKLByLocale[parts[1]+"-"+parts[0]]; ok {
			return kl
		}
	}
	return ""
}

// duckDuckGoProvider scrapes html.duckduckgo.com.
type duckDuckGoProvider struct {
	endpoint string
}

func newDuckDuckGo(endpoint string) SearchProvider {
	if endpoint == "" {
		endpoint = duckDuckGoHTMLURL
	}
	return &duckDuckGoProvider{endpoint: endpoint}
}

func (p *duckDuckGoProvider) ID() string                  { return "duckduckgo" }
func (p *duckDuckGoProvider) Label() string               { return providerMeta["duckduckgo"] }
func (p *duckDuckGoProvider) IsAvailable() bool           { return true }
func (p *duckDuckGoProvider) IsExplicitlyAvailable() bool { return true }
func (p *duckDuckGoProvider) Syntax() QuerySyntax         { return ddgQuerySyntax }

func (p *duckDuckGoProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	want := clampNumResults(params.NumResultsOrDefault(), duckDuckGoDefaultN, duckDuckGoResults)

	form := url.Values{}
	form.Set("q", formatScraperQuery(params.Query, parsed, ddgQuerySyntax))
	form.Set("b", "")
	if kl := localeToKL(parsed.Lang); kl != "" {
		form.Set("kl", kl)
	} else {
		form.Set("kl", "us-en")
	}
	if df, ok := recencyToDDGDF[params.Recency]; ok {
		form.Set("df", df)
	}

	client := NewHTTPClient()
	defer client.CloseIdleConnections()
	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Referer", "https://html.duckduckgo.com/")

	sources := make([]SearchSource, 0, want)
	seen := make(map[string]struct{}, want)
	next := form
	for len(sources) < want {
		response, err := Load(ctx, client, http.MethodPost, p.endpoint, []byte(next.Encode()), headers, DefaultMaxBytes)
		if err != nil {
			if ctx.Err() != nil {
				return SearchResponse{}, ctx.Err()
			}
			return SearchResponse{}, &SearchError{
				Provider: p.ID(),
				Message:  "DuckDuckGo " + classifyProviderMessage(p.ID(), err),
				Status:   response.StatusCode,
			}
		}
		page := string(response.Body)
		if isDDGAnomaly(page) {
			return SearchResponse{}, &SearchError{
				Provider: p.ID(),
				Message: "DuckDuckGo blocked the request with a bot-detection challenge; DuckDuckGo throttles " +
					"automated HTML searches from shared egress IPs",
				Status: http.StatusTooManyRequests,
			}
		}

		before := len(sources)
		for _, result := range parseDDGResults(page) {
			if _, ok := seen[result.URL]; ok {
				continue
			}
			seen[result.URL] = struct{}{}
			sources = append(sources, result)
			if len(sources) >= want {
				break
			}
		}
		if len(sources) == before {
			break
		}
		continuation := parseDDGContinuation(page)
		if continuation == nil {
			break
		}
		next = continuation
	}

	return SearchResponse{Provider: p.ID(), Sources: sources, AuthMode: "keyless"}, nil
}

// isDDGAnomaly reports DDG's bot-challenge modal. It mixes 200 and 202 statuses
// on those pages, so the marker is the reliable signal.
func isDDGAnomaly(page string) bool {
	return strings.Contains(page, "anomaly-modal") || strings.Contains(page, "anomaly.js")
}

// parseDDGResults lifts the result rows in document order. Each result lives in
// a div carrying the `result` class, with `result__a` for the title anchor and
// an optional `result__snippet` for the preview.
func parseDDGResults(page string) []SearchSource {
	root, err := searchDOMTree([]byte(page))
	if err != nil {
		return nil
	}
	results := make([]SearchSource, 0, 10)
	searchDOMWalk(root, func(node *html.Node) bool {
		if node.Type != html.ElementNode || !searchDOMHasClass(node, "result") {
			return true
		}
		anchor := searchDOMFirstClass(node, "result__a")
		if anchor == nil {
			return false
		}
		target := unwrapDDGURL(searchDOMAttr(anchor, "href"))
		if target == "" {
			return false
		}
		title := searchDOMText(anchor)
		if title == "" {
			return false
		}
		result := SearchSource{Title: title, URL: target}
		if snippet := searchDOMFirstClass(node, "result__snippet"); snippet != nil {
			result.Snippet = searchDOMText(snippet)
		}
		result.PublishedDate = extractDDGPublishedDate(node)
		result.AgeSeconds = dateToAgeSeconds(result.PublishedDate, time.Now())
		results = append(results, result)
		return false
	})
	return results
}

// extractDDGPublishedDate lifts the publishing timestamp DDG renders inside
// `result__extras__url`. The scan is restricted to that container so a
// date-shaped value in a snippet is not attributed as publication metadata.
func extractDDGPublishedDate(block *html.Node) string {
	extras := searchDOMFirstClass(block, "result__extras__url")
	if extras == nil {
		return ""
	}
	found := ""
	searchDOMWalk(extras, func(node *html.Node) bool {
		if found != "" {
			return false
		}
		if node.Type != html.ElementNode || node.Data != "span" {
			return true
		}
		text := searchDOMText(node)
		if isPublishedDateText(text) {
			found = text
			return false
		}
		return true
	})
	return found
}

// unwrapDDGURL resolves a DDG href to the target URL: redirect wrappers, then
// protocol-relative links, then absolute result URLs. Ad click redirects are
// excluded.
func unwrapDDGURL(href string) string {
	if href == "" {
		return ""
	}
	decoded := strings.ReplaceAll(href, "&amp;", "&")
	if isDDGAdRedirect(decoded) {
		return ""
	}
	if parsed, err := url.Parse(decoded); err == nil {
		if uddg := parsed.Query().Get("uddg"); uddg != "" {
			return normalizeHTTPURL(uddg)
		}
	}
	if strings.HasPrefix(decoded, "//") {
		return normalizeHTTPURL("https:" + decoded)
	}
	return normalizeHTTPURL(decoded)
}

// isDDGAdRedirect reports the y.js click-through used by sponsored placements.
func isDDGAdRedirect(href string) bool {
	lower := strings.ToLower(href)
	trimmed := strings.TrimPrefix(strings.TrimPrefix(lower, "https:"), "http:")
	trimmed = strings.TrimPrefix(trimmed, "//")
	trimmed = strings.TrimPrefix(trimmed, "www.")
	return strings.HasPrefix(trimmed, "duckduckgo.com/y.js")
}

// parseDDGContinuation extracts the hidden fields of DDG's next-page form. Each
// input tag is parsed independently because attribute order varies.
func parseDDGContinuation(page string) url.Values {
	root, err := searchDOMTree([]byte(page))
	if err != nil {
		return nil
	}
	var found url.Values
	searchDOMWalk(root, func(node *html.Node) bool {
		if found != nil {
			return false
		}
		if node.Type != html.ElementNode || node.Data != "form" {
			return true
		}
		form := url.Values{}
		searchDOMWalk(node, func(input *html.Node) bool {
			if input.Type != html.ElementNode || input.Data != "input" {
				return true
			}
			name := searchDOMAttr(input, "name")
			if name == "" {
				return true
			}
			form.Set(name, searchDOMAttr(input, "value"))
			return true
		})
		if form.Get("s") != "" && form.Get("vqd") != "" {
			found = form
			return false
		}
		return true
	})
	return found
}

// isPublishedDateText reports a YYYY-MM-DD or date-time fragment.
func isPublishedDateText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 10 {
		return false
	}
	if _, ok := parseDateValue(trimmed[:10]); !ok {
		return false
	}
	if len(trimmed) == 10 {
		return true
	}
	rest := trimmed[10]
	return rest == 'T' || rest == ' '
}

// duckDuckGoProvider implements SearchProvider.
// FiltersRecency reports that the engine applies the recency window natively.
func (p *duckDuckGoProvider) FiltersRecency() bool { return true }

// FiltersLanguage reports that the engine applies a lang: locale natively, as
// its documented `kl` region parameter.
func (p *duckDuckGoProvider) FiltersLanguage() bool { return true }

var _ SearchProvider = (*duckDuckGoProvider)(nil)
