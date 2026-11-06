package ai

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func usageTestReport(limits ...UsageLimit) *UsageReport {
	return &UsageReport{Provider: "test", FetchedAt: 1, Limits: limits}
}

func usageTestProvider(fetch func(context.Context, UsageFetchParams, UsageFetchContext) (*UsageReport, error)) *UsageProvider {
	return &UsageProvider{ID: "test", FetchUsage: fetch}
}

func TestUsageUsedFractionPrecedence(t *testing.T) {
	used := 25.0
	limit := 200.0
	remainingFraction := 0.9
	cases := []struct {
		name   string
		amount UsageAmount
		want   float64
		ok     bool
	}{
		{"explicit fraction", UsageAmount{UsedFraction: new(0.42), Used: &used, Limit: &limit, Unit: UsageUnitPercent}, 0.42, true},
		{"used over limit", UsageAmount{Used: &used, Limit: &limit, Unit: UsageUnitTokens}, 0.125, true},
		{"percent used", UsageAmount{Used: new(75.0), Unit: UsageUnitPercent}, 0.75, true},
		{"inverted remaining fraction", UsageAmount{RemainingFraction: &remainingFraction, Unit: UsageUnitPercent}, 0.1, true},
		{"no amount", UsageAmount{Unit: UsageUnitUnknown}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := UsageUsedFraction(UsageLimit{Amount: tc.amount})
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("fraction = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsageLimitStatusKeepsProviderVerdict(t *testing.T) {
	// A provider that still allows requests over a 100% advisory window reports
	// warning itself; the status must not be re-derived from the fraction.
	full := 1.0
	limit := UsageLimit{Amount: UsageAmount{UsedFraction: &full, Unit: UsageUnitPercent}, Status: UsageStatusWarning}
	if got := UsageLimitStatus(limit); got != UsageStatusWarning {
		t.Fatalf("status = %q, want warning", got)
	}
	limit.Status = ""
	if got := UsageLimitStatus(limit); got != UsageStatusExhausted {
		t.Fatalf("status = %q, want exhausted", got)
	}
}

func TestSortUsageLimitsExhaustedFirst(t *testing.T) {
	low := 0.1
	high := 0.8
	full := 1.0
	limits := []UsageLimit{
		{ID: "a", Label: "A", Amount: UsageAmount{UsedFraction: &low, Unit: UsageUnitPercent}},
		{ID: "b", Label: "B", Amount: UsageAmount{UsedFraction: &full, Unit: UsageUnitPercent}},
		{ID: "c", Label: "C", Amount: UsageAmount{UsedFraction: &high, Unit: UsageUnitPercent}},
	}
	SortUsageLimits(limits)
	got := []string{limits[0].ID, limits[1].ID, limits[2].ID}
	want := []string{"b", "c", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestCodexRateLimitHeadersProduceWindows(t *testing.T) {
	headers := map[string]string{
		"x-codex-primary-used-percent":     "40",
		"x-codex-primary-window-minutes":   "300",
		"x-codex-secondary-used-percent":   "95",
		"x-codex-secondary-window-minutes": "10080",
	}
	report := parseCodexRateLimitHeaders(headers, 1_700_000_000_000, new(int(200)))
	if report == nil {
		t.Fatal("report is nil")
	}
	if len(report.Limits) != 2 {
		t.Fatalf("limits = %d, want 2", len(report.Limits))
	}
	if report.Limits[0].Window.ID != "5h" || report.Limits[1].Window.ID != "7d" {
		t.Fatalf("windows = %q/%q", report.Limits[0].Window.ID, report.Limits[1].Window.ID)
	}
	// A successful response marks both windows explicitly allowed, so even a
	// rounded 100% window stays a warning rather than a block.
	if report.Limits[1].Status != UsageStatusWarning {
		t.Fatalf("secondary status = %q, want warning", report.Limits[1].Status)
	}
	if fraction, ok := UsageUsedFraction(report.Limits[0]); !ok || fraction != 0.4 {
		t.Fatalf("primary fraction = %v/%v", fraction, ok)
	}
}

func TestCodexUsagePayloadKeepsSparkMeterSeparate(t *testing.T) {
	payload := map[string]any{
		"plan_type": "pro",
		"rate_limit": map[string]any{
			"allowed":       true,
			"limit_reached": false,
			"primary_window": map[string]any{
				"used_percent":         10,
				"limit_window_seconds": 18000,
			},
		},
		"additional_rate_limits": []any{
			map[string]any{
				"limit_name":      "GPT-5.3-Codex-Spark",
				"metered_feature": "bengalfox",
				"rate_limit": map[string]any{
					"allowed":       false,
					"limit_reached": true,
					"primary_window": map[string]any{
						"used_percent":         100,
						"limit_window_seconds": 18000,
					},
				},
			},
		},
	}
	parsed := parseCodexUsagePayload(payload)
	if parsed == nil {
		t.Fatal("payload did not parse")
	}
	if parsed.creditOverage {
		t.Fatal("credit overage must not apply without credits")
	}
	slug := codexAdditionalLimitSlug("GPT-5.3-Codex-Spark", "bengalfox")
	if slug != "spark" {
		t.Fatalf("slug = %q, want spark", slug)
	}
	limit := buildCodexAdditionalUsageLimit("primary", slug, codexAdditionalDisplayName(slug, "GPT-5.3-Codex-Spark"), parsed.additional[0].primary, "", "GPT-5.3-Codex-Spark", "bengalfox", parsed.additional[0].allowed, parsed.additional[0].limitReached, 1_700_000_000_000)
	if limit.ID != "openai-codex:spark:primary" {
		t.Fatalf("id = %q", limit.ID)
	}
	if limit.Status != UsageStatusExhausted {
		t.Fatalf("status = %q, want exhausted", limit.Status)
	}
}

func TestCodexSpendCapDrivesCreditSeats(t *testing.T) {
	payload := map[string]any{
		"plan_type":     "business",
		"spend_control": map[string]any{"reached": false, "individual_limit": map[string]any{"used": 80, "limit": 100}},
	}
	parsed := parseCodexUsagePayload(payload)
	if parsed == nil || parsed.spendCap == nil {
		t.Fatal("spend cap did not parse")
	}
	limit := buildCodexSpendCapLimit(parsed.spendCap, "acct", 1_700_000_000_000)
	if limit.Amount.Unit != UsageUnitCredits {
		t.Fatalf("unit = %q", limit.Amount.Unit)
	}
	// Upstream's thresholds warn at 90%, so 80% of the cap is still healthy.
	if limit.Status != UsageStatusOK {
		t.Fatalf("status = %q, want ok", limit.Status)
	}
}

func TestClaudeUsagePayloadScopedWeeklyRowsStayUnshared(t *testing.T) {
	payload := map[string]any{
		"five_hour": map[string]any{"utilization": 20.0},
		"seven_day": map[string]any{"utilization": 50.0},
		"limits": []any{
			map[string]any{
				"kind":      "weekly_scoped",
				"percent":   100,
				"is_active": false,
				"scope":     map[string]any{"model": map[string]any{"display_name": "Fable"}},
			},
		},
	}
	report := parseClaudeUsagePayload(payload, claudeUsageIdentity{}, "", "", 1_700_000_000_000)
	if report == nil {
		t.Fatal("report is nil")
	}
	var scoped *UsageLimit
	for index := range report.Limits {
		if report.Limits[index].Scope.Tier == "fable" {
			scoped = &report.Limits[index]
		}
	}
	if scoped == nil {
		t.Fatal("scoped weekly row missing")
	}
	if scoped.Scope.Shared {
		t.Fatal("a model-scoped weekly row must not be shared")
	}
	if scoped.Amount.UsedFraction == nil || *scoped.Amount.UsedFraction != 1 {
		t.Fatalf("fraction = %v", scoped.Amount.UsedFraction)
	}
	for _, limit := range report.Limits {
		if limit.ID == "anthropic:5h" && !limit.Scope.Shared {
			t.Fatal("the 5h umbrella window must be shared")
		}
	}
}

func TestClaudeExtraUsagePrefersSpendBlock(t *testing.T) {
	payload := map[string]any{
		"five_hour": map[string]any{"utilization": 10.0},
		"spend": map[string]any{
			"enabled": true,
			"used":    map[string]any{"amount_minor": 1250, "currency": "USD", "exponent": 2},
			"limit":   map[string]any{"amount_minor": 5000, "currency": "USD", "exponent": 2},
		},
		"extra_usage": map[string]any{"is_enabled": true, "monthly_limit": 100},
	}
	limit := buildClaudeExtraUsageLimit(payload)
	if limit == nil {
		t.Fatal("extra usage missing")
	}
	if limit.Amount.Unit != UsageUnitUSD {
		t.Fatalf("unit = %q", limit.Amount.Unit)
	}
	if limit.Amount.Used == nil || *limit.Amount.Used != 12.5 {
		t.Fatalf("used = %v", limit.Amount.Used)
	}
	if limit.Amount.Limit == nil || *limit.Amount.Limit != 50 {
		t.Fatalf("limit = %v", limit.Amount.Limit)
	}
}

func TestCursorUnifiedPrepaidCreditExhaustsThirdParty(t *testing.T) {
	payload := map[string]any{
		"membershipType": "unified_pro",
		"individualUsage": map[string]any{
			"plan":     map[string]any{"enabled": true, "limit": 2000, "totalPercentUsed": 25},
			"onDemand": map[string]any{"enabled": false, "limit": 0},
		},
	}
	report := parseCursorIndividualUsage(payload, 1_700_000_000_000, new(int64(1_700_100_000_000)))
	if report == nil {
		t.Fatal("report is nil")
	}
	byID := map[string]UsageLimit{}
	for _, limit := range report.Limits {
		byID[limit.ID] = limit
	}
	pool, ok := byID["cursor:usd:individual-auto"]
	if !ok {
		t.Fatal("weekly pool missing")
	}
	if pool.Window == nil || pool.Window.ID != "weekly" {
		t.Fatalf("pool window = %+v", pool.Window)
	}
	credit, ok := byID["cursor:usd:individual-api"]
	if !ok {
		t.Fatal("prepaid credit row missing")
	}
	if credit.Status != UsageStatusExhausted {
		t.Fatalf("credit status = %q, want exhausted", credit.Status)
	}
	if credit.Window == nil || credit.Window.ResetsAt != nil {
		t.Fatal("prepaid credit must carry no reset deadline")
	}
}

func TestZaiCreditLimitPrefersAbsoluteRatio(t *testing.T) {
	payload := map[string]any{
		"success": true,
		"data": map[string]any{
			"level": "pro",
			"limits": []any{
				map[string]any{"type": "CREDIT_LIMIT", "usage": 12000, "currentValue": 1438, "percentage": 11, "unit": 3, "number": 5},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != zaiQuotaPath {
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(payload)
	}))
	defer server.Close()
	report, err := fetchZaiUsage(context.Background(), UsageFetchParams{
		Provider:   "zai",
		Credential: UsageCredential{Type: CredentialAPIKey, APIKey: "key"},
		BaseURL:    server.URL,
	}, UsageFetchContext{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report == nil || len(report.Limits) == 0 {
		t.Fatal("no limits parsed")
	}
	limit := report.Limits[0]
	if limit.Window.ID != "5h" {
		t.Fatalf("window = %q", limit.Window.ID)
	}
	if limit.Amount.UsedFraction == nil {
		t.Fatal("fraction missing")
	}
	// 1438/12000 ≈ 0.1198, not the rounded 11%.
	if got := *limit.Amount.UsedFraction; got < 0.119 || got > 0.121 {
		t.Fatalf("fraction = %v", got)
	}
	if report.Metadata["planType"] != "pro" {
		t.Fatalf("planType = %v", report.Metadata["planType"])
	}
}

func TestDevinUserStatusRequestEncodesSessionToken(t *testing.T) {
	body := encodeDevinUserStatusRequest("abc.def.ghi")
	if len(body) == 0 {
		t.Fatal("empty request body")
	}
	// The metadata (field 1) must carry the prefixed session token (field 3).
	if !strings.Contains(string(body), devinSessionPrefix) {
		t.Fatal("session token prefix missing from request")
	}
}

func TestDevinDecodesPlanStatusQuotas(t *testing.T) {
	planInfo := &devinPbWriter{}
	planInfo.appendString(2, "Devin Pro")
	planInfo.appendTag(12, 0)
	planInfo.appendVarint(500)
	status := &devinPbWriter{}
	status.appendTag(1, 2)
	status.appendVarint(uint64(len(planInfo.buffer)))
	status.buffer = append(status.buffer, planInfo.buffer...)
	status.appendTag(14, 0)
	status.appendVarint(40)
	status.appendTag(17, 0)
	status.appendVarint(1_700_000_000)
	userStatus := &devinPbWriter{}
	userStatus.appendString(7, "dev@example.com")
	userStatus.appendString(36, "user-1")
	userStatus.appendTag(13, 2)
	userStatus.appendVarint(uint64(len(status.buffer)))
	userStatus.buffer = append(userStatus.buffer, status.buffer...)
	response := &devinPbWriter{}
	response.appendTag(1, 2)
	response.appendVarint(uint64(len(userStatus.buffer)))
	response.buffer = append(response.buffer, userStatus.buffer...)

	decoded, rootPlan := decodeDevinUserStatusResponse(response.buffer)
	if decoded == nil || decoded.planStatus == nil {
		t.Fatal("user status did not decode")
	}
	report := buildDevinReport(decoded, rootPlan)
	if report == nil {
		t.Fatal("report is nil")
	}
	if report.Metadata["planType"] != "Devin Pro" {
		t.Fatalf("planType = %v", report.Metadata["planType"])
	}
	var daily *UsageLimit
	for index := range report.Limits {
		if report.Limits[index].ID == "devin:quota:daily" {
			daily = &report.Limits[index]
		}
	}
	if daily == nil {
		t.Fatal("daily quota missing")
	}
	// 40% remaining means 60% used.
	if daily.Amount.UsedFraction == nil || *daily.Amount.UsedFraction != 0.6 {
		t.Fatalf("fraction = %v", daily.Amount.UsedFraction)
	}
}

func TestAlibabaUsedFractionAcceptsRatioOrPercent(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  float64
	}{{0.25, 0.25}, {25.0, 0.25}, {150.0, 1}} {
		fraction := parseAlibabaUsedFraction(tc.value)
		if fraction == nil || *fraction != tc.want {
			t.Fatalf("value %v -> %v, want %v", tc.value, fraction, tc.want)
		}
	}
}

func TestMinimaxUnavailablePlanRequiresUnlimitedZeroBuckets(t *testing.T) {
	zero := 0.0
	unlimited := float64(minimaxStatusUnlimited)
	bucket := &minimaxTokenPlanBucket{
		modelName:          "video",
		intervalTotalCount: &zero,
		weeklyTotalCount:   &zero,
		intervalStatus:     &unlimited,
		weeklyStatus:       &unlimited,
	}
	if !minimaxUnavailablePlan(bucket) {
		t.Fatal("bucket should read as unavailable")
	}
	one := 1.0
	bucket.intervalStatus = &one
	if minimaxUnavailablePlan(bucket) {
		t.Fatal("a live bucket must not read as unavailable")
	}
}

func TestUsageCacheIdentitySeparatesAccountsAndStaysStableAcrossRefresh(t *testing.T) {
	first := UsageCacheIdentity(UsageCredential{Type: CredentialOAuth, Email: "a@example.com", AccessToken: "one"})
	second := UsageCacheIdentity(UsageCredential{Type: CredentialOAuth, Email: "A@Example.com", AccessToken: "two"})
	if first != second {
		t.Fatalf("token rotation changed the identity: %q vs %q", first, second)
	}
	other := UsageCacheIdentity(UsageCredential{Type: CredentialOAuth, Email: "b@example.com"})
	if first == other {
		t.Fatal("different accounts share an identity")
	}
	// Without a stable identifier the secret decides, so two keys never share.
	keyOne := UsageCacheIdentity(UsageCredential{Type: CredentialAPIKey, APIKey: "sk-one"})
	keyTwo := UsageCacheIdentity(UsageCredential{Type: CredentialAPIKey, APIKey: "sk-two"})
	if keyOne == keyTwo {
		t.Fatal("two API keys share an identity")
	}
	if strings.Contains(keyOne, "sk-one") {
		t.Fatal("a secret leaked into the cache identity")
	}
}

func TestUsageProvidersCoverRegistry(t *testing.T) {
	providers := DefaultUsageProviders()
	seen := map[string]bool{}
	for _, provider := range providers {
		if provider.ID == "" || provider.FetchUsage == nil {
			t.Fatalf("incomplete provider %+v", provider)
		}
		if seen[provider.ID] {
			t.Fatalf("duplicate provider %q", provider.ID)
		}
		seen[provider.ID] = true
	}
	for _, id := range []string{
		"alibaba-token-plan", "openai-codex", "kimi-code", "minimax-code", "muse-code",
		"google-antigravity", "google-gemini-cli", "ollama", "factory-droid", "ollama-cloud",
		"anthropic", "cline-pass", "zai", "umans", "opencode-go", "github-copilot", "cursor",
		"synthetic", "xai-oauth", "devin", "charm-hyper", "commandcode",
	} {
		if DefaultUsageProvider(id) == nil {
			t.Fatalf("registry is missing %q", id)
		}
	}
	if len(providers) != 22 {
		t.Fatalf("providers = %d, want 22", len(providers))
	}
}

func TestOllamaReportsNoQuotaEndpoint(t *testing.T) {
	report := fetchOllamaUsage(UsageFetchParams{Provider: "ollama"})
	if report == nil || len(report.Limits) != 0 || len(report.Notes) == 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestOllamaDoesNotTouchNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))
	defer server.Close()
	provider := DefaultUsageProvider("ollama")
	report, err := provider.FetchUsage(context.Background(), UsageFetchParams{
		Provider:   "ollama",
		Credential: UsageCredential{Type: CredentialAPIKey, APIKey: "key"},
		BaseURL:    server.URL,
	}, UsageFetchContext{HTTPClient: server.Client()})
	if err != nil || report == nil {
		t.Fatalf("fetch: %v %v", report, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requests.Load())
	}
}

func TestCharmHyperBalanceIsSharedAndWindowless(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/credits" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{"balance": 94.5}`))
	}))
	defer server.Close()
	report, err := fetchCharmHyperUsage(context.Background(), UsageFetchParams{
		Provider:   "charm-hyper",
		Credential: UsageCredential{Type: CredentialAPIKey, APIKey: "key"},
		BaseURL:    server.URL,
	}, UsageFetchContext{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report = %+v", report)
	}
	limit := report.Limits[0]
	if !limit.Scope.Shared || limit.Window != nil {
		t.Fatalf("limit = %+v", limit)
	}
	if limit.Amount.Unit != UsageUnitCredits || limit.Amount.Remaining == nil || *limit.Amount.Remaining != 94.5 {
		t.Fatalf("amount = %+v", limit.Amount)
	}
}
