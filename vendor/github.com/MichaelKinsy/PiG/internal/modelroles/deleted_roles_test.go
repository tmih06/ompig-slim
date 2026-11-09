package modelroles

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestDeletedBuiltinsStayDeleted rejects both role resolver paths and omits
// deleted roles from discovery/cycling, without blocking direct model selectors.
func TestDeletedBuiltinsStayDeleted(t *testing.T) {
	for _, role := range Builtins() {
		config := Config{ModelRoles: map[string]string{role: "p/a"}, ModelTags: map[string]Tag{role: (Tag{}).WithDeleted(true)}}
		if slices.Contains(config.Known(nil), role) || slices.Contains(config.Order(), role) {
			t.Fatalf("@%s resurrected", role)
		}
		if _, err := config.Resolve("@"+role, "p/a"); err == nil || !strings.Contains(err.Error(), "deleted") {
			t.Fatalf("@%s resolved after deletion: %v", role, err)
		}
		if _, err := config.ResolveAvailable("@"+role, "p/a", []Candidate{{Provider: "p", ID: "a"}}); err == nil {
			t.Fatalf("available @%s resolved after deletion", role)
		}
		if got, err := config.Resolve("p/a", ""); err != nil || got != "p/a" {
			t.Fatal("direct model selection blocked", got, err)
		}
	}
}

// TestDeletedTagLayering preserves authored false markers and makes an explicit
// project assignment revive a global deletion, but not a project deletion.
func TestDeletedTagLayering(t *testing.T) {
	global := Config{ModelRoles: map[string]string{"default": "p/a"}, ModelTags: map[string]Tag{"default": (Tag{}).WithDeleted(true)}}
	for _, project := range []Config{
		{ModelRoles: map[string]string{"default": "p/b"}},
		{ModelTags: map[string]Tag{"default": (Tag{}).WithDeleted(false)}},
	} {
		data, err := json.Marshal(project)
		if err != nil {
			t.Fatal(err)
		}
		var restored Config
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		merged := global.Merge(restored)
		if merged.ModelTags["default"].Deleted || !slices.Contains(merged.Known(nil), "default") {
			t.Fatalf("restoration lost: %s", data)
		}
	}
	project := Config{ModelTags: map[string]Tag{"default": (Tag{}).WithDeleted(true)}}
	if _, err := global.Merge(project).Resolve("@default", "p/a"); err == nil {
		t.Fatal("project deletion did not override global assignment")
	}
	config := Config{ModelTags: map[string]Tag{}}
	for _, role := range []string{"smol", "default", "slow"} {
		config.ModelTags[role] = (Tag{}).WithDeleted(true)
	}
	if order := config.Order(); len(order) != 0 {
		t.Fatal("deleted default cycle returned", order)
	}
}
