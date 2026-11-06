package ai

// Ports packages/ai/src/usage/gemini.ts and usage/google-antigravity.ts: the
// Gemini CLI quota buckets and Antigravity's Cloud Code Assist quota summary
// (with the legacy per-model parser as a fallback).

import (
	"context"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// quotaTierFor ports pi-catalog's quotaTierFor over the compiled rules that
// matter to usage reports: cursor, google-gemini-cli, google-antigravity,
// openai-codex and factory-droid.
func quotaTierFor(provider, model string) (string, bool) {
	rule, ok := usageQuotaTierRules[provider]
	if !ok {
		return "", false
	}
	for _, tier := range rule.tiers {
		for _, candidate := range tier.models {
			if candidate == model {
				return tier.label, true
			}
		}
	}
	for _, fallback := range rule.fallbacks {
		if strings.Contains(model, fallback.substring) {
			return fallback.label, true
		}
	}
	if rule.defaultTier != "" {
		return rule.defaultTier, true
	}
	return "", false
}

// usageQuotaTierRule is one compiled quota-tier rule.
type usageQuotaTierRule struct {
	defaultTier string
	tiers       []usageQuotaTier
	fallbacks   []usageQuotaFallback
}

type usageQuotaTier struct {
	label  string
	models []string
}

type usageQuotaFallback struct {
	label     string
	substring string
}

// usageQuotaTierRules mirrors the compiled runtime-behavior quota tiers that
// usage reports consult.
var usageQuotaTierRules = map[string]usageQuotaTierRule{
	"cursor": {
		defaultTier: "api",
		tiers: []usageQuotaTier{
			{label: "auto", models: []string{"composer-2.5", "default"}},
		},
		fallbacks: []usageQuotaFallback{{label: "auto", substring: "grok"}, {label: "auto", substring: "composer"}},
	},
	"google-gemini-cli": {
		tiers: []usageQuotaTier{
			{label: "3-Flash", models: []string{"gemini-3-flash-preview", "gemini-3-flash", "gemini-3.5-flash"}},
			{label: "Flash", models: []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.0-flash", "gemini-1.5-flash"}},
			{label: "Pro", models: []string{"gemini-2.5-pro", "gemini-3-pro-preview", "gemini-3.1-pro-preview", "gemini-3-pro", "gemini-3.1-pro", "gemini-pro-agent", "gemini-1.5-pro"}},
		},
		fallbacks: []usageQuotaFallback{{label: "Flash", substring: "flash"}, {label: "Pro", substring: "pro"}},
	},
	"google-antigravity": {
		tiers: []usageQuotaTier{
			{label: "anthropic", models: []string{"claude-opus-4-6"}},
			{label: "google", models: []string{"gemini-3-pro", "gemma-3", "tab_flash_lite_preview", "tab_jump_flash_lite_preview"}},
			{label: "openai", models: []string{"gpt-oss-120b", "openai/gpt-oss-120b"}},
		},
		fallbacks: []usageQuotaFallback{
			{label: "anthropic", substring: "claude-"},
			{label: "google", substring: "gemini-"},
			{label: "google", substring: "gemma-"},
			{label: "google", substring: "tab_"},
			{label: "openai", substring: "gpt-"},
			{label: "openai", substring: "openai/"},
		},
	},
	"openai-codex": {
		tiers: []usageQuotaTier{
			{label: "spark", models: []string{"gpt-5.3-codex-spark"}},
			{label: "chat", models: []string{"gpt-5.3-codex"}},
		},
		fallbacks: []usageQuotaFallback{{label: "spark", substring: "-spark"}, {label: "chat", substring: "gpt-"}},
	},
	"factory-droid": {
		tiers: []usageQuotaTier{
			{label: "core", models: []string{
				"inkling", "mistral-large-4", "mistral-medium-3.5", "glm-5.3-flash", "glm-5.3", "glm-5.2",
				"glm-5.2-fast", "kimi-k3", "qwen3.8-max", "nemotron-3-ultra", "deepseek-v4.1-flash",
				"deepseek-v4.1-flash-1017", "nemotron-3.5-super", "beam-1.0", "deepseek-v4-flash-0731",
				"deepseek-v4-pro", "minimax-m3", "minimax-m2.7",
			}},
			{label: "standard", models: []string{
				"claude-fable-5.1", "claude-fable-5", "claude-opus-5-5", "claude-opus-5-5-fast", "claude-opus-5",
				"claude-opus-5-fast", "claude-opus-4-8", "claude-opus-4-8-fast", "claude-opus-4-7", "claude-opus-4-6",
				"claude-opus-4-5-20251101", "claude-sonnet-5-5", "claude-sonnet-5", "claude-sonnet-4-6",
				"claude-sonnet-4-5-20250929", "claude-haiku-5-5", "claude-haiku-4-5-20251001", "gpt-6-astra",
				"gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "gpt-5.6-sol", "gpt-5.6-sol-fast", "gpt-5.6-terra",
				"gpt-6-astra-ultrafast", "gpt-6.1-sol-ultrafast", "gpt-6.1-terra", "gpt-6-sol-fast",
				"gpt-6-luna-fast", "muse-spark-open", "muse-spark-1.2", "gpt-5.6-luna", "gpt-5.5", "gpt-5.5-fast",
				"gpt-5.5-pro", "gpt-5.4", "gpt-5.4-fast", "gpt-5.4-mini", "gpt-5.4-mini-fast", "gpt-5.3-codex",
				"gpt-5.3-codex-fast", "gpt-5.2", "garnet-07-15", "gemini-3.1-pro-preview", "gemini-3.8-flash",
				"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3-flash-preview", "grok-4.7",
				"grok-4.6", "grok-4.5", "atlas-07-21", "aster-07-15", "gpt-image-2.5-sunburst",
				"gpt-image-2.5-flare", "gemini-3-pro-image-preview",
			}},
		},
	},
}

// --- Gemini CLI -------------------------------------------------------------

const geminiCLIDefaultEndpoint = "https://cloudcode-pa.googleapis.com"

// getGeminiCLIHeaders mirrors the Gemini CLI identity, which unlocks higher
// rate limits.
func getGeminiCLIHeaders() map[string]string {
	version := "0.46.0"
	platform := runtime.GOOS
	arch := runtime.GOARCH
	return map[string]string{
		"User-Agent":      "GeminiCLI/" + version + "/gemini-3.1-pro-preview (" + platform + "; " + arch + "; terminal)",
		"Client-Metadata": "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
	}
}

// geminiParseWindow renders a bucket window from its reset time.
func geminiParseWindow(resetTime string) UsageWindow {
	if resetTime == "" {
		return UsageWindow{ID: "quota", Label: "Quota window"}
	}
	if resetsAt, ok := usageIsoTimestamp(resetTime); ok {
		return UsageWindow{ID: "reset-" + usageFormatCount(resetsAt), Label: "Quota window", ResetsAt: &resetsAt}
	}
	return UsageWindow{ID: "quota", Label: "Quota window"}
}

// geminiBuildAmount renders a bucket's percent amount from remainingFraction.
func geminiBuildAmount(remainingFraction *float64) UsageAmount {
	if remainingFraction == nil || math.IsNaN(*remainingFraction) || math.IsInf(*remainingFraction, 0) {
		return UsageAmount{Unit: UsageUnitPercent}
	}
	remaining := usageClampFraction(*remainingFraction)
	used := usageClampFraction(1 - remaining)
	usedPercent := math.Round(used*1000) / 10
	remainingPercent := math.Round(remaining*1000) / 10
	return UsageAmount{
		Unit:              UsageUnitPercent,
		Used:              &usedPercent,
		Remaining:         &remainingPercent,
		Limit:             new(100.0),
		UsedFraction:      &used,
		RemainingFraction: &remaining,
	}
}

// geminiResolveAccessToken returns the unexpired OAuth token for `/v1internal`.
func geminiResolveAccessToken(params UsageFetchParams) string {
	credential := params.Credential
	if credential.Type != CredentialOAuth || credential.AccessToken == "" {
		return ""
	}
	if credential.ExpiresAt != nil && *credential.ExpiresAt <= timeNowMillis() {
		return ""
	}
	return credential.AccessToken
}

// geminiProjectID reads `cloudaicompanionProject`.
func geminiProjectID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	switch typed := payload["cloudaicompanionProject"].(type) {
	case string:
		return typed
	default:
		if record, ok := usageRecord(typed); ok {
			id, _ := usageString(record, "id")
			return id
		}
	}
	return ""
}

// geminiPostCodeAssist posts a Cloud Code Assist RPC.
func geminiPostCodeAssist(ctx context.Context, fctx UsageFetchContext, endpoint, accessToken string, body map[string]any) (map[string]any, bool) {
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
	}
	for name, value := range getGeminiCLIHeaders() {
		headers[name] = value
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "POST", endpoint, headers, []byte(usageJSONObject(body)))
	if err != nil || !response.OK() {
		return nil, false
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, false
	}
	record, ok := usageRecord(payload)
	return record, ok
}

// fetchGoogleGeminiCLIUsage probes the Gemini CLI quota endpoints.
func fetchGoogleGeminiCLIUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "google-gemini-cli" {
		return nil, nil
	}
	credential := params.Credential
	accessToken := geminiResolveAccessToken(params)
	if accessToken == "" {
		return nil, nil
	}
	baseURL := usageTrimTrailingSlashes(trimJSWhitespace(params.BaseURL))
	if baseURL == "" {
		baseURL = geminiCLIDefaultEndpoint
	}
	loadBody := map[string]any{
		"metadata": map[string]any{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}
	if credential.ProjectID != "" {
		loadBody["cloudaicompanionProject"] = credential.ProjectID
	}
	loadResponse, _ := geminiPostCodeAssist(ctx, fctx, baseURL+"/v1internal:loadCodeAssist", accessToken, loadBody)
	projectID := credential.ProjectID
	if projectID == "" {
		projectID = geminiProjectID(loadResponse)
	}
	quotaBody := map[string]any{}
	if projectID != "" {
		quotaBody["project"] = projectID
	}
	quotaResponse, ok := geminiPostCodeAssist(ctx, fctx, baseURL+"/v1internal:retrieveUserQuota", accessToken, quotaBody)
	if !ok {
		return nil, nil
	}
	limits := make([]UsageLimit, 0, 4)
	for index, item := range usageArrayOrEmpty(quotaResponse["buckets"]) {
		bucket, ok := usageRecord(item)
		if !ok {
			continue
		}
		modelID, _ := usageString(bucket, "modelId")
		resetTime, _ := usageString(bucket, "resetTime")
		window := geminiParseWindow(resetTime)
		amount := geminiBuildAmount(usageNumberOrNil(bucket["remainingFraction"]))
		tier := ""
		if modelID != "" {
			tier, _ = quotaTierFor("google-gemini-cli", modelID)
		}
		label := "Gemini quota"
		if modelID != "" {
			label = "Gemini " + modelID
		}
		id := "unknown:" + window.ID
		if modelID != "" {
			id = modelID + ":" + window.ID
		} else {
			id = "unknown:" + window.ID + ":" + usageFormatCount(int64(index))
		}
		limits = append(limits, UsageLimit{
			ID:     id,
			Label:  label,
			Scope:  UsageScope{Provider: params.Provider, AccountID: credential.AccountID, ProjectID: projectID, ModelID: modelID, Tier: tier, WindowID: window.ID},
			Window: &window,
			Amount: amount,
		})
	}
	metadata := map[string]any{}
	if tier, ok := usageRecord(loadResponse["currentTier"]); ok {
		if id, ok := usageString(tier, "id"); ok {
			metadata["currentTierId"] = id
		}
		if name, ok := usageString(tier, "name"); ok {
			metadata["currentTierName"] = name
		}
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata, Raw: quotaResponse}, nil
}

// usageNumberOrNil reads an optional finite number.
func usageNumberOrNil(value any) *float64 {
	number, ok := usageNumber(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil
	}
	return &number
}

// googleGeminiCLIUsageProvider returns the Gemini CLI usage provider.
func googleGeminiCLIUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "google-gemini-cli",
		Supports: func(params UsageFetchParams) bool {
			return params.Credential.Type == CredentialOAuth && params.Credential.AccessToken != ""
		},
		FetchUsage: fetchGoogleGeminiCLIUsage,
	}
}

// --- Antigravity ------------------------------------------------------------

const (
	antigravityDefaultEndpoint     = "https://daily-cloudcode-pa.googleapis.com"
	antigravitySandboxEndpoint     = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	antigravityAvailableModelsPath = "/v1internal:fetchAvailableModels"
	antigravityQuotaSummaryPath    = "/v1internal:retrieveUserQuotaSummary"
)

// antigravityQuotaInfo is one per-model quota entry.
type antigravityQuotaInfo struct {
	remainingFraction *float64
	resetTime         string
	tier              string
	windowID          string
	windowLabel       string
	apiProvider       string
	modelProvider     string
	// window is the descriptor inferred for entries that name no window.
	window *antigravityWindowDescriptor
}

// antigravityWindowDescriptor is a normalized quota window.
type antigravityWindowDescriptor struct {
	id       string
	label    string
	duration *int64
}

// antigravityClassifyWindow maps a window id/label to a descriptor.
func antigravityClassifyWindow(id, label string) *antigravityWindowDescriptor {
	source := strings.ToLower(id + " " + label)
	switch {
	case strings.Contains(source, "week") || strings.Contains(source, "7d") || containsDigitsThenUnit(source, 7, "day"):
		return &antigravityWindowDescriptor{id: "weekly", label: "Weekly", duration: new(usageWeekMs)}
	case strings.Contains(source, "5h") || strings.Contains(source, "five hour") || containsDigitsThenUnit(source, 5, "hour"):
		return &antigravityWindowDescriptor{id: "5h", label: "5 Hour", duration: new(5 * usageHourMs)}
	case strings.Contains(source, "day") || strings.Contains(source, "daily") || strings.Contains(source, "24h"):
		return &antigravityWindowDescriptor{id: "daily", label: "Daily", duration: new(usageDayMs)}
	}
	if id != "" || label != "" {
		windowID := id
		if windowID == "" {
			windowID = label
		}
		windowLabel := label
		if windowLabel == "" {
			windowLabel = id
		}
		if windowID == "" {
			windowID = "default"
		}
		if windowLabel == "" {
			windowLabel = "Default"
		}
		return &antigravityWindowDescriptor{id: windowID, label: windowLabel}
	}
	return nil
}

// containsDigitsThenUnit matches "7 days"/"5_hour" style spans.
func containsDigitsThenUnit(source string, digits int, unit string) bool {
	target := usageFormatCount(int64(digits)) + " " + unit
	return strings.Contains(source, target)
}

// antigravityInferWindowFromReset guesses a window from its reset distance.
func antigravityInferWindowFromReset(resetAt *int64, nowMs int64) *antigravityWindowDescriptor {
	if resetAt != nil && *resetAt-nowMs > usageDayMs {
		return &antigravityWindowDescriptor{id: "weekly", label: "Weekly", duration: new(usageWeekMs)}
	}
	return &antigravityWindowDescriptor{id: "daily", label: "Daily", duration: new(usageDayMs)}
}

// antigravityQuotaKey groups quota entries that describe one counter.
func antigravityQuotaKey(info antigravityQuotaInfo) string {
	return info.modelProvider + "|" + info.apiProvider + "|" + info.tier
}

// antigravityClampFraction clamps a remaining fraction.
func antigravityClampFraction(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	clamped := math.Min(math.Max(*value, 0), 1)
	return &clamped
}

// antigravityGetUsageStatus maps a remaining fraction to a status.
func antigravityGetUsageStatus(remainingFraction *float64) UsageStatus {
	if remainingFraction == nil {
		return UsageStatusUnknown
	}
	if *remainingFraction <= 0 {
		return UsageStatusExhausted
	}
	if *remainingFraction <= 0.1 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// antigravityParseWindow renders a window from an entry and its descriptor.
func antigravityParseWindow(info antigravityQuotaInfo, descriptor *antigravityWindowDescriptor) *UsageWindow {
	resetAt, hasReset := usageIsoTimestamp(info.resetTime)
	if descriptor == nil && !hasReset {
		return nil
	}
	window := &UsageWindow{Label: info.windowLabel}
	if descriptor != nil {
		window.ID = descriptor.id
		window.DurationMs = descriptor.duration
	}
	if window.ID == "" {
		window.ID = info.windowID
	}
	if window.Label == "" && descriptor != nil {
		window.Label = descriptor.label
	}
	if window.ID == "" {
		window.ID = "default"
	}
	if window.Label == "" {
		window.Label = "Default"
	}
	if hasReset {
		window.ResetsAt = &resetAt
	}
	return window
}

// antigravityBuildPercentAmount renders a percent amount from a remaining
// fraction.
func antigravityBuildPercentAmount(remainingFraction *float64) UsageAmount {
	amount := UsageAmount{Unit: UsageUnitPercent}
	if remainingFraction == nil {
		return amount
	}
	usedFraction := 1 - *remainingFraction
	used := usedFraction * 100
	remaining := *remainingFraction * 100
	amount.RemainingFraction = remainingFraction
	amount.UsedFraction = &usedFraction
	amount.Remaining = &remaining
	amount.Used = &used
	amount.Limit = new(100.0)
	return amount
}

// antigravityBuildAmount renders an entry's amount.
func antigravityBuildAmount(info antigravityQuotaInfo) UsageAmount {
	remaining := antigravityClampFraction(info.remainingFraction)
	// Legacy responses omit remainingFraction for exhausted counters and keep
	// only resetTime: that shape is "blocked until reset", not unknown.
	if remaining == nil && info.resetTime != "" {
		remaining = new(0.0)
	}
	return antigravityBuildPercentAmount(remaining)
}

// antigravityCounterName names a backend counter.
func antigravityCounterName(info antigravityQuotaInfo) string {
	provider := info.modelProvider
	if provider == "" {
		provider = info.apiProvider
	}
	switch provider {
	case "MODEL_PROVIDER_ANTHROPIC", "API_PROVIDER_ANTHROPIC_VERTEX":
		return "Anthropic"
	case "MODEL_PROVIDER_GOOGLE", "API_PROVIDER_GOOGLE_GEMINI":
		return "Google"
	case "MODEL_PROVIDER_OPENAI", "API_PROVIDER_OPENAI_VERTEX":
		return "OpenAI"
	}
	return ""
}

// antigravityParseQuotaInfo reads one quota entry.
func antigravityParseQuotaInfo(value any, apiProvider, modelProvider, tier string, descriptor *antigravityWindowDescriptor) *antigravityQuotaInfo {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	info := &antigravityQuotaInfo{apiProvider: apiProvider, modelProvider: modelProvider, tier: tier}
	info.remainingFraction = usageNumberOrNil(record["remainingFraction"])
	info.resetTime, _ = usageString(record, "resetTime")
	if value, ok := usageString(record, "tier"); ok && tier == "" {
		info.tier = value
	}
	info.windowID, _ = usageString(record, "windowId")
	info.windowLabel, _ = usageString(record, "windowLabel")
	if descriptor != nil {
		if info.windowID == "" {
			info.windowID = descriptor.id
		}
		if info.windowLabel == "" {
			info.windowLabel = descriptor.label
		}
	}
	return info
}

// antigravityNormalizeQuotaInfos collects every quota entry of a model.
func antigravityNormalizeQuotaInfos(record map[string]any) []antigravityQuotaInfo {
	apiProvider, _ := usageString(record, "apiProvider")
	modelProvider, _ := usageString(record, "modelProvider")
	results := make([]antigravityQuotaInfo, 0, 4)
	addValue := func(value any, tier string, descriptor *antigravityWindowDescriptor) {
		if value == nil {
			return
		}
		if items, ok := value.([]any); ok {
			for _, item := range items {
				if info := antigravityParseQuotaInfo(item, apiProvider, modelProvider, tier, descriptor); info != nil {
					results = append(results, *info)
				}
			}
			return
		}
		if info := antigravityParseQuotaInfo(value, apiProvider, modelProvider, tier, descriptor); info != nil {
			results = append(results, *info)
		}
	}
	addValue(record["quotaInfo"], "", nil)
	addValue(record["quotaInfos"], "", nil)
	addValue(record["dailyQuotaInfo"], "", antigravityClassifyWindow("daily", "Daily"))
	addValue(record["dailyQuotaInfos"], "", antigravityClassifyWindow("daily", "Daily"))
	addValue(record["weeklyQuotaInfo"], "", antigravityClassifyWindow("weekly", "Weekly"))
	addValue(record["weeklyQuotaInfos"], "", antigravityClassifyWindow("weekly", "Weekly"))
	if byTier, ok := usageRecord(record["quotaInfoByTier"]); ok {
		for tier, value := range quotaTierOrder(byTier) {
			addValue(value, tier, nil)
		}
	}
	for _, key := range []string{"quotaInfoByWindow", "quotaInfosByWindow"} {
		if byWindow, ok := usageRecord(record[key]); ok {
			for windowID, value := range byWindow {
				addValue(value, "", antigravityClassifyWindow(windowID, ""))
			}
		}
	}
	return results
}

// quotaTierOrder returns the tier-keyed map (order is irrelevant to results).
func quotaTierOrder(values map[string]any) map[string]any { return values }

// antigravityInferWindowDescriptors assigns descriptors to entries that name
// no window, grouping by counter and treating the latest reset as weekly.
func antigravityInferWindowDescriptors(infos []antigravityQuotaInfo, nowMs int64) []*antigravityWindowDescriptor {
	descriptors := make([]*antigravityWindowDescriptor, len(infos))
	groups := map[string][]int{}
	order := make([]string, 0, 2)
	for index, info := range infos {
		if explicit := antigravityClassifyWindow(info.windowID, info.windowLabel); explicit != nil {
			descriptors[index] = explicit
			continue
		}
		key := antigravityQuotaKey(info)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], index)
	}
	for _, key := range order {
		indexes := groups[key]
		resetSet := map[int64]bool{}
		for _, index := range indexes {
			if resetAt, ok := usageIsoTimestamp(infos[index].resetTime); ok {
				resetSet[resetAt] = true
			}
		}
		resets := make([]int64, 0, len(resetSet))
		for reset := range resetSet {
			resets = append(resets, reset)
		}
		sort.Slice(resets, func(i, j int) bool { return resets[i] < resets[j] })
		var latest *int64
		if len(resets) > 1 {
			latest = &resets[len(resets)-1]
		}
		for _, index := range indexes {
			resetAt, ok := usageIsoTimestamp(infos[index].resetTime)
			if latest != nil && ok && resetAt == *latest {
				descriptors[index] = &antigravityWindowDescriptor{id: "weekly", label: "Weekly", duration: new(usageWeekMs)}
				continue
			}
			var resetPtr *int64
			if ok {
				resetPtr = &resetAt
			}
			descriptors[index] = antigravityInferWindowFromReset(resetPtr, nowMs)
		}
	}
	return descriptors
}

// antigravityFetchEndpoint posts to the first endpoint that answers.
func antigravityFetchEndpoint(ctx context.Context, fctx UsageFetchContext, endpoints []string, path, projectID, accessToken string) (*usageResponse, string, error) {
	attempted := antigravityDefaultEndpoint
	if len(endpoints) > 0 {
		attempted = endpoints[0]
	}
	headers := map[string]string{
		"Authorization":   "Bearer " + accessToken,
		"Content-Type":    "application/json",
		"User-Agent":      antigravityUserAgent(),
		"Accept-Encoding": "gzip",
	}
	body := []byte(usageJSONObject(map[string]any{"project": projectID}))
	var last *usageResponse
	for index, endpoint := range endpoints {
		attempted = endpoint
		response, err := usageDo(ctx, fctx, usageFollowRedirects, "POST", endpoint+path, headers, body)
		if err != nil {
			if index == len(endpoints)-1 {
				return nil, attempted, err
			}
			continue
		}
		last = response
		if response.OK() || !isTransientUsageStatus(response.Status) {
			return response, endpoint, nil
		}
	}
	return last, attempted, nil
}

// isTransientUsageStatus mirrors the upstream transient-status classification.
func isTransientUsageStatus(status int) bool {
	switch status {
	case 408, 409, 425, 429, 500, 502, 503, 504, 522, 524:
		return true
	}
	return false
}

// antigravityUserAgent is the native Antigravity client identity.
func antigravityUserAgent() string {
	version := trimJSWhitespace(lookupEnvValue("PI_AI_ANTIGRAVITY_VERSION"))
	if version == "" {
		version = "2.26.0"
	}
	cl := trimJSWhitespace(lookupEnvValue("PI_AI_ANTIGRAVITY_CL"))
	if cl == "" {
		cl = "1005016104"
	}
	osName := trimJSWhitespace(lookupEnvValue("PI_AI_ANTIGRAVITY_OS"))
	arch := trimJSWhitespace(lookupEnvValue("PI_AI_ANTIGRAVITY_ARCH"))
	if osName == "" || arch == "" {
		defaultOS, defaultArch := antigravityPlatform(runtime.GOOS)
		if osName == "" {
			osName = defaultOS
		}
		if arch == "" {
			arch = defaultArch
		}
	}
	return "antigravity/hub/" + version + " (aidev_client; os_type=" + osName + "; arch=" + arch + "; cl=" + cl + ")"
}

// antigravityPlatform is the native build a host would run.
func antigravityPlatform(goos string) (string, string) {
	switch goos {
	case "darwin":
		return "darwin", "arm64"
	case "windows":
		return "windows", "amd64"
	default:
		return "linux", "amd64"
	}
}

// antigravityQuotaSummaryBucket is one summary bucket.
type antigravityQuotaSummaryBucket struct {
	bucketID          string
	displayName       string
	window            string
	remainingFraction *float64
	remainingAmount   any
	disabled          bool
	resetTime         string
}

// antigravityQuotaSummaryGroup is one summary group.
type antigravityQuotaSummaryGroup struct {
	displayName string
	buckets     []antigravityQuotaSummaryBucket
}

// antigravityParseSummaryBucket reads one bucket.
func antigravityParseSummaryBucket(value any) *antigravityQuotaSummaryBucket {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	bucket := &antigravityQuotaSummaryBucket{}
	bucket.bucketID, _ = usageString(record, "bucketId")
	bucket.displayName, _ = usageString(record, "displayName")
	bucket.window, _ = usageString(record, "window")
	bucket.remainingFraction = usageNumberOrNil(record["remainingFraction"])
	if raw, present := record["remainingAmount"]; present {
		bucket.remainingAmount = raw
	}
	bucket.disabled, _ = usageBool(record, "disabled")
	bucket.resetTime, _ = usageString(record, "resetTime")
	return bucket
}

// antigravitySummaryCounterKeys maps a bucket to the counters it meters.
func antigravitySummaryCounterKeys(group *antigravityQuotaSummaryGroup, bucket *antigravityQuotaSummaryBucket) []string {
	bucketID := strings.ToLower(bucket.bucketID)
	groupName := ""
	if group != nil {
		groupName = strings.ToLower(group.displayName)
	}
	if strings.HasPrefix(bucketID, "gemini-") || strings.Contains(groupName, "gemini") {
		return []string{"google"}
	}
	if strings.HasPrefix(bucketID, "3p-") || strings.Contains(groupName, "claude") ||
		strings.Contains(groupName, "gpt") || strings.Contains(groupName, "third party") {
		// One shared third-party group: duplicate into the model-family scopes so
		// both Claude and GPT requests rank against it.
		return []string{"anthropic", "openai"}
	}
	return []string{"default"}
}

// antigravitySummaryCounterName names a counter key.
func antigravitySummaryCounterName(key string) string {
	switch key {
	case "google":
		return "Google"
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	}
	return ""
}

// antigravitySummaryAmount renders a bucket amount.
func antigravitySummaryAmount(bucket *antigravityQuotaSummaryBucket) UsageAmount {
	if remaining := antigravityClampFraction(bucket.remainingFraction); remaining != nil {
		return antigravityBuildPercentAmount(remaining)
	}
	var amount float64
	hasAmount := false
	switch typed := bucket.remainingAmount.(type) {
	case float64:
		amount, hasAmount = typed, true
	default:
		if number, ok := usageNumber(bucket.remainingAmount); ok {
			amount, hasAmount = number, true
		}
	}
	if !hasAmount || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return UsageAmount{Unit: UsageUnitUnknown}
	}
	return UsageAmount{Unit: UsageUnitUnknown, Remaining: &amount}
}

// antigravityBuildQuotaSummaryReport renders `retrieveUserQuotaSummary`.
func antigravityBuildQuotaSummaryReport(data map[string]any, params UsageFetchParams, endpoint string, nowMs int64) *UsageReport {
	groups := make([]antigravityQuotaSummaryGroup, 0, 2)
	for _, rawGroup := range usageArrayOrEmpty(data["groups"]) {
		record, ok := usageRecord(rawGroup)
		if !ok {
			continue
		}
		group := antigravityQuotaSummaryGroup{}
		group.displayName, _ = usageString(record, "displayName")
		for _, rawBucket := range usageArrayOrEmpty(record["buckets"]) {
			if bucket := antigravityParseSummaryBucket(rawBucket); bucket != nil {
				group.buckets = append(group.buckets, *bucket)
			}
		}
		groups = append(groups, group)
	}
	topLevel := make([]antigravityQuotaSummaryBucket, 0, 4)
	for _, rawBucket := range usageArrayOrEmpty(data["buckets"]) {
		if bucket := antigravityParseSummaryBucket(rawBucket); bucket != nil {
			topLevel = append(topLevel, *bucket)
		}
	}
	hasGrouped := false
	for _, group := range groups {
		if len(group.buckets) > 0 {
			hasGrouped = true
			break
		}
	}
	if !hasGrouped && len(topLevel) == 0 {
		return nil
	}
	limits := make([]UsageLimit, 0, 6)
	addBucket := func(bucket *antigravityQuotaSummaryBucket, group *antigravityQuotaSummaryGroup) {
		if bucket.disabled {
			return
		}
		descriptor := antigravityClassifyWindow(bucket.window, bucket.displayName)
		info := antigravityQuotaInfo{
			windowID:          bucket.window,
			windowLabel:       descriptorLabel(descriptor),
			resetTime:         bucket.resetTime,
			remainingFraction: bucket.remainingFraction,
		}
		window := antigravityParseWindow(info, descriptor)
		amount := antigravitySummaryAmount(bucket)
		counterKeys := antigravitySummaryCounterKeys(group, bucket)
		sharedGroup := ""
		if len(counterKeys) > 1 {
			groupName := ""
			if group != nil {
				groupName = group.displayName
			}
			base := bucket.bucketID
			if base == "" {
				base = groupName
			}
			if base == "" {
				base = "third-party"
			}
			windowID := bucket.window
			if window != nil {
				windowID = window.ID
			}
			if windowID == "" {
				windowID = "default"
			}
			sharedGroup = base + ":" + windowID
		}
		// Argon's weekly-only quota is separate from the shared Gemini weekly
		// bucket; the bucket id is the only distinguishing signal.
		bucketLabel := ""
		if strings.HasPrefix(strings.ToLower(bucket.bucketID), "gemini-argon-") {
			bucketLabel = "Gemini (Argon)"
		}
		for _, counterKey := range counterKeys {
			counterName := antigravitySummaryCounterName(counterKey)
			windowID := ""
			if window != nil {
				windowID = window.ID
			}
			if windowID == "" {
				windowID = bucket.window
			}
			if windowID == "" {
				windowID = bucket.bucketID
			}
			if windowID == "" {
				windowID = "default"
			}
			label := bucketLabel
			switch {
			case sharedGroup != "":
				label = "Claude & GPT (shared)"
			case label != "":
			case counterKey == "google":
				label = "Gemini"
			default:
				label = counterName
			}
			if label == "" && group != nil {
				label = group.displayName
			}
			if label == "" {
				label = bucket.displayName
			}
			if label == "" {
				label = "Usage"
			}
			base := bucket.bucketID
			if base == "" {
				base = windowID
			}
			scope := UsageScope{
				Provider:  params.Provider,
				AccountID: params.Credential.AccountID,
				ProjectID: params.Credential.ProjectID,
				WindowID:  windowID,
			}
			if sharedGroup != "" {
				scope.Shared = true
				scope.SharedGroup = sharedGroup
			}
			status := UsageStatusUnknown
			if amount.RemainingFraction != nil {
				status = antigravityGetUsageStatus(amount.RemainingFraction)
			} else if amount.Remaining != nil && *amount.Remaining == 0 {
				status = UsageStatusExhausted
			}
			limits = append(limits, UsageLimit{
				ID:     params.Provider + ":" + counterKey + ":default:" + base,
				Label:  label,
				Scope:  scope,
				Window: window,
				Amount: amount,
				Status: status,
			})
		}
	}
	if hasGrouped {
		for index := range groups {
			group := &groups[index]
			for bucketIndex := range group.buckets {
				addBucket(&group.buckets[bucketIndex], group)
			}
		}
	} else {
		for index := range topLevel {
			addBucket(&topLevel[index], nil)
		}
	}
	sort.SliceStable(limits, func(i, j int) bool {
		return antigravityFractionKey(limits[i]) < antigravityFractionKey(limits[j])
	})
	metadata := map[string]any{"endpoint": endpoint}
	if params.Credential.ProjectID != "" {
		metadata["projectId"] = params.Credential.ProjectID
	}
	if params.Credential.Email != "" {
		metadata["email"] = params.Credential.Email
	}
	if params.Credential.AccountID != "" {
		metadata["accountId"] = params.Credential.AccountID
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: nowMs, Limits: limits, Metadata: metadata, Raw: data}
}

// descriptorLabel returns a descriptor's label, or "".
func descriptorLabel(descriptor *antigravityWindowDescriptor) string {
	if descriptor == nil {
		return ""
	}
	return descriptor.label
}

// antigravityFractionKey sorts ascending by remaining fraction (unknown last
// within the same value).
func antigravityFractionKey(limit UsageLimit) float64 {
	if limit.Amount.RemainingFraction != nil {
		return *limit.Amount.RemainingFraction
	}
	return 1
}

// antigravityLegacyDeduped accumulates the merged per-counter quota entry.
type antigravityLegacyDeduped struct {
	amount     UsageAmount
	window     *UsageWindow
	tier       string
	tierKey    string
	windowID   string
	counterKey string
}

// antigravityBuildLegacyReport renders `fetchAvailableModels`.
func antigravityBuildLegacyReport(data map[string]any, params UsageFetchParams, endpoint string, nowMs int64) *UsageReport {
	deduped := map[string]*antigravityLegacyDeduped{}
	order := make([]string, 0, 6)
	models, _ := usageRecord(data["models"])
	keys := make([]string, 0, len(models))
	for modelID := range models {
		keys = append(keys, modelID)
	}
	sort.Strings(keys)
	for _, modelID := range keys {
		modelInfo, ok := usageRecord(models[modelID])
		if !ok {
			continue
		}
		quotaInfos := antigravityNormalizeQuotaInfos(modelInfo)
		descriptors := antigravityInferWindowDescriptors(quotaInfos, nowMs)
		for index, info := range quotaInfos {
			amount := antigravityBuildAmount(info)
			window := antigravityParseWindow(info, descriptors[index])
			tierKey := "default"
			if info.tier != "" {
				tierKey = strings.ToLower(info.tier)
			}
			counterName := antigravityCounterName(info)
			counterKey := "default"
			if counterName != "" {
				counterKey = strings.ToLower(counterName)
			}
			windowID := "default"
			if window != nil {
				windowID = window.ID
			}
			if windowID == "" {
				windowID = info.windowID
			}
			if windowID == "" {
				windowID = "default"
			}
			key := counterKey + "|" + tierKey + "|" + windowID
			existing, seen := deduped[key]
			if !seen {
				deduped[key] = &antigravityLegacyDeduped{
					amount:     amount,
					window:     window,
					tier:       info.tier,
					tierKey:    tierKey,
					windowID:   windowID,
					counterKey: counterKey,
				}
				order = append(order, key)
				continue
			}
			bestAmount := existing.amount
			bestWindow := existing.window
			if bestWindow == nil || bestWindow.ResetsAt == nil {
				if window != nil {
					bestWindow = window
				}
			}
			bestTier := existing.tier
			if bestTier == "" {
				bestTier = info.tier
			}
			existingFraction := existing.amount.RemainingFraction
			currentFraction := amount.RemainingFraction
			switch {
			case existingFraction == nil && currentFraction != nil:
				bestAmount = amount
				if info.tier != "" {
					bestTier = info.tier
				}
			case existingFraction != nil && currentFraction != nil && *currentFraction < *existingFraction:
				bestAmount = amount
				if info.tier != "" {
					bestTier = info.tier
				}
			}
			if bestWindow != nil && bestWindow.ResetsAt == nil && window != nil && window.ResetsAt != nil {
				bestWindow = window
			}
			existing.amount = bestAmount
			existing.window = bestWindow
			existing.tier = bestTier
		}
	}
	// A counter with any windowed entry keeps only its windowed entries, so an
	// unmetered autocomplete entry cannot surface as a phantom window.
	metered := map[string]bool{}
	for _, entry := range deduped {
		if entry.window != nil && entry.window.ResetsAt != nil {
			metered[entry.counterKey+"|"+entry.tierKey] = true
		}
	}
	limits := make([]UsageLimit, 0, len(order))
	for _, key := range order {
		entry := deduped[key]
		if (entry.window == nil || entry.window.ResetsAt == nil) && metered[entry.counterKey+"|"+entry.tierKey] {
			continue
		}
		label := "Usage"
		if name := antigravitySummaryCounterName(entry.counterKey); name != "" {
			label = "Usage (" + name + ")"
		}
		limits = append(limits, UsageLimit{
			ID:    params.Provider + ":" + entry.counterKey + ":" + entry.tierKey + ":" + entry.windowID,
			Label: label,
			Scope: UsageScope{
				Provider:  params.Provider,
				AccountID: params.Credential.AccountID,
				ProjectID: params.Credential.ProjectID,
				Tier:      entry.tier,
				WindowID:  entry.windowID,
			},
			Window: entry.window,
			Amount: entry.amount,
			Status: antigravityGetUsageStatus(entry.amount.RemainingFraction),
		})
	}
	sort.SliceStable(limits, func(i, j int) bool {
		return antigravityFractionKey(limits[i]) < antigravityFractionKey(limits[j])
	})
	metadata := map[string]any{"endpoint": endpoint}
	if params.Credential.ProjectID != "" {
		metadata["projectId"] = params.Credential.ProjectID
	}
	if params.Credential.Email != "" {
		metadata["email"] = params.Credential.Email
	}
	if params.Credential.AccountID != "" {
		metadata["accountId"] = params.Credential.AccountID
	}
	return &UsageReport{Provider: params.Provider, FetchedAt: nowMs, Limits: limits, Metadata: metadata, Raw: data}
}

// fetchAntigravityUsage probes the Antigravity quota summary, falling back to
// the legacy per-model catalog for older deployments.
func fetchAntigravityUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	credential := params.Credential
	if params.Provider != "google-antigravity" || credential.ProjectID == "" {
		return nil, nil
	}
	nowMs := timeNowMillis()
	accessToken := credential.AccessToken
	if accessToken == "" {
		return nil, nil
	}
	if credential.ExpiresAt != nil && *credential.ExpiresAt <= nowMs {
		return nil, nil
	}
	baseURL := usageTrimTrailingSlashes(trimJSWhitespace(params.BaseURL))
	endpoints := []string{antigravityDefaultEndpoint, antigravitySandboxEndpoint}
	if baseURL != "" {
		endpoints = []string{baseURL}
	}
	// The quota summary is what Antigravity's own quota UI uses: it reports both
	// the rolling five-hour and weekly buckets even when neither is exhausted.
	summaryResponse, summaryEndpoint, err := antigravityFetchEndpoint(ctx, fctx, endpoints, antigravityQuotaSummaryPath, credential.ProjectID, accessToken)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, nil
	}
	if summaryResponse != nil && summaryResponse.OK() {
		if payload, ok := summaryResponse.JSON(); ok {
			if record, ok := usageRecord(payload); ok {
				if report := antigravityBuildQuotaSummaryReport(record, params, summaryEndpoint, nowMs); report != nil {
					return report, nil
				}
			}
		}
	}
	legacyResponse, legacyEndpoint, err := antigravityFetchEndpoint(ctx, fctx, endpoints, antigravityAvailableModelsPath, credential.ProjectID, accessToken)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, nil
	}
	if legacyResponse == nil || !legacyResponse.OK() {
		usageLogWarn(fctx, "Antigravity usage fetch failed", nil)
		return nil, nil
	}
	payload, ok := legacyResponse.JSON()
	if !ok {
		return nil, nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil
	}
	return antigravityBuildLegacyReport(record, params, legacyEndpoint, nowMs), nil
}

// antigravityUsageProvider returns the Antigravity usage provider.
func antigravityUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID:           "google-antigravity",
		CacheVersion: 2,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "google-antigravity"
		},
		FetchUsage: fetchAntigravityUsage,
	}
}

// lookupEnvValue reads a process environment variable.
func lookupEnvValue(name string) string {
	return osGetenv(name)
}

// strconvInt parses an integer, reporting !ok on failure.
func strconvInt(text string) (int, bool) {
	value, err := strconv.Atoi(text)
	return value, err == nil
}
