package codingagent

// `/usage reset`: the transcript listing, the typed target form, and the result
// messages. Ports packages/coding-agent/src/slash-commands/helpers/reset-usage.ts
// and the reset branch of the `/usage` built-in.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui"
)

// resetUsageArgument is the parsed shape of `/usage reset <target>`.
type resetUsageArgument struct {
	providerID string
	// account preserves the store's case-sensitive identity key, or "active".
	account string
}

// parseUsageSubcommand splits `/usage`'s argument into its verb and remainder.
func parseUsageSubcommand(args string) (verb, rest string) {
	trimmed := jsstring.Trim(args)
	if trimmed == "" {
		return "", ""
	}
	verb, rest, _ = strings.Cut(trimmed, " ")
	return strings.ToLower(verb), jsstring.Trim(rest)
}

// normalizeResetProvider maps a typed provider name or alias to its id.
func normalizeResetProvider(value string) (string, bool) {
	switch strings.ToLower(jsstring.Trim(value)) {
	case "anthropic", "claude":
		return "anthropic", true
	case "openai-codex", "codex":
		return "openai-codex", true
	}
	return "", false
}

// parseResetUsageArgument parses `<provider>/<credential>` or
// `<provider>/active`. The error text names the command that lists the choices.
func parseResetUsageArgument(args, command string) (resetUsageArgument, string) {
	text := jsstring.Trim(args)
	before, after, found := strings.Cut(text, "/")
	if !found || before == "" {
		return resetUsageArgument{}, "Choose an account with `" + command + " <provider>/<credential>`."
	}
	providerID, ok := normalizeResetProvider(before)
	if !ok {
		return resetUsageArgument{}, "Unknown reset provider \"" + tui.SanitizeDisplayLine(before) +
			"\". Use anthropic or openai-codex."
	}
	return resetUsageArgument{providerID: providerID, account: jsstring.Trim(after)}, ""
}

// resolveResetUsageTarget maps a typed target onto a listed account that can
// spend a reset now, or explains why nothing will be spent.
func resolveResetUsageTarget(accounts []tui.ResetUsageAccount, args, command string) (tui.ResetUsageAccount, string) {
	parsed, problem := parseResetUsageArgument(args, command)
	if problem != "" {
		return tui.ResetUsageAccount{}, problem
	}
	var match *tui.ResetUsageAccount
	// An empty account part names the provider's identity-less account, which
	// the typed reference cannot spell. It is only unambiguous while the
	// provider has exactly one candidate, so a second one withdraws the match.
	candidates := 0
	var sole *tui.ResetUsageAccount
	for index := range accounts {
		account := &accounts[index]
		if account.ProviderID != parsed.providerID {
			continue
		}
		candidates++
		sole = account
		switch {
		case strings.EqualFold(parsed.account, "active"):
			if account.Active {
				match = account
			}
		case parsed.account != "" && account.TargetRef == parsed.account:
			match = account
		}
	}
	if match == nil && parsed.account == "" && candidates == 1 && sole.TargetRef == "" {
		match = sole
	}
	if match == nil {
		return tui.ResetUsageAccount{}, "No stored account matches \"" + tui.SanitizeDisplayLine(args) +
			"\". List choices with `" + command + "`."
	}
	if match.Err != "" {
		return tui.ResetUsageAccount{}, tui.SanitizeDisplayLine(match.Label) + " [" + match.ProviderLabel +
			"]: saved resets unavailable (" + tui.SanitizeDisplayLine(match.Err) + "), so nothing was spent."
	}
	if match.RedeemableCount <= 0 {
		reason := ""
		if match.UnavailableReason != "" {
			reason = " (" + tui.SanitizeDisplayLine(match.UnavailableReason) + ")"
		}
		return tui.ResetUsageAccount{}, tui.SanitizeDisplayLine(match.Label) + " [" + match.ProviderLabel +
			"]: no saved resets usable right now" + reason + "."
	}
	return *match, ""
}

// ResetUsageAccountFromInventory converts one inventory entry into a picker row.
// It reads the counts each provider reports through the shared display summary,
// so the transcript, the picker, and the SDK all agree on "saved" versus
// "usable now".
func ResetUsageAccountFromInventory(entry UsageResetCreditAccount) tui.ResetUsageAccount {
	return resetUsageAccountFromInventory(entry, time.Now().UnixMilli())
}

// ResolveResetTarget maps a typed `<provider>/<account>` reference onto the
// listed account it names. A non-empty problem explains why nothing will be
// spent; account is then the listed row the reference reached, when it reached
// one, so a caller can still name it.
func ResolveResetTarget(accounts []tui.ResetUsageAccount, reference string) (UsageResetTarget, tui.ResetUsageAccount, string) {
	return resetUsageTargetFromArgs(accounts, reference, "/usage reset")
}

// resetUsageAccountFromInventory converts one inventory entry into a picker row,
// reading the counts each provider reports through the shared summary.
func resetUsageAccountFromInventory(entry UsageResetCreditAccount, nowMs int64) tui.ResetUsageAccount {
	row := tui.ResetUsageAccount{
		Label:         tui.SanitizeDisplayLine(entry.Label),
		ProviderID:    entry.ProviderID,
		ProviderLabel: resetProviderLabel(entry.ProviderID),
		Active:        entry.Active,
		Err:           tui.SanitizeDisplayLine(entry.Err),
		TargetRef:     accountKeyText(entry.AccountKey),
	}
	if row.Label == "" {
		row.Label = "account"
	}
	if entry.Credits == nil {
		return row
	}
	banked, redeemable, expiresAt, reason := tui.ResetCreditSummaryLines(entry.Credits, nowMs)
	row.AvailableCount = banked
	row.RedeemableCount = redeemable
	row.ExpiresAt = expiresAt
	row.UnavailableReason = reason
	if credit := selectedResetCredit(entry.Credits); credit != nil {
		row.CreditTitle = tui.SanitizeDisplayLine(credit.Title)
		row.CreditID = credit.ID
	}
	return row
}

// selectedResetCredit reports the credit a spend would use, so the row's title
// and expiry describe the grant the spend actually passes back.
//
// Claude names the one grant it permits spending, and that pin wins. Codex
// permits any available credit, so the soonest-expiring one wins: credits are
// perishable, and spending the soonest is what keeps the bank's value.
func selectedResetCredit(credits *ai.UsageResetCredits) *ai.UsageResetCreditDetail {
	if credits == nil {
		return nil
	}
	if credits.NextCreditID != "" {
		for index := range credits.Credits {
			if credits.Credits[index].ID == credits.NextCreditID {
				return &credits.Credits[index]
			}
		}
	}
	if picked := ai.PickSoonestExpiringResetCredit(credits.Credits); picked != nil {
		return picked
	}
	return nil
}

// resetUsageAccounts converts an inventory listing into sorted picker rows:
// the active account first, then the most spendable, then the label.
func resetUsageAccounts(entries []UsageResetCreditAccount) []tui.ResetUsageAccount {
	nowMs := time.Now().UnixMilli()
	rows := make([]tui.ResetUsageAccount, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, resetUsageAccountFromInventory(entry, nowMs))
	}
	// Stable insertion sort keeps the store's own order among equals, which is
	// the order the account listing already reported.
	for index := 1; index < len(rows); index++ {
		for current := index; current > 0 && resetUsageRowLess(rows[current], rows[current-1]); current-- {
			rows[current], rows[current-1] = rows[current-1], rows[current]
		}
	}
	return rows
}

// resetUsageRowLess ranks one reset row above another.
func resetUsageRowLess(left, right tui.ResetUsageAccount) bool {
	if left.Active != right.Active {
		return left.Active
	}
	if left.RedeemableCount != right.RedeemableCount {
		return left.RedeemableCount > right.RedeemableCount
	}
	if left.AvailableCount != right.AvailableCount {
		return left.AvailableCount > right.AvailableCount
	}
	if left.ProviderID != right.ProviderID {
		return left.ProviderID < right.ProviderID
	}
	return left.Label < right.Label
}

// handleUsageResetCommand runs `/usage reset [<provider>/<credential>|active]`
// with no picker available: it lists the accounts, or spends the named one and
// writes the outcome to `output`.
func handleUsageResetCommand(sc *SlashContext, args string) {
	if sc.ListResetCredits == nil {
		resetUsageOutput(sc, "Saved rate-limit resets are unavailable in this session.")
		return
	}
	ctx := resetDispatchContext(sc)
	entries, err := sc.ListResetCredits(ctx)
	if err != nil {
		resetUsageOutput(sc, "Could not load saved resets: "+tui.SanitizeDisplayLine(err.Error()))
		return
	}
	accounts := resetUsageAccounts(entries)
	if len(accounts) == 0 {
		resetUsageOutput(sc, "No provider accounts found. Use /login to add one.")
		return
	}
	if jsstring.Trim(args) == "" {
		lines := make([]string, 0, len(accounts)+2)
		lines = append(lines, "Saved rate-limit resets:")
		for _, account := range accounts {
			lines = append(lines, tui.ResetUsageSummaryLine(account))
		}
		lines = append(lines, "", "Spend one with `/usage reset <provider>/<credential>` or `/usage reset <provider>/active`.")
		resetUsageOutput(sc, strings.Join(lines, "\n"))
		return
	}
	account, problem := resolveResetUsageTarget(accounts, args, "/usage reset")
	if problem != "" {
		resetUsageOutput(sc, problem)
		return
	}
	if sc.RedeemResetCredit == nil {
		resetUsageOutput(sc, "Spending a saved reset is unavailable in this session.")
		return
	}
	outcome, err := sc.RedeemResetCredit(ctx, tui.ResetUsageTargetRef(account))
	if err != nil {
		resetUsageOutput(sc, "Reset failed for "+account.Label+": "+tui.SanitizeDisplayLine(err.Error()))
		return
	}
	resetUsageOutput(sc, describeResetOutcome(outcome, account.Label))
}

// resetUsageOutput writes one reset message to the transcript.
func resetUsageOutput(sc *SlashContext, message string) {
	if sc.AppendText != nil {
		sc.AppendText(message)
		return
	}
	if sc.Append != nil {
		sc.Append(message)
	}
}

// resetDispatchContext is the context a reset run uses: the mode's background
// context when the host wired one, else the command's own.
func resetDispatchContext(sc *SlashContext) context.Context {
	if sc.ResetDispatchContext != nil {
		if ctx := sc.ResetDispatchContext(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// describeResetOutcome renders one redeem outcome as a user-facing sentence.
func describeResetOutcome(outcome UsageResetOutcome, label string) string {
	provider := ""
	if outcome.ProviderID != "" {
		provider = " (" + resetProviderLabel(outcome.ProviderID) + ")"
	}
	if label == "" {
		label = outcome.Label
	}
	reason := ""
	if outcome.Reason != "" {
		reason = " — " + tui.SanitizeDisplayLine(outcome.Reason)
	}
	switch outcome.Code {
	case "reset":
		scope := "your rate-limit window has been refreshed"
		switch {
		case len(outcome.Cleared) == 1 && outcome.Cleared[0] == "anthropic:5h":
			scope = "Claude's 5h session limit has been refreshed; weekly limits are unchanged"
		case len(outcome.Cleared) > 0:
			scope = "the covered rate-limit window has been refreshed"
		}
		return "Reset applied for " + label + provider + " — " + scope + "."
	case "already_redeemed":
		return label + provider + ": that reset was already redeemed."
	case "no_credit":
		return label + provider + ": no saved resets available to spend" + reason + "."
	case "credit_list_failed":
		return label + provider + ": couldn't load this account's saved resets (network/auth) — nothing was spent, try again."
	case "nothing_to_reset":
		return label + provider + ": nothing to reset right now — your covered limits aren't constrained, so no credit was spent."
	case "no_account":
		return "Could not find the stored account " + label + provider + "."
	case "account_unavailable":
		return label + provider + ": could not authenticate this account — try /login."
	case "offer_changed":
		return label + provider + ": the reset offer changed — nothing was spent; reopen /usage reset."
	case "reset_in_progress":
		return label + provider + ": a reset is already in progress."
	case "network_error", "malformed_response", "reset_unconfirmed":
		return label + provider + ": couldn't confirm whether the reset applied — check /usage before trying again" + reason + "."
	}
	return fmt.Sprintf("%s%s: reset was not confirmed (%s)%s.", label, provider, outcome.Code, reason)
}

// resetUsageTargetForRow builds the redeem target of one account row.
func resetUsageTargetForRow(account tui.ResetUsageAccount) UsageResetTarget {
	return UsageResetTarget{
		ProviderID: account.ProviderID,
		AccountKey: resetAccountKey(account.TargetRef),
		CreditID:   account.CreditID,
		Label:      account.Label,
	}
}

// resetAccountKey maps a typed account reference back onto a nullable key: an
// empty reference is the identity-less account.
func resetAccountKey(reference string) *string {
	if reference == "" {
		return nil
	}
	key := reference
	return &key
}

// resetUsageTargetFromArgs resolves a typed target against the live inventory
// and returns the redeem target plus the row it named.
func resetUsageTargetFromArgs(accounts []tui.ResetUsageAccount, args, command string) (UsageResetTarget, tui.ResetUsageAccount, string) {
	account, problem := resolveResetUsageTarget(accounts, args, command)
	if problem != "" {
		return UsageResetTarget{}, tui.ResetUsageAccount{}, problem
	}
	return resetUsageTargetForRow(account), account, ""
}
