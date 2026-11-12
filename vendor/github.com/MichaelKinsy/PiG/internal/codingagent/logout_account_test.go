package codingagent

import (
	"errors"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// TestLogoutIdentitylessAccountKeepsNamedSibling ensures legacy/API-key rows
// with a NULL identity can be removed without falling back to provider deletion.
func TestLogoutIdentitylessAccountKeepsNamedSibling(t *testing.T) {
	dir := t.TempDir()
	store, err := ai.OpenLocalCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()
	for _, key := range []*string{nil, new("")} {
		if _, err := store.SaveAccount(ctx, "fixture", key, ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"}); err != nil {
			t.Fatal(err)
		}
	}
	mode := &InteractiveMode{opts: InteractiveModeOptions{AgentDir: dir}}
	removed, err := mode.runAccountLogout(ctx, ai.CredentialAccountInfo{ProviderID: "fixture"})
	if err != nil || !removed {
		t.Fatalf("identity-less logout: %v %v", removed, err)
	}
	accounts, err := store.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].AccountKey == nil || *accounts[0].AccountKey != "" {
		t.Fatalf("nil/empty identities confused: %+v %v", accounts, err)
	}
}

// TestLogoutCancellationAndFailuresNeverDeleteOthers checks the public command
// boundary: cancelling either picker or a failed/stale operation cannot widen
// removal to the provider, and unsuccessful removal cannot report success.
func TestLogoutCancellationAndFailuresNeverDeleteOthers(t *testing.T) {
	for _, scenario := range []string{"provider cancel", "account cancel", "list failure", "delete failure", "stale account"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New("fixture unavailable")
			provider := tui.OAuthProvider{ID: "fixture", Name: "Fixture"}
			account := ai.CredentialAccountInfo{ProviderID: "fixture", AccountKey: new("one")}
			selected, deleted := false, false
			var status string
			sc := &SlashContext{
				LogoutProviders: func() ([]tui.OAuthProvider, error) { return []tui.OAuthProvider{provider}, nil },
				SelectAuthProvider: func(string, []tui.OAuthProvider, string) (tui.OAuthProvider, bool) {
					return provider, scenario != "provider cancel"
				},
				LogoutAccounts: func(string) ([]ai.CredentialAccountInfo, error) {
					if scenario == "provider cancel" {
						t.Fatal("listed accounts after provider cancellation")
					}
					if scenario == "list failure" {
						return nil, failure
					}
					return []ai.CredentialAccountInfo{account}, nil
				},
				SelectLogoutAccount: func(tui.OAuthProvider, []ai.CredentialAccountInfo) (ai.CredentialAccountInfo, bool) {
					selected = true
					return account, scenario != "account cancel"
				},
				LogoutAccount: func(ai.CredentialAccountInfo) (bool, error) {
					deleted = true
					if scenario == "delete failure" {
						return false, failure
					}
					return false, nil
				},
				Logout:     func(string) error { t.Fatal("provider-wide fallback called"); return nil },
				ShowStatus: func(message string) { status = message },
			}
			err := handleLogoutCommand(sc)
			if scenario == "list failure" || scenario == "delete failure" {
				if !errors.Is(err, failure) {
					t.Fatalf("failure hidden: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(scenario, "cancel") && (deleted || status != "") {
				t.Fatalf("cancel mutated state/reported success: %v %q", deleted, status)
			}
			if scenario == "list failure" && selected {
				t.Fatal("picker opened after failed read")
			}
			if scenario == "stale account" && (!deleted || !strings.Contains(status, "already removed")) {
				t.Fatalf("stale account reported success: %q", status)
			}
		})
	}
}

// TestLogoutSelectsOneAccount drives the slash command against a real vault:
// selecting a non-active account must keep the active sibling and other providers.
func TestLogoutSelectsOneAccount(t *testing.T) {
	dir := t.TempDir()
	mode := &InteractiveMode{opts: InteractiveModeOptions{AgentDir: dir}}
	store, err := ai.OpenLocalCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()
	for _, entry := range []struct{ provider, key string }{{"fixture", "remove"}, {"fixture", "keep"}, {"other", "untouched"}} {
		if _, err := store.SaveAccount(ctx, entry.provider, new(entry.key), ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-access", Refresh: "fixture-refresh"}); err != nil {
			t.Fatal(err)
		}
	}
	provider := tui.OAuthProvider{ID: "fixture", Name: "Fixture", AuthType: "oauth"}
	sc := &SlashContext{
		LogoutProviders:    func() ([]tui.OAuthProvider, error) { return []tui.OAuthProvider{provider}, nil },
		SelectAuthProvider: func(string, []tui.OAuthProvider, string) (tui.OAuthProvider, bool) { return provider, true },
		LogoutAccounts:     func(id string) ([]ai.CredentialAccountInfo, error) { return mode.getLogoutAccounts(ctx, id) },
		SelectLogoutAccount: func(_ tui.OAuthProvider, accounts []ai.CredentialAccountInfo) (ai.CredentialAccountInfo, bool) {
			if len(accounts) != 2 || !accounts[0].Active {
				t.Fatalf("missing active-first accounts: %+v", accounts)
			}
			for _, account := range accounts {
				if account.AccountKey != nil && *account.AccountKey == "remove" {
					return account, true
				}
			}
			t.Fatal("account to remove absent")
			return ai.CredentialAccountInfo{}, false
		},
		LogoutAccount: func(account ai.CredentialAccountInfo) (bool, error) { return mode.runAccountLogout(ctx, account) },
		Logout:        func(string) error { t.Fatal("provider-wide logout called"); return nil },
		ShowStatus:    func(string) {},
	}
	if err := handleLogoutCommand(sc); err != nil {
		t.Fatal(err)
	}
	remaining, err := store.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("logout changed siblings: %+v", remaining)
	}
	for _, account := range remaining {
		if account.ProviderID == "fixture" && (*account.AccountKey != "keep" || !account.Active) {
			t.Fatalf("active sibling changed: %+v", account)
		}
	}
}
