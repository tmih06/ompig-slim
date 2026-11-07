package tui

import (
	"strings"
	"testing"
)

// resetSelectorFixture is two accounts: one spendable, one banked but unusable.
func resetSelectorFixture() []ResetUsageAccount {
	return []ResetUsageAccount{
		{
			Label: "active@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex",
			AvailableCount: 2, RedeemableCount: 2, TargetRef: "key-1", Active: true,
			ExpiresAt: "2099-01-01T00:00:00Z", CreditTitle: "One free rate limit reset",
		},
		{
			Label: "idle@example.com", ProviderID: "anthropic", ProviderLabel: "Claude",
			AvailableCount: 1, RedeemableCount: 0, TargetRef: "key-2",
			UnavailableReason: "not usable right now",
		},
	}
}

func TestResetUsageSelectorRequiresConfirmation(t *testing.T) {
	selected := []ResetUsageAccount{}
	selector := NewResetUsageSelectorComponent(resetSelectorFixture(), func(account ResetUsageAccount) {
		selected = append(selected, account)
	}, nil)

	// Enter once only opens the confirmation: a reset is irreversible.
	selector.HandleInput("\r")
	if selector.Done() {
		t.Fatal("one confirm spent a reset")
	}
	if len(selected) != 0 {
		t.Fatalf("selected = %+v", selected)
	}
	frame := strings.Join(selector.Render(100), "\n")
	if !strings.Contains(frame, "again to spend") {
		t.Fatalf("confirmation missing from frame:\n%s", frame)
	}
	selector.HandleInput("\r")
	if !selector.Done() || len(selected) != 1 {
		t.Fatalf("second confirm did not spend: done=%v selected=%+v", selector.Done(), selected)
	}
	if selected[0].TargetRef != "key-1" {
		t.Fatalf("spent %+v", selected[0])
	}
}

func TestResetUsageSelectorMovesAndDropsConfirmation(t *testing.T) {
	accounts := resetSelectorFixture()
	accounts[1].RedeemableCount = 1
	accounts[1].UnavailableReason = ""
	selector := NewResetUsageSelectorComponent(accounts, func(ResetUsageAccount) {}, nil)
	selector.HandleInput("\r")
	selector.HandleInput(downKey())
	if selector.Done() {
		t.Fatal("moving spent a reset")
	}
	frame := strings.Join(selector.Render(100), "\n")
	if strings.Contains(frame, "again to spend") {
		t.Fatalf("confirmation survived a move:\n%s", frame)
	}
	// The next valid account still requires its own two-step confirmation.
	selector.HandleInput("\r")
	if selector.Done() {
		t.Fatal("moving reused the previous account's confirmation")
	}
	frame = strings.Join(selector.Render(100), "\n")
	if !strings.Contains(frame, "again to spend") || !strings.Contains(frame, "idle@example.com") {
		t.Fatalf("new confirmation missing from frame:\n%s", frame)
	}
}

func TestResetUsageSelectorCancelNeedsTwoPresses(t *testing.T) {
	cancelled := 0
	selector := NewResetUsageSelectorComponent(resetSelectorFixture(), func(ResetUsageAccount) {}, func() { cancelled++ })
	selector.HandleInput("\r")
	// The first cancel drops the pending confirmation, exactly as the second
	// would close the picker if one were pending anyway.
	selector.HandleInput("\x1b")
	if selector.Done() || cancelled != 0 {
		t.Fatalf("first cancel closed the picker: done=%v cancelled=%d", selector.Done(), cancelled)
	}
	selector.HandleInput("\x1b")
	if !selector.Done() || !selector.Cancelled() || cancelled != 1 {
		t.Fatalf("second cancel did not close: done=%v cancelled=%d", selector.Done(), cancelled)
	}
}

func TestResetUsageSelectorRendersCountsAndExpiry(t *testing.T) {
	selector := NewResetUsageSelectorComponent(resetSelectorFixture(), func(ResetUsageAccount) {}, nil)
	frame := strings.Join(selector.Render(120), "\n")
	for _, want := range []string{"active@example.com", "2 saved", "Codex · key-1", "expires in"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "idle@example.com") || strings.Contains(frame, "not usable right now") {
		t.Fatalf("unusable account cluttered the picker:\n%s", frame)
	}
}

func TestResetUsageSelectorStartsOnASpendableAccount(t *testing.T) {
	accounts := []ResetUsageAccount{
		{Label: "banked", ProviderID: "anthropic", ProviderLabel: "Claude", AvailableCount: 1, TargetRef: "a"},
		{Label: "spendable", ProviderID: "openai-codex", ProviderLabel: "Codex", AvailableCount: 1, RedeemableCount: 1, TargetRef: "b"},
	}
	selected := []ResetUsageAccount{}
	selector := NewResetUsageSelectorComponent(accounts, func(account ResetUsageAccount) {
		selected = append(selected, account)
	}, nil)
	selector.HandleInput("\r")
	selector.HandleInput("\r")
	if len(selected) != 1 || selected[0].Label != "spendable" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestResetUsageSummaryLineIsOneLine(t *testing.T) {
	account := ResetUsageAccount{
		Label: "user@example.com\nsecond line", ProviderID: "openai-codex", ProviderLabel: "Codex",
		AvailableCount: 3, RedeemableCount: 0, TargetRef: "k", Active: true,
		ExpiresAt: "2099-01-01T00:00:00Z", UnavailableReason: "blocked\nby the 5h window",
	}
	line := ResetUsageSummaryLine(account)
	if strings.Contains(line, "\n") {
		t.Fatalf("line spans lines: %q", line)
	}
	for _, want := range []string{"openai-codex/k", "3 saved, 0 usable now", "(active)", "blocked by the 5h window", "expires in"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line lacks %q: %q", want, line)
		}
	}
}

func TestResetUsageSelectorHidesInvalidAccounts(t *testing.T) {
	accounts := []ResetUsageAccount{
		{Label: "empty"},
		{Label: "ineligible", AvailableCount: 2},
		{Label: "failed", AvailableCount: 1, RedeemableCount: 1, Err: "unreachable"},
		{Label: "expired", AvailableCount: 1, RedeemableCount: 1, ExpiresAt: "2000-01-01T00:00:00Z"},
		{Label: "malformed", AvailableCount: 1, RedeemableCount: 1, ExpiresAt: "invalid"},
		{Label: "valid", AvailableCount: 1, RedeemableCount: 1, ExpiresAt: "2099-01-01T00:00:00Z"},
		{Label: "undated", AvailableCount: 1, RedeemableCount: 1},
	}
	selector := NewResetUsageSelectorComponent(accounts, nil, nil)
	rows := selector.Accounts()
	if len(rows) != 2 || rows[0].Label != "valid" || rows[1].Label != "undated" {
		t.Fatalf("only valid reset accounts should be shown: %+v", rows)
	}
	if accounts[0].Label != "empty" || len(accounts) != 7 {
		t.Fatal("filter mutated the source inventory")
	}
}

// downKey is the sequence the keybinding registry binds to tui.select.down.
func downKey() string { return keySequence(KBSelectDown) }
