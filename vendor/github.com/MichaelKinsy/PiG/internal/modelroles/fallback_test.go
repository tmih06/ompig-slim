package modelroles

import (
	"reflect"
	"testing"
)

// TestFallbackSelectorLiteralRoutingAndWildcard covers OMP's exact-first @ IDs,
// upstream routing, thinking suffixes, and aggregator/direct wildcard translation.
func TestFallbackSelectorLiteralRoutingAndWildcard(t *testing.T) {
	lookup := func(p, id string) bool {
		return p == "vertex" && id == "claude@default" || p == "google" && id == "gemini"
	}
	for _, tc := range []struct{ raw, id, route, level string }{{"vertex/claude@default:off", "claude@default", "", "off"}, {"openrouter/vendor/model@fireworks:high", "vendor/model", "fireworks", "high"}, {"p/m:inherit", "m", "", "inherit"}} {
		parsed, ok := ParseSelector(tc.raw, lookup)
		if !ok || parsed.ID != tc.id || parsed.Upstream != tc.route || parsed.Thinking != tc.level {
			t.Fatalf("%s: %+v", tc.raw, parsed)
		}
	}
	current := Selector{Provider: "openrouter", ID: "google/gemini"}
	if got := ExpandWildcard("google/*", current, lookup); got != "google/gemini" {
		t.Fatal(got)
	}
	if got := ExpandWildcard("openrouter/vendor/*", current, lookup); got != "openrouter/vendor/gemini" {
		t.Fatal(got)
	}
}

// TestEffectiveCandidatesPreserveEffortAndAlias checks exact effort outranks a base
// match and an explicit empty role does not inherit default fallbacks.
func TestEffectiveCandidatesPreserveEffortAndAlias(t *testing.T) {
	c := Config{ModelRoles: map[string]string{"review": "p/m:high", "other": "q/n:off"}}
	chains := map[string][]string{"review": {}, "default": {"r/x"}, "p/m:high": {"q/a"}, "p/m": {"q/b"}}
	if got := ResolveChainKey(c, chains, "review", "p/m:high", nil); got != "p/m:high" {
		t.Fatal(got)
	}
	if got := ResolveChainKey(c, chains, "review", "z/m:high", nil); got != "review" {
		t.Fatal(got)
	}
	got := EffectiveCandidates(c, "p/m:high", "p/m:high", "", []string{"p/m:low", "@other", "q/*"}, nil, false)
	raws := []string{}
	for _, candidate := range got {
		raws = append(raws, candidate.Raw)
	}
	if !reflect.DeepEqual(raws, []string{"p/m:low", "q/n:off", "q/m"}) {
		t.Fatal(raws)
	}
}

// TestAutomaticRolePriority mirrors built-in defaults without live credentials.
func TestAutomaticRolePriority(t *testing.T) {
	pool := []Candidate{{Provider: "anthropic", ID: "claude-haiku-5-5"}, {Provider: "openai-codex", ID: "gpt-5.6-sol"}, {Provider: "image", ID: "render", Kind: "image"}}
	c := Config{}
	for _, tc := range []struct{ selector, want string }{{"@smol", "anthropic/claude-haiku-5-5"}, {"@slow", "openai-codex/gpt-5.6-sol"}, {"@advisor", "openai-codex/gpt-5.6-sol"}, {"@widget", "anthropic/claude-haiku-5-5"}} {
		got, err := c.ResolveAvailable(tc.selector, "", pool)
		if err != nil || got != tc.want {
			t.Fatalf("%s=%s %v", tc.selector, got, err)
		}
	}
	c.ModelRoles = map[string]string{"default": "anthropic/claude-haiku-5-5", "review": "@default:off"}
	if got, err := c.ResolveAvailable("@review", "", pool); err != nil || got != "anthropic/claude-haiku-5-5:off" {
		t.Fatal(got, err)
	}
	if _, err := c.ResolveAvailable("anthropic/claude-haiku", "", pool); err == nil {
		t.Fatal("qualified near match accepted")
	}
}

// TestCompactionPoints shares exact parsing/threshold rules with menu and runtime.
func TestCompactionPoints(t *testing.T) {
	for _, tc := range []struct {
		input  string
		tokens int
	}{{"90k", 73616}, {"f90k", 90000}, {"80%", 102400}} {
		point, err := ParseCompactionInput(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := ParseCompactionEntry(point.Raw())
		if err != nil || restored != point {
			t.Fatal(restored, err)
		}
		if got := CompactionTrigger(128000, 16384, 0, 0, point); got != tc.tokens {
			t.Fatalf("%s trigger %d want %d", tc.input, got, tc.tokens)
		}
	}
	for _, input := range []string{"0", "101%", "1.5k", "nope", "f0"} {
		if _, err := ParseCompactionInput(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
}
