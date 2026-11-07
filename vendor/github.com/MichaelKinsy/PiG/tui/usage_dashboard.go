package tui

// Ports packages/coding-agent/src/modes/interactive/components/usage-dashboard.ts:
// the fullscreen /usage board. A subscriptions grid (one card per provider and
// plan, mean used fraction per quota window) sits above a daily activity
// heatmap; the configurable toggle key switches to the classic per-account
// report, scrollable in place. The host mounts this as an overlay; the
// component owns its frame, scrolling, refresh, and abort-on-close behavior.

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// UsageDashboardSnapshot is the quota snapshot a dashboard refresh produced.
// It is ai.UsageSnapshot, with unavailable entries narrowed to what the board
// draws so the host passes the backend snapshot through unchanged.
type UsageDashboardSnapshot struct {
	Reports             []ai.UsageReport
	UnavailableAccounts []UsageUnavailableAccountView
}

// NewUsageDashboardSnapshot narrows a backend snapshot for the dashboard.
func NewUsageDashboardSnapshot(snapshot ai.UsageSnapshot) UsageDashboardSnapshot {
	unavailable := make([]UsageUnavailableAccountView, 0, len(snapshot.UnavailableAccounts))
	for _, account := range snapshot.UnavailableAccounts {
		label := account.Label
		if label == "" {
			label = account.Provider
		}
		unavailable = append(unavailable, UsageUnavailableAccountView{Provider: account.Provider, Label: label})
	}
	return UsageDashboardSnapshot{
		Reports:             snapshot.Reports,
		UnavailableAccounts: unavailable,
	}
}

// UsageDashboardOptions supplies the dashboard's data sources and callbacks.
type UsageDashboardOptions struct {
	Reports             []ai.UsageReport
	UnavailableAccounts []UsageUnavailableAccountView
	// LoadActivity streams daily activity into the heatmap; ctx is cancelled
	// when the dashboard closes.
	LoadActivity func(ctx context.Context, push func([]DailyActivityPoint)) error
	// Refresh re-fetches usage; ctx is cancelled when the dashboard closes.
	Refresh func(ctx context.Context) (UsageDashboardSnapshot, error)
	// RefreshProgress publishes usable results before slower requests finish.
	// force is false on opening and true for the user's refresh action.
	RefreshProgress func(ctx context.Context, force bool, push func(UsageDashboardSnapshot)) (UsageDashboardSnapshot, error)
	// InitialRefresh starts the first refresh when the host mounts the board so
	// an empty board reads as loading.
	InitialRefresh bool
	// ViewportRows reports the visible content rows the terminal offers.
	// Nil means 24.
	ViewportRows func() int
	// ResolveActiveAccount reports the read-only identity of the account this
	// session uses, per provider.
	ResolveActiveAccount func(provider string) *ai.UsageActiveAccount
	// RequestRender asks the host to repaint. Nil is a no-op.
	RequestRender func()
	// OnClose runs once when the dashboard closes.
	OnClose func()
}

// HeatmapLayout is a GitHub-style week-per-column heatmap grid derived from
// daily activity.
type HeatmapLayout struct {
	// MonthLabels holds the short month name when the column starts a new month.
	MonthLabels []string
	// Cells is 7 rows (Mon..Sun) × N week columns; 0..4 intensity, -1 = future day.
	Cells [][]int
	// TotalCost and TotalRequests cover the in-range days.
	TotalCost     float64
	TotalRequests int
	// Start is local midnight of the first cell (column 0, Monday).
	Start time.Time
}

var usageMonthNames = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

var usageHeatmapDayLabels = [...]string{"M", "T", "W", "T", "F", "S", "S"}

// usageLocalIso formats a time as YYYY-MM-DD in local time.
func usageLocalIso(date time.Time) string { return date.Format("2006-01-02") }

// usageAddDays adds whole calendar days at local midnight.
func usageAddDays(date time.Time, days int) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day()+days, 0, 0, 0, 0, date.Location())
}

// usageWeekdayIndex maps Sunday=0 to Monday-first row indices.
func usageWeekdayIndex(date time.Time) int {
	return (int(date.Weekday()) + 6) % 7
}

// BuildHeatmapLayout lays out daily activity into a Monday-first week grid
// ending at `today`'s week. Intensity is magnitude-scaled against the busiest
// day (square-root compressed so mid-size days stay distinguishable from
// outliers), over per-day cost, falling back to request counts when nothing in
// range is priced.
func BuildHeatmapLayout(points []DailyActivityPoint, weeks int, today time.Time) HeatmapLayout {
	if weeks < 1 {
		weeks = 1
	}
	byDay := make(map[string]DailyActivityPoint, len(points))
	anyCost := false
	for _, point := range points {
		byDay[point.Day] = point
		if point.Cost > 0 {
			anyCost = true
		}
	}
	metric := func(point DailyActivityPoint) float64 {
		if anyCost {
			return point.Cost
		}
		return float64(point.Requests)
	}

	today0 := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	start := usageAddDays(today0, -usageWeekdayIndex(today0)-(weeks-1)*7)
	startIso := usageLocalIso(start)
	todayIso := usageLocalIso(today0)

	maxValue := 0.0
	totalCost := 0.0
	totalRequests := 0
	for _, point := range points {
		if point.Day < startIso || point.Day > todayIso {
			continue
		}
		totalCost += point.Cost
		totalRequests += point.Requests
		if value := metric(point); value > maxValue {
			maxValue = value
		}
	}
	level := func(value float64) int {
		if value <= 0 || maxValue <= 0 {
			return 0
		}
		scaled := int(math.Ceil(math.Sqrt(value/maxValue) * 4))
		return min(4, max(1, scaled))
	}

	monthLabels := make([]string, weeks)
	cells := make([][]int, 7)
	for day := range cells {
		cells[day] = make([]int, weeks)
		for week := range cells[day] {
			cells[day][week] = -1
		}
	}
	previousMonth := -1
	for week := 0; week < weeks; week++ {
		weekStart := usageAddDays(start, week*7)
		if month := int(weekStart.Month()) - 1; month != previousMonth {
			monthLabels[week] = usageMonthNames[month]
			previousMonth = month
		}
		for day := 0; day < 7; day++ {
			date := usageAddDays(weekStart, day)
			if date.After(today0) {
				continue
			}
			value := 0.0
			if point, ok := byDay[usageLocalIso(date)]; ok {
				value = metric(point)
			}
			cells[day][week] = level(value)
		}
	}

	return HeatmapLayout{
		MonthLabels:   monthLabels,
		Cells:         cells,
		TotalCost:     totalCost,
		TotalRequests: totalRequests,
		Start:         start,
	}
}

// formatActivityTotals renders `$1,234 · 5.6K requests`.
func formatActivityTotals(layout HeatmapLayout) string {
	var cost string
	if layout.TotalCost >= 1 {
		cost = fmt.Sprintf("$%s", roundThousands(float64(layout.TotalCost), 0))
	} else {
		cost = fmt.Sprintf("$%.2f", layout.TotalCost)
	}
	return cost + " · " + FormatCompactCount(float64(layout.TotalRequests)) + " requests"
}

// roundThousands formats value with grouping separators and the given fraction
// digits, as Intl.NumberFormat does for en-US.
func roundThousands(value float64, fractionDigits int) string {
	factor := math.Pow(10, float64(fractionDigits))
	rounded := math.Round(value*factor) / factor
	sign := ""
	if rounded < 0 {
		sign = "-"
		rounded = -rounded
	}
	text := strconv.FormatFloat(rounded, 'f', fractionDigits, 64)
	whole, fraction, found := strings.Cut(text, ".")
	var grouped strings.Builder
	for index, char := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(char)
	}
	out := sign + grouped.String()
	if found {
		out += "." + fraction
	}
	return out
}

// FormatActivityErrorDetail sanitizes activity loading error text for safe
// single-line display: strip ANSI/control sequences, collapse whitespace,
// shorten home paths to `~`, and drop trailing dots.
//
// Home shortening is case-insensitive on both the native and forward-slash
// spellings, as upstream's `new RegExp(`${escaped}|${forward}`, "gi")` is. The
// regexp compiles only on this error path, never on a normal render.
func FormatActivityErrorDetail(message, homeDir string) string {
	text := SanitizeDisplayLine(message)
	if homeDir != "" {
		text = shortenUsageHomePath(text, homeDir)
	}
	return strings.TrimRight(text, ".")
}

// shortenUsageHomePath replaces the home directory, in native and
// forward-slash spellings and in any case, with `~`.
func shortenUsageHomePath(text, homeDir string) string {
	variants := []string{homeDir}
	if forward := strings.ReplaceAll(homeDir, "\\", "/"); forward != homeDir {
		variants = append(variants, forward)
	}
	pattern := make([]string, 0, len(variants))
	for _, variant := range variants {
		if variant == "" {
			continue
		}
		pattern = append(pattern, regexp.QuoteMeta(variant))
	}
	if len(pattern) == 0 {
		return text
	}
	matcher, err := regexp.Compile("(?i)" + strings.Join(pattern, "|"))
	if err != nil {
		return text
	}
	return matcher.ReplaceAllLiteralString(text, "~")
}

// clipRow clips a single row to a width, ending with an ellipsis when it must
// shrink.
func clipRow(text string, width int) string {
	if widthx.VisibleWidth(text) > width {
		return widthx.TruncateToWidth(text, width, "…", false)
	}
	return text
}

const (
	usageCardMinWidth    = 32
	usageCardGutter      = 3
	usageCardMinBarWidth = 12
	usageCardMaxLabelRow = 2
)

type usageCardRowLayout struct {
	labelWidth   int
	resetWidth   int
	barWidth     int
	stacked      bool
	labelHeights []int
}

// usageDashboardView is which board the dashboard shows.
type usageDashboardView string

const (
	usageViewOverview usageDashboardView = "overview"
	usageViewDetail   usageDashboardView = "detail"
)

// UsageDashboardComponent is the fullscreen /usage board.
type UsageDashboardComponent struct {
	mu sync.Mutex

	options             UsageDashboardOptions
	reports             []ai.UsageReport
	unavailableAccounts []UsageUnavailableAccountView
	cards               []ProviderCard
	nowMs               int64
	refreshing          bool
	refreshError        string
	// awaitingInitial is true until the first automatic fetch settles, so an
	// empty board reads as loading.
	awaitingInitial bool
	view            usageDashboardView
	scroll          int
	activity        []DailyActivityPoint
	hasActivity     bool
	activityError   string
	syncing         bool
	// revision prevents a render in flight from reinstalling a cache after
	// a streamed quota/activity update invalidated it.
	revision      uint64
	detailCache   *usageLinesCache
	overviewCache *usageLinesCache
	lastViewport  int
	closed        bool
	closeOnce     sync.Once
	// runCtx is the lifetime of the mounted dashboard and runCancel ends it;
	// both are set by Start and cancelled on Dispose. runCtx != nil also means
	// Start already ran.
	runCtx    context.Context
	runCancel context.CancelFunc
	// refreshCancel ends the one in-flight refresh. Refresh is single-flight on
	// refreshing, so one field is enough; each refresh clears it under mu before
	// it clears refreshing, and Dispose cancels whatever is still active
	// (including a refresh that ran before Start).
	refreshCancel context.CancelFunc
	// background counts in-flight loader goroutines that must finish before a
	// closed dashboard stops publishing.
	background sync.WaitGroup
}

// goBackground runs fn on a tracked goroutine so Dispose can wait for loaders.
func (d *UsageDashboardComponent) goBackground(fn func()) {
	d.background.Add(1)
	go func() {
		defer d.background.Done()
		fn()
	}()
}

// usageLinesCache is rendered lines for one content width.
type usageLinesCache struct {
	width int
	lines []string
}

// NewUsageDashboard creates the dashboard. Nothing is fetched until Start is
// called, so the host can mount an empty board first.
func NewUsageDashboard(options UsageDashboardOptions) *UsageDashboardComponent {
	reports := append([]ai.UsageReport(nil), options.Reports...)
	unavailable := append([]UsageUnavailableAccountView(nil), options.UnavailableAccounts...)
	nowMs := time.Now().UnixMilli()
	dashboard := &UsageDashboardComponent{
		options:             options,
		reports:             reports,
		unavailableAccounts: unavailable,
		nowMs:               nowMs,
		cards:               BuildProviderCards(reports, nowMs, unavailable),
		awaitingInitial:     (options.Refresh != nil || options.RefreshProgress != nil) && options.InitialRefresh,
		lastViewport:        10,
		syncing:             true,
	}
	return dashboard
}

// Start launches the activity scan and, with InitialRefresh, the first quota
// fetch. Call it on the owner loop after mounting.
func (d *UsageDashboardComponent) Start(ctx context.Context) {
	d.mu.Lock()
	if d.closed || d.runCtx != nil {
		d.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.runCtx, d.runCancel = runCtx, cancel
	loadActivity := d.options.LoadActivity
	refresh := d.options.InitialRefresh && (d.options.Refresh != nil || d.options.RefreshProgress != nil)
	d.mu.Unlock()

	if loadActivity != nil {
		d.goBackground(func() { d.loadActivity(runCtx, loadActivity) })
	} else {
		d.mu.Lock()
		d.syncing = false
		d.mu.Unlock()
	}
	if refresh {
		d.refresh(false)
	}
}

// loadActivity streams activity points until the scan finishes or the
// dashboard closes.
func (d *UsageDashboardComponent) loadActivity(ctx context.Context, load func(context.Context, func([]DailyActivityPoint)) error) {
	push := func(points []DailyActivityPoint) {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return
		}
		d.activity = points
		d.hasActivity = true
		d.mu.Unlock()
		d.changed()
	}
	err := load(ctx, push)
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	if err != nil && ctx.Err() == nil {
		d.activityError = err.Error()
	}
	d.syncing = false
	d.mu.Unlock()
	d.changed()
}

// Refresh re-fetches the reports; the last reports stay on failure, with the
// error shown in the header.
func (d *UsageDashboardComponent) Refresh() { d.refresh(true) }

func (d *UsageDashboardComponent) refresh(force bool) {
	d.mu.Lock()
	refresh, progress := d.options.Refresh, d.options.RefreshProgress
	if (refresh == nil && progress == nil) || d.refreshing || d.closed {
		d.mu.Unlock()
		return
	}
	d.refreshing = true
	d.refreshError = ""
	parent := d.runCtx
	if parent == nil {
		// A host that refreshes before Start still gets a cancellable lifetime:
		// refreshCancel hands it to Dispose, and Start keeps its own.
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	d.refreshCancel = cancel
	d.mu.Unlock()
	d.changed()

	d.goBackground(func() {
		// Release this refresh's context as soon as its work settles; the field
		// below is only the handle Dispose cancels while it runs.
		defer cancel()
		push := func(snapshot UsageDashboardSnapshot) {
			d.mu.Lock()
			if d.closed || ctx.Err() != nil {
				d.mu.Unlock()
				return
			}
			d.applySnapshotLocked(snapshot)
			d.awaitingInitial = false
			d.mu.Unlock()
			d.changed()
		}
		var snapshot UsageDashboardSnapshot
		var err error
		if progress != nil {
			snapshot, err = progress(ctx, force, push)
		} else {
			snapshot, err = refresh(ctx)
		}

		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				d.refreshError = err.Error()
			}
		} else {
			d.applySnapshotLocked(snapshot)
		}
		// Clear the handle before refreshing, under this lock, so the next
		// refresh cannot inherit a finished one's cancel and Dispose never
		// cancels a settled context.
		d.refreshCancel = nil
		d.refreshing = false
		d.awaitingInitial = false
		d.mu.Unlock()
		d.changed()
	})
}

// applySnapshotLocked publishes one immutable refresh snapshot under d.mu.
func (d *UsageDashboardComponent) applySnapshotLocked(snapshot UsageDashboardSnapshot) {
	d.reports = snapshot.Reports
	d.unavailableAccounts = snapshot.UnavailableAccounts
	d.nowMs = time.Now().UnixMilli()
	d.cards = BuildProviderCards(d.reports, d.nowMs, d.unavailableAccounts)
}

// Invalidate drops the rendered caches.
func (d *UsageDashboardComponent) Invalidate() {
	d.mu.Lock()
	d.revision++
	d.detailCache = nil
	d.overviewCache = nil
	d.mu.Unlock()
}

// Dispose stops loading and rendering for good.
func (d *UsageDashboardComponent) Dispose() {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		runCancel, refreshCancel := d.runCancel, d.refreshCancel
		d.runCancel, d.refreshCancel = nil, nil
		d.mu.Unlock()
		if refreshCancel != nil {
			refreshCancel()
		}
		if runCancel != nil {
			runCancel()
		}
	})
}

// closedForRender reports whether the dashboard may still draw.
func (d *UsageDashboardComponent) closedForRender() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// changed invalidates caches and repaints; never touches state or requests
// renders after close.
func (d *UsageDashboardComponent) changed() {
	d.mu.Lock()
	closed := d.closed
	d.revision++
	d.detailCache = nil
	d.overviewCache = nil
	d.mu.Unlock()
	if closed {
		return
	}
	if d.options.RequestRender != nil {
		d.options.RequestRender()
	}
}

// ─── Subscriptions grid ──────────────────────────────────────────────────────

func (d *UsageDashboardComponent) statusGlyph(status ai.UsageStatus) string {
	theme := ActiveTheme()
	switch status {
	case ai.UsageStatusExhausted:
		return theme.Fg(string(ThemeColorError), UsageGlyphError)
	case ai.UsageStatusWarning:
		return theme.Fg(string(ThemeColorWarning), UsageGlyphWarning)
	case ai.UsageStatusOK:
		return theme.Fg(string(ThemeColorSuccess), UsageGlyphSuccess)
	}
	return theme.Fg(string(ThemeColorDim), UsageGlyphInfo)
}

func miniBar(fraction *float64, status ai.UsageStatus, width int) string {
	theme := ActiveTheme()
	if fraction == nil {
		return theme.Fg(string(ThemeColorDim), strings.Repeat("·", width))
	}
	clamped := math.Min(math.Max(*fraction, 0), 1)
	filled := int(math.Round(clamped * float64(width)))
	return theme.Fg(StatusColorToken(status), strings.Repeat("█", filled)) +
		theme.Fg(string(ThemeColorDim), strings.Repeat("░", width-filled))
}

// renderCardLines draws one provider card at the given width.
func (d *UsageDashboardComponent) renderCardLines(card ProviderCard, width int, labels [][]string, layout usageCardRowLayout) []string {
	theme := ActiveTheme()
	lines := make([]string, 0, 12)
	var cardStatus ai.UsageStatus
	switch {
	case len(card.UnavailableAccounts) > 0:
		windowStatuses := make([]ai.UsageStatus, 0, len(card.Windows))
		for _, window := range card.Windows {
			windowStatuses = append(windowStatuses, window.Status)
		}
		cardStatus = StatusWithUnavailableAccounts(windowStatuses)
	case card.Unlimited:
		cardStatus = ai.UsageStatusOK
	default:
		windowStatuses := make([]ai.UsageStatus, 0, len(card.Windows))
		for _, window := range card.Windows {
			windowStatuses = append(windowStatuses, window.Status)
		}
		cardStatus = AggregateStatus(windowStatuses)
	}
	accountsText := ""
	if card.Accounts > 1 {
		accountsText = theme.Fg(string(ThemeColorDim), strconv.Itoa(card.Accounts)+" accts")
	}
	titleBudget := width - 2 - widthx.VisibleWidth(accountsText)
	if accountsText != "" {
		titleBudget--
	}
	title := theme.Bold(clipRow(card.Name, max(4, titleBudget)))
	titlePad := max(0, width-2-widthx.VisibleWidth(title)-widthx.VisibleWidth(accountsText))
	lines = append(lines, d.statusGlyph(cardStatus)+" "+title+strings.Repeat(" ", titlePad)+accountsText)

	for _, account := range card.DaybreakAccounts {
		label := SanitizeDisplayLine(account)
		lines = append(lines, "  "+theme.Fg(string(ThemeColorSuccess), clipRow("daybreak · "+label, width-2)))
	}

	if card.ResetCredits != nil {
		resets := card.ResetCredits
		plural := "s"
		if resets.BankedCount == 1 {
			plural = ""
		}
		resetText := "✦ " + strconv.Itoa(resets.BankedCount) + " reset" + plural
		if resets.RedeemableCount != resets.BankedCount {
			resetText += " · " + strconv.Itoa(resets.RedeemableCount) + " usable"
		}
		if resets.SoonestExpiryMs != nil {
			if *resets.SoonestExpiryMs > 0 {
				resetText += " · expires " + FormatDuration(*resets.SoonestExpiryMs)
			} else {
				resetText += " · expired"
			}
		}
		color := string(ThemeColorWarning)
		if resets.RedeemableCount > 0 {
			color = string(ThemeColorSuccess)
		}
		lines = append(lines, "  "+theme.Fg(color, clipRow(resetText, width-2)))
		if resets.RedeemableCount == 0 && len(resets.UnavailableReasons) > 0 {
			reason := SanitizeDisplayLine(strings.Join(resets.UnavailableReasons, " • "))
			lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), clipRow("unavailable: "+reason, width-2)))
		}
	}

	for _, account := range card.UnavailableAccounts {
		text := SanitizeDisplayLine(account + " — usage unavailable")
		for _, line := range widthx.WrapTextWithAnsi(text, max(1, width-2)) {
			lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), line))
		}
	}

	if card.Unlimited {
		lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), "no limits"))
		return lines
	}

	hidden := len(card.Windows) - CardMaxWindows
	contentWidth := max(1, width-2)
	for index := 0; index < min(len(card.Windows), CardMaxWindows); index++ {
		window := card.Windows[index]
		labelLines := labels[index]
		label := ""
		if len(labelLines) > 0 {
			label = labelLines[0]
		}
		prefix := ""
		if !layout.stacked {
			prefix = label + strings.Repeat(" ", max(0, layout.labelWidth-widthx.VisibleWidth(label))) + " "
		}
		if layout.stacked {
			for line := 0; line < layout.labelHeights[index]; line++ {
				if line < len(labelLines) {
					lines = append(lines, "  "+labelLines[line])
				}
			}
		}
		if window.Fraction == nil {
			text := window.UsedText
			if text == "" {
				text = "no data"
			}
			text = theme.Fg(string(ThemeColorDim), text)
			for _, line := range widthx.WrapTextWithAnsi(prefix+text, contentWidth) {
				lines = append(lines, "  "+line)
			}
			continue
		}
		freePct := max(0, int(math.Round((1-*window.Fraction)*100)))
		pctText := theme.Fg(StatusColorToken(window.Status), padLeft(strconv.Itoa(freePct)+"%", 5))
		resetPlain := ""
		if window.ResetMs != nil {
			resetPlain = FormatDuration(*window.ResetMs)
		}
		resetText := ""
		if layout.resetWidth > 0 {
			resetText = " " + theme.Fg(string(ThemeColorDim), padLeft(resetPlain, layout.resetWidth))
		}
		for _, line := range widthx.WrapTextWithAnsi(
			prefix+miniBar(window.Fraction, window.Status, layout.barWidth)+pctText+resetText, contentWidth) {
			lines = append(lines, "  "+line)
		}
	}
	if hidden > 0 {
		lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), "+"+strconv.Itoa(hidden)+" more"))
	}
	return lines
}

// padLeft pads text with leading spaces to width.
func padLeft(text string, width int) string {
	visible := widthx.VisibleWidth(text)
	if visible >= width {
		return text
	}
	return strings.Repeat(" ", width-visible) + text
}

// renderCardsGrid lays the provider cards out in a responsive grid, then
// collapses untouched providers into one tick line.
func (d *UsageDashboardComponent) renderCardsGrid(innerWidth int) []string {
	theme := ActiveTheme()
	d.mu.Lock()
	cards := append([]ProviderCard(nil), d.cards...)
	refreshing, awaitingInitial := d.refreshing, d.awaitingInitial
	refreshError := d.refreshError
	d.mu.Unlock()

	if len(cards) == 0 {
		if refreshing || awaitingInitial {
			return []string{theme.Fg(string(ThemeColorDim), "Loading usage quotas…")}
		}
		if refreshError != "" {
			return []string{theme.Fg(string(ThemeColorWarning),
				clipRow("Usage unavailable: "+SanitizeDisplayLine(refreshError), innerWidth))}
		}
		return []string{theme.Fg(string(ThemeColorDim), "No usage data available.")}
	}

	active := make([]ProviderCard, 0, len(cards))
	idle := make([]ProviderCard, 0, len(cards))
	for _, card := range cards {
		if card.Idle {
			idle = append(idle, card)
		} else {
			active = append(active, card)
		}
	}
	columns := max(1, (innerWidth+usageCardGutter)/(usageCardMinWidth+usageCardGutter))
	cardWidth := (innerWidth - (columns-1)*usageCardGutter) / columns
	lines := make([]string, 0, 32)
	for start := 0; start < len(active); start += columns {
		rowCards := active[start:min(start+columns, len(active))]
		windows := make([][]CardWindowRow, 0, len(rowCards))
		labels := make([][]string, 0, len(rowCards))
		for _, card := range rowCards {
			rows := card.Windows[:min(len(card.Windows), CardMaxWindows)]
			windows = append(windows, rows)
			cardLabels := make([]string, 0, len(rows))
			for _, window := range rows {
				label := theme.Fg(string(ThemeColorMuted), SanitizeDisplayLine(window.Label))
				if window.WindowTag != "" {
					label += " " + theme.Fg(string(ThemeColorDim), SanitizeDisplayLine(window.WindowTag))
				}
				cardLabels = append(cardLabels, label)
			}
			labels = append(labels, cardLabels)
		}
		// One geometry per grid row: labels, bars, and resets share columns,
		// and every card stacks together when inline bars would be too short.
		labelWidth := 0
		resetWidth := 0
		for _, rows := range labels {
			for _, label := range rows {
				if width := widthx.VisibleWidth(label); width > labelWidth {
					labelWidth = width
				}
			}
		}
		for _, rows := range windows {
			for _, window := range rows {
				if window.ResetMs != nil {
					if width := len(FormatDuration(*window.ResetMs)); width > resetWidth {
						resetWidth = width
					}
				}
			}
		}
		contentWidth := max(1, cardWidth-2)
		suffixWidth := 5
		if resetWidth > 0 {
			suffixWidth += resetWidth + 1
		}
		inlineBarWidth := contentWidth - labelWidth - 1 - suffixWidth
		stacked := inlineBarWidth < usageCardMinBarWidth
		labelLines := make([][][]string, 0, len(rowCards))
		for _, rows := range labels {
			cardLines := make([][]string, 0, len(rows))
			for _, label := range rows {
				if !stacked {
					cardLines = append(cardLines, []string{label})
					continue
				}
				// Bound wrapping work as well as height, keeping the suffix that
				// distinguishes model-specific quota buckets.
				bounded := TruncateMiddleToWidth(label, contentWidth*usageCardMaxLabelRow)
				wrapped := widthx.WrapTextWithAnsi(bounded, contentWidth)
				if len(wrapped) <= usageCardMaxLabelRow {
					cardLines = append(cardLines, wrapped)
					continue
				}
				// Word wrapping can waste enough cells to create a third line even
				// when the label fits; split by columns instead.
				first := widthx.SliceByColumn(bounded, 0, contentWidth, true)
				rest := widthx.SliceByColumn(bounded, widthx.VisibleWidth(first),
					widthx.VisibleWidth(bounded)-widthx.VisibleWidth(first), true)
				cardLines = append(cardLines, []string{first, clipRow(rest, contentWidth)})
			}
			labelLines = append(labelLines, cardLines)
		}
		labelHeights := make([]int, CardMaxWindows)
		for index := range labelHeights {
			height := 0
			for _, cardLines := range labelLines {
				if index < len(cardLines) && len(cardLines[index]) > height {
					height = len(cardLines[index])
				}
			}
			labelHeights[index] = height
		}
		layout := usageCardRowLayout{
			labelWidth:   labelWidth,
			resetWidth:   resetWidth,
			barWidth:     max(1, inlineBarWidth),
			stacked:      stacked,
			labelHeights: labelHeights,
		}
		if stacked {
			layout.barWidth = max(1, contentWidth-suffixWidth)
		}
		rendered := make([][]string, 0, len(rowCards))
		for index, card := range rowCards {
			rendered = append(rendered, d.renderCardLines(card, cardWidth, labelLines[index], layout))
		}
		height := 0
		for _, cardLines := range rendered {
			if len(cardLines) > height {
				height = len(cardLines)
			}
		}
		for lineIndex := 0; lineIndex < height; lineIndex++ {
			segments := make([]string, 0, len(rendered))
			for _, cardLines := range rendered {
				line := ""
				if lineIndex < len(cardLines) {
					line = cardLines[lineIndex]
				}
				segments = append(segments, line+strings.Repeat(" ", max(0, cardWidth-widthx.VisibleWidth(line))))
			}
			lines = append(lines, strings.TrimRight(strings.Join(segments, strings.Repeat(" ", usageCardGutter)), " "))
		}
		if start+columns < len(active) {
			lines = append(lines, "")
		}
	}
	// Untouched providers collapse into a single tick line: per-window bars at
	// 100% free (or no limits at all) are noise.
	if len(idle) > 0 {
		if len(active) > 0 {
			lines = append(lines, "")
		}
		names := make([]string, 0, len(idle))
		for _, card := range idle {
			names = append(names, card.Name)
		}
		lines = append(lines, clipRow(
			theme.Fg(string(ThemeColorSuccess), UsageGlyphSuccess)+" "+
				theme.Fg(string(ThemeColorDim), "untouched: "+strings.Join(names, " · ")), innerWidth))
	}
	return lines
}

// ─── Heatmap ─────────────────────────────────────────────────────────────────

// heatRamp is the truecolor ramp from the background side toward the accent so
// cell brightness reads as amount of work. It is anchored by the text color's
// luma so the ramp keeps its direction on light themes.
func heatRamp() []string {
	theme := ActiveTheme()
	textColor, ok := theme.Colors()[string(ThemeColorText)]
	if !ok {
		textColor = RgbColorValue{R: 255, G: 255, B: 255}
	}
	textRgb := ColorToRgb(textColor)
	darkBackground := (0.2126*textRgb.R+0.7152*textRgb.G+0.0722*textRgb.B)/255 > 0.5
	from := Color(RgbColorValue{R: 20, G: 20, B: 24})
	if !darkBackground {
		from = RgbColorValue{R: 244, G: 244, B: 246}
	}
	to := theme.Colors()[string(ThemeColorAccent)]
	if to == nil {
		to = RgbColorValue{R: 120, G: 160, B: 255}
	}
	ramp := make([]string, 0, 4)
	for _, stop := range []float64{0.3, 0.5, 0.72, 1} {
		mixed, err := MixColors(from, to, stop, ColorMixSpaceSrgb)
		if err != nil {
			mixed = to
		}
		ramp = append(ramp, ForegroundAnsi(mixed, theme.GetColorMode()))
	}
	return ramp
}

// renderHeatmap draws the activity calendar.
func (d *UsageDashboardComponent) renderHeatmap(innerWidth int) []string {
	theme := ActiveTheme()
	d.mu.Lock()
	activityError := d.activityError
	hasActivity := d.hasActivity
	points := d.activity
	syncing := d.syncing
	d.mu.Unlock()

	if activityError != "" {
		detail := FormatActivityErrorDetail(activityError, homeDirectory())
		message := "Usage history unavailable."
		if detail != "" {
			message = "Usage history unavailable (" + detail + ")."
		}
		return []string{clipRow(theme.Fg(string(ThemeColorDim), message), innerWidth)}
	}
	if !hasActivity {
		return []string{theme.Fg(string(ThemeColorDim), "Loading usage history…")}
	}

	labelWidth := 2
	weeks := max(4, min(53, (innerWidth-labelWidth)/2))
	layout := BuildHeatmapLayout(points, weeks, time.Now())
	ramp := heatRamp()
	reset := "\x1b[39m"
	summary := make([]string, 0, 12)

	syncSuffix := ""
	if syncing {
		syncSuffix = theme.Fg(string(ThemeColorDim), " · syncing…")
	}
	summary = append(summary, clipRow(
		theme.Bold(theme.Fg(string(ThemeColorAccent), "Activity"))+" "+
			theme.Fg(string(ThemeColorDim), formatActivityTotals(layout)+" · last "+strconv.Itoa(weeks)+" weeks")+
			syncSuffix, innerWidth))
	summary = append(summary, "")

	monthLine := strings.Repeat(" ", labelWidth)
	for week := 0; week < weeks; week++ {
		label := layout.MonthLabels[week]
		targetCol := labelWidth + week*2
		if label != "" && targetCol >= widthx.VisibleWidth(monthLine) {
			padding := targetCol - widthx.VisibleWidth(monthLine)
			monthLine += strings.Repeat(" ", padding) + label
		}
	}
	summary = append(summary, theme.Fg(string(ThemeColorDim), clipRow(monthLine, innerWidth)))

	for day := 0; day < 7; day++ {
		var line strings.Builder
		line.WriteString(theme.Fg(string(ThemeColorDim), usageHeatmapDayLabels[day]))
		line.WriteString(" ")
		for week := 0; week < weeks; week++ {
			cell := layout.Cells[day][week]
			switch {
			case cell < 0:
				line.WriteString("  ")
			case cell == 0:
				line.WriteString(theme.Fg(string(ThemeColorDim), "·") + " ")
			default:
				line.WriteString(ramp[cell-1] + "■" + reset + " ")
			}
		}
		summary = append(summary, clipRow(strings.TrimRight(line.String(), " "), innerWidth))
	}
	return summary
}

// homeDirectory returns the user's home directory, or "" when unknown.
func homeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ─── Views ───────────────────────────────────────────────────────────────────

// overviewLines renders the cards grid then the heatmap, cached per width.
func (d *UsageDashboardComponent) overviewLines(innerWidth int) []string {
	d.mu.Lock()
	revision := d.revision
	if cached := d.overviewCache; cached != nil && cached.width == innerWidth {
		lines := cached.lines
		d.mu.Unlock()
		return lines
	}
	d.mu.Unlock()

	lines := make([]string, 0, 48)
	lines = append(lines, d.renderCardsGrid(innerWidth)...)
	lines = append(lines, "")
	lines = append(lines, d.renderHeatmap(innerWidth)...)

	d.mu.Lock()
	if !d.closed && revision == d.revision {
		d.overviewCache = &usageLinesCache{width: innerWidth, lines: lines}
	}
	d.mu.Unlock()
	return lines
}

// detailLines renders the classic per-account report, cached per width.
func (d *UsageDashboardComponent) detailLines(innerWidth int) []string {
	d.mu.Lock()
	revision := d.revision
	reports := append([]ai.UsageReport(nil), d.reports...)
	unavailable := append([]UsageUnavailableAccountView(nil), d.unavailableAccounts...)
	nowMs := d.nowMs
	awaiting, refreshing := d.awaitingInitial, d.refreshing
	refreshError := d.refreshError
	resolveActive := d.options.ResolveActiveAccount
	if cached := d.detailCache; cached != nil && cached.width == innerWidth {
		lines := cached.lines
		d.mu.Unlock()
		return lines
	}
	d.mu.Unlock()

	// While the first fetch is in flight an empty report set means "loading",
	// not "this provider reports no quotas".
	if len(reports) == 0 && len(unavailable) == 0 && (awaiting || refreshing) {
		return []string{ActiveTheme().Fg(string(ThemeColorDim), "Loading usage quotas…")}
	}
	if len(reports) == 0 && len(unavailable) == 0 && refreshError != "" {
		return []string{ActiveTheme().Fg(string(ThemeColorWarning), "Usage unavailable.")}
	}
	rendered := RenderUsageReports(UsageReportOptions{
		Reports:              reports,
		NowMs:                nowMs,
		Width:                innerWidth,
		UnavailableAccounts:  unavailable,
		ResolveActiveAccount: resolveActive,
	})
	lines := strings.Split(rendered, "\n")
	d.mu.Lock()
	if !d.closed && revision == d.revision {
		d.detailCache = &usageLinesCache{width: innerWidth, lines: lines}
	}
	d.mu.Unlock()
	return lines
}

// ─── Frame ───────────────────────────────────────────────────────────────────

func (d *UsageDashboardComponent) frameRow(content string, innerWidth int) string {
	bar := ActiveTheme().Fg(string(ThemeColorBorder), "│")
	return bar + " " + padToWidth(clipRow(content, innerWidth), innerWidth) + " " + bar
}

func (d *UsageDashboardComponent) topBorder(innerWidth int, title string) string {
	theme := ActiveTheme()
	inner := max(0, innerWidth+2)
	shown := clipRow(" "+title+" ", max(0, inner-2))
	fill := max(0, inner-1-widthx.VisibleWidth(shown))
	return theme.Fg(string(ThemeColorBorder), "╭─") +
		theme.Bold(theme.Fg(string(ThemeColorAccent), shown)) +
		theme.Fg(string(ThemeColorBorder), strings.Repeat("─", fill)+"╮")
}

func (d *UsageDashboardComponent) divider(innerWidth int) string {
	return ActiveTheme().Fg(string(ThemeColorBorder), "├"+strings.Repeat("─", max(0, innerWidth+2))+"┤")
}

func (d *UsageDashboardComponent) bottomBorder(innerWidth int) string {
	return ActiveTheme().Fg(string(ThemeColorBorder), "╰"+strings.Repeat("─", max(0, innerWidth+2))+"╯")
}

// usageActionText formats an action's keys, falling back to the upstream
// default text when the installed table does not define the action.
func usageActionText(action, fallback string) string {
	if keys := GetKeybindings().GetKeys(action); len(keys) > 0 {
		return FormatKeyText(strings.Join(keys, "/"), false)
	}
	return FormatKeyText(fallback, false)
}

// hint renders the footer key hints from the configured bindings.
func (d *UsageDashboardComponent) hint() string {
	theme := ActiveTheme()
	up := usageActionText(KBSelectUp, "up")
	down := usageActionText(KBSelectDown, "down")
	cancel := usageActionText(KBSelectCancel, "escape")
	parts := make([]string, 0, 4)
	if up != "" || down != "" {
		keys := make([]string, 0, 2)
		for _, key := range []string{up, down} {
			if key != "" {
				keys = append(keys, key)
			}
		}
		parts = append(parts, strings.Join(keys, "/")+" scroll")
	}
	d.mu.Lock()
	hasRefresh, view := d.options.Refresh != nil || d.options.RefreshProgress != nil, d.view
	d.mu.Unlock()
	if hasRefresh {
		if refresh := usageActionText("app.usage.refresh", "r"); refresh != "" {
			parts = append(parts, refresh+" refresh")
		}
	}
	if view == usageViewDetail {
		if cancel != "" {
			parts = append(parts, cancel+" back")
		}
	} else {
		if toggle := usageActionText("app.usage.toggleView", "tab"); toggle != "" {
			parts = append(parts, toggle+" details")
		}
		if cancel != "" {
			parts = append(parts, cancel+" close")
		}
	}
	return theme.Fg(string(ThemeColorDim), strings.Join(parts, " · "))
}

// Render draws the dashboard frame at the given terminal width.
func (d *UsageDashboardComponent) Render(width int) []string {
	if d.closedForRender() {
		return nil
	}
	theme := ActiveTheme()
	totalWidth := max(1, width)
	// Below the width the frame needs, keep the row honest instead of drawing
	// outside the terminal.
	if totalWidth < 6 {
		return []string{clipRow(theme.Bold(theme.Fg(string(ThemeColorAccent), "Usage")), totalWidth)}
	}
	innerWidth := totalWidth - 4

	requestedRows := 24
	if d.options.ViewportRows != nil {
		requestedRows = d.options.ViewportRows()
	}
	height := max(6, requestedRows)
	// Fixed chrome: top border, header, body…, divider, hint, bottom border.
	contentRows := max(1, height-5)

	d.mu.Lock()
	d.lastViewport = contentRows
	view := d.view
	scroll := d.scroll
	nowMs := d.nowMs
	reports := d.reports
	refreshing := d.refreshing
	refreshError := d.refreshError
	d.mu.Unlock()

	var contentSource []string
	if view == usageViewDetail {
		contentSource = d.detailLines(innerWidth)
	} else {
		contentSource = d.overviewLines(innerWidth)
	}
	maxScroll := max(0, len(contentSource)-contentRows)
	if scroll > maxScroll {
		scroll = maxScroll
		d.mu.Lock()
		if d.scroll > maxScroll {
			d.scroll = maxScroll
		}
		d.mu.Unlock()
	}

	latestFetchedAt := int64(0)
	for _, report := range reports {
		if report.FetchedAt > latestFetchedAt {
			latestFetchedAt = report.FetchedAt
		}
	}
	checkedText := ""
	switch {
	case refreshing:
		checkedText = "refreshing…"
	case refreshError != "":
		checkedText = "refresh failed: " + SanitizeDisplayLine(refreshError)
	case latestFetchedAt > 0:
		checkedText = "checked " + FormatDuration(nowMs-latestFetchedAt) + " ago"
	}
	title := "Usage"
	if view == usageViewDetail {
		title = "Usage · Details"
	}
	header := theme.Fg(string(ThemeColorDim), clipRow(checkedText, innerWidth))
	if refreshError != "" && !refreshing {
		header = theme.Fg(string(ThemeColorWarning), clipRow(checkedText, innerWidth))
	}

	body := make([]string, 0, contentRows)
	for row := 0; row < contentRows; row++ {
		line := ""
		if index := scroll + row; index < len(contentSource) {
			line = contentSource[index]
		}
		body = append(body, d.frameRow(line, innerWidth))
	}

	position := ""
	if maxScroll > 0 {
		position = " · " + strconv.Itoa(min(len(contentSource), scroll+contentRows)) + "/" + strconv.Itoa(len(contentSource))
	}
	lines := make([]string, 0, contentRows+4)
	lines = append(lines, d.topBorder(innerWidth, title))
	lines = append(lines, d.frameRow(header, innerWidth))
	lines = append(lines, body...)
	lines = append(lines, d.divider(innerWidth))
	lines = append(lines, d.frameRow(d.hint()+theme.Fg(string(ThemeColorDim), position), innerWidth))
	lines = append(lines, d.bottomBorder(innerWidth))
	return lines
}

// HandleMouse scrolls on the wheel.
func (d *UsageDashboardComponent) HandleMouse(event TuiMouseEvent) *TuiMouseDispatchResult {
	if d.closedForRender() || event.Type != MouseWheel || event.WheelDelta == 0 {
		return nil
	}
	delta := 1
	if event.WheelDelta < 0 {
		delta = -1
	}
	d.scrollBy(delta)
	render := true
	// An empty Target makes this a leaf result: DispatchMouseEvent fills the
	// target and origin in, as it does for every other leaf handler.
	return &TuiMouseDispatchResult{
		TuiMouseEventResult: TuiMouseEventResult{Handled: true, Render: &render},
	}
}

// usageActionMatches reports whether data runs action. An action the installed
// keybinding table does not define falls back to the upstream defaults, so the
// board keeps working where only tui.* actions are registered (component tests,
// extension surfaces) while still honoring every user remap.
func usageActionMatches(keybindings *TUIKeybindingsManager, data, action string, defaults ...string) bool {
	if keybindings.HasBinding(action) {
		return keybindings.Matches(data, action)
	}
	for _, key := range defaults {
		if MatchesKeyID(data, key) {
			return true
		}
	}
	return false
}

// HandleInput routes one key through the configured actions: cancel closes the
// board (or returns from the detail view), the usage toggle switches boards,
// the refresh action re-fetches, and the select/alt-screen scroll actions move
// the body. No key is hardcoded beyond the upstream defaults used when an
// action is missing from the installed table.
func (d *UsageDashboardComponent) HandleInput(data string) {
	if d.closedForRender() {
		return
	}
	keybindings := GetKeybindings()
	// The cancel action closes the board, or returns from the detail view.
	if usageActionMatches(keybindings, data, KBSelectCancel, "escape", "ctrl+c") {
		d.mu.Lock()
		view := d.view
		d.mu.Unlock()
		if view == usageViewDetail {
			d.setView(usageViewOverview)
			return
		}
		d.close()
		return
	}
	switch {
	case usageActionMatches(keybindings, data, "app.usage.refresh", "r"):
		d.Refresh()
		return
	case usageActionMatches(keybindings, data, "app.usage.toggleView", "tab", "enter"):
		d.mu.Lock()
		view := d.view
		d.mu.Unlock()
		if view == usageViewDetail {
			d.setView(usageViewOverview)
		} else {
			d.setView(usageViewDetail)
		}
		return
	}
	switch {
	case usageActionMatches(keybindings, data, KBSelectUp, "up"):
		d.scrollBy(-1)
	case usageActionMatches(keybindings, data, KBSelectDown, "down"):
		d.scrollBy(1)
	case usageActionMatches(keybindings, data, KBSelectPageUp, "pageUp"), usageActionMatches(keybindings, data, KBAltScreenPageUp, "pageUp"):
		d.mu.Lock()
		rows := d.lastViewport
		d.mu.Unlock()
		d.scrollBy(-rows)
	case usageActionMatches(keybindings, data, KBSelectPageDown, "pageDown"), usageActionMatches(keybindings, data, KBAltScreenPageDown, "pageDown"):
		d.mu.Lock()
		rows := d.lastViewport
		d.mu.Unlock()
		d.scrollBy(rows)
	case usageActionMatches(keybindings, data, KBAltScreenTop, "ctrl+home"):
		d.scrollTo(0)
	case usageActionMatches(keybindings, data, KBAltScreenBottom, "ctrl+end"):
		d.scrollTo(math.MaxInt32)
	}
}

// CapturesViewportInput keeps the fullscreen transcript viewport from taking
// the board's scroll keys while the overlay has focus.
func (d *UsageDashboardComponent) CapturesViewportInput() bool { return true }

// scrollBy moves the viewport by delta rows.
func (d *UsageDashboardComponent) scrollBy(delta int) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.scroll = max(0, d.scroll+delta)
	d.mu.Unlock()
	if d.options.RequestRender != nil {
		d.options.RequestRender()
	}
}

// scrollTo moves the viewport to position.
func (d *UsageDashboardComponent) scrollTo(position int) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.scroll = max(0, position)
	d.mu.Unlock()
	if d.options.RequestRender != nil {
		d.options.RequestRender()
	}
}

// setView switches the board and resets scrolling.
func (d *UsageDashboardComponent) setView(view usageDashboardView) {
	d.mu.Lock()
	d.view = view
	d.scroll = 0
	d.mu.Unlock()
	d.changed()
}

// close disposes the dashboard and reports the close once.
func (d *UsageDashboardComponent) close() {
	if d.closedForRender() {
		return
	}
	d.Dispose()
	if d.options.OnClose != nil {
		d.options.OnClose()
	}
}

// View reports which board the dashboard shows.
func (d *UsageDashboardComponent) View() usageDashboardView {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.view
}

// waitBlocks is a test and host hook that waits for in-flight loaders.
func (d *UsageDashboardComponent) waitBlocks() { d.background.Wait() }
