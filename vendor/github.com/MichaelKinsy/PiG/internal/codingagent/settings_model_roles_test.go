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
