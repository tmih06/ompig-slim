package codingagent

import (
	"testing"

	"github.com/MichaelKinsy/PiG/tui"
)

// TestLoginOpensCombinedSearch ensures /login never hides either auth method
// behind a category menu and preserves search text for non-exact provider names.
func TestLoginOpensCombinedSearch(t *testing.T) {
	for _, query := range []string{"", "cdxjsn"} {
		t.Run(query, func(t *testing.T) {
			options := []tui.OAuthProvider{
				{ID: "codex-json", Name: "Codex JSON", AuthType: "oauth"},
				{ID: "fixture", Name: "Fixture", AuthType: "api_key"},
			}
			selected, started := false, false
			sc := &SlashContext{Args: query,
				LoginProviders:   func() []tui.OAuthProvider { return options },
				SelectAuthMethod: func([]tui.OAuthProvider) (string, bool) { t.Fatal("unexpected auth-type gate"); return "", false },
				SelectAuthProvider: func(mode string, providers []tui.OAuthProvider, search string) (tui.OAuthProvider, bool) {
					selected = true
					if mode != "login" || search != query || len(providers) != 2 {
						t.Fatalf("incorrect combined list: %s %q %+v", mode, search, providers)
					}
					picker := tui.NewOAuthSelectorComponent(mode, providers, nil, nil, search)
					if picker.SelectedID() != "codex-json" {
						t.Fatalf("fuzzy search did not find Codex JSON: %q", picker.SelectedID())
					}
					return providers[0], true
				},
				StartProviderLogin: func(provider tui.OAuthProvider) error {
					started = true
					if provider.ID != "codex-json" {
						t.Fatalf("incorrect selection: %+v", provider)
					}
					return nil
				},
			}
			if err := handleLoginCommand(sc); err != nil || !selected || !started {
				t.Fatalf("login list not used: %v", err)
			}
		})
	}
}

// TestLoginExactProviderRetainsMethodChoice preserves explicit dual-method logins.
func TestLoginExactProviderRetainsMethodChoice(t *testing.T) {
	options := []tui.OAuthProvider{{ID: "fixture", Name: "Fixture", AuthType: "oauth"}, {ID: "fixture", Name: "Fixture", AuthType: "api_key"}}
	started := false
	sc := &SlashContext{Args: "fixture", LoginProviders: func() []tui.OAuthProvider { return options },
		SelectAuthMethod: func(providers []tui.OAuthProvider) (string, bool) {
			if len(providers) != 2 {
				t.Fatal("missing methods")
			}
			return "api_key", true
		},
		StartProviderLogin: func(provider tui.OAuthProvider) error {
			started = true
			if provider.AuthType != "api_key" {
				t.Fatal("incorrect method")
			}
			return nil
		},
	}
	if err := handleLoginCommand(sc); err != nil || !started {
		t.Fatalf("exact provider login failed: %v", err)
	}
}
