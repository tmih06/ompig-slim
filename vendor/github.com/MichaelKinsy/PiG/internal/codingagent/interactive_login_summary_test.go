package codingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// TestLoginSummaryText covers optional metadata and checks token ownership so
// success, identity and storage details do not regress to one dim status line.
func TestLoginSummaryText(t *testing.T) {
	theme := footerTheme(t)
	identity := func(value string) string { return value }
	tests := []struct {
		name     string
		action   string
		model    string
		accounts []ai.CredentialAccountInfo
		want     []string
		absent   []string
	}{
		{
			name: "active account and selected model", action: "Logged in to OpenAI", model: "gpt-5",
			accounts: []ai.CredentialAccountInfo{{Label: "other@example.com"}, {Label: "me@example.com", Active: true}},
			want:     []string{"✓ Logged in to OpenAI", "Account  me@example.com", "2 accounts available", "Model    gpt-5"},
			absent:   []string{"other@example.com"},
		},
		{
			name: "metadata unavailable", action: "Logged in to Custom",
			want:   []string{"✓ Logged in to Custom", "Credentials saved to /profile/agent.db"},
			absent: []string{"Account", "Model", "accounts available"},
		},
		{
			name: "identity unavailable", action: "Saved API key for Custom",
			accounts: []ai.CredentialAccountInfo{{Active: true, Type: ai.CredentialAPIKey}},
			want:     []string{"✓ Saved API key for Custom", "Account connected"},
			absent:   []string{"accounts available"},
		},
		{
			name: "no active account", action: "Logged in to Custom",
			accounts: []ai.CredentialAccountInfo{{Label: "not-the-new-account"}},
			absent:   []string{"not-the-new-account", "Account"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := loginSummaryText(theme, test.action, test.model, "/profile/agent.db", test.accounts, identity)
			plain := widthx.StripTerminalSequences(raw)
			for _, want := range test.want {
				if !strings.Contains(plain, want) {
					t.Errorf("summary %q missing %q", plain, want)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(plain, absent) {
					t.Errorf("summary %q unexpectedly contains %q", plain, absent)
				}
			}
			if !strings.HasPrefix(raw, theme.Fg("success", "✓ "+test.action)) {
				t.Error("success header does not use the success token")
			}
			if !strings.Contains(raw, theme.Fg("dim", "  Credentials saved to /profile/agent.db")) {
				t.Error("storage location does not use the dim token")
			}
			if test.model != "" && !strings.Contains(raw, theme.Fg("accent", test.model)) {
				t.Error("selected model does not use the accent token")
			}
		})
	}
}

// TestLoginSummaryEnrichment verifies that the initial confirmation needs no
// metadata and that the worker updates the same notice only through the owner
// queue. An unavailable vault must leave that successful confirmation intact.
func TestLoginSummaryEnrichment(t *testing.T) {
	footerFixtureTheme(t)
	for _, available := range []bool{true, false} {
		name := "unavailable"
		if available {
			name = "available"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if available {
				store, err := ai.OpenLocalCredentialStore(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				for _, provider := range []string{"fixture", "other"} {
					_, err = store.SaveAccount(t.Context(), provider, new(provider), ai.Credential{
						Type: ai.CredentialOAuth, AccountID: provider + "-account", Access: "secret-access", Refresh: "secret-refresh",
					})
					if err != nil {
						t.Fatal(err)
					}
				}
			} else {
				dir = filepath.Join(dir, "not-a-directory")
				if err := os.WriteFile(dir, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			m := &InteractiveMode{
				opts:          InteractiveModeOptions{AgentDir: dir},
				chatContainer: tui.NewContainer(), backgroundCtx: t.Context(), uiTaskCh: make(chan func(), 8),
			}
			m.showLoginSummary("fixture", "Logged in to Fixture", "model", "vault", func(s string) string { return s })
			initial := widthx.StripTerminalSequences(strings.Join(m.chatContainer.Render(100), "\n"))
			if !strings.Contains(initial, "✓ Logged in to Fixture") || strings.Contains(initial, "fixture-account") {
				t.Fatalf("initial confirmation: %q", initial)
			}
			m.backgroundTasks.Wait()
			select {
			case apply := <-m.uiTaskCh:
				apply()
			default:
				if available {
					t.Fatal("account metadata was not posted to the owner")
				}
			}
			final := widthx.StripTerminalSequences(strings.Join(m.chatContainer.Render(100), "\n"))
			if available != strings.Contains(final, "fixture-account") {
				t.Fatalf("unexpected account metadata: %q", final)
			}
			if !available && final != initial {
				t.Fatalf("failed lookup changed confirmation: %q", final)
			}
			if strings.Count(final, "✓ Logged in") != 1 || strings.Contains(final, "other-account") || strings.Contains(final, "secret-") {
				t.Fatalf("duplicate confirmation or leaked metadata: %q", final)
			}
		})
	}
}

// TestLoginSummarySanitizesMetadata ensures untrusted labels and paths cannot
// introduce terminal controls, and that redaction also applies to fetched data.
func TestLoginSummarySanitizesMetadata(t *testing.T) {
	theme := footerTheme(t)
	redact := func(value string) string { return strings.ReplaceAll(value, "private", "[redacted]") }
	accounts := []ai.CredentialAccountInfo{{Active: true, Label: "private\x1b[31m\nforged"}}
	got := widthx.StripTerminalSequences(loginSummaryText(theme, "Logged in\rforged", "model\tname", "/private\x1b]0;title\a", accounts, redact))
	if strings.Contains(got, "private") || strings.ContainsAny(got, "\x1b\r\t\a") {
		t.Fatalf("unsanitized summary: %q", got)
	}
	if strings.Count(got, "\n") != 3 {
		t.Fatalf("metadata injected an extra line: %q", got)
	}
}
