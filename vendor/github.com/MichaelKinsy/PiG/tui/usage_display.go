package tui

// Ports packages/coding-agent/src/modes/interactive/components/usage-display.ts:
// shared formatting, sanitization, plan grouping, and provider-card
// aggregation for the /usage dashboard.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// ─── Sanitization ────────────────────────────────────────────────────────────

// SanitizeDisplayText strips ANSI/OSC/APC sequences and C0/C1 controls, then
// expands tabs. Use it before theming anything a provider reported.
func SanitizeDisplayText(text string) string {
	return widthx.ReplaceControlCharacters(widthx.StripTerminalSequences(text))
}

// SanitizeDisplayLine collapses newlines and trims: safe for a single TUI row.
func SanitizeDisplayLine(text string) string {
	return strings.Join(strings.Fields(SanitizeDisplayText(text)), " ")
}

// TruncateMiddleToWidth middle-truncates to a visible width, keeping the
// suffix that distinguishes buckets.
func TruncateMiddleToWidth(text string, maxWidth int) string {
	width := widthx.VisibleWidth(text)
	if width <= maxWidth {
		return text
	}
	if maxWidth <= 1 {
		if maxWidth == 1 {
			return "…"
		}
		return ""
	}
	tailWidth := (maxWidth - 1 + 1) / 2
	headWidth := maxWidth - 1 - tailWidth
	return widthx.SliceByColumn(text, 0, headWidth, true) + "…" +
		widthx.SliceByColumn(text, width-tailWidth, tailWidth, true)
}

// ─── Labels and durations ────────────────────────────────────────────────────

// FormatProviderName title-cases a provider id for display
// (`openai-codex` → `Openai Codex`).
func FormatProviderName(provider string) string {
	parts := strings.FieldsFunc(provider, func(r rune) bool { return r == '-' || r == '_' })
	for i, part := range parts {
		runes := []rune(part)
		parts[i] = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	return strings.Join(parts, " ")
}

const (
	usageSecondMs = int64(1000)
	usageMinuteMs = 60 * usageSecondMs
	usageHourMs   = 60 * usageMinuteMs
	usageDayMs    = 24 * usageHourMs
)

// FormatDuration renders a coarse, human duration: `45s`, `1.2s`, `3m20s`,
// `2h15m`, `3d4h`.
func FormatDuration(ms int64) string {
	if ms <= 0 {
		return "0ms"
	}
	if ms < usageSecondMs {
		return strconv.FormatInt(ms, 10) + "ms"
	}
	if ms < usageMinuteMs {
		tenths := int64(math.Round(float64(ms) / 100))
		if tenths < 600 {
			return strconv.FormatFloat(float64(tenths)/10, 'f', 1, 64) + "s"
		}
		return "1m"
	}
	if ms < usageHourMs {
		mins := ms / usageMinuteMs
		secs := (ms % usageMinuteMs) / usageSecondMs
		if secs > 0 {
			return fmt.Sprintf("%dm%ds", mins, secs)
		}
		return strconv.FormatInt(mins, 10) + "m"
	}
	if ms < usageDayMs {
		hours := ms / usageHourMs
		mins := (ms % usageHourMs) / usageMinuteMs
		if mins > 0 {
			return fmt.Sprintf("%dh%dm", hours, mins)
		}
		return strconv.FormatInt(hours, 10) + "h"
	}
	days := ms / usageDayMs
	hours := (ms % usageDayMs) / usageHourMs
	if hours > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return strconv.FormatInt(days, 10) + "d"
}

// compactUnits are Intl.NumberFormat's en-US compact suffixes in magnitude order.
var compactUnits = []struct {
	value  float64
	suffix string
}{
	{1e15, "Q"},
	{1e12, "T"},
	{1e9, "B"},
	{1e6, "M"},
	{1e3, "K"},
}

// FormatCompactCount formats a count the way Intl.NumberFormat's en-US compact
// notation does with at most one fraction digit (`5.6K`, `1.2M`).
func FormatCompactCount(value float64) string {
	negative := value < 0
	absolute := math.Abs(value)
	if math.IsNaN(absolute) || math.IsInf(absolute, 0) {
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
	for _, unit := range compactUnits {
		if absolute >= unit.value {
			scaled := absolute / unit.value
			text := trimCompactFraction(strconv.FormatFloat(scaled, 'f', 1, 64))
			if negative {
				text = "-" + text
			}
			return text + unit.suffix
		}
	}
	if absolute == math.Trunc(absolute) {
		return strconv.FormatInt(int64(absolute), 10)
	}
	return trimCompactFraction(strconv.FormatFloat(absolute, 'f', 1, 64))
}

// trimCompactFraction drops a trailing `.0`, as compact notation does.
func trimCompactFraction(text string) string {
	return strings.TrimSuffix(text, ".0")
}

// FormatLimitTitle includes the usage tier in a limit title unless its label
// already names it.
func FormatLimitTitle(limit ai.UsageLimit) string {
	tier := limit.Scope.Tier
	if tier != "" && !strings.Contains(strings.ToLower(limit.Label), strings.ToLower(tier)) {
		return limit.Label + " (" + tier + ")"
	}
	return limit.Label
}

// formatUsageResetWindow renders a normalized saved-reset window id.
func formatUsageResetWindow(windowID string) string {
	switch windowID {
	case "anthropic:5h":
		return "Claude 5h"
	case "anthropic:7d":
		return "Claude weekly"
	case "anthropic:7d:opus":
		return "Claude Opus weekly"
	case "anthropic:7d:sonnet":
		return "Claude Sonnet weekly"
	}
	return windowID
}

// ─── Reset credits ───────────────────────────────────────────────────────────

// UsageResetSummary is the saved inventory and current eligibility shared by
// every usage display.
type UsageResetSummary struct {
	BankedCount       int
	RedeemableCount   int
	SoonestExpiry     string // ISO timestamp, "" when none
	UnavailableReason string
}

// bankedResetCreditExpiryMs is the expiry of a credit that still banks a reset,
// whether or not it can be spent now.
func bankedResetCreditExpiryMs(credit ai.UsageResetCreditDetail) (int64, bool) {
	if credit.ExpiresAt == "" || credit.RemainingCount == 0 || credit.Status == "redeemed" {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, credit.ExpiresAt)
	if err != nil {
		return 0, false
	}
	return parsed.UnixMilli(), true
}

// SummarizeUsageResetCredits normalizes count-only and current provider reset
// metadata for display. AvailableCount is banked inventory; RedeemableCount is
// the subset that may be spent now.
func SummarizeUsageResetCredits(reset *ai.UsageResetCredits, nowMs int64) *UsageResetSummary {
	if reset == nil {
		return nil
	}
	banked := max(0, reset.AvailableCount)
	redeemable := banked
	if reset.RedeemableCount != nil {
		redeemable = max(0, *reset.RedeemableCount)
	}
	var soonestExpiry string
	soonestExpiryMs := int64(math.MaxInt64)
	var latestExpired string
	latestExpiredMs := int64(math.MinInt64)
	for _, credit := range reset.Credits {
		expiryMs, ok := bankedResetCreditExpiryMs(credit)
		if !ok {
			continue
		}
		switch {
		case expiryMs > nowMs && expiryMs < soonestExpiryMs:
			soonestExpiryMs, soonestExpiry = expiryMs, credit.ExpiresAt
		case expiryMs <= nowMs && expiryMs > latestExpiredMs:
			latestExpiredMs, latestExpired = expiryMs, credit.ExpiresAt
		}
	}
	if soonestExpiry == "" {
		soonestExpiry = latestExpired
	}
	var reason string
	switch {
	case reset.Reason != "":
		reason = reset.Reason
	case reset.CooldownUntil != "":
		reason = "cooldown until " + reset.CooldownUntil
	}
	if reason == "" {
		var selected *ai.UsageResetCreditDetail
		if reset.NextCreditID != "" {
			for i := range reset.Credits {
				if reset.Credits[i].ID == reset.NextCreditID {
					selected = &reset.Credits[i]
					break
				}
			}
		}
		if selected == nil {
			for i := range reset.Credits {
				if reset.Credits[i].Usable == nil || *reset.Credits[i].Usable {
					selected = &reset.Credits[i]
					break
				}
			}
			if selected == nil && len(reset.Credits) > 0 {
				selected = &reset.Credits[0]
			}
		}
		if selected != nil {
			var blocked []string
			for _, window := range selected.Blocking {
				blocked = append(blocked, formatUsageResetWindow(window))
			}
			switch {
			case len(blocked) > 0:
				reason = "blocked by " + strings.Join(blocked, ", ")
			case selected.Status != "" && selected.Status != "available":
				reason = selected.Status
			}
		}
	}
	if reason == "" && reset.Eligible != nil && !*reset.Eligible {
		reason = "not eligible"
	}
	if reason == "" && banked > 0 && redeemable == 0 {
		reason = "not usable right now"
	}
	return &UsageResetSummary{
		BankedCount:       banked,
		RedeemableCount:   redeemable,
		SoonestExpiry:     soonestExpiry,
		UnavailableReason: reason,
	}
}

// ─── Absolute-only amounts ───────────────────────────────────────────────────

func isAbsoluteUnit(limit ai.UsageLimit) bool {
	return limit.Amount.Unit != ai.UsageUnitPercent && limit.Amount.Unit != ai.UsageUnitUnknown
}

func finitePointer(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

// IsUsedOnlyAbsoluteAmount reports a spend-to-date amount with no allowance.
func IsUsedOnlyAbsoluteAmount(limit ai.UsageLimit) bool {
	amount := limit.Amount
	if !isAbsoluteUnit(limit) || !finitePointer(amount.Used) {
		return false
	}
	if amount.Limit != nil || amount.Remaining != nil {
		return false
	}
	_, ok := ai.UsageUsedFraction(limit)
	return !ok
}

// isRemainingOnlyAbsoluteAmount reports a prepaid balance with no total.
func isRemainingOnlyAbsoluteAmount(limit ai.UsageLimit) bool {
	amount := limit.Amount
	if !isAbsoluteUnit(limit) || !finitePointer(amount.Remaining) {
		return false
	}
	if amount.Limit != nil || amount.Used != nil {
		return false
	}
	_, ok := ai.UsageUsedFraction(limit)
	return !ok
}

func formatQuantity(value float64, unit ai.UsageUnit, suffix string) string {
	if unit == ai.UsageUnitUSD {
		return fmt.Sprintf("$%.2f %s", value, suffix)
	}
	if unit == "" {
		return fmt.Sprintf("%s %s", FormatCompactCount(value), suffix)
	}
	return fmt.Sprintf("%s %s %s", FormatCompactCount(value), unit, suffix)
}

// totalRemainingOnly totals the prepaid headroom a set of limits represents. A
// shared pool is observed once per stored credential, so N keys yield N
// identical rows that collapse to one value instead of summing.
func totalRemainingOnly(limits []ai.UsageLimit) (float64, ai.UsageUnit, bool) {
	if len(limits) == 0 {
		return 0, "", false
	}
	unit := limits[0].Amount.Unit
	var total float64
	var sharedMax float64
	hasShared := false
	for _, limit := range limits {
		if !isRemainingOnlyAbsoluteAmount(limit) || limit.Amount.Unit != unit {
			return 0, "", false
		}
		remaining := 0.0
		if limit.Amount.Remaining != nil {
			remaining = *limit.Amount.Remaining
		}
		if limit.Scope.Shared {
			if !hasShared || remaining > sharedMax {
				sharedMax, hasShared = remaining, true
			}
			continue
		}
		total += remaining
	}
	if hasShared {
		total += sharedMax
	}
	return total, unit, true
}

// FormatRemainingOnlyTotal renders `100 credits left` for a bucket of
// remaining-only limits.
func FormatRemainingOnlyTotal(limits []ai.UsageLimit) string {
	total, unit, ok := totalRemainingOnly(limits)
	if !ok {
		return ""
	}
	return formatQuantity(total, unit, "left")
}

// FormatAbsoluteOnlyAmount renders text for a bucket no quota bar can draw:
// spend-to-date or prepaid headroom.
func FormatAbsoluteOnlyAmount(limits []ai.UsageLimit) string {
	if len(limits) == 0 {
		return ""
	}
	unit := limits[0].Amount.Unit
	allUsed := true
	var used float64
	for _, limit := range limits {
		if !IsUsedOnlyAbsoluteAmount(limit) || limit.Amount.Unit != unit {
			allUsed = false
			break
		}
		if limit.Amount.Used != nil && *limit.Amount.Used > used {
			used = *limit.Amount.Used
		}
	}
	if allUsed {
		return formatQuantity(used, unit, "used")
	}
	return FormatRemainingOnlyTotal(limits)
}

// ─── Shared routing collapse ─────────────────────────────────────────────────

func collapseSharedLimits(limits []ai.UsageLimit) []ai.UsageLimit {
	seenGroups := map[string]bool{}
	collapsed := make([]ai.UsageLimit, 0, len(limits))
	changed := false
	for _, limit := range limits {
		group := limit.Scope.SharedGroup
		if group != "" && seenGroups[group] {
			changed = true
			continue
		}
		if group != "" {
			seenGroups[group] = true
		}
		collapsed = append(collapsed, limit)
	}
	if !changed {
		return limits
	}
	return collapsed
}

// CollapseSharedUsageReports collapses routing-specific copies of a shared
// quota for user-facing usage views.
func CollapseSharedUsageReports(reports []ai.UsageReport) []ai.UsageReport {
	collapsed := make([]ai.UsageReport, 0, len(reports))
	changed := false
	for _, report := range reports {
		limits := collapseSharedLimits(report.Limits)
		if len(limits) != len(report.Limits) {
			changed = true
			copied := report
			copied.Limits = limits
			collapsed = append(collapsed, copied)
			continue
		}
		collapsed = append(collapsed, report)
	}
	if !changed {
		return reports
	}
	return collapsed
}

// ─── Codex labels ────────────────────────────────────────────────────────────

func metadataString(report ai.UsageReport, key string) string {
	value, ok := report.Metadata[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func metadataBool(report ai.UsageReport, key string) bool {
	value, ok := report.Metadata[key]
	if !ok {
		return false
	}
	flag, _ := value.(bool)
	return flag
}

// FormatCodexUsageReportLabel qualifies a Codex account label only when a
// sibling report shares its email: multiple workspaces on one login otherwise
// render as identical accounts, so the org id (then account id) disambiguates
// them.
func FormatCodexUsageReportLabel(report ai.UsageReport, peers []ai.UsageReport, base string) string {
	email := metadataString(report, "email")
	collision := false
	if email != "" {
		for _, peer := range peers {
			if peer.Provider == report.Provider && peer.FetchedAt == report.FetchedAt &&
				metadataString(peer, "email") == email && !sameReportIdentity(peer, report) {
				collision = true
				break
			}
		}
	}
	rawOrg := ""
	if collision {
		rawOrg = metadataString(report, "orgId")
		if rawOrg == "" {
			rawOrg = metadataString(report, "accountId")
		}
		if rawOrg == "" {
			rawOrg = metadataString(report, "accountKey")
		}
		if rawOrg == "" {
			rawOrg = "account " + strconv.Itoa(indexOfReport(peers, report)+1)
		}
	}
	suffix := ""
	if rawOrg != "" && rawOrg != base {
		suffix = " (" + SanitizeDisplayLine(rawOrg) + ")"
	}
	return SanitizeDisplayLine(base) + suffix
}

// sameReportIdentity reports whether two reports describe one account: the
// identity fields the usage views disambiguate by.
func sameReportIdentity(left, right ai.UsageReport) bool {
	return left.Provider == right.Provider &&
		metadataString(left, "accountKey") == metadataString(right, "accountKey") &&
		metadataString(left, "accountId") == metadataString(right, "accountId") &&
		metadataString(left, "orgId") == metadataString(right, "orgId")
}

// indexOfReport returns the position of report in peers, or -1.
func indexOfReport(peers []ai.UsageReport, report ai.UsageReport) int {
	for i, peer := range peers {
		if sameReportIdentity(peer, report) && peer.FetchedAt == report.FetchedAt {
			return i
		}
	}
	return -1
}

// FormatUsageAccountLabel labels one account of a provider, disambiguating
// Codex workspaces.
func FormatUsageAccountLabel(report ai.UsageReport, peers []ai.UsageReport, base string) string {
	if report.Provider != "openai-codex" {
		return SanitizeDisplayLine(base)
	}
	return FormatCodexUsageReportLabel(report, peers, base)
}

// ─── Active account identity ─────────────────────────────────────────────────

func normalizeIdentityValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

// LimitMatchesActiveAccount reports whether a usage-limit column belongs to the
// given identity. orgId gates, accountKey is authoritative when both sides carry
// one, accountId mismatches reject, then email decides.
func LimitMatchesActiveAccount(report ai.UsageReport, limit ai.UsageLimit, active *ai.UsageActiveAccount) bool {
	if active == nil {
		return false
	}
	activeEmail := normalizeIdentityValue(active.Email)
	activeAccountID := normalizeIdentityValue(active.AccountID)
	activeAccountKey := ""
	if active.AccountKey != nil {
		activeAccountKey = normalizeIdentityValue(*active.AccountKey)
	}
	activeOrgID := normalizeIdentityValue(active.OrgID)
	reportOrgID := normalizeIdentityValue(metadataString(report, "orgId"))
	if activeOrgID != "" || reportOrgID != "" {
		if activeOrgID != reportOrgID {
			return false
		}
		if activeEmail == "" && activeAccountID == "" && activeAccountKey == "" {
			return true
		}
	}

	reportAccountKey := normalizeIdentityValue(metadataString(report, "accountKey"))
	if activeAccountKey != "" && reportAccountKey != "" && activeAccountKey != reportAccountKey {
		return false
	}

	reportAccountID := normalizeIdentityValue(metadataString(report, "accountId"))
	if reportAccountID == "" {
		reportAccountID = normalizeIdentityValue(metadataString(report, "account_id"))
	}
	if reportAccountID == "" {
		reportAccountID = normalizeIdentityValue(limit.Scope.AccountID)
	}
	if activeAccountID != "" && reportAccountID != "" && activeAccountID != reportAccountID {
		return false
	}

	reportEmail := normalizeIdentityValue(metadataString(report, "email"))
	if activeEmail != "" && reportEmail != "" {
		return activeEmail == reportEmail
	}

	if activeAccountID != "" && reportAccountID == activeAccountID {
		return true
	}
	if activeAccountKey != "" && reportAccountKey == activeAccountKey {
		return true
	}

	reportProjectID := normalizeIdentityValue(metadataString(report, "projectId"))
	if reportProjectID == "" {
		reportProjectID = normalizeIdentityValue(limit.Scope.ProjectID)
	}
	if activeAccountKey != "" {
		return activeAccountKey == reportEmail ||
			activeAccountKey == reportAccountID ||
			activeAccountKey == reportProjectID
	}
	return false
}

// ReportMatchesActiveAccount reports whether any limit column in report belongs
// to the identity.
func ReportMatchesActiveAccount(report ai.UsageReport, active *ai.UsageActiveAccount) bool {
	if active == nil {
		return false
	}
	for _, limit := range report.Limits {
		if LimitMatchesActiveAccount(report, limit, active) {
			return true
		}
	}
	return false
}

// ActiveAccountLabel is the best display label for the active account record:
// email, then account id, then the store key, with the organization appended
// when it adds detail.
func ActiveAccountLabel(active *ai.UsageActiveAccount) string {
	if active == nil {
		return ""
	}
	base := active.Email
	if base == "" {
		base = active.AccountID
	}
	if base == "" && active.AccountKey != nil {
		base = *active.AccountKey
	}
	if base == "" {
		return ""
	}
	org := active.OrgName
	if org == "" {
		org = active.OrgID
	}
	if org != "" && org != base {
		return SanitizeDisplayLine(base) + " (" + SanitizeDisplayLine(org) + ")"
	}
	return SanitizeDisplayLine(base)
}

// ─── Status presentation ─────────────────────────────────────────────────────

// Usage status glyphs used by the board and report.
const (
	UsageGlyphSuccess = "✔"
	UsageGlyphError   = "✘"
	UsageGlyphWarning = "⚠"
	UsageGlyphInfo    = "ⓘ"
	UsageGlyphPending = "⏳"
)

// StatusColorToken maps a quota status to its theme foreground token.
func StatusColorToken(status ai.UsageStatus) string {
	switch status {
	case ai.UsageStatusExhausted:
		return string(ThemeColorError)
	case ai.UsageStatusWarning:
		return string(ThemeColorWarning)
	case ai.UsageStatusOK:
		return string(ThemeColorSuccess)
	}
	return string(ThemeColorDim)
}

// AggregateStatus aggregates status across a bucket's limits, mirroring the
// classic report: a mix of healthy and pressured accounts reads as a warning,
// not as the worst account's status.
func AggregateStatus(statuses []ai.UsageStatus) ai.UsageStatus {
	hasOK, hasWarning, hasExhausted := false, false, false
	for _, status := range statuses {
		switch status {
		case ai.UsageStatusOK:
			hasOK = true
		case ai.UsageStatusWarning:
			hasWarning = true
		case ai.UsageStatusExhausted:
			hasExhausted = true
		}
	}
	switch {
	case hasOK && (hasWarning || hasExhausted):
		return ai.UsageStatusWarning
	case hasOK:
		return ai.UsageStatusOK
	case hasWarning:
		return ai.UsageStatusWarning
	case hasExhausted:
		return ai.UsageStatusExhausted
	}
	return ai.UsageStatusUnknown
}

// StatusWithUnavailableAccounts is the card status when some connected accounts
// reported no usage: the missing report raises the card to a warning but never
// hides an exhausted quota.
func StatusWithUnavailableAccounts(statuses []ai.UsageStatus) ai.UsageStatus {
	if len(statuses) == 0 {
		return ai.UsageStatusUnknown
	}
	if AggregateStatus(statuses) == ai.UsageStatusExhausted {
		return ai.UsageStatusExhausted
	}
	return ai.UsageStatusWarning
}

// ─── Provider cards ──────────────────────────────────────────────────────────

// CardWindowRow is one quota bucket on a provider card: the account-aggregate
// of one window group.
type CardWindowRow struct {
	// Label is the display label (limit label with tier folded in).
	Label string
	// WindowTag is the window label/id shown dim after the label when sibling
	// rows share a label.
	WindowTag string
	// Fraction is the mean used fraction across accounts (0..1, >1 = overage).
	Fraction *float64
	Status   ai.UsageStatus
	// ResetMs is the reset countdown of the worst account, ms from now.
	ResetMs *int64
	// UsedText is an absolute one-sided amount for limits without a fraction.
	UsedText string
}

// UsageUnavailableAccountView is a connected account whose usage lookup
// produced no attributable report. The backend snapshot entry carries extra
// fields (reason, accountKey, ambient) this view ignores.
type UsageUnavailableAccountView struct {
	Provider string
	Label    string
}

// UsagePlanGroup is one provider subscription plan's reports.
type UsagePlanGroup struct {
	Provider            string
	Name                string
	Reports             []ai.UsageReport
	UnavailableAccounts []UsageUnavailableAccountView
}

// GroupUsageReportsByPlan keeps quota totals and account columns separate for
// each provider subscription plan.
func GroupUsageReportsByPlan(reports []ai.UsageReport, unavailable []UsageUnavailableAccountView) []UsagePlanGroup {
	type planKey struct{ provider, plan string }
	order := make([]planKey, 0, len(reports)+len(unavailable))
	groups := map[planKey]*UsagePlanGroup{}
	getPlan := func(provider, plan string) *UsagePlanGroup {
		key := planKey{provider: provider, plan: plan}
		if group, ok := groups[key]; ok {
			return group
		}
		group := &UsagePlanGroup{Provider: provider}
		groups[key] = group
		order = append(order, key)
		return group
	}
	for _, report := range reports {
		group := getPlan(report.Provider, normalizeUsagePlan(metadataString(report, "planType")))
		group.Reports = append(group.Reports, report)
	}
	for _, account := range unavailable {
		group := getPlan(account.Provider, "")
		group.UnavailableAccounts = append(group.UnavailableAccounts, account)
	}
	// A provider with any named plan labels its unnamed bucket "Unknown plan".
	named := map[string]bool{}
	for key := range groups {
		if key.plan != "" {
			named[key.provider] = true
		}
	}
	out := make([]UsagePlanGroup, 0, len(order))
	for _, key := range order {
		group := groups[key]
		label := ""
		switch {
		case key.plan != "":
			label = titleCasePlan(key.plan)
		case named[key.provider]:
			label = "Unknown plan"
		}
		name := SanitizeDisplayLine(FormatProviderName(key.provider))
		if label != "" {
			name += " · " + label
		}
		group.Name = name
		out = append(out, *group)
	}
	return out
}

// normalizeUsagePlan lowercases a provider plan name and strips provider
// prefixes so sibling subscriptions group together.
func normalizeUsagePlan(raw string) string {
	plan := strings.ToLower(SanitizeDisplayLine(raw))
	if rest, ok := strings.CutPrefix(plan, "chatgpt"); ok {
		// Upstream requires at least one separator after the provider prefix:
		// `chatgptplus` names no plan, while `chatgpt-plus` is Plus.
		if trimmed := strings.TrimLeft(rest, " \t_-"); trimmed != rest {
			plan = trimmed
		}
	}
	var b strings.Builder
	b.Grow(len(plan))
	lastUnderscore := false
	for _, char := range plan {
		if char == '_' || char == '-' {
			if !lastUnderscore {
				b.WriteByte(' ')
			}
			lastUnderscore = true
			continue
		}
		lastUnderscore = false
		b.WriteRune(char)
	}
	return b.String()
}

// titleCasePlan upper-cases the first letter of each word.
func titleCasePlan(plan string) string {
	var b strings.Builder
	b.Grow(len(plan))
	upperNext := true
	for _, char := range plan {
		switch {
		case char == ' ':
			upperNext = true
			b.WriteRune(char)
		case upperNext:
			b.WriteString(strings.ToUpper(string(char)))
			upperNext = false
		default:
			b.WriteRune(char)
		}
	}
	return b.String()
}

// ProviderCardResetCredits is the saved-reset summary on a provider card.
type ProviderCardResetCredits struct {
	BankedCount        int
	RedeemableCount    int
	SoonestExpiryMs    *int64
	UnavailableReasons []string
}

// ProviderCard is the compact per-provider summary backing one card in the
// subscriptions grid.
type ProviderCard struct {
	Provider string
	Name     string
	// Accounts counts represented accounts, including unavailable usage lookups.
	Accounts            int
	UnavailableAccounts []string
	Windows             []CardWindowRow
	Unlimited           bool
	Notes               []string
	Idle                bool
	ResetCredits        *ProviderCardResetCredits
	DaybreakAccounts    []string
}

// CardMaxWindows is the number of rows a provider card may show before
// "+N more".
const CardMaxWindows = 4

// idleFraction is the fraction below which a window counts as untouched.
const idleFraction = 0.005

// compactWindowTag renders a compact duration tag for a window (`7d`, `1d`,
// `5h`, `mo`).
func compactWindowTag(window ai.UsageWindow) string {
	if window.DurationMs != nil && *window.DurationMs > 0 {
		hours := float64(*window.DurationMs) / 3600000
		switch {
		case hours >= 28*24:
			return "mo"
		case hours >= 24:
			return strconv.Itoa(int(math.Round(hours/24))) + "d"
		}
		return strconv.Itoa(int(math.Round(hours))) + "h"
	}
	id := strings.ToLower(window.ID)
	if len([]rune(id)) <= 3 {
		return id
	}
	runes := []rune(id)
	return string(runes[:1])
}

// limitFractionValue is the bucket-ordering fraction, -1 when unreported.
func limitFractionValue(limit ai.UsageLimit) float64 {
	fraction, ok := ai.UsageUsedFraction(limit)
	if !ok {
		return -1
	}
	return fraction
}

// BuildProviderCards collapses usage reports into one compact card per provider
// and plan: limits grouped by quota bucket, each bucket showing the mean used
// fraction across accounts with the most-used account's reset countdown. Rows
// keep the provider's declared window order; cards sort by their fullest window
// so what is burning is on top-left; fully idle providers collapse into a tick.
func BuildProviderCards(reports []ai.UsageReport, nowMs int64, unavailable []UsageUnavailableAccountView) []ProviderCard {
	displayReports := CollapseSharedUsageReports(reports)
	grouped := GroupUsageReportsByPlan(displayReports, unavailable)
	cards := make([]ProviderCard, 0, len(grouped))
	for _, group := range grouped {
		missingLabels := make([]string, 0, len(group.UnavailableAccounts))
		for _, account := range group.UnavailableAccounts {
			missingLabels = append(missingLabels, account.Label)
		}

		type bucket struct {
			label  string
			limits []ai.UsageLimit
		}
		buckets := make([]*bucket, 0, len(group.Reports))
		bucketByKey := map[string]*bucket{}
		for _, report := range group.Reports {
			for _, limit := range report.Limits {
				label := FormatLimitTitle(limit)
				key := limit.Scope.SharedGroup
				if key == "" {
					windowID := limit.Scope.WindowID
					if limit.Window != nil && limit.Window.ID != "" {
						windowID = limit.Window.ID
					}
					if windowID == "" {
						windowID = "default"
					}
					key = label + "|" + windowID + "|" + limit.ID
				}
				entry, ok := bucketByKey[key]
				if !ok {
					entry = &bucket{label: label}
					bucketByKey[key] = entry
					buckets = append(buckets, entry)
				}
				entry.limits = append(entry.limits, limit)
			}
		}

		declared := make([]CardWindowRow, 0, len(buckets))
		for _, entry := range buckets {
			var sum float64
			count := 0
			statuses := make([]ai.UsageStatus, 0, len(entry.limits))
			worst := entry.limits[0]
			worstFraction := limitFractionValue(worst)
			for _, limit := range entry.limits {
				if fraction, ok := ai.UsageUsedFraction(limit); ok {
					sum += fraction
					count++
				}
				if value := limitFractionValue(limit); value > worstFraction {
					worst, worstFraction = limit, value
				}
				statuses = append(statuses, limit.Status)
			}
			var fraction *float64
			if count > 0 {
				mean := sum / float64(count)
				fraction = &mean
			}
			var resetMs *int64
			tag := ""
			if worst.Window != nil {
				tag = compactWindowTag(*worst.Window)
				if worst.Window.ResetsAt != nil && *worst.Window.ResetsAt > nowMs {
					delta := *worst.Window.ResetsAt - nowMs
					resetMs = &delta
				}
			}
			row := CardWindowRow{
				Label:     entry.label,
				WindowTag: tag,
				Fraction:  fraction,
				Status:    AggregateStatus(statuses),
				ResetMs:   resetMs,
			}
			if fraction == nil {
				row.UsedText = FormatAbsoluteOnlyAmount(entry.limits)
			}
			declared = append(declared, row)
		}

		// Cards render at most CardMaxWindows rows; keep the fullest visible
		// (still in declared order) so an exhausted bucket never hides.
		visible := map[int]bool{}
		indices := make([]int, len(declared))
		for i := range declared {
			indices[i] = i
		}
		sort.SliceStable(indices, func(a, b int) bool {
			return windowSortFraction(declared[indices[a]]) > windowSortFraction(declared[indices[b]])
		})
		for _, index := range indices {
			if len(visible) >= CardMaxWindows {
				break
			}
			visible[index] = true
		}
		windows := make([]CardWindowRow, 0, len(declared))
		for index, row := range declared {
			if visible[index] {
				windows = append(windows, row)
			}
		}
		for index, row := range declared {
			if !visible[index] {
				windows = append(windows, row)
			}
		}
		// The window tag earns its columns only when sibling rows would
		// otherwise be indistinguishable.
		for i := range windows {
			duplicated := false
			for j := range windows {
				if i != j && windows[j].Label == windows[i].Label {
					duplicated = true
					break
				}
			}
			if !duplicated {
				windows[i].WindowTag = ""
			}
		}

		var resetRows []*UsageResetSummary
		for _, report := range group.Reports {
			summary := SummarizeUsageResetCredits(report.ResetCredits, nowMs)
			if summary != nil && summary.BankedCount > 0 {
				resetRows = append(resetRows, summary)
			}
		}
		bankedCount := 0
		redeemableCount := 0
		expiries := make([]int64, 0, len(resetRows))
		reasons := make([]string, 0, len(resetRows))
		seenReasons := map[string]bool{}
		for _, summary := range resetRows {
			bankedCount += summary.BankedCount
			redeemableCount += summary.RedeemableCount
			if summary.SoonestExpiry != "" {
				if parsed, err := time.Parse(time.RFC3339Nano, summary.SoonestExpiry); err == nil {
					expiries = append(expiries, parsed.UnixMilli())
				}
			}
			if summary.UnavailableReason != "" && !seenReasons[summary.UnavailableReason] {
				seenReasons[summary.UnavailableReason] = true
				reasons = append(reasons, summary.UnavailableReason)
			}
		}
		sort.Slice(expiries, func(i, j int) bool { return expiries[i] < expiries[j] })
		var soonestResetExpiry *int64
		for _, expiry := range expiries {
			if expiry > nowMs {
				value := expiry
				soonestResetExpiry = &value
				break
			}
		}
		if soonestResetExpiry == nil && len(expiries) > 0 {
			value := expiries[len(expiries)-1]
			soonestResetExpiry = &value
		}
		var resetCredits *ProviderCardResetCredits
		if bankedCount > 0 {
			resetCredits = &ProviderCardResetCredits{
				BankedCount:        bankedCount,
				RedeemableCount:    redeemableCount,
				UnavailableReasons: reasons,
			}
			if soonestResetExpiry != nil {
				delta := *soonestResetExpiry - nowMs
				resetCredits.SoonestExpiryMs = &delta
			}
		}

		daybreakAccounts := make([]string, 0, 1)
		for index, report := range group.Reports {
			if !metadataBool(report, "daybreak") {
				continue
			}
			label := metadataString(report, "email")
			if label == "" {
				label = metadataString(report, "accountId")
			}
			if label == "" {
				label = "account " + strconv.Itoa(index+1)
			}
			daybreakAccounts = append(daybreakAccounts, label)
		}

		notes := make([]string, 0, len(group.Reports))
		seenNotes := map[string]bool{}
		for _, report := range group.Reports {
			for _, note := range report.Notes {
				if !seenNotes[note] {
					seenNotes[note] = true
					notes = append(notes, note)
				}
			}
		}

		idle := len(missingLabels) == 0 && resetCredits == nil && len(daybreakAccounts) == 0
		if idle {
			for _, window := range windows {
				if window.Fraction == nil || *window.Fraction >= idleFraction {
					idle = false
					break
				}
			}
		}

		cards = append(cards, ProviderCard{
			Provider:            group.Provider,
			Name:                group.Name,
			Accounts:            len(group.Reports) + len(missingLabels),
			UnavailableAccounts: missingLabels,
			Windows:             windows,
			Unlimited:           len(windows) == 0 && len(missingLabels) == 0,
			Notes:               notes,
			Idle:                idle,
			ResetCredits:        resetCredits,
			DaybreakAccounts:    daybreakAccounts,
		})
	}

	worstByCard := make(map[int]float64, len(cards))
	for index, card := range cards {
		worst := -1.0
		for _, window := range card.Windows {
			if window.Fraction != nil && *window.Fraction > worst {
				worst = *window.Fraction
			}
		}
		worstByCard[index] = worst
	}
	// Sort cards by their fullest window, then by name.
	positions := make([]int, len(cards))
	for i := range positions {
		positions[i] = i
	}
	sort.SliceStable(positions, func(a, b int) bool {
		left, right := worstByCard[positions[a]], worstByCard[positions[b]]
		if left != right {
			return right < left
		}
		return cardNameLess(cards[positions[a]].Name, cards[positions[b]].Name)
	})
	sorted := make([]ProviderCard, 0, len(cards))
	for _, position := range positions {
		sorted = append(sorted, cards[position])
	}
	return sorted
}

// windowSortFraction orders declared rows by fullness for the visible cap.
func windowSortFraction(row CardWindowRow) float64 {
	if row.Fraction == nil {
		return -1
	}
	return *row.Fraction
}

// cardNameLess compares card names the way localeCompare does for ASCII names.
func cardNameLess(left, right string) bool { return left < right }
