package ai

// Ports packages/ai/src/usage/claude-reset.ts: Claude's saved rate-limit reset
// inventory (Cedar grants and the Juniper session reset).

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"time"
)

var (
	claudeCedarGrantID   = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
	claudeOrganizationID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	claudeResetWindowID  = regexp.MustCompile(`^[a-z0-9_-]{1,80}$`)
)

// claudeResetWindowIDs maps provider window keys to normalized limit ids.
var claudeResetWindowIDs = map[string]string{
	"five_hour":        "anthropic:5h",
	"seven_day":        "anthropic:7d",
	"seven_day_opus":   "anthropic:7d:opus",
	"seven_day_sonnet": "anthropic:7d:sonnet",
}

// usageISOString renders epoch milliseconds as an ISO-8601 UTC timestamp.
func usageISOString(millis int64) string {
	return time.UnixMilli(millis).UTC().Format("2006-01-02T15:04:05.000Z")
}

// usageOptionalIsoTimestamp normalizes an optional ISO timestamp; ok is false
// for a malformed value.
func usageOptionalIsoTimestamp(value any) (string, bool, bool) {
	if value == nil {
		return "", false, true
	}
	text, ok := value.(string)
	if !ok || trimJSWhitespace(text) == "" {
		return "", false, false
	}
	millis, ok := usageIsoTimestamp(trimJSWhitespace(text))
	if !ok {
		return "", false, false
	}
	return usageISOString(millis), true, true
}

// usageOptionalString trims an optional string value.
func usageOptionalString(value any) (string, bool, bool) {
	if value == nil {
		return "", false, true
	}
	text, ok := value.(string)
	if !ok {
		return "", false, false
	}
	trimmed := trimJSWhitespace(text)
	if trimmed == "" {
		return "", false, false
	}
	return trimmed, true, true
}

// normalizeClaudeResetWindowID maps a provider window key to a limit id.
func normalizeClaudeResetWindowID(value any) (string, bool) {
	text, ok := value.(string)
	if !ok || !claudeResetWindowID.MatchString(text) {
		return "", false
	}
	if id, ok := claudeResetWindowIDs[text]; ok {
		return id, true
	}
	return "anthropic:reset:" + text, true
}

// normalizeClaudeResetWindowList maps a window-key array to limit ids.
func normalizeClaudeResetWindowList(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		id, ok := normalizeClaudeResetWindowID(item)
		if !ok {
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, true
}

// normalizeClaudeUsedFractions maps a percent map to used fractions.
func normalizeClaudeUsedFractions(value any) (map[string]float64, bool) {
	if value == nil {
		return map[string]float64{}, true
	}
	record, ok := usageRecord(value)
	if !ok {
		return nil, false
	}
	out := make(map[string]float64, len(record))
	for rawID, percent := range record {
		id, ok := normalizeClaudeResetWindowID(rawID)
		if !ok {
			return nil, false
		}
		number, ok := usageNumber(percent)
		if !ok || math.Trunc(number) != number || number < 0 || number > 100 {
			return nil, false
		}
		out[id] = number / 100
	}
	return out, true
}

// claudeCedarGrant is one parsed Cedar grant.
type claudeCedarGrant struct {
	id             string
	label          string
	remainingCount int
	grantedAt      string
	expiresAt      string
	clears         []string
	paused         bool
	usableNow      bool
	requiresLimit  bool
	usedFractions  map[string]float64
	blocking       []string
}

// claudeCedarStatus is the parsed Cedar block.
type claudeCedarStatus struct {
	eligible      bool
	reason        string
	grants        []claudeCedarGrant
	nextGrantID   string
	cooldownUntil string
}

// claudeJuniperStatus is the parsed Juniper block.
type claudeJuniperStatus struct {
	eligible        bool
	reason          string
	arm             string
	available       bool
	nextAvailableAt string
	weeklyResetsAt  string
}

// parseClaudeCedarGrant reads one Cedar grant object.
func parseClaudeCedarGrant(value any) (*claudeCedarGrant, bool) {
	record, ok := usageRecord(value)
	if !ok {
		return nil, false
	}
	id, ok := record["id"].(string)
	if !ok || !claudeCedarGrantID.MatchString(id) {
		return nil, false
	}
	resetsLeft, ok := usageNumber(record["resets_left"])
	if !ok || math.Trunc(resetsLeft) != resetsLeft || resetsLeft < 0 {
		return nil, false
	}
	if resetsTotal, present := record["resets_total"]; present {
		number, ok := usageNumber(resetsTotal)
		if !ok || math.Trunc(number) != number || number < 0 {
			return nil, false
		}
	}
	label, _, ok := usageOptionalString(record["label"])
	if !ok {
		return nil, false
	}
	grantedAt, _, ok := usageOptionalIsoTimestamp(record["starts_at"])
	if !ok {
		return nil, false
	}
	expiresAt, _, ok := usageOptionalIsoTimestamp(record["ends_at"])
	if !ok {
		return nil, false
	}
	clears, ok := normalizeClaudeResetWindowList(record["clears"])
	if !ok {
		return nil, false
	}
	blocking, ok := normalizeClaudeResetWindowList(record["blocking"])
	if !ok {
		return nil, false
	}
	usedFractions, ok := normalizeClaudeUsedFractions(record["percent_used"])
	if !ok {
		return nil, false
	}
	if paused, present := record["paused"]; present {
		if _, ok := paused.(bool); !ok {
			return nil, false
		}
	}
	if usableNow, present := record["usable_now"]; present {
		if _, ok := usableNow.(bool); !ok {
			return nil, false
		}
	}
	if requiresLimit, present := record["use_requires_limit"]; present {
		if _, ok := requiresLimit.(bool); !ok {
			return nil, false
		}
	}
	grant := &claudeCedarGrant{
		id:             id,
		label:          label,
		remainingCount: int(resetsLeft),
		grantedAt:      grantedAt,
		expiresAt:      expiresAt,
		clears:         clears,
		blocking:       blocking,
		usedFractions:  usedFractions,
	}
	grant.paused, _ = usageBool(record, "paused")
	grant.usableNow, _ = usageBool(record, "usable_now")
	grant.requiresLimit = true
	if value, ok := usageBool(record, "use_requires_limit"); ok {
		grant.requiresLimit = value
	}
	return grant, true
}

// parseClaudeCedarStatus reads the Cedar block; a null block is "unknown".
func parseClaudeCedarStatus(value any) (*claudeCedarStatus, bool) {
	if value == nil {
		return nil, true
	}
	record, ok := usageRecord(value)
	if !ok {
		return nil, false
	}
	eligible, ok := usageBool(record, "eligible")
	if !ok {
		return nil, false
	}
	reason, _, ok := usageOptionalString(record["ineligible_reason"])
	if !ok {
		return nil, false
	}
	cooldownUntil, _, ok := usageOptionalIsoTimestamp(record["cooldown_until"])
	if !ok {
		return nil, false
	}
	if atLimit, present := record["at_limit"]; present {
		if _, ok := atLimit.(bool); !ok {
			return nil, false
		}
	}
	if exhausted, present := record["exhausted"]; present {
		if _, ok := normalizeClaudeResetWindowList(exhausted); !ok {
			return nil, false
		}
	}
	if grants, present := record["grants"]; present {
		if _, ok := grants.([]any); !ok {
			return nil, false
		}
	}
	status := &claudeCedarStatus{eligible: eligible, reason: reason, cooldownUntil: cooldownUntil}
	for _, rawGrant := range usageArrayOrEmpty(record["grants"]) {
		grant, ok := parseClaudeCedarGrant(rawGrant)
		if !ok {
			return nil, false
		}
		status.grants = append(status.grants, *grant)
	}
	if rawNext, present := record["next_grant_id"]; present && rawNext != nil {
		text, ok := rawNext.(string)
		if !ok || !claudeCedarGrantID.MatchString(text) {
			return nil, false
		}
		for _, grant := range status.grants {
			if grant.id == text {
				status.nextGrantID = text
				break
			}
		}
	}
	return status, true
}

// parseClaudeJuniperStatus reads the Juniper block.
func parseClaudeJuniperStatus(value any) (*claudeJuniperStatus, bool) {
	if value == nil {
		return nil, true
	}
	record, ok := usageRecord(value)
	if !ok {
		return nil, false
	}
	eligible, ok := usageBool(record, "eligible")
	if !ok {
		return nil, false
	}
	reason, _, ok := usageOptionalString(record["ineligible_reason"])
	if !ok {
		return nil, false
	}
	nextAvailableAt, _, ok := usageOptionalIsoTimestamp(record["next_available_at"])
	if !ok {
		return nil, false
	}
	weeklyResetsAt, _, ok := usageOptionalIsoTimestamp(record["weekly_resets_at"])
	if !ok {
		return nil, false
	}
	if available, present := record["available"]; present {
		if _, ok := available.(bool); !ok {
			return nil, false
		}
	}
	arm := ""
	if rawArm, present := record["arm"]; present && rawArm != nil {
		text, ok := rawArm.(string)
		if !ok || (text != "control" && text != "reset") {
			return nil, false
		}
		arm = text
	}
	if resetsPerWeek, present := record["resets_per_week"]; present {
		number, ok := usageNumber(resetsPerWeek)
		if !ok || math.Trunc(number) != number || number < 0 {
			return nil, false
		}
	}
	status := &claudeJuniperStatus{
		eligible:        eligible,
		reason:          reason,
		arm:             arm,
		nextAvailableAt: nextAvailableAt,
		weeklyResetsAt:  weeklyResetsAt,
	}
	status.available, _ = usageBool(record, "available")
	return status, true
}

// usageArrayOrEmpty returns a JSON array field, or nil.
func usageArrayOrEmpty(value any) []any {
	items, _ := value.([]any)
	return items
}

// normalizeClaudeCedarList renders Cedar grants as reset-credit state.
func normalizeClaudeCedarList(status *claudeCedarStatus, orgID, baseURL string) *UsageResetCredits {
	now := timeNowMillis()
	cooldownActive := false
	if status.cooldownUntil != "" {
		if cooldownMs, ok := usageIsoTimestamp(status.cooldownUntil); ok {
			cooldownActive = cooldownMs > now
		}
	}
	credits := make([]UsageResetCreditDetail, 0, len(status.grants))
	var selected *UsageResetCreditDetail
	availableCount := 0
	for index := range status.grants {
		grant := status.grants[index]
		expired := false
		if grant.expiresAt != "" {
			if millis, ok := usageIsoTimestamp(grant.expiresAt); ok {
				expired = millis <= now
			}
		}
		selectedGrant := grant.id == status.nextGrantID
		usable := status.eligible && selectedGrant && grant.usableNow && !grant.paused && !expired &&
			!cooldownActive && grant.remainingCount > 0
		creditStatus := "unavailable"
		switch {
		case expired:
			creditStatus = "expired"
		case grant.remainingCount == 0:
			creditStatus = "redeemed"
		case grant.paused:
			creditStatus = "paused"
		case usable:
			creditStatus = "available"
		}
		if !expired && grant.remainingCount > 0 {
			availableCount += grant.remainingCount
		}
		title := grant.label
		if title == "" {
			title = "Claude limit reset"
		}
		credit := UsageResetCreditDetail{
			ID:             grant.id,
			Title:          title,
			Program:        claudeCedarProgram,
			RemainingCount: grant.remainingCount,
			Usable:         new(usable),
			RequiresLimit:  new(grant.requiresLimit),
			Clears:         grant.clears,
			Blocking:       grant.blocking,
			UsedFractions:  grant.usedFractions,
			GrantedAt:      grant.grantedAt,
			ExpiresAt:      grant.expiresAt,
			Status:         creditStatus,
		}
		credits = append(credits, credit)
		if selectedGrant {
			selected = &credits[len(credits)-1]
		}
	}
	result := &UsageResetCredits{
		AvailableCount: availableCount,
		Eligible:       new(status.eligible),
		Credits:        credits,
	}
	if selected != nil && selected.Usable != nil && *selected.Usable {
		redeemable := selected.RemainingCount
		result.RedeemableCount = &redeemable
	} else {
		result.RedeemableCount = new(0)
	}
	if status.nextGrantID != "" && status.eligible {
		result.NextCreditID = status.nextGrantID
	}
	if status.reason != "" {
		result.Reason = status.reason
	} else if cooldownActive {
		result.Reason = "cooldown"
	}
	result.CooldownUntil = status.cooldownUntil
	_ = orgID
	_ = baseURL
	return result
}

// normalizeClaudeJuniperList renders the Juniper session reset.
func normalizeClaudeJuniperList(status *claudeJuniperStatus, orgID, baseURL string) *UsageResetCredits {
	eligible := status.eligible && status.arm == "reset"
	usable := eligible && status.available
	offered := status.arm == "reset"
	reason := status.reason
	if reason == "" {
		switch {
		case !status.eligible:
			reason = "ineligible"
		case status.arm != "reset":
			reason = "control"
		case !status.available:
			reason = "unavailable"
		}
	}
	result := &UsageResetCredits{
		AvailableCount: 0,
		Eligible:       new(eligible),
		Reason:         reason,
	}
	if offered {
		credit := UsageResetCreditDetail{
			ID:             claudeJuniperProgram,
			Title:          "Claude session limit reset",
			Program:        claudeJuniperProgram,
			RemainingCount: 0,
			Usable:         new(usable),
			RequiresLimit:  new(true),
			Clears:         []string{"anthropic:5h"},
			Blocking:       []string{},
			UsedFractions:  map[string]float64{},
			ExpiresAt:      status.weeklyResetsAt,
			Status:         "unavailable",
		}
		if usable {
			credit.RemainingCount = 1
			credit.Status = "available"
			result.AvailableCount = 1
			result.RedeemableCount = new(1)
		} else {
			result.RedeemableCount = new(0)
		}
		result.Credits = []UsageResetCreditDetail{credit}
	} else {
		result.RedeemableCount = new(0)
	}
	if eligible {
		result.NextCreditID = claudeJuniperProgram
	}
	if !usable && status.nextAvailableAt != "" {
		result.CooldownUntil = status.nextAvailableAt
	}
	_ = orgID
	_ = baseURL
	return result
}

// parseClaudeResetCreditsFromUsagePayload reuses program blocks a normal usage
// response already evaluated. A null block is "unknown", never "no resets".
func parseClaudeResetCreditsFromUsagePayload(payload any, orgID, baseURL string) *UsageResetCredits {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	safeOrgID := ""
	if trimmed := trimJSWhitespace(orgID); claudeOrganizationID.MatchString(trimmed) {
		safeOrgID = trimmed
	}
	var cedarList *UsageResetCredits
	if cedarBlock, ok := usageRecord(record[claudeCedarProgram]); ok {
		status, ok := parseClaudeCedarStatus(cedarBlock)
		if !ok || status == nil {
			return nil
		}
		cedarList = normalizeClaudeCedarList(status, safeOrgID, baseURL)
		if cedarList.AvailableCount > 0 || cedarList.NextCreditID != "" {
			return cedarList
		}
	}
	if juniperBlock, ok := usageRecord(record[claudeJuniperProgram]); ok {
		status, ok := parseClaudeJuniperStatus(juniperBlock)
		if !ok || status == nil {
			return nil
		}
		return normalizeClaudeJuniperList(status, safeOrgID, baseURL)
	}
	return cedarList
}

// claudeResetDiscovery is the outcome of one reset-inventory probe.
type claudeResetDiscovery struct {
	kind    string // "answered" or "failed"
	credits *UsageResetCredits
	baseURL string
}

// discoverClaudeResetStatus probes one program's evaluation query.
func discoverClaudeResetStatus(ctx context.Context, fctx UsageFetchContext, accessToken, baseURL, query, field string, parse func(any) (*UsageResetCredits, bool)) claudeResetDiscovery {
	for _, candidate := range claudeOAuthBaseURLs(baseURL) {
		if ctx.Err() != nil {
			return claudeResetDiscovery{kind: "failed"}
		}
		probeCtx, cancel := usageDeadline(ctx, 5*time.Second)
		response, err := usageDo(probeCtx, fctx, usageFollowRedirects, "GET", candidate+"/usage?"+query, buildClaudeOAuthHeaders(accessToken, claudeOAuthBeta, true), nil)
		cancel()
		if err != nil {
			return claudeResetDiscovery{kind: "failed"}
		}
		if !response.OK() {
			canFallback := claudeEndpointAbsentStatus(response.Status) && candidate != claudeDefaultOAuthBaseURL
			if canFallback {
				continue
			}
			return claudeResetDiscovery{kind: "failed"}
		}
		payload, ok := response.JSON()
		record, isRecord := usageRecord(payload)
		if !ok || !isRecord {
			return claudeResetDiscovery{kind: "failed"}
		}
		if _, present := record[field]; present {
			credits, ok := parse(record[field])
			if !ok {
				return claudeResetDiscovery{kind: "failed"}
			}
			return claudeResetDiscovery{kind: "answered", credits: credits, baseURL: candidate}
		}
		if claudeHasUsageData(record) || claudeLooksLikeUsagePayload(record) {
			return claudeResetDiscovery{kind: "answered", baseURL: candidate}
		}
		if candidate == claudeDefaultOAuthBaseURL {
			return claudeResetDiscovery{kind: "failed"}
		}
	}
	return claudeResetDiscovery{kind: "failed"}
}

// listClaudeResetCredits discovers Cedar grants, then the Juniper reset. A nil
// result means discovery failed; a zero-count object is an authoritative answer.
func listClaudeResetCredits(ctx context.Context, fctx UsageFetchContext, accessToken string, credential UsageCredential, baseURL string) *UsageResetCredits {
	configuredOrg := trimJSWhitespace(credential.OrgID)
	orgID := ""
	if claudeOrganizationID.MatchString(configuredOrg) {
		orgID = configuredOrg
	}
	cedar := discoverClaudeResetStatus(ctx, fctx, accessToken, baseURL, "cedar_ember=1&skip_spend=1", claudeCedarProgram, func(value any) (*UsageResetCredits, bool) {
		status, ok := parseClaudeCedarStatus(value)
		if !ok || status == nil {
			return nil, ok
		}
		return normalizeClaudeCedarList(status, orgID, ""), true
	})
	if cedar.kind == "failed" {
		return nil
	}
	var cedarList *UsageResetCredits
	if cedar.credits != nil {
		cedarList = cedar.credits
		if cedarList.AvailableCount > 0 || cedarList.NextCreditID != "" {
			return resolveClaudeResetOrganization(ctx, fctx, accessToken, credential, cedarList)
		}
	}
	juniper := discoverClaudeResetStatus(ctx, fctx, accessToken, baseURL, "at_wall=1&skip_spend=1", claudeJuniperProgram, func(value any) (*UsageResetCredits, bool) {
		status, ok := parseClaudeJuniperStatus(value)
		if !ok || status == nil {
			return nil, ok
		}
		return normalizeClaudeJuniperList(status, orgID, ""), true
	})
	if juniper.kind == "answered" && juniper.credits != nil {
		return resolveClaudeResetOrganization(ctx, fctx, accessToken, credential, juniper.credits)
	}
	if cedarList != nil && juniper.kind == "answered" {
		return cedarList
	}
	if juniper.kind == "answered" {
		return &UsageResetCredits{
			AvailableCount:  0,
			RedeemableCount: new(0),
			Eligible:        new(false),
			Credits:         []UsageResetCreditDetail{},
		}
	}
	return nil
}

// resolveClaudeResetOrganization fills the organization a redemption needs.
func resolveClaudeResetOrganization(ctx context.Context, fctx UsageFetchContext, accessToken string, credential UsageCredential, credits *UsageResetCredits) *UsageResetCredits {
	if credits == nil || credits.NextCreditID != "" || len(credits.Credits) == 0 {
		return credits
	}
	configured := trimJSWhitespace(credential.OrgID)
	if claudeOrganizationID.MatchString(configured) {
		return credits
	}
	probeCtx, cancel := usageDeadline(ctx, 5*time.Second)
	defer cancel()
	response, err := usageDo(probeCtx, fctx, usageFollowRedirects, "GET", claudeOAuthBaseURL("")+"/profile", buildClaudeOAuthHeaders(accessToken, claudeOAuthBeta, true), nil)
	if err != nil || !response.OK() {
		return credits
	}
	payload, ok := response.JSON()
	record, isRecord := usageRecord(payload)
	if !ok || !isRecord {
		return credits
	}
	candidate := ""
	if organization, ok := usageRecord(record["organization"]); ok {
		candidate, _ = usageString(organization, "uuid")
	}
	if candidate == "" {
		candidate, _ = usageString(record, "organization_uuid")
	}
	if !claudeOrganizationID.MatchString(candidate) {
		return credits
	}
	return credits
}

// usageJSONObject renders a value as a JSON object string, for raw payloads
// that a provider embeds in a request.
func usageJSONObject(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
