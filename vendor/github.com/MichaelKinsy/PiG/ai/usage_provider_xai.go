package ai

// Ports packages/ai/src/usage/xai-oauth.ts, usage/factory-droid.ts,
// usage/commandcode.ts and usage/devin.ts.

import (
	"context"
	"math"
	"runtime"
	"strings"
	"time"
)

// --- SuperGrok (xai-oauth) --------------------------------------------------

const (
	xaiUsageProviderID   = "xai-oauth"
	xaiBillingSource     = "cli-chat-proxy.grok.com/v1/billing"
	xaiCLIBillingBaseURL = "https://cli-chat-proxy.grok.com"
	xaiCLIBillingPath    = "/v1/billing"
	xaiUserInfoURL       = "https://auth.x.ai/oauth2/userinfo"
)

// xaiBillingPeriod is one billing period.
type xaiBillingPeriod struct {
	start string
	end   string
	kind  string
}

// xaiProductUsage is one product's utilization.
type xaiProductUsage struct {
	product      string
	usagePercent float64
}

// xaiBillingConfig is a parsed billing payload.
type xaiBillingConfig struct {
	weekly          bool
	period          xaiBillingPeriod
	monthlyUsed     float64
	monthlyLimit    float64
	creditPercent   float64
	inferredPercent bool
	productUsage    []xaiProductUsage
	onDemandCap     *float64
	onDemandUsed    *float64
}

// xaiParsePercent reads a 0..100 percent.
func xaiParsePercent(value any) *float64 {
	number, ok := usageNumber(value)
	if !ok || number < 0 || number > 100 {
		return nil
	}
	return &number
}

// xaiParseAmount reads an `{val: N}` on-demand amount.
func xaiParseAmount(value any) *float64 {
	record, ok := usageRecord(value)
	if !ok {
		return nil
	}
	number, ok := usageNumber(record["val"])
	if !ok || number < 0 {
		return nil
	}
	return &number
}

// xaiBuildPercentAmount renders a percent amount.
func xaiBuildPercentAmount(usagePercent float64) UsageAmount {
	fraction := usagePercent / 100
	remaining := 100 - usagePercent
	remainingFraction := 1 - fraction
	return UsageAmount{
		Used:              &usagePercent,
		Limit:             new(100.0),
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitPercent,
	}
}

// xaiBuildPeriodWindow renders the weekly credits window.
func xaiBuildPeriodWindow(period xaiBillingPeriod) *UsageWindow {
	window := &UsageWindow{ID: "1w", Label: "Weekly", DurationMs: new(usageWeekMs)}
	if reset, ok := usageIsoTimestamp(period.end); ok {
		window.ResetsAt = &reset
	}
	return window
}

// xaiBuildMonthlyWindow renders the unified-billing monthly window.
func xaiBuildMonthlyWindow(periodStart, periodEnd string) *UsageWindow {
	startMs, okStart := usageIsoTimestamp(periodStart)
	endMs, okEnd := usageIsoTimestamp(periodEnd)
	if !okStart || !okEnd || endMs <= startMs {
		return nil
	}
	duration := endMs - startMs
	approxDays := int64(math.Round(float64(duration) / float64(usageDayMs)))
	if approxDays < 1 {
		approxDays = 1
	}
	label := usageFormatCount(approxDays) + "d"
	if approxDays == 30 || approxDays == 31 {
		label = "Monthly"
	}
	return &UsageWindow{ID: "1mo", Label: label, DurationMs: &duration, ResetsAt: &endMs}
}

// xaiParseWeeklyBillingConfig reads the legacy weekly credits shape.
func xaiParseWeeklyBillingConfig(record map[string]any) *xaiBillingConfig {
	period, ok := usageRecord(record["currentPeriod"])
	if !ok {
		return nil
	}
	startText, _ := usageString(period, "start")
	endText, _ := usageString(period, "end")
	startMs, okStart := usageIsoTimestamp(startText)
	endMs, okEnd := usageIsoTimestamp(endText)
	periodType, _ := usageString(period, "type")
	// Keep recently-ended weekly windows so /usage still renders across the
	// rollover while billing is mid-refresh.
	if !okStart || !okEnd || endMs <= startMs || !strings.Contains(strings.ToUpper(periodType), "WEEK") {
		return nil
	}
	inferred := !usageHasOwn(record, "creditUsagePercent") || record["creditUsagePercent"] == nil
	var creditPercent float64
	if inferred {
		// Fresh or zero-usage periods omit the percent; default to 0 only while
		// the period is active.
		if endMs > timeNowMillis() {
			creditPercent = 0
		} else {
			return nil
		}
	} else {
		parsed := xaiParsePercent(record["creditUsagePercent"])
		if parsed == nil {
			return nil
		}
		creditPercent = *parsed
	}
	productUsage := make([]xaiProductUsage, 0, 2)
	if raw, present := record["productUsage"]; present {
		items, ok := raw.([]any)
		if !ok {
			return nil
		}
		for _, item := range items {
			productRecord, ok := usageRecord(item)
			if !ok {
				continue
			}
			product, _ := usageString(productRecord, "product")
			if product == "" {
				continue
			}
			percent := 0.0
			if rawPercent, present := productRecord["usagePercent"]; present && rawPercent != nil {
				parsed := xaiParsePercent(rawPercent)
				if parsed == nil {
					continue
				}
				percent = *parsed
			}
			productUsage = append(productUsage, xaiProductUsage{product: product, usagePercent: percent})
		}
	}
	return &xaiBillingConfig{
		weekly:          true,
		period:          xaiBillingPeriod{start: startText, end: endText, kind: periodType},
		creditPercent:   creditPercent,
		inferredPercent: inferred,
		productUsage:    productUsage,
		onDemandCap:     xaiParseAmount(record["onDemandCap"]),
		onDemandUsed:    xaiParseAmount(record["onDemandUsed"]),
	}
}

// xaiParseMonthlyBillingConfig reads the unified-billing monthly shape.
func xaiParseMonthlyBillingConfig(record map[string]any) *xaiBillingConfig {
	periodStart, _ := usageString(record, "billingPeriodStart")
	periodEnd, _ := usageString(record, "billingPeriodEnd")
	startMs, okStart := usageIsoTimestamp(periodStart)
	endMs, okEnd := usageIsoTimestamp(periodEnd)
	if periodStart == "" || periodEnd == "" || !okStart || !okEnd || endMs <= startMs {
		return nil
	}
	limit := xaiParseAmount(record["monthlyLimit"])
	used := xaiParseAmount(record["used"])
	// A zero or missing included quota is not a usable report.
	if limit == nil || *limit <= 0 || used == nil {
		return nil
	}
	return &xaiBillingConfig{
		weekly:       false,
		period:       xaiBillingPeriod{start: periodStart, end: periodEnd},
		monthlyUsed:  *used,
		monthlyLimit: *limit,
		onDemandCap:  xaiParseAmount(record["onDemandCap"]),
		onDemandUsed: xaiParseAmount(record["onDemandUsed"]),
	}
}

// xaiConfirmsNoMonthlyQuota reports an authoritative "no monthly quota".
func xaiConfirmsNoMonthlyQuota(record map[string]any) bool {
	if limit := xaiParseAmount(record["monthlyLimit"]); limit != nil {
		return *limit == 0
	}
	weekly := xaiParseWeeklyBillingConfig(record)
	return weekly != nil && weekly.inferredPercent
}

// xaiBuildOnDemandLimit renders the on-demand meter.
func xaiBuildOnDemandLimit(cap, used *float64, accountID string) *UsageLimit {
	if cap == nil || *cap <= 0 || used == nil {
		return nil
	}
	fraction := math.Min(*used / *cap, 1)
	remaining := math.Max(0, *cap-*used)
	remainingFraction := 1 - fraction
	scope := UsageScope{Provider: xaiUsageProviderID, Shared: true}
	if accountID != "" {
		scope.AccountID = accountID
	}
	return &UsageLimit{
		ID:    xaiUsageProviderID + ":on-demand",
		Label: "On-demand",
		Scope: scope,
		Amount: UsageAmount{
			Used:              used,
			Limit:             cap,
			Remaining:         &remaining,
			UsedFraction:      &fraction,
			RemainingFraction: &remainingFraction,
			Unit:              UsageUnitUnknown,
		},
		Status: usageStatusFor(&fraction),
	}
}

// xaiBuildLimits renders a billing config's limits.
func xaiBuildLimits(config *xaiBillingConfig, accountID string) []UsageLimit {
	if config == nil {
		return nil
	}
	if config.weekly {
		window := xaiBuildPeriodWindow(config.period)
		scope := UsageScope{Provider: xaiUsageProviderID, WindowID: window.ID, Shared: true}
		if accountID != "" {
			scope.AccountID = accountID
		}
		overall := xaiBuildPercentAmount(config.creditPercent)
		limits := []UsageLimit{{
			ID:     xaiUsageProviderID + ":credits:1w",
			Label:  "SuperGrok Weekly Credits",
			Scope:  scope,
			Window: window,
			Amount: overall,
			Status: usageStatusFor(overall.UsedFraction),
		}}
		for _, item := range config.productUsage {
			slug := usageSlugify(item.product)
			if slug == "" {
				continue
			}
			label := item.product
			switch item.product {
			case "GrokBuild":
				label = "Grok Build"
			case "Api":
				label = "API"
			}
			amount := xaiBuildPercentAmount(item.usagePercent)
			limits = append(limits, UsageLimit{
				ID:     xaiUsageProviderID + ":product:" + slug + ":1w",
				Label:  label + " (Weekly)",
				Scope:  scope,
				Window: window,
				Amount: amount,
				Status: usageStatusFor(amount.UsedFraction),
			})
		}
		if onDemand := xaiBuildOnDemandLimit(config.onDemandCap, config.onDemandUsed, accountID); onDemand != nil {
			limits = append(limits, *onDemand)
		}
		return limits
	}
	window := xaiBuildMonthlyWindow(config.period.start, config.period.end)
	if window == nil {
		return nil
	}
	fraction := math.Min(config.monthlyUsed/config.monthlyLimit, 1)
	remaining := math.Max(0, config.monthlyLimit-config.monthlyUsed)
	remainingFraction := 1 - fraction
	scope := UsageScope{Provider: xaiUsageProviderID, WindowID: window.ID, Shared: true}
	if accountID != "" {
		scope.AccountID = accountID
	}
	limits := []UsageLimit{{
		ID:     xaiUsageProviderID + ":included:1mo",
		Label:  "SuperGrok Monthly Included",
		Scope:  scope,
		Window: window,
		Amount: UsageAmount{
			Used:              new(config.monthlyUsed),
			Limit:             new(config.monthlyLimit),
			Remaining:         &remaining,
			UsedFraction:      &fraction,
			RemainingFraction: &remainingFraction,
			// xAI does not label the unit; amounts match dashboard quota points.
			Unit: UsageUnitUnknown,
		},
		Status: usageStatusFor(&fraction),
	}}
	if onDemand := xaiBuildOnDemandLimit(config.onDemandCap, config.onDemandUsed, accountID); onDemand != nil {
		limits = append(limits, *onDemand)
	}
	return limits
}

// xaiFetchBilling fetches one billing payload.
func xaiFetchBilling(ctx context.Context, fctx UsageFetchContext, format, accessToken string) (map[string]any, bool) {
	endpoint := xaiCLIBillingBaseURL + xaiCLIBillingPath
	if format != "" {
		endpoint += "?format=" + percentEncode(format)
	}
	response, err := usageDo(ctx, fctx, usageRejectRedirects, "GET", endpoint, map[string]string{
		"Authorization":    "Bearer " + accessToken,
		"Accept":           "application/json",
		"X-XAI-Token-Auth": "xai-grok-cli",
	}, nil)
	if err != nil || !response.OK() {
		return nil, false
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, false
	}
	return usageRecord(payload)
}

// xaiIdentity is the optional OIDC userinfo.
type xaiIdentity struct {
	accountID string
	email     string
}

// xaiFetchIdentity reads the OIDC userinfo for a token.
func xaiFetchIdentity(ctx context.Context, fctx UsageFetchContext, accessToken string) xaiIdentity {
	probeCtx, cancel := usageDeadline(ctx, 15*time.Second)
	defer cancel()
	response, err := usageDo(probeCtx, fctx, usageRejectRedirects, "GET", xaiUserInfoURL, map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Accept":        "application/json",
	}, nil)
	if err != nil || !response.OK() {
		return xaiIdentity{}
	}
	payload, ok := response.JSON()
	if !ok {
		return xaiIdentity{}
	}
	record, ok := usageRecord(payload)
	if !ok {
		return xaiIdentity{}
	}
	identity := xaiIdentity{}
	identity.accountID, _ = usageString(record, "sub")
	identity.email, _ = usageString(record, "email")
	return identity
}

// xaiTokenSubject reads the `sub` claim of an access token.
func xaiTokenSubject(accessToken string) string {
	claims, ok := usageJWTClaims(accessToken)
	if !ok {
		return ""
	}
	subject, _ := usageString(claims, "sub")
	return subject
}

// fetchXaiOAuthUsage probes the SuperGrok billing endpoints.
func fetchXaiOAuthUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != xaiUsageProviderID || params.Credential.Type != CredentialOAuth {
		return nil, nil
	}
	accessToken := trimJSWhitespace(params.Credential.AccessToken)
	if accessToken == "" {
		return nil, nil
	}
	if params.Credential.ExpiresAt != nil && *params.Credential.ExpiresAt <= timeNowMillis() {
		return nil, nil
	}
	accountID := trimJSWhitespace(params.Credential.AccountID)
	if accountID == "" {
		accountID = xaiTokenSubject(accessToken)
	}
	email := strings.ToLower(trimJSWhitespace(params.Credential.Email))
	if email == "" {
		// Identity enrichment is best effort; billing remains authoritative.
		identity := xaiFetchIdentity(ctx, fctx, accessToken)
		email = strings.ToLower(trimJSWhitespace(identity.email))
		if accountID == "" {
			accountID = trimJSWhitespace(identity.accountID)
		}
	}

	// Always probe weekly credits first (the legacy SuperGrok shape).
	creditsPayload, hasCredits := xaiFetchBilling(ctx, fctx, "credits", accessToken)
	var weekly *xaiBillingConfig
	creditsLooksUnified := false
	if hasCredits {
		if config, ok := usageRecord(creditsPayload["config"]); ok {
			weekly = xaiParseWeeklyBillingConfig(config)
			if unified, ok := usageBool(config, "isUnifiedBillingUser"); ok {
				creditsLooksUnified = unified
			}
		}
	}

	// Unified accounts expose a separate monthly included-quota payload.
	var monthlyPayload map[string]any
	var monthly *xaiBillingConfig
	if weekly == nil || creditsLooksUnified {
		if payload, ok := xaiFetchBilling(ctx, fctx, "", accessToken); ok {
			monthlyPayload = payload
			if config, ok := usageRecord(payload["config"]); ok {
				monthly = xaiParseMonthlyBillingConfig(config)
			}
		}
	}

	// A positive monthly quota normally wins over an inferred weekly percent.
	effectiveWeekly := weekly
	if weekly != nil && weekly.inferredPercent && creditsLooksUnified {
		if monthly != nil {
			effectiveWeekly = nil
		} else if config, ok := usageRecord(monthlyPayload["config"]); !ok || !xaiConfirmsNoMonthlyQuota(config) {
			effectiveWeekly = nil
		}
	}
	monthlyAdvisory := weekly != nil && weekly.inferredPercent && creditsLooksUnified &&
		monthly != nil && monthly.monthlyUsed >= monthly.monthlyLimit
	if effectiveWeekly == nil && monthly == nil {
		return nil, nil
	}
	limits := make([]UsageLimit, 0, 4)
	if effectiveWeekly != nil {
		limits = append(limits, xaiBuildLimits(effectiveWeekly, accountID)...)
	}
	if monthly != nil {
		monthlyLimits := xaiBuildLimits(monthly, accountID)
		if monthlyAdvisory && len(monthlyLimits) > 0 {
			monthlyLimits[0].Status = UsageStatusUnknown
			monthlyLimits[0].Notes = []string{"Monthly counter exceeds its limit, but active weekly credits leave enforcement uncertain."}
		}
		limits = append(limits, monthlyLimits...)
	}
	seen := map[string]bool{}
	deduped := make([]UsageLimit, 0, len(limits))
	for _, limit := range limits {
		if seen[limit.ID] {
			continue
		}
		seen[limit.ID] = true
		deduped = append(deduped, limit)
	}
	if len(deduped) == 0 {
		return nil, nil
	}
	billingKind := "monthly"
	endpoint := xaiCLIBillingBaseURL + xaiCLIBillingPath
	if effectiveWeekly != nil && monthly != nil {
		billingKind = "unified"
		endpoint += " + " + xaiCLIBillingBaseURL + xaiCLIBillingPath
	} else if effectiveWeekly != nil {
		billingKind = "weekly"
		endpoint += "?format=credits"
	}
	metadata := map[string]any{"endpoint": endpoint, "source": xaiBillingSource, "billingKind": billingKind}
	if monthlyAdvisory {
		metadata["monthlyQuotaAdvisory"] = true
	}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	if email != "" {
		metadata["email"] = email
	}
	var raw any
	switch {
	case effectiveWeekly != nil && monthly != nil:
		raw = map[string]any{"credits": creditsPayload, "monthly": monthlyPayload}
	case effectiveWeekly != nil:
		raw = creditsPayload
	default:
		raw = monthlyPayload
	}
	return &UsageReport{Provider: xaiUsageProviderID, FetchedAt: timeNowMillis(), Limits: deduped, Metadata: metadata, Raw: raw}, nil
}

// xaiOAuthUsageProvider returns the SuperGrok usage provider.
func xaiOAuthUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: xaiUsageProviderID,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == xaiUsageProviderID && params.Credential.Type == CredentialOAuth && params.Credential.AccessToken != ""
		},
		FetchUsage: fetchXaiOAuthUsage,
	}
}

// --- Factory Droid ----------------------------------------------------------

const (
	factoryDroidDefaultRegion = "global"
	factoryDroidClientVersion = "0.237.0"
)

// factoryDroidWindow is one billing window definition.
type factoryDroidWindow struct {
	key      string
	id       string
	label    string
	duration int64
}

var factoryDroidWindows = []factoryDroidWindow{
	{key: "fiveHour", id: "5h", label: "5 Hour", duration: 5 * usageHourMs},
	{key: "weekly", id: "weekly", label: "Weekly", duration: usageWeekMs},
	{key: "monthly", id: "monthly", label: "Monthly", duration: 30 * usageDayMs},
}

// factoryDroidPool is one billing pool definition.
type factoryDroidPool struct {
	key   string
	label string
}

var factoryDroidPools = []factoryDroidPool{
	{key: "standard", label: "Standard credits"},
	{key: "core", label: "Droid Core"},
}

// factoryDroidAPIBaseURL resolves the account host for a residency region.
func factoryDroidAPIBaseURL(region string) string {
	if region == "eu" {
		return "https://api.eu.factory.ai"
	}
	return "https://api.factory.ai"
}

// factoryDroidClientHeaders builds the Factory client identity headers.
func factoryDroidClientHeaders(orgID string) map[string]string {
	headers := map[string]string{
		"X-Client-Version": factoryDroidClientVersion,
		"X-Factory-Client": "cli",
	}
	if orgID != "" {
		headers["X-Factory-Org-Id"] = orgID
	}
	return headers
}

// parseFactoryDroidUsage parses `GET /api/billing/limits`.
func parseFactoryDroidUsage(payload any, fetchedAt int64) *UsageReport {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	if usesTokens, ok := usageBool(record, "usesTokenRateLimitsBilling"); ok && !usesTokens {
		return &UsageReport{
			Provider:  "factory-droid",
			FetchedAt: fetchedAt,
			Limits:    []UsageLimit{},
			Notes:     []string{"This Factory account does not use token-rate-limit billing; no quota windows are exposed."},
			Raw:       payload,
		}
	}
	baseLimits, ok := usageRecord(record["limits"])
	if !ok {
		return nil
	}
	limits := make([]UsageLimit, 0, 6)
	for _, pool := range factoryDroidPools {
		poolValue, ok := usageRecord(baseLimits[pool.key])
		if !ok {
			continue
		}
		for _, windowDef := range factoryDroidWindows {
			windowValue, ok := usageRecord(poolValue[windowDef.key])
			if !ok {
				continue
			}
			usedPercent, ok := usageNumber(windowValue["usedPercent"])
			if !ok {
				continue
			}
			// Factory freezes a lapsed window instead of rolling it forward: a
			// past windowEnd with its final usedPercent means "inactive", which the
			// droid CLI renders as 0% with no countdown.
			windowEnd, hasWindowEnd := usageIsoTimestamp(windowValue["windowEnd"])
			active := hasWindowEnd && windowEnd >= fetchedAt
			window := &UsageWindow{
				ID:         pool.key + "-" + windowDef.id,
				Label:      pool.label + " " + windowDef.label,
				DurationMs: &windowDef.duration,
			}
			if active {
				window.ResetsAt = &windowEnd
			}
			effective := 0.0
			if active {
				effective = usedPercent
			}
			fraction := math.Min(1, math.Max(0, effective/100))
			remaining := math.Max(0, 100-effective)
			remainingFraction := math.Max(0, 1-fraction)
			limits = append(limits, UsageLimit{
				ID:     "factory-droid:" + pool.key + ":" + windowDef.id,
				Label:  pool.label + " " + windowDef.label + " window",
				Scope:  UsageScope{Provider: "factory-droid", WindowID: window.ID},
				Window: window,
				Amount: UsageAmount{
					Used:              &effective,
					Limit:             new(100.0),
					Remaining:         &remaining,
					UsedFraction:      &fraction,
					RemainingFraction: &remainingFraction,
					Unit:              UsageUnitPercent,
				},
				Status: usageStatusFor(&fraction),
			})
		}
	}
	if balanceCents, ok := usageNumber(record["extraUsageBalanceCents"]); ok && balanceCents > 0 {
		balanceUSD := balanceCents / 100
		zero := 0.0
		one := 1.0
		limits = append(limits, UsageLimit{
			ID:    "factory-droid:extra-balance",
			Label: "Extra usage balance",
			Scope: UsageScope{Provider: "factory-droid"},
			Amount: UsageAmount{
				Used:              &zero,
				Limit:             &balanceUSD,
				Remaining:         &balanceUSD,
				UsedFraction:      &zero,
				RemainingFraction: &one,
				Unit:              UsageUnitUSD,
			},
			Status: UsageStatusOK,
		})
	}
	if len(limits) == 0 {
		return nil
	}
	return &UsageReport{Provider: "factory-droid", FetchedAt: fetchedAt, Limits: limits, Raw: payload}
}

// fetchFactoryDroidUsage probes the Factory billing limits route.
func fetchFactoryDroidUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "factory-droid" {
		return nil, nil
	}
	credential := params.Credential
	if credential.Type != CredentialOAuth || credential.AccessToken == "" {
		return nil, nil
	}
	region := credential.Region
	if region == "" {
		region = factoryDroidDefaultRegion
	}
	endpoint := factoryDroidAPIBaseURL(region) + "/api/billing/limits"
	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + credential.AccessToken,
	}
	for name, value := range factoryDroidClientHeaders(credential.OrgID) {
		headers[name] = value
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, headers, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, nil
	}
	if !response.OK() {
		if response.Status == 401 || response.Status == 403 {
			return nil, NewUsageHTTPError("Factory Droid usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response), response.Status)
		}
		return nil, nil
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, nil
	}
	report := parseFactoryDroidUsage(payload, timeNowMillis())
	if report == nil {
		usageLogWarn(fctx, "Factory Droid usage request failed", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	metadata := map[string]any{}
	if credential.Email != "" {
		metadata["email"] = credential.Email
	}
	if credential.OrgID != "" {
		metadata["orgId"] = credential.OrgID
	}
	if len(metadata) > 0 {
		report.Metadata = metadata
	}
	return report, nil
}

// factoryDroidUsageProvider returns the Factory Droid usage provider.
func factoryDroidUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID:           "factory-droid",
		CacheVersion: 4,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == "factory-droid" && params.Credential.Type == CredentialOAuth &&
				params.Credential.AccessToken != ""
		},
		FetchUsage: fetchFactoryDroidUsage,
	}
}

// --- Command Code -----------------------------------------------------------

const (
	commandCodeProvider    = "commandcode"
	commandCodeDefaultURL  = "https://api.commandcode.ai"
	commandCodeWhoamiPath  = "/alpha/whoami"
	commandCodeCreditsPath = "/alpha/billing/credits"
)

// commandCodeWindow is one limit window definition.
type commandCodeWindow struct {
	key         string
	id          string
	limitLabel  string
	windowLabel string
	duration    int64
}

var commandCodeWindows = []commandCodeWindow{
	{key: "fiveHour", id: "5h", limitLabel: "5-hour limit", windowLabel: "5-hour", duration: 5 * usageHourMs},
	{key: "weekly", id: "7d", limitLabel: "Weekly limit", windowLabel: "Weekly", duration: usageWeekMs},
}

// commandCodeGetJSON reads an account route.
func commandCodeGetJSON(ctx context.Context, fctx UsageFetchContext, endpoint, apiKey string) (map[string]any, error) {
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET", endpoint, map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		return nil, err
	}
	if !response.OK() {
		// Only 401 throws: a revoked key purges the cached report.
		if response.Status == 401 {
			return nil, NewUsageHTTPError("Command Code usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response), response.Status)
		}
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
		return data, nil
	}
	return record, nil
}

// commandCodeBuildWindowLimit renders one credit window.
func commandCodeBuildWindowLimit(spec commandCodeWindow, raw any, accountID, orgID string) *UsageLimit {
	record, ok := usageRecord(raw)
	if !ok {
		return nil
	}
	used, ok := usageNumber(record["used"])
	if !ok {
		return nil
	}
	cap, hasCap := usageNumber(record["cap"])
	var fraction *float64
	if hasCap && cap > 0 {
		value := used / cap
		fraction = &value
	}
	resetsAt, hasReset := usagePositiveTimestamp(record["resetAt"])
	scope := UsageScope{Provider: commandCodeProvider, AccountID: accountID, WindowID: spec.id, Shared: true}
	if orgID != "" {
		scope.OrgID = orgID
	}
	duration := spec.duration
	window := &UsageWindow{ID: spec.id, Label: spec.windowLabel, DurationMs: &duration}
	if hasReset {
		window.ResetsAt = &resetsAt
	}
	amount := UsageAmount{Used: &used, Unit: UsageUnitCredits}
	if hasCap {
		amount.Limit = &cap
	}
	if fraction != nil {
		amount.UsedFraction = fraction
	}
	status := usageStatusFor(fraction)
	if exceeded, ok := usageBool(record, "exceeded"); ok && exceeded {
		status = UsageStatusExhausted
	}
	return &UsageLimit{
		ID:     commandCodeProvider + ":" + spec.id,
		Label:  spec.limitLabel,
		Scope:  scope,
		Window: window,
		Amount: amount,
		Status: status,
	}
}

// fetchCommandCodeUsage probes the account routes the Command Code CLI uses.
func fetchCommandCodeUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != commandCodeProvider || params.Credential.Type != CredentialAPIKey || params.Credential.APIKey == "" {
		return nil, nil
	}
	origin := usageOrigin(params.BaseURL, commandCodeDefaultURL)
	apiKey := params.Credential.APIKey
	whoami, err := commandCodeGetJSON(ctx, fctx, origin+commandCodeWhoamiPath, apiKey)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		httpErr, ok := err.(*UsageHTTPError)
		if ok && httpErr.Status == 401 {
			return nil, httpErr
		}
		return nil, nil
	}
	if whoami == nil {
		return nil, nil
	}
	user, _ := usageRecord(whoami["user"])
	org, _ := usageRecord(whoami["org"])
	userID, ok := usageString(user, "id")
	if !ok {
		return nil, nil
	}
	orgID, _ := usageString(org, "id")
	orgLogin, _ := usageString(org, "login")
	creditsURL := origin + commandCodeCreditsPath
	if orgID != "" {
		creditsURL += "?orgId=" + percentEncode(orgID)
	}
	creditsBody, err := commandCodeGetJSON(ctx, fctx, creditsURL, apiKey)
	if err != nil || creditsBody == nil {
		if err != nil {
			if httpErr, ok := err.(*UsageHTTPError); ok && httpErr.Status == 401 {
				return nil, httpErr
			}
		}
		return nil, nil
	}
	credits, ok := usageRecord(creditsBody["credits"])
	if !ok {
		return nil, nil
	}
	monthly, hasMonthly := usageNumber(credits["monthlyCredits"])
	purchased, hasPurchased := usageNumber(credits["purchasedCredits"])
	free, hasFree := usageNumber(credits["freeCredits"])
	if !hasMonthly && !hasPurchased && !hasFree {
		return nil, nil
	}
	limits := make([]UsageLimit, 0, 3)
	windowLimits, _ := usageRecord(creditsBody["windowLimits"])
	for _, spec := range commandCodeWindows {
		if limit := commandCodeBuildWindowLimit(spec, windowLimits[spec.key], userID, orgID); limit != nil {
			limits = append(limits, *limit)
		}
	}
	balance := 0.0
	if hasMonthly {
		balance += monthly
	}
	if hasPurchased {
		balance += purchased
	}
	if hasFree {
		balance += free
	}
	balanceScope := UsageScope{Provider: commandCodeProvider, AccountID: userID, WindowID: "balance", Shared: true}
	if orgID != "" {
		balanceScope.OrgID = orgID
	}
	limits = append(limits, UsageLimit{
		ID:     commandCodeProvider + ":balance",
		Label:  "Credit balance",
		Scope:  balanceScope,
		Amount: UsageAmount{Remaining: &balance, Unit: UsageUnitCredits},
	})
	email, _ := usageString(user, "email")
	metadata := map[string]any{"accountId": userID}
	if email != "" {
		metadata["email"] = email
	}
	if orgID != "" {
		metadata["orgId"] = orgID
	}
	if orgLogin != "" {
		metadata["orgName"] = orgLogin
	}
	if planID, ok := usageString(credits, "planId"); ok {
		metadata["planType"] = planID
	}
	return &UsageReport{
		Provider:  commandCodeProvider,
		FetchedAt: timeNowMillis(),
		Limits:    limits,
		Metadata:  metadata,
		Raw:       map[string]any{"whoami": whoami, "credits": creditsBody},
	}, nil
}

// commandCodeUsageProvider returns the Command Code usage provider.
func commandCodeUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: commandCodeProvider,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == commandCodeProvider && params.Credential.Type == CredentialAPIKey
		},
		FetchUsage:           fetchCommandCodeUsage,
		ValidatesCredentials: true,
	}
}

// --- Devin (Codeium Cascade) ------------------------------------------------

const (
	devinProvider       = "devin"
	devinDefaultBaseURL = "https://server.codeium.com"
	devinUserStatusPath = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	devinSessionPrefix  = "devin-session-token$"
	devinCliVersion     = "3000.11.205"
	devinMicrosPerUSD   = 1_000_000
	devinBillingQuota   = 2
)

// devinCredentialToken reads the stored token of either credential kind.
func devinCredentialToken(credential UsageCredential) string {
	raw := credential.APIKey
	if credential.Type == CredentialOAuth {
		raw = credential.AccessToken
	}
	return trimJSWhitespace(raw)
}

// encodeDevinCredential prefixes a bare JWT-shaped session token.
func encodeDevinCredential(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, devinSessionPrefix) {
		return token
	}
	if len(strings.Split(token, ".")) == 3 {
		return devinSessionPrefix + token
	}
	return token
}

// devinCLIMetadata builds the released-CLI request identity.
func devinCLIMetadata(token string) map[string]string {
	osName := "linux"
	switch runtimeGOOS() {
	case "darwin":
		osName = "darwin"
	case "windows":
		osName = "windows"
	}
	return map[string]string{
		"ideName":          "devin-cli",
		"ideType":          "chisel",
		"ideVersion":       devinCliVersion,
		"extensionName":    "chisel",
		"extensionVersion": devinCliVersion,
		"locale":           "en",
		"os":               osName,
	}
}

// runtimeGOOS returns the host OS.
func runtimeGOOS() string { return runtime.GOOS }

// devinTimestampMs converts a protobuf Timestamp to epoch milliseconds.
func devinTimestampMs(seconds int64, nanos int32) float64 {
	return float64(seconds)*1000 + float64(nanos)/1_000_000
}

// devinTierLabel renders a teams-tier enum as a plan name.
func devinTierLabel(tier int64) string {
	names := map[int64]string{
		1: "Teams", 2: "Pro", 3: "Enterprise Saas", 4: "Hybrid", 5: "Enterprise Self Hosted", 6: "Waitlist Pro",
		7: "Teams Ultimate", 8: "Pro Ultimate", 9: "Trial", 10: "Enterprise Self Serve", 11: "Enterprise Saas Pooled",
		12: "Devin Enterprise", 14: "Devin Teams", 15: "Devin Teams V2", 16: "Devin Pro", 17: "Devin Max",
		18: "Max", 19: "Devin Free", 20: "Devin Trial",
	}
	return names[tier]
}

// devinPbWriter writes protobuf wire bytes for the GetUserStatusRequest.
type devinPbWriter struct {
	buffer []byte
}

// appendVarint appends a base-128 varint.
func (w *devinPbWriter) appendVarint(value uint64) {
	for value >= 0x80 {
		w.buffer = append(w.buffer, byte(value)|0x80)
		value >>= 7
	}
	w.buffer = append(w.buffer, byte(value))
}

// appendTag appends a field tag.
func (w *devinPbWriter) appendTag(field int, wireType int) {
	w.appendVarint(uint64(field)<<3 | uint64(wireType))
}

// appendString appends one length-delimited string field.
func (w *devinPbWriter) appendString(field int, value string) {
	if value == "" {
		return
	}
	w.appendTag(field, 2)
	w.appendVarint(uint64(len(value)))
	w.buffer = append(w.buffer, value...)
}

// encodeDevinUserStatusRequest builds the GetUserStatusRequest message: the
// CLI identity metadata (field 1) carries the session token.
func encodeDevinUserStatusRequest(token string) []byte {
	metadata := &devinPbWriter{}
	metadata.appendString(1, "devin-cli")                  // ideName
	metadata.appendString(28, "chisel")                    // ideType
	metadata.appendString(7, devinCliVersion)              // ideVersion
	metadata.appendString(12, "chisel")                    // extensionName
	metadata.appendString(2, devinCliVersion)              // extensionVersion
	metadata.appendString(3, encodeDevinCredential(token)) // apiKey
	metadata.appendString(4, "en")                         // locale
	osName := devinCLIMetadata(token)["os"]
	metadata.appendString(5, osName) // os
	metadata.appendString(21, "")    // userJwt (empty)
	request := &devinPbWriter{}
	request.appendTag(1, 2)
	request.appendVarint(uint64(len(metadata.buffer)))
	request.buffer = append(request.buffer, metadata.buffer...)
	return request.buffer
}

// devinPbReader walks protobuf wire bytes.
type devinPbReader struct {
	data  []byte
	index int
}

// readVarint reads one base-128 varint.
func (r *devinPbReader) readVarint() (uint64, bool) {
	var result uint64
	var shift uint
	for {
		if r.index >= len(r.data) {
			return 0, false
		}
		value := r.data[r.index]
		r.index++
		result |= uint64(value&0x7f) << shift
		if value < 0x80 {
			return result, true
		}
		shift += 7
		if shift > 63 {
			return 0, false
		}
	}
}

// readBytes reads one length-delimited payload.
func (r *devinPbReader) readBytes() ([]byte, bool) {
	length, ok := r.readVarint()
	if !ok || uint64(len(r.data)-r.index) < length {
		return nil, false
	}
	payload := r.data[r.index : r.index+int(length)]
	r.index += int(length)
	return payload, true
}

// devinSkippedField advances past one field of an unknown wire type.
func (r *devinPbReader) skip(wireType int) bool {
	switch wireType {
	case 0:
		_, ok := r.readVarint()
		return ok
	case 1:
		if len(r.data)-r.index < 8 {
			return false
		}
		r.index += 8
		return true
	case 2:
		_, ok := r.readBytes()
		return ok
	case 5:
		if len(r.data)-r.index < 4 {
			return false
		}
		r.index += 4
		return true
	}
	return false
}

// devinTimestamp is a decoded Timestamp.
type devinTimestamp struct {
	seconds int64
	nanos   int32
	set     bool
}

// devinPlanStatus is the decoded plan status subset usage needs.
type devinPlanStatus struct {
	planStart                   *devinTimestamp
	planEnd                     *devinTimestamp
	availablePromptCredits      int64
	availableFlowCredits        int64
	availableFlexCredits        int64
	usedFlexCredits             int64
	usedFlowCredits             int64
	usedPromptCredits           int64
	dailyQuotaRemainingPercent  float64
	weeklyQuotaRemainingPercent float64
	overageBalanceMicros        int64
	dailyQuotaResetAtUnix       int64
	weeklyQuotaResetAtUnix      int64
	planInfo                    *devinPlanInfo
}

// devinPlanInfo is the decoded plan info subset usage needs.
type devinPlanInfo struct {
	planName                  string
	teamsTier                 int64
	monthlyPromptCredits      float64
	monthlyFlowCredits        float64
	monthlyFlexCreditPurchase float64
	billingStrategy           int64
	hideDailyQuota            bool
	hideWeeklyQuota           bool
	orgID                     string
	accountDisplayName        string
}

// devinUserStatus is the decoded user status subset usage needs.
type devinUserStatus struct {
	email      string
	userID     string
	teamID     string
	teamsTier  int64
	planStatus *devinPlanStatus
}

// decodeDevinTimestamp reads a Timestamp message.
func decodeDevinTimestamp(data []byte) *devinTimestamp {
	reader := &devinPbReader{data: data}
	timestamp := &devinTimestamp{set: true}
	for reader.index < len(reader.data) {
		tag, ok := reader.readVarint()
		if !ok {
			return nil
		}
		field := int(tag >> 3)
		wireType := int(tag & 7)
		switch {
		case field == 1 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			timestamp.seconds = int64(value)
		case field == 2 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			timestamp.nanos = int32(value)
		default:
			if !reader.skip(wireType) {
				return nil
			}
		}
	}
	return timestamp
}

// decodeDevinPlanInfo reads a PlanInfo message.
func decodeDevinPlanInfo(data []byte) *devinPlanInfo {
	reader := &devinPbReader{data: data}
	info := &devinPlanInfo{}
	for reader.index < len(reader.data) {
		tag, ok := reader.readVarint()
		if !ok {
			return nil
		}
		field := int(tag >> 3)
		wireType := int(tag & 7)
		switch {
		case field == 1 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.teamsTier = int64(value)
		case field == 2 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			info.planName = string(payload)
		case field == 12 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.monthlyPromptCredits = float64(int64(value))
		case field == 13 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.monthlyFlowCredits = float64(int64(value))
		case field == 14 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.monthlyFlexCreditPurchase = float64(int64(value))
		case field == 33 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			devin := &devinPbReader{data: payload}
			for devin.index < len(devin.data) {
				nestedTag, ok := devin.readVarint()
				if !ok {
					return nil
				}
				nestedField := int(nestedTag >> 3)
				nestedWire := int(nestedTag & 7)
				switch {
				case nestedField == 4 && nestedWire == 2:
					value, ok := devin.readBytes()
					if !ok {
						return nil
					}
					info.orgID = string(value)
				case nestedField == 8 && nestedWire == 2:
					value, ok := devin.readBytes()
					if !ok {
						return nil
					}
					info.accountDisplayName = string(value)
				default:
					if !devin.skip(nestedWire) {
						return nil
					}
				}
			}
		case field == 35 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.billingStrategy = int64(value)
		case field == 36 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.hideDailyQuota = value != 0
		case field == 37 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			info.hideWeeklyQuota = value != 0
		default:
			if !reader.skip(wireType) {
				return nil
			}
		}
	}
	return info
}

// decodeDevinPlanStatus reads a PlanStatus message.
func decodeDevinPlanStatus(data []byte) *devinPlanStatus {
	reader := &devinPbReader{data: data}
	status := &devinPlanStatus{}
	for reader.index < len(reader.data) {
		tag, ok := reader.readVarint()
		if !ok {
			return nil
		}
		field := int(tag >> 3)
		wireType := int(tag & 7)
		readInt := func() (int64, bool) {
			value, ok := reader.readVarint()
			return int64(value), ok
		}
		switch {
		case field == 1 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.planInfo = decodeDevinPlanInfo(payload)
		case field == 2 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.planStart = decodeDevinTimestamp(payload)
		case field == 3 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.planEnd = decodeDevinTimestamp(payload)
		case field == 8 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.availablePromptCredits = value
		case field == 9 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.availableFlowCredits = value
		case field == 4 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.availableFlexCredits = value
		case field == 7 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.usedFlexCredits = value
		case field == 5 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.usedFlowCredits = value
		case field == 6 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.usedPromptCredits = value
		case field == 14 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.dailyQuotaRemainingPercent = float64(value)
		case field == 15 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.weeklyQuotaRemainingPercent = float64(value)
		case field == 16 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.overageBalanceMicros = value
		case field == 17 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.dailyQuotaResetAtUnix = value
		case field == 18 && wireType == 0:
			value, ok := readInt()
			if !ok {
				return nil
			}
			status.weeklyQuotaResetAtUnix = value
		default:
			if !reader.skip(wireType) {
				return nil
			}
		}
	}
	return status
}

// decodeDevinUserStatus reads a UserStatus message.
func decodeDevinUserStatus(data []byte) *devinUserStatus {
	reader := &devinPbReader{data: data}
	status := &devinUserStatus{}
	for reader.index < len(reader.data) {
		tag, ok := reader.readVarint()
		if !ok {
			return nil
		}
		field := int(tag >> 3)
		wireType := int(tag & 7)
		switch {
		case field == 7 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.email = string(payload)
		case field == 5 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.teamID = string(payload)
		case field == 10 && wireType == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			status.teamsTier = int64(value)
		case field == 13 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.planStatus = decodeDevinPlanStatus(payload)
		case field == 36 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			status.userID = string(payload)
		default:
			if !reader.skip(wireType) {
				return nil
			}
		}
	}
	return status
}

// decodeDevinUserStatusResponse reads a GetUserStatusResponse message.
func decodeDevinUserStatusResponse(data []byte) (*devinUserStatus, *devinPlanInfo) {
	reader := &devinPbReader{data: data}
	var status *devinUserStatus
	var planInfo *devinPlanInfo
	for reader.index < len(reader.data) {
		tag, ok := reader.readVarint()
		if !ok {
			return nil, nil
		}
		field := int(tag >> 3)
		wireType := int(tag & 7)
		switch {
		case field == 1 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil, nil
			}
			status = decodeDevinUserStatus(payload)
		case field == 2 && wireType == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil, nil
			}
			planInfo = decodeDevinPlanInfo(payload)
		default:
			if !reader.skip(wireType) {
				return nil, nil
			}
		}
	}
	return status, planInfo
}

// devinPlanWindow renders the billing-cycle window.
func devinPlanWindow(status *devinPlanStatus) UsageWindow {
	window := UsageWindow{ID: "monthly", Label: "Plan Period"}
	if status == nil {
		return window
	}
	var start, end int64
	hasStart, hasEnd := false, false
	if status.planStart != nil && status.planStart.set {
		start, hasStart = int64(devinTimestampMs(status.planStart.seconds, status.planStart.nanos)), true
	}
	if status.planEnd != nil && status.planEnd.set {
		end, hasEnd = int64(devinTimestampMs(status.planEnd.seconds, status.planEnd.nanos)), true
	}
	if hasStart && hasEnd && end > start {
		duration := end - start
		window.DurationMs = &duration
	}
	if hasEnd && end > 0 {
		window.ResetsAt = &end
	}
	return window
}

// devinCreditBucket is one credit meter.
type devinCreditBucket struct {
	id        string
	label     string
	limit     float64
	used      float64
	available float64
}

// devinCreditLimit renders one credit bucket.
func devinCreditLimit(bucket devinCreditBucket, window UsageWindow, scope UsageScope) *UsageLimit {
	var limit *float64
	if bucket.limit > 0 {
		limit = &bucket.limit
	}
	used := math.Max(0, bucket.used)
	remaining := math.Max(0, bucket.available)
	if limit == nil && used == 0 && remaining == 0 {
		return nil
	}
	var fraction *float64
	if limit != nil {
		value := used / *limit
		fraction = &value
	}
	amount := UsageAmount{Used: &used, Remaining: &remaining, Unit: UsageUnitUnknown}
	if limit != nil {
		amount.Limit = limit
	}
	if fraction != nil {
		remainingFraction := math.Max(0, 1-*fraction)
		amount.UsedFraction = fraction
		amount.RemainingFraction = &remainingFraction
	}
	scope.WindowID = window.ID
	return &UsageLimit{
		ID:     "devin:credits:" + bucket.id,
		Label:  bucket.label,
		Scope:  scope,
		Window: &window,
		Amount: amount,
		Status: usageStatusFor(fraction),
	}
}

// devinQuotaWindow is one percent quota window.
type devinQuotaWindow struct {
	id               string
	label            string
	windowID         string
	duration         int64
	remainingPercent float64
	resetAtUnix      int64
	hidden           bool
}

// devinQuotaApplies reports whether a quota window is meaningful. Credit-billed
// plans leave the percents at their proto defaults, which would read as fully
// consumed.
func devinQuotaApplies(quota devinQuotaWindow, plan *devinPlanInfo) bool {
	if quota.hidden {
		return false
	}
	if quota.resetAtUnix > 0 {
		return true
	}
	return plan != nil && plan.billingStrategy == devinBillingQuota
}

// devinQuotaLimit renders a percent quota window.
func devinQuotaLimit(quota devinQuotaWindow, scope UsageScope) UsageLimit {
	remaining := math.Max(0, math.Min(100, quota.remainingPercent))
	used := 100 - remaining
	fraction := used / 100
	remainingFraction := remaining / 100
	resetsAt, hasReset := usagePositiveTimestamp(float64(quota.resetAtUnix))
	duration := quota.duration
	window := &UsageWindow{ID: quota.windowID, Label: quota.label, DurationMs: &duration}
	if hasReset {
		window.ResetsAt = &resetsAt
	}
	scope.WindowID = quota.windowID
	return UsageLimit{
		ID:     "devin:quota:" + quota.id,
		Label:  quota.label,
		Scope:  scope,
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

// buildDevinReport renders a decoded GetUserStatusResponse.
func buildDevinReport(status *devinUserStatus, rootPlan *devinPlanInfo) *UsageReport {
	if status == nil {
		return nil
	}
	planStatus := status.planStatus
	// The response-level plan_info is authoritative; the nested one only fills
	// in for older servers.
	plan := rootPlan
	if plan == nil && planStatus != nil {
		plan = planStatus.planInfo
	}
	email := trimJSWhitespace(status.email)
	accountID := trimJSWhitespace(status.userID)
	orgID := ""
	orgName := ""
	planName := ""
	if plan != nil {
		orgID = trimJSWhitespace(plan.orgID)
		orgName = trimJSWhitespace(plan.accountDisplayName)
		planName = trimJSWhitespace(plan.planName)
	}
	if orgID == "" {
		orgID = trimJSWhitespace(status.teamID)
	}
	if planName == "" {
		planName = devinTierLabel(status.teamsTier)
	}
	scope := UsageScope{Provider: devinProvider, AccountID: accountID, OrgID: orgID, Tier: planName}
	limits := make([]UsageLimit, 0, 5)
	if planStatus != nil {
		window := devinPlanWindow(planStatus)
		buckets := []devinCreditBucket{
			{id: "prompt", label: "Prompt Credits", used: float64(planStatus.usedPromptCredits), available: float64(planStatus.availablePromptCredits)},
			{id: "flow", label: "Flow Credits", used: float64(planStatus.usedFlowCredits), available: float64(planStatus.availableFlowCredits)},
			{id: "flex", label: "Flex Credits", used: float64(planStatus.usedFlexCredits), available: float64(planStatus.availableFlexCredits)},
		}
		if plan != nil {
			buckets[0].limit = plan.monthlyPromptCredits
			buckets[1].limit = plan.monthlyFlowCredits
			buckets[2].limit = plan.monthlyFlexCreditPurchase
		}
		for _, bucket := range buckets {
			if limit := devinCreditLimit(bucket, window, scope); limit != nil {
				limits = append(limits, *limit)
			}
		}
		quotas := []devinQuotaWindow{
			{id: "daily", label: "Daily Quota", windowID: "1d", duration: usageDayMs, remainingPercent: planStatus.dailyQuotaRemainingPercent, resetAtUnix: planStatus.dailyQuotaResetAtUnix},
			{id: "weekly", label: "Weekly Quota", windowID: "7d", duration: usageWeekMs, remainingPercent: planStatus.weeklyQuotaRemainingPercent, resetAtUnix: planStatus.weeklyQuotaResetAtUnix},
		}
		if plan != nil {
			quotas[0].hidden = plan.hideDailyQuota
			quotas[1].hidden = plan.hideWeeklyQuota
		}
		for _, quota := range quotas {
			if devinQuotaApplies(quota, plan) {
				limits = append(limits, devinQuotaLimit(quota, scope))
			}
		}
	}
	metadata := map[string]any{"source": "seat-management"}
	if email != "" {
		metadata["email"] = email
	}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	if orgID != "" {
		metadata["orgId"] = orgID
	}
	if orgName != "" {
		metadata["orgName"] = orgName
	}
	if planName != "" {
		metadata["planType"] = planName
	}
	report := &UsageReport{Provider: devinProvider, FetchedAt: timeNowMillis(), Limits: limits, Metadata: metadata}
	if planStatus != nil && planStatus.planEnd != nil && planStatus.planEnd.set {
		planEnd := int64(devinTimestampMs(planStatus.planEnd.seconds, planStatus.planEnd.nanos))
		if planEnd > 0 {
			metadata["planEnd"] = planEnd
		}
	}
	if planStatus != nil {
		overageUSD := float64(planStatus.overageBalanceMicros) / devinMicrosPerUSD
		if overageUSD != 0 {
			metadata["overageBalanceUsd"] = overageUSD
			report.Notes = []string{"Overage balance: $" + usageFormatFixed(overageUSD, 2)}
		}
	}
	return report
}

// fetchDevinUsage issues the GetUserStatus Connect RPC.
func fetchDevinUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != devinProvider {
		return nil, nil
	}
	token := devinCredentialToken(params.Credential)
	if token == "" {
		return nil, nil
	}
	baseURL := usageTrimTrailingSlashes(params.BaseURL)
	if baseURL == "" {
		baseURL = devinDefaultBaseURL
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "POST", baseURL+devinUserStatusPath, map[string]string{
		"content-type":             "application/proto",
		"connect-protocol-version": "1",
		"accept":                   "*/*",
	}, encodeDevinUserStatusRequest(token))
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		usageLogWarn(fctx, "Devin user status request failed", map[string]any{"provider": params.Provider, "error": err.Error()})
		return nil, nil
	}
	if !response.OK() {
		usageLogWarn(fctx, "Devin user status fetch failed", map[string]any{"provider": params.Provider, "status": response.Status})
		return nil, nil
	}
	status, planInfo := decodeDevinUserStatusResponse(response.Body)
	report := buildDevinReport(status, planInfo)
	if report == nil {
		usageLogWarn(fctx, "Devin user status response invalid", map[string]any{"provider": params.Provider})
		return nil, nil
	}
	return report, nil
}

// devinUsageProvider returns the Devin usage provider.
func devinUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: devinProvider,
		Supports: func(params UsageFetchParams) bool {
			return params.Provider == devinProvider && devinCredentialToken(params.Credential) != ""
		},
		FetchUsage:           fetchDevinUsage,
		ValidatesCredentials: true,
	}
}
