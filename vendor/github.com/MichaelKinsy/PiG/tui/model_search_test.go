package tui

import "testing"

func browserFixture() *ModelBrowser {
	items := []ModelSelectorItem{
		{Kind: "chat", Provider: "fixture", ID: "alpha"},
		{Kind: "chat", Provider: "fixture", ID: "beta"},
		{Kind: "chat", Provider: "fixture", ID: "gamma"},
		{Kind: "chat", Provider: "fixture", ID: "delta"},
	}
	browser := NewModelBrowser("")
	browser.SetItems(items)
	browser.SetQuery("a")
	return browser
}

// TestModelBrowserSearchSelectsBestMatch checks that a typed query moves the
// highlight to the first match instead of keeping the row the arrows reached.
func TestModelBrowserSearchSelectsBestMatch(t *testing.T) {
	browser := browserFixture()

	browser.HandleInput("\x1b[B")
	browser.HandleInput("\x1b[B")
	if got := browser.Selected().ID; got == browser.items[browser.filtered[0]].ID {
		t.Fatalf("Down did not move the selection past the top row: %q", got)
	}
	browser.HandleInput("lph")
	if got := browser.Selected().ID; got != "alpha" {
		t.Fatalf("query left the selection on %q, want alpha", got)
	}
}

// TestModelBrowserSearchCursorKeysKeepSelection checks that moving the caret
// inside the model search keeps the row the arrows selected.
func TestModelBrowserSearchCursorKeysKeepSelection(t *testing.T) {
	browser := browserFixture()

	browser.HandleInput("\x1b[B")
	selected := browser.Selected().ID
	if selected == browser.items[browser.filtered[0]].ID {
		t.Fatalf("Down did not move the selection past the top row: %q", selected)
	}
	for _, key := range []string{"\x1b[D", "\x1b[C", "\x1b[H", "\x1b[F"} {
		browser.HandleInput(key)
		if got := browser.Selected().ID; got != selected {
			t.Fatalf("cursor key %q moved the selection from %q to %q", key, selected, got)
		}
	}
}

// TestSelectSubmenuSearchCursorKeysKeepSelection checks the per-model settings
// model picker: caret movement inside its search keeps the selected row.
func TestSelectSubmenuSearchCursorKeysKeepSelection(t *testing.T) {
	items := []SelectItem{
		{Value: "one", Label: "one"},
		{Value: "two", Label: "two"},
		{Value: "three", Label: "three"},
		{Value: "four", Label: "four"},
	}
	submenu := NewSelectSubmenu("Pick", "", items, "one", SelectSubmenuOptions{Searchable: true})

	submenu.HandleInput("o")
	submenu.HandleInput("\x1b[B")
	selected := submenu.CurrentValue()
	if selected == "" || selected == "one" {
		t.Fatalf("Down did not move the selection past the top row: %q", selected)
	}
	for _, key := range []string{"\x1b[D", "\x1b[C", "\x1b[H", "\x1b[F"} {
		submenu.HandleInput(key)
		if got := submenu.CurrentValue(); got != selected {
			t.Fatalf("cursor key %q moved the selection from %q to %q", key, selected, got)
		}
	}
}
