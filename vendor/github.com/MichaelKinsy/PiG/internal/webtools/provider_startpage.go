package webtools

// Startpage keyless search.
//
// Ports packages/coding-agent/src/web/search/providers/startpage.ts. Startpage
// proxies Google's index and serves fully server-rendered results. Its bot
// defense keys on requests that skip the homepage handshake: the search form
// carries a session token (`sc`) plus sibling hidden inputs, and posting the form
// with a stale or absent token redirects to the CAPTCHA shell. The flow is
// therefore the one a browser performs: GET the homepage, lift the form's hidden
// inputs, POST them back with the query. A failed handshake degrades to a direct
// GET.

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const (
	startpageHomeURL   = "https://www.startpage.com/"
	startpageSearchURL = "https://www.startpage.com/sp/search"
	startpageResults   = 20
	startpageDefaultN  = 10
)

// recencyToStartpageWithDate maps a recency window to Startpage's `with_date`.
var recencyToStartpageWithDate = map[string]string{"day": "d", "week": "w", "month": "m", "year": "y"}

// startpageProvider scrapes www.startpage.com.
type startpageProvider struct {
	endpoint string
}

func newStartpage(endpoint string) SearchProvider {
	if endpoint == "" {
		endpoint = startpageSearchURL
	}
	return &startpageProvider{endpoint: endpoint}
}

func (p *startpageProvider) ID() string                  { return "startpage" }
func (p *startpageProvider) Label() string               { return providerMeta["startpage"] }
func (p *startpageProvider) IsAvailable() bool           { return true }
func (p *startpageProvider) IsExplicitlyAvailable() bool { return true }
func (p *startpageProvider) Syntax() QuerySyntax         { return GoogleQuerySyntax }

func (p *startpageProvider) Search(ctx context.Context, params SearchParams) (SearchResponse, error) {
	parsed := params.Parsed
	if parsed == nil {
		parsed = ParseSearchQuery(params.Query)
	}
	want := clampNumResults(params.NumResultsOrDefault(), startpageDefaultN, startpageResults)
	query := formatScraperQuery(params.Query, parsed, GoogleQuerySyntax)
	withDate := recencyToStartpageWithDate[params.Recency]

	client := NewHTTPClient()
	defer client.CloseIdleConnections()

	home := homeOf(p.endpoint)
	page, pageURL, status, err := p.postSearch(ctx, client, home, query, withDate)
	if err != nil {
		if ctx.Err() != nil {
			return SearchResponse{}, ctx.Err()
		}
		return SearchResponse{}, &SearchError{
			Provider: p.ID(),
			Message:  "Startpage " + classifyProviderMessage(p.ID(), err),
			Status:   status,
		}
	}
	if isStartpageChallenge(pageURL, page) {
		return SearchResponse{}, &SearchError{
			Provider: p.ID(),
			Message: "Startpage blocked the request with a CAPTCHA challenge; Startpage rate-limits " +
				"automated searches from shared egress IPs",
			Status: http.StatusTooManyRequests,
		}
	}

	sources := make([]SearchSource, 0, want)
	seen := make(map[string]struct{}, want)
	for _, result := range parseStartpageResults(page) {
		if _, ok := seen[result.URL]; ok {
			continue
		}
		seen[result.URL] = struct{}{}
		sources = append(sources, result)
		if len(sources) >= want {
			break
		}
	}
	return SearchResponse{Provider: p.ID(), Sources: sources, AuthMode: "keyless"}, nil
}

// postSearch runs the homepage handshake and the search POST, falling back to a
// tokenless GET when the form cannot be lifted.
func (p *startpageProvider) postSearch(ctx context.Context, client *http.Client, home, query, withDate string) (string, string, int, error) {
	headers := make(http.Header)
	headers.Set("Referer", home)

	formInputs := p.fetchFormInputs(ctx, client, home)
	if formInputs != nil {
		form := url.Values{}
		for name, value := range formInputs {
			form.Set(name, value)
		}
		form.Set("query", query)
		if withDate != "" {
			form.Set("with_date", withDate)
		}
		postHeaders := headers.Clone()
		postHeaders.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := Load(ctx, client, http.MethodPost, p.endpoint, []byte(form.Encode()), postHeaders, DefaultMaxBytes)
		if err != nil {
			return "", "", response.StatusCode, err
		}
		return string(response.Body), response.URL, response.StatusCode, nil
	}

	target, err := url.Parse(p.endpoint)
	if err != nil {
		return "", "", 0, &SearchError{Provider: p.ID(), Message: "Startpage endpoint is not a valid URL"}
	}
	values := target.Query()
	values.Set("query", query)
	if withDate != "" {
		values.Set("with_date", withDate)
	}
	target.RawQuery = values.Encode()
	response, err := Load(ctx, client, http.MethodGet, target.String(), nil, headers, DefaultMaxBytes)
	if err != nil {
		return "", "", response.StatusCode, err
	}
	return string(response.Body), response.URL, response.StatusCode, nil
}

// fetchFormInputs lifts the hidden inputs of the homepage's /sp/search form.
// Any failure yields nil so the caller degrades to a direct GET.
func (p *startpageProvider) fetchFormInputs(ctx context.Context, client *http.Client, home string) map[string]string {
	response, err := Load(ctx, client, http.MethodGet, home, nil, nil, DefaultMaxBytes)
	if err != nil {
		return nil
	}
	page := string(response.Body)
	if isStartpageChallenge(response.URL, page) {
		return nil
	}
	return parseStartpageFormInputs(page)
}

// parseStartpageFormInputs returns the hidden inputs of the search form,
// requiring the `sc` anti-bot token.
func parseStartpageFormInputs(page string) map[string]string {
	root, err := searchDOMTree([]byte(page))
	if err != nil {
		return nil
	}
	var inputs map[string]string
	searchDOMWalk(root, func(node *html.Node) bool {
		if inputs != nil {
			return false
		}
		if node.Type != html.ElementNode || node.Data != "form" {
			return true
		}
		if !strings.Contains(searchDOMAttr(node, "action"), "/sp/search") {
			return true
		}
		collected := map[string]string{}
		searchDOMWalk(node, func(input *html.Node) bool {
			if input.Type != html.ElementNode || input.Data != "input" {
				return true
			}
			if !strings.EqualFold(searchDOMAttr(input, "type"), "hidden") {
				return true
			}
			name := searchDOMAttr(input, "name")
			if name == "" {
				return true
			}
			collected[name] = searchDOMAttr(input, "value")
			return true
		})
		if collected["sc"] != "" {
			inputs = collected
			return false
		}
		return true
	})
	return inputs
}

// isStartpageChallenge reports the bot wall. Three shells are in use: rejected
// requests redirect to /en/errors/ or /sp/captcha, whose Gatsby chunk map names
// the captcha components; the frontend serves an Anubis proof-of-work page whose
// JSON carries `anubis_challenge`; and a hard block lands on Startpage's CDN
// error page (`/sp/cdn/error-pages/blocked.html`, titled "Access Denied").
// The body marker matters because a mock carries no final URL. A bare "captcha"
// or "challenge" substring is deliberately not used: result snippets for
// captcha-related queries would false-positive.
func isStartpageChallenge(pageURL, page string) bool {
	lower := strings.ToLower(pageURL)
	if strings.Contains(lower, "/errors/") || strings.Contains(lower, "/captcha/") ||
		strings.Contains(lower, "/sp/captcha") || strings.Contains(lower, "/error-pages/") {
		return true
	}
	return strings.Contains(page, "component---src-pages-captcha") ||
		strings.Contains(page, "/sp/captcha") ||
		strings.Contains(page, `id="anubis_challenge"`) ||
		strings.Contains(page, `id="anubis_version"`) ||
		strings.Contains(page, "<title>Access Denied - Startpage</title>")
}

// parseStartpageResults walks the server-rendered page. Each organic hit lives
// in a div.result holding the title anchor a.result-link (with an h2 heading) and
// an optional p.description snippet. Hrefs are direct target URLs.
func parseStartpageResults(page string) []SearchSource {
	root, err := searchDOMTree([]byte(page))
	if err != nil {
		return nil
	}
	results := make([]SearchSource, 0, 10)
	for _, block := range searchDOMClassElements(root, "result") {
		anchor := searchDOMFirstClass(block, "result-link")
		if anchor == nil {
			continue
		}
		target := sanitizeStartpageURL(searchDOMAttr(anchor, "href"))
		if target == "" {
			continue
		}
		title := searchDOMText(anchor)
		if title == "" {
			continue
		}
		result := SearchSource{Title: title, URL: target}
		if snippet := searchDOMFirstClass(block, "description"); snippet != nil {
			result.Snippet = searchDOMText(snippet)
		}
		results = append(results, result)
	}
	return results
}

// sanitizeStartpageURL accepts only http(s) targets that point away from
// Startpage itself.
func sanitizeStartpageURL(href string) string {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		base, baseErr := url.Parse(startpageHomeURL)
		if baseErr != nil {
			return ""
		}
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "startpage.com" || strings.HasSuffix(host, ".startpage.com") {
		return ""
	}
	return normalizeHTTPURL(parsed.String())
}

// homeOf derives the homepage of a search endpoint, so an endpoint override in
// webSearch.endpoints keeps the handshake on the same fixture host.
func homeOf(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return startpageHomeURL
	}
	return parsed.Scheme + "://" + parsed.Host + "/"
}

// FiltersRecency reports that the engine applies the recency window natively.
func (p *startpageProvider) FiltersRecency() bool { return true }

var _ SearchProvider = (*startpageProvider)(nil)
