package tui

import "testing"

func settingsFixture() []SettingItem {
	return []SettingItem{
		{ID: "theme", Label: "Theme", Description: "Color theme", CurrentValue: "dark", Values: []string{"dark", "light"}},
		{ID: "editor", Label: "Editor", Description: "External editor", CurrentValue: "vi"},
		{ID: "tokens", Label: "Token budget", Description: "Token display", CurrentValue: "on"},
		{ID: "mouse", Label: "Mouse mode", Description: "Mouse reporting", CurrentValue: "off"},
		{ID: "quiet", Label: "Quiet startup", Description: "Startup notice", CurrentValue: "off"},
	}
}

// TestSettingsSearchSelectsBestMatch checks that a query moves the highlight to
// the top match instead of keeping the row the arrows scrolled to.
func TestSettingsSearchSelectsBestMatch(t *testing.T) {
	list := NewSettingsList(settingsFixture(), 10, GetSettingsListTheme(), nil, nil, SettingsListOptions{EnableSearch: true})

	list.HandleInput("\x1b[B")
	list.HandleInput("\x1b[B")
	if got := list.items[list.displayItems()[list.cursor]].ID; got != "tokens" {
		t.Fatalf("after two Downs the selection is %q, want tokens", got)
	}
	list.HandleInput("theme")
	if got := list.items[list.displayItems()[list.cursor]].ID; got != "theme" {
		t.Fatalf("query left the selection on %q, want theme", got)
	}
}

// TestSettingsSearchCursorKeysKeepSelection checks that moving the caret inside
// the search field keeps the row the arrows selected.
func TestSettingsSearchCursorKeysKeepSelection(t *testing.T) {
	list := NewSettingsList(settingsFixture(), 10, GetSettingsListTheme(), nil, nil, SettingsListOptions{EnableSearch: true})

	list.HandleInput("e")
	list.HandleInput("\x1b[B")
	selected := list.items[list.displayItems()[list.cursor]].ID
	if selected == list.items[list.displayItems()[0]].ID {
		t.Fatalf("Down did not move the selection past the top row: %q", selected)
	}
	for _, key := range []string{"\x1b[D", "\x1b[C", "\x1b[H", "\x1b[F"} {
		list.HandleInput(key)
		if got := list.items[list.displayItems()[list.cursor]].ID; got != selected {
			t.Fatalf("cursor key %q moved the selection from %q to %q", key, selected, got)
		}
	}
}
