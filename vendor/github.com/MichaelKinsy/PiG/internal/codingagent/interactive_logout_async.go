package codingagent

import (
	"context"
	"errors"
	"fmt"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// startInteractiveLogout runs /logout as a cooperative state machine. Credential
// list and mutation I/O happens on mode-owned workers; only selectors and UI
// mutations run on the owner loop. This keeps input and rendering responsive
// when a remote credential broker is slow.
func (m *InteractiveMode) startInteractiveLogout(ctx context.Context) {
	if ctx == nil {
		ctx = m.runCtx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !m.logoutInFlight.CompareAndSwap(false, true) {
		m.showStatus("Logout is already in progress.")
		return
	}
	finish := func() { m.logoutInFlight.Store(false) }
	sc := m.buildSlashContext(ctx)
	m.showStatus("Loading stored credentials…")
	m.backgroundTasks.Go(func() {
		providers, err := m.getLogoutProviderOptions()
		m.runOnMain(ctx, func() {
			if ctx.Err() != nil {
				finish()
				return
			}
			if err != nil {
				finish()
				m.showError(fmt.Sprintf("Could not read stored credentials: %v", err))
				return
			}
			m.selectInteractiveLogoutProvider(ctx, sc, providers, finish)
		})
	})
}

func (m *InteractiveMode) selectInteractiveLogoutProvider(ctx context.Context, sc *SlashContext, providers []tui.OAuthProvider, finish func()) {
	if len(providers) == 0 {
		finish()
		m.showStatus("No stored credentials to remove. /logout only removes credentials saved by /login; environment variables and models.json config are unchanged.")
		return
	}
	provider, ok := sc.SelectAuthProvider("logout", providers, "")
	if !ok {
		finish()
		return
	}
	m.showStatus("Loading stored accounts for " + widthx.ReplaceControlCharacters(provider.Name) + "…")
	m.backgroundTasks.Go(func() {
		accounts, err := sc.LogoutAccounts(provider.ID)
		m.runOnMain(ctx, func() {
			if ctx.Err() != nil {
				finish()
				return
			}
			if err != nil {
				finish()
				m.showError(fmt.Sprintf("Could not read stored accounts: %v", err))
				return
			}
			if len(accounts) == 0 {
				m.startInteractiveOAuthLogout(ctx, provider, finish)
				return
			}
			account, ok := sc.SelectLogoutAccount(provider, accounts)
			if !ok {
				finish()
				return
			}
			m.startInteractiveAccountLogout(ctx, provider, account, finish)
		})
	})
}

func (m *InteractiveMode) startInteractiveAccountLogout(ctx context.Context, provider tui.OAuthProvider, account ai.CredentialAccountInfo, finish func()) {
	m.showStatus("Removing stored account for " + widthx.ReplaceControlCharacters(provider.Name) + "…")
	m.backgroundTasks.Go(func() {
		removed, err := m.deleteAccountCredential(ctx, account)
		m.runOnMain(ctx, func() {
			finish()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if _, ok := errors.AsType[*CredentialSynchronizationError](err); ok {
					m.showError(fmt.Sprintf("Account removed for %s, but local model state could not be synchronized: %v", provider.Name, err))
					return
				}
				m.showError(fmt.Sprintf("Logout failed: %v", err))
				return
			}
			if !removed {
				m.showStatus("That stored account was already removed. No other accounts were changed.")
				return
			}
			m.updateProviderInfo()
			if m.tuiInst != nil {
				m.tuiInst.ForceFullRender()
			}
			m.showStatus("Removed one stored account for " + widthx.ReplaceControlCharacters(provider.Name) + ". Other accounts, environment variables and models.json config are unchanged.")
		})
	})
}

func (m *InteractiveMode) startInteractiveOAuthLogout(ctx context.Context, provider tui.OAuthProvider, finish func()) {
	m.showStatus("Removing credentials for " + widthx.ReplaceControlCharacters(provider.Name) + "…")
	m.backgroundTasks.Go(func() {
		deleted, err := m.deleteOAuthCredentialState(ctx, provider.ID)
		m.runOnMain(ctx, func() {
			finish()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if _, ok := errors.AsType[*CredentialSynchronizationError](err); ok {
					m.showError(fmt.Sprintf("Credentials removed for %s, but local model state could not be synchronized: %s", provider.Name, err.Error()))
					return
				}
				m.showError(fmt.Sprintf("Logout failed: %v", err))
				return
			}
			if !deleted {
				m.showStatus(fmt.Sprintf("No credentials stored for %q. Use /login first.", provider.ID))
				return
			}
			m.updateProviderInfo()
			if m.tuiInst != nil {
				m.tuiInst.ForceFullRender()
			}
			message := "Logged out of " + provider.Name
			if provider.AuthType == "api_key" {
				message = "Removed stored API key for " + provider.Name + ". Environment variables and models.json config are unchanged."
			}
			m.showStatus(message)
		})
	})
}
