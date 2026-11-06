package ai

// Ports packages/ai/src/usage/claude.ts, usage/claude-api.ts and
// usage/claude-reset.ts: the Anthropic subscription usage endpoint, its
// `anthropic-ratelimit-unified-*` header snapshot and saved reset credits.

import (
	"context"
	"math"
	"strings"
)

const (
	claudeDefaultAPIBaseURL   = "https://api.anthropic.com"
	claudeDefaultOAuthBaseURL = "https://api.anthropic.com/api/oauth"
	claudeOAuthBeta           = "oauth-2025-04-20"
	// claudeUsageBetas is the OAuth client's full beta list; the account routes
	// accept the whole set and the CLI sends it.
	claudeUsageBetas       = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05,mid-conversation-system-2026-04-07,advanced-tool-use-2025-11-20,effort-2025-11-24,extended-cache-ttl-2025-04-11"
	claudeMaxAttempts      = 3
	claudeBaseRetryDelayMs = 500
	// claudeFailureBackoffMs: `/usage` stays rate-limited for minutes after a
	// 429, so a failed refresh waits instead of re-polling.
	claudeFailureBackoffMs = 60_000
	claudeCedarProgram     = "cedar_ember"
	claudeJuniperProgram   = "juniper_tide"
)

// claudeUsageBucket is one utilization/reset pair.
type claudeUsageBucket struct {
	utilization *float64
	resetsAt    *int64
}

// claudeAPILimitEntry is one generic `limits[]` row.
type claudeAPILimitEntry struct {
	kind        string
	bucket      claudeUsageBucket
	displayName string
}

// normalizeClaudeAPIBaseURL reduces a Messages or OAuth endpoint to the API root.
func normalizeClaudeAPIBaseURL(baseURL string) string {
	trimmed := trimJSWhitespace(baseURL)
	if trimmed == "" {
		return claudeDefaultAPIBaseURL
	}
	parsed, err := parseHTTPURL(trimmed)
	if err != nil {
		return claudeDefaultAPIBaseURL
	}
	path := usageTrimTrailingSlashes(parsedURLPath(trimmed))
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, "/api/oauth"):
		path = path[:len(path)-len("/api/oauth")]
	case strings.HasSuffix(lower, "/v1"):
		path = path[:len(path)-len("/v1")]
	}
	return parsed.origin() + path
}

// parsedURLPath returns a URL's path, or "" when unparseable.
func parsedURLPath(raw string) string {
	parsed, err := parseRequestPath(raw)
	if err != nil {
		return ""
	}
	return parsed
}

// claudeOAuthBaseURL is the OAuth API root, keeping any configured prefix.
func claudeOAuthBaseURL(baseURL string) string {
	return normalizeClaudeAPIBaseURL(baseURL) + "/api/oauth"
}

// claudeOAuthBaseURLs lists the configured OAuth root then the canonical
// fallback, without duplicates.
func claudeOAuthBaseURLs(baseURL string) []string {
	configured := claudeOAuthBaseURL(baseURL)
	if configured == claudeDefaultOAuthBaseURL {
		return []string{claudeDefaultOAuthBaseURL}
	}
	return []string{configured, claudeDefaultOAuthBaseURL}
}

// claudeAPIURL resolves a first-party account route against the API root.
func claudeAPIURL(baseURL, path string) string {
	return usageAbsoluteURL(normalizeClaudeAPIBaseURL(baseURL), path)
}

// buildClaudeOAuthHeaders builds the OAuth request headers.
func buildClaudeOAuthHeaders(accessToken, beta string, jsonBody bool) map[string]string {
	headers := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-encoding": "gzip, compress, deflate, br",
		"connection":      "keep-alive",
		"user-agent":      usageClaudeCodeUserAgent(),
		"authorization":   "Bearer " + accessToken,
		"anthropic-beta":  beta,
	}
	if jsonBody {
		headers["content-type"] = "application/json"
	}
	return headers
}

// usageClaudeCodeUserAgent mirrors Claude Code's CLI inference User-Agent.
func usageClaudeCodeUserAgent() string { return "claude-cli/2.1.280 (external, cli)" }

// parseClaudeBucket reads one `{utilization, resets_at}` bucket.
func parseClaudeBucket(value any) *claudeUsageBucket {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	bucket := &claudeUsageBucket{}
	if number, ok := usageNumber(record["utilization"]); ok {
		bucket.utilization = &number
	}
	if reset, ok := usageIsoTimestamp(record["resets_at"]); ok {
		bucket.resetsAt = &reset
	}
	if bucket.utilization == nil && bucket.resetsAt == nil {
		return nil
	}
	return bucket
}

// claudeAPILimitDisplayName reads `scope.model.display_name`.
func claudeAPILimitDisplayName(scope any) string {
	record, ok := usageRecord(scope)
	if !ok {
		return ""
	}
	model, ok := usageRecord(record["model"])
	if !ok {
		return ""
	}
	name, _ := usageString(model, "display_name")
	return name
}

// parseClaudeAPILimitEntries reads generic `limits[]` rows.
//
// Legacy per-model weekly buckets are permanently null as of 2026-07-02;
// scoped weekly caps now arrive only here with the family in
// `scope.model.display_name`. `is_active` is ignored on purpose: it marks the
// currently binding limit, not whether the bucket exists.
func parseClaudeAPILimitEntries(raw any) []claudeAPILimitEntry {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	entries := make([]claudeAPILimitEntry, 0, len(items))
	for _, item := range items {
		record, ok := usageRecord(item)
		if !ok {
			continue
		}
		kind, ok := usageString(record, "kind")
		if !ok {
			continue
		}
		utilization, hasUtilization := usageNumber(record["percent"])
		reset, hasReset := usageIsoTimestamp(record["resets_at"])
		if !hasUtilization && !hasReset {
			continue
		}
		entry := claudeAPILimitEntry{kind: kind, displayName: claudeAPILimitDisplayName(record["scope"])}
		if hasUtilization {
			entry.bucket.utilization = &utilization
		}
		if hasReset {
			entry.bucket.resetsAt = &reset
		}
		entries = append(entries, entry)
	}
	return entries
}

// claudeHasUsageData reports whether a payload carries any usable counter.
func claudeHasUsageData(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	for _, key := range []string{"five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet"} {
		if bucket := parseClaudeBucket(payload[key]); bucket != nil && bucket.utilization != nil {
			return true
		}
	}
	for _, entry := range parseClaudeAPILimitEntries(payload["limits"]) {
		if entry.bucket.utilization != nil {
			return true
		}
	}
	if buildClaudeExtraUsageLimit(payload) != nil {
		return true
	}
	return parseClaudeResetCreditsFromUsagePayload(payload, "", "") != nil
}

// claudeLooksLikeUsagePayload distinguishes a usage body from an index page.
func claudeLooksLikeUsagePayload(payload map[string]any) bool {
	for _, key := range []string{"five_hour", "seven_day", "limits", "extra_usage", "spend", "cedar_ember", "juniper_tide"} {
		if usageHasOwn(payload, key) {
			return true
		}
	}
	return false
}

// claudeUsageIdentity is the account identity a payload reports.
type claudeUsageIdentity struct {
	accountID string
	email     string
}

// extractClaudeUsageIdentity reads the identity fields a payload carries.
func extractClaudeUsageIdentity(payload map[string]any) claudeUsageIdentity {
	nested := func(parent, key string) string {
		record, ok := usageRecord(payload[parent])
		if !ok {
			return ""
		}
		value, _ := usageString(record, key)
		return value
	}
	identity := claudeUsageIdentity{}
	for _, key := range []string{"account_id", "accountId", "user_id", "userId"} {
		if value, ok := usageString(payload, key); ok {
			identity.accountID = value
			break
		}
	}
	if identity.accountID == "" {
		for _, pair := range [][2]string{{"account", "uuid"}, {"account", "id"}, {"user", "uuid"}, {"user", "id"}} {
			if value := nested(pair[0], pair[1]); value != "" {
				identity.accountID = value
				break
			}
		}
	}
	for _, key := range []string{"email", "user_email", "userEmail"} {
		if value, ok := usageString(payload, key); ok {
			identity.email = value
			break
		}
	}
	if identity.email == "" {
		for _, parent := range []string{"account", "user"} {
			if value := nested(parent, "email"); value != "" {
				identity.email = value
				break
			}
		}
	}
	return identity
}

// isClaudeRetryableStatus excludes 429: the endpoint is rate-limited per source
// IP, so replaying a rate_limit_error inside one fetch only deepens it.
func isClaudeRetryableStatus(status int) bool {
	switch status {
	case 408, 500, 502, 503, 504, 522, 524:
		return true
	}
	return false
}

// claudeRetryDelayMs mirrors the exponential backoff with retry-after.
func claudeRetryDelayMs(attempt int, retryAfter string) int64 {
	baseline := int64(claudeBaseRetryDelayMs) << uint(attempt)
	trimmed := trimJSWhitespace(retryAfter)
	if trimmed == "" {
		return baseline
	}
	if seconds, err := parseFloatPrefix(trimmed); err == nil && !math.IsNaN(seconds) {
		value := int64(math.Max(float64(baseline), math.Max(0, seconds*1000)))
		return value
	}
	if delay := DateParse(trimmed); !math.IsNaN(delay) {
		value := int64(math.Max(float64(baseline), math.Max(0, delay-float64(timeNowMillis()))))
		return value
	}
	return baseline
}

// claudePayloadResult is the outcome of reading a usage body.
type claudePayloadResult struct {
	payload        map[string]any
	endpointAbsent bool
}

// claudeEndpointAbsentStatuses answer "this host has no endpoint here".
func claudeEndpointAbsentStatus(status int) bool {
	switch status {
	case 404, 405, 410, 501:
		return true
	}
	return false
}

// fetchClaudeUsagePayload reads one host's usage body with bounded retries.
func fetchClaudeUsagePayload(ctx context.Context, fctx UsageFetchContext, endpoint string, headers map[string]string) claudePayloadResult {
	var lastPayload map[string]any
	endpointAbsent := false
	for attempt := 0; attempt < claudeMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return claudePayloadResult{}
		}
		response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
		if err != nil {
			if ctx.Err() != nil {
				return claudePayloadResult{}
			}
			usageLogWarn(fctx, "Claude usage fetch error", map[string]any{"error": err.Error(), "attempt": attempt})
			if !sleepBeforeClaudeRetry(ctx, attempt, "") {
				break
			}
			continue
		}
		if !response.OK() {
			absent := claudeEndpointAbsentStatus(response.Status)
			retryable := !absent && isClaudeRetryableStatus(response.Status)
			usageLogWarn(fctx, "Claude usage fetch failed", map[string]any{
				"status":    response.Status,
				"attempt":   attempt,
				"willRetry": retryable && attempt < claudeMaxAttempts-1,
			})
			if !retryable {
				return claudePayloadResult{payload: nil, endpointAbsent: absent}
			}
			if !sleepBeforeClaudeRetry(ctx, attempt, response.HeaderValue("retry-after")) {
				break
			}
			continue
		}
		body := trimJSWhitespace(response.Text())
		if body == "" {
			// An empty 2xx serves nothing here.
			return claudePayloadResult{payload: lastPayload, endpointAbsent: true}
		}
		payload, ok := response.JSON()
		record, isRecord := usageRecord(payload)
		if !ok || !isRecord {
			claimsJSON := strings.Contains(strings.ToLower(response.HeaderValue("content-type")), "json")
			usageLogWarn(fctx, "Claude usage response was not JSON", map[string]any{
				"contentType": response.HeaderValue("content-type"),
				"attempt":     attempt,
			})
			if !claimsJSON {
				return claudePayloadResult{payload: lastPayload, endpointAbsent: true}
			}
			if !sleepBeforeClaudeRetry(ctx, attempt, "") {
				break
			}
			continue
		}
		lastPayload = record
		if claudeHasUsageData(record) {
			return claudePayloadResult{payload: record}
		}
		endpointAbsent = !claudeLooksLikeUsagePayload(record)
		usageLogWarn(fctx, "Claude usage response missing usage data", map[string]any{"attempt": attempt})
		if !sleepBeforeClaudeRetry(ctx, attempt, "") {
			break
		}
	}
	return claudePayloadResult{payload: lastPayload, endpointAbsent: endpointAbsent}
}

// sleepBeforeClaudeRetry waits for the attempt's backoff, reporting whether to
// retry.
func sleepBeforeClaudeRetry(ctx context.Context, attempt int, retryAfter string) bool {
	if ctx.Err() != nil || attempt >= claudeMaxAttempts-1 {
		return false
	}
	if err := Sleep(ctx, float64(claudeRetryDelayMs(attempt, retryAfter))); err != nil {
		return false
	}
	return ctx.Err() == nil
}

// buildClaudeUsageLimit renders one subscription window.
func buildClaudeUsageLimit(id, label, windowID, windowLabel string, durationMs int64, bucket *claudeUsageBucket, tier string, shared *bool) *UsageLimit {
	if bucket == nil || bucket.utilization == nil {
		return nil
	}
	amount := usagePercentAmount(*bucket.utilization)
	duration := durationMs
	window := &UsageWindow{ID: windowID, Label: windowLabel, DurationMs: &duration, ResetsAt: bucket.resetsAt}
	limit := &UsageLimit{
		ID:     id,
		Label:  label,
		Scope:  UsageScope{Provider: "anthropic", WindowID: windowID, Tier: tier},
		Window: window,
		Amount: amount,
		Status: usageStatusFor(amount.UsedFraction),
	}
	if shared != nil {
		limit.Scope.Shared = *shared
	}
	return limit
}

// buildClaudeScopedWeeklyLimits renders `weekly_scoped` rows as per-family
// counters. They deliberately leave scope.shared unset: an exhausted Fable cap
// must not block Opus or Sonnet on the same credential.
func buildClaudeScopedWeeklyLimits(entries []claudeAPILimitEntry) []UsageLimit {
	seen := map[string]bool{}
	limits := make([]UsageLimit, 0, len(entries))
	for _, entry := range entries {
		if entry.kind != "weekly_scoped" || entry.displayName == "" {
			continue
		}
		slug := usageSlugify(entry.displayName)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		if limit := buildClaudeUsageLimit(
			"anthropic:7d:"+slug,
			"Claude 7 Day ("+entry.displayName+")",
			"7d", "7 Day", usageWeekMs, &entry.bucket, slug, nil,
		); limit != nil {
			limits = append(limits, *limit)
		}
	}
	return limits
}

// parseClaudeDollarAmount reads a minor-unit money object in USD.
func parseClaudeDollarAmount(amountMinor any, exponent any, currency any, currencyRequired bool) (float64, bool) {
	minor, ok := usageNumber(amountMinor)
	if !ok || math.Trunc(minor) != minor || minor < 0 {
		return 0, false
	}
	exp, ok := usageNumber(exponent)
	if !ok || math.Trunc(exp) != exp || exp < 0 {
		return 0, false
	}
	if currency == nil {
		if currencyRequired {
			return 0, false
		}
	} else {
		text, ok := currency.(string)
		if !ok || strings.ToUpper(text) != "USD" {
			return 0, false
		}
	}
	divisor := math.Pow(10, exp)
	if math.IsInf(divisor, 0) || divisor == 0 {
		return 0, false
	}
	value := minor / divisor
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// claudeExtraUsage is the parsed extra-usage meter.
type claudeExtraUsage struct {
	used  float64
	limit *float64
}

// parseClaudeSpendExtraUsage reads the current `spend` block.
func parseClaudeSpendExtraUsage(value any) *claudeExtraUsage {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	enabled, _ := usageBool(record, "enabled")
	if !enabled || !usageHasOwn(record, "limit") {
		return nil
	}
	usedRecord, ok := usageRecord(record["used"])
	if !ok {
		return nil
	}
	used, ok := parseClaudeDollarAmount(usedRecord["amount_minor"], usedRecord["exponent"], usedRecord["currency"], true)
	if !ok {
		return nil
	}
	if record["limit"] == nil {
		return &claudeExtraUsage{used: used}
	}
	limitRecord, ok := usageRecord(record["limit"])
	if !ok {
		return nil
	}
	limit, ok := parseClaudeDollarAmount(limitRecord["amount_minor"], limitRecord["exponent"], limitRecord["currency"], true)
	if !ok || limit <= 0 {
		return nil
	}
	return &claudeExtraUsage{used: used, limit: &limit}
}

// parseClaudeLegacyExtraUsage reads `extra_usage`.
func parseClaudeLegacyExtraUsage(value any) *claudeExtraUsage {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	enabled, _ := usageBool(record, "is_enabled")
	if !enabled || !usageHasOwn(record, "monthly_limit") {
		return nil
	}
	decimalPlaces := 2.0
	if number, ok := usageNumber(record["decimal_places"]); ok {
		decimalPlaces = number
	}
	used, ok := parseClaudeDollarAmount(record["used_credits"], decimalPlaces, record["currency"], false)
	if !ok {
		return nil
	}
	monthly := record["monthly_limit"]
	if monthly == nil {
		return &claudeExtraUsage{used: used}
	}
	limit, ok := parseClaudeDollarAmount(monthly, decimalPlaces, record["currency"], false)
	if !ok || limit <= 0 {
		return nil
	}
	return &claudeExtraUsage{used: used, limit: &limit}
}

// buildClaudeExtraUsageLimit renders the `anthropic:extra` dollar meter.
func buildClaudeExtraUsageLimit(payload map[string]any) *UsageLimit {
	var parsed *claudeExtraUsage
	if payload["spend"] == nil {
		parsed = parseClaudeLegacyExtraUsage(payload["extra_usage"])
	} else {
		parsed = parseClaudeSpendExtraUsage(payload["spend"])
	}
	if parsed == nil {
		return nil
	}
	amount := UsageAmount{Used: &parsed.used, Unit: UsageUnitUSD}
	var status UsageStatus
	if parsed.limit != nil {
		remaining := math.Max(0, *parsed.limit-parsed.used)
		fraction := parsed.used / *parsed.limit
		remainingFraction := remaining / *parsed.limit
		if math.IsNaN(fraction) || math.IsInf(fraction, 0) || math.IsNaN(remainingFraction) || math.IsInf(remainingFraction, 0) {
			return nil
		}
		amount.Limit = parsed.limit
		amount.Remaining = &remaining
		amount.UsedFraction = &fraction
		amount.RemainingFraction = &remainingFraction
		if parsed.used >= *parsed.limit {
			status = UsageStatusExhausted
		} else {
			status = usageStatusFor(&fraction)
		}
	}
	return &UsageLimit{
		ID:     "anthropic:extra",
		Label:  "Claude Extra Usage",
		Scope:  UsageScope{Provider: "anthropic", WindowID: "extra"},
		Amount: amount,
		Status: status,
	}
}

// parseClaudeUnifiedWindow reads one `anthropic-ratelimit-unified-<window>-*`
// header pair.
func parseClaudeUnifiedWindow(headers map[string]string, window string) *claudeUsageBucket {
	prefix := "anthropic-ratelimit-unified-" + window + "-"
	fraction, hasFraction := usageNumber(headers[prefix+"utilization"])
	resetSeconds, hasReset := usageNumber(headers[prefix+"reset"])
	if !hasFraction && !hasReset {
		return nil
	}
	bucket := &claudeUsageBucket{}
	if hasFraction {
		utilization := fraction * 100
		bucket.utilization = &utilization
	}
	if hasReset && resetSeconds > 0 {
		if millis, ok := usageMillisFromEpochSeconds(resetSeconds); ok {
			bucket.resetsAt = &millis
		}
	}
	if bucket.utilization == nil && bucket.resetsAt == nil {
		return nil
	}
	return bucket
}

// parseClaudeRateLimitHeaders parses the unified rate-limit response headers.
func parseClaudeRateLimitHeaders(headers map[string]string, nowMs int64, _ *int) *UsageReport {
	limits := make([]UsageLimit, 0, 3)
	shared := new(true)
	for _, spec := range []struct {
		id, label, windowID, windowLabel string
		durationMs                       int64
		header                           string
		tier                             string
		shared                           *bool
	}{
		{"anthropic:5h", "Claude 5 Hour", "5h", "5 Hour", 5 * usageHourMs, "5h", "", shared},
		{"anthropic:7d", "Claude 7 Day", "7d", "7 Day", usageWeekMs, "7d", "", shared},
		{"anthropic:7d:fable", "Claude 7 Day (Fable)", "7d", "7 Day", usageWeekMs, "7d_oi", "fable", nil},
	} {
		if limit := buildClaudeUsageLimit(spec.id, spec.label, spec.windowID, spec.windowLabel, spec.durationMs, parseClaudeUnifiedWindow(headers, spec.header), spec.tier, spec.shared); limit != nil {
			limits = append(limits, *limit)
		}
	}
	if len(limits) == 0 {
		return nil
	}
	return &UsageReport{
		Provider:  "anthropic",
		FetchedAt: nowMs,
		Limits:    limits,
		Metadata:  map[string]any{"source": "ratelimit-headers"},
	}
}

// claudeUsageToken resolves the token a usage probe may authenticate with: an
// OAuth access token, or an API-key field that actually holds an OAuth token.
func claudeUsageToken(credential UsageCredential) string {
	if credential.Type == CredentialOAuth {
		return credential.AccessToken
	}
	if strings.HasPrefix(credential.APIKey, "sk-ant-oat") {
		return credential.APIKey
	}
	return ""
}

// parseClaudeUsagePayload parses one live usage body without extra requests.
func parseClaudeUsagePayload(raw any, identity claudeUsageIdentity, orgID, endpoint string, fetchedAt int64) *UsageReport {
	payload, ok := usageRecord(raw)
	if !ok {
		return nil
	}
	apiLimitEntries := parseClaudeAPILimitEntries(payload["limits"])
	fiveHour := parseClaudeBucket(payload["five_hour"])
	if fiveHour == nil {
		for _, entry := range apiLimitEntries {
			if entry.kind == "session" {
				fiveHour = &entry.bucket
				break
			}
		}
	}
	sevenDay := parseClaudeBucket(payload["seven_day"])
	if sevenDay == nil {
		for _, entry := range apiLimitEntries {
			if entry.kind == "weekly_all" {
				sevenDay = &entry.bucket
				break
			}
		}
	}
	shared := new(true)
	limits := make([]UsageLimit, 0, 6)
	for _, spec := range []struct {
		id, label, windowID, windowLabel string
		durationMs                       int64
		bucket                           *claudeUsageBucket
		tier                             string
		shared                           *bool
	}{
		{"anthropic:5h", "Claude 5 Hour", "5h", "5 Hour", 5 * usageHourMs, fiveHour, "", shared},
		{"anthropic:7d", "Claude 7 Day", "7d", "7 Day", usageWeekMs, sevenDay, "", shared},
		{"anthropic:7d:opus", "Claude 7 Day (Opus)", "7d", "7 Day", usageWeekMs, parseClaudeBucket(payload["seven_day_opus"]), "opus", nil},
		{"anthropic:7d:sonnet", "Claude 7 Day (Sonnet)", "7d", "7 Day", usageWeekMs, parseClaudeBucket(payload["seven_day_sonnet"]), "sonnet", nil},
	} {
		if limit := buildClaudeUsageLimit(spec.id, spec.label, spec.windowID, spec.windowLabel, spec.durationMs, spec.bucket, spec.tier, spec.shared); limit != nil {
			limits = append(limits, *limit)
		}
	}
	limits = append(limits, buildClaudeScopedWeeklyLimits(apiLimitEntries)...)
	if extra := buildClaudeExtraUsageLimit(payload); extra != nil {
		limits = append(limits, *extra)
	}
	if len(limits) == 0 {
		return nil
	}
	// The response speaks for the token that fetched it; stored metadata may be stale.
	payloadIdentity := extractClaudeUsageIdentity(payload)
	if payloadIdentity.accountID == "" {
		payloadIdentity.accountID = identity.accountID
	}
	if payloadIdentity.email == "" {
		payloadIdentity.email = identity.email
	}
	metadata := map[string]any{}
	if endpoint != "" {
		metadata["endpoint"] = endpoint
	}
	if payloadIdentity.accountID != "" {
		metadata["accountId"] = payloadIdentity.accountID
	}
	if payloadIdentity.email != "" {
		metadata["email"] = payloadIdentity.email
	}
	if orgID != "" {
		metadata["orgId"] = orgID
	}
	return &UsageReport{Provider: "anthropic", FetchedAt: fetchedAt, Limits: limits, Metadata: metadata, Raw: payload}
}

// claudeProfile is the account profile response.
type claudeProfile struct {
	accountID string
	email     string
}

// fetchClaudeProfile reads `/profile` for the missing identity fields.
func fetchClaudeProfile(ctx context.Context, fctx UsageFetchContext, baseURL string, headers map[string]string) claudeProfile {
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", usageAbsoluteURL(baseURL, "profile"), headers, nil)
	if err != nil || !response.OK() {
		return claudeProfile{}
	}
	payload, ok := response.JSON()
	record, isRecord := usageRecord(payload)
	if !ok || !isRecord {
		return claudeProfile{}
	}
	profile := claudeProfile{}
	if value, ok := usageString(record, "uuid"); ok {
		profile.accountID = value
	}
	if profile.accountID == "" {
		if account, ok := usageRecord(record["account"]); ok {
			profile.accountID, _ = usageString(account, "uuid")
		}
	}
	if value, ok := usageString(record, "email"); ok {
		profile.email = value
	}
	if profile.email == "" {
		if account, ok := usageRecord(record["account"]); ok {
			profile.email, _ = usageString(account, "email")
		}
	}
	return profile
}

// claudeUsageProvider returns the Anthropic subscription usage provider.
func claudeUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "anthropic",
		// v3 retires parsed reports created before the extra-usage row existed.
		CacheVersion:     3,
		FailureBackoffMs: claudeFailureBackoffMs,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "anthropic" && claudeUsageToken(params.Credential) != ""
		},
		ParseRateLimitHeaders: parseClaudeRateLimitHeaders,
		FetchUsage:            fetchClaudeUsage,
	}
}

// fetchClaudeUsage probes `/usage` on each candidate host.
func fetchClaudeUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "anthropic" {
		return nil, nil
	}
	credential := params.Credential
	accessToken := claudeUsageToken(credential)
	if accessToken == "" {
		return nil, nil
	}
	headers := buildClaudeOAuthHeaders(accessToken, claudeUsageBetas, true)

	var payload map[string]any
	var baseURL string
	for _, candidate := range claudeOAuthBaseURLs(params.BaseURL) {
		result := fetchClaudeUsagePayload(ctx, fctx, candidate+"/usage", headers)
		if result.payload != nil && claudeHasUsageData(result.payload) {
			baseURL = candidate
			payload = result.payload
			break
		}
		// A usage-shaped body without numbers: retries already gave up on fresher
		// numbers here, so hold it while the remaining candidate is probed.
		if result.payload != nil && payload == nil {
			baseURL = candidate
			payload = result.payload
		}
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		// Only a host answering "no usage endpoint here" justifies moving off the
		// configured host.
		if !result.endpointAbsent {
			break
		}
	}
	if payload == nil || baseURL == "" {
		return nil, nil
	}
	identity := claudeUsageIdentity{accountID: credential.AccountID, email: credential.Email}
	endpoint := baseURL + "/usage"
	report := parseClaudeUsagePayload(payload, identity, credential.OrgID, endpoint, timeNowMillis())
	resetCredits := parseClaudeResetCreditsFromUsagePayload(payload, credential.OrgID, baseURL)
	if resetCredits == nil {
		resetCredits = listClaudeResetCredits(ctx, fctx, accessToken, credential, params.BaseURL)
	}
	hasResetInventory := resetCredits != nil && (resetCredits.AvailableCount > 0 || resetCredits.NextCreditID != "" || len(resetCredits.Credits) > 0)
	if report == nil && !hasResetInventory {
		return nil, nil
	}
	if report == nil {
		report = &UsageReport{
			Provider:  "anthropic",
			FetchedAt: timeNowMillis(),
			Limits:    []UsageLimit{},
			Metadata:  map[string]any{"endpoint": endpoint},
			Raw:       payload,
		}
		if identity.accountID != "" {
			report.Metadata["accountId"] = identity.accountID
		}
		if identity.email != "" {
			report.Metadata["email"] = identity.email
		}
	}
	if report.Metadata == nil {
		report.Metadata = map[string]any{}
	}
	accountID := ""
	if value, ok := report.Metadata["accountId"].(string); ok {
		accountID = value
	}
	if accountID == "" {
		accountID = credential.AccountID
	}
	email := ""
	if value, ok := report.Metadata["email"].(string); ok {
		email = value
	}
	if email == "" {
		email = credential.Email
	}
	if (accountID == "" || email == "") && ctx.Err() == nil {
		profile := fetchClaudeProfile(ctx, fctx, baseURL, headers)
		if accountID == "" {
			accountID = profile.accountID
		}
		if email == "" {
			email = profile.email
		}
	}
	if resetCredits != nil {
		report.ResetCredits = resetCredits
		if resetCredits.NextCreditID != "" {
			report.Metadata["orgId"] = credential.OrgID
		}
	} else if fctx.PreviousReport != nil && fctx.PreviousReport.ResetCredits != nil {
		// `nil` means the reset probe failed, not that the account has no resets:
		// keep the block this credential last reported.
		report.ResetCredits = fctx.PreviousReport.ResetCredits
	}
	if accountID != "" {
		report.Metadata["accountId"] = accountID
	}
	if email != "" {
		report.Metadata["email"] = email
	}
	if credential.OrgID != "" {
		report.Metadata["orgId"] = credential.OrgID
	}
	return report, nil
}
