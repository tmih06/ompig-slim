package codingagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// loginSummaryText renders only non-secret metadata. Redaction and terminal-control
// escaping happen before styling, so provider-supplied labels cannot inject ANSI.
// The caller renders again on theme changes rather than caching escape sequences.
func loginSummaryText(theme *tui.Theme, action, model, path string, accounts []ai.CredentialAccountInfo, redact func(string) string) string {
	clean := func(value string) string { return widthx.ReplaceControlCharacters(redact(value)) }
	lines := []string{theme.Fg("success", "✓ "+clean(action))}
	for _, account := range accounts {
		if !account.Active {
			continue
		}
		label := account.Label
		if label == "" {
			label = "Account connected"
		}
		lines = append(lines, theme.Fg("muted", "  Account  ")+theme.Fg("accent", clean(label)))
		break
	}
	if len(accounts) > 1 {
		lines = append(lines, theme.Fg("muted", fmt.Sprintf("  %d accounts available for this provider", len(accounts))))
	}
	if model != "" {
		lines = append(lines, theme.Fg("muted", "  Model    ")+theme.Fg("accent", clean(model)))
	}
	lines = append(lines, theme.Fg("dim", "  Credentials saved to "+clean(path)))
	return strings.Join(lines, "\n")
}

// showLoginSummary immediately confirms a committed login, then enriches that
// same notice with the vault's active account metadata. Local and broker reads
// run off the owner loop with a short deadline; unavailable metadata is silently
// omitted and never turns successful authentication into an error. UI state and
// theme-dependent rendering remain owned by the main loop.
func (m *InteractiveMode) showLoginSummary(providerID, action, model, path string, redact func(string) string) {
	if m.chatContainer == nil {
		return
	}
	var accounts []ai.CredentialAccountInfo
	notice := tui.NewThemedText(func() string {
		return loginSummaryText(tui.ActiveTheme(), action, model, path, accounts, redact)
	}, 1, 0)
	m.chatContainer.Add(tui.NewSpacer(1))
	m.chatContainer.Add(notice)
	m.lastStatusSpacer, m.lastStatusText = nil, nil
	if m.tuiInst != nil {
		m.tuiInst.RequestRender()
	}
	ctx := m.backgroundCtx
	if ctx == nil {
		ctx = m.runCtx
	}
	ownerCtx := ctx
	if ctx == nil {
		ctx = context.Background()
	}
	m.backgroundTasks.Go(func() {
		lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		found, err := m.getLogoutAccounts(lookupCtx, providerID)
		if err != nil || len(found) == 0 || lookupCtx.Err() != nil {
			return
		}
		m.runOnMain(ownerCtx, func() {
			if ctx.Err() != nil {
				return
			}
			accounts = found
			notice.Invalidate()
			if m.tuiInst != nil {
				m.tuiInst.RequestRender()
			}
		})
	})
}
