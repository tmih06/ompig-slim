package ai

// Ports packages/ai/src/usage/kimi.ts, usage/minimax-code.ts and
// usage/muse-code.ts.

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/internal/configroot"
)

// --- Kimi Code --------------------------------------------------------------

const (
	kimiDefaultBaseURL = "https://api.kimi.com/coding/v1"
	kimiUsagePath      = "usages"
)

// kimiUsageRow is one normalized usage row before it becomes a limit.
type kimiUsageRow struct {
	label     string
	unit      UsageUnit
	used      *float64
	limit     *float64
	remaining *float64
	resetsAt  *int64
	window    *UsageWindow
}

// normalizeKimiBaseURL resolves the base URL from the override or env.
func normalizeKimiBaseURL(baseURL string) string {
	candidate := trimJSWhitespace(baseURL)
	if candidate == "" {
		candidate = trimJSWhitespace(os.Getenv("KIMI_CODE_BASE_URL"))
	}
	if candidate == "" {
		candidate = kimiDefaultBaseURL
	}
	return usageTrimTrailingSlashes(candidate)
}

// kimiResetTime reads an absolute or relative reset from a payload object.
func kimiResetTime(record map[string]any, nowMs int64) *int64 {
	for _, key := range []string{"reset_at", "resetAt", "reset_time", "resetTime"} {
		if value, present := record[key]; present {
			if text, ok := value.(string); ok && trimJSWhitespace(text) != "" {
				if parsed, ok := usageIsoTimestamp(text); ok {
					return &parsed
				}
			}
			if number, ok := usageNumber(value); ok {
				millis := int64(number)
				if number <= 1_000_000_000_000 {
					millis = int64(number * 1000)
				}
				return &millis
			}
		}
	}
	for _, key := range []string{"reset_in", "resetIn", "ttl", "window"} {
		if seconds, ok := usageNumber(record[key]); ok {
			value := nowMs + int64(seconds*1000)
			return &value
		}
	}
	return nil
}

// kimiDurationLabel renders a duration label from a count and unit word.
func kimiDurationLabel(duration float64, timeUnit string) string {
	upper := strings.ToUpper(timeUnit)
	switch {
	case strings.Contains(upper, "MINUTE"):
		if duration >= 60 && math.Mod(duration, 60) == 0 {
			return usageFormatFloat(duration/60) + "h limit"
		}
		return usageFormatFloat(duration) + "m limit"
	case strings.Contains(upper, "HOUR"):
		return usageFormatFloat(duration) + "h limit"
	case strings.Contains(upper, "DAY"):
		return usageFormatFloat(duration) + "d limit"
	case strings.Contains(upper, "SECOND"):
		return usageFormatFloat(duration) + "s limit"
	}
	return ""
}

// kimiCanonicalWindowID derives a canonical window id from a duration.
func kimiCanonicalWindowID(durationMs int64) string {
	if durationMs > 0 && durationMs%usageDayMs == 0 {
		return usageFormatCount(durationMs/usageDayMs) + "d"
	}
	if durationMs > 0 && durationMs%usageHourMs == 0 {
		return usageFormatCount(durationMs/usageHourMs) + "h"
	}
	minutes := int64(math.Round(float64(durationMs) / 60_000))
	if minutes > 0 {
		return usageFormatCount(minutes) + "m"
	}
	return "default"
}

// kimiBuildWindow renders a window object.
func kimiBuildWindow(windowData map[string]any, nowMs int64) *UsageWindow {
	duration, hasDuration := usageNumber(windowData["duration"])
	timeUnit, _ := windowData["timeUnit"].(string)
	label := ""
	if hasDuration && timeUnit != "" {
		label = kimiDurationLabel(duration, timeUnit)
	}
	resetsAt := kimiResetTime(windowData, nowMs)
	if !hasDuration && label == "" && resetsAt == nil {
		return nil
	}
	var durationMs *int64
	if hasDuration {
		upper := strings.ToUpper(timeUnit)
		var millis int64
		switch {
		case strings.Contains(upper, "MINUTE"):
			millis = int64(duration) * 60_000
		case strings.Contains(upper, "HOUR"):
			millis = int64(duration) * usageHourMs
		case strings.Contains(upper, "DAY"):
			millis = int64(duration) * usageDayMs
		case strings.Contains(upper, "WEEK"):
			millis = int64(duration) * 7 * usageDayMs
		case strings.Contains(upper, "SECOND"):
			millis = int64(duration) * 1000
		}
		if millis != 0 {
			durationMs = &millis
		}
	}
	id := "default"
	if durationMs != nil {
		id = kimiCanonicalWindowID(*durationMs)
	}
	if label == "" {
		label = "Usage window"
	}
	return &UsageWindow{ID: id, Label: label, DurationMs: durationMs, ResetsAt: resetsAt}
}

// kimiBuildUsageRow reads a quota object.
func kimiBuildUsageRow(record map[string]any, defaultLabel string, nowMs int64) *kimiUsageRow {
	limit, hasLimit := usageNumber(record["limit"])
	used, hasUsed := usageNumber(record["used"])
	remaining, hasRemaining := usageNumber(record["remaining"])
	if !hasUsed && hasRemaining && hasLimit {
		value := limit - remaining
		used = value
		hasUsed = true
	}
	if !hasUsed && !hasLimit {
		return nil
	}
	label := defaultLabel
	if name, ok := usageString(record, "name"); ok {
		label = name
	} else if title, ok := usageString(record, "title"); ok {
		label = title
	}
	row := &kimiUsageRow{label: label, unit: UsageUnitUnknown, resetsAt: kimiResetTime(record, nowMs)}
	if hasUsed {
		row.used = &used
	}
	if hasLimit {
		row.limit = &limit
	}
	if hasRemaining {
		row.remaining = &remaining
	}
	return row
}

// kimiBuildAggregateRow reads one `usages[key]` aggregate.
func kimiBuildAggregateRow(key string, record map[string]any, nowMs int64) *kimiUsageRow {
	usedRatio, ok := usageNumber(record["used_ratio"])
	if !ok {
		return nil
	}
	fraction := usageClampFraction(usedRatio)
	label := key
	switch key {
	case "limit_month_total":
		label = "Monthly total"
	case "limit_month_code":
		label = "Monthly code"
	}
	used := fraction * 100
	limit := 100.0
	remaining := (1 - fraction) * 100
	resetsAt := kimiResetTime(record, nowMs)
	return &kimiUsageRow{
		label:     label,
		unit:      UsageUnitPercent,
		used:      &used,
		limit:     &limit,
		remaining: &remaining,
		resetsAt:  resetsAt,
		window:    &UsageWindow{ID: key, Label: label, ResetsAt: resetsAt},
	}
}

// kimiUsageAmount renders a row's amount.
func kimiUsageAmount(row kimiUsageRow) UsageAmount {
	amount := UsageAmount{Unit: row.unit}
	if row.limit != nil {
		amount.Limit = row.limit
	}
	if row.used != nil {
		amount.Used = row.used
	}
	if row.remaining != nil {
		amount.Remaining = row.remaining
	}
	if row.limit != nil && row.used != nil && *row.limit > 0 {
		fraction := usageClampFraction(*row.used / *row.limit)
		remainingFraction := usageClampFraction((*row.limit - *row.used) / *row.limit)
		amount.UsedFraction = &fraction
		amount.RemainingFraction = &remainingFraction
		if amount.Remaining == nil {
			value := *row.limit - *row.used
			amount.Remaining = &value
		}
	}
	return amount
}

// kimiToUsageLimit converts a row into a limit.
func kimiToUsageLimit(row kimiUsageRow, provider string, index int, accountID string) UsageLimit {
	// Kimi carries resetTime on the limit detail, not the window, so fall back
	// to the row-level reset.
	window := row.window
	if window != nil {
		if window.ResetsAt == nil && row.resetsAt != nil {
			window = &UsageWindow{ID: window.ID, Label: window.Label, DurationMs: window.DurationMs, ResetsAt: row.resetsAt, ResetLabel: window.ResetLabel}
		}
	} else if row.resetsAt != nil {
		window = &UsageWindow{ID: "default", Label: "Usage window", ResetsAt: row.resetsAt}
	}
	amount := kimiUsageAmount(row)
	limit := UsageLimit{
		ID:     provider + ":" + usageFormatCount(int64(index)),
		Label:  row.label,
		Scope:  UsageScope{Provider: provider, AccountID: accountID, Shared: true},
		Window: window,
		Amount: amount,
		Status: usageStatusFor(amount.UsedFraction),
	}
	if window != nil {
		limit.Scope.WindowID = window.ID
	}
	return limit
}

// kimiParsePayload reads the usages response into rows.
func kimiParsePayload(payload any, nowMs int64) ([]kimiUsageRow, any, bool) {
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil, false
	}
	rows := make([]kimiUsageRow, 0, 4)
	if usage, ok := usageRecord(record["usage"]); ok {
		if summary := kimiBuildUsageRow(usage, "Weekly limit", nowMs); summary != nil {
			// Kimi Code's aggregate quota resets weekly, but the payload carries
			// only `resetTime` and no duration.
			summary.window = &UsageWindow{ID: "7d", Label: "7 Day", ResetsAt: summary.resetsAt}
			rows = append(rows, *summary)
		}
	}
	if totalQuota, ok := usageRecord(record["totalQuota"]); ok {
		windowData, _ := usageRecord(totalQuota["window"])
		if total := kimiBuildUsageRow(totalQuota, "Total quota", nowMs); total != nil {
			total.window = kimiBuildWindow(windowData, nowMs)
			rows = append(rows, *total)
		}
	}
	for index, item := range usageArrayOrEmpty(record["limits"]) {
		itemRecord, ok := usageRecord(item)
		if !ok {
			continue
		}
		detail := itemRecord
		if nested, ok := usageRecord(itemRecord["detail"]); ok {
			detail = nested
		}
		windowData, _ := usageRecord(itemRecord["window"])
		label := ""
		for _, key := range []string{"name", "title", "scope"} {
			if value, ok := usageString(itemRecord, key); ok {
				label = value
				break
			}
		}
		if label == "" {
			for _, key := range []string{"name", "title"} {
				if value, ok := usageString(detail, key); ok {
					label = value
					break
				}
			}
		}
		if label == "" {
			duration, _ := usageNumber(windowData["duration"])
			unit, _ := windowData["timeUnit"].(string)
			label = kimiDurationLabel(duration, unit)
		}
		if label == "" {
			label = "Limit #" + usageFormatCount(int64(index+1))
		}
		if row := kimiBuildUsageRow(detail, label, nowMs); row != nil {
			row.window = kimiBuildWindow(windowData, nowMs)
			rows = append(rows, *row)
		}
	}
	if usages, ok := usageRecord(record["usages"]); ok {
		for key, aggregate := range usages {
			if key == "limit_5h" {
				continue
			}
			aggregateRecord, ok := usageRecord(aggregate)
			if !ok {
				continue
			}
			if row := kimiBuildAggregateRow(key, aggregateRecord, nowMs); row != nil {
				rows = append(rows, *row)
			}
		}
	}
	return rows, record, true
}

// fetchKimiUsage probes the Kimi Code usage endpoint.
func fetchKimiUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "kimi-code" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialOAuth {
		return nil, nil
	}
	accessToken := credential.AccessToken
	if accessToken == "" {
		return nil, nil
	}
	nowMs := timeNowMillis()
	// AuthStorage refreshes OAuth pre-emptively; a probe landing with an expired
	// token short-circuits instead of posting the broker sentinel back.
	if credential.ExpiresAt != nil && *credential.ExpiresAt <= nowMs {
		usageLogDebug(fctx, "Kimi usage token expired; skipping probe", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	endpoint := usageAbsoluteURL(normalizeKimiBaseURL(params.BaseURL), kimiUsagePath)
	headers := map[string]string{"Authorization": "Bearer " + accessToken}
	for name, value := range kimiCommonHeaders() {
		headers[name] = value
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Kimi usage request error", map[string]any{"provider": params.Provider, "error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		usageLogWarn(fctx, "Kimi usage request failed", map[string]any{"status": response.Status, "provider": params.Provider})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		usageLogWarn(fctx, "Kimi usage response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	rows, raw, ok := kimiParsePayload(payload, nowMs)
	if !ok || len(rows) == 0 {
		if record, ok := usageRecord(payload); ok && len(record) == 0 {
			usageLogDebug(fctx, "Kimi account has no Kimi Code plan to meter", map[string]any{"provider": params.Provider})
			return nil, nil
		}
		usageLogWarn(fctx, "Kimi usage response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	limits := make([]UsageLimit, 0, len(rows))
	for index, row := range rows {
		limits = append(limits, kimiToUsageLimit(row, params.Provider, index, credential.AccountID))
	}
	metadata := map[string]any{"endpoint": endpoint}
	if credential.AccountID != "" {
		metadata["accountId"] = credential.AccountID
	}
	return &UsageReport{
		Provider:  params.Provider,
		FetchedAt: nowMs,
		Limits:    limits,
		Metadata:  metadata,
		Raw:       raw,
	}, nil
}

// kimiUsageProvider returns the Kimi Code usage provider.
func kimiUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "kimi-code",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "kimi-code" && params.Credential.Type == CredentialOAuth
		},
		FetchUsage: fetchKimiUsage,
	}
}

// kimiCommonHeaders builds the Kimi CLI identity headers. The device id is
// best-effort: an unwritable agent dir must never break header construction.
func kimiCommonHeaders() map[string]string {
	version := ProductVersion
	deviceName := sanitizeHeaderValue(nodeHostname(), "unknown")
	return map[string]string{
		"User-Agent":         "KimiCLI/" + version,
		"X-Msh-Platform":     "kimi_cli",
		"X-Msh-Version":      version,
		"X-Msh-Device-Name":  deviceName,
		"X-Msh-Device-Model": sanitizeHeaderValue(deviceModelName(), "unknown"),
		"X-Msh-Os-Version":   sanitizeHeaderValue(platformRelease(), "unknown"),
		"X-Msh-Device-Id":    sanitizeHeaderValue(kimiDeviceID(), "unknown"),
	}
}

// sanitizeHeaderValue strips non-printable ASCII from a header value.
func sanitizeHeaderValue(value, fallback string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 0x20 && r < 0x7f {
			builder.WriteRune(r)
		}
	}
	sanitized := trimJSWhitespace(builder.String())
	if sanitized == "" {
		return fallback
	}
	return sanitized
}

// nodeHostname is os.hostname(), or "" when unavailable.
func nodeHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// deviceModelName renders the platform release/arch triple Kimi reports.
func deviceModelName() string {
	platform := runtime.GOOS
	switch platform {
	case "darwin":
		platform = "macOS"
	case "windows":
		platform = "Windows"
	case "linux":
		platform = "Linux"
	}
	return strings.Join([]string{platform, platformRelease(), runtime.GOARCH}, " ")
}

// platformRelease is os.release() for the running kernel.
func platformRelease() string {
	if release := readKernelRelease(); release != "" {
		return release
	}
	return runtime.GOOS
}

// kimiDeviceID is the persisted Kimi device id, or a per-process id.
var kimiDeviceID = sync.OnceValue(func() string {
	root, err := configroot.Resolve()
	if err == nil && root != "" {
		path := filepath.Join(root, "kimi-device-id")
		if data, readErr := os.ReadFile(path); readErr == nil {
			if trimmed := trimJSWhitespace(string(data)); trimmed != "" {
				return strings.ReplaceAll(trimmed, "-", "")
			}
		}
		id := strings.ReplaceAll(randomUUIDString(), "-", "")
		if writeErr := os.MkdirAll(root, 0o755); writeErr == nil {
			_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
		}
		return id
	}
	return strings.ReplaceAll(randomUUIDString(), "-", "")
})

// --- MiniMax Token Plan -----------------------------------------------------

const (
	minimaxCodeBaseURL     = "https://api.minimax.io"
	minimaxRemainsPath     = "/v1/token_plan/remains"
	minimaxStatusExhausted = 2
	minimaxStatusUnlimited = 3
	minimaxSharedBucket    = "general"
)

// minimaxTokenPlanBucket is one `model_remains[]` entry.
type minimaxTokenPlanBucket struct {
	modelName            string
	intervalStart        *int64
	intervalEnd          *int64
	intervalRemainingPct *float64
	intervalTotalCount   *float64
	intervalUsageCount   *float64
	intervalStatus       *float64
	weeklyStart          *int64
	weeklyEnd            *int64
	weeklyRemainingPct   *float64
	weeklyTotalCount     *float64
	weeklyUsageCount     *float64
	weeklyStatus         *float64
}

// minimaxUsedFractionFromRemaining converts a 0..100 remaining percent.
func minimaxUsedFractionFromRemaining(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	fraction := math.Min(1, math.Max(0, (100-*value)/100))
	return &fraction
}

// parseMinimaxBucket reads one bucket.
func parseMinimaxBucket(value any) *minimaxTokenPlanBucket {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	name, ok := usageString(record, "model_name")
	if !ok {
		return nil
	}
	bucket := &minimaxTokenPlanBucket{modelName: name}
	readTimestamp := func(key string) *int64 {
		if timestamp, ok := usagePositiveTimestamp(record[key]); ok {
			return &timestamp
		}
		return nil
	}
	readNumber := func(key string) *float64 {
		if number, ok := usageNumber(record[key]); ok {
			return &number
		}
		return nil
	}
	bucket.intervalStart = readTimestamp("start_time")
	bucket.intervalEnd = readTimestamp("end_time")
	bucket.intervalRemainingPct = readNumber("current_interval_remaining_percent")
	bucket.intervalTotalCount = readNumber("current_interval_total_count")
	bucket.intervalUsageCount = readNumber("current_interval_usage_count")
	bucket.intervalStatus = readNumber("current_interval_status")
	bucket.weeklyStart = readTimestamp("weekly_start_time")
	bucket.weeklyEnd = readTimestamp("weekly_end_time")
	bucket.weeklyRemainingPct = readNumber("current_weekly_remaining_percent")
	bucket.weeklyTotalCount = readNumber("current_weekly_total_count")
	bucket.weeklyUsageCount = readNumber("current_weekly_usage_count")
	bucket.weeklyStatus = readNumber("current_weekly_status")
	return bucket
}

// minimaxUnavailablePlan reports a model outside the current plan, which the
// backend reports as both windows unlimited with zero totals.
func minimaxUnavailablePlan(bucket *minimaxTokenPlanBucket) bool {
	zero := func(value *float64) bool { return value != nil && *value == 0 }
	status := func(value *float64) bool { return value != nil && int(*value) == minimaxStatusUnlimited }
	return zero(bucket.intervalTotalCount) && zero(bucket.weeklyTotalCount) &&
		status(bucket.intervalStatus) && status(bucket.weeklyStatus)
}

// minimaxIntervalWindow derives a window id/label from the reported span.
func minimaxIntervalWindow(durationMs *int64) (string, string) {
	if durationMs == nil || *durationMs <= 0 {
		return "interval", "Interval"
	}
	if *durationMs%usageHourMs == 0 {
		hours := *durationMs / usageHourMs
		return usageFormatCount(hours) + "h", usageFormatCount(hours) + " Hour"
	}
	minutes := int64(math.Round(float64(*durationMs) / 60_000))
	if minutes <= 0 {
		return "interval", "Interval"
	}
	return usageFormatCount(minutes) + "m", usageFormatCount(minutes) + " Minute"
}

// minimaxBuildLimit renders one bucket window.
func minimaxBuildLimit(provider string, bucket *minimaxTokenPlanBucket, windowID, windowLabel string, durationMs, resetsAt *int64, usedFraction *float64, windowStatus, usageCount, totalCount *float64, accountID string) *UsageLimit {
	// The endpoint's own status outranks the percentage.
	if windowStatus != nil && int(*windowStatus) == minimaxStatusExhausted {
		usedFraction = new(1.0)
	}
	if usedFraction == nil {
		return nil
	}
	scope := UsageScope{Provider: provider, AccountID: accountID, WindowID: windowID}
	if bucket.modelName == minimaxSharedBucket {
		scope.Shared = true
	} else {
		scope.ModelID = bucket.modelName
	}
	label := strings.ToUpper(bucket.modelName[:1]) + bucket.modelName[1:] + " " + windowLabel
	used := *usedFraction * 100
	remaining := 100 - used
	remainingFraction := 1 - *usedFraction
	amount := UsageAmount{
		Used:              &used,
		UsedFraction:      usedFraction,
		Remaining:         &remaining,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitPercent,
	}
	limit := &UsageLimit{
		ID:     bucket.modelName + ":" + windowID,
		Label:  label,
		Scope:  scope,
		Window: &UsageWindow{ID: windowID, Label: windowLabel, DurationMs: durationMs, ResetsAt: resetsAt},
		Amount: amount,
		Status: usageStatusFor(usedFraction),
	}
	if totalCount != nil && *totalCount > 0 {
		usedCount := 0.0
		if usageCount != nil {
			usedCount = *usageCount
		}
		limit.Notes = []string{"Requests: " + usageFormatFloat(usedCount) + "/" + usageFormatFloat(*totalCount)}
	}
	return limit
}

// minimaxBuildBucketLimits renders both windows of one bucket.
func minimaxBuildBucketLimits(provider string, bucket *minimaxTokenPlanBucket, accountID string) []UsageLimit {
	var intervalDuration, weeklyDuration *int64
	if bucket.intervalStart != nil && bucket.intervalEnd != nil {
		value := *bucket.intervalEnd - *bucket.intervalStart
		intervalDuration = &value
	}
	if bucket.weeklyStart != nil && bucket.weeklyEnd != nil {
		value := *bucket.weeklyEnd - *bucket.weeklyStart
		weeklyDuration = &value
	}
	intervalID, intervalLabel := minimaxIntervalWindow(intervalDuration)
	limits := make([]UsageLimit, 0, 2)
	if limit := minimaxBuildLimit(provider, bucket, intervalID, intervalLabel, intervalDuration, bucket.intervalEnd,
		minimaxUsedFractionFromRemaining(bucket.intervalRemainingPct), bucket.intervalStatus,
		bucket.intervalUsageCount, bucket.intervalTotalCount, accountID); limit != nil {
		limits = append(limits, *limit)
	}
	if limit := minimaxBuildLimit(provider, bucket, "7d", "7 Day", weeklyDuration, bucket.weeklyEnd,
		minimaxUsedFractionFromRemaining(bucket.weeklyRemainingPct), bucket.weeklyStatus,
		bucket.weeklyUsageCount, bucket.weeklyTotalCount, accountID); limit != nil {
		limits = append(limits, *limit)
	}
	return limits
}

// fetchMinimaxCodeUsage probes the MiniMax Token Plan endpoint.
func fetchMinimaxCodeUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "minimax-code" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	// Token Plan base URLs may point at the OpenAI (/v1) or Anthropic route;
	// the quota endpoint lives at the host root.
	baseURL := usageTrimTrailingSlashes(trimJSWhitespace(params.BaseURL))
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	baseURL = strings.TrimSuffix(baseURL, "/anthropic")
	if baseURL == "" {
		baseURL = minimaxCodeBaseURL
	}
	endpoint := baseURL + minimaxRemainsPath
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + credential.APIKey,
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "MiniMax Token Plan usage request failed", map[string]any{"provider": params.Provider, "error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		usageLogWarn(fctx, "MiniMax Token Plan usage fetch failed", map[string]any{"provider": params.Provider, "status": response.Status})
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
	// MiniMax answers HTTP 200 even for rejected credentials.
	statusCode := math.NaN()
	if baseResp, ok := usageRecord(record["base_resp"]); ok {
		if number, ok := usageNumber(baseResp["status_code"]); ok {
			statusCode = number
		}
	}
	if math.IsNaN(statusCode) || statusCode != 0 {
		usageLogWarn(fctx, "MiniMax Token Plan usage response rejected", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	items, ok := record["model_remains"].([]any)
	if !ok {
		return nil, nil
	}
	limits := make([]UsageLimit, 0, 4)
	models := make([]string, 0, len(items))
	unavailable := make([]string, 0, 2)
	for _, item := range items {
		bucket := parseMinimaxBucket(item)
		if bucket == nil {
			continue
		}
		models = append(models, bucket.modelName)
		if minimaxUnavailablePlan(bucket) {
			unavailable = append(unavailable, bucket.modelName)
			continue
		}
		limits = append(limits, minimaxBuildBucketLimits(params.Provider, bucket, credential.AccountID)...)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	metadata := map[string]any{"source": "minimax-token-plan", "models": models}
	if len(unavailable) > 0 {
		metadata["unavailableModels"] = unavailable
	}
	if credential.AccountID != "" {
		metadata["accountId"] = credential.AccountID
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata, Raw: payload}, nil
}

// minimaxCodeUsageProvider returns the MiniMax Token Plan usage provider.
func minimaxCodeUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "minimax-code",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "minimax-code" && params.Credential.Type == CredentialAPIKey && params.Credential.APIKey != ""
		},
		FetchUsage: fetchMinimaxCodeUsage,
	}
}

// --- Muse Code (Meta) -------------------------------------------------------

const (
	museCodeProvider              = "muse-code"
	museCodeKeyURL                = "https://api.meta.ai/muse-code/key"
	museCodeSource                = "api.meta.ai/muse-code/key"
	museCodeFailureBackoff        = 5 * 60_000
	museCodeWeeklyID              = "1w"
	museCodeDefaultRollingMinutes = 300.0
)

// museCodeCredential is the encoded OAuth credential pair.
type museCodeCredential struct {
	oauthAccessToken string
	apiKey           string
}

// parseMuseCodeCredential decodes a Muse Code OAuth credential.
func parseMuseCodeCredential(value string) (museCodeCredential, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return museCodeCredential{}, false
	}
	access, ok := payload["oauthAccessToken"].(string)
	if !ok || trimJSWhitespace(access) == "" {
		return museCodeCredential{}, false
	}
	key, ok := payload["apiKey"].(string)
	if !ok || trimJSWhitespace(key) == "" {
		return museCodeCredential{}, false
	}
	return museCodeCredential{oauthAccessToken: access, apiKey: key}, true
}

// museCodeWindow is one usage window payload.
type museCodeWindow struct {
	usedPercent        *float64
	resetsAt           any
	windowDurationMins *float64
}

// museCodeUsage is the `subs_usage` block.
type museCodeUsage struct {
	window *museCodeWindow
	weekly *museCodeWindow
}

// museCodeKeyResponse is the key endpoint payload.
type museCodeKeyResponse struct {
	requirePaymentActionURL string
	requirePayment          *bool
	actionURL               *string
	userEmail               string
	userID                  string
	isSubsActive            *bool
	subsTierID              string
	subsTierName            string
	subsUsage               *museCodeUsage
	raw                     map[string]any
}

// parseMuseCodeWindow reads one window object.
func parseMuseCodeWindow(value any) *museCodeWindow {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	window := &museCodeWindow{}
	if percent, ok := usageNumber(record["used_percent"]); ok {
		window.usedPercent = &percent
	}
	if reset, present := record["resets_at"]; present {
		window.resetsAt = reset
	}
	if minutes, ok := usageNumber(record["window_duration_mins"]); ok {
		window.windowDurationMins = &minutes
	}
	return window
}

// parseMuseCodeUsage reads the `subs_usage` block.
func parseMuseCodeUsage(value any) *museCodeUsage {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	usage := &museCodeUsage{}
	usage.window = parseMuseCodeWindow(record["window"])
	usage.weekly = parseMuseCodeWindow(record["weekly"])
	return usage
}

// parseMuseCodeKeyResponse reads the key endpoint payload.
func parseMuseCodeKeyResponse(payload any) (*museCodeKeyResponse, bool) {
	record, ok := usageRecord(payload)
	if !ok {
		return nil, false
	}
	response := &museCodeKeyResponse{raw: record}
	response.requirePaymentActionURL, _ = usageString(record, "require_payment_action_url")
	if flag, ok := usageBool(record, "require_payment"); ok {
		response.requirePayment = &flag
	}
	if raw, present := record["action_url"]; present {
		if text, ok := raw.(string); ok {
			trimmed := trimJSWhitespace(text)
			response.actionURL = &trimmed
		}
	}
	response.userEmail, _ = usageString(record, "user_email")
	response.userID, _ = usageString(record, "user_id")
	if flag, ok := usageBool(record, "is_subs_active"); ok {
		response.isSubsActive = &flag
	}
	response.subsTierID, _ = usageString(record, "subs_tier_id")
	response.subsTierName, _ = usageString(record, "subs_tier_name")
	if raw, present := record["subs_usage"]; present && raw != nil {
		response.subsUsage = parseMuseCodeUsage(raw)
	}
	return response, true
}

// museCodeRollingWindow builds a rolling window descriptor.
func museCodeRollingWindow(minutes *float64) UsageWindow {
	valid := minutes != nil && !math.IsNaN(*minutes) && !math.IsInf(*minutes, 0) && *minutes > 0
	var durationMs *int64
	label := "Rolling Window"
	id := "rolling"
	rounded := 0.0
	if valid {
		rounded = math.Round(*minutes)
		value := int64(*minutes * 60_000)
		durationMs = &value
		if int64(rounded)%60 == 0 {
			hours := int64(rounded) / 60
			if hours == 1 {
				label = "1 Hour"
			} else {
				label = usageFormatCount(hours) + " Hours"
			}
		} else if rounded == 1 {
			label = "1 Minute"
		} else {
			label = usageFormatCount(int64(rounded)) + " Minutes"
		}
		id = usageFormatCount(int64(math.Round(float64(value)/60_000))) + "m"
	}
	return UsageWindow{ID: id, Label: label, DurationMs: durationMs}
}

// museCodePercentAmount renders a used percentage as an amount.
func museCodePercentAmount(usedPercent float64) UsageAmount {
	used := math.Min(usedPercent, 100)
	fraction := used / 100
	remaining := 100 - used
	remainingFraction := 1 - fraction
	return UsageAmount{
		Used:              &used,
		Limit:             new(100.0),
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitPercent,
	}
}

// museCodePercentLimit renders one metered window.
func museCodePercentLimit(id, label string, usedPercent *float64, window UsageWindow, tier, accountID string, notes []string) *UsageLimit {
	if usedPercent == nil || math.IsNaN(*usedPercent) || math.IsInf(*usedPercent, 0) || *usedPercent < 0 {
		return nil
	}
	amount := museCodePercentAmount(*usedPercent)
	return &UsageLimit{
		ID:     id,
		Label:  label,
		Scope:  UsageScope{Provider: museCodeProvider, AccountID: accountID, Tier: tier, WindowID: window.ID, Shared: true},
		Window: &window,
		Amount: amount,
		Status: usageStatusFor(amount.UsedFraction),
		Notes:  notes,
	}
}

// museCodeParseReset reads a reset value that may be an ISO string or epoch.
func museCodeParseReset(value any) *int64 {
	switch typed := value.(type) {
	case string:
		if millis, ok := usageIsoTimestamp(typed); ok {
			return &millis
		}
	case nil:
		return nil
	default:
		if millis, ok := usagePositiveTimestamp(typed); ok {
			return &millis
		}
	}
	return nil
}

// museCodeBuildLimits renders a payload's subscription usage.
func museCodeBuildLimits(usage *museCodeUsage, tier, accountID string) []UsageLimit {
	limits := make([]UsageLimit, 0, 2)
	if usage != nil && usage.window != nil {
		window := museCodeRollingWindow(usage.window.windowDurationMins)
		window.ResetsAt = museCodeParseReset(usage.window.resetsAt)
		if limit := museCodePercentLimit(window.ID, window.Label, usage.window.usedPercent, window, tier, accountID, nil); limit != nil {
			limits = append(limits, *limit)
		}
	}
	if usage != nil && usage.weekly != nil {
		window := UsageWindow{ID: museCodeWeeklyID, Label: "Weekly", DurationMs: new(usageWeekMs)}
		window.ResetsAt = museCodeParseReset(usage.weekly.resetsAt)
		if limit := museCodePercentLimit(museCodeWeeklyID, "Weekly", usage.weekly.usedPercent, window, tier, accountID, nil); limit != nil {
			limits = append(limits, *limit)
		}
	}
	return limits
}

// museCodeIdleLimits renders limits for a key response that omits usage while
// the rolling window is idle, carrying the previous report's weekly figure.
func museCodeIdleLimits(previous *UsageReport, tier, accountID string, now int64) []UsageLimit {
	var previousRolling, previousWeekly *UsageLimit
	if previous != nil {
		for index := range previous.Limits {
			limit := &previous.Limits[index]
			if previousRolling == nil && (limit.ID == "rolling" || strings.HasSuffix(limit.ID, "m")) && !strings.Contains(limit.ID, ":") {
				previousRolling = limit
			}
			if limit.ID == museCodeWeeklyID {
				previousWeekly = limit
			}
		}
	}
	shape := museCodeRollingWindow(new(museCodeDefaultRollingMinutes))
	if previousRolling != nil && previousRolling.Window != nil {
		shape = *previousRolling.Window
		shape.ResetsAt = nil
		shape.ResetLabel = ""
	}
	zero := 0.0
	limits := make([]UsageLimit, 0, 2)
	if limit := museCodePercentLimit(shape.ID, shape.Label, &zero, shape, tier, accountID, []string{"Idle: no request has opened this window"}); limit != nil {
		limits = append(limits, *limit)
	}
	var usedFraction *float64
	var resetsAt int64
	hasReset := false
	if previousWeekly != nil {
		if fraction, ok := UsageUsedFraction(*previousWeekly); ok {
			usedFraction = &fraction
		}
		if previousWeekly.Window != nil && previousWeekly.Window.ResetsAt != nil {
			resetsAt = *previousWeekly.Window.ResetsAt
			hasReset = true
		}
	}
	if usedFraction == nil || !hasReset {
		return limits
	}
	reset := resetsAt <= now
	if reset {
		// Meta's weekly boundary is fixed, so a passed reset rolls forward by
		// whole weeks.
		resetsAt += (int64(math.Floor(float64(now-resetsAt)/float64(usageWeekMs))) + 1) * usageWeekMs
	}
	usedPercent := *usedFraction * 100
	note := "Carried from the last reported usage; later usage not visible"
	if reset {
		usedPercent = 0
		note = "Reset since the last reported usage"
	}
	window := UsageWindow{ID: museCodeWeeklyID, Label: "Weekly", DurationMs: new(usageWeekMs), ResetsAt: &resetsAt}
	if limit := museCodePercentLimit(museCodeWeeklyID, "Weekly", &usedPercent, window, tier, accountID, []string{note}); limit != nil {
		limits = append(limits, *limit)
	}
	return limits
}

// requestMuseCodeKey exchanges a Meta access token for the key response.
func requestMuseCodeKey(ctx context.Context, fctx UsageFetchContext, accessToken string) (*museCodeKeyResponse, error) {
	probeCtx, cancel := usageDeadline(ctx, 20*time.Second)
	defer cancel()
	response, err := usageDo(probeCtx, fctx, usageRejectRedirects, "POST", museCodeKeyURL, map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
		"x-api-version": "1.0.0",
	}, []byte("{}"))
	if err != nil {
		return nil, err
	}
	if !response.OK() {
		return nil, NewUsageHTTPError("Muse Code key exchange failed: "+usageFormatCount(int64(response.Status)), response.Status)
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, NewUsageHTTPError("Muse Code key exchange returned invalid JSON", response.Status)
	}
	parsed, ok := parseMuseCodeKeyResponse(payload)
	if !ok {
		return nil, NewUsageHTTPError("Invalid Muse Code key response", response.Status)
	}
	return parsed, nil
}

// fetchMuseCodeUsage probes Meta's Muse Code key endpoint.
func fetchMuseCodeUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != museCodeProvider || params.Credential.Type != CredentialOAuth {
		return nil, nil
	}
	encoded := trimJSWhitespace(params.Credential.AccessToken)
	if encoded == "" {
		return nil, nil
	}
	credential, ok := parseMuseCodeCredential(encoded)
	if !ok {
		return nil, nil
	}
	payload, err := requestMuseCodeKey(ctx, fctx, credential.oauthAccessToken)
	if err != nil {
		httpErr, isHTTP := err.(*UsageHTTPError)
		if isHTTP && (httpErr.Status == 401 || httpErr.Status == 403) {
			return nil, NewUsageHTTPError(httpErr.Message, httpErr.Status)
		}
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, nil
	}
	if payload.isSubsActive != nil && !*payload.isSubsActive {
		return nil, NewUsageHTTPError("Muse Code subscription is inactive", 403)
	}
	email := strings.ToLower(trimJSWhitespace(payload.userEmail))
	if email == "" {
		email = params.Credential.Email
	}
	accountID := trimJSWhitespace(payload.userID)
	if accountID == "" {
		accountID = params.Credential.AccountID
	}
	if accountID == "" {
		accountID = email
	}
	tier := trimJSWhitespace(payload.subsTierName)
	if tier == "" {
		tier = trimJSWhitespace(payload.subsTierID)
	}
	now := timeNowMillis()
	// Payment-required responses omit `subs_usage` too; only an explicitly
	// active subscription with no payment demand means an idle window.
	actionURL := ""
	if payload.actionURL != nil {
		actionURL = trimJSWhitespace(*payload.actionURL)
	}
	idle := payload.isSubsActive != nil && *payload.isSubsActive &&
		(payload.requirePayment == nil || !*payload.requirePayment) &&
		trimJSWhitespace(payload.requirePaymentActionURL) == "" && actionURL == ""
	var limits []UsageLimit
	if payload.subsUsage != nil {
		limits = museCodeBuildLimits(payload.subsUsage, tier, accountID)
	} else if idle {
		limits = museCodeIdleLimits(fctx.PreviousReport, tier, accountID, now)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	raw := payload.raw
	if raw != nil {
		copied := make(map[string]any, len(raw))
		for key, value := range raw {
			copied[key] = value
		}
		delete(copied, "api_key")
		raw = copied
	}
	metadata := map[string]any{"source": museCodeSource}
	if email != "" {
		metadata["email"] = email
	}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	if payload.subsTierName != "" {
		metadata["tier"] = payload.subsTierName
	}
	return &UsageReport{Provider: museCodeProvider, FetchedAt: now, Limits: limits, Metadata: metadata, Raw: raw}, nil
}

// museCodeUsageProvider returns the Muse Code usage provider.
func museCodeUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID:                   museCodeProvider,
		ValidatesCredentials: true,
		FailureBackoffMs:     museCodeFailureBackoff,
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != museCodeProvider || params.Credential.Type != CredentialOAuth || params.Credential.AccessToken == "" {
				return false
			}
			_, ok := parseMuseCodeCredential(params.Credential.AccessToken)
			return ok
		},
		FetchUsage: fetchMuseCodeUsage,
	}
}
