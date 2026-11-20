package tui

import "testing"

func configFixtureGroups() []*ResourceGroup {
	items := func(names ...string) []*ResourceItem {
		out := make([]*ResourceItem, 0, len(names))
		for _, name := range names {
			out = append(out, &ResourceItem{
				Path: name, DisplayName: name, ResourceType: ResourceExtensions,
				GroupKey: "user", SubgroupKey: "Extensions", Scope: "user", Origin: "top-level",
			})
		}
		return out
	}
	return []*ResourceGroup{{
		Key: "user", Label: "User", Scope: "user", Origin: "top-level",
		Subgroups: []*ResourceSubgroup{{
			Type: ResourceExtensions, Label: "Extensions",
			Items: items("verify", "lint", "audit", "review", "deploy"),
		}},
	}}
}

func configSelectedItem(cs *ConfigSelectorComponent) string {
	if cs.cursor < 0 || cs.cursor >= len(cs.filtered) {
		return ""
	}
	if entry := cs.filtered[cs.cursor]; entry.item != nil {
		return entry.item.DisplayName
	}
	return ""
}

// TestConfigSearchSelectsBestMatch checks that a query moves the highlight to
// the first matching item instead of keeping the row the arrows scrolled to.
func TestConfigSearchSelectsBestMatch(t *testing.T) {
	cs := NewConfigSelector(configFixtureGroups(), 30)

	cs.HandleInput("\x1b[B")
	cs.HandleInput("\x1b[B")
	if got := configSelectedItem(cs); got != "audit" {
		t.Fatalf("after two Downs the selection is %q, want audit", got)
	}
	cs.HandleInput("verify")
	if got := configSelectedItem(cs); got != "verify" {
		t.Fatalf("query left the selection on %q, want verify", got)
	}
}

// TestConfigSearchCursorKeysKeepSelection checks that moving the caret inside
// the config selector's search field keeps the row the arrows selected.
func TestConfigSearchCursorKeysKeepSelection(t *testing.T) {
	cs := NewConfigSelector(configFixtureGroups(), 30)

	cs.HandleInput("e")
	cs.HandleInput("\x1b[B")
	selected := configSelectedItem(cs)
	if selected == "" || selected == "verify" {
		t.Fatalf("Down did not move the selection past the first matching item: %q", selected)
	}
	for _, key := range []string{"\x1b[D", "\x1b[C", "\x1b[H", "\x1b[F"} {
		cs.HandleInput(key)
		if got := configSelectedItem(cs); got != selected {
			t.Fatalf("cursor key %q moved the selection from %q to %q", key, selected, got)
		}
	}
}
