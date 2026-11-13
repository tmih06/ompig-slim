package tui

import "testing"

func TestAutoResetSelectorLocksProviderForFutureAccounts(t *testing.T) {
	selector := NewAutoResetSelectorComponent([]AutoResetAccount{
		{ProviderID: "codex-json", ProviderLabel: "Codex JSON", Label: "a", TargetRef: "a"},
	}, nil, nil)
	selector.HandleInput("l")
	if !selector.Accounts()[0].ProviderLocked || !selector.Accounts()[0].Enabled {
		t.Fatalf("provider lock = %+v", selector.Accounts()[0])
	}
}

func TestAutoResetSelectorTogglesAccountAndProviderRows(t *testing.T) {
	selector := NewAutoResetSelectorComponent([]AutoResetAccount{
		{ProviderID: "anthropic", ProviderLabel: "Claude", Label: "a", TargetRef: "a", Enabled: false},
		{ProviderID: "anthropic", ProviderLabel: "Claude", Label: "b", TargetRef: "b", Enabled: true},
		{ProviderID: "openai-codex", ProviderLabel: "Codex", Label: "c", TargetRef: "c", Enabled: false},
	}, nil, nil)
	// The first row is the Claude provider group; toggling it enables both accounts.
	selector.HandleInput("\n")
	accounts := selector.Accounts()
	if !accounts[0].Enabled || !accounts[1].Enabled || accounts[2].Enabled {
		t.Fatalf("provider toggle = %+v", accounts)
	}
	// Move past the provider and tier rows to the first account, then disable only it.
	selector.HandleInput("\x1b[B")
	selector.HandleInput("\x1b[B")
	selector.HandleInput("\n")
	accounts = selector.Accounts()
	if accounts[0].Enabled || !accounts[1].Enabled {
		t.Fatalf("account toggle = %+v", accounts)
	}
}
