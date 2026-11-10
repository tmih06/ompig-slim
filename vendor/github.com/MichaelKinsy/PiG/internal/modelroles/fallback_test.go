package modelroles

import (
	"reflect"
	"slices"
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

// TestChainEntriesExpandRoleDestinations covers role references as chain entries:
// a role contributes its own chain after itself, order is depth-first and stable,
// and a loop stops instead of recursing. A bare fragment that shares a role's name
// is a model, not a reference.
func TestChainEntriesExpandRoleDestinations(t *testing.T) {
	chains := map[string][]string{
		"review": {"p/primary", "@slow:off", "q/tail"},
		"slow":   {"p/thinking", "@tiny"},
		"tiny":   {"p/tiny"},
		"bare":   {"review"},
	}
	if got, want := ChainEntries(chains, "review"), []string{"p/primary", "@slow:off", "p/thinking", "@tiny", "p/tiny", "q/tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if got := ChainEntries(chains, "bare"); !reflect.DeepEqual(got, []string{"review"}) {
		t.Fatalf("bare fragment expanded as a role: %v", got)
	}
	looped := map[string][]string{"review": {"@slow"}, "slow": {"@review"}}
	if got := ChainEntries(looped, "review"); !reflect.DeepEqual(got, []string{"@slow", "@review"}) {
		t.Fatalf("loop did not terminate: %v", got)
	}
	// A loop that exists only in the inherited default tail still terminates, and
	// the entry that closes it is kept so the referenced role's own model is tried.
	inherited := map[string][]string{"default": {"@audit"}, "review": {"@audit"}, "audit": {"@review"}}
	if got := ChainEntries(inherited, "review"); !reflect.DeepEqual(got, []string{"@audit", "@review"}) {
		t.Fatalf("inherited loop did not terminate: %v", got)
	}
}

// TestAssignableRolesFollowsAliases covers primary assignment, which is a
// different graph from a chain's role references: only ModelRoles aliases and the
// built-in inheritance Resolve applies are edges. A role whose assignment reaches
// the target would make both roles unresolvable, so it is never offered.
func TestAssignableRolesFollowsAliases(t *testing.T) {
	c := Config{ModelRoles: map[string]string{"default": "p/a", "review": "@default", "audit": "p/c", "loop": "@other", "other": "@loop"}}
	// task inherits @default without an authored entry, so it reaches default.
	got := AssignableRoles(c, nil, "default")
	if slices.Contains(got, "task") || slices.Contains(got, "review") || slices.Contains(got, "default") {
		t.Fatalf("alias-closing roles offered: %v", got)
	}
	if !slices.Contains(got, "audit") {
		t.Fatalf("eligible role missing: %v", got)
	}
	// A role always reaches itself, so no role is offered for its own primary, and
	// a cycle already saved in settings must not make the walk hang.
	if got := AssignableRoles(c, nil, "audit"); !slices.Contains(got, "loop") {
		t.Fatalf("stored cycle escaped: %v", got)
	}
	if got := AssignableRoles(c, nil, "loop"); slices.Contains(got, "other") || slices.Contains(got, "loop") {
		t.Fatalf("stored cycle not excluded: %v", got)
	}
	// An alias carrying an effort suffix still names the role it points at, so the
	// edge exists: Resolve strips that suffix the same way, and a guard that did not
	// would let `default: '@task:high'` close a loop the manager shows as unpickable.
	c.ModelRoles["task"], c.ModelRoles["audit"] = "@default:high", "p/d"
	if !AliasReaches(c, "task", "default") {
		t.Fatal("effort-suffixed alias edge missed")
	}
}

// TestChainReachesFindsLoops is the write-time guard: appending @from to target's
// chain closes a loop exactly when from already reaches target. A role reaches
// itself, a stored loop terminates, and a model or provider entry ends the walk
// because only a role entry expands into another chain.
func TestChainReachesFindsLoops(t *testing.T) {
	chains := map[string][]string{"review": {"@slow"}, "slow": {"@tiny"}, "tiny": {"@audit"}, "audit": {"p/m", "@review"}}
	for _, tc := range []struct {
		from, target string
		want         bool
	}{{"slow", "review", true}, {"tiny", "review", true}, {"audit", "review", true}, {"review", "review", true}, {"tiny", "tiny", true}, {"audit", "orphan", false}, {"orphan", "review", false}} {
		if got := ChainReaches(chains, tc.from, tc.target); got != tc.want {
			t.Fatalf("%s→%s: %v want %v", tc.from, tc.target, got, tc.want)
		}
	}
	looped := map[string][]string{"a": {"@b"}, "b": {"@a"}}
	if ChainReaches(looped, "a", "missing") {
		t.Fatal("stored loop escaped")
	}
	// A bare model fragment that names a role is not a reference on the other side
	// of the edge either: "review" here is a model id, so @audit reaches nothing.
	if ChainReaches(map[string][]string{"audit": {"review"}}, "audit", "review") {
		t.Fatal("bare fragment treated as a reference")
	}
}

// TestFallbackRolesExcludesCycles keeps the picker and the write guard in
// agreement: a role the list omits is exactly a role whose entry would be refused.
func TestFallbackRolesExcludesCycles(t *testing.T) {
	c := Config{ModelRoles: map[string]string{"default": "p/a", "review": "p/a", "slow": "p/b", "audit": "p/c"}}
	chains := map[string][]string{"slow": {"@review"}, "audit": {"p/c"}}
	got := FallbackRoles(c, chains, "review")
	if slices.Contains(got, "review") || slices.Contains(got, "slow") {
		t.Fatal("loop-closing roles offered", got)
	}
	if !slices.Contains(got, "audit") || !slices.Contains(got, "default") {
		t.Fatal("eligible roles missing", got)
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
