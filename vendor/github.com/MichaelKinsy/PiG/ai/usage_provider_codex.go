package ai

// Ports packages/ai/src/usage/openai-codex.ts, openai-codex-base-url.ts and
// openai-codex-reset.ts: the ChatGPT account usage endpoint, its saved
// rate-limit resets, and the `x-codex-*` response-header snapshot.

import (
	"context"
	"math"
	"strings"
)

const (
	codexDefaultBaseURL     = "https://chatgpt.com/backend-api"
	codexUsagePath          = "wham/usage"
	codexVerifiedAccessPath = "accounts/verified_access"
	codexResetCreditsPath   = "wham/rate-limit-reset-credits"
	codexJWTAuthClaim       = "https://api.openai.com/auth"
	codexJWTProfileClaim    = "https://api.openai.com/profile"
	codexSpendCapLimitID    = "openai-codex:spend-cap"
)

// normalizeCodexBaseURL resolves the account-API base URL. These endpoints live
// on the canonical ChatGPT origin behind the Codex bearer, so a streaming proxy
// override must not be used here; an accepted canonical override keeps its
// origin with `/backend-api` restored.
func normalizeCodexBaseURL(baseURL string) string {
	trimmed := usageTrimTrailingSlashes(trimJSWhitespace(baseURL))
	if trimmed == "" {
		return codexDefaultBaseURL
	}
	parsed, err := parseHTTPURL(trimmed)
	if err != nil {
		return codexDefaultBaseURL
	}
	host := strings.ToLower(parsed.Hostname)
	if host != "chatgpt.com" && host != "chat.openai.com" {
		return codexDefaultBaseURL
	}
	return parsed.origin() + "/backend-api"
}

// codexUsageWindow is one parsed `*_window` payload.
type codexUsageWindow struct {
	usedPercent        *float64
	limitWindowSeconds *int64
	resetAfterSeconds  *float64
	resetAt            *float64
}

// codexAdditionalUsage is one parsed `additional_rate_limits[]` entry.
type codexAdditionalUsage struct {
	limitName    string
	meteredName  string
	allowed      *bool
	limitReached *bool
	primary      *codexUsageWindow
	secondary    *codexUsageWindow
}

// codexSpendCap is the per-user credit cap from `spend_control.individual_limit`.
type codexSpendCap struct {
	used              *float64
	limit             *float64
	remaining         *float64
	usedFraction      *float64
	reached           *bool
	resetAt           *float64
	resetAfterSeconds *float64
}

// codexParsedUsage is the parsed `/wham/usage` payload.
type codexParsedUsage struct {
	planType      string
	allowed       *bool
	limitReached  *bool
	primary       *codexUsageWindow
	secondary     *codexUsageWindow
	additional    []codexAdditionalUsage
	spendCap      *codexSpendCap
	creditOverage bool
	raw           any
}

// parseCodexUsageWindow reads one window object.
func parseCodexUsageWindow(value any) *codexUsageWindow {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	window := &codexUsageWindow{}
	if number, ok := usageNumber(record["used_percent"]); ok {
		window.usedPercent = &number
	}
	if number, ok := usageNumber(record["limit_window_seconds"]); ok {
		seconds := int64(math.Trunc(number))
		window.limitWindowSeconds = &seconds
	}
	if number, ok := usageNumber(record["reset_after_seconds"]); ok {
		window.resetAfterSeconds = &number
	}
	if number, ok := usageNumber(record["reset_at"]); ok {
		window.resetAt = &number
	}
	if window.usedPercent == nil && window.limitWindowSeconds == nil && window.resetAfterSeconds == nil && window.resetAt == nil {
		return nil
	}
	return window
}

// parseCodexAdditionalUsage reads one additional meter entry.
func parseCodexAdditionalUsage(value any) *codexAdditionalUsage {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	rateLimit, ok := usageRecord(record["rate_limit"])
	if !ok {
		return nil
	}
	entry := &codexAdditionalUsage{
		primary:   parseCodexUsageWindow(rateLimit["primary_window"]),
		secondary: parseCodexUsageWindow(rateLimit["secondary_window"]),
	}
	if name, ok := usageString(record, "limit_name"); ok {
		entry.limitName = name
	}
	if name, ok := usageString(record, "metered_feature"); ok {
		entry.meteredName = name
	}
	if flag, ok := usageBool(rateLimit, "allowed"); ok {
		entry.allowed = &flag
	}
	if flag, ok := usageBool(rateLimit, "limit_reached"); ok {
		entry.limitReached = &flag
	}
	if entry.primary == nil && entry.secondary == nil && entry.allowed == nil && entry.limitReached == nil {
		return nil
	}
	return entry
}

// parseCodexSpendCap reads `spend_control.individual_limit`.
func parseCodexSpendCap(payload map[string]any) *codexSpendCap {
	spendControl, ok := usageRecord(payload["spend_control"])
	if !ok {
		return nil
	}
	individual, ok := usageRecord(spendControl["individual_limit"])
	if !ok {
		return nil
	}
	cap := &codexSpendCap{}
	used, hasUsed := usageNumber(individual["used"])
	limit, hasLimit := usageNumber(individual["limit"])
	remaining, hasRemaining := usageNumber(individual["remaining"])
	usedPercent, hasUsedPercent := usageNumber(individual["used_percent"])
	if hasUsed {
		cap.used = &used
	}
	if hasLimit {
		cap.limit = &limit
	}
	if hasRemaining {
		cap.remaining = &remaining
	}
	var fraction *float64
	switch {
	case hasUsed && hasLimit && limit > 0:
		value := used / limit
		fraction = &value
	case hasUsedPercent:
		value := usedPercent / 100
		fraction = &value
	case hasRemaining && remaining <= 0:
		value := 1.0
		fraction = &value
	}
	if hasUsed || hasLimit || hasRemaining || fraction != nil {
		if fraction != nil {
			clamped := math.Min(math.Max(*fraction, 0), 1)
			cap.usedFraction = &clamped
		}
		if flag, ok := usageBool(spendControl, "reached"); ok {
			cap.reached = &flag
		}
		if number, ok := usageNumber(individual["reset_at"]); ok {
			cap.resetAt = &number
		}
		if number, ok := usageNumber(individual["reset_after_seconds"]); ok {
			cap.resetAfterSeconds = &number
		}
		return cap
	}
	return nil
}

// hasCodexPlanCreditOverage reports whether paid credits can fund plan-window
// overage, which is what keeps a spent plan meter serving requests.
func hasCodexPlanCreditOverage(payload map[string]any, planLimitReached *bool, spendCap *codexSpendCap) bool {
	if planLimitReached == nil || !*planLimitReached {
		return false
	}
	credits, ok := usageRecord(payload["credits"])
	if !ok {
		return false
	}
	if unlimited, ok := usageBool(credits, "unlimited"); !ok || !unlimited {
		if hasCredits, ok := usageBool(credits, "has_credits"); !ok || !hasCredits {
			return false
		}
	}
	if overage, ok := usageBool(credits, "overage_limit_reached"); ok && overage {
		return false
	}
	if spendControl, ok := usageRecord(payload["spend_control"]); ok {
		if reached, ok := usageBool(spendControl, "reached"); ok && reached {
			return false
		}
	}
	if spendCap != nil && spendCap.reached == nil && spendCap.usedFraction != nil && *spendCap.usedFraction >= 1 {
		return false
	}
	return true
}

// parseCodexUsagePayload reads the plan usage payload.
func parseCodexUsagePayload(payload any) *codexParsedUsage {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	parsed := &codexParsedUsage{raw: payload}
	if planType, ok := usageString(record, "plan_type"); ok {
		parsed.planType = planType
	}
	rateLimit, hasRateLimit := usageRecord(record["rate_limit"])
	for _, entry := range usageObjectArray(record, "additional_rate_limits") {
		if additional := parseCodexAdditionalUsage(entry); additional != nil {
			parsed.additional = append(parsed.additional, *additional)
		}
	}
	parsed.spendCap = parseCodexSpendCap(record)
	if !hasRateLimit && len(parsed.additional) == 0 && parsed.spendCap == nil {
		return nil
	}
	var planLimitReached *bool
	if hasRateLimit {
		if flag, ok := usageBool(rateLimit, "limit_reached"); ok {
			planLimitReached = &flag
		}
		parsed.primary = parseCodexUsageWindow(rateLimit["primary_window"])
		parsed.secondary = parseCodexUsageWindow(rateLimit["secondary_window"])
		if flag, ok := usageBool(rateLimit, "allowed"); ok {
			parsed.allowed = &flag
		}
	} else if parsed.spendCap != nil && parsed.spendCap.reached != nil {
		allowed := !*parsed.spendCap.reached
		parsed.allowed = &allowed
	}
	if hasRateLimit {
		parsed.limitReached = planLimitReached
	} else if parsed.spendCap != nil {
		parsed.limitReached = parsed.spendCap.reached
	}
	parsed.creditOverage = hasCodexPlanCreditOverage(record, planLimitReached, parsed.spendCap)
	if parsed.primary == nil && parsed.secondary == nil && parsed.allowed == nil && parsed.limitReached == nil &&
		len(parsed.additional) == 0 && parsed.spendCap == nil {
		return nil
	}
	return parsed
}

// codexBuildWindowLabel returns the id/label pair for a window span.
func codexBuildWindow(seconds int64) (string, string) {
	return usageWindowFromSeconds(seconds)
}

// codexResolveResetTime resolves a window's absolute reset time.
func codexResolveResetTime(window *codexUsageWindow, nowMs int64) *int64 {
	if window == nil {
		return nil
	}
	if window.resetAt != nil {
		value := *window.resetAt
		millis := value
		if value <= 1_000_000_000_000 {
			millis = value * 1000
		}
		if !math.IsNaN(millis) && !math.IsInf(millis, 0) {
			converted := int64(millis)
			return &converted
		}
	}
	if window.resetAfterSeconds != nil {
		value := nowMs + int64(*window.resetAfterSeconds*1000)
		return &value
	}
	return nil
}

// codexBuildUsageWindow renders a parsed window as a UsageWindow.
func codexBuildUsageWindow(window *codexUsageWindow, key string, nowMs int64) UsageWindow {
	resetsAt := codexResolveResetTime(window, nowMs)
	if window != nil && window.limitWindowSeconds != nil {
		id, label := codexBuildWindow(*window.limitWindowSeconds)
		duration := *window.limitWindowSeconds * 1000
		return UsageWindow{ID: id, Label: label, DurationMs: &duration, ResetsAt: resetsAt}
	}
	label := "Secondary window"
	if key == "primary" {
		label = "Primary window"
	}
	return UsageWindow{ID: key, Label: label, ResetsAt: resetsAt}
}

// codexBuildUsageAmount renders a window's percent amount.
func codexBuildUsageAmount(window *codexUsageWindow) UsageAmount {
	if window == nil || window.usedPercent == nil {
		return UsageAmount{Unit: UsageUnitPercent}
	}
	return usagePercentAmount(usageClampPercent(*window.usedPercent))
}

// codexBuildUsageStatus keeps a 100% advisory window a warning while the
// backend still reports the account as allowed.
func codexBuildUsageStatus(usedFraction *float64, explicitlyAllowed bool) UsageStatus {
	status := usageStatusFor(usedFraction)
	if status == UsageStatusExhausted && explicitlyAllowed {
		return UsageStatusWarning
	}
	return status
}

// codexRequestAllowed reports whether Codex will still serve this meter.
func codexRequestAllowed(allowed, limitReached *bool, creditOverage bool) bool {
	if creditOverage && limitReached != nil && *limitReached {
		return true
	}
	return allowed != nil && *allowed && limitReached != nil && !*limitReached
}

// buildCodexUsageLimit renders a plan window limit.
func buildCodexUsageLimit(key string, window *codexUsageWindow, allowed, limitReached *bool, creditOverage bool, nowMs int64) UsageLimit {
	usageWindow := codexBuildUsageWindow(window, key, nowMs)
	amount := codexBuildUsageAmount(window)
	return UsageLimit{
		ID:     "openai-codex:" + key,
		Label:  usageWindow.Label,
		Scope:  UsageScope{Provider: "openai-codex", WindowID: usageWindow.ID, Shared: true},
		Window: &usageWindow,
		Amount: amount,
		Status: codexBuildUsageStatus(amount.UsedFraction, codexRequestAllowed(allowed, limitReached, creditOverage)),
	}
}

// codexAdditionalLimitSlug derives the meter slug used in limit ids.
func codexAdditionalLimitSlug(limitName, meteredName string) string {
	probe := strings.ToLower(limitName + " " + meteredName)
	if strings.Contains(probe, "spark") || strings.Contains(probe, "bengalfox") {
		return "spark"
	}
	source := meteredName
	if source == "" {
		source = limitName
	}
	if source == "" {
		source = "extra"
	}
	slug := usageSlugify(strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(source), "codex-"), "codex_"))
	if slug == "" {
		return "extra"
	}
	return slug
}

// codexAdditionalDisplayName renders a meter's display name.
func codexAdditionalDisplayName(slug, limitName string) string {
	if slug == "spark" {
		return "Spark"
	}
	if limitName != "" {
		return limitName
	}
	words := strings.Split(slug, "-")
	for index, word := range words {
		if word == "" {
			continue
		}
		words[index] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// buildCodexAdditionalUsageLimit renders one additional meter window.
func buildCodexAdditionalUsageLimit(key, slug, displayName string, window *codexUsageWindow, accountID, limitName, meteredName string, allowed, limitReached *bool, nowMs int64) UsageLimit {
	usageWindow := codexBuildUsageWindow(window, key, nowMs)
	amount := codexBuildUsageAmount(window)
	return UsageLimit{
		ID:    "openai-codex:" + slug + ":" + key,
		Label: usageWindow.Label + " (" + displayName + ")",
		Scope: UsageScope{
			Provider:  "openai-codex",
			AccountID: accountID,
			Tier:      slug,
			ModelID:   limitName,
			WindowID:  usageWindow.ID,
			Shared:    true,
		},
		Window: &usageWindow,
		Amount: amount,
		Status: codexBuildUsageStatus(amount.UsedFraction, codexRequestAllowed(allowed, limitReached, false)),
	}
}

// buildCodexSpendCapLimit renders the per-user credit cap.
func buildCodexSpendCapLimit(cap *codexSpendCap, accountID string, nowMs int64) UsageLimit {
	var resetsAt *int64
	if cap.resetAt != nil || cap.resetAfterSeconds != nil {
		resetsAt = codexResolveResetTime(&codexUsageWindow{resetAt: cap.resetAt, resetAfterSeconds: cap.resetAfterSeconds}, nowMs)
	}
	amount := UsageBuildAmount(cap.used, cap.limit, cap.remaining, cap.usedFraction, UsageUnitCredits)
	status := usageStatusFor(cap.usedFraction)
	if cap.reached != nil {
		if *cap.reached {
			status = UsageStatusExhausted
		} else {
			status = codexBuildUsageStatus(cap.usedFraction, true)
		}
	}
	return UsageLimit{
		ID:     codexSpendCapLimitID,
		Label:  "Spend cap",
		Scope:  UsageScope{Provider: "openai-codex", AccountID: accountID, Tier: "credits", WindowID: "spend-cap", Shared: true},
		Window: &UsageWindow{ID: "spend-cap", Label: "Spend cap", ResetsAt: resetsAt},
		Amount: amount,
		Status: status,
	}
}

// parseCodexResetCredits reads `rate_limit_reset_credits.available_count`.
func parseCodexResetCredits(payload any) *UsageResetCredits {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	block, ok := usageRecord(record["rate_limit_reset_credits"])
	if !ok {
		return nil
	}
	count, ok := usageNumber(block["available_count"])
	if !ok {
		return nil
	}
	available := int(math.Max(0, math.Trunc(count)))
	return &UsageResetCredits{AvailableCount: available}
}

// codexResetCredit is one redeemable saved reset from the detail endpoint.
type codexResetCredit struct {
	ID        string
	Status    string
	GrantedAt string
	ExpiresAt string
}

// listCodexResetCredits lists an account's saved resets. A transport or auth
// failure reports !ok so the caller keeps the count from `/wham/usage`.
func listCodexResetCredits(ctx context.Context, fctx UsageFetchContext, accessToken, accountID, baseURL string) ([]codexResetCredit, int, bool) {
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    usageUserAgent(),
	}
	if accountID != "" {
		headers["ChatGPT-Account-Id"] = accountID
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", usageAbsoluteURL(normalizeCodexBaseURL(baseURL), codexResetCreditsPath), headers, nil)
	if err != nil || !response.OK() {
		return nil, 0, false
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, 0, false
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, 0, false
	}
	credits := make([]codexResetCredit, 0, 4)
	for _, item := range usageObjectArray(record, "credits") {
		id, ok := usageString(item, "id")
		if !ok {
			continue
		}
		credit := codexResetCredit{ID: id}
		credit.Status, _ = usageString(item, "status")
		credit.GrantedAt, _ = usageString(item, "granted_at")
		credit.ExpiresAt, _ = usageString(item, "expires_at")
		credits = append(credits, credit)
	}
	available := 0
	if reported, ok := usageNumber(record["available_count"]); ok {
		available = int(math.Max(0, math.Trunc(reported)))
	} else {
		for _, credit := range credits {
			status := credit.Status
			if status == "" {
				status = "available"
			}
			if status == "available" {
				available++
			}
		}
	}
	return credits, available, true
}

// codexHasDaybreakAccess reports whether the verified-access payload grants
// the cyber (Daybreak) program.
func codexHasDaybreakAccess(payload any) bool {
	record, ok := usageRecord(payload)
	if !ok {
		return false
	}
	items, ok := usageArray(record, "programs")
	if !ok {
		return false
	}
	for _, item := range items {
		program, ok := usageRecord(item)
		if !ok {
			continue
		}
		name, _ := usageString(program, "program")
		if name != "cyber" {
			continue
		}
		state, _ := usageString(program, "state")
		if state != "inactive" {
			return true
		}
		if grants, ok := usageArray(program, "grants"); ok && len(grants) > 0 {
			return true
		}
	}
	return false
}

// fetchCodexDaybreakAccess reports cyber access; every failure reads as false.
func fetchCodexDaybreakAccess(ctx context.Context, fctx UsageFetchContext, baseURL string, headers map[string]string) bool {
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", usageAbsoluteURL(baseURL, codexVerifiedAccessPath), headers, nil)
	if err != nil || !response.OK() {
		return false
	}
	payload, ok := response.JSON()
	if !ok {
		return false
	}
	return codexHasDaybreakAccess(payload)
}

// openAICodexUsageProvider returns the ChatGPT/Codex usage provider.
func openAICodexUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "openai-codex",
		// v1: credit-billed seats gained the spend-cap limit.
		CacheVersion: 1,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "openai-codex" && params.Credential.Type == CredentialOAuth
		},
		ParseRateLimitHeaders: parseCodexRateLimitHeaders,
		FetchUsage:            fetchOpenAICodexUsage,
	}
}

// parseCodexRateLimitHeaders parses `x-codex-{primary,secondary}-*` headers.
func parseCodexRateLimitHeaders(headers map[string]string, nowMs int64, responseStatus *int) *UsageReport {
	parseWindow := func(key string) *codexUsageWindow {
		usedPercent, ok := usageNumber(headers["x-codex-"+key+"-used-percent"])
		if !ok {
			return nil
		}
		window := &codexUsageWindow{usedPercent: &usedPercent}
		if minutes, ok := usageNumber(headers["x-codex-"+key+"-window-minutes"]); ok {
			seconds := int64(math.Trunc(minutes * 60))
			window.limitWindowSeconds = &seconds
		}
		if resetAt, ok := usageNumber(headers["x-codex-"+key+"-reset-at"]); ok {
			window.resetAt = &resetAt
		}
		return window
	}
	primary := parseWindow("primary")
	secondary := parseWindow("secondary")
	if primary == nil && secondary == nil {
		return nil
	}
	var allowed, limitReached *bool
	if responseStatus != nil && *responseStatus >= 200 && *responseStatus < 300 {
		allowed = new(true)
		limitReached = new(false)
	}
	limits := make([]UsageLimit, 0, 2)
	if primary != nil {
		limits = append(limits, buildCodexUsageLimit("primary", primary, allowed, limitReached, false, nowMs))
	}
	if secondary != nil {
		limits = append(limits, buildCodexUsageLimit("secondary", secondary, allowed, limitReached, false, nowMs))
	}
	return &UsageReport{
		Provider:  "openai-codex",
		FetchedAt: nowMs,
		Limits:    limits,
		Metadata:  map[string]any{"source": "ratelimit-headers"},
	}
}

// fetchOpenAICodexUsage probes `/wham/usage` for one Codex credential.
func fetchOpenAICodexUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "openai-codex" {
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
	if credential.ExpiresAt != nil && *credential.ExpiresAt <= nowMs {
		usageLogWarn(fctx, "Codex usage token expired", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	baseURL := normalizeCodexBaseURL(params.BaseURL)
	accountID := credential.AccountID
	if accountID == "" {
		if claims, ok := usageJWTClaims(accessToken); ok {
			if auth, ok := usageRecord(claims[codexJWTAuthClaim]); ok {
				accountID, _ = usageString(auth, "chatgpt_account_id")
			}
		}
	}
	email := usageNormalizeEmail(credential.Email)
	if email == "" {
		if claims, ok := usageJWTClaims(accessToken); ok {
			if profile, ok := usageRecord(claims[codexJWTProfileClaim]); ok {
				raw, _ := usageString(profile, "email")
				email = usageNormalizeEmail(raw)
			}
		}
	}
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    usageUserAgent(),
	}
	if accountID != "" {
		headers["ChatGPT-Account-Id"] = accountID
	}

	// Runs beside the usage request and never fails the probe: a missing
	// entitlement lookup only omits the badge.
	type daybreakResult struct{ granted bool }
	daybreakCh := make(chan daybreakResult, 1)
	go func() {
		daybreakCh <- daybreakResult{granted: fetchCodexDaybreakAccess(ctx, fctx, baseURL, headers)}
	}()

	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", usageAbsoluteURL(baseURL, codexUsagePath), headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Codex usage request error", map[string]any{"provider": params.Provider, "error": err.Error()})
		<-daybreakCh
		return nil, nil
	}
	if !response.OK() {
		<-daybreakCh
		if response.Status == 401 || response.Status == 403 {
			return nil, NewUsageHTTPError(strings.TrimSpace("Codex usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response)), response.Status)
		}
		usageLogWarn(fctx, "Codex usage request failed", map[string]any{"status": response.Status, "provider": params.Provider})
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		<-daybreakCh
		usageLogWarn(fctx, "Codex usage request error", map[string]any{"provider": params.Provider, "error": "invalid JSON"})
		return nil, nil
	}

	parsed := parseCodexUsagePayload(payload)
	planType := ""
	if parsed != nil {
		planType = parsed.planType
	}
	if planType == "" {
		if record, ok := usageRecord(payload); ok {
			planType, _ = usageString(record, "plan_type")
		}
	}

	creditOverage := parsed != nil && parsed.creditOverage
	limits := make([]UsageLimit, 0, 4)
	meterStates := map[string]any{}
	planAllowed, planLimitReached := codexPlanMeterState(parsed, creditOverage)
	if planAllowed != nil {
		meterStates["chat"] = map[string]any{"allowed": *planAllowed, "limitReached": *planLimitReached}
	} else {
		meterStates["chat"] = map[string]any{}
	}
	if parsed != nil && parsed.primary != nil {
		limits = append(limits, buildCodexUsageLimit("primary", parsed.primary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
	}
	if parsed != nil && parsed.secondary != nil {
		limits = append(limits, buildCodexUsageLimit("secondary", parsed.secondary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
	}
	if parsed != nil {
		for _, extra := range parsed.additional {
			slug := codexAdditionalLimitSlug(extra.limitName, extra.meteredName)
			displayName := codexAdditionalDisplayName(slug, extra.limitName)
			state := map[string]any{}
			if extra.allowed != nil {
				state["allowed"] = *extra.allowed
			}
			if extra.limitReached != nil {
				state["limitReached"] = *extra.limitReached
			}
			meterStates[slug] = state
			if extra.primary != nil {
				limits = append(limits, buildCodexAdditionalUsageLimit("primary", slug, displayName, extra.primary, accountID, extra.limitName, extra.meteredName, extra.allowed, extra.limitReached, nowMs))
			}
			if extra.secondary != nil {
				limits = append(limits, buildCodexAdditionalUsageLimit("secondary", slug, displayName, extra.secondary, accountID, extra.limitName, extra.meteredName, extra.allowed, extra.limitReached, nowMs))
			}
		}
		if parsed.spendCap != nil {
			limits = append(limits, buildCodexSpendCapLimit(parsed.spendCap, accountID, nowMs))
		}
	}

	resetCredits := parseCodexResetCredits(payload)
	if resetCredits != nil && resetCredits.AvailableCount > 0 {
		credits, available, ok := listCodexResetCredits(ctx, fctx, accessToken, accountID, params.BaseURL)
		if ok {
			if len(credits) > 0 {
				details := make([]UsageResetCreditDetail, 0, len(credits))
				for _, credit := range credits {
					status := credit.Status
					if status == "" {
						status = "available"
					}
					if status != "available" {
						continue
					}
					details = append(details, UsageResetCreditDetail{
						GrantedAt: credit.GrantedAt,
						ExpiresAt: credit.ExpiresAt,
						Status:    credit.Status,
					})
				}
				resetCredits.Credits = details
			}
			// The detail endpoint is authoritative after expiry/redeem.
			resetCredits.AvailableCount = available
		} else {
			usageLogWarn(fctx, "Codex reset credits detail fetch failed", nil)
		}
	}

	daybreak := (<-daybreakCh).granted
	metadata := map[string]any{
		"planType":    planType,
		"email":       email,
		"accountId":   accountID,
		"meterStates": meterStates,
	}
	if planAllowed != nil {
		metadata["allowed"] = *planAllowed
	}
	if planLimitReached != nil {
		metadata["limitReached"] = *planLimitReached
	}
	if daybreak {
		metadata["daybreak"] = true
	}
	report := &UsageReport{
		Provider:     "openai-codex",
		FetchedAt:    nowMs,
		Limits:       limits,
		ResetCredits: resetCredits,
		Metadata:     metadata,
	}
	if parsed != nil {
		report.Raw = parsed.raw
	} else {
		report.Raw = payload
	}
	return report, nil
}

// codexPlanMeterState applies the credit-overage override to the plan verdict.
func codexPlanMeterState(parsed *codexParsedUsage, creditOverage bool) (*bool, *bool) {
	if parsed == nil {
		return nil, nil
	}
	if !creditOverage {
		return parsed.allowed, parsed.limitReached
	}
	return new(true), new(false)
}
