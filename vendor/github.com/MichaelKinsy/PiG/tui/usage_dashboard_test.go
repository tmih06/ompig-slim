package tui

// Behavioral tests for the /usage board: plan grouping, provider cards, the
// detailed report layout, the activity scanner, and dashboard interaction.
// Mirrors packages/coding-agent/test/usage-dashboard.test.ts and
// usage-activity.test.ts at the boundaries that are uncertain in the port.

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// usageTestLimit builds one limit the way providers report it.
func usageTestLimit(provider, accountID, windowID, label string, amount ai.UsageAmount, status ai.UsageStatus) ai.UsageLimit {
	return ai.UsageLimit{
		ID:     provider + ":" + accountID + ":" + windowID,
		Label:  label,
		Scope:  ai.UsageScope{Provider: provider, AccountID: accountID, WindowID: windowID},
		Window: &ai.UsageWindow{ID: windowID, Label: windowID},
		Amount: amount,
		Status: status,
	}
}

// usageTestReport builds one provider account's report.
func usageTestReport(provider, email string, limits []ai.UsageLimit, metadata map[string]any) ai.UsageReport {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["email"]; !ok {
		metadata["email"] = email
	}
	return ai.UsageReport{Provider: provider, FetchedAt: 1000, Limits: limits, Metadata: metadata}
}

func plainLines(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, stripUsageLine(line))
	}
	return strings.Join(out, "\n")
}

func TestUsagePlanGroupingSeparatesSubscriptions(t *testing.T) {
	now := time.Now().UnixMilli()
	account := func(email, plan string, used float64) ai.UsageReport {
		limit := usageTestLimit("openai-codex", email, "5h", "5 Hour",
			ai.UsageAmount{UsedFraction: new(used), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK)
		limit.ID = "five-hour"
		return usageTestReport("openai-codex", email, []ai.UsageLimit{limit}, map[string]any{"email": email, "planType": plan})
	}
	cards := BuildProviderCards([]ai.UsageReport{
		account("plus-one@test", "chatgpt-plus", 0.2),
		account("free@test", "free", 0.9),
		account("plus-two@test", "Plus", 0.4),
		account("pro@test", "chatgpt-pro", 0.5),
		account("unknown@test", "", 0.1),
	}, now, []UsageUnavailableAccountView{{Provider: "openai-codex", Label: "unavailable@test"}})

	byName := map[string]ProviderCard{}
	for _, card := range cards {
		byName[card.Name] = card
	}
	if len(cards) != 4 {
		t.Fatalf("got %d cards, want 4: %v", len(cards), byName)
	}
	plus := byName["Openai Codex · Plus"]
	if plus.Accounts != 2 {
		t.Errorf("plus accounts = %d, want 2", plus.Accounts)
	}
	if plus.Windows[0].Fraction == nil || math.Abs(*plus.Windows[0].Fraction-0.3) > 1e-9 {
		t.Errorf("plus fraction = %v, want 0.3", plus.Windows[0].Fraction)
	}
	if len(plus.UnavailableAccounts) != 0 {
		t.Errorf("plus unavailable = %v, want none", plus.UnavailableAccounts)
	}
	free := byName["Openai Codex · Free"]
	if free.Windows[0].Fraction == nil || *free.Windows[0].Fraction != 0.9 {
		t.Errorf("free fraction = %v, want 0.9", free.Windows[0].Fraction)
	}
	unknown := byName["Openai Codex · Unknown plan"]
	if unknown.Accounts != 2 {
		t.Errorf("unknown plan accounts = %d, want 2", unknown.Accounts)
	}
	if len(unknown.UnavailableAccounts) != 1 || unknown.UnavailableAccounts[0] != "unavailable@test" {
		t.Errorf("unknown plan unavailable = %v", unknown.UnavailableAccounts)
	}
	if _, ok := byName["Openai Codex · Pro"]; !ok {
		t.Errorf("pro plan missing: %v", byName)
	}
}

func TestUsageCardFoldsTierIntoLabelAndCollapsesShared(t *testing.T) {
	now := time.Now().UnixMilli()
	shared := func(id string, used float64) ai.UsageLimit {
		limit := usageTestLimit("google-antigravity", "account", "5h", "Claude & GPT (shared)",
			ai.UsageAmount{UsedFraction: new(used), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK)
		limit.ID = id
		limit.Scope.Shared = true
		limit.Scope.SharedGroup = "3p-5h"
		return limit
	}
	report := usageTestReport("google-antigravity", "u@test", []ai.UsageLimit{shared("a", 0.2), shared("b", 0.4)}, nil)
	cards := BuildProviderCards([]ai.UsageReport{report}, now, nil)
	if len(cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(cards))
	}
	if len(cards[0].Windows) != 1 {
		t.Fatalf("windows = %d, want 1 collapsed shared row", len(cards[0].Windows))
	}
	if cards[0].Windows[0].Fraction == nil || math.Abs(*cards[0].Windows[0].Fraction-0.2) > 1e-9 {
		t.Errorf("fraction = %v, want 0.2", cards[0].Windows[0].Fraction)
	}

	tiered := usageTestLimit("anthropic", "a", "7d", "Weekly",
		ai.UsageAmount{UsedFraction: new(0.5), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK)
	tiered.Scope.Tier = "opus"
	if got := FormatLimitTitle(tiered); got != "Weekly (opus)" {
		t.Errorf("tier title = %q, want %q", got, "Weekly (opus)")
	}
	dup := tiered
	dup.Label = "Weekly (opus)"
	if got := FormatLimitTitle(dup); got != "Weekly (opus)" {
		t.Errorf("already-tiered title = %q", got)
	}
}

func TestUsageCardKeepsExhaustedBucketVisiblePastCap(t *testing.T) {
	now := time.Now().UnixMilli()
	labels := []string{"a", "b", "c", "d", "e"}
	limits := make([]ai.UsageLimit, 0, len(labels))
	for index, label := range labels {
		used := float64(index+1) / 10
		if index == 4 {
			used = 1
		}
		limits = append(limits, usageTestLimit("google-gemini-cli", "g", label, "Model "+label,
			ai.UsageAmount{UsedFraction: new(used), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK))
	}
	cards := BuildProviderCards([]ai.UsageReport{usageTestReport("google-gemini-cli", "g@test", limits, nil)}, now, nil)
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	got := make([]string, 0, 4)
	for _, window := range cards[0].Windows[:CardMaxWindows] {
		got = append(got, window.Label)
	}
	want := []string{"Model b", "Model c", "Model d", "Model e"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("visible windows = %v, want %v", got, want)
	}
	if len(cards[0].Windows) != len(labels) {
		t.Errorf("declared windows = %d, want %d kept after the cap", len(cards[0].Windows), len(labels))
	}
}

func TestUsageCardsCollapseIdleProvidersIntoOneTick(t *testing.T) {
	idle := usageTestReport("anthropic", "idle@test", []ai.UsageLimit{
		usageTestLimit("anthropic", "idle", "7d", "Weekly",
			ai.UsageAmount{UsedFraction: new(0.0), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
	}, nil)
	busy := usageTestReport("openai", "busy@test", []ai.UsageLimit{
		usageTestLimit("openai", "busy", "5h", "5 Hour",
			ai.UsageAmount{UsedFraction: new(0.6), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
	}, nil)
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports:       []ai.UsageReport{idle, busy},
		RequestRender: func() {},
	})
	defer dashboard.Dispose()
	text := plainLines(dashboard.Render(100))
	if !strings.Contains(text, "untouched: Anthropic") {
		t.Errorf("idle provider not collapsed:\n%s", text)
	}
	if !strings.Contains(text, "Openai") {
		t.Errorf("busy provider missing:\n%s", text)
	}
}

func TestUsageResetCreditSummary(t *testing.T) {
	now := time.Now()
	soon := now.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	spent := now.Add(time.Second).UTC().Format(time.RFC3339)
	summary := SummarizeUsageResetCredits(&ai.UsageResetCredits{
		AvailableCount: 3,
		Credits: []ai.UsageResetCreditDetail{
			{ID: "spent", RemainingCount: 0, ExpiresAt: spent},
			{ID: "banked", RemainingCount: 3, ExpiresAt: soon},
		},
	}, now.UnixMilli())
	if summary == nil {
		t.Fatal("nil summary")
	}
	if summary.BankedCount != 3 {
		t.Errorf("banked = %d, want 3", summary.BankedCount)
	}
	// RedeemableCount is absent, so it falls back to the banked count.
	if summary.RedeemableCount != 3 {
		t.Errorf("redeemable = %d, want 3", summary.RedeemableCount)
	}
	if summary.SoonestExpiry != soon {
		t.Errorf("soonest = %q, want %q", summary.SoonestExpiry, soon)
	}

	blocked := SummarizeUsageResetCredits(&ai.UsageResetCredits{
		AvailableCount:  2,
		RedeemableCount: new(0),
		Credits: []ai.UsageResetCreditDetail{
			{ID: "c", RemainingCount: 2, Usable: new(false), Blocking: []string{"anthropic:7d"}},
		},
	}, now.UnixMilli())
	if blocked.UnavailableReason != "blocked by Claude weekly" {
		t.Errorf("reason = %q, want %q", blocked.UnavailableReason, "blocked by Claude weekly")
	}

	notEligible := SummarizeUsageResetCredits(&ai.UsageResetCredits{
		AvailableCount: 1, RedeemableCount: new(0), Eligible: new(false),
	}, now.UnixMilli())
	if notEligible.UnavailableReason != "not eligible" {
		t.Errorf("reason = %q, want not eligible", notEligible.UnavailableReason)
	}
}

func TestUsageReportWrapsAndStacksOnNarrowWidths(t *testing.T) {
	now := time.Now().UnixMilli()
	reports := make([]ai.UsageReport, 0, 12)
	for index := 0; index < 12; index++ {
		email := "a" + twoDigits(index+1) + "@test"
		limit := usageTestLimit("openai", email, "7d", "Weekly",
			ai.UsageAmount{Used: new(float64((index + 1) * 8)), Limit: new(100.0), Unit: ai.UsageUnitPercent},
			ai.UsageStatusOK)
		// One upstream quota bucket: account columns share the bucket id.
		limit.ID = "openai:7d"
		reports = append(reports, usageTestReport("openai", email, []ai.UsageLimit{limit}, nil))
	}

	wrapped := plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
		Reports: reports, NowMs: now, Width: 40,
	}), "\n"))
	for index := 0; index < 12; index++ {
		email := "a" + twoDigits(index+1) + "@test"
		if !strings.Contains(wrapped, email) {
			t.Fatalf("account %s dropped when wrapping:\n%s", email, wrapped)
		}
	}
	if !strings.Contains(wrapped, "of 12") {
		t.Errorf("wrapped blocks do not announce their ranges:\n%s", wrapped)
	}
	for _, line := range strings.Split(wrapped, "\n") {
		if width := visibleWidthForTest(line); width > 40 {
			t.Errorf("line %q is %d cells wide, want <= 40", line, width)
		}
	}

	stacked := plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
		Reports: reports[:1], NowMs: now, Width: 14,
	}), "\n"))
	lines := strings.Split(stacked, "\n")
	labelAt := -1
	for index, line := range lines {
		if strings.Contains(line, "a01@test") {
			labelAt = index
			break
		}
	}
	if labelAt < 0 || labelAt+1 >= len(lines) {
		t.Fatalf("label row missing:\n%s", stacked)
	}
	if !strings.ContainsAny(lines[labelAt+1], "█░") {
		t.Errorf("stacked bar row = %q", lines[labelAt+1])
	}
	if !strings.Contains(lines[labelAt+1], "92% free") {
		t.Errorf("stacked value row = %q", lines[labelAt+1])
	}
	for _, line := range lines {
		if width := visibleWidthForTest(line); width > 14 {
			t.Errorf("line %q is %d cells wide, want <= 14", line, width)
		}
	}
}

func TestUsageReportMarksOnlyTheKeyedAccountActive(t *testing.T) {
	now := time.Now().UnixMilli()
	makeReport := func(accountID, accountKey string, used float64) ai.UsageReport {
		limit := usageTestLimit("openai", accountID, "7d", "Weekly",
			ai.UsageAmount{UsedFraction: new(used), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK)
		limit.ID = "openai:7d"
		return usageTestReport("openai", "shared@test", []ai.UsageLimit{limit},
			map[string]any{"email": "shared@test", "accountId": accountID, "accountKey": accountKey})
	}
	reports := []ai.UsageReport{makeReport("acct-1", "key-1", 0.1), makeReport("acct-2", "key-2", 0.9)}
	render := func(key string) string {
		return plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
			Reports: reports, NowMs: now, Width: 100,
			ResolveActiveAccount: func(string) *ai.UsageActiveAccount {
				return &ai.UsageActiveAccount{Email: "shared@test", AccountKey: &key}
			},
		}), "\n"))
	}
	first := render("key-1")
	if strings.Count(first, "●") != 1 {
		t.Errorf("key-1 marker count = %d, want 1:\n%s", strings.Count(first, "●"), first)
	}
	header := ""
	for _, line := range strings.Split(first, "\n") {
		if strings.Contains(line, "●") {
			header = line
			break
		}
	}
	// Worst-first ordering puts acct-2 (90% used) in the first column.
	if index := strings.Index(header, "●"); index < strings.Index(header, "acct-1@test") && strings.Contains(header, "acct-1@test") {
		t.Errorf("marker sits on the wrong column: %q", header)
	}
	second := render("key-2")
	if strings.Count(second, "●") != 1 {
		t.Errorf("key-2 marker count = %d, want 1:\n%s", strings.Count(second, "●"), second)
	}

	// A same-email report whose account id differs from the active identity is
	// never marked.
	unmarked := plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
		Reports: []ai.UsageReport{makeReport("acct-2", "", 0.5)},
		NowMs:   now,
		Width:   100,
		ResolveActiveAccount: func(string) *ai.UsageActiveAccount {
			return &ai.UsageActiveAccount{Email: "shared@test", AccountID: "acct-1"}
		},
	}), "\n"))
	if strings.Contains(unmarked, "●") {
		t.Errorf("mismatched account id was marked:\n%s", unmarked)
	}
}

func TestUsageReportDisambiguatesCodexWorkspaces(t *testing.T) {
	now := time.Now().UnixMilli()
	workspace := func(orgID string) ai.UsageReport {
		limit := usageTestLimit("openai-codex", orgID, "7d", "Weekly",
			ai.UsageAmount{UsedFraction: new(0.5), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK)
		// One upstream quota bucket: the account columns of a window keep the
		// provider's limit id, so both workspaces share it.
		limit.ID = "codex:7d"
		return usageTestReport("openai-codex", "login@test", []ai.UsageLimit{limit},
			map[string]any{"email": "login@test", "orgId": orgID, "planType": "chatgpt-plus"})
	}
	two := plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
		Reports: []ai.UsageReport{workspace("org-1"), workspace("org-2")}, NowMs: now, Width: 120,
	}), "\n"))
	if !strings.Contains(two, "login@test (org-1)") || !strings.Contains(two, "login@test (org-2)") {
		t.Errorf("workspaces not disambiguated:\n%s", two)
	}
	solo := plainLines(strings.Split(RenderUsageReports(UsageReportOptions{
		Reports: []ai.UsageReport{workspace("org-1")}, NowMs: now, Width: 120,
	}), "\n"))
	if strings.Contains(solo, "(org-1)") {
		t.Errorf("single workspace gained a synthetic suffix:\n%s", solo)
	}
}

func TestUsageHeatmapLayout(t *testing.T) {
	monday := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.Local)
	layout := BuildHeatmapLayout([]DailyActivityPoint{{Day: "2026-08-31", Cost: 5, Requests: 1}}, 2, monday)
	if got := layout.Cells[0][1]; got != 4 {
		t.Errorf("monday cell = %d, want 4", got)
	}
	for row := 1; row < 7; row++ {
		if got := layout.Cells[row][1]; got != -1 {
			t.Errorf("future cell[%d] = %d, want -1", row, got)
		}
	}

	scaled := BuildHeatmapLayout([]DailyActivityPoint{
		{Day: "2026-08-24", Cost: 100, Requests: 1},
		{Day: "2026-08-25", Cost: 30, Requests: 1},
		{Day: "2026-08-26", Cost: 6, Requests: 1},
		{Day: "2026-08-27", Cost: 0, Requests: 1},
	}, 2, monday)
	want := []int{4, 3, 1, 0, 0, 0, 0}
	for row := 0; row < 7; row++ {
		if scaled.Cells[row][0] != want[row] {
			t.Errorf("scaled cell[%d] = %d, want %d", row, scaled.Cells[row][0], want[row])
		}
	}

	counts := BuildHeatmapLayout([]DailyActivityPoint{
		{Day: "2026-08-24", Cost: 0, Requests: 50},
		{Day: "2026-08-25", Cost: 0, Requests: 3},
	}, 2, monday)
	if counts.Cells[0][0] != 4 || counts.Cells[1][0] != 1 {
		t.Errorf("request-scaled cells = %d,%d, want 4,1", counts.Cells[0][0], counts.Cells[1][0])
	}
	if counts.TotalRequests != 53 {
		t.Errorf("total requests = %d, want 53", counts.TotalRequests)
	}
	if len(counts.MonthLabels) != 2 || counts.MonthLabels[0] != "Aug" {
		t.Errorf("month labels = %v", counts.MonthLabels)
	}
}

func TestUsageActivityDedupesForksAndRescansChangedFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UnixMilli()
	entry := `{"type":"message","id":"shared","timestamp":` + usageTestInt(now) +
		`,"message":{"role":"assistant","timestamp":` + usageTestInt(now) + `,"usage":{"cost":{"total":1.25}}}}` + "\n"
	writeFileForTest(t, filepath.Join(root, "original.jsonl"), entry)
	writeFileForTest(t, filepath.Join(root, "fork.jsonl"), entry+
		`{"type":"message","id":"new","timestamp":`+usageTestInt(now)+
		`,"message":{"role":"assistant","timestamp":`+usageTestInt(now)+`,"usage":{"cost":{"total":0.5}}}}`+"\n")

	activity := NewUsageActivity([]string{root})
	points := loadActivityForTest(t, activity)
	if len(points) != 1 {
		t.Fatalf("points = %v", points)
	}
	if math.Abs(points[0].Cost-1.75) > 1e-9 || points[0].Requests != 2 {
		t.Errorf("deduped point = %+v, want cost 1.75 / 2 requests", points[0])
	}

	// Appending re-reads the changed file; a rewritten file drops its old spend.
	appendFileForTest(t, filepath.Join(root, "fork.jsonl"),
		`{"type":"usage","id":"cache","timestamp":`+usageTestInt(now)+`,"usage":{"cost":{"total":0.5}}}`+"\n")
	points = loadActivityForTest(t, activity)
	if math.Abs(points[0].Cost-2.25) > 1e-9 || points[0].Requests != 3 {
		t.Errorf("after append = %+v, want cost 2.25 / 3 requests", points[0])
	}

	writeFileForTest(t, filepath.Join(root, "original.jsonl"), "")
	points = loadActivityForTest(t, activity)
	// The fork's own copy of the shared request plus its two new ones remain.
	if math.Abs(points[0].Cost-2.25) > 1e-9 || points[0].Requests != 3 {
		t.Errorf("after rewrite = %+v, want cost 2.25 / 3 requests", points[0])
	}

	if err := os.Remove(filepath.Join(root, "fork.jsonl")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	points = loadActivityForTest(t, activity)
	if len(points) != 0 {
		t.Errorf("after delete = %+v, want no points", points)
	}
}

func TestUsageActivityToleratesForeignAndTruncatedLines(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UnixMilli()
	iso := time.UnixMilli(now).UTC().Format(time.RFC3339Nano)
	content := "malformed\n" +
		`{"type":"message","id":"no-usage","message":{"role":"assistant"}}` + "\n" +
		`{"type":"usage","id":"cache","timestamp":"` + iso + `","usage":{"cost":{"total":0.5}}}` + "\n" +
		`{"type":"model_usage","id":"omp-call","timestamp":"` + iso + `","usage":{"cost":{"total":1}}}` + "\n" +
		`{"type":"message","id":"user","message":{"role":"user"},"timestamp":"` + iso + `"}` + "\n" +
		`{"type":"message","id":"truncated","timestamp":` + usageTestInt(now) + `,"message":{"role":"assistant","timestamp":` + usageTestInt(now)
	writeFileForTest(t, filepath.Join(root, "session.jsonl"), content)
	points := loadActivityForTest(t, NewUsageActivity([]string{root}))
	if len(points) != 1 {
		t.Fatalf("points = %+v", points)
	}
	if math.Abs(points[0].Cost-1.5) > 1e-9 || points[0].Requests != 2 {
		t.Errorf("parsed point = %+v, want cost 1.5 / 2 requests", points[0])
	}
}

func TestUsageActivityExcludesExpiredHistoryAndStopsAfterClose(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UnixMilli()
	old := now - 372*24*60*60*1000
	writeFileForTest(t, filepath.Join(root, "session.jsonl"),
		`{"type":"message","id":"old","timestamp":`+usageTestInt(old)+
			`,"message":{"role":"assistant","timestamp":`+usageTestInt(old)+`,"usage":{"cost":{"total":10}}}}`+"\n"+
			`{"type":"message","id":"new","timestamp":`+usageTestInt(now)+
			`,"message":{"role":"assistant","timestamp":`+usageTestInt(now)+`,"usage":{"cost":{"total":1}}}}`+"\n")
	points := loadActivityForTest(t, NewUsageActivity([]string{root}))
	if len(points) != 1 || math.Abs(points[0].Cost-1) > 1e-9 {
		t.Errorf("points = %+v, want only the in-retention day", points)
	}

	activity := NewUsageActivity([]string{root})
	if err := activity.Load(context.Background(), nil); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	published := 0
	err := activity.Load(ctx, func([]DailyActivityPoint) {
		published++
		cancel()
	})
	if err == nil {
		t.Error("cancelled load returned nil error")
	}
	if published != 1 {
		t.Errorf("published %d times after cancel, want 1 (cached points only)", published)
	}
}

func TestUsageDashboardInteraction(t *testing.T) {
	closed := 0
	reports := []ai.UsageReport{usageTestReport("anthropic", "a@test", []ai.UsageLimit{
		usageTestLimit("anthropic", "a", "7d", "Weekly",
			ai.UsageAmount{UsedFraction: new(0.35), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
	}, nil)}
	renders := 0
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports: reports,
		LoadActivity: func(_ context.Context, push func([]DailyActivityPoint)) error {
			push([]DailyActivityPoint{{Day: "2026-08-24", Cost: 3, Requests: 2}})
			return nil
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() { renders++ },
		OnClose:       func() { closed++ },
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()

	overview := plainLines(dashboard.Render(100))
	if !strings.Contains(overview, "65%") {
		t.Errorf("overview missing bar value:\n%s", overview)
	}
	if !strings.Contains(overview, "Activity") {
		t.Errorf("overview missing heatmap:\n%s", overview)
	}

	dashboard.HandleInput("\t")
	detail := plainLines(dashboard.Render(100))
	if !strings.Contains(detail, "Usage · Details") || !strings.Contains(detail, "Weekly") {
		t.Errorf("toggle to details failed:\n%s", detail)
	}
	// Escape from details returns to the board instead of closing.
	dashboard.HandleInput("\x1b")
	if closed != 0 {
		t.Error("escape closed the board from the details view")
	}
	if !strings.Contains(plainLines(dashboard.Render(100)), "Activity") {
		t.Error("escape did not return to the overview")
	}
	// Enter is an equivalent toggle, not a close.
	dashboard.HandleInput("\r")
	if !strings.Contains(plainLines(dashboard.Render(100)), "Usage · Details") {
		t.Error("enter did not toggle to details")
	}
}

func TestUsageDashboardHonorsRemappedBindings(t *testing.T) {
	previous := GetKeybindings()
	defer SetKeybindings(previous)

	definitions := TUIKeybindingDefinitionsFor(HostKeybindingPlatform())
	definitions["app.usage.toggleView"] = TUIKeybindingDef{DefaultKeys: []string{"ctrl+y"}, Description: "toggle"}
	definitions["app.usage.refresh"] = TUIKeybindingDef{DefaultKeys: []string{"shift+r"}, Description: "refresh"}
	SetKeybindings(NewKeybindingsManager(definitions, nil))

	refreshes := 0
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports: []ai.UsageReport{usageTestReport("anthropic", "a@test", []ai.UsageLimit{
			usageTestLimit("anthropic", "a", "7d", "Weekly",
				ai.UsageAmount{UsedFraction: new(0.3), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
		}, nil)},
		LoadActivity: func(_ context.Context, push func([]DailyActivityPoint)) error {
			push(nil)
			return nil
		},
		Refresh: func(context.Context) (UsageDashboardSnapshot, error) {
			refreshes++
			return UsageDashboardSnapshot{}, nil
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() {},
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()

	// The default tab no longer toggles once the user remaps the action.
	dashboard.HandleInput("\t")
	if !strings.Contains(plainLines(dashboard.Render(100)), "Activity") {
		t.Error("tab toggled the remapped view")
	}
	dashboard.HandleInput("\x19")
	dashboard.waitBlocks()
	if !strings.Contains(plainLines(dashboard.Render(100)), "Usage · Details") {
		t.Error("remapped toggle key did not switch views")
	}
	dashboard.HandleInput("R")
	dashboard.waitBlocks()
	if refreshes != 1 {
		t.Errorf("remapped refresh ran %d times, want 1", refreshes)
	}
}

func TestUsageDashboardClampsScrollAndKeepsFrameHeight(t *testing.T) {
	windows := make([]ai.UsageLimit, 0, 10)
	for index := 0; index < 10; index++ {
		windowID := "w" + strconv.Itoa(index+1)
		windows = append(windows, usageTestLimit("openai", "a", windowID, "Quota "+strconv.Itoa(index+1),
			ai.UsageAmount{UsedFraction: new(float64(index+1) / 10), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK))
	}
	rows := 12
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports: []ai.UsageReport{usageTestReport("openai", "a@test", windows, nil)},
		LoadActivity: func(_ context.Context, push func([]DailyActivityPoint)) error {
			push(nil)
			return nil
		},
		ViewportRows:  func() int { return rows },
		RequestRender: func() {},
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()

	if lines := dashboard.Render(100); len(lines) != 12 {
		t.Fatalf("frame rows = %d, want 12", len(lines))
	}
	dashboard.HandleInput("\t")
	if strings.Contains(plainLines(dashboard.Render(100)), "Quota 10") {
		t.Error("last quota visible before scrolling")
	}
	for range 10 {
		dashboard.HandleInput("\x1b[6~")
	}
	if !strings.Contains(plainLines(dashboard.Render(100)), "Quota 10") {
		t.Error("scrolling did not reach the last quota")
	}
	// Further scrolling stays clamped.
	dashboard.HandleInput("\x1b[6~")
	if lines := dashboard.Render(100); len(lines) != 12 {
		t.Errorf("frame rows after clamped scroll = %d, want 12", len(lines))
	}
	// Shrinking the terminal keeps the frame consistent.
	rows = 8
	if lines := dashboard.Render(100); len(lines) != 8 {
		t.Errorf("narrow frame rows = %d, want 8", len(lines))
	}
	// The configured top binding returns to the report head.
	dashboard.HandleInput("\x1b[1;5H")
	if strings.Contains(plainLines(dashboard.Render(100)), "Quota 10") {
		t.Error("top binding did not jump to the head")
	}
	dashboard.HandleInput("\x1b[1;5F")
	if !strings.Contains(plainLines(dashboard.Render(100)), "Quota 10") {
		t.Error("bottom binding did not jump to the tail")
	}
}

func TestUsageDashboardRefreshKeepsLastGoodReports(t *testing.T) {
	failure := dashboardRefreshError{}
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		InitialRefresh: true,
		Reports: []ai.UsageReport{usageTestReport("anthropic", "a@test", []ai.UsageLimit{
			usageTestLimit("anthropic", "a", "7d", "Old quota",
				ai.UsageAmount{UsedFraction: new(0.5), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
		}, nil)},
		LoadActivity: func(_ context.Context, push func([]DailyActivityPoint)) error {
			push(nil)
			return nil
		},
		Refresh: func(context.Context) (UsageDashboardSnapshot, error) {
			return UsageDashboardSnapshot{}, failure
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() {},
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()
	text := plainLines(dashboard.Render(100))
	if !strings.Contains(text, "refresh failed: quota endpoint down") {
		t.Errorf("refresh failure not reported:\n%s", text)
	}
	if !strings.Contains(text, "50%") {
		t.Errorf("last good reports not retained:\n%s", text)
	}
}

type dashboardRefreshError struct{}

func (dashboardRefreshError) Error() string { return "quota endpoint down" }

func TestUsageDashboardReportsActivityFailure(t *testing.T) {
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		LoadActivity: func(context.Context, func([]DailyActivityPoint)) error {
			return errActivityFailed
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() {},
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()
	text := plainLines(dashboard.Render(100))
	if !strings.Contains(text, "Usage history unavailable (activity scan failed)") {
		t.Errorf("activity failure not reported:\n%s", text)
	}
}

var errActivityFailed = activityTestError("activity scan failed")

type activityTestError string

func (e activityTestError) Error() string { return string(e) }

func TestUsageDashboardRefreshCancelsWithoutStart(t *testing.T) {
	cancelled := make(chan struct{})
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		// No Start: a host that refreshes first must still be able to stop it.
		Refresh: func(ctx context.Context) (UsageDashboardSnapshot, error) {
			<-ctx.Done()
			close(cancelled)
			return UsageDashboardSnapshot{}, ctx.Err()
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() {},
	})
	dashboard.HandleInput("r")
	dashboard.Dispose()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Dispose did not cancel a refresh started before Start")
	}
}

func TestUsageDashboardCloseAbortsAndStopsRendering(t *testing.T) {
	closed := 0
	renders := 0
	aborted := make(chan struct{})
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		LoadActivity: func(ctx context.Context, push func([]DailyActivityPoint)) error {
			push([]DailyActivityPoint{{Day: "2026-08-24", Cost: 1, Requests: 1}})
			<-ctx.Done()
			close(aborted)
			return ctx.Err()
		},
		Refresh: func(context.Context) (UsageDashboardSnapshot, error) {
			return UsageDashboardSnapshot{}, nil
		},
		ViewportRows:  func() int { return 14 },
		RequestRender: func() { renders++ },
		OnClose:       func() { closed++ },
	})
	dashboard.Start(context.Background())
	dashboard.HandleInput("\x1b")
	if closed != 1 {
		t.Fatalf("close ran %d times, want 1", closed)
	}
	if lines := dashboard.Render(100); len(lines) != 0 {
		t.Errorf("closed dashboard rendered %d lines", len(lines))
	}
	dashboard.Dispose()
	select {
	case <-aborted:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not abort the activity scan")
	}
	before := renders
	dashboard.HandleInput("r")
	if renders != before {
		t.Errorf("closed dashboard requested %d renders", renders-before)
	}
	dashboard.HandleInput("\x1b")
	if closed != 1 {
		t.Errorf("second close ran, count = %d", closed)
	}
}

func TestUsageDashboardStripsControlCharactersAndFitsWidth(t *testing.T) {
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports: []ai.UsageReport{usageTestReport("anthropic", "a@test", []ai.UsageLimit{
			usageTestLimit("anthropic", "a", "5h", "Claude\t7 Day\x1b[2J\x07\r\n(Fable)",
				ai.UsageAmount{UsedFraction: new(0.4), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
			usageTestLimit("anthropic", "a", "7d", "Claude\t7 Day\x1b[2J\x07\r\n(Fable)",
				ai.UsageAmount{UsedFraction: new(0.2), Unit: ai.UsageUnitPercent}, ai.UsageStatusOK),
		}, nil)},
		LoadActivity: func(_ context.Context, push func([]DailyActivityPoint)) error {
			push(nil)
			return nil
		},
		ViewportRows:  func() int { return 24 },
		RequestRender: func() {},
	})
	dashboard.Start(context.Background())
	dashboard.waitBlocks()
	defer dashboard.Dispose()
	for _, width := range []int{24, 40, 72} {
		lines := dashboard.Render(width)
		for _, line := range lines {
			if strings.ContainsAny(line, "\t\r\n\x07") {
				t.Errorf("width %d line keeps a control character: %q", width, line)
			}
			if strings.Contains(line, "\x1b[2J") {
				t.Errorf("width %d line keeps a terminal sequence", width)
			}
			if got := visibleWidthForTest(line); got > width {
				t.Errorf("width %d line is %d cells", width, got)
			}
		}
		text := plainLines(lines)
		if !strings.Contains(text, "60%") || !strings.Contains(text, "80%") {
			t.Errorf("width %d lost bar values:\n%s", width, text)
		}
	}
}

func TestFormatActivityErrorDetail(t *testing.T) {
	detail := FormatActivityErrorDetail(
		"worker spawn failed at /Users/tester/.omp/stats.db:\n\ttrace\x1b[2J\r\nline 2...", "/Users/tester")
	want := "worker spawn failed at ~/.omp/stats.db: trace line 2"
	if detail != want {
		t.Errorf("detail = %q, want %q", detail, want)
	}
	// Home shortening is case-insensitive and covers the forward-slash form.
	upper := FormatActivityErrorDetail(
		"read C:\\Users\\Tester\\.omp and C:/USERS/TESTER/.omp failed", "C:\\Users\\Tester")
	if strings.Contains(strings.ToLower(upper), "tester") {
		t.Errorf("home path survived reporting: %q", upper)
	}
}

func TestUsageDurationAndCompactCount(t *testing.T) {
	cases := map[int64]string{
		0:                   "0ms",
		450:                 "450ms",
		1500:                "1.5s",
		200000:              "3m20s",
		8100000:             "2h15m",
		273600000:           "3d4h",
		24 * 60 * 60 * 1000: "1d",
	}
	for input, want := range cases {
		if got := FormatDuration(input); got != want {
			t.Errorf("FormatDuration(%d) = %q, want %q", input, got, want)
		}
	}
	counts := map[float64]string{
		5600: "5.6K", 1200000: "1.2M", 42: "42", 999: "999", 1000: "1K",
	}
	for input, want := range counts {
		if got := FormatCompactCount(input); got != want {
			t.Errorf("FormatCompactCount(%v) = %q, want %q", input, got, want)
		}
	}
}

func usageTestInt(value int64) string { return strconv.FormatInt(value, 10) }

func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// stripUsageLine removes ANSI/OSC sequences for plain-text assertions.
func stripUsageLine(line string) string { return widthx.StripTerminalSequences(line) }

// visibleWidthForTest is the terminal cell width of rendered text.
func visibleWidthForTest(line string) int { return widthx.VisibleWidth(line) }

// writeFileForTest writes a fixture file.
func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// appendFileForTest appends to a fixture file.
func appendFileForTest(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}

// loadActivityForTest loads and returns the scanner's points.
func loadActivityForTest(t *testing.T, activity *UsageActivity) []DailyActivityPoint {
	t.Helper()
	var points []DailyActivityPoint
	if err := activity.Load(context.Background(), func(next []DailyActivityPoint) { points = next }); err != nil {
		t.Fatalf("load activity: %v", err)
	}
	return points
}
