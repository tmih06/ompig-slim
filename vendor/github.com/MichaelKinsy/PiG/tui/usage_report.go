package tui

// Ports packages/coding-agent/src/modes/interactive/components/usage-report.ts:
// the detailed per-account quota report rendered inside the usage dashboard's
// Details view. The dashboard frame supplies the title and freshness line.

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

const (
	usageReportBarWidthMax = 24
	// usageReportColumnInlineMin is the narrowest inline account column that
	// still shows a readable label prefix. Below it the block stacks label over
	// bar instead of abbreviating every account to the same `a…st`.
	usageReportColumnInlineMin = 10
)

// usageDisplayStatus is a limit status plus the report's own neutral state.
type usageDisplayStatus string

const usageStatusNeutral usageDisplayStatus = "neutral"

// UsageReportOptions configures RenderUsageReports.
type UsageReportOptions struct {
	Reports []ai.UsageReport
	// NowMs is the reference time for reset countdowns and freshness values.
	NowMs int64
	// Width is the available content width in terminal cells.
	Width int
	// UnavailableAccounts are accounts whose usage lookup produced no report.
	UnavailableAccounts []UsageUnavailableAccountView
	// ResolveActiveAccount reports the identity of the account this session
	// uses, per provider; nil is unknown.
	ResolveActiveAccount func(provider string) *ai.UsageActiveAccount
}

// usageAccountColumn is one account cell in a bucket. Missing limits remain as
// empty cells so an account keeps the same horizontal position across windows.
type usageAccountColumn struct {
	limit    ai.UsageLimit
	report   ai.UsageReport
	rank     float64 // Worst-use order shared by every quota window.
	position int     // Original index in the provider's report list.
	present  bool    // False for a reserved cell when this account lacks the bucket.
}

// padUsageColumn pads (never truncates) to an exact visible width.
func padUsageColumn(text string, width int) string {
	visible := widthx.VisibleWidth(text)
	if visible >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visible)
}

// usageOrgSuffix renders the organization that distinguishes sibling accounts.
func usageOrgSuffix(report ai.UsageReport) string {
	org := metadataString(report, "orgName")
	if org == "" {
		org = metadataString(report, "orgId")
	}
	if org == "" {
		return ""
	}
	return " (" + SanitizeDisplayLine(org) + ")"
}

// usageReportIdentity picks the account label identity for a report.
func usageReportIdentity(report ai.UsageReport, projectFallback bool) string {
	if email := metadataString(report, "email"); email != "" {
		return email
	}
	if accountID := metadataString(report, "accountId"); accountID != "" {
		return accountID
	}
	if projectFallback {
		return metadataString(report, "projectId")
	}
	return ""
}

// formatUsageReportAccountLabel is the stable per-account label: email, then
// account id, then project id, then positional. Codex labels gain the workspace
// suffix when sibling reports share an email.
func formatUsageReportAccountLabel(
	limit ai.UsageLimit,
	report ai.UsageReport,
	peers []ai.UsageReport,
	index int,
) string {
	identity := usageReportIdentity(report, false)
	if identity == "" {
		identity = limit.Scope.AccountID
		if identity == "" {
			identity = limit.Scope.ProjectID
		}
	}
	base := ""
	switch {
	case identity == "":
		base = "account " + strconv.Itoa(index+1)
	case report.Provider == "openai-codex":
		// Codex labels carry their workspace suffix from live report metadata,
		// so the generic org suffix must not pre-empt it.
		base = SanitizeDisplayLine(identity)
	default:
		base = SanitizeDisplayLine(identity) + usageOrgSuffix(report)
	}
	return FormatUsageAccountLabel(report, peers, base)
}

// formatUsageReportLabel is the provider-level label for a report.
func formatUsageReportLabel(report ai.UsageReport, peers []ai.UsageReport, index int) string {
	identity := usageReportIdentity(report, true)
	base := ""
	switch {
	case identity == "":
		base = "account " + strconv.Itoa(index+1)
	case report.Provider == "openai-codex":
		base = SanitizeDisplayLine(identity)
	default:
		base = SanitizeDisplayLine(identity) + usageOrgSuffix(report)
	}
	return FormatUsageAccountLabel(report, peers, base)
}

// formatUsageResetShort renders a reset countdown, or "" when the recorded reset
// is already in the past. Codex returns the prior window's reset_at until a new
// request opens a fresh window, so a negative delta would be meaningless.
func formatUsageResetShort(limit ai.UsageLimit, nowMs int64) string {
	if limit.Window == nil || limit.Window.ResetsAt == nil {
		return ""
	}
	resetsAt := *limit.Window.ResetsAt
	if resetsAt <= nowMs {
		return ""
	}
	return FormatDuration(resetsAt - nowMs)
}

// formatUsageWindowSuffix renders the window label dim unless the bucket label
// already names it.
func formatUsageWindowSuffix(theme *Theme, label, windowLabel string) string {
	normalizedLabel := strings.ToLower(label)
	normalizedWindow := strings.ToLower(windowLabel)
	if normalizedWindow == "quota window" || strings.Contains(normalizedLabel, normalizedWindow) {
		return ""
	}
	return theme.Fg(string(ThemeColorDim), "("+windowLabel+")")
}

// formatUsageAccountHeaderRow renders per-account header cells: `● email (2h)`
// on one row above the bars, padded to the bar column so each account stays in
// the same column on every row.
func formatUsageAccountHeaderRow(
	theme *Theme,
	accounts []usageAccountColumn,
	peers []ai.UsageReport,
	startIndex int,
	nowMs int64,
	columnWidth int,
	active *ai.UsageActiveAccount,
) []string {
	type part struct {
		label    string
		suffix   string
		active   bool
		daybreak bool
		present  bool
	}
	parts := make([]part, 0, len(accounts))
	maxSuffixWidth := 0
	for index, account := range accounts {
		if !account.present {
			parts = append(parts, part{})
			continue
		}
		isActive := ReportMatchesActiveAccount(account.report, active)
		label := formatUsageReportAccountLabel(account.limit, account.report, peers, startIndex+index)
		if isActive {
			label = "● " + label
		}
		suffix := formatUsageResetShort(account.limit, nowMs)
		if width := widthx.VisibleWidth(suffix); width > maxSuffixWidth {
			maxSuffixWidth = width
		}
		parts = append(parts, part{
			label:    label,
			suffix:   suffix,
			active:   isActive,
			daybreak: metadataBool(account.report, "daybreak"),
			present:  true,
		})
	}
	gap := 0
	if maxSuffixWidth > 0 {
		gap = 1
	}
	prefixBudget := columnWidth - maxSuffixWidth - gap
	out := make([]string, 0, len(parts))

	// If the suffix cannot share the cell with at least `x…`, fall back to
	// truncating the whole label.
	if prefixBudget < 2 {
		for _, item := range parts {
			if !item.present {
				out = append(out, strings.Repeat(" ", columnWidth))
				continue
			}
			full := item.label
			if item.daybreak {
				full += " daybreak"
			}
			if item.suffix != "" {
				full += " " + item.suffix
			}
			cell := padUsageColumn(widthx.TruncateToWidth(full, columnWidth, "…", false), columnWidth)
			if item.active {
				cell = theme.Fg(string(ThemeColorAccent), cell)
			}
			out = append(out, cell)
		}
		return out
	}

	for _, item := range parts {
		if !item.present {
			out = append(out, strings.Repeat(" ", columnWidth))
			continue
		}
		// Keep the full badge visible by taking its columns from the label.
		badge := ""
		if item.daybreak && prefixBudget >= 10 {
			badge = " daybreak"
		}
		label := widthx.TruncateToWidth(item.label, prefixBudget-widthx.VisibleWidth(badge), "…", false)
		prefix := label
		if item.active {
			prefix = theme.Fg(string(ThemeColorAccent), label)
		}
		if badge != "" {
			prefix += theme.Fg(string(ThemeColorSuccess), badge)
		}
		prefixCell := prefix + strings.Repeat(" ", max(0, prefixBudget-widthx.VisibleWidth(prefix)))
		if item.suffix == "" {
			out = append(out, prefixCell+strings.Repeat(" ", maxSuffixWidth+gap))
			continue
		}
		suffixPad := strings.Repeat(" ", max(0, maxSuffixWidth-widthx.VisibleWidth(item.suffix)))
		out = append(out, prefixCell+" "+suffixPad+theme.Fg(string(ThemeColorDim), item.suffix))
	}
	return out
}

// usageStatusIcon renders the glyph for a bucket status.
func usageStatusIcon(theme *Theme, status usageDisplayStatus) string {
	switch status {
	case usageStatusNeutral:
		return theme.Fg(string(ThemeColorDim), UsageGlyphInfo)
	case usageDisplayStatus(ai.UsageStatusExhausted):
		return theme.Fg(string(ThemeColorError), UsageGlyphError)
	case usageDisplayStatus(ai.UsageStatusWarning):
		return theme.Fg(string(ThemeColorWarning), UsageGlyphWarning)
	case usageDisplayStatus(ai.UsageStatusOK):
		return theme.Fg(string(ThemeColorSuccess), UsageGlyphSuccess)
	}
	return theme.Fg(string(ThemeColorDim), UsageGlyphPending)
}

// resolveUsageAggregateStatus maps an aggregate limit status to the report's
// display status, marking absolute-only buckets neutral.
func resolveUsageAggregateStatus(limits []ai.UsageLimit) usageDisplayStatus {
	statuses := make([]ai.UsageStatus, 0, len(limits))
	for _, limit := range limits {
		statuses = append(statuses, limit.Status)
	}
	status := AggregateStatus(statuses)
	if status == ai.UsageStatusUnknown && len(limits) > 0 {
		allUsedOnly := true
		for _, limit := range limits {
			if !IsUsedOnlyAbsoluteAmount(limit) {
				allUsedOnly = false
				break
			}
		}
		if allUsedOnly {
			return usageStatusNeutral
		}
	}
	return usageDisplayStatus(status)
}

// formatUsagePercentFree renders `75% free` with up to one fraction digit.
func formatUsagePercentFree(value float64) string {
	return strconv.FormatFloat(math.Round(value*10)/10, 'f', -1, 64) + "% free"
}

// formatUsageAggregateAmount is the provider-level column summary: `% free`
// when every limit carries a fraction or amount.
func formatUsageAggregateAmount(limits []ai.UsageLimit) string {
	fractions := make([]float64, 0, len(limits))
	for _, limit := range limits {
		if fraction, ok := ai.UsageUsedFraction(limit); ok {
			fractions = append(fractions, fraction)
		}
	}
	if len(fractions) == len(limits) && len(fractions) > 0 {
		var sum float64
		for _, fraction := range fractions {
			sum += fraction
		}
		return formatUsagePercentFree(max(0, ((float64(len(limits))-sum)/float64(len(limits)))*100))
	}

	totalUsed, totalLimit := 0.0, 0.0
	amounts := 0
	for _, limit := range limits {
		if limit.Amount.Used != nil && limit.Amount.Limit != nil && *limit.Amount.Limit > 0 {
			totalUsed += *limit.Amount.Used
			totalLimit += *limit.Amount.Limit
			amounts++
		}
	}
	if amounts == len(limits) && amounts > 0 {
		if totalLimit > 0 {
			return formatUsagePercentFree(max(0, 100-(totalUsed/totalLimit)*100))
		}
		return formatUsagePercentFree(0)
	}

	if len(limits) > 0 {
		allUsedOnly := true
		for _, limit := range limits {
			if !IsUsedOnlyAbsoluteAmount(limit) {
				allUsedOnly = false
				break
			}
		}
		if allUsedOnly {
			return ""
		}
	}

	if remaining := FormatRemainingOnlyTotal(limits); remaining != "" {
		return remaining
	}

	uniqueAccountIDs := map[string]bool{}
	for _, limit := range limits {
		if limit.Scope.AccountID != "" {
			uniqueAccountIDs[limit.Scope.AccountID] = true
		}
	}
	if len(uniqueAccountIDs) > 0 {
		if len(uniqueAccountIDs) == 1 {
			return "1 acct"
		}
		return strconv.Itoa(len(uniqueAccountIDs)) + " accts"
	}
	return strconv.Itoa(len(limits)) + " accts"
}

// resolveUsageResetRange renders `resets in 2h` when one window, a range when
// the accounts' windows disagree.
func resolveUsageResetRange(limits []ai.UsageLimit, nowMs int64) string {
	type reset struct {
		label  string
		offset int64
	}
	resets := make([]reset, 0, len(limits))
	labels := map[string]bool{}
	for _, limit := range limits {
		if limit.Window == nil || limit.Window.ResetsAt == nil {
			continue
		}
		resetsAt := *limit.Window.ResetsAt
		if resetsAt <= nowMs {
			continue
		}
		label := limit.Window.ResetLabel
		if label == "" {
			label = "resets"
		}
		labels[label] = true
		resets = append(resets, reset{label: label, offset: resetsAt - nowMs})
	}
	if len(resets) == 0 {
		return ""
	}
	verb := "resets"
	if len(labels) == 1 {
		for label := range labels {
			verb = label
		}
	}
	minReset, maxReset := resets[0].offset, resets[0].offset
	for _, item := range resets[1:] {
		if item.offset < minReset {
			minReset = item.offset
		}
		if item.offset > maxReset {
			maxReset = item.offset
		}
	}
	if maxReset-minReset > 60000 {
		return verb + " in " + FormatDuration(minReset) + "–" + FormatDuration(maxReset)
	}
	return verb + " in " + FormatDuration(minReset)
}

// RenderUsageBar draws one account's quota bar.
func RenderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
	if limit.Amount.Used != nil && IsUsedOnlyAbsoluteAmount(limit) {
		used := ""
		if limit.Amount.Unit == ai.UsageUnitUSD {
			used = "$" + strconv.FormatFloat(*limit.Amount.Used, 'f', 2, 64)
		} else {
			used = strconv.FormatFloat(math.Round(*limit.Amount.Used*100)/100, 'f', -1, 64) + " " + string(limit.Amount.Unit)
		}
		return theme.Fg(string(ThemeColorDim), widthx.TruncateToWidth(used+" used", barWidth, "…", false))
	}
	fraction, ok := ai.UsageUsedFraction(limit)
	if !ok {
		return theme.Fg(string(ThemeColorDim), strings.Repeat("·", barWidth))
	}
	clamped := math.Min(math.Max(fraction, 0), 1)
	exact := clamped * float64(barWidth)
	fullCells := int(math.Floor(exact))
	remainder := exact - float64(fullCells)
	partial := ""
	switch {
	case remainder >= 2.0/3.0:
		partial = "▓"
	case remainder >= 1.0/3.0:
		partial = "▒"
	}
	leading := strings.Repeat("█", fullCells) + partial
	filled := fullCells
	if partial != "" {
		filled++
	}
	empty := strings.Repeat("░", max(0, barWidth-filled))
	return theme.Fg(StatusColorToken(limit.Status), leading) + theme.Fg(string(ThemeColorDim), empty)
}

// usageAccountBlock is one width-fitting slice of a bucket's account columns.
type usageAccountBlock struct {
	start       int
	end         int
	columnWidth int
	barWidth    int
	// stacked prints label and bar on separate rows when even one column
	// cannot fit.
	stacked bool
}

// layoutUsageAccountBlocks splits a bucket's account columns into width-fitting
// blocks. The upstream single-row table is kept whenever every account column
// fits; otherwise accounts wrap into consecutive blocks repeating the header,
// bars, and per-account values, and below one column's minimum width each
// account stacks label over bar. No account is ever dropped horizontally.
func layoutUsageAccountBlocks(count, available, trailing int) []usageAccountBlock {
	blocks := make([]usageAccountBlock, 0, count)
	// Reserve the aggregate's columns in every block; it prints once at the end.
	budget := available - 2
	if trailing > 0 {
		budget -= trailing + 1
	}
	maxPerBlock := (budget + 1) / (usageReportColumnInlineMin + 1)
	if maxPerBlock < 1 {
		barWidth := max(1, budget)
		for index := 0; index < count; index++ {
			blocks = append(blocks, usageAccountBlock{
				start:       index,
				end:         index + 1,
				columnWidth: barWidth,
				barWidth:    barWidth,
				stacked:     true,
			})
		}
		return blocks
	}
	for start := 0; start < count; start += maxPerBlock {
		perBlock := min(maxPerBlock, count-start)
		columnWidth := (budget - (perBlock - 1)) / perBlock
		blocks = append(blocks, usageAccountBlock{
			start:       start,
			end:         start + perBlock,
			columnWidth: columnWidth,
			barWidth:    min(columnWidth, usageReportBarWidthMax),
		})
	}
	return blocks
}

// resolveProviderUsageTotal sums every account limit's used fraction.
func resolveProviderUsageTotal(reports []ai.UsageReport) float64 {
	var sum float64
	for _, report := range reports {
		for _, limit := range report.Limits {
			if fraction, ok := ai.UsageUsedFraction(limit); ok {
				sum += fraction
			}
		}
	}
	return sum
}

// RenderUsageReports renders the complete per-account usage report. Every
// provider, quota bucket, and account row is included so the dashboard can
// scroll the whole list; width is the content width the caller can draw.
func RenderUsageReports(options UsageReportOptions) string {
	theme := ActiveTheme()
	availableWidth := max(1, options.Width)
	nowMs := options.NowMs
	displayReports := CollapseSharedUsageReports(options.Reports)
	lines := make([]string, 0, 64)

	type providerEntry struct {
		group      UsagePlanGroup
		totalUsage float64
	}
	providerEntries := make([]providerEntry, 0, len(displayReports))
	for _, group := range GroupUsageReportsByPlan(displayReports, options.UnavailableAccounts) {
		providerEntries = append(providerEntries, providerEntry{group: group, totalUsage: resolveProviderUsageTotal(group.Reports)})
	}
	// Least-used providers first, then by name.
	for i := 1; i < len(providerEntries); i++ {
		for j := i; j > 0; j-- {
			left, right := providerEntries[j-1], providerEntries[j]
			if left.totalUsage != right.totalUsage {
				if left.totalUsage < right.totalUsage {
					break
				}
				providerEntries[j-1], providerEntries[j] = right, left
				continue
			}
			if left.group.Name <= right.group.Name {
				break
			}
			providerEntries[j-1], providerEntries[j] = right, left
		}
	}

	for _, entry := range providerEntries {
		group := entry.group
		providerReports := group.Reports
		lines = append(lines, "")

		var activeAccount *ai.UsageActiveAccount
		if options.ResolveActiveAccount != nil {
			activeAccount = options.ResolveActiveAccount(group.Provider)
		}

		type limitGroup struct {
			label       string
			windowLabel string
			accounts    []usageAccountColumn
		}
		limitGroups := make([]*limitGroup, 0, 8)
		groupByKey := map[string]*limitGroup{}
		accountRanks := make([]float64, len(providerReports))
		accountOrder := make([]int, 0, len(providerReports))
		for position, report := range providerReports {
			if len(report.Limits) > 0 {
				accountOrder = append(accountOrder, position)
			}
		}
		// Rank limited accounts once and preserve empty cells for missing windows,
		// so every bucket maps each account to the same column.
		for position, report := range providerReports {
			worst := -1.0
			for _, limit := range report.Limits {
				if fraction, ok := ai.UsageUsedFraction(limit); ok && fraction > worst {
					worst = fraction
				}
			}
			rank := -worst*1000 + float64(position)
			accountRanks[position] = rank
			for _, limit := range report.Limits {
				windowID := limit.Scope.WindowID
				if limit.Window != nil && limit.Window.ID != "" {
					windowID = limit.Window.ID
				}
				if windowID == "" {
					windowID = "default"
				}
				key := limit.Scope.SharedGroup
				if key == "" {
					key = FormatLimitTitle(limit) + "|" + windowID + "|" + limit.ID
				}
				windowLabel := windowID
				if limit.Window != nil && limit.Window.Label != "" {
					windowLabel = limit.Window.Label
				}
				limitGroupEntry, ok := groupByKey[key]
				if !ok {
					limitGroupEntry = &limitGroup{
						label:       SanitizeDisplayLine(FormatLimitTitle(limit)),
						windowLabel: SanitizeDisplayLine(windowLabel),
					}
					groupByKey[key] = limitGroupEntry
					limitGroups = append(limitGroups, limitGroupEntry)
				}
				limitGroupEntry.accounts = append(limitGroupEntry.accounts, usageAccountColumn{
					limit: limit, report: report, rank: rank, position: position, present: true,
				})
			}
		}
		for index := 1; index < len(accountOrder); index++ {
			for current := index; current > 0 &&
				accountRanks[accountOrder[current-1]] > accountRanks[accountOrder[current]]; current-- {
				accountOrder[current-1], accountOrder[current] = accountOrder[current], accountOrder[current-1]
			}
		}
		columnByPosition := make([]int, len(providerReports))
		for column, position := range accountOrder {
			columnByPosition[position] = column
		}

		lines = append(lines, theme.Bold(theme.Fg(string(ThemeColorAccent), group.Name)))
		var activeReport *ai.UsageReport
		if activeAccount != nil {
			for i := range providerReports {
				if ReportMatchesActiveAccount(providerReports[i], activeAccount) {
					activeReport = &providerReports[i]
					break
				}
			}
		}
		if activeReport != nil {
			label := formatUsageReportLabel(*activeReport, providerReports, indexOfUsageReport(providerReports, *activeReport))
			lines = append(lines, "  "+theme.Fg(string(ThemeColorAccent), "in use by this session:")+" "+SanitizeDisplayLine(label))
		}
		for _, account := range group.UnavailableAccounts {
			label := SanitizeDisplayLine(account.Label)
			status := " — usage unavailable"
			bounded := widthx.TruncateToWidth(label, max(0, availableWidth-2-widthx.VisibleWidth(status)), "…", false)
			lines = append(lines, "  "+theme.Fg(string(ThemeColorDim),
				widthx.TruncateToWidth(bounded+status, availableWidth, "…", false)))
		}

		// Provider-wide disclaimers render once above the per-account sections.
		providerNotes := make([]string, 0, 2)
		seenNotes := map[string]bool{}
		for _, report := range providerReports {
			for _, note := range report.Notes {
				if !seenNotes[note] {
					seenNotes[note] = true
					providerNotes = append(providerNotes, note)
				}
			}
		}
		if len(providerNotes) > 0 {
			cleaned := make([]string, 0, len(providerNotes))
			for _, note := range providerNotes {
				cleaned = append(cleaned, SanitizeDisplayLine(note))
			}
			text := strings.Join(cleaned, " • ")
			lines = append(lines, strings.TrimRight("  "+theme.Fg(string(ThemeColorDim),
				widthx.TruncateToWidth(text, availableWidth, "…", false)), " "))
		}

		resetLines := make([]string, 0, 4)
		for index, report := range providerReports {
			resets := SummarizeUsageResetCredits(report.ResetCredits, nowMs)
			if resets == nil || resets.BankedCount <= 0 {
				continue
			}
			label := SanitizeDisplayLine(formatUsageReportLabel(report, providerReports, index))
			isActive := activeReport != nil && sameUsageReport(*activeReport, report)
			availability := ""
			if resets.RedeemableCount != resets.BankedCount {
				availability = " · " + strconv.Itoa(resets.RedeemableCount) + " usable now"
			}
			plural := "s"
			if resets.BankedCount == 1 {
				plural = ""
			}
			line := "    • " + label + ": " + strconv.Itoa(resets.BankedCount) + " saved reset" + plural + availability
			if isActive {
				line += " (active)"
			}
			resetLines = append(resetLines, line)
			if resets.SoonestExpiry != "" {
				expiry, err := time.Parse(time.RFC3339Nano, resets.SoonestExpiry)
				expiryDate := resets.SoonestExpiry
				if len(expiryDate) > 10 {
					expiryDate = expiryDate[:10]
				}
				if err == nil && expiry.UnixMilli() > nowMs {
					resetLines = append(resetLines, "        soonest expires in "+
						FormatDuration(expiry.UnixMilli()-nowMs)+" ("+expiryDate+")")
				} else {
					resetLines = append(resetLines, "        expired ("+expiryDate+")")
				}
			}
			if resets.RedeemableCount == 0 && resets.UnavailableReason != "" {
				resetLines = append(resetLines, "        unavailable: "+SanitizeDisplayLine(resets.UnavailableReason))
			}
		}
		if len(resetLines) > 0 {
			lines = append(lines, "  "+theme.Fg(string(ThemeColorAccent), "Saved rate-limit resets"))
			for _, line := range resetLines {
				lines = append(lines, theme.Fg(string(ThemeColorDim),
					widthx.TruncateToWidth(line, availableWidth, "…", false)))
			}
		}

		for _, limitGroupEntry := range limitGroups {
			accounts := limitGroupEntry.accounts
			// The aggregate uses only accounts with this bucket; the displayed
			// slots also reserve missing accounts to keep columns stable.
			for i := 1; i < len(accounts); i++ {
				for j := i; j > 0 && accounts[j-1].rank > accounts[j].rank; j-- {
					accounts[j-1], accounts[j] = accounts[j], accounts[j-1]
				}
			}
			sortedLimits := make([]ai.UsageLimit, 0, len(accounts))
			// Only reports with quota limits participate in these columns. Accounts
			// without limits are rendered separately below and must not consume a
			// blank slot that narrows or shifts the visible quota bars.
			accountColumns := make([]usageAccountColumn, len(accountOrder))
			for _, account := range accounts {
				sortedLimits = append(sortedLimits, account.limit)
				accountColumns[columnByPosition[account.position]] = account
			}
			amountText := formatUsageAggregateAmount(sortedLimits)
			status := resolveUsageAggregateStatus(sortedLimits)
			windowSuffix := formatUsageWindowSuffix(theme, limitGroupEntry.label, limitGroupEntry.windowLabel)
			header := usageStatusIcon(theme, status) + " " + theme.Bold(limitGroupEntry.label)
			if windowSuffix != "" {
				header += " " + windowSuffix
			}
			lines = append(lines, strings.TrimRight(header, " "))
			blocks := layoutUsageAccountBlocks(len(accountColumns), availableWidth, widthx.VisibleWidth(amountText))
			wrapped := len(blocks) > 1 || (len(blocks) == 1 && blocks[0].stacked)
			for blockIndex, block := range blocks {
				blockAccounts := accountColumns[min(block.start, len(accountColumns)):min(block.end, len(accountColumns))]
				blockHasAccount := false
				for _, account := range blockAccounts {
					if account.present {
						blockHasAccount = true
						break
					}
				}
				if !blockHasAccount {
					continue
				}
				if blockIndex > 0 && !block.stacked {
					// Wrapped blocks name their range so a reader can tell one
					// page of accounts from the next.
					lines = append(lines, "  "+theme.Fg(string(ThemeColorDim),
						"⋯ accounts "+strconv.Itoa(block.start+1)+"–"+strconv.Itoa(block.end)+
							" of "+strconv.Itoa(len(sortedLimits))))
				}
				if block.stacked {
					// One column cannot fit beside its neighbours: each account
					// prints its own label, bar, and remaining value.
					for index, account := range blockAccounts {
						if !account.present {
							continue
						}
						label := formatUsageReportAccountLabel(account.limit, account.report, providerReports, block.start+index)
						lines = append(lines, strings.TrimRight("  "+label, " "))
						suffix := ""
						if fraction, ok := ai.UsageUsedFraction(account.limit); ok {
							suffix = " " + theme.Fg(StatusColorToken(account.limit.Status),
								formatUsagePercentFree((1-math.Min(math.Max(fraction, 0), 1))*100))
						}
						lines = append(lines, strings.TrimRight(
							"  "+RenderUsageBar(theme, account.limit, block.barWidth)+suffix, " "))
					}
					continue
				}
				headers := formatUsageAccountHeaderRow(
					theme, blockAccounts, providerReports, block.start, nowMs, block.columnWidth, activeAccount)
				lines = append(lines, strings.TrimRight("  "+strings.Join(headers, " "), " "))
				bars := make([]string, 0, len(blockAccounts))
				for _, account := range blockAccounts {
					if !account.present {
						bars = append(bars, strings.Repeat(" ", block.columnWidth))
						continue
					}
					bars = append(bars, padUsageColumn(RenderUsageBar(theme, account.limit, block.barWidth), block.columnWidth))
				}
				barLine := "  " + strings.Join(bars, " ")
				if blockIndex == 0 && !wrapped && amountText != "" {
					barLine += " " + amountText
				}
				lines = append(lines, strings.TrimRight(barLine, " "))
				if wrapped {
					// Wrapped buckets lose the single trailing aggregate, so each
					// block repeats its own per-account values underneath.
					values := make([]string, 0, len(blockAccounts))
					for _, account := range blockAccounts {
						if !account.present {
							values = append(values, strings.Repeat(" ", block.columnWidth))
							continue
						}
						text := "--"
						if fraction, ok := ai.UsageUsedFraction(account.limit); ok {
							text = formatUsagePercentFree((1 - math.Min(math.Max(fraction, 0), 1)) * 100)
						}
						values = append(values, padUsageColumn(
							theme.Fg(StatusColorToken(account.limit.Status), text), block.columnWidth))
					}
					lines = append(lines, strings.TrimRight("  "+strings.Join(values, " "), " "))
				}
			}
			if wrapped && amountText != "" {
				lines = append(lines, "  "+amountText)
			}
			if len(sortedLimits) <= 1 {
				if resetText := resolveUsageResetRange(sortedLimits, nowMs); resetText != "" {
					lines = append(lines, strings.TrimRight("  "+theme.Fg(string(ThemeColorDim), resetText), " "))
				}
			}
			notes := make([]string, 0, 2)
			seen := map[string]bool{}
			for _, limit := range sortedLimits {
				for _, note := range limit.Notes {
					if !seen[note] {
						seen[note] = true
						notes = append(notes, note)
					}
				}
			}
			if len(notes) > 0 {
				cleaned := make([]string, 0, len(notes))
				for _, note := range notes {
					cleaned = append(cleaned, SanitizeDisplayText(note))
				}
				text := SanitizeDisplayLine(strings.Join(cleaned, " • "))
				lines = append(lines, strings.TrimRight("  "+theme.Fg(string(ThemeColorDim),
					widthx.TruncateToWidth(text, availableWidth, "…", false)), " "))
			}
		}

		// Accounts with no rate limits (e.g. business/enterprise plans).
		for index, report := range providerReports {
			if len(report.Limits) != 0 {
				continue
			}
			label := formatUsageReportLabel(report, providerReports, index)
			daybreakSuffix := ""
			if metadataBool(report, "daybreak") {
				daybreakSuffix = theme.Fg(string(ThemeColorSuccess), " daybreak")
			}
			lines = append(lines, theme.Fg(string(ThemeColorSuccess), UsageGlyphSuccess)+" "+label+daybreakSuffix+
				" "+theme.Fg(string(ThemeColorDim), "-- no limits"))
		}
	}

	if len(lines) == 0 {
		return theme.Fg(string(ThemeColorDim), "No usage data available.")
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if widthx.VisibleWidth(line) > availableWidth {
			line = widthx.TruncateToWidth(line, availableWidth, "…", false)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// indexOfUsageReport returns the position of report within reports.
func indexOfUsageReport(reports []ai.UsageReport, report ai.UsageReport) int {
	for index, candidate := range reports {
		if sameUsageReport(candidate, report) {
			return index
		}
	}
	return -1
}

// sameUsageReport reports whether two reports describe the same account record.
func sameUsageReport(left, right ai.UsageReport) bool {
	return left.Provider == right.Provider &&
		left.FetchedAt == right.FetchedAt &&
		metadataString(left, "accountKey") == metadataString(right, "accountKey") &&
		metadataString(left, "email") == metadataString(right, "email") &&
		metadataString(left, "accountId") == metadataString(right, "accountId") &&
		metadataString(left, "orgId") == metadataString(right, "orgId")
}
