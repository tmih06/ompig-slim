package codingagent

import (
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestLegacyScopeWithRoleCycle migrates stale filters even when a previous
// role edit already created cycleOrder, preserving assignments, effort and order.
func TestLegacyScopeWithRoleCycle(t *testing.T) {
	for _, cycle := range [][]string{nil, {}, {"review", "scoped-1"}} {
		t.Run("cycle="+stringifyCycle(cycle), func(t *testing.T) {
			sm := NewInMemorySettingsManager(Settings{
				EnabledModels: []string{"p/a:high", "p/b"},
				Config:        modelroles.Config{ModelRoles: map[string]string{"review": "p/r", "scoped-1": "p/a:high", "scoped-2": "p/keep"}, CycleOrder: cycle},
			})
			choices := []ScopedModel{{Model: RuntimeModel{Provider: "p", ID: "a"}, ThinkingLevel: "high"}, {Model: RuntimeModel{Provider: "p", ID: "b"}}}
			migrated, err := sm.ImportLegacyModelScope(choices)
			if err != nil || !migrated {
				t.Fatalf("migration = %v, %v", migrated, err)
			}
			got := sm.Get()
			if got.EnabledModels == nil || len(got.EnabledModels) != 0 {
				t.Fatalf("legacy filter survives: %v", got.EnabledModels)
			}
			if got.ModelRoles["scoped-1"] != "p/a:high" || got.ModelRoles["scoped-2"] != "p/keep" || got.ModelRoles["scoped-2-2"] != "p/b" || got.ModelRoles["review"] != "p/r" {
				t.Fatal(got.ModelRoles)
			}
			if cycle != nil {
				want := []string{}
				if len(cycle) > 0 {
					want = append(append([]string{}, cycle...), "scoped-2-2")
				}
				if !reflect.DeepEqual(got.CycleOrder, want) {
					t.Fatalf("cycle = %v, want %v", got.CycleOrder, want)
				}
			}
			if again, err := sm.ImportLegacyModelScope(choices); err != nil || again {
				t.Fatalf("not idempotent: %v, %v", again, err)
			}
		})
	}
}

// stringifyCycle labels nil and explicit empty cycles separately in test output.
func stringifyCycle(cycle []string) string {
	if cycle == nil {
		return "nil"
	}
	if len(cycle) == 0 {
		return "empty"
	}
	return "existing"
}
