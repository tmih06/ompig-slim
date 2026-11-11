package modelroles

import (
	"reflect"
	"slices"
	"testing"
)

// TestResolve covers aliases, effort precedence, inheritance, and cycle rejection.
func TestResolve(t *testing.T) {
	c := Config{ModelRoles: map[string]string{"default": "openai/main:high", "compact": "openai/compact", "review": "@default:low", "a": "@b", "b": "@a"}}
	for _, tc := range []struct{ input, want string }{{"*", "openai/main:high"}, {"pi/default", "openai/main:high"}, {"@compact", "openai/compact"}, {"@review", "openai/main:low"}, {"@review:off", "openai/main:off"}, {"@memory", "openai/main:high"}, {"openrouter/vendor/model:nitro", "openrouter/vendor/model:nitro"}} {
		got, err := c.Resolve(tc.input, "")
		if err != nil || got != tc.want {
			t.Errorf("%s = %q, %v; want %s", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"@a", "@unknown"} {
		if _, err := c.Resolve(input, ""); err == nil {
			t.Errorf("%s should fail", input)
		}
	}
}

func TestCompactIsBuiltinRole(t *testing.T) {
	if !slices.Contains(Builtins(), "compact") {
		t.Fatal("compact role is not built in")
	}
}

// TestChainSpecificity matches OMP's exact / longest wildcard / role / default ordering.
func TestChainSpecificity(t *testing.T) {
	chains := map[string][]string{"default": {"p/d"}, "review": {}, "p/*": {"q/*"}, "p/vendor/*": {"q/x"}, "p/vendor/m": {"q/y"}}
	for _, tc := range []struct{ role, model, want string }{{"review", "p/vendor/m", "p/vendor/m"}, {"review", "p/vendor/n", "p/vendor/*"}, {"review", "p/x", "p/*"}, {"review", "other/x", "review"}, {"missing", "other/x", "default"}} {
		if got := ChainKey(chains, tc.role, tc.model); got != tc.want {
			t.Errorf("%+v = %s", tc, got)
		}
	}
}

// TestCandidates ensures a chain never wraps and preserves per-entry thinking.
func TestCandidates(t *testing.T) {
	chain := []string{"q/b:low", "q/b:low", "r/*", "s/c"}
	for _, tc := range []struct {
		current string
		want    []string
	}{{"p/a", []string{"q/b:low", "r/a", "s/c"}}, {"q/b:high", []string{"r/b", "s/c"}}, {"s/c", []string{}}} {
		got := Candidates("p/a", tc.current, chain)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %#v, want %#v", tc.current, got, tc.want)
		}
	}
}

// TestMergeOwnership protects settings snapshots and explicit empty cycle order.
func TestMergeOwnership(t *testing.T) {
	base := Config{ModelRoles: map[string]string{"default": "p/a", "review": "p/b"}, CycleOrder: []string{"default"}}
	merged := base.Merge(Config{ModelRoles: map[string]string{"review": "q/c"}, CycleOrder: []string{}})
	merged.ModelRoles["default"] = "changed"
	if base.ModelRoles["default"] != "p/a" || merged.ModelRoles["review"] != "q/c" || merged.Order() == nil || len(merged.Order()) != 0 {
		t.Fatal("merge lost ownership or empty cycle order", base, merged)
	}
}
