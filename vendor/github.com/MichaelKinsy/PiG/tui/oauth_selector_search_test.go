package tui

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func oauthFixtureProviders() []OAuthProvider {
	names := []string{"Anthropic", "Google", "Groq", "Mistral", "OpenAI", "OpenCode Go", "OpenCode Zen", "xAI"}
	providers := make([]OAuthProvider, 0, len(names))
	for _, name := range names {
		providers = append(providers, OAuthProvider{
			ID: strings.ToLower(strings.ReplaceAll(name, " ", "-")), Name: name, AuthType: "api_key",
		})
	}
	return providers
}

// TestOAuthSelectorSearchSelectsBestMatch is the /login regression: after
// scrolling, typing a query highlights the top match, and Enter picks it.
func TestOAuthSelectorSearchSelectsBestMatch(t *testing.T) {
	var selected []string
	selector := NewOAuthSelectorComponent("login", oauthFixtureProviders(),
		func(providerID, _ string) { selected = append(selected, providerID) }, nil)

	for range 5 {
		selector.HandleInput("\x1b[B")
	}
	for _, char := range "openai" {
		selector.HandleInput(string(char))
	}
	rows := selector.Render(120)
	if plain := widthx.StripAnsi(strings.Join(rows, "\n")); !strings.Contains(plain, "→ OpenAI") {
		t.Fatalf("best match is not highlighted:\n%s", plain)
	}
	selector.HandleInput("\r")
	if len(selected) != 1 || selected[0] != "openai" {
		t.Fatalf("selected = %v, want openai", selected)
	}
}

// TestOAuthSelectorSearchCursorKeysKeepSelection checks that moving the cursor
// inside the search field keeps the row the arrows selected.
func TestOAuthSelectorSearchCursorKeysKeepSelection(t *testing.T) {
	selector := NewOAuthSelectorComponent("login", oauthFixtureProviders(), nil, nil)

	selector.HandleInput("o")
	selector.HandleInput("\x1b[B")
	provider := selector.SelectedID()
	if provider == "" || provider == selector.filtered[0].ID {
		t.Fatalf("Down did not move the selection past the top row: %q", provider)
	}
	for _, key := range []string{"\x1b[D", "\x1b[C", "\x1b[H", "\x1b[F"} {
		selector.HandleInput(key)
		if got := selector.SelectedID(); got != provider {
			t.Fatalf("cursor key %q moved the selection from %q to %q", key, provider, got)
		}
	}
}

// TestOAuthSelectorClearingQueryKeepsClampedSelection checks that clearing the
// query keeps the row under the cursor instead of forcing the first row.
func TestOAuthSelectorClearingQueryKeepsClampedSelection(t *testing.T) {
	selector := NewOAuthSelectorComponent("login", oauthFixtureProviders(), nil, nil)

	selector.HandleInput("g")
	selector.HandleInput("\x1b[B")
	for range 2 {
		selector.HandleInput("\x7f")
	}
	if got := selector.SelectedID(); got == "" || got == selector.filtered[0].ID {
		t.Fatalf("clearing the query forced the selection to the first row: %q", got)
	}
	if len(selector.filtered) != 8 {
		t.Fatalf("clearing the query left %d rows, want 8", len(selector.filtered))
	}
}
