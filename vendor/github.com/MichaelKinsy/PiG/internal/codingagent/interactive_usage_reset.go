package codingagent

// The interactive side of `/usage reset`: reading the saved-reset inventory,
// mounting the account picker in the editor slot, and spending the chosen
// reset off the input loop. Ports the reset branch of
// packages/coding-agent/src/modes/controllers/selector-controller.ts.

import (
	"context"

	"github.com/MichaelKinsy/PiG/tui"
)

// resetQuotaService returns the reset-capable quota source of this session, or
// nil when the build wired none.
func (m *InteractiveMode) resetQuotaService() UsageResetQuotaService {
	service := m.opts.UsageService
	if service == nil {
		service = m.usageService
	}
	resets, _ := service.(UsageResetQuotaService)
	return resets
}

// listResetCredits reads every stored account's saved-reset inventory. A mode
// without a reset-capable quota source reports an empty inventory, so the
// command can explain itself instead of failing.
func (m *InteractiveMode) listResetCredits(ctx context.Context) ([]UsageResetCreditAccount, error) {
	service := m.resetQuotaService()
	if service == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = m.backgroundContext()
	}
	return service.ListResetCredits(ctx)
}

// showAutoResetSelector opens the grouped provider/account checkbox view for
// automatic saved-reset redemption preferences.
func (m *InteractiveMode) showAutoResetSelector() {
	if m.editorContainer == nil || m.editor == nil || m.tuiInst == nil {
		return
	}
	service := m.resetQuotaService()
	auto, ok := service.(UsageAutoResetService)
	if !ok {
		m.showWarning("Automatic saved-reset settings are unavailable in this session.")
		return
	}
	ctx := m.backgroundContext()
	m.showStatus(tui.ActiveTheme().Fg("dim", "Loading saved-reset accounts…"))
	entries, err := service.ListResetCredits(ctx)
	if err != nil {
		m.showError("Could not load saved-reset accounts: " + tui.SanitizeDisplayLine(err.Error()))
		return
	}
	accounts := make([]tui.AutoResetAccount, 0, len(entries))
	for _, entry := range entries {
		accounts = append(accounts, tui.AutoResetAccount{
			ProviderID: entry.ProviderID, ProviderLabel: resetProviderLabel(entry.ProviderID), Tier: entry.Tier,
			Label: tui.SanitizeDisplayLine(entry.Label), TargetRef: usageAccountKeyText(entry.AccountKey),
			Active: entry.Active, Enabled: entry.AutoReset,
			ProviderLocked: auto.ProviderAutoResetDefault(entry.ProviderID),
			Err:            tui.SanitizeDisplayLine(entry.Err),
		})
	}
	if len(accounts) == 0 {
		m.showStatus("No Codex or Claude accounts found. Use /login to add one.")
		return
	}
	selector := tui.NewAutoResetSelectorComponent(accounts, func(account tui.AutoResetAccount, enabled bool) {
		if err := auto.SetAutoReset(ctx, account.ProviderID, resetAccountKey(account.TargetRef), enabled); err != nil {
			m.showError("Could not update " + account.Label + ": " + tui.SanitizeDisplayLine(err.Error()))
			return
		}
		state := "disabled"
		if enabled {
			state = "enabled"
		}
		m.showStatus("Automatic saved-reset redemption " + state + " for " + account.Label + ".")
	}, nil)
	selector.SetProviderToggleHandler(func(provider string, enabled bool) {
		if err := auto.SetAutoResetProvider(ctx, provider, enabled); err != nil {
			m.showError("Could not lock " + provider + ": " + tui.SanitizeDisplayLine(err.Error()))
			return
		}
		state := "unlocked"
		if enabled {
			state = "locked"
		}
		m.showStatus("Auto-reset " + state + " for new " + provider + " accounts.")
	})
	m.runEditorSlotComponent(selector, selector.HandleInput, selector.Done)
	m.requestRender()
}

// showResetUsageSelector opens the `/usage reset` account picker in the editor
// slot. It blocks the input loop while the picker is up; the spend that follows
// runs on the background task group so the prompt returns immediately.
func (m *InteractiveMode) showResetUsageSelector() {
	if m.editorContainer == nil || m.editor == nil || m.tuiInst == nil {
		return
	}
	service := m.resetQuotaService()
	if service == nil {
		m.showWarning("Saved rate-limit resets are unavailable in this session.")
		return
	}
	ctx := m.backgroundContext()
	m.showStatus(tui.ActiveTheme().Fg("dim", "Checking saved rate-limit resets…"))
	entries, err := service.ListResetCredits(ctx)
	if err != nil {
		m.showError("Could not load saved resets: " + tui.SanitizeDisplayLine(err.Error()))
		return
	}
	accounts := resetUsageAccounts(entries)
	if len(accounts) == 0 {
		m.showStatus("No provider accounts found. Use /login to add one.")
		return
	}
	failed := false
	for _, account := range accounts {
		failed = failed || account.Err != ""
	}
	// The spend picker is not an inventory dashboard: show only accounts that
	// can spend a valid reset now, leaving full inventory in /usage details.
	accounts = tui.RedeemableResetUsageAccounts(accounts)
	if len(accounts) == 0 {
		if failed {
			m.showStatus("No saved resets available — some accounts couldn't be reached (try /login).")
			return
		}
		m.showStatus("No saved rate-limit resets available to spend right now.")
		return
	}

	var chosen *tui.ResetUsageAccount
	selector := tui.NewResetUsageSelectorComponent(accounts, func(account tui.ResetUsageAccount) {
		chosen = &account
	}, nil)
	if !m.runEditorSlotComponent(selector, selector.HandleInput, selector.Done) || chosen == nil {
		m.requestRender()
		return
	}
	m.redeemResetRow(*chosen)
}

// redeemResetRow spends one picker row's reset on the background task group and
// reports the outcome when the owner loop publishes it.
func (m *InteractiveMode) redeemResetRow(account tui.ResetUsageAccount) {
	ctx := m.backgroundContext()
	label := tui.SanitizeDisplayLine(account.Label)
	m.showStatus(tui.ActiveTheme().Fg("dim", "Spending 1 saved reset for "+label+" ("+account.ProviderLabel+")…"))
	target := resetUsageTargetForRow(account)
	service := m.resetQuotaService()
	if service == nil {
		return
	}
	m.backgroundTasks.Go(func() {
		outcome, err := service.RedeemResetCredit(ctx, target)
		_ = m.postToMain(ctx, func() {
			if ctx.Err() != nil {
				return
			}
			m.reportResetOutcome(outcome, err, label)
		})
	})
}

// reportResetOutcome publishes one spend outcome: the status line for a spend
// that happened, a warning otherwise.
func (m *InteractiveMode) reportResetOutcome(outcome UsageResetOutcome, err error, label string) {
	if err != nil {
		m.showError("Reset failed for " + label + ": " + tui.SanitizeDisplayLine(err.Error()))
		return
	}
	message := tui.SanitizeDisplayLine(describeResetOutcome(outcome, label))
	if outcome.Ok {
		// The redeem invalidated the account's cached usage report. Re-reading the
		// quota now is what turns the spend into a visible countdown: the prompt
		// border otherwise keeps showing the spent window until its minute tick.
		m.refreshPromptQuota(m.backgroundContext())
		if m.editor != nil {
			m.editor.Invalidate()
		}
		m.showStatus(message)
	} else {
		m.showWarning(message)
	}
	m.requestRender()
}

// redeemResetCredit resolves a typed `<provider>/<account>` target against the
// live inventory and spends its reset. Resolving first keeps the provider's own
// eligibility verdict and the human label in the outcome, instead of asking a
// provider to spend a credit the caller only half described.
func (m *InteractiveMode) redeemResetCredit(ctx context.Context, resetTarget string) (UsageResetOutcome, error) {
	service := m.resetQuotaService()
	if service == nil {
		return UsageResetOutcome{}, nil
	}
	if ctx == nil {
		ctx = m.backgroundContext()
	}
	entries, err := service.ListResetCredits(ctx)
	if err != nil {
		return UsageResetOutcome{}, err
	}
	accounts := resetUsageAccounts(entries)
	target, account, problem := resetUsageTargetFromArgs(accounts, resetTarget, "/usage reset")
	if problem != "" {
		return UsageResetOutcome{ProviderID: account.ProviderID, Label: account.Label, Code: "no_credit", Reason: problem}, nil
	}
	return service.RedeemResetCredit(ctx, target)
}
