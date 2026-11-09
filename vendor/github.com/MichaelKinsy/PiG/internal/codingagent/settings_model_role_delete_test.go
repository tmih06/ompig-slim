package codingagent

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestDeleteModelRolePersistsAllFields covers one-layer cleanup, duplicate cycle
// entries, lossless unrelated fields, lower-layer reveal, and restart behavior.
func TestDeleteModelRolePersistsAllFields(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	sm := NewSettingsManager(cwd, dir)
	if err := os.WriteFile(sm.GlobalPath(), []byte(`{"modelRoles":{"review":"p/a","keep":"p/b"},"modelTags":{"review":{"name":"Reviewer","future":true}},"cycleOrder":["review","keep","review"],"retry":{"fallbackChains":{"review":["@review"],"keep":["p/c"],"p/*":["p/d"]}},"futureSetting":{"keep":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	sm = NewSettingsManager(cwd, dir)
	if err := sm.SetModelRole("review", "p/project", "project"); err != nil {
		t.Fatal(err)
	}
	if err := sm.DeleteModelRole("review", "global"); err != nil {
		t.Fatal(err)
	}
	global := sm.GetGlobalSettings()
	if _, ok := global.ModelRoles["review"]; ok {
		t.Fatal("assignment survives")
	}
	if _, ok := global.ModelTags["review"]; ok {
		t.Fatal("tag survives")
	}
	if _, ok := global.Retry.FallbackChains["review"]; ok {
		t.Fatal("fallback survives")
	}
	if !reflect.DeepEqual(global.CycleOrder, []string{"keep"}) || global.ModelRoles["keep"] != "p/b" || global.Retry.FallbackChains["p/*"][0] != "p/d" {
		t.Fatal("unrelated fields changed", global)
	}
	reloaded := NewSettingsManager(cwd, dir)
	if reloaded.Get().ModelRoles["review"] != "p/project" {
		t.Fatal("project override was deleted")
	}
	if err := reloaded.DeleteModelRole("review", "project"); err != nil {
		t.Fatal(err)
	}
	reloaded = NewSettingsManager(cwd, dir)
	if slices.Contains(reloaded.Get().Config.Known(reloaded.ModelFallbackChains()), "review") {
		t.Fatal("deleted role reappeared after restart")
	}
	if err := reloaded.DeleteModelRole("review", "global"); err != nil {
		t.Fatal("repeated deletion", err)
	}
	data, err := os.ReadFile(sm.GlobalPath())
	if err != nil || !strings.Contains(string(data), `"futureSetting"`) || strings.Contains(string(data), `"review"`) {
		t.Fatalf("persisted cleanup: %s, %v", data, err)
	}
}

// TestDeleteModelRoleGuards ensures invalid names, inbound aliases and
// untrusted project settings fail without removing any authored role data.
func TestDeleteModelRoleGuards(t *testing.T) {
	for _, role := range []string{"bad/name", ""} {
		sm := NewInMemorySettingsManager(Settings{})
		if err := sm.DeleteModelRole(role, "global"); err == nil {
			t.Fatalf("accepted invalid role %q", role)
		}
	}
	for _, selector := range []string{"@review", "@review:high", "pi/review:off"} {
		for _, inChain := range []bool{false, true} {
			s := Settings{Config: modelroles.Config{ModelRoles: map[string]string{"review": "p/a"}}}
			if inChain {
				s.Retry = &RetrySettingsJSON{FallbackChains: map[string][]string{"p/*": {selector}}}
			} else {
				s.ModelRoles["keep"] = selector
			}
			sm := NewInMemorySettingsManager(s)
			before := sm.Get()
			if err := sm.DeleteModelRole("review", "global"); err == nil || !strings.Contains(err.Error(), "referenced") {
				t.Fatalf("accepted inbound alias %q: %v", selector, err)
			}
			if !reflect.DeepEqual(before, sm.Get()) {
				t.Fatal("rejected deletion mutated settings")
			}
		}
	}
	sm := NewSettingsManagerWithProjectTrust(t.TempDir(), t.TempDir(), false)
	if err := sm.DeleteModelRole("review", "project"); err == nil {
		t.Fatal("untrusted project accepted")
	}
	if err := sm.DeleteModelRole("review", "typo"); err == nil {
		t.Fatal("invalid scope accepted")
	}
}

// TestDeleteModelRolePreservesCycleSemantics keeps nil defaults and authored
// empty cycles distinct, including deleting the last explicit cycle entry.
func TestDeleteModelRolePreservesCycleSemantics(t *testing.T) {
	for _, cycle := range [][]string{nil, {}, {"review"}} {
		sm := NewInMemorySettingsManager(Settings{Config: modelroles.Config{CycleOrder: cycle}})
		if err := sm.DeleteModelRole("review", "global"); err != nil {
			t.Fatal(err)
		}
		got := sm.Get().CycleOrder
		if len(got) != 0 || (got == nil) != (cycle == nil) {
			t.Fatalf("cycle %v became %v", cycle, got)
		}
	}
}
