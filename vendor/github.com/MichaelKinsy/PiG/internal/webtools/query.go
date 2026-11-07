package webtools

// Structured web-search query parsing.
//
// Agents habitually embed Google-style directives in search queries — `site:`,
// `before:`/`after:`, `inurl:`, `filetype:`, quoted phrases, `OR` groups,
// `-exclusions` — regardless of whether the backing engine parses them. This file
// turns a raw query into a StructuredQuery so each provider can
//  1. map constraints onto native parameters where they exist,
//  2. rebuild a query containing only the syntax the target engine understands
//     (FormatQuery), and
//  3. post-filter returned sources leniently (ApplyQueryConstraints): a dimension
//     that would eliminate every result is dropped and reported rather than
//     returning nothing.
//
// Ports packages/coding-agent/src/web/search/query.ts.

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// QueryTerm is one free-text token of the query: everything that is not a
// recognized directive.
type QueryTerm struct {
	// Text is the term without quotes or operator prefixes.
	Text string `json:"text"`
	// Phrase marks a quoted exact phrase ("like this") or verbatim term (+term).
	Phrase bool `json:"phrase,omitempty"`
	// Negated marks an exclusion (-term or NOT term).
	Negated bool `json:"negated,omitempty"`
	// Group is an OR-group id; terms sharing one are alternatives. Zero means
	// the term stands alone (implicit AND).
	Group int `json:"group,omitempty"`
}

// StructuredQuery is a raw query decomposed into free text plus every recognized
// constraint. List fields are always non-nil so consumers can range over them.
type StructuredQuery struct {
	Raw   string      `json:"raw"`
	Text  string      `json:"text"`
	Terms []QueryTerm `json:"terms"`

	// Sites are site:/domain:/host: includes — any-of. Lowercased, scheme
	// stripped, may carry a path (github.com/anthropics).
	Sites []string `json:"sites"`
	// ExcludedSites are -site: exclusions, normalized like Sites.
	ExcludedSites []string `json:"excludedSites"`
	// InURL are inurl:/url: substrings — all must appear in the URL.
	InURL []string `json:"inURL"`
	// ExcludedInURL are -inurl: substrings — none may appear in the URL.
	ExcludedInURL []string `json:"excludedInURL"`
	// InTitle are intitle:/title: substrings — all must appear in the title.
	InTitle []string `json:"inTitle"`
	// ExcludedInTitle are -intitle: substrings.
	ExcludedInTitle []string `json:"excludedInTitle"`
	// InText are intext:/inbody:/inanchor: body substrings. Not post-filterable
	// (snippets are partial); query-building only.
	InText []string `json:"inText"`
	// ExcludedInText are -intext: body exclusions. Query-building only.
	ExcludedInText []string `json:"excludedInText"`
	// Filetypes are filetype:/ext: extensions — any-of, lowercased, no dot.
	Filetypes []string `json:"filetypes"`
	// ExcludedFiletypes are -filetype: extensions.
	ExcludedFiletypes []string `json:"excludedFiletypes"`
	// After is the inclusive lower published-date bound (ISO YYYY-MM-DD).
	After string `json:"after,omitempty"`
	// Before is the exclusive upper published-date bound (ISO YYYY-MM-DD).
	Before string `json:"before,omitempty"`
	// Lang is a language code from lang:/language:, lowercased.
	Lang string `json:"lang,omitempty"`
	// HasDirectives is true when any directive or boolean operator was recognized.
	HasDirectives bool `json:"hasDirectives"`
	// HasConstraints is true when any post-filterable constraint is set.
	HasConstraints bool `json:"hasConstraints"`
}

// QuerySyntax is the query capability mask of a target engine. The zero value
// emits plain keywords, which is what natural-language APIs want.
type QuerySyntax struct {
	Phrases   bool `json:"phrases,omitempty"`
	Negation  bool `json:"negation,omitempty"`
	Or        bool `json:"or,omitempty"`
	Site      bool `json:"site,omitempty"`
	InURL     bool `json:"inURL,omitempty"`
	InTitle   bool `json:"inTitle,omitempty"`
	InText    bool `json:"inText,omitempty"`
	Filetype  bool `json:"filetype,omitempty"`
	DateRange bool `json:"dateRange,omitempty"`
}

// GoogleQuerySyntax is the full classic operator set: engines that parse it
// include Startpage, Firecrawl's SERP-backed search, Brave, Kagi and SearXNG.
var GoogleQuerySyntax = QuerySyntax{
	Phrases: true, Negation: true, Or: true, Site: true,
	InURL: true, InTitle: true, InText: true, Filetype: true, DateRange: true,
}

// ConstraintFilterResult is the outcome of ApplyQueryConstraints.
type ConstraintFilterResult struct {
	// Sources survive the lenient filter — never empty when the input was not.
	Sources []SearchSource
	// Dropped lists directive renderings (`site:arxiv.org`, `before:2024-01-01`)
	// of the dimensions that matched zero sources and were therefore relaxed.
	Dropped []string
}

type rawToken struct {
	text        string
	quoted      bool
	quotedValue bool
}

func isQuote(r rune) bool { return r == '"' || r == '\u201c' || r == '\u201d' }

func isSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f', '\u00a0':
		return true
	}
	return false
}

// tokenize splits a raw query into whitespace-delimited tokens, honoring quoted
// spans and standalone parens.
func tokenize(raw string) []rawToken {
	rs := []rune(raw)
	n := len(rs)
	tokens := make([]rawToken, 0, 16)
	i := 0
	for i < n {
		if isSpace(rs[i]) {
			i++
			continue
		}
		if isQuote(rs[i]) {
			j := i + 1
			var buf []rune
			for j < n && !isQuote(rs[j]) {
				buf = append(buf, rs[j])
				j++
			}
			if text := strings.TrimSpace(string(buf)); text != "" {
				tokens = append(tokens, rawToken{text: text, quoted: true})
			}
			i = j + 1
			continue
		}
		var buf []rune
		quotedValue := false
		for i < n && !isSpace(rs[i]) {
			c := rs[i]
			if isQuote(c) && len(buf) > 0 && buf[len(buf)-1] == ':' {
				j := i + 1
				for j < n && !isQuote(rs[j]) {
					buf = append(buf, rs[j])
					j++
				}
				quotedValue = true
				i = j + 1
				continue
			}
			if isQuote(c) {
				break // foo"bar — stop the word, let the quote start a phrase
			}
			buf = append(buf, c)
			i++
		}
		if len(buf) > 0 {
			tokens = append(tokens, rawToken{text: string(buf), quotedValue: quotedValue})
		}
	}
	return splitParens(tokens)
}

// splitParens turns leading "(" and unbalanced trailing ")" into standalone
// tokens so `(react OR vue)` parses while `site:wikipedia.org/Foo_(bar)` stays
// whole.
func splitParens(tokens []rawToken) []rawToken {
	out := make([]rawToken, 0, len(tokens))
	for _, tok := range tokens {
		if tok.quoted || tok.quotedValue {
			out = append(out, tok)
			continue
		}
		text := tok.text
		for strings.HasPrefix(text, "(") {
			out = append(out, rawToken{text: "("})
			text = text[1:]
		}
		trailing := 0
		for strings.HasSuffix(text, ")") {
			body := text[:len(text)-1]
			depth := 0
			for _, c := range body {
				switch c {
				case '(':
					depth++
				case ')':
					depth--
				}
			}
			if depth > 0 {
				break
			}
			text = body
			trailing++
		}
		if text != "" {
			out = append(out, rawToken{text: text})
		}
		for range trailing {
			out = append(out, rawToken{text: ")"})
		}
	}
	return out
}

func isoDate(year, month, day int) (string, bool) {
	if year < 1000 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 {
		return "", false
	}
	// Reject days that do not exist in the month (2024-02-31).
	if t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC); t.Day() != day || int(t.Month()) != month {
		return "", false
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), true
}

var (
	dateYearFirst = regexp.MustCompile(`^(\d{4})(?:[-/.](\d{1,2})(?:[-/.](\d{1,2}))?)?$`)
	dateYearLast  = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{4})$`)
)

// parseDateValue parses a before:/after: value into ISO YYYY-MM-DD. Accepts
// YYYY, YYYY-MM, YYYY-MM-DD (also / and . separators) and MM/DD/YYYY. Bare
// years/months resolve to the first day of the period, matching Google's
// `after:2024` semantics.
func parseDateValue(value string) (string, bool) {
	t := strings.TrimSpace(value)
	if m := dateYearFirst.FindStringSubmatch(t); m != nil {
		month, day := 1, 1
		if m[2] != "" {
			month = mustAtoi(m[2])
		}
		if m[3] != "" {
			day = mustAtoi(m[3])
		}
		return isoDate(mustAtoi(m[1]), month, day)
	}
	if m := dateYearLast.FindStringSubmatch(t); m != nil {
		month, day := mustAtoi(m[1]), mustAtoi(m[2])
		if month > 12 && day <= 12 {
			month, day = day, month
		}
		return isoDate(mustAtoi(m[3]), month, day)
	}
	return "", false
}

// normalizeSite lowercases a site: value and strips scheme, *. wildcard and
// trailing slash/dot.
func normalizeSite(value string) string {
	site := strings.ToLower(strings.TrimSpace(value))
	if i := strings.Index(site, "://"); i > 0 && isSchemePrefix(site[:i]) {
		site = site[i+3:]
	}
	site = strings.TrimPrefix(site, "*.")
	site = strings.TrimRight(site, "/.")
	return site
}

func isSchemePrefix(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '-', r == '.':
		default:
			return false
		}
		if i == 0 && !(r >= 'a' && r <= 'z') {
			return false
		}
	}
	return s != ""
}

type directiveField int

const (
	fieldNone directiveField = iota
	fieldSite
	fieldInURL
	fieldInTitle
	fieldInText
	fieldFiletype
	fieldBefore
	fieldAfter
	fieldLang
)

var directiveFields = map[string]directiveField{
	"site": fieldSite, "domain": fieldSite, "host": fieldSite,
	"inurl": fieldInURL, "url": fieldInURL,
	"intitle": fieldInTitle, "title": fieldInTitle,
	"intext": fieldInText, "inbody": fieldInText, "inanchor": fieldInText,
	"filetype": fieldFiletype, "ext": fieldFiletype,
	"before": fieldBefore, "until": fieldBefore,
	"after": fieldAfter, "since": fieldAfter,
	"lang": fieldLang, "language": fieldLang,
}

var allModes = map[string]directiveField{
	"allintitle": fieldInTitle,
	"allinurl":   fieldInURL,
	"allintext":  fieldInText,
}

// splitDirective matches `[-+]?name:value`.
func splitDirective(text string) (negated bool, name, value string, ok bool) {
	t := text
	if strings.HasPrefix(t, "-") {
		negated = true
		t = t[1:]
	} else if strings.HasPrefix(t, "+") {
		t = t[1:]
	}
	colon := strings.IndexByte(t, ':')
	if colon <= 0 {
		return false, "", "", false
	}
	name = t[:colon]
	if name == "" {
		return false, "", "", false
	}
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		if i == 0 {
			return false, "", "", false
		}
		if r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return false, "", "", false
	}
	return negated, name, t[colon+1:], true
}

// isReservedToken reports operator/paren tokens and recognized directives —
// anything a bare `name:` must not adopt as its value.
func isReservedToken(text string) bool {
	switch text {
	case "(", ")", "OR", "AND", "NOT", "|", "||", "&&", "!":
		return true
	}
	_, name, _, ok := splitDirective(text)
	if !ok {
		return false
	}
	name = strings.ToLower(name)
	return directiveFields[name] != fieldNone || allModes[name] != fieldNone
}

// ParseSearchQuery decomposes a raw query. It is lenient by construction:
// unknown `name:value` tokens (URLs, C:\paths, TS2345:, jargon) stay in the free
// text verbatim, and a directive with an unparseable value degrades to a plain
// term instead of being dropped.
func ParseSearchQuery(raw string) *StructuredQuery {
	q := &StructuredQuery{
		Raw:               raw,
		Sites:             []string{},
		ExcludedSites:     []string{},
		InURL:             []string{},
		ExcludedInURL:     []string{},
		InTitle:           []string{},
		ExcludedInTitle:   []string{},
		InText:            []string{},
		ExcludedInText:    []string{},
		Filetypes:         []string{},
		ExcludedFiletypes: []string{},
		Terms:             []QueryTerm{},
	}

	tokens := tokenize(raw)
	negateNext := false
	orPending := false
	lastWasTerm := false
	groupSeq := 0
	mode := fieldNone

	pushConstraint := func(field directiveField, value string, negated bool) {
		q.HasDirectives = true
		orPending = false
		lastWasTerm = false
		v := strings.TrimSpace(value)
		if v == "" {
			return
		}
		switch field {
		case fieldSite:
			if site := normalizeSite(v); site != "" {
				if negated {
					q.ExcludedSites = append(q.ExcludedSites, site)
				} else {
					q.Sites = append(q.Sites, site)
				}
			}
		case fieldInURL:
			if negated {
				q.ExcludedInURL = append(q.ExcludedInURL, v)
			} else {
				q.InURL = append(q.InURL, v)
			}
		case fieldInTitle:
			if negated {
				q.ExcludedInTitle = append(q.ExcludedInTitle, v)
			} else {
				q.InTitle = append(q.InTitle, v)
			}
		case fieldInText:
			if negated {
				q.ExcludedInText = append(q.ExcludedInText, v)
			} else {
				q.InText = append(q.InText, v)
			}
		case fieldFiletype:
			ext := strings.ToLower(strings.TrimLeft(v, "."))
			if ext != "" {
				if negated {
					q.ExcludedFiletypes = append(q.ExcludedFiletypes, ext)
				} else {
					q.Filetypes = append(q.Filetypes, ext)
				}
			}
		}
	}

	pushTerm := func(text string, phrase bool) {
		negated := negateNext
		negateNext = false
		if mode != fieldNone {
			pushConstraint(mode, text, negated)
			return
		}
		term := QueryTerm{Text: text, Phrase: phrase, Negated: negated}
		if orPending && lastWasTerm && len(q.Terms) > 0 {
			prev := &q.Terms[len(q.Terms)-1]
			if prev.Group == 0 {
				groupSeq++
				prev.Group = groupSeq
			}
			term.Group = prev.Group
		}
		orPending = false
		lastWasTerm = true
		q.Terms = append(q.Terms, term)
	}

	for idx := 0; idx < len(tokens); idx++ {
		tok := tokens[idx]

		if tok.quoted {
			pushTerm(tok.text, true)
			continue
		}

		switch tok.text {
		case "(", ")":
			continue
		case "OR", "|", "||":
			orPending = true
			q.HasDirectives = true
			continue
		case "AND", "&&":
			q.HasDirectives = true
			continue
		case "NOT", "!":
			negateNext = true
			q.HasDirectives = true
			continue
		case "-", "+":
			// The tokenizer splits `-"exact phrase"` into `-` + phrase.
			if tok.text == "-" && idx+1 < len(tokens) && tokens[idx+1].quoted {
				negateNext = true
			}
			continue
		}

		negated, name, value, isDirective := splitDirective(tok.text)
		if isDirective {
			name = strings.ToLower(name)
			if m := allModes[name]; m != fieldNone {
				mode = m
				q.HasDirectives = true
				if inline := strings.TrimSpace(value); inline != "" {
					pushConstraint(m, inline, negated)
				}
				orPending = false
				lastWasTerm = false
				continue
			}
			if field := directiveFields[name]; field != fieldNone {
				value = strings.TrimSpace(value)
				// `site: example.com` — adopt the next plain token as the value.
				if value == "" && idx+1 < len(tokens) {
					next := tokens[idx+1]
					if next.quoted || !isReservedToken(next.text) {
						value = strings.TrimSpace(next.text)
						idx++
					}
				}
				if value == "" {
					q.HasDirectives = true
					continue
				}
				isNeg := negated || negateNext
				negateNext = false
				switch field {
				case fieldBefore, fieldAfter:
					iso, ok := parseDateValue(value)
					if !ok {
						pushTerm(tok.text, false)
						continue
					}
					if field == fieldBefore {
						q.Before = iso
					} else {
						q.After = iso
					}
					q.HasDirectives = true
					orPending = false
					lastWasTerm = false
				case fieldLang:
					q.Lang = strings.ToLower(value)
					q.HasDirectives = true
					orPending = false
					lastWasTerm = false
				default:
					pushConstraint(field, value, isNeg)
				}
				continue
			}
		}

		text := tok.text
		if strings.HasPrefix(text, "-") && len(text) > 1 {
			negateNext = true
			q.HasDirectives = true
			text = strings.TrimLeft(text, "-")
			if text == "" {
				continue
			}
			// `-site:x` written pre-split re-checks as a directive.
			if _, name, value, ok := splitDirective(text); ok {
				if field := directiveFields[strings.ToLower(name)]; field != fieldNone &&
					field != fieldBefore && field != fieldAfter && field != fieldLang {
					negateNext = false
					pushConstraint(field, value, true)
					continue
				}
			}
			pushTerm(text, false)
			continue
		}
		if strings.HasPrefix(text, "+") && len(text) > 1 {
			// Legacy Google +term: verbatim/required — exact phrase.
			pushTerm(text[1:], true)
			q.HasDirectives = true
			continue
		}
		pushTerm(text, false)
	}

	q.Text = renderTerms(q.Terms, QuerySyntax{Phrases: true, Negation: true, Or: true})
	q.HasConstraints = len(q.Sites) > 0 || len(q.ExcludedSites) > 0 ||
		len(q.InURL) > 0 || len(q.ExcludedInURL) > 0 ||
		len(q.InTitle) > 0 || len(q.ExcludedInTitle) > 0 ||
		len(q.Filetypes) > 0 || len(q.ExcludedFiletypes) > 0 ||
		q.After != "" || q.Before != ""
	return q
}

func quoteValue(value string) string {
	if strings.ContainsAny(value, " \t") {
		return `"` + value + `"`
	}
	return value
}

func renderTerms(terms []QueryTerm, syntax QuerySyntax) string {
	parts := make([]string, 0, len(terms))
	for i := 0; i < len(terms); i++ {
		term := terms[i]
		if term.Group != 0 && syntax.Or {
			members := make([]string, 0, 2)
			j := i
			for ; j < len(terms) && terms[j].Group == term.Group; j++ {
				if rendered := renderTerm(terms[j], syntax); rendered != "" {
					members = append(members, rendered)
				}
			}
			i = j - 1
			switch len(members) {
			case 0:
			case 1:
				parts = append(parts, members[0])
			default:
				parts = append(parts, "("+strings.Join(members, " OR ")+")")
			}
			continue
		}
		if rendered := renderTerm(term, syntax); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, " ")
}

func renderTerm(term QueryTerm, syntax QuerySyntax) string {
	if term.Negated && !syntax.Negation {
		return ""
	}
	body := term.Text
	if term.Phrase && syntax.Phrases {
		body = `"` + term.Text + `"`
	}
	if term.Negated {
		return "-" + body
	}
	return body
}

// FormatQuery rebuilds a query string for an engine with the given syntax.
// Constraints whose syntax the engine lacks are omitted; the caller maps them
// onto native parameters or relies on ApplyQueryConstraints. It never returns
// an empty string for a non-empty input: a directives-only query falls back to
// the constraint values as keywords, then to the raw query.
func FormatQuery(q *StructuredQuery, syntax QuerySyntax) string {
	if q == nil {
		return ""
	}
	parts := make([]string, 0, 8)
	if text := renderTerms(q.Terms, syntax); text != "" {
		parts = append(parts, text)
	}
	if syntax.Site {
		if len(q.Sites) > 1 && syntax.Or {
			sites := make([]string, len(q.Sites))
			for i, s := range q.Sites {
				sites[i] = "site:" + s
			}
			parts = append(parts, "("+strings.Join(sites, " OR ")+")")
		} else {
			for _, s := range q.Sites {
				parts = append(parts, "site:"+s)
			}
		}
		for _, s := range q.ExcludedSites {
			parts = append(parts, "-site:"+s)
		}
	}
	if syntax.InURL {
		for _, v := range q.InURL {
			parts = append(parts, "inurl:"+quoteValue(v))
		}
		for _, v := range q.ExcludedInURL {
			parts = append(parts, "-inurl:"+quoteValue(v))
		}
	}
	if syntax.InTitle {
		for _, v := range q.InTitle {
			parts = append(parts, "intitle:"+quoteValue(v))
		}
		for _, v := range q.ExcludedInTitle {
			parts = append(parts, "-intitle:"+quoteValue(v))
		}
	}
	if syntax.InText {
		for _, v := range q.InText {
			parts = append(parts, "intext:"+quoteValue(v))
		}
		for _, v := range q.ExcludedInText {
			parts = append(parts, "-intext:"+quoteValue(v))
		}
	}
	if syntax.Filetype {
		if len(q.Filetypes) > 1 && syntax.Or {
			exts := make([]string, len(q.Filetypes))
			for i, f := range q.Filetypes {
				exts[i] = "filetype:" + f
			}
			parts = append(parts, "("+strings.Join(exts, " OR ")+")")
		} else {
			for _, f := range q.Filetypes {
				parts = append(parts, "filetype:"+f)
			}
		}
		for _, f := range q.ExcludedFiletypes {
			parts = append(parts, "-filetype:"+f)
		}
	}
	if syntax.DateRange {
		if q.After != "" {
			parts = append(parts, "after:"+q.After)
		}
		if q.Before != "" {
			parts = append(parts, "before:"+q.Before)
		}
	}

	result := strings.TrimSpace(strings.Join(parts, " "))
	if result == "" {
		fallback := make([]string, 0, 8)
		fallback = append(fallback, q.Sites...)
		fallback = append(fallback, q.InTitle...)
		fallback = append(fallback, q.InURL...)
		fallback = append(fallback, q.InText...)
		fallback = append(fallback, q.Filetypes...)
		result = strings.TrimSpace(strings.Join(fallback, " "))
	}
	if result != "" {
		return result
	}
	return strings.TrimSpace(q.Raw)
}

// formatScraperQuery builds the engine query for a credential-free HTML engine.
// It canonicalizes directives via FormatQuery with the engine's syntax, but
// first demotes the operators that zero-match across the scraper set: engines
// only match site: against a bare domain (a path yields zero results), and some
// engines ignore inurl: entirely — either operator silently empties the result
// set. The demoted values become plain keywords and the pipeline post-filter
// enforces them on the returned sources. Directive-free queries pass through
// byte-identical.
func formatScraperQuery(query string, parsed *StructuredQuery, syntax QuerySyntax) string {
	if parsed == nil {
		parsed = ParseSearchQuery(query)
	}
	if !parsed.HasDirectives {
		return query
	}
	demoted := make([]string, 0, len(parsed.InURL))
	for _, site := range parsed.Sites {
		if strings.Contains(site, "/") {
			demoted = append(demoted, site)
		}
	}
	demoted = append(demoted, parsed.InURL...)

	downgraded := *parsed
	downgraded.Sites = make([]string, 0, len(parsed.Sites))
	for _, site := range parsed.Sites {
		if !strings.Contains(site, "/") {
			downgraded.Sites = append(downgraded.Sites, site)
		}
	}
	downgraded.InURL = []string{}
	downgraded.Terms = make([]QueryTerm, 0, len(parsed.Terms)+len(demoted))
	downgraded.Terms = append(downgraded.Terms, parsed.Terms...)
	for _, text := range demoted {
		downgraded.Terms = append(downgraded.Terms, QueryTerm{Text: text})
	}
	return FormatQuery(&downgraded, syntax)
}

// matchesSite matches a URL against a site: value: exact host or subdomain of
// the site host; a site carrying a path requires the URL path to start with it.
func matchesSite(rawURL, site string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	siteHost, sitePath, hasPath := strings.Cut(site, "/")
	host := strings.ToLower(u.Hostname())
	if host != siteHost && !strings.HasSuffix(host, "."+siteHost) {
		return false
	}
	if hasPath {
		prefix := "/" + sitePath
		if !strings.HasPrefix(strings.ToLower(u.Path), strings.ToLower(prefix)) {
			return false
		}
	}
	return true
}

// matchesFiletype matches a URL whose pathname ends with .ext.
func matchesFiletype(rawURL, ext string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), "."+strings.ToLower(ext))
}

var (
	relativeAgePattern = regexp.MustCompile(`^(\d+)\s*(minute|min|hour|hr|day|week|month|mo|year|yr|[mhdwy])s?\s+ago$`)
)

var relativeUnitSeconds = map[string]int{
	"m": 60, "min": 60, "minute": 60,
	"h": 3600, "hr": 3600, "hour": 3600,
	"d": 86400, "day": 86400,
	"w": 604800, "week": 604800,
	"mo": 2592000, "month": 2592000,
	"y": 31536000, "yr": 31536000, "year": 31536000,
}

// sourceTime is the best-effort publish time (ms epoch) of a source.
func sourceTime(source SearchSource, now time.Time) (int64, bool) {
	if source.AgeSeconds != nil {
		return now.UnixMilli() - int64(*source.AgeSeconds)*1000, true
	}
	if source.PublishedDate == "" {
		return 0, false
	}
	if m := relativeAgePattern.FindStringSubmatch(strings.TrimSpace(source.PublishedDate)); m != nil {
		seconds := mustAtoi(m[1]) * relativeUnitSeconds[strings.ToLower(m[2])]
		if seconds <= 0 {
			return 0, false
		}
		return now.UnixMilli() - int64(seconds)*1000, true
	}
	if parsed, ok := parsePublishedAt(source.PublishedDate); ok {
		return parsed.UnixMilli(), true
	}
	return 0, false
}

type constraintDimension struct {
	label string
	pred  func(SearchSource) bool
}

func constraintDimensions(q *StructuredQuery) []constraintDimension {
	dims := make([]constraintDimension, 0, 10)
	add := func(values []string, prefix, separator string, pred func(SearchSource) bool) {
		if len(values) == 0 {
			return
		}
		labels := make([]string, len(values))
		for i, value := range values {
			labels[i] = prefix + value
		}
		dims = append(dims, constraintDimension{label: strings.Join(labels, separator), pred: pred})
	}

	add(q.Sites, "site:", " OR ", func(src SearchSource) bool {
		return anySite(src.URL, q.Sites)
	})
	add(q.ExcludedSites, "-site:", " ", func(src SearchSource) bool {
		return !anySite(src.URL, q.ExcludedSites)
	})

	addText := func(name string, included, excluded []string, text func(SearchSource) string) {
		add(included, name+":", " ", func(src SearchSource) bool {
			value := strings.ToLower(text(src))
			for _, term := range included {
				if !strings.Contains(value, strings.ToLower(term)) {
					return false
				}
			}
			return true
		})
		add(excluded, "-"+name+":", " ", func(src SearchSource) bool {
			value := strings.ToLower(text(src))
			for _, term := range excluded {
				if strings.Contains(value, strings.ToLower(term)) {
					return false
				}
			}
			return true
		})
	}
	addText("inurl", q.InURL, q.ExcludedInURL, func(src SearchSource) string { return src.URL })
	addText("intitle", q.InTitle, q.ExcludedInTitle, func(src SearchSource) string { return src.Title })

	add(q.Filetypes, "filetype:", " OR ", func(src SearchSource) bool {
		return slices.ContainsFunc(q.Filetypes, func(ext string) bool { return matchesFiletype(src.URL, ext) })
	})
	add(q.ExcludedFiletypes, "-filetype:", " ", func(src SearchSource) bool {
		return !slices.ContainsFunc(q.ExcludedFiletypes, func(ext string) bool { return matchesFiletype(src.URL, ext) })
	})
	if q.After != "" || q.Before != "" {
		labelParts := make([]string, 0, 2)
		if q.After != "" {
			labelParts = append(labelParts, "after:"+q.After)
		}
		if q.Before != "" {
			labelParts = append(labelParts, "before:"+q.Before)
		}
		var afterMs, beforeMs int64
		var hasAfter, hasBefore bool
		if q.After != "" {
			if t, err := time.Parse("2006-01-02", q.After); err == nil {
				afterMs, hasAfter = t.UnixMilli(), true
			}
		}
		if q.Before != "" {
			if t, err := time.Parse("2006-01-02", q.Before); err == nil {
				beforeMs, hasBefore = t.UnixMilli(), true
			}
		}
		dims = append(dims, constraintDimension{
			label: strings.Join(labelParts, " "),
			pred: func(src SearchSource) bool {
				millis, ok := sourceTime(src, time.Now())
				if !ok {
					return true // undated → cannot prove violation
				}
				if hasAfter && millis < afterMs {
					return false
				}
				if hasBefore && millis >= beforeMs {
					return false
				}
				return true
			},
		})
	}
	return dims
}

func anySite(rawURL string, sites []string) bool {
	for _, site := range sites {
		if matchesSite(rawURL, site) {
			return true
		}
	}
	return false
}

// ApplyQueryConstraints applies each constraint dimension of q in turn, skipping
// (and reporting) any dimension that would eliminate every remaining source. It
// guarantees a non-empty result for a non-empty input, so a mis-scoped directive
// degrades to unfiltered results plus a note instead of a dead search.
func ApplyQueryConstraints(sources []SearchSource, q *StructuredQuery) ConstraintFilterResult {
	current := append([]SearchSource(nil), sources...)
	dropped := []string{}
	if q == nil || len(current) == 0 {
		return ConstraintFilterResult{Sources: current, Dropped: dropped}
	}
	for _, dim := range constraintDimensions(q) {
		kept := make([]SearchSource, 0, len(current))
		for _, src := range current {
			if dim.pred(src) {
				kept = append(kept, src)
			}
		}
		if len(kept) > 0 {
			current = kept
			continue
		}
		dropped = append(dropped, dim.label)
	}
	return ConstraintFilterResult{Sources: current, Dropped: dropped}
}

// mustAtoi parses a regexp-matched decimal, where a failure is a programming error.
func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
