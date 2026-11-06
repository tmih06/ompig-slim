package ai

// Ports packages/ai/src/usage/cursor.ts: the legacy `/auth/usage` request-count
// API, the current `/api/usage-summary` dashboard buckets, and the X
// Subscription weekly pool/prepaid-credit split.

import (
	"context"
	"math"
	"strings"
)

const (
	cursorUsageDefaultBaseURL = "https://api2.cursor.sh"
	cursorPlanInfoURL         = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetPlanInfo"
	cursorUsageSummaryURL     = "https://cursor.com/api/usage-summary"
	cursorModelsLimitID       = "cursor:usd:individual-auto"
	cursorOtherModelsID       = "cursor:usd:individual-api"
)

// cursorUsageConnectHeaders is the unary Connect RPC identity for usage
// probes. It is local to this file so the usage package never depends on the
// OAuth module's request builder.
func cursorUsageConnectHeaders(accessToken string) map[string]string {
	return map[string]string{
		"Accept":                   "application/json",
		"Authorization":            "Bearer " + accessToken,
		"Connect-Protocol-Version": "1",
		"Content-Type":             "application/json",
	}
}

// cursorParseTimestamp reads an epoch seconds/millis number or ISO string.
func cursorParseTimestamp(value any) *int64 {
	if number, ok := usageNumber(value); ok {
		millis := int64(number)
		if number < 1_000_000_000_000 {
			millis = int64(number * 1000)
		}
		return &millis
	}
	if millis, ok := usageIsoTimestamp(value); ok {
		return &millis
	}
	return nil
}

// normalizeCursorBaseURL strips trailing slashes from the RPC host.
func normalizeCursorBaseURL(baseURL string) string {
	if baseURL == "" {
		return cursorUsageDefaultBaseURL
	}
	return usageTrimTrailingSlashes(baseURL)
}

// cursorDeriveResetsAt finds the bucket end, else one month after its start.
func cursorDeriveResetsAt(record map[string]any) *int64 {
	for _, key := range []string{"billingCycleEnd", "endOfMonth", "resetsAt", "nextReset"} {
		if millis := cursorParseTimestamp(record[key]); millis != nil {
			return millis
		}
	}
	for _, key := range []string{"startOfMonth", "billingCycleStart", "startOfBillingCycle"} {
		if millis := cursorParseTimestamp(record[key]); millis != nil {
			return new(usageAddUTCMonth(*millis))
		}
	}
	return nil
}

// usageAddUTCMonth advances an epoch-millisecond timestamp by one UTC month.
func usageAddUTCMonth(millis int64) int64 {
	value := timeFromMillis(millis).UTC()
	return value.AddDate(0, 1, 0).UnixMilli()
}

// parseCursorCentsBucket reads a cents bucket. Disabled or non-positive
// buckets report nil.
func parseCursorCentsBucket(bucket map[string]any) *UsageAmount {
	if disabled, ok := usageBool(bucket, "enabled"); ok && !disabled {
		return nil
	}
	reportedUsed, hasUsed := usageNumber(bucket["used"])
	reportedRemaining, hasRemaining := usageNumber(bucket["remaining"])
	validUsed := hasUsed && reportedUsed >= 0
	validRemaining := hasRemaining && reportedRemaining >= 0
	rawLimit, hasLimit := usageNumber(bucket["limit"])
	limitValue := bucket["limit"]

	if limitValue == nil {
		if !validUsed {
			return nil
		}
		usedUSD := reportedUsed / 100
		return &UsageAmount{Used: &usedUSD, Unit: UsageUnitUSD}
	}
	if !hasLimit || rawLimit <= 0 {
		return nil
	}
	var usedCents float64
	switch {
	case hasUsed && reportedUsed > 0:
		usedCents = reportedUsed
	case validRemaining && reportedRemaining < rawLimit:
		usedCents = math.Max(0, rawLimit-reportedRemaining)
	case validUsed:
		usedCents = reportedUsed
	default:
		return nil
	}
	remainingCents := math.Max(0, rawLimit-usedCents)
	used := usedCents / 100
	limit := rawLimit / 100
	remaining := remainingCents / 100
	fraction := usedCents / rawLimit
	remainingFraction := remainingCents / rawLimit
	return &UsageAmount{
		Used:              &used,
		Limit:             &limit,
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitUSD,
	}
}

// cursorPlanRails is the parsed plan dashboard bucket.
type cursorPlanRails struct {
	auto     *UsageAmount
	api      *UsageAmount
	fallback *UsageAmount
}

// parseCursorPlanDashboardAmounts reads the plan bucket rails.
//
// Cursor's dashboard does not treat plan.used/plan.limit as the visible
// percentage: Pro+ shows separate Cursor Models (autoPercentUsed) and Other
// Models (apiPercentUsed) pools. Both rails stay percentages because Cursor no
// longer publishes the Other Models pool size.
func parseCursorPlanDashboardAmounts(bucket map[string]any) cursorPlanRails {
	var result cursorPlanRails
	if disabled, ok := usageBool(bucket, "enabled"); ok && !disabled {
		return result
	}
	limitCents, hasLimit := usageNumber(bucket["limit"])
	var limitUSD *float64
	if hasLimit && limitCents > 0 {
		value := limitCents / 100
		limitUSD = &value
	}
	fromPercent := func(percent float64, withLimit bool) UsageAmount {
		fraction := math.Max(0, percent) / 100
		if withLimit && limitUSD != nil {
			used := *limitUSD * fraction
			remaining := math.Max(0, *limitUSD-used)
			remainingFraction := math.Max(0, 1-fraction)
			return UsageAmount{
				Used:              &used,
				Limit:             limitUSD,
				Remaining:         &remaining,
				UsedFraction:      &fraction,
				RemainingFraction: &remainingFraction,
				Unit:              UsageUnitUSD,
			}
		}
		used := fraction * 100
		return UsageAmount{Used: &used, UsedFraction: &fraction, Unit: UsageUnitPercent}
	}
	if percent, ok := usageNumber(bucket["autoPercentUsed"]); ok {
		amount := fromPercent(percent, false)
		result.auto = &amount
	}
	if percent, ok := usageNumber(bucket["apiPercentUsed"]); ok {
		amount := fromPercent(percent, false)
		result.api = &amount
	}
	if result.auto == nil && result.api == nil {
		if percent, ok := usageNumber(bucket["totalPercentUsed"]); ok {
			amount := fromPercent(percent, true)
			result.fallback = &amount
		} else if cents := parseCursorCentsBucket(bucket); cents != nil {
			result.fallback = cents
		}
	}
	return result
}

// pushCursorPlanRails appends the plan rails to limits.
func pushCursorPlanRails(limits []UsageLimit, bucket map[string]any, window *UsageWindow) []UsageLimit {
	rails := parseCursorPlanDashboardAmounts(bucket)
	appendRail := func(id, label string, amount *UsageAmount) {
		if amount == nil {
			return
		}
		status := UsageStatus("")
		if amount.UsedFraction != nil {
			status = usageStatusFor(amount.UsedFraction)
		}
		limits = append(limits, UsageLimit{
			ID:     id,
			Label:  label,
			Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
			Window: window,
			Amount: *amount,
			Status: status,
		})
	}
	appendRail("cursor:usd:individual-auto", "Cursor Models", rails.auto)
	appendRail("cursor:usd:individual-api", "Other Models", rails.api)
	appendRail("cursor:usd:individual-plan", "Personal Usage", rails.fallback)
	return limits
}

// cursorNoPrepaidCredit marks a disabled or empty prepaid balance.
func cursorNoPrepaidCredit() UsageAmount {
	zero := 0.0
	one := 1.0
	return UsageAmount{Used: &zero, Limit: &zero, Remaining: &zero, UsedFraction: &one, RemainingFraction: &zero, Unit: UsageUnitUSD}
}

// cursorIsUnifiedMembership reports the X Subscription billing shape.
func cursorIsUnifiedMembership(payload map[string]any) bool {
	membership, ok := usageString(payload, "membershipType")
	return ok && strings.HasPrefix(membership, "unified_")
}

// parseCursorUnifiedPool reads one weekly pool amount.
func parseCursorUnifiedPool(plan map[string]any) *UsageAmount {
	if disabled, ok := usageBool(plan, "enabled"); ok && !disabled {
		return nil
	}
	limitCents, hasLimit := usageNumber(plan["limit"])
	usedCents, hasUsed := usageNumber(plan["used"])
	percent, hasPercent := usageNumber(plan["totalPercentUsed"])
	if !hasLimit || limitCents <= 0 {
		if !hasPercent {
			return nil
		}
		fraction := math.Max(0, percent) / 100
		used := math.Max(0, percent)
		return &UsageAmount{Used: &used, UsedFraction: &fraction, Unit: UsageUnitPercent}
	}
	fraction := math.Max(func() float64 {
		if hasPercent {
			return percent / 100
		}
		return 0
	}(), func() float64 {
		if hasUsed {
			return usedCents / limitCents
		}
		return 0
	}())
	fraction = math.Max(fraction, 0)
	limit := limitCents / 100
	used := math.Min(limit, limit*fraction)
	remaining := limit - used
	remainingFraction := math.Max(0, 1-fraction)
	return &UsageAmount{
		Used:              &used,
		Limit:             &limit,
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitUSD,
	}
}

// parseCursorUnifiedUsage renders the X Subscription meters.
func parseCursorUnifiedUsage(individualUsage map[string]any, weeklyResetsAt *int64) []UsageLimit {
	limits := make([]UsageLimit, 0, 2)
	if plan, ok := usageRecord(individualUsage["plan"]); ok {
		if pool := parseCursorUnifiedPool(plan); pool != nil {
			duration := 7 * usageDayMs
			window := &UsageWindow{ID: "weekly", Label: "Weekly", DurationMs: &duration, ResetsAt: weeklyResetsAt}
			status := UsageStatus("")
			if pool.UsedFraction != nil {
				status = usageStatusFor(pool.UsedFraction)
			}
			limits = append(limits, UsageLimit{
				ID:     cursorModelsLimitID,
				Label:  "Cursor Models",
				Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
				Window: window,
				Amount: *pool,
				Status: status,
			})
		}
	}
	if onDemand, ok := usageRecord(individualUsage["onDemand"]); ok {
		var credit *UsageAmount
		disabled, hasDisabled := usageBool(onDemand, "enabled")
		limitCents, hasLimit := usageNumber(onDemand["limit"])
		if hasDisabled && !disabled || hasLimit && limitCents == 0 {
			amount := cursorNoPrepaidCredit()
			credit = &amount
		} else {
			credit = parseCursorCentsBucket(onDemand)
		}
		if credit != nil {
			// Prepaid credit has no calendar reset.
			window := &UsageWindow{ID: "prepaid", Label: "Prepaid credit"}
			status := UsageStatus("")
			if credit.UsedFraction != nil {
				status = usageStatusFor(credit.UsedFraction)
			}
			limits = append(limits, UsageLimit{
				ID:     cursorOtherModelsID,
				Label:  "Other Models (prepaid credit)",
				Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
				Window: window,
				Amount: *credit,
				Status: status,
			})
		}
	}
	return limits
}

// parseCursorIndividualUsage parses `/api/usage-summary`.
func parseCursorIndividualUsage(payload any, fetchedAt int64, weeklyResetsAt *int64) *UsageReport {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	individualUsage, ok := usageRecord(record["individualUsage"])
	if !ok {
		return nil
	}
	resetsAt := cursorDeriveResetsAt(record)
	window := &UsageWindow{ID: "monthly", Label: "Monthly", ResetsAt: resetsAt}
	if cursorIsUnifiedMembership(record) {
		limits := parseCursorUnifiedUsage(individualUsage, weeklyResetsAt)
		if len(limits) == 0 {
			return nil
		}
		return &UsageReport{Provider: "cursor", FetchedAt: fetchedAt, Limits: limits, Raw: payload}
	}
	limits := make([]UsageLimit, 0, 3)
	usedOverall := false
	if overall, ok := usageRecord(individualUsage["overall"]); ok {
		if amount := parseCursorCentsBucket(overall); amount != nil {
			usedOverall = true
			status := UsageStatus("")
			if amount.UsedFraction != nil {
				status = usageStatusFor(amount.UsedFraction)
			}
			limits = append(limits, UsageLimit{
				ID:     "cursor:usd:individual-overall",
				Label:  "Personal Usage",
				Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
				Window: window,
				Amount: *amount,
				Status: status,
			})
		}
	}
	if !usedOverall {
		if plan, ok := usageRecord(individualUsage["plan"]); ok {
			limits = pushCursorPlanRails(limits, plan, window)
		}
	}
	// Keep on-demand even when the included bucket is absent or unusable.
	if onDemand, ok := usageRecord(individualUsage["onDemand"]); ok {
		if amount := parseCursorCentsBucket(onDemand); amount != nil && amount.Limit != nil && *amount.Limit > 0 {
			status := UsageStatus("")
			if amount.UsedFraction != nil {
				status = usageStatusFor(amount.UsedFraction)
			}
			limits = append(limits, UsageLimit{
				ID:     "cursor:usd:individual-ondemand",
				Label:  "On-Demand Usage",
				Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
				Window: window,
				Amount: *amount,
				Status: status,
			})
		}
	}
	if len(limits) == 0 {
		return nil
	}
	return &UsageReport{Provider: "cursor", FetchedAt: fetchedAt, Limits: limits, Raw: payload}
}

// parseCursorUsage parses the legacy `/auth/usage` request-count API.
func parseCursorUsage(payload any, fetchedAt int64) *UsageReport {
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	limits := make([]UsageLimit, 0, 4)
	resetsAt := cursorDeriveResetsAt(record)
	window := &UsageWindow{ID: "monthly", Label: "Monthly", ResetsAt: resetsAt}
	for key, value := range record {
		bucket, ok := usageRecord(value)
		if !ok {
			continue
		}
		var usedVal *float64
		for _, field := range []string{"numRequests", "used", "amountUsed", "usdUsed"} {
			if number, ok := usageNumber(bucket[field]); ok {
				usedVal = &number
				break
			}
		}
		var limitVal *float64
		for _, field := range []string{"maxRequestUsage", "limit", "amountLimit", "usdLimit"} {
			if number, ok := usageNumber(bucket[field]); ok {
				limitVal = &number
				break
			}
		}
		if usedVal == nil {
			continue
		}
		lower := strings.ToLower(key)
		isUSD := key == "planUsage" || strings.Contains(lower, "usd") ||
			strings.Contains(lower, "billing") || strings.Contains(lower, "stripe")
		unit := UsageUnitRequests
		limitID := "cursor:requests:" + trimJSWhitespace(lower)
		label := key + " requests"
		if isUSD {
			unit = UsageUnitUSD
			limitID = "cursor:usd:" + trimJSWhitespace(lower)
			label = key + " spend"
		}
		// Plans without a legacy numeric cap get an uncapped meter rather than
		// being dropped.
		if limitVal == nil {
			limits = append(limits, UsageLimit{
				ID:     limitID,
				Label:  label,
				Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
				Window: window,
				Amount: UsageAmount{Used: usedVal, Unit: unit},
			})
			continue
		}
		remaining := math.Max(0, *limitVal-*usedVal)
		fraction := 0.0
		remainingFraction := 0.0
		if *limitVal > 0 {
			fraction = *usedVal / *limitVal
			remainingFraction = remaining / *limitVal
		}
		limits = append(limits, UsageLimit{
			ID:     limitID,
			Label:  label,
			Scope:  UsageScope{Provider: "cursor", WindowID: window.ID},
			Window: window,
			Amount: UsageAmount{
				Used:              usedVal,
				Limit:             limitVal,
				Remaining:         &remaining,
				UsedFraction:      &fraction,
				RemainingFraction: &remainingFraction,
				Unit:              unit,
			},
			Status: usageStatusFor(&fraction),
		})
	}
	if len(limits) == 0 {
		return nil
	}
	return &UsageReport{Provider: "cursor", FetchedAt: fetchedAt, Limits: limits, Raw: payload}
}

// cursorFetchJSON performs one Cursor request, reporting !ok on any failure.
func cursorFetchJSON(ctx context.Context, fctx UsageFetchContext, endpoint, method string, headers map[string]string) (any, bool) {
	if method == "" {
		method = "GET"
	}
	var body []byte
	if method == "POST" {
		body = []byte("{}")
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, method, endpoint, headers, body)
	if err != nil || !response.OK() {
		return nil, false
	}
	return response.JSON()
}

// cursorFetchWeeklyResetsAt reads the X Subscription pool's next reset.
func cursorFetchWeeklyResetsAt(ctx context.Context, fctx UsageFetchContext, accessToken string) *int64 {
	payload, ok := cursorFetchJSON(ctx, fctx, cursorPlanInfoURL, "POST", cursorUsageConnectHeaders(accessToken))
	if !ok {
		return nil
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil
	}
	planInfo, ok := usageRecord(record["planInfo"])
	if !ok {
		return nil
	}
	return cursorParseTimestamp(planInfo["includedUsageResetsAt"])
}

// fetchCursorUsage probes both Cursor usage surfaces and merges them.
func fetchCursorUsage(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error) {
	if params.Provider != "cursor" {
		return nil, nil
	}
	credential := params.Credential
	token := credential.APIKey
	if credential.Type == CredentialOAuth {
		token = credential.AccessToken
	}
	if token == "" {
		return nil, nil
	}
	baseURL := normalizeCursorBaseURL(params.BaseURL)
	if baseURL == cursorUsageDefaultBaseURL && credential.APIEndpoint != "" {
		baseURL = normalizeCursorBaseURL(credential.APIEndpoint)
	}
	fetchedAt := timeNowMillis()
	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + token,
	}

	legacyPayload, legacyOK := cursorFetchJSON(ctx, fctx, baseURL+"/auth/usage", "GET", headers)
	var legacyReport *UsageReport
	if legacyOK {
		legacyReport = parseCursorUsage(legacyPayload, fetchedAt)
	}

	var summaryReport *UsageReport
	email := ""
	if credential.Type == CredentialOAuth && baseURL == cursorUsageDefaultBaseURL {
		if userID := ExtractCursorAccessTokenUserID(token); userID != "" {
			if payload, ok := cursorFetchJSON(ctx, fctx, cursorUsageSummaryURL, "GET", CursorSessionHeaders(userID, token)); ok {
				var weeklyResetsAt *int64
				if record, ok := usageRecord(payload); ok && cursorIsUnifiedMembership(record) {
					// The X Subscription's weekly pool reset is not in the summary.
					weeklyResetsAt = cursorFetchWeeklyResetsAt(ctx, fctx, token)
				}
				summaryReport = parseCursorIndividualUsage(payload, fetchedAt, weeklyResetsAt)
			}
			if resolved, err := FetchCursorAccountEmail(ctx, token); err == nil {
				email = resolved
			}
		}
	}

	var report *UsageReport
	switch {
	case legacyReport != nil && summaryReport != nil:
		// `/auth/usage` is the pre-usage-plan request-count API. Beside the
		// summary's dollar rails an uncapped, unused bucket carries nothing, so it
		// is dropped; capped buckets and uncapped buckets with requests stay.
		legacyLimits := make([]UsageLimit, 0, len(legacyReport.Limits))
		for _, limit := range legacyReport.Limits {
			used := 0.0
			if limit.Amount.Used != nil {
				used = *limit.Amount.Used
			}
			if limit.Amount.Limit != nil || used > 0 {
				legacyLimits = append(legacyLimits, limit)
			}
		}
		report = &UsageReport{
			Provider:  "cursor",
			FetchedAt: fetchedAt,
			Limits:    append(legacyLimits, summaryReport.Limits...),
			Raw:       map[string]any{"authUsage": legacyReport.Raw, "usageSummary": summaryReport.Raw},
		}
	case legacyReport != nil:
		report = legacyReport
	default:
		report = summaryReport
	}
	if report == nil {
		return nil, nil
	}
	if email == "" {
		email = trimJSWhitespace(credential.Email)
	}
	metadata := map[string]any{}
	if email != "" {
		metadata["email"] = email
	}
	if credential.AccountID != "" {
		metadata["accountId"] = credential.AccountID
	}
	if credential.ProjectID != "" {
		metadata["projectId"] = credential.ProjectID
	}
	if len(metadata) > 0 {
		report.Metadata = metadata
	}
	return report, nil
}

// cursorUsageProvider returns the Cursor usage provider.
func cursorUsageProvider() *UsageProvider {
	return &UsageProvider{
		ID: "cursor",
		Supports: func(params UsageFetchParams) bool {
			if params.Provider != "cursor" {
				return false
			}
			if params.Credential.Type == CredentialOAuth {
				return params.Credential.AccessToken != ""
			}
			return params.Credential.Type == CredentialAPIKey && params.Credential.APIKey != ""
		},
		FetchUsage: fetchCursorUsage,
	}
}
