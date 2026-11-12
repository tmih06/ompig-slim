package codingagent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// getLogoutAccounts reads live metadata from the configured local or broker vault.
// Stable sorting puts the active account first; no credential secrets enter the picker.
func (m *InteractiveMode) getLogoutAccounts(ctx context.Context, provider string) ([]ai.CredentialAccountInfo, error) {
	store, err := ai.OpenCredentialStore(m.opts.AgentDir)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	accounts, err := store.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	accounts = slices.DeleteFunc(accounts, func(account ai.CredentialAccountInfo) bool { return account.ProviderID != provider })
	slices.SortStableFunc(accounts, func(a, b ai.CredentialAccountInfo) int {
		if a.Active == b.Active {
			return 0
		}
		if a.Active {
			return -1
		}
		return 1
	})
	return accounts, nil
}

// selectLogoutAccount uses the existing editor-slot selector. Identity
// keys disambiguate matching labels (for example, an email shared across orgs).
// Esc cancels without changing storage; even a single account requires selection.
func (m *InteractiveMode) selectLogoutAccount(provider tui.OAuthProvider, accounts []ai.CredentialAccountInfo) (ai.CredentialAccountInfo, bool) {
	labels := make([]string, len(accounts))
	for i, account := range accounts {
		parts := []string{string(account.Type)}
		if account.Label != "" {
			parts = append(parts, account.Label)
		}
		switch {
		case account.AccountKey == nil:
			parts = append(parts, "no identity")
		case *account.AccountKey == "":
			parts = append(parts, "empty identity")
		case *account.AccountKey != account.Label:
			parts = append(parts, *account.AccountKey)
		}
		if account.Active {
			parts = append(parts, "active")
		}
		labels[i] = widthx.ReplaceControlCharacters(strings.Join(parts, " · "))
	}
	selector := tui.NewExtensionSelectorComponent("Select account to log out of "+widthx.ReplaceControlCharacters(provider.Name)+":", labels, nil, nil)
	index, ok := m.runEditorSlotExtensionSelector(selector)
	if !ok {
		return ai.CredentialAccountInfo{}, false
	}
	return accounts[index], true
}

// runAccountLogout removes only the selected identity and synchronizes model state
// after the committed removal. Missing accounts are a no-op, never provider logout.
// Runtime/environment overrides and extension-owned credentials remain untouched.
func (m *InteractiveMode) runAccountLogout(ctx context.Context, account ai.CredentialAccountInfo) (bool, error) {
	store, err := ai.OpenCredentialStore(m.opts.AgentDir)
	if err != nil {
		return false, fmt.Errorf("auth storage: %w", err)
	}
	defer store.Close()
	removed, err := store.DeleteAccountIdentity(ctx, account.ProviderID, account.AccountKey)
	if err != nil || !removed {
		return removed, err
	}
	if runtime := m.opts.RequestAuthRuntime; runtime != nil {
		if err := runtime.SynchronizeCredentialState(ctx, account.ProviderID, CredentialSynchronizationLogout, nil); err != nil {
			return true, err
		}
	}
	m.updateProviderInfo()
	if m.tuiInst != nil {
		m.tuiInst.ForceFullRender()
		m.tuiInst.Render()
	}
	return true, nil
}
