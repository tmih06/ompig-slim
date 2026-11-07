package webtools

import (
	"reflect"
	"testing"
)

func TestParseSearchQueryDirectives(t *testing.T) {
	t.Parallel()
	query := ParseSearchQuery(`golang "context cancellation" site:go.dev after:2024-01-02 before:2025 filetype:pdf inurl:/blog intitle:context -deprecated OR obsolete -site:medium.com lang:en`)
	if got, want := query.Sites, []string{"go.dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sites = %q, want %q", got, want)
	}
	if got, want := query.ExcludedSites, []string{"medium.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("excludedSites = %q, want %q", got, want)
	}
	if got, want := query.InURL, []string{"/blog"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inURL = %q, want %q", got, want)
	}
	if got, want := query.InTitle, []string{"context"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inTitle = %q, want %q", got, want)
	}
	if got, want := query.Filetypes, []string{"pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filetypes = %q, want %q", got, want)
	}
	if query.After != "2024-01-02" || query.Before != "2025-01-01" {
		t.Fatalf("date bounds = %q..%q", query.After, query.Before)
	}
	if query.Lang != "en" {
		t.Fatalf("lang = %q", query.Lang)
	}
	if !query.HasDirectives || !query.HasConstraints {
		t.Fatalf("flags = directives %v constraints %v", query.HasDirectives, query.HasConstraints)
	}
	// The OR group joins two free terms; the negated term stays separate.
	var groups []int
	negated := 0
	for _, term := range query.Terms {
		if term.Group != 0 {
			groups = append(groups, term.Group)
		}
		if term.Negated {
			negated++
		}
	}
	if len(groups) != 2 || groups[0] != groups[1] {
		t.Fatalf("OR group ids = %v", groups)
	}
	if negated != 1 {
		t.Fatalf("negated terms = %d", negated)
	}
}

func TestParseSearchQueryLenient(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`https://example.com/a:b`,
		`C:\Users\me`,
		`TS2345: argument type`,
		`before:someday report`,
	} {
		query := ParseSearchQuery(raw)
		if query.HasConstraints {
			t.Fatalf("%q produced constraints: %+v", raw, query)
		}
		if query.Text == "" {
			t.Fatalf("%q lost its free text", raw)
		}
	}
	// An unparseable date degrades to a plain term instead of vanishing.
	degraded := ParseSearchQuery(`before:someday handoff notes`)
	if degraded.Before != "" {
		t.Fatalf("before = %q", degraded.Before)
	}
	if got := FormatQuery(degraded, GoogleQuerySyntax); got != `before:someday handoff notes` {
		t.Fatalf("formatted = %q", got)
	}
}

func TestParseDateValue(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2024":       "2024-01-01",
		"2024-03":    "2024-03-01",
		"2024-03-05": "2024-03-05",
		"2024/03/05": "2024-03-05",
		"03/05/2024": "2024-03-05",
		"25/12/2024": "2024-12-25",
	}
	for input, want := range cases {
		got, ok := parseDateValue(input)
		if !ok || got != want {
			t.Fatalf("parseDateValue(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, invalid := range []string{"someday", "2024-13-01", "2024-02-31", "24"} {
		if got, ok := parseDateValue(invalid); ok {
			t.Fatalf("parseDateValue(%q) = %q, want failure", invalid, got)
		}
	}
}

func TestFormatQuerySyntaxMasks(t *testing.T) {
	t.Parallel()
	query := ParseSearchQuery(`"exact phrase" golang site:go.dev -site:medium.com after:2024-01-01 filetype:pdf -term`)

	plain := FormatQuery(query, QuerySyntax{})
	if want := `exact phrase golang`; plain != want {
		t.Fatalf("plain = %q, want %q", plain, want)
	}

	ddg := FormatQuery(query, ddgQuerySyntax)
	if want := `"exact phrase" golang -term site:go.dev -site:medium.com filetype:pdf`; ddg != want {
		t.Fatalf("ddg = %q, want %q", ddg, want)
	}

	full := FormatQuery(query, GoogleQuerySyntax)
	if want := `"exact phrase" golang -term site:go.dev -site:medium.com filetype:pdf after:2024-01-01`; full != want {
		t.Fatalf("full = %q, want %q", full, want)
	}

	// A directives-only query never formats to the empty string.
	onlyDirective := ParseSearchQuery(`site:go.dev`)
	if got := FormatQuery(onlyDirective, QuerySyntax{}); got != "go.dev" {
		t.Fatalf("directives-only = %q", got)
	}
}

func TestFormatScraperQueryDemotesPathAndInURL(t *testing.T) {
	t.Parallel()
	// A path-carrying site: matches nothing on scraper engines, so it becomes a
	// plain keyword; bare-domain site: stays a filter.
	got := formatScraperQuery(`handbook site:github.com/anthropics inurl:/docs`, nil, ddgQuerySyntax)
	if want := `handbook github.com/anthropics /docs`; got != want {
		t.Fatalf("scraper query = %q, want %q", got, want)
	}
	// A negated site: is never demoted: that would invert an exclusion.
	negated := formatScraperQuery(`golang -site:medium.com`, nil, ddgQuerySyntax)
	if negated != `golang -site:medium.com` {
		t.Fatalf("negated = %q", negated)
	}
	// Directive-free queries pass through byte-identical.
	if raw := formatScraperQuery(`plain golang query`, nil, ddgQuerySyntax); raw != "plain golang query" {
		t.Fatalf("raw = %q", raw)
	}
}

func TestApplyQueryConstraintsRelaxesEliminatingDimension(t *testing.T) {
	t.Parallel()
	sources := []SearchSource{
		{Title: "Go blog", URL: "https://go.dev/blog/context", PublishedDate: "2024-06-01"},
		{Title: "Docs", URL: "https://go.dev/doc/context"},
	}

	// A site: that matches nothing is relaxed and reported.
	result := ApplyQueryConstraints(sources, ParseSearchQuery(`context site:example.com`))
	if len(result.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(result.Sources))
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != "site:example.com" {
		t.Fatalf("dropped = %v", result.Dropped)
	}

	// A site: that matches narrows without a note.
	narrowed := ApplyQueryConstraints(sources, ParseSearchQuery(`context inurl:/blog`))
	if len(narrowed.Sources) != 1 || narrowed.Sources[0].URL != "https://go.dev/blog/context" {
		t.Fatalf("narrowed = %+v", narrowed.Sources)
	}
	if len(narrowed.Dropped) != 0 {
		t.Fatalf("dropped = %v", narrowed.Dropped)
	}

	// A date bound drops the source dated before it and keeps the undated one,
	// because a missing date is not proof of a violation.
	dated := ApplyQueryConstraints(sources, ParseSearchQuery(`context after:2025-01-01`))
	if len(dated.Sources) != 1 || dated.Sources[0].URL != "https://go.dev/doc/context" {
		t.Fatalf("dated filter kept %+v", dated.Sources)
	}
	if len(dated.Dropped) != 0 {
		t.Fatalf("date dimension was relaxed unexpectedly: %v", dated.Dropped)
	}

	// A date bound that eliminates everything is relaxed.
	relaxed := ApplyQueryConstraints(sources[:1], ParseSearchQuery(`context after:2030-01-01`))
	if len(relaxed.Sources) != 1 || len(relaxed.Dropped) != 1 || relaxed.Dropped[0] != "after:2030-01-01" {
		t.Fatalf("relaxed = %+v", relaxed)
	}
}

func TestApplyQueryConstraintsMatchesSitesAndFiletypes(t *testing.T) {
	t.Parallel()
	if !matchesSite("https://sub.github.com/a", "github.com") {
		t.Fatal("subdomain should match")
	}
	if !matchesSite("https://github.com/anthropics/x", "github.com/anthropics") {
		t.Fatal("path prefix should match")
	}
	if matchesSite("https://github.com/other/x", "github.com/anthropics") {
		t.Fatal("path prefix should not match")
	}
	if matchesSite("https://notgithub.com/a", "github.com") {
		t.Fatal("suffix-only host should not match")
	}
	if !matchesFiletype("https://x.dev/paper.PDF", "pdf") {
		t.Fatal("filetype should be case-insensitive")
	}
}
