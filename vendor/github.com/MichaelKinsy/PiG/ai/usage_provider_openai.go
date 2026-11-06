package ai

// Ports packages/ai/src/usage/ollama.ts, usage/opencode-go.ts,
// usage/github-copilot.ts, usage/synthetic.ts and usage/charm-hyper.ts.

import (
	"context"
	"math"
	"strconv"
	"strings"
)

// --- Ollama -----------------------------------------------------------------

// fetchOllamaUsage reports that Ollama exposes no standalone quota endpoint.
func fetchOllamaUsage(params UsageFetchParams) *UsageReport {
	if params.Provider != "ollama" && params.Provider != "ollama-cloud" {
		return nil
	}
	metadata := map[string]any{}
	if params.Credential.Email != "" {
		metadata["email"] = params.Credential.Email
	}
	if params.Credential.AccountID != "" {
		metadata["accountId"] = params.Credential.AccountID
	}
	if params.Credential.ProjectID != "" {
		metadata["projectId"] = params.Credential.ProjectID
	}
	report := &UsageReport{
		Provider:  params.Provider,
		FetchedAt: timeNowMillis(),
		Limits:    []UsageLimit{},
		Notes:     []string{"Ollama does not expose a standalone quota usage API; per-response token usage is reported during requests."},
	}
	if len(metadata) > 0 {
		report.Metadata = metadata
	}
	return report
}

// ollamaUsageProvider registers local Ollama accounts with usage views.
func ollamaUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "ollama",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "ollama"
		},
		FetchUsage: func(_ context.Context, params UsageFetchParams, _ UsageFetchContext) (*UsageReport, error) {
			return fetchOllamaUsage(params), nil
		},
	}
}

// ollamaCloudUsageProvider registers Ollama Cloud accounts until a quota
// endpoint exists.
func ollamaCloudUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "ollama-cloud",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "ollama-cloud"
		},
		FetchUsage: func(_ context.Context, params UsageFetchParams, _ UsageFetchContext) (*UsageReport, error) {
			return fetchOllamaUsage(params), nil
		},
	}
}

// --- OpenCode Go ------------------------------------------------------------

const (
	openCodeGoDefaultEndpoint = "https://opencode.ai/zen/go"
	openCodeGoModelEndpoint   = "https://opencode.ai/inference/go"
	openCodeGoUsagePath       = "/v1/usage"
)

// openCodeGoWindow describes one account window.
type openCodeGoWindow struct {
	key      string
	limitID  string
	windowID string
	label    string
	duration *int64
}

// openCodeGoWindows are the three plan windows in payload order.
var openCodeGoWindows = []openCodeGoWindow{
	{key: "rolling", limitID: "rolling-5h", windowID: "5h", label: "5 Hour", duration: new(5 * usageHourMs)},
	{key: "weekly", limitID: "weekly", windowID: "7d", label: "Weekly", duration: new(usageWeekMs)},
	// The monthly window anchors on the subscription anniversary, not a 30-day
	// rolling span, so it carries no duration.
	{key: "monthly", limitID: "monthly", windowID: "monthly", label: "Monthly"},
}

// normalizeOpenCodeGoBaseURL keeps a gateway prefix but maps first-party model
// URLs back to the Zen account endpoint.
func normalizeOpenCodeGoBaseURL(baseURL string) string {
	trimmed := trimJSWhitespace(baseURL)
	if trimmed == "" {
		return openCodeGoDefaultEndpoint
	}
	normalized := usageTrimTrailingSlashes(trimmed)
	if strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		normalized = normalized[:len(normalized)-len("/v1")]
	}
	if normalized == openCodeGoModelEndpoint || normalized == "" {
		return openCodeGoDefaultEndpoint
	}
	return normalized
}

// resolveOpenCodeGoStatus maps a window verdict to a status.
func resolveOpenCodeGoStatus(windowStatus string, usedFraction float64) UsageStatus {
	if windowStatus == "rate-limited" {
		return UsageStatusExhausted
	}
	if usedFraction >= 1 {
		return UsageStatusExhausted
	}
	if usedFraction >= 0.8 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// buildOpenCodeGoWindowLimit renders one window.
func buildOpenCodeGoWindowLimit(descriptor openCodeGoWindow, payload any) *UsageLimit {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	percent, hasPercent := usageNumber(record["percent"])
	status, hasStatus := usageString(record, "status")
	if !hasPercent || percent < 0 || percent > 100 || !hasStatus || (status != "ok" && status != "rate-limited") {
		return nil
	}
	resetText, ok := usageString(record, "resetsAt")
	if !ok {
		return nil
	}
	resetsAt := DateParse(resetText)
	if math.IsNaN(resetsAt) || math.IsInf(resetsAt, 0) {
		return nil
	}
	usedFraction := percent / 100
	remainingFraction := math.Max(0, 1-usedFraction)
	window := &UsageWindow{ID: descriptor.windowID, Label: descriptor.label, DurationMs: descriptor.duration, ResetsAt: new(int64(resetsAt))}
	return &UsageLimit{
		ID:     descriptor.limitID,
		Label:  descriptor.label + " limit",
		Scope:  UsageScope{Provider: "opencode-go", WindowID: descriptor.windowID, Shared: true},
		Window: window,
		Amount: UsageAmount{
			Used:              &percent,
			UsedFraction:      &usedFraction,
			RemainingFraction: &remainingFraction,
			Unit:              UsageUnitPercent,
		},
		Status: resolveOpenCodeGoStatus(status, usedFraction),
	}
}

// readOpenCodeGoErrorMessage reads an `error.message` envelope.
func readOpenCodeGoErrorMessage(response *usageResponse) string {
	payload, ok := response.JSON()
	if !ok {
		return ""
	}
	record, ok := usageRecord(payload)
	if !ok {
		return ""
	}
	errorRecord, ok := usageRecord(record["error"])
	if !ok {
		return ""
	}
	message, _ := usageString(errorRecord, "message")
	return message
}

// fetchOpenCodeGoUsage probes `/v1/usage` on the Zen account endpoint.
func fetchOpenCodeGoUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "opencode-go" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	endpoint := normalizeOpenCodeGoBaseURL(params.BaseURL) + openCodeGoUsagePath
	headers := map[string]string{
		"accept":             "application/json",
		"authorization":      "Bearer " + credential.APIKey,
		"User-Agent":         usageUserAgent(),
		"x-opencode-session": usageInstallID(),
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "OpenCode Go usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		// 401 (invalid key) and 403 (no Go subscription) must throw so credential
		// health flags the key; any other status serves the last good report.
		if response.Status == 401 || response.Status == 403 {
			detail := readOpenCodeGoErrorMessage(response)
			message := "OpenCode Go usage endpoint returned " + usageFormatCount(int64(response.Status))
			if detail != "" {
				message += ": " + detail
			}
			return nil, NewUsageHTTPError(message, response.Status)
		}
		usageLogWarn(fctx, "OpenCode Go usage fetch failed", map[string]any{"status": response.Status})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		usageLogWarn(fctx, "OpenCode Go usage response had no usage object", nil)
		return nil, nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, nil
	}
	usage, ok := usageRecord(record["usage"])
	if !ok {
		usageLogWarn(fctx, "OpenCode Go usage response had no usage object", nil)
		return nil, nil
	}
	limits := make([]UsageLimit, 0, len(openCodeGoWindows))
	for _, descriptor := range openCodeGoWindows {
		if limit := buildOpenCodeGoWindowLimit(descriptor, usage[descriptor.key]); limit != nil {
			limits = append(limits, *limit)
		}
	}
	// All-or-nothing: a partial report would overwrite the complete last-good
	// report in the cache, dropping the windows ranking and display rely on.
	if len(limits) != len(openCodeGoWindows) {
		usageLogWarn(fctx, "OpenCode Go usage response missing or malformed windows", nil)
		return nil, nil
	}
	return &UsageReport{
		Provider:  "opencode-go",
		FetchedAt: timeNowMillis(),
		Limits:    limits,
		Metadata:  map[string]any{"planType": "OpenCode Go", "endpoint": endpoint},
		Raw:       payload,
	}, nil
}

// openCodeGoUsageProvider returns the OpenCode Go usage provider.
func openCodeGoUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "opencode-go",
		// v2 retires reports from the OMP-observed spend estimator.
		CacheVersion: 2,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "opencode-go" && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage:           fetchOpenCodeGoUsage,
		ValidatesCredentials: true,
	}
}

// --- GitHub Copilot ---------------------------------------------------------

const copilotCLIUserAgent = "copilot/1.0.82"

// copilotQuotaDetail is one `quota_snapshots` entry.
type copilotQuotaDetail struct {
	entitlement      float64
	overageCount     float64
	overagePermitted bool
	percentRemaining float64
	quotaID          string
	quotaRemaining   float64
	remaining        float64
	unlimited        bool
}

// resolveCopilotGitHubAPIBaseURL picks the GitHub API host for a credential.
func resolveCopilotGitHubAPIBaseURL(params UsageFetchParams) string {
	baseURL := usageTrimTrailingSlashes(params.BaseURL)
	if baseURL != "" && !strings.Contains(baseURL, "githubcopilot.com") {
		return baseURL
	}
	enterprise := trimJSWhitespace(params.Credential.EnterpriseURL)
	if enterprise == "" {
		return "https://api.github.com"
	}
	lower := strings.ToLower(enterprise)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return usageTrimTrailingSlashes(enterprise)
	case strings.HasPrefix(lower, "api."):
		return "https://" + enterprise
	}
	return "https://api." + enterprise
}

// buildCopilotWindow renders the monthly reset window.
func buildCopilotWindow(resetDate string) *UsageWindow {
	if resetDate == "" {
		return nil
	}
	resetAt, ok := usageIsoTimestamp(resetDate)
	if !ok {
		return nil
	}
	return &UsageWindow{ID: "monthly", Label: "Monthly", ResetsAt: &resetAt}
}

// buildCopilotAmount renders an amount from counters.
func buildCopilotAmount(used, limit *float64, unit UsageUnit) UsageAmount {
	amount := UsageAmount{Unit: unit}
	if limit != nil {
		amount.Limit = limit
	}
	if used != nil {
		amount.Used = used
	}
	if used != nil && limit != nil {
		remaining := math.Max(0, *limit-*used)
		amount.Remaining = &remaining
		if *limit > 0 {
			fraction := *used / *limit
			remainingFraction := remaining / *limit
			amount.UsedFraction = &fraction
			amount.RemainingFraction = &remainingFraction
		}
	}
	return amount
}

// deriveCopilotStatus maps remaining headroom to a status.
func deriveCopilotStatus(amount UsageAmount, unlimited bool) UsageStatus {
	if unlimited {
		return UsageStatusOK
	}
	if amount.RemainingFraction == nil {
		return UsageStatusUnknown
	}
	if *amount.RemainingFraction <= 0 {
		return UsageStatusExhausted
	}
	if *amount.RemainingFraction <= 0.1 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// parseCopilotQuotaDetail reads one quota snapshot.
func parseCopilotQuotaDetail(value any) *copilotQuotaDetail {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	entitlement, hasEntitlement := usageNumber(record["entitlement"])
	remaining, hasRemaining := usageNumber(record["remaining"])
	percentRemaining, hasPercent := usageNumber(record["percent_remaining"])
	unlimited, hasUnlimited := usageBool(record, "unlimited")
	if !hasEntitlement || !hasRemaining || !hasPercent || !hasUnlimited {
		return nil
	}
	detail := &copilotQuotaDetail{
		entitlement:      entitlement,
		remaining:        remaining,
		percentRemaining: percentRemaining,
		unlimited:        unlimited,
		quotaRemaining:   remaining,
	}
	if value, ok := usageNumber(record["overage_count"]); ok {
		detail.overageCount = value
	}
	if value, ok := usageBool(record, "overage_permitted"); ok {
		detail.overagePermitted = value
	}
	if value, ok := usageString(record, "quota_id"); ok {
		detail.quotaID = value
	}
	if value, ok := usageNumber(record["quota_remaining"]); ok {
		detail.quotaRemaining = value
	}
	return detail
}

// buildCopilotLimitFromQuota renders one snapshot as a limit.
func buildCopilotLimitFromQuota(key, label string, quota *copilotQuotaDetail, plan string, window *UsageWindow, accountID string) UsageLimit {
	var used, capLimit *float64
	if !quota.unlimited {
		usedValue := math.Max(0, quota.entitlement-quota.remaining)
		used = &usedValue
		limitValue := quota.entitlement
		capLimit = &limitValue
	}
	amount := buildCopilotAmount(used, capLimit, UsageUnitRequests)
	notes := make([]string, 0, 2)
	if quota.unlimited {
		notes = append(notes, "Unlimited")
	}
	if quota.overageCount > 0 {
		notes = append(notes, "Overage requests: "+usageFormatFloat(quota.overageCount))
	}
	result := UsageLimit{
		ID:     "copilot:" + key,
		Label:  label,
		Scope:  UsageScope{Provider: "github-copilot", AccountID: accountID, Tier: plan},
		Window: window,
		Amount: amount,
		Status: deriveCopilotStatus(amount, quota.unlimited),
	}
	if window != nil {
		result.Scope.WindowID = window.ID
	}
	if len(notes) > 0 {
		result.Notes = notes
	}
	return result
}

// normalizeCopilotQuotaSnapshots renders a `/copilot_internal/user` body.
func normalizeCopilotQuotaSnapshots(record map[string]any, accountID string) []UsageLimit {
	resetDate, _ := usageString(record, "quota_reset_date")
	plan, _ := usageString(record, "copilot_plan")
	window := buildCopilotWindow(resetDate)
	snapshots, _ := usageRecord(record["quota_snapshots"])
	limits := make([]UsageLimit, 0, 3)
	if premium := parseCopilotQuotaDetail(snapshots["premium_interactions"]); premium != nil {
		limits = append(limits, buildCopilotLimitFromQuota("premium", "Premium Requests", premium, plan, window, accountID))
	}
	if chat := parseCopilotQuotaDetail(snapshots["chat"]); chat != nil && !chat.unlimited {
		limits = append(limits, buildCopilotLimitFromQuota("chat", "Chat Requests", chat, plan, window, accountID))
	}
	if completions := parseCopilotQuotaDetail(snapshots["completions"]); completions != nil && !completions.unlimited {
		limits = append(limits, buildCopilotLimitFromQuota("completions", "Completions", completions, plan, window, accountID))
	}
	return limits
}

// resolveCopilotBillingUnit names a billing item's unit and label.
func resolveCopilotBillingUnit(unitType string) (UsageUnit, string) {
	if unitType == "" {
		return UsageUnitRequests, "Premium Requests"
	}
	upper := strings.ToUpper(unitType)
	filtered := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, upper)
	if strings.Contains(filtered, "CREDIT") {
		return UsageUnitCredits, "AI Credits"
	}
	return UsageUnitRequests, "Premium Requests"
}

// normalizeCopilotBillingUsage renders the premium-request billing report.
func normalizeCopilotBillingUsage(record map[string]any) []UsageLimit {
	timePeriod, _ := usageRecord(record["timePeriod"])
	year := int64(0)
	if value, ok := usageNumber(timePeriod["year"]); ok {
		year = int64(value)
	}
	periodLabel := usageFormatCount(year)
	if month, ok := usageNumber(timePeriod["month"]); ok && month != 0 {
		periodLabel = usageFormatCount(year) + "-" + leftPadTwo(int(month))
	}
	window := &UsageWindow{ID: "monthly", Label: periodLabel}
	user, _ := usageString(record, "user")

	items := usageObjectArray(record, "usageItems")
	premium := make([]map[string]any, 0, len(items))
	for _, item := range items {
		sku, _ := usageString(item, "sku")
		if sku == "Copilot Premium Request" || strings.Contains(sku, "Premium") {
			premium = append(premium, item)
		}
	}
	unit := UsageUnitRequests
	label := "Premium Requests"
	for _, item := range premium {
		if _, present := item["unitType"]; !present {
			continue
		}
		unitType, _ := usageString(item, "unitType")
		unit, label = resolveCopilotBillingUnit(unitType)
		break
	}
	totalUsed := 0.0
	totalLimit := 0.0
	hasLimit := false
	for _, item := range premium {
		if gross, ok := usageNumber(item["grossQuantity"]); ok {
			totalUsed += gross
		}
		if limit, ok := usageNumber(item["limit"]); ok {
			totalLimit += limit
			hasLimit = true
		}
	}
	var limitPtr *float64
	if hasLimit && totalLimit != 0 {
		limitPtr = &totalLimit
	}
	usedValue := totalUsed
	totalAmount := buildCopilotAmount(&usedValue, limitPtr, unit)
	limits := []UsageLimit{{
		ID:     "copilot:premium",
		Label:  label,
		Scope:  UsageScope{Provider: "github-copilot", AccountID: user, WindowID: window.ID},
		Window: window,
		Amount: totalAmount,
		Status: deriveCopilotStatus(totalAmount, false),
	}}
	for _, item := range items {
		model, _ := usageString(item, "model")
		if model == "" {
			continue
		}
		gross, ok := usageNumber(item["grossQuantity"])
		if !ok || gross <= 0 {
			continue
		}
		var itemLimit *float64
		itemUnit := UsageUnitRequests
		if limit, ok := usageNumber(item["limit"]); ok {
			itemLimit = &limit
		}
		if unitType, ok := usageString(item, "unitType"); ok {
			itemUnit, _ = resolveCopilotBillingUnit(unitType)
		}
		amount := buildCopilotAmount(&gross, itemLimit, itemUnit)
		limits = append(limits, UsageLimit{
			ID:     "copilot:model:" + model,
			Label:  "Model " + model,
			Scope:  UsageScope{Provider: "github-copilot", AccountID: user, ModelID: model, WindowID: window.ID},
			Window: window,
			Amount: amount,
			Status: deriveCopilotStatus(amount, false),
		})
	}
	return limits
}

// leftPadTwo renders a month number as two digits.
func leftPadTwo(value int) string {
	text := usageFormatCount(int64(value))
	if len(text) < 2 {
		return "0" + text
	}
	return text
}

// copilotUsageFetchJSON performs one GitHub request and decodes its body.
func copilotUsageFetchJSON(ctx context.Context, fctx UsageFetchContext, endpoint string, headers map[string]string) (any, error) {
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		return nil, err
	}
	if !response.OK() {
		return nil, NewUsageHTTPError(usageFormatCount(int64(response.Status))+" "+usageStatusText(response)+": "+response.Text(), response.Status)
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, usageErrorf("invalid JSON response")
	}
	return payload, nil
}

// resolveCopilotUsername resolves the token owner's login.
func resolveCopilotUsername(ctx context.Context, fctx UsageFetchContext, baseURL, token string) (string, error) {
	payload, err := copilotUsageFetchJSON(ctx, fctx, usageAbsoluteURL(baseURL, "user"), map[string]string{
		"Accept":               "application/vnd.github+json",
		"Authorization":        "Bearer " + token,
		"X-GitHub-Api-Version": "2022-11-28",
	})
	if err != nil {
		return "", err
	}
	record, ok := usageRecord(payload)
	if !ok {
		return "", NewUsageHTTPError("Invalid GitHub user response", 200)
	}
	login, _ := usageString(record, "login")
	return login, nil
}

// fetchCopilotInternalUsage reads `/copilot_internal/user`.
func fetchCopilotInternalUsage(ctx context.Context, fctx UsageFetchContext, baseURL, token string) (map[string]any, error) {
	payload, err := copilotUsageFetchJSON(ctx, fctx, usageAbsoluteURL(baseURL, "copilot_internal/user"), map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
		"User-Agent":    copilotCLIUserAgent,
	})
	if err != nil {
		return nil, err
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, NewUsageHTTPError("Invalid Copilot usage response", 200)
	}
	return record, nil
}

// fetchCopilotBillingUsage reads the premium-request billing report.
func fetchCopilotBillingUsage(ctx context.Context, fctx UsageFetchContext, baseURL, username, token string) (map[string]any, error) {
	payload, err := copilotUsageFetchJSON(ctx, fctx, usageAbsoluteURL(baseURL, "users/"+username+"/settings/billing/premium_request/usage"), map[string]string{
		"Accept":               "application/vnd.github+json",
		"Authorization":        "Bearer " + token,
		"X-GitHub-Api-Version": "2022-11-28",
	})
	if err != nil {
		return nil, err
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, NewUsageHTTPError("Invalid Copilot billing usage response", 200)
	}
	return record, nil
}

// rethrowCopilotAuthRejection rethrows when every endpoint rejected the token.
func rethrowCopilotAuthRejection(failures []error) error {
	if len(failures) == 0 {
		return nil
	}
	for _, failure := range failures {
		httpErr, ok := failure.(*UsageHTTPError)
		if !ok || (httpErr.Status != 401 && httpErr.Status != 403) {
			return nil
		}
	}
	return failures[len(failures)-1]
}

// fetchGitHubCopilotUsage probes the Copilot quota and billing endpoints.
func fetchGitHubCopilotUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	provider := githubCopilotUsageProvider()
	if !provider.SupportsCredential(params) {
		return nil, nil
	}
	apiBase := resolveCopilotGitHubAPIBaseURL(params)
	var report *UsageReport
	failures := make([]error, 0, 3)

	if params.Credential.Type == CredentialAPIKey {
		username := ""
		if candidate := params.Credential.AccountID; candidate != "" {
			username = candidate
		} else if candidate, ok := params.Credential.Metadata["username"].(string); ok && trimJSWhitespace(candidate) != "" {
			username = trimJSWhitespace(candidate)
		} else if candidate, ok := params.Credential.Metadata["user"].(string); ok && trimJSWhitespace(candidate) != "" {
			username = trimJSWhitespace(candidate)
		}
		if username == "" && params.Credential.APIKey != "" {
			resolved, err := resolveCopilotUsername(ctx, fctx, apiBase, params.Credential.APIKey)
			if err != nil {
				// A failed lookup skips billing, so it votes in the auth consensus:
				// a transient `/user` failure must keep the cached report.
				failures = append(failures, err)
			} else {
				username = resolved
			}
		}
		if username == "" {
			usageLogWarn(fctx, "Copilot usage requires username for billing API", map[string]any{"provider": params.Provider})
		} else if params.Credential.APIKey != "" {
			billing, err := fetchCopilotBillingUsage(ctx, fctx, apiBase, username, params.Credential.APIKey)
			if err != nil {
				failures = append(failures, err)
				usageLogWarn(fctx, "Copilot usage fetch failed", map[string]any{"error": err.Error()})
			} else {
				report = &UsageReport{
					Provider:  "github-copilot",
					FetchedAt: timeNowMillis(),
					Limits:    normalizeCopilotBillingUsage(billing),
					Metadata:  map[string]any{"accountId": username, "account": username, "period": billing["timePeriod"]},
				}
			}
		}
		if report == nil && params.Credential.APIKey != "" {
			usage, err := fetchCopilotInternalUsage(ctx, fctx, apiBase, params.Credential.APIKey)
			if err != nil {
				failures = append(failures, err)
				usageLogWarn(fctx, "Copilot usage fetch failed", map[string]any{"error": err.Error()})
			} else {
				plan, _ := usageString(usage, "copilot_plan")
				resetDate, _ := usageString(usage, "quota_reset_date")
				report = &UsageReport{
					Provider:  "github-copilot",
					FetchedAt: timeNowMillis(),
					Limits:    normalizeCopilotQuotaSnapshots(usage, username),
					Metadata:  map[string]any{"accountId": username, "plan": plan, "quotaResetDate": resetDate},
					Raw:       usage,
				}
			}
		}
	} else {
		refreshToken := params.Credential.RefreshToken
		accessToken := params.Credential.AccessToken
		if refreshToken == "" && accessToken == "" {
			return nil, nil
		}
		githubToken := refreshToken
		if githubToken == "" {
			githubToken = accessToken
		}
		usage, err := fetchCopilotInternalUsage(ctx, fctx, apiBase, githubToken)
		if err != nil {
			failures = append(failures, err)
			usageLogWarn(fctx, "Copilot usage fetch failed", map[string]any{"error": err.Error()})
		} else {
			accountID := params.Credential.AccountID
			// The usage report already landed; the login only labels it.
			if accountID == "" && refreshToken != "" {
				if resolved, err := resolveCopilotUsername(ctx, fctx, apiBase, refreshToken); err == nil {
					accountID = resolved
				}
			}
			if accountID == "" && accessToken != "" {
				if resolved, err := resolveCopilotUsername(ctx, fctx, apiBase, accessToken); err == nil {
					accountID = resolved
				}
			}
			plan, _ := usageString(usage, "copilot_plan")
			resetDate, _ := usageString(usage, "quota_reset_date")
			metadata := map[string]any{"accountId": accountID, "plan": plan, "quotaResetDate": resetDate}
			if params.Credential.Email != "" {
				metadata["email"] = params.Credential.Email
			}
			report = &UsageReport{
				Provider:  "github-copilot",
				FetchedAt: timeNowMillis(),
				Limits:    normalizeCopilotQuotaSnapshots(usage, accountID),
				Metadata:  metadata,
				Raw:       usage,
			}
		}
	}
	if report == nil {
		if err := rethrowCopilotAuthRejection(failures); err != nil {
			return nil, err
		}
	}
	return report, nil
}

// githubCopilotUsageProvider returns the GitHub Copilot usage provider.
func githubCopilotUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "github-copilot",
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != "github-copilot" {
				return false
			}
			if params.Credential.Type == CredentialOAuth {
				return params.Credential.RefreshToken != "" || params.Credential.AccessToken != ""
			}
			return params.Credential.APIKey != ""
		},
		FetchUsage: fetchGitHubCopilotUsage,
	}
}

// --- Synthetic --------------------------------------------------------------

const syntheticQuotasURL = "https://api.synthetic.new/v2/quotas"

// parseSyntheticDollarAmount reads a "$1.23" string amount.
func parseSyntheticDollarAmount(value any) (float64, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	trimmed := strings.TrimPrefix(trimJSWhitespace(text), "$")
	parsed, err := parseFloatPrefix(trimmed)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

// parseSyntheticRollingFiveHourLimit renders `rollingFiveHourLimit`.
func parseSyntheticRollingFiveHourLimit(raw any, provider string) *UsageLimit {
	record, ok := usageRecord(raw)
	if !ok {
		return nil
	}
	remaining, hasRemaining := usageNumber(record["remaining"])
	max, hasMax := usageNumber(record["max"])
	limited, _ := usageBool(record, "limited")
	nextTickAt, hasTick := usageIsoTimestamp(record["nextTickAt"])
	tickPercent, hasTickPercent := usageNumber(record["tickPercent"])
	if !hasRemaining && !hasMax {
		return nil
	}
	var used *float64
	if hasMax && hasRemaining {
		value := max - remaining
		used = &value
	}
	label := "5h"
	if hasTickPercent {
		value := math.Round(tickPercent*100*100) / 100
		label = "5h · regen " + usageFormatFloat(value) + "%/tick"
	}
	window := &UsageWindow{ID: "5h", Label: label, DurationMs: new(5 * usageHourMs)}
	if hasTick {
		window.ResetsAt = &nextTickAt
		window.ResetLabel = "tick"
	}
	var remainingPtr, maxPtr *float64
	if hasRemaining {
		remainingPtr = &remaining
	}
	if hasMax {
		maxPtr = &max
	}
	amount := UsageBuildAmount(used, maxPtr, remainingPtr, nil, UsageUnitRequests)
	status := UsageStatusOK
	if limited {
		status = UsageStatusExhausted
	} else if amount.UsedFraction != nil {
		status = usageStatusFor(amount.UsedFraction)
	}
	return &UsageLimit{
		ID:     "synthetic:requests:5h",
		Label:  "Synthetic Requests",
		Scope:  UsageScope{Provider: provider, WindowID: "5h", Shared: true},
		Window: window,
		Amount: amount,
		Status: status,
	}
}

// parseSyntheticWeeklyTokenLimit renders `weeklyTokenLimit`.
func parseSyntheticWeeklyTokenLimit(raw any, provider string) *UsageLimit {
	record, ok := usageRecord(raw)
	if !ok {
		return nil
	}
	remainingCredits, hasRemaining := parseSyntheticDollarAmount(record["remainingCredits"])
	maxCredits, hasMax := parseSyntheticDollarAmount(record["maxCredits"])
	percentRemaining, hasPercent := usageNumber(record["percentRemaining"])
	nextRegenAt, hasRegen := usageIsoTimestamp(record["nextRegenAt"])
	if !hasRemaining && !hasMax {
		return nil
	}
	var usedFraction *float64
	if hasPercent {
		value := math.Min(math.Max(1-percentRemaining/100, 0), 1)
		usedFraction = &value
	}
	var used *float64
	if usedFraction != nil && hasMax {
		value := *usedFraction * maxCredits
		used = &value
	}
	label := "7d"
	if nextRegenCredits, ok := parseSyntheticDollarAmount(record["nextRegenCredits"]); ok {
		label = "7d · regen $" + usageFormatFixed(nextRegenCredits, 2) + "/tick"
	}
	window := &UsageWindow{ID: "7d", Label: label, DurationMs: new(usageWeekMs)}
	if hasRegen {
		window.ResetsAt = &nextRegenAt
		window.ResetLabel = "regen"
	}
	var remainingPtr, maxPtr *float64
	if hasRemaining {
		remainingPtr = &remainingCredits
	}
	if hasMax {
		maxPtr = &maxCredits
	}
	amount := UsageBuildAmount(used, maxPtr, remainingPtr, usedFraction, UsageUnitUSD)
	return &UsageLimit{
		ID:     "synthetic:usd:7d",
		Label:  "Synthetic Credits",
		Scope:  UsageScope{Provider: provider, WindowID: "7d", Shared: true},
		Window: window,
		Amount: amount,
		Status: usageStatusFor(amount.UsedFraction),
	}
}

// usageFormatFixed renders value with the given decimals, dropping trailing zeros.
func usageFormatFixed(value float64, digits int) string {
	text := strconv.FormatFloat(value, 'f', digits, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text
}

// fetchSyntheticUsage probes Synthetic's quota endpoint.
func fetchSyntheticUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "synthetic" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", syntheticQuotasURL, map[string]string{
		"Authorization": "Bearer " + credential.APIKey,
		"Content-Type":  "application/json",
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Synthetic usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		usageLogWarn(fctx, "Synthetic usage fetch failed", map[string]any{"status": response.Status})
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
	limits := make([]UsageLimit, 0, 2)
	if limit := parseSyntheticRollingFiveHourLimit(record["rollingFiveHourLimit"], params.Provider); limit != nil {
		limits = append(limits, *limit)
	}
	if limit := parseSyntheticWeeklyTokenLimit(record["weeklyTokenLimit"], params.Provider); limit != nil {
		limits = append(limits, *limit)
	}
	if len(limits) == 0 {
		return nil, nil
	}
	return &UsageReport{
		Provider:  params.Provider,
		FetchedAt: timeNowMillis(),
		Limits:    limits,
		Metadata:  map[string]any{"endpoint": syntheticQuotasURL},
		Raw:       payload,
	}, nil
}

// syntheticUsageProvider returns the Synthetic usage provider.
func syntheticUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "synthetic",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "synthetic" && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage: fetchSyntheticUsage,
	}
}

// --- Charm Hyper ------------------------------------------------------------

const charmHyperAPIBaseURL = "https://hyper.charm.land/v1"

// normalizeCharmHyperBaseURL resolves the gateway's `/v1` surface; blank means
// the canonical host.
func normalizeCharmHyperBaseURL(baseURL string) string {
	trimmed := usageTrimTrailingSlashes(trimJSWhitespace(baseURL))
	if trimmed == "" {
		return charmHyperAPIBaseURL
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}

// fetchCharmHyperUsage reads the prepaid credit balance.
func fetchCharmHyperUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "charm-hyper" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialAPIKey || credential.APIKey == "" {
		return nil, nil
	}
	// A configured proxy base is honored: sending the stored key to the
	// canonical host would fail for a proxy-scoped credential and disclose it.
	endpoint := normalizeCharmHyperBaseURL(params.BaseURL) + "/credits"
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, map[string]string{
		"Authorization": "Bearer " + credential.APIKey,
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Charm Hyper usage fetch error", map[string]any{"error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		// A revoked key must invalidate the cached balance rather than re-serve
		// the last-good report.
		if response.Status == 401 || response.Status == 403 {
			return nil, NewUsageHTTPError("Charm Hyper credits endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response), response.Status)
		}
		usageLogWarn(fctx, "Charm Hyper usage fetch failed", map[string]any{"status": response.Status})
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
	balance, ok := usageNumber(record["balance"])
	if !ok || math.IsNaN(balance) || math.IsInf(balance, 0) {
		return nil, nil
	}
	// The balance is account-wide, not per key, and the endpoint exposes no
	// account identity to group keys by, so the limit is marked shared.
	return &UsageReport{
		Provider:  params.Provider,
		FetchedAt: timeNowMillis(),
		Limits: []UsageLimit{{
			ID:     "charm-hyper:credits",
			Label:  "Credit balance",
			Scope:  UsageScope{Provider: params.Provider, WindowID: "balance", Shared: true},
			Amount: UsageAmount{Remaining: &balance, Unit: UsageUnitCredits},
		}},
		Metadata: map[string]any{"endpoint": endpoint},
		Raw:      payload,
	}, nil
}

// charmHyperUsageProvider returns the Charm Hyper usage provider.
func charmHyperUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "charm-hyper",
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "charm-hyper" && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage:           fetchCharmHyperUsage,
		ValidatesCredentials: true,
	}
}
