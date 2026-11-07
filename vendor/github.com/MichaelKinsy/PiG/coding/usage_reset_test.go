package coding

import (
	"strings"
	"testing"

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// newResetSlashSession builds a Session with no stored credentials, which is
// what `/usage reset` must explain rather than fail on.
func newResetSlashSession(t *testing.T) *Session {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD:             t.TempDir(),
		AgentDir:        t.TempDir(),
		SettingsManager: icodingagent.NewInMemorySettingsManager(Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svcs.Close)
	session, err := NewSession(svcs, SessionOptions{NoSession: true, ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestDispatchSlashUsageResetExplainsMissingAccounts(t *testing.T) {
	session := newResetSlashSession(t)
	for _, line := range []string{"/usage reset", "/usage reset codex/active"} {
		output, err := session.DispatchSlash(line)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		joined := strings.Join(output, "\n")
		if !strings.Contains(joined, "No provider accounts found") {
			t.Fatalf("%s output = %q", line, joined)
		}
	}
}

func TestDispatchSlashUsageRejectsUnknownArguments(t *testing.T) {
	session := newResetSlashSession(t)
	output, err := session.DispatchSlash("/usage resets")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if joined := strings.Join(output, "\n"); !strings.Contains(joined, "Usage: /usage") {
		t.Fatalf("output = %q", joined)
	}
}

func TestSessionRedeemResetCreditWithoutAServiceReportsNothing(t *testing.T) {
	session := newResetSlashSession(t)
	accounts, err := session.ListResetCredits(nil)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("accounts = %+v", accounts)
	}
	outcome, err := session.RedeemResetCredit(nil, "codex/active")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	// No stored account can be named, so the refusal is a business outcome and
	// not an error: nothing was spent.
	if outcome.Ok {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Code == "" {
		t.Fatalf("outcome carries no code: %+v", outcome)
	}
}
