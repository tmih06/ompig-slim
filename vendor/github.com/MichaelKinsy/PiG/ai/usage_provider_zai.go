package ai

// Ports packages/ai/src/usage/zai.ts, usage/umans.ts, usage/cline-pass.ts and
// usage/alibaba-token-plan.ts.

import (
	"context"
	"math"
	"strings"
)

// --- Z.AI -------------------------------------------------------------------

const (
	zaiDefaultEndpoint = "https://api.z.ai"
	zaiQuotaPath       = "/api/monitor/usage/quota/limit"
	zaiModelUsagePath  = "/api/monitor/usage/model-usage"
	zaiMonthMs         = 30 * usageDayMs
)

// zaiUsageLimitItem is one `limits[]` entry.
type zaiUsageLimitItem struct {
	kind         string
	usage        *float64
	currentValue *float64
	percentage   *float64
	remaining    *float64
	nextReset    *int64
	unit         *float64
	number       *float64
	usageDetails []zaiUsageDetail
}

// zaiUsageDetail is one `usageDetails[]` entry.
type zaiUsageDetail struct {
	modelCode string
	usage     *float64
}

// normalizeZaiBaseURL reduces a configured base URL to its origin.
func normalizeZaiBaseURL(baseURL string) string {
	trimmed := trimJSWhitespace(baseURL)
	if trimmed == "" {
		return zaiDefaultEndpoint
	}
	return usageOrigin(trimmed, zaiDefaultEndpoint)
}

// zaiParseMillis reads an epoch timestamp that may be seconds or millis.
func zaiParseMillis(value any) *int64 {
	number, ok := usageNumber(value)
	if !ok {
		return nil
	}
	millis := int64(number)
	if number <= 1_000_000_000_000 {
		millis = int64(number * 1000)
	}
	return &millis
}

// parseZaiUsageDetails reads `usageDetails[]`.
func parseZaiUsageDetails(value any) []zaiUsageDetail {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	details := make([]zaiUsageDetail, 0, len(items))
	for _, item := range items {
		record, ok := usageRecord(item)
		if !ok {
			continue
		}
		detail := zaiUsageDetail{}
		detail.modelCode, _ = usageString(record, "modelCode")
		if usage, ok := usageNumber(record["usage"]); ok {
			detail.usage = &usage
		}
		details = append(details, detail)
	}
	if len(details) == 0 {
		return nil
	}
	return details
}

// parseZaiLimitItem reads one quota item.
func parseZaiLimitItem(value any) *zaiUsageLimitItem {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	kind, ok := usageString(record, "type")
	if !ok {
		return nil
	}
	item := &zaiUsageLimitItem{kind: kind, usageDetails: parseZaiUsageDetails(record["usageDetails"])}
	if number, ok := usageNumber(record["usage"]); ok {
		item.usage = &number
	}
	if number, ok := usageNumber(record["currentValue"]); ok {
		item.currentValue = &number
	}
	if number, ok := usageNumber(record["percentage"]); ok {
		item.percentage = &number
	}
	if number, ok := usageNumber(record["remaining"]); ok {
		item.remaining = &number
	}
	item.nextReset = zaiParseMillis(record["nextResetTime"])
	if number, ok := usageNumber(record["unit"]); ok {
		item.unit = &number
	}
	if number, ok := usageNumber(record["number"]); ok {
		item.number = &number
	}
	return item
}

// zaiPercentageFraction clamps a 0..100 percentage into a used fraction.
func zaiPercentageFraction(percentage *float64) *float64 {
	if percentage == nil {
		return nil
	}
	fraction := usageClampFraction(*percentage / 100)
	return &fraction
}

// zaiFormatCountedUnit renders "5 Hours".
func zaiFormatCountedUnit(count int64, singular string) string {
	text := usageFormatCount(count) + " "
	if count != 1 {
		return text + singular + "s"
	}
	return text + singular
}

// buildZaiWindow renders a window from the unit/number fields.
func buildZaiWindow(item *zaiUsageLimitItem) UsageWindow {
	count := int64(1)
	if item.number != nil && *item.number > 0 {
		count = int64(*item.number)
	}
	window := UsageWindow{ResetsAt: item.nextReset}
	switch {
	case item.unit != nil && int(*item.unit) == 3:
		window.ID = usageFormatCount(count) + "h"
		window.Label = zaiFormatCountedUnit(count, "Hour")
		window.DurationMs = new(count * usageHourMs)
	case item.unit != nil && int(*item.unit) == 4:
		window.ID = usageFormatCount(count) + "d"
		window.Label = zaiFormatCountedUnit(count, "Day")
		window.DurationMs = new(count * usageDayMs)
	case item.unit != nil && int(*item.unit) == 5:
		window.ID = usageFormatCount(count) + "mo"
		if count == 1 {
			window.Label = "Monthly"
		} else {
			window.Label = zaiFormatCountedUnit(count, "Month")
		}
		window.DurationMs = new(count * zaiMonthMs)
	case item.unit != nil && int(*item.unit) == 6:
		window.ID = "1w"
		window.Label = "Weekly"
		window.DurationMs = new(usageWeekMs)
	default:
		if item.unit != nil {
			window.ID = usageFormatCount(count) + "u" + usageFormatCount(int64(*item.unit))
		} else {
			window.ID = "quota"
		}
		window.Label = "Quota"
	}
	return window
}

// zaiIsFeatureRequestLimit reports the Zread/web-reader feature quota.
func zaiIsFeatureRequestLimit(item *zaiUsageLimitItem) bool {
	has := map[string]bool{}
	for _, detail := range item.usageDetails {
		if detail.modelCode != "" {
			has[detail.modelCode] = true
		}
	}
	return has["search-prime"] && has["web-reader"] && has["zread"]
}

// zaiRequestQuotaLabel names the request quota row.
func zaiRequestQuotaLabel(item *zaiUsageLimitItem) string {
	if zaiIsFeatureRequestLimit(item) {
		return "ZAI Zread Quota"
	}
	return "ZAI Request Quota"
}

// zaiFormatDate renders `YYYY-MM-DD+HH:mm:ss` in local time.
func zaiFormatDate(year int, month int, day int, hour int, minute int, second int) string {
	return usageFormatCount(int64(year)) + "-" + leftPadTwo(month) + "-" + leftPadTwo(day) + "+" +
		leftPadTwo(hour) + ":" + leftPadTwo(minute) + ":" + leftPadTwo(second)
}

// buildZaiModelUsageURL builds the trailing-week model usage query.
func buildZaiModelUsageURL(baseURL string) string {
	now := timeNow()
	start := now.Add(-7 * 24 * hourDuration)
	startTime := zaiFormatDate(start.Year(), int(start.Month()), start.Day(), start.Hour(), start.Minute(), start.Second())
	endTime := zaiFormatDate(now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	return baseURL + zaiModelUsagePath + "?startTime=" + percentEncode(startTime) + "&endTime=" + percentEncode(endTime)
}

// fetchZaiUsage probes the Z.AI quota endpoint.
func fetchZaiUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "zai" {
		return nil, nil
	}
	credential := params.Credential
	// Sign-in stores the minted key in accessToken and the paste path stores it
	// in apiKey; both go out verbatim, without a Bearer prefix.
	token := credential.APIKey
	if credential.Type == CredentialOAuth {
		token = credential.AccessToken
	}
	if token == "" {
		return nil, nil
	}
	baseURL := normalizeZaiBaseURL(params.BaseURL)
	endpoint := baseURL + zaiQuotaPath
	headers := map[string]string{
		"Authorization": token,
		"Content-Type":  "application/json",
		"User-Agent":    usageUserAgent(),
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "ZAI usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		usageLogWarn(fctx, "ZAI usage fetch failed", map[string]any{"status": response.Status})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil
	}
	success, _ := usageBool(record, "success")
	if !success {
		// An account without a GLM Coding Plan answers `{code: 500, msg: ...}`:
		// nothing to meter, not a failure.
		if message, ok := usageString(record, "msg"); ok &&
			(strings.Contains(message, "不存在") || strings.Contains(strings.ToLower(message), "no coding plan")) {
			usageLogDebug(fctx, "ZAI account has no Coding Plan to meter", nil)
			return nil, nil
		}
		usageLogWarn(fctx, "ZAI usage response invalid", nil)
		return nil, nil
	}
	data, _ := usageRecord(record["data"])
	limits := make([]UsageLimit, 0, 4)
	for _, rawLimit := range usageArrayOrEmpty(data["limits"]) {
		item := parseZaiLimitItem(rawLimit)
		if item == nil {
			continue
		}
		window := buildZaiWindow(item)
		switch item.kind {
		case "TOKENS_LIMIT":
			amount := UsageBuildAmount(item.currentValue, item.usage, item.remaining, zaiPercentageFraction(item.percentage), UsageUnitTokens)
			limits = append(limits, UsageLimit{
				ID:     "zai:tokens:" + window.ID,
				Label:  "ZAI " + window.Label + " Token Quota",
				Scope:  UsageScope{Provider: params.Provider, WindowID: window.ID, Shared: true},
				Window: &window,
				Amount: amount,
				Status: usageStatusFor(amount.UsedFraction),
			})
		case "TIME_LIMIT":
			amount := UsageBuildAmount(item.currentValue, item.usage, item.remaining, zaiPercentageFraction(item.percentage), UsageUnitRequests)
			feature := zaiIsFeatureRequestLimit(item)
			scope := UsageScope{Provider: params.Provider, WindowID: window.ID, Shared: !feature}
			id := "zai:requests:" + window.ID
			if feature {
				id = "zai:features:zread:" + window.ID
				scope.Tier = "zread"
			}
			limits = append(limits, UsageLimit{
				ID:     id,
				Label:  zaiRequestQuotaLabel(item),
				Scope:  scope,
				Window: &window,
				Amount: amount,
				Status: usageStatusFor(amount.UsedFraction),
			})
		case "CREDIT_LIMIT":
			// `percentage` is a server-rounded integer, so prefer the exact ratio.
			hasAbsolute := item.currentValue != nil && item.usage != nil && *item.usage > 0
			var fraction *float64
			if !hasAbsolute {
				fraction = zaiPercentageFraction(item.percentage)
			}
			amount := UsageBuildAmount(item.currentValue, item.usage, item.remaining, fraction, UsageUnitCredits)
			limits = append(limits, UsageLimit{
				ID:     "zai:credits:" + window.ID,
				Label:  "ZAI " + window.Label + " Credit Quota",
				Scope:  UsageScope{Provider: params.Provider, WindowID: window.ID, Shared: true},
				Window: &window,
				Amount: amount,
				Status: usageStatusFor(amount.UsedFraction),
			})
		}
	}
	if len(limits) == 0 {
		return nil, nil
	}
	metadata := map[string]any{"endpoint": endpoint}
	if credential.AccountID != "" {
		metadata["accountId"] = credential.AccountID
	}
	if credential.Email != "" {
		metadata["email"] = credential.Email
	}
	if level, ok := usageString(data, "level"); ok {
		metadata["planType"] = level
	}
	report := &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata, Raw: payload}

	// The model-usage overlay is display-only metadata.
	if response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", buildZaiModelUsageURL(baseURL), headers, nil); err == nil && response.OK() {
		if modelUsage, ok := response.JSON(); ok {
			if _, isRecord := usageRecord(modelUsage); isRecord {
				report.Metadata["modelUsage"] = modelUsage
			}
		}
	}
	return report, nil
}

// zaiUsageProvider returns the Z.AI usage provider.
func zaiUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID:           "zai",
		CacheVersion: 2,
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != "zai" {
				return false
			}
			if params.Credential.Type == CredentialOAuth {
				return params.Credential.AccessToken != ""
			}
			return params.Credential.APIKey != ""
		},
		FetchUsage: fetchZaiUsage,
	}
}

// --- Umans ------------------------------------------------------------------

const (
	umansProvider        = "umans"
	umansDefaultEndpoint = "https://api.code.umans.ai"
	umansUsagePath       = "/v1/usage"
)

// normalizeUmansBaseURL strips a trailing `/v1` but keeps a path prefix.
func normalizeUmansBaseURL(baseURL string) string {
	trimmed := trimJSWhitespace(baseURL)
	if trimmed == "" {
		return umansDefaultEndpoint
	}
	normalized := usageTrimTrailingSlashes(trimmed)
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		normalized = normalized[:len(normalized)-len("/v1")]
	}
	if normalized == "" {
		return umansDefaultEndpoint
	}
	return normalized
}

// umansFiniteNumber reads a finite JSON number.
func umansFiniteNumber(value any) *float64 {
	number, ok := usageNumber(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil
	}
	return &number
}

// umansSoftCapStatus never reports exhausted: hitting the soft cap only means
// burst headroom is being consumed.
func umansSoftCapStatus(fraction *float64) UsageStatus {
	if fraction == nil {
		return ""
	}
	if *fraction >= 0.9 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// umansBuildRequestsLimits renders the request meters.
func umansBuildRequestsLimits(record map[string]any, provider string) []UsageLimit {
	limitsRecord, _ := usageRecord(record["limits"])
	requests, _ := usageRecord(limitsRecord["requests"])
	usageRecordBody, _ := usageRecord(record["usage"])
	limit := umansFiniteNumber(requests["limit"])
	hardCap := umansFiniteNumber(requests["hard_cap"])
	windowSeconds := umansFiniteNumber(requests["window_seconds"])
	rawUsed := umansFiniteNumber(usageRecordBody["requests_in_window"])
	rawRemaining := umansFiniteNumber(usageRecordBody["remaining_requests"])
	weightedUsed := umansFiniteNumber(usageRecordBody["weighted_in_window"])
	weightedRemaining := umansFiniteNumber(usageRecordBody["weighted_remaining_requests"])
	if limit == nil && rawUsed == nil && weightedUsed == nil {
		return nil
	}
	var resetsAt *int64
	if window, ok := usageRecord(record["window"]); ok {
		if text, ok := usageString(window, "resets_at"); ok {
			if millis, ok := usageIsoTimestamp(text); ok {
				resetsAt = &millis
			}
		}
	}
	durationMs := 5 * usageHourMs
	if windowSeconds != nil && *windowSeconds != 0 {
		durationMs = int64(*windowSeconds * 1000)
	}
	window := &UsageWindow{ID: "5h", Label: "rolling 5h", DurationMs: &durationMs, ResetsAt: resetsAt}
	if resetsAt != nil {
		window.ResetLabel = "tick"
	}
	// Single row: payloads without weighted counters or without a burst ceiling.
	if weightedUsed == nil || hardCap == nil {
		used := weightedUsed
		if used == nil {
			used = rawUsed
		}
		remaining := weightedRemaining
		if weightedUsed == nil {
			remaining = rawRemaining
		}
		amount := UsageBuildAmount(used, limit, remaining, nil, UsageUnitRequests)
		return []UsageLimit{{
			ID:     "umans:requests",
			Label:  "Requests (rolling 5h)",
			Scope:  UsageScope{Provider: provider, WindowID: window.ID, Shared: true},
			Window: window,
			Amount: amount,
			Status: usageStatusFor(amount.UsedFraction),
		}}
	}
	softAmount := UsageBuildAmount(weightedUsed, limit, weightedRemaining, nil, UsageUnitRequests)
	limits := []UsageLimit{{
		ID:     "umans:requests:soft",
		Label:  "Requests (soft cap)",
		Scope:  UsageScope{Provider: provider, WindowID: window.ID, Shared: true},
		Window: window,
		Amount: softAmount,
		Status: umansSoftCapStatus(softAmount.UsedFraction),
	}}
	if rawUsed != nil {
		hardAmount := UsageBuildAmount(rawUsed, hardCap, nil, nil, UsageUnitRequests)
		limits = append(limits, UsageLimit{
			ID:     "umans:requests:hard",
			Label:  "Requests (burst ceiling)",
			Scope:  UsageScope{Provider: provider, WindowID: window.ID, Shared: true},
			Window: window,
			Amount: hardAmount,
			Status: usageStatusFor(hardAmount.UsedFraction),
		})
	}
	return limits
}

// umansBuildConcurrencyLimit renders the instantaneous concurrency meter.
func umansBuildConcurrencyLimit(record map[string]any, provider string) *UsageLimit {
	limitsRecord, _ := usageRecord(record["limits"])
	concurrency, _ := usageRecord(limitsRecord["concurrency"])
	usageRecordBody, _ := usageRecord(record["usage"])
	limit := umansFiniteNumber(concurrency["limit"])
	used := umansFiniteNumber(usageRecordBody["concurrent_sessions"])
	if limit == nil && used == nil {
		return nil
	}
	amount := UsageBuildAmount(used, limit, nil, nil, UsageUnitRequests)
	return &UsageLimit{
		ID:     "umans:concurrency",
		Label:  "Concurrency",
		Scope:  UsageScope{Provider: provider, WindowID: "concurrency"},
		Amount: amount,
		Status: usageStatusFor(amount.UsedFraction),
	}
}

// fetchUmansUsage probes Umans' `/v1/usage`.
func fetchUmansUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != umansProvider {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	endpoint := normalizeUmansBaseURL(params.BaseURL) + umansUsagePath
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, map[string]string{
		"authorization": "Bearer " + credential.APIKey,
		"accept":        "application/json",
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Umans usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		// Auth failures must throw so the credential is flagged, not marked unknown.
		if response.Status == 401 || response.Status == 403 {
			return nil, NewUsageHTTPError("Umans usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response), response.Status)
		}
		usageLogWarn(fctx, "Umans usage fetch failed", map[string]any{"status": response.Status})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		usageLogWarn(fctx, "Umans usage response was not a JSON object", nil)
		return nil, nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil
	}
	limits := umansBuildRequestsLimits(record, params.Provider)
	if concurrency := umansBuildConcurrencyLimit(record, params.Provider); concurrency != nil {
		limits = append(limits, *concurrency)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	notes := make([]string, 0, 1)
	if body, ok := usageRecord(record["usage"]); ok {
		if priority, ok := usageRecord(body["priority"]); ok {
			if low, ok := usageBool(priority, "low"); ok && low {
				notes = append(notes, "Requests deprioritized after a rate-limit burst.")
			}
		}
	}
	metadata := map[string]any{"endpoint": endpoint}
	if plan, ok := usageRecord(record["plan"]); ok {
		if name, ok := usageString(plan, "display_name"); ok {
			metadata["plan"] = name
		}
	}
	if credential.AccountID != "" {
		metadata["accountId"] = credential.AccountID
	}
	if credential.Email != "" {
		metadata["email"] = credential.Email
	}
	report := &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata, Raw: record}
	if len(notes) > 0 {
		report.Notes = notes
	}
	return report, nil
}

// umansUsageProvider returns the Umans usage provider.
func umansUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: umansProvider,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == umansProvider && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage:           fetchUmansUsage,
		ValidatesCredentials: true,
	}
}

// --- ClinePass --------------------------------------------------------------

const (
	clinePassBaseURL         = "https://api.cline.bot/api/v1"
	clinePassUsageLimitsPath = "/users/me/plan/usage-limits"
	clinePassAccountPath     = "/users/me"
)

// clinePassClientHeaders mirrors the official Cline CLI's request identity.
func clinePassClientHeaders() map[string]string {
	return map[string]string{
		"HTTP-Referer":       "https://cline.bot",
		"X-Title":            "Cline",
		"X-IS-MULTIROOT":     "false",
		"X-CLIENT-TYPE":      "cline-sdk",
		"User-Agent":         "Cline/3.0.58",
		"X-CLIENT-VERSION":   "3.0.58",
		"X-PLATFORM":         "linux",
		"X-PLATFORM-VERSION": "3.0.54",
		"X-CORE-VERSION":     "0.0.79",
	}
}

// clinePassWindow describes one plan window.
type clinePassWindow struct {
	id       string
	label    string
	duration int64
}

// clinePassWindows maps payload window types.
var clinePassWindows = map[string]clinePassWindow{
	"five_hour": {id: "5h", label: "5 Hour", duration: 5 * usageHourMs},
	"weekly":    {id: "7d", label: "Weekly", duration: usageWeekMs},
	"monthly":   {id: "30d", label: "Monthly", duration: 30 * usageDayMs},
}

// parseClinePassLimit reads one `limits[]` entry.
func parseClinePassLimit(raw any, provider string) *UsageLimit {
	record, ok := usageRecord(raw)
	if !ok {
		return nil
	}
	kind, ok := usageString(record, "type")
	if !ok {
		return nil
	}
	config, ok := clinePassWindows[kind]
	if !ok {
		return nil
	}
	percentUsed, ok := usageNumber(record["percentUsed"])
	if !ok || math.IsNaN(percentUsed) || math.IsInf(percentUsed, 0) {
		return nil
	}
	used := math.Max(0, percentUsed)
	fraction := used / 100
	remaining := math.Max(0, 100-used)
	remainingFraction := math.Max(0, 1-fraction)
	window := &UsageWindow{ID: config.id, Label: config.label, DurationMs: &config.duration}
	if resetsAt, ok := usageIsoTimestamp(record["resetsAt"]); ok {
		window.ResetsAt = &resetsAt
	}
	return &UsageLimit{
		ID:     "cline-pass:" + config.id,
		Label:  "ClinePass",
		Scope:  UsageScope{Provider: provider, WindowID: config.id, Shared: true},
		Window: window,
		Amount: UsageAmount{
			Used:              &used,
			Limit:             new(100.0),
			Remaining:         &remaining,
			UsedFraction:      &fraction,
			RemainingFraction: &remainingFraction,
			Unit:              UsageUnitPercent,
		},
		Status: usageStatusFor(&fraction),
	}
}

// fetchClinePassIdentity reads the account identity for report labels.
func fetchClinePassIdentity(ctx context.Context, fctx UsageFetchContext, endpoint string, headers map[string]string) (string, string) {
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil || !response.OK() {
		return "", ""
	}
	payload, ok := response.JSON()
	if !ok {
		return "", ""
	}
	record, ok := usageRecord(payload)
	if !ok {
		return "", ""
	}
	if data, ok := usageRecord(record["data"]); ok {
		record = data
	}
	email, _ := usageString(record, "email")
	accountID, _ := usageString(record, "id")
	return email, accountID
}

// fetchClinePassUsage probes the ClinePass plan quota route.
func fetchClinePassUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "cline-pass" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	baseURL := usageTrimTrailingSlashes(params.BaseURL)
	if baseURL == "" {
		baseURL = clinePassBaseURL
	}
	endpoint := baseURL + clinePassUsageLimitsPath
	headers := clinePassClientHeaders()
	headers["Accept"] = "application/json"
	headers["Authorization"] = "Bearer " + credential.APIKey
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "ClinePass usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		if response.Status == 401 || response.Status == 403 {
			return nil, NewUsageHTTPError("ClinePass usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response), response.Status)
		}
		// An account that never subscribed answers 404 "no plan history found":
		// nothing to meter, not a failure.
		if response.Status == 404 && strings.Contains(strings.ToLower(response.Text()), "no plan history") {
			usageLogDebug(fctx, "ClinePass account has no plan to meter", nil)
			return nil, nil
		}
		usageLogWarn(fctx, "ClinePass usage fetch failed", map[string]any{"status": response.Status})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil
	}
	if data, ok := usageRecord(record["data"]); ok {
		record = data
	}
	items, ok := record["limits"].([]any)
	if !ok {
		return nil, nil
	}
	limits := make([]UsageLimit, 0, len(items))
	for _, item := range items {
		if limit := parseClinePassLimit(item, params.Provider); limit != nil {
			limits = append(limits, *limit)
		}
	}
	if len(limits) == 0 {
		return nil, nil
	}
	email := credential.Email
	accountID := credential.AccountID
	if email == "" || accountID == "" {
		resolvedEmail, resolvedAccount := fetchClinePassIdentity(ctx, fctx, baseURL+clinePassAccountPath, headers)
		if email == "" {
			email = resolvedEmail
		}
		if accountID == "" {
			accountID = resolvedAccount
		}
	}
	metadata := map[string]any{"endpoint": endpoint}
	if email != "" {
		metadata["email"] = email
	}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata, Raw: payload}, nil
}

// clinePassUsageProvider returns the ClinePass usage provider.
func clinePassUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "cline-pass",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "cline-pass" && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage:           fetchClinePassUsage,
		ValidatesCredentials: true,
	}
}

// --- Alibaba Token Plan -----------------------------------------------------

const (
	usageAlibabaProvider     = "alibaba-token-plan"
	alibabaTokenPlanUsageAPI = "zeldaHttp.apikeyMgr./tokenplan/personal/api/v2/usage"
	alibabaBrowserUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
)

// alibabaConsole describes one Token Plan console region.
type alibabaConsole struct {
	origin        string
	dashboardURL  string
	sessionURL    string
	gatewayAction string
	region        string
	usageURL      string
	cornerstone   map[string]any
	isChina       bool
}

// alibabaInternationalConsole is the Singapore console.
func alibabaInternationalConsole() alibabaConsole {
	return alibabaConsole{
		origin:        "https://home.qwencloud.com",
		dashboardURL:  "https://home.qwencloud.com/billing/subscription/token-plan-individual",
		sessionURL:    "https://home.qwencloud.com/tool/user/info.json",
		gatewayAction: "IntlBroadScopeAspnGateway",
		region:        "ap-southeast-1",
		usageURL:      "https://cs-data.qwencloud.com/data/api.json?product=sfm_bailian&action=IntlBroadScopeAspnGateway&api=" + percentEncode(alibabaTokenPlanUsageAPI),
		cornerstone: map[string]any{
			"domain":      "home.qwencloud.com",
			"consoleSite": "QWENCLOUD",
			"console":     "ONE_CONSOLE",
			"xsp_lang":    "en-US",
			"protocol":    "V2",
			"productCode": "p_efm",
		},
	}
}

// alibabaChinaConsole is the Beijing console.
func alibabaChinaConsole() alibabaConsole {
	return alibabaConsole{
		origin:        "https://bailian.console.aliyun.com",
		dashboardURL:  "https://bailian.console.aliyun.com/cn-beijing?tab=plan",
		sessionURL:    "https://bailian.console.aliyun.com/cn-beijing?tab=plan",
		gatewayAction: "BroadScopeAspnGateway",
		region:        "cn-beijing",
		usageURL:      "https://bailian-cs.console.aliyun.com/data/api.json?action=BroadScopeAspnGateway&product=sfm_bailian&api=" + percentEncode(alibabaTokenPlanUsageAPI),
		isChina:       true,
		cornerstone: map[string]any{
			"feURL":             "https://bailian.console.aliyun.com/cn-beijing?tab=plan#/efm/subscription/token-plan/personal",
			"protocol":          "V2",
			"console":           "ONE_CONSOLE",
			"productCode":       "p_efm",
			"switchUserType":    3,
			"domain":            "bailian.console.aliyun.com",
			"consoleSite":       "BAILIAN_ALIYUN",
			"userNickName":      "",
			"userPrincipalName": "",
			"xsp_lang":          "zh-CN",
		},
	}
}

// alibabaSEC_TOKEN reads the China console's embedded SEC_TOKEN literal.
var usageAlibabaSEC_TOKEN = regexpMustCompile(`\bSEC_TOKEN\s*:\s*"([^"]+)"`)

// extractAlibabaCookieValue reads one cookie from a Cookie header.
func extractAlibabaCookieValue(header, name string) string {
	for _, segment := range strings.Split(header, ";") {
		index := strings.Index(segment, "=")
		if index < 0 || trimJSWhitespace(segment[:index]) != name {
			continue
		}
		value := trimJSWhitespace(segment[index+1:])
		if value != "" {
			return value
		}
	}
	return ""
}

// unwrapAlibabaGatewayData peels the gateway envelope.
func unwrapAlibabaGatewayData(record map[string]any) map[string]any {
	current := record
	if data, ok := current["Data"].(string); ok {
		if parsed, ok := usageRecordJSON(data); ok {
			current = parsed
		} else {
			return current
		}
	}
	if dataV2, ok := usageRecord(current["DataV2"]); ok {
		if inner, ok := usageRecord(dataV2["data"]); ok {
			current = inner
		}
	}
	if inner, ok := usageRecord(current["data"]); ok {
		current = inner
	}
	return current
}

// parseAlibabaUsedFraction reads a 0..1 or 0..100 used fraction.
func parseAlibabaUsedFraction(value any) *float64 {
	parsed, ok := usageNumber(value)
	if !ok || parsed < 0 {
		return nil
	}
	if parsed > 1 {
		parsed /= 100
	}
	fraction := math.Min(1, parsed)
	return &fraction
}

// alibabaUsageStatus maps a used fraction to a status (warning at 80%).
func alibabaUsageStatus(fraction float64) UsageStatus {
	if fraction >= 1 {
		return UsageStatusExhausted
	}
	if fraction >= 0.8 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// buildAlibabaLimit renders one credits window.
func buildAlibabaLimit(id, label string, durationMs *int64, fraction *float64, resetsAt *int64, accountID string) *UsageLimit {
	if fraction == nil {
		return nil
	}
	used := *fraction * 100
	window := &UsageWindow{ID: id, Label: label, DurationMs: durationMs, ResetsAt: resetsAt}
	return &UsageLimit{
		ID:     "credits:" + id,
		Label:  label,
		Scope:  UsageScope{Provider: usageAlibabaProvider, AccountID: accountID, WindowID: id},
		Window: window,
		Amount: UsageAmount{Used: &used, UsedFraction: fraction, Unit: UsageUnitPercent},
		Status: alibabaUsageStatus(*fraction),
	}
}

// alibabaAccountIDFromUserData reads an identity from the session payload.
func alibabaAccountIDFromUserData(record map[string]any) string {
	for _, key := range []string{"accountId", "userId", "aliyunId", "loginId"} {
		if value, ok := usageString(record, key); ok {
			return value
		}
		if number, ok := usageNumber(record[key]); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
			return usageFormatFloat(number)
		}
	}
	return ""
}

// fetchAlibabaTokenPlanUsage reads the console usage gateway.
func fetchAlibabaTokenPlanUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != usageAlibabaProvider || params.Credential.Type != CredentialAPIKey || params.Credential.APIKey == "" {
		return nil, nil
	}
	// The shared pi-catalog parser owns the token/cookie/baseUrl envelope,
	// including its trimming and explicit-value type checks.
	credential, ok := parseAlibabaTokenPlanCredential(params.Credential.APIKey)
	if !ok || credential.Cookie == "" {
		return nil, nil
	}
	console := alibabaInternationalConsole()
	if credential.BaseURL == alibabaTokenPlanCNBaseURL {
		console = alibabaChinaConsole()
	}
	cookie := credential.Cookie

	accept := "application/json, text/plain, */*"
	if console.isChina {
		accept = "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8"
	}
	sessionResponse, err := usageDo(ctx, fctx, usageManualRedirects, "GET", console.sessionURL, map[string]string{
		"Accept":     accept,
		"Cookie":     cookie,
		"Referer":    console.origin + "/",
		"User-Agent": alibabaBrowserUserAgent,
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Alibaba Token Plan usage request failed", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	if !sessionResponse.OK() {
		usageLogWarn(fctx, "Alibaba Token Plan session lookup failed", map[string]any{"provider": params.Provider, "status": sessionResponse.Status})
		return nil, nil
	}
	secToken := ""
	accountID := ""
	if console.isChina {
		match := usageAlibabaSEC_TOKEN.FindStringSubmatch(sessionResponse.Text())
		if len(match) < 2 {
			usageLogWarn(fctx, "Alibaba Token Plan China session response invalid", map[string]any{"provider": params.Provider})
			return nil, nil
		}
		secToken = match[1]
	} else {
		payload, ok := sessionResponse.JSON()
		record, isRecord := usageRecord(payload)
		if !ok || !isRecord {
			usageLogWarn(fctx, "QwenCloud session response invalid", map[string]any{"provider": params.Provider})
			return nil, nil
		}
		data, ok := usageRecord(record["data"])
		if !ok {
			usageLogWarn(fctx, "QwenCloud session response invalid", map[string]any{"provider": params.Provider})
			return nil, nil
		}
		token, ok := usageString(data, "secToken")
		if !ok {
			usageLogWarn(fctx, "QwenCloud session response invalid", map[string]any{"provider": params.Provider})
			return nil, nil
		}
		secToken = token
		accountID = alibabaAccountIDFromUserData(data)
	}

	csrf := extractAlibabaCookieValue(cookie, "login_aliyunid_csrf")
	if csrf == "" {
		csrf = extractAlibabaCookieValue(cookie, "csrf")
	}
	headers := map[string]string{
		"Accept":           "application/json, text/plain, */*",
		"Content-Type":     "application/x-www-form-urlencoded",
		"Cookie":           cookie,
		"Origin":           console.origin,
		"Referer":          console.dashboardURL,
		"User-Agent":       alibabaBrowserUserAgent,
		"X-Requested-With": "XMLHttpRequest",
	}
	if csrf != "" {
		headers["x-xsrf-token"] = csrf
		headers["x-csrf-token"] = csrf
	}
	cornerstone := map[string]any{}
	if console.isChina {
		cornerstone["feTraceId"] = randomUUIDString()
	}
	for key, value := range console.cornerstone {
		cornerstone[key] = value
	}
	paramsJSON := usageJSONObject(map[string]any{
		"Api": alibabaTokenPlanUsageAPI,
		"Data": map[string]any{
			"cornerstoneParam": cornerstone,
		},
		"V": "1.0",
	})
	form := urlValues(map[string]string{
		"product":   "sfm_bailian",
		"action":    console.gatewayAction,
		"region":    console.region,
		"sec_token": secToken,
		"params":    paramsJSON,
	})
	usageResponse, err := usageDo(ctx, fctx, usageManualRedirects, "POST", console.usageURL, headers, []byte(form))
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Alibaba Token Plan usage request failed", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	if !usageResponse.OK() {
		usageLogWarn(fctx, "Alibaba Token Plan usage fetch failed", map[string]any{"provider": params.Provider, "status": usageResponse.Status})
		return nil, nil
	}
	payload, ok := usageResponse.JSON()
	record, isRecord := usageRecord(payload)
	if !ok || !isRecord {
		usageLogWarn(fctx, "Alibaba Token Plan usage response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	if success, ok := usageBool(record, "successResponse"); ok && !success {
		usageLogWarn(fctx, "Alibaba Token Plan usage response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	data, ok := usageRecord(record["data"])
	if !ok {
		usageLogWarn(fctx, "Alibaba Token Plan usage response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	if success, ok := usageBool(data, "success"); ok && !success {
		usageLogWarn(fctx, "Alibaba Token Plan usage request rejected", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	responseData := unwrapAlibabaGatewayData(data)
	limits := make([]UsageLimit, 0, 3)
	if limit := buildAlibabaLimit("5h", "5 Hour Credits", new(5*usageHourMs), parseAlibabaUsedFraction(responseData["per5HourPercentage"]), alibabaResetTime(responseData["per5HourResetTime"]), accountID); limit != nil {
		limits = append(limits, *limit)
	}
	if limit := buildAlibabaLimit("7d", "7 Day Credits", new(usageWeekMs), parseAlibabaUsedFraction(responseData["per1WeekPercentage"]), alibabaResetTime(responseData["per1WeekResetTime"]), accountID); limit != nil {
		limits = append(limits, *limit)
	}
	// Monthly-only plans report just this bucket; the console never states its
	// span (calendar months differ), so no duration.
	if limit := buildAlibabaLimit("monthly", "Monthly Credits", nil, parseAlibabaUsedFraction(responseData["per1MonthPercentage"]), alibabaResetTime(responseData["per1MonthResetTime"]), accountID); limit != nil {
		limits = append(limits, *limit)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	source := "qwencloud-console"
	if console.isChina {
		source = "bailian-console"
	}
	metadata := map[string]any{"source": source}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	return &UsageReport{Provider: usageAlibabaProvider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata}, nil
}

// alibabaResetTime reads a positive epoch reset.
func alibabaResetTime(value any) *int64 {
	if timestamp, ok := usagePositiveTimestamp(value); ok {
		return &timestamp
	}
	return nil
}

// alibabaTokenPlanUsageProvider returns the Alibaba Token Plan usage provider.
func usageAlibabaProviderImpl() *UsageProvider {
	return &UsageProvider{
		ID: usageAlibabaProvider,
		// Token-plan percentages come from a console scrape; a stale report must
		// not mask a failed refresh.
		RetainLastGoodOnFailure: new(false),
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != usageAlibabaProvider || params.Credential.Type != CredentialAPIKey || params.Credential.APIKey == "" {
				return false
			}
			credential, ok := parseAlibabaTokenPlanCredential(params.Credential.APIKey)
			return ok && credential.Cookie != ""
		},
		FetchUsage: fetchAlibabaTokenPlanUsage,
	}
}
