package modelroles

import (
	"reflect"
	"slices"
	"testing"
)

// TestCycleOrderRepairsDanglingRoles covers old scoped-N entries without changing
// intentional empty, partial or valid-but-unavailable user cycle selections.
func TestCycleOrderRepairsDanglingRoles(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cycle, want []string
	}{
		{"implicit", nil, []string{"smol", "default", "slow"}},
		{"disabled", []string{}, []string{}},
		{"removed scopes", []string{"scoped-1", "scoped-2", "scoped-3", "scoped-4"}, []string{"smol", "default"}},
		{"partial", []string{"scoped-1", "default"}, []string{"default"}},
		{"explicit", []string{"default", "smol"}, []string{"default", "smol"}},
		{"unavailable", []string{"audit"}, []string{"audit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{CycleOrder: tc.cycle, ModelRoles: map[string]string{"default": "p/a", "smol": "p/b"}, ModelTags: map[string]Tag{"audit": {Name: "Audit"}}}
			before := config.Clone()
			got := config.Order()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("order = %v, want %v", got, tc.want)
			}
			if len(got) > 0 {
				got[0] = "mutated"
			}
			if !reflect.DeepEqual(before, config) {
				t.Fatal("read mutated settings")
			}
			if tc.name == "removed scopes" && slices.Contains(config.Known(nil), "scoped-1") {
				t.Fatal("removed cycle role reappeared in manager")
			}
		})
	}
}

// TestDanglingCycleRecoveryKinds excludes non-chat roles and orders custom chat
// assignments deterministically; no assignments falls back to built-in defaults.
func TestDanglingCycleRecoveryKinds(t *testing.T) {
	config := Config{CycleOrder: []string{"removed"}, ModelRoles: map[string]string{
		"default": "p/a", "smol": "p/b", "image": "p/image", "zebra": "p/z", "audit": "p/c",
	}}
	if got := config.Order(); !reflect.DeepEqual(got, []string{"smol", "default", "audit", "zebra"}) {
		t.Fatal(got)
	}
	config.ModelRoles = nil
	if got := config.Order(); !reflect.DeepEqual(got, []string{"smol", "default", "slow"}) {
		t.Fatal(got)
	}
}
