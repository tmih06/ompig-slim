package codingagent

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestModelRoleSettingsRoundTrip exercises custom JSON, both scopes, and detached snapshots.
func TestModelRoleSettingsRoundTrip(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"modelRoles":{"default":"p/a:high","review":"p/b"},"cycleOrder":[],"modelTags":{"review":{"name":"Reviewer","color":"accent"}},"retry":{"fallbackChains":{"default":["q/a"],"review":[]}}}`), &s); err != nil {
		t.Fatal(err)
	}
	sm := NewInMemorySettingsManager(s)
	if err := sm.SetModelRole("review", "q/b:off", "project"); err != nil {
		t.Fatal(err)
	}
	if got, err := sm.ResolveModelRole("@review"); err != nil || got != "q/b:off" {
		t.Fatal(got, err)
	}
	if got, err := sm.ResolveModelRole("@default"); err != nil || got != "p/a:high" {
		t.Fatal(got, err)
	}
	chains := sm.ModelFallbackChains()
	if len(chains["review"]) != 0 || !reflect.DeepEqual(chains["smol"], []string{"q/a"}) {
		t.Fatal(chains)
	}
	if _, ok := chains["web"]; ok {
		t.Fatal("kind role inherited chat fallback")
	}
	chains["default"][0] = "mutated"
	if sm.ModelFallbackChains()["default"][0] != "q/a" {
		t.Fatal("shared chain")
	}
	copy := sm.Get()
	copy.ModelRoles["default"] = "mutated"
	if sm.Get().ModelRoles["default"] != "p/a:high" {
		t.Fatal("shared roles")
	}
	data, err := json.Marshal(sm.Get())
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Config, sm.Get().Config) || restored.CycleOrder == nil {
		t.Fatalf("round trip: %s", data)
	}
	if err = sm.SetFallbackChain("review", []string{}, "project"); err != nil {
		t.Fatal(err)
	}
	if chain, ok := sm.ModelFallbackChains()["review"]; !ok || len(chain) != 0 {
		t.Fatal("lost empty opt-out")
	}
}

// TestModelRoleAliasCycleRefused keeps every saved alias resolvable. Assigning a
// role to another is now an ordinary manager action, so a two-click loop must be
// refused at the closing write; Resolve's own check only covers hand-edited files.
func TestModelRoleAliasCycleRefused(t *testing.T) {
	sm := NewInMemorySettingsManager(Settings{})
	if err := sm.SetModelRole("default", "p/a", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetModelRole("task", "@default", "global"); err != nil {
		t.Fatal(err)
	}
	if got, err := sm.ResolveModelRole("@task"); err != nil || got != "p/a" {
		t.Fatal(got, err)
	}
	// The closing write is refused and leaves the accepted assignment intact.
	if err := sm.SetModelRole("default", "@task", "global"); err == nil {
		t.Fatal("mutual alias accepted")
	}
	if got, err := sm.ResolveModelRole("@default"); err != nil || got != "p/a" {
		t.Fatal("default left unresolvable", got, err)
	}
	if err := sm.SetModelRole("audit", "@audit", "global"); err == nil {
		t.Fatal("self alias accepted")
	}
	// Following a role that is itself inherited counts as an edge, so the closing
	// write of a two-role loop is refused whichever side completes it.
	if err := sm.SetModelRole("smol", "p/b", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetModelRole("task", "@smol", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetModelRole("smol", "@task", "global"); err == nil {
		t.Fatal("alias cycle accepted")
	}
	// Clearing a role is always allowed, and the freed role is then re-usable.
	if err := sm.SetModelRole("task", "", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetModelRole("smol", "@task", "global"); err != nil {
		t.Fatal(err)
	}
}

// TestFallbackChainRoleCycleRefused keeps a saved chain terminating: a write that
// would close a role-reference loop is refused before any layer changes, while
// model-keyed chains stay legal because they never expand into another chain.
func TestFallbackChainRoleCycleRefused(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	sm := NewSettingsManagerWithProjectTrust(cwd, dir, true)
	if err := sm.SetFallbackChain("review", []string{"@slow"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("slow", []string{"openai/other"}, "global"); err != nil {
		t.Fatal(err)
	}
	// A self reference closes a loop; a mixed chain that only references an
	// acyclic role does not, so the same call shape is accepted below.
	if err := sm.SetFallbackChain("review", []string{"@review"}, "global"); err == nil {
		t.Fatal("self reference accepted")
	}
	if err := sm.SetFallbackChain("slow", []string{"@review"}, "global"); err == nil {
		t.Fatal("mutual reference accepted")
	}
	if got := sm.GetGlobalSettings().Retry.FallbackChains["slow"]; !reflect.DeepEqual(got, []string{"openai/other"}) {
		t.Fatalf("refused write mutated slow to %v", got)
	}
	// The merged view refuses a chain that closes a loop existing only because a
	// project entry references a global one, whichever layer completes it.
	if err := sm.SetFallbackChain("slow", []string{"@review"}, "project"); err == nil {
		t.Fatal("cross-layer loop accepted")
	}
	// A model-keyed chain has no role edges, and a role that is acyclic stays legal.
	if err := sm.SetFallbackChain("openai/other", []string{"@review"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("openai/other", nil, "global"); err != nil {
		t.Fatal(err)
	}
	// The default tail is inherited by every chat role, so pointing default at a
	// role and that role back at default is a loop even though no single role key
	// repeats in one chain.
	if err := sm.SetFallbackChain("default", []string{"@audit"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("audit", []string{"@default"}, "global"); err == nil {
		t.Fatal("default-tail loop accepted")
	}
	if err := sm.SetFallbackChain("audit", []string{"openai/x"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("review", []string{"openai/x", "@audit:off"}, "global"); err != nil {
		t.Fatal(err)
	}
	// Removing an edge is always allowed, and the freed role is then re-usable.
	if err := sm.SetFallbackChain("slow", nil, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("review", []string{"openai/x"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("slow", []string{"@review"}, "project"); err != nil {
		t.Fatal(err)
	}
	if got := sm.ModelFallbackChains()["review"]; !reflect.DeepEqual(got, []string{"openai/x"}) {
		t.Fatal("accepted write lost", got)
	}
}

// TestModelRoleFilePersistence verifies real save/reload and rejects untrusted writes.
func TestModelRoleFilePersistence(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	sm := NewSettingsManager(cwd, dir)
	if err := sm.SetModelRole("review", "openai/model:high", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("review", []string{"other/model:off"}, "global"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewSettingsManager(cwd, dir)
	if got, err := reloaded.ResolveModelRole("@review"); err != nil || got != "openai/model:high" {
		t.Fatal(got, err)
	}
	if got := reloaded.ModelFallbackChains()["review"]; len(got) != 1 || got[0] != "other/model:off" {
		t.Fatal(got)
	}
	data, err := os.ReadFile(sm.GlobalPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	untrusted := NewSettingsManagerWithProjectTrust(cwd, dir, false)
	if err = untrusted.SetModelRole("review", "other/model", "project"); err == nil {
		t.Fatal("untrusted project write succeeded")
	}
}

// TestModelRolesCustomOrder includes custom roles introduced only by chains or tags.
func TestModelRolesCustomOrder(t *testing.T) {
	c := modelroles.Config{ModelTags: map[string]modelroles.Tag{"review": {Name: "Review"}}}
	known := c.Known(map[string][]string{"audit": {}})
	if known[len(known)-2] != "audit" || known[len(known)-1] != "review" {
		t.Fatal(known)
	}
	sm := NewInMemorySettingsManager(Settings{})
	if err := sm.SetModelRole("bad/name", "p/a", "global"); err == nil {
		t.Fatal("invalid role accepted")
	}
	if got := sm.ModelRoleCycle(); !reflect.DeepEqual(got, []string{"smol", "default", "slow"}) {
		t.Fatal("OMP default cycle", got)
	}
	if err := sm.SetRoleCycleOrder([]string{}, "global"); err != nil {
		t.Fatal(err)
	}
	if got := sm.ModelRoleCycle(); got == nil || len(got) != 0 {
		t.Fatal("empty cycle must disable cycling", got)
	}
}
