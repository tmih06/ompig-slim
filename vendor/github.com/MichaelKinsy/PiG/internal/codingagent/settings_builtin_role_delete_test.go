package codingagent

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestDeleteBuiltinRoleRestart checks every builtin's persisted deletion and
// explicit restoration; removing authored fields alone would resurrect defaults.
func TestDeleteBuiltinRoleRestart(t *testing.T) {
	for _, role := range modelroles.Builtins() {
		t.Run(role, func(t *testing.T) {
			cwd, dir := t.TempDir(), t.TempDir()
			sm := NewSettingsManager(cwd, dir)
			if err := sm.UpdateGlobal(func(s *Settings) {
				s.ModelRoles = map[string]string{role: "p/a"}
				s.ModelTags = map[string]modelroles.Tag{role: {Name: "Remove me"}}
				s.CycleOrder = []string{role}
				s.Retry = &RetrySettingsJSON{FallbackChains: map[string][]string{role: {"p/b"}}}
			}); err != nil {
				t.Fatal(err)
			}
			if err := sm.DeleteModelRole(role, "global"); err != nil {
				t.Fatal(err)
			}
			sm = NewSettingsManager(cwd, dir)
			s := sm.Get()
			if !s.ModelTags[role].Deleted || s.ModelTags[role].Name != "" || s.ModelRoles[role] != "" || len(s.CycleOrder) != 0 || slices.Contains(s.Config.Known(sm.ModelFallbackChains()), role) {
				t.Fatal("built-in deletion did not survive restart", s.Config)
			}
			if _, exists := sm.ModelFallbackChains()[role]; exists {
				t.Fatal("deleted fallback survived")
			}
			if err := sm.SetModelRole(role, "p/restored", "global"); err != nil {
				t.Fatal(err)
			}
			sm = NewSettingsManager(cwd, dir)
			if got, err := sm.ResolveModelRole("@" + role); err != nil || got != "p/restored" {
				t.Fatalf("restoration = %s, %v", got, err)
			}
		})
	}
}

// TestDeleteBuiltinRoleLayers verifies project deletion suppresses inherited
// chains/assignments, and a later project assignment can restore a global deletion.
func TestDeleteBuiltinRoleLayers(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	sm := NewSettingsManager(cwd, dir)
	if err := sm.SetModelRole("default", "p/global", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetFallbackChain("default", []string{"p/fallback"}, "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.DeleteModelRole("default", "project"); err != nil {
		t.Fatal(err)
	}
	if sm.GetGlobalSettings().ModelRoles["default"] != "p/global" {
		t.Fatal("project deletion changed global assignment")
	}
	if _, err := sm.ResolveModelRole("@default"); err == nil {
		t.Fatal("deleted project role resolved through global")
	}
	if _, ok := sm.ModelFallbackChains()["default"]; ok {
		t.Fatal("deleted inherited chain still active")
	}
	if err := sm.DeleteModelRole("default", "global"); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetModelRole("default", "p/project", "project"); err != nil {
		t.Fatal(err)
	}
	sm = NewSettingsManager(cwd, dir)
	if got, err := sm.ResolveModelRole("@default"); err != nil || got != "p/project" {
		t.Fatalf("explicit false restoration lost: %s %v", got, err)
	}
	if !sm.GetGlobalSettings().ModelTags["default"].Deleted {
		t.Fatal("project restoration changed global deletion")
	}
	// The project now masks the global deletion. Explicitly assigning the
	// global layer must still clear its own marker, not just the merged one.
	if err := sm.SetModelRole("default", "p/global-restored", "global"); err != nil {
		t.Fatal(err)
	}
	sm = NewSettingsManager(cwd, dir)
	if sm.GetGlobalSettings().ModelTags["default"].Deleted || sm.Get().ModelRoles["default"] != "p/project" {
		t.Fatal("masked global deletion was not restored independently")
	}
}
