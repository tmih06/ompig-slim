package ai

// Ports packages/ai/src/utils/usage.ts and the provider contract of
// packages/ai/src/usage.ts: the normalized quota-report schema shared by the
// usage dashboard and the multi-account quota service, plus the pure helpers
// the display layer resolves limits with.

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// UsageUnit is the unit of one quantitative amount.
type UsageUnit string

const (
	UsageUnitPercent  UsageUnit = "percent"
	UsageUnitTokens   UsageUnit = "tokens"
	UsageUnitRequests UsageUnit = "requests"
	UsageUnitCredits  UsageUnit = "credits"
	UsageUnitUSD      UsageUnit = "usd"
	UsageUnitMinutes  UsageUnit = "minutes"
	UsageUnitBytes    UsageUnit = "bytes"
	UsageUnitUnknown  UsageUnit = "unknown"
)

// UsageStatus is the normalized health of one limit.
type UsageStatus string

const (
	UsageStatusOK        UsageStatus = "ok"
	UsageStatusWarning   UsageStatus = "warning"
	UsageStatusExhausted UsageStatus = "exhausted"
	UsageStatusUnknown   UsageStatus = "unknown"
)

// UsageWindow is the time window a limit applies to.
type UsageWindow struct {
	// ID is a stable identifier such as "5h", "7d" or "monthly".
	ID string `json:"id"`
	// Label is the human label ("5 Hour", "7 Day").
	Label string `json:"label"`
	// DurationMs is the window length in milliseconds when known.
	DurationMs *int64 `json:"durationMs,omitempty"`
	// ResetsAt is the absolute reset time in epoch milliseconds.
	ResetsAt *int64 `json:"resetsAt,omitempty"`
	// ResetLabel is the verb shown before the countdown ("tick", "regen");
	// empty means "resets".
	ResetLabel string `json:"resetLabel,omitempty"`
}

// UsageAmount is the quantitative data of one limit.
type UsageAmount struct {
	Used              *float64  `json:"used,omitempty"`
	Limit             *float64  `json:"limit,omitempty"`
	Remaining         *float64  `json:"remaining,omitempty"`
	UsedFraction      *float64  `json:"usedFraction,omitempty"`
	RemainingFraction *float64  `json:"remainingFraction,omitempty"`
	Unit              UsageUnit `json:"unit"`
}

// UsageScope describes what a limit applies to.
type UsageScope struct {
	Provider    string `json:"provider"`
	AccountID   string `json:"accountId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	OrgID       string `json:"orgId,omitempty"`
	ModelID     string `json:"modelId,omitempty"`
	Tier        string `json:"tier,omitempty"`
	WindowID    string `json:"windowId,omitempty"`
	Shared      bool   `json:"shared,omitempty"`
	SharedGroup string `json:"sharedGroup,omitempty"`
}

// UsageLimit is one normalized quota window or bucket.
type UsageLimit struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Scope  UsageScope   `json:"scope"`
	Window *UsageWindow `json:"window,omitempty"`
	Amount UsageAmount  `json:"amount"`
	Status UsageStatus  `json:"status,omitempty"`
	Notes  []string     `json:"notes,omitempty"`
}

// UsageResetCreditDetail is one saved banked reset a provider issued.
type UsageResetCreditDetail struct {
	ID             string             `json:"id,omitempty"`
	Title          string             `json:"title,omitempty"`
	Program        string             `json:"program,omitempty"`
	RemainingCount int                `json:"remainingCount,omitempty"`
	Usable         *bool              `json:"usable,omitempty"`
	RequiresLimit  *bool              `json:"requiresLimit,omitempty"`
	Clears         []string           `json:"clears,omitempty"`
	Blocking       []string           `json:"blocking,omitempty"`
	UsedFractions  map[string]float64 `json:"usedFractions,omitempty"`
	GrantedAt      string             `json:"grantedAt,omitempty"`
	ExpiresAt      string             `json:"expiresAt,omitempty"`
	Status         string             `json:"status,omitempty"`
}

// UsageResetCredits is an account's banked rate-limit resets.
type UsageResetCredits struct {
	AvailableCount  int                      `json:"availableCount"`
	RedeemableCount *int                     `json:"redeemableCount,omitempty"`
	NextCreditID    string                   `json:"nextCreditId,omitempty"`
	Eligible        *bool                    `json:"eligible,omitempty"`
	Reason          string                   `json:"reason,omitempty"`
	CooldownUntil   string                   `json:"cooldownUntil,omitempty"`
	Credits         []UsageResetCreditDetail `json:"credits,omitempty"`
}

// UsageReport is one provider account's quota snapshot.
type UsageReport struct {
	Provider     string             `json:"provider"`
	FetchedAt    int64              `json:"fetchedAt"`
	Limits       []UsageLimit       `json:"limits"`
	ResetCredits *UsageResetCredits `json:"resetCredits,omitempty"`
	Notes        []string           `json:"notes,omitempty"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
	Raw          any                `json:"raw,omitempty"`
}

// UsageLogger is the optional logger handed to usage fetchers.
type UsageLogger interface {
	Debug(message string, meta map[string]any)
	Warn(message string, meta map[string]any)
}

// UsageCredential is the narrow credential bundle a usage fetcher authenticates
// with, built from a stored credential or the ambient auth resolution.
type UsageCredential struct {
	Type                 CredentialType `json:"type"`
	APIKey               string         `json:"apiKey,omitempty"`
	AccessToken          string         `json:"accessToken,omitempty"`
	RefreshToken         string         `json:"refreshToken,omitempty"`
	ExpiresAt            *int64         `json:"expiresAt,omitempty"`
	AccountID            string         `json:"accountId,omitempty"`
	ProjectID            string         `json:"projectId,omitempty"`
	Email                string         `json:"email,omitempty"`
	OrgID                string         `json:"orgId,omitempty"`
	OrgName              string         `json:"orgName,omitempty"`
	EnterpriseURL        string         `json:"enterpriseUrl,omitempty"`
	Region               string         `json:"region,omitempty"`
	InferenceRegion      string         `json:"inferenceRegion,omitempty"`
	ActiveOrganizationID string         `json:"activeOrganizationId,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	APIEndpoint          string         `json:"apiEndpoint,omitempty"`
	token                string         `json:"-"`
}

// UsageFetchParams is what one provider probe is asked to describe.
type UsageFetchParams struct {
	Provider   string
	Credential UsageCredential
	// AccountKey is the stable store identity of the probed account.
	AccountKey *string
	BaseURL    string
}

// UsageFetchContext carries the shared runtime services a fetcher may use.
type UsageFetchContext struct {
	// HTTPClient executes the probe's HTTP requests. Nil means http.DefaultClient.
	HTTPClient *http.Client
	Logger     UsageLogger
	// PreviousReport is the last report cached for this credential, so a fetcher
	// can keep a field it could not re-read.
	PreviousReport *UsageReport
}

// UsageHeaderParser parses a provider's rate-limit response headers.
type UsageHeaderParser func(headers map[string]string, nowMs int64, responseStatus *int) *UsageReport

// UsageProvider is one provider's usage endpoint implementation. Field
// semantics mirror the upstream UsageProvider object: a nil Supports accepts
// every credential, RetainLastGoodOnFailure nil means true, and
// FailureBackoffMs 0 means the service default.
type UsageProvider struct {
	ID string
	// CacheVersion is bumped upstream when cached reports of an older shape
	// must be retired.
	CacheVersion int
	Supports     func(params UsageFetchParams) bool
	FetchUsage   func(ctx context.Context, params UsageFetchParams, fctx UsageFetchContext) (*UsageReport, error)
	// ParseRateLimitHeaders parses lowercased response headers into a report.
	ParseRateLimitHeaders UsageHeaderParser
	// ValidatesCredentials is true when FetchUsage contacts upstream and can
	// authenticate the credential.
	ValidatesCredentials bool
	// RetainLastGoodOnFailure nil means true.
	RetainLastGoodOnFailure *bool
	FailureBackoffMs        int64
}

// SupportsCredential reports whether this provider handles params.
func (provider *UsageProvider) SupportsCredential(params UsageFetchParams) bool {
	if provider == nil {
		return false
	}
	if provider.Supports == nil {
		return true
	}
	return provider.Supports(params)
}

// RetainsLastGoodOnFailure reports the provider's last-good retention policy.
func (provider *UsageProvider) RetainsLastGoodOnFailure() bool {
	if provider == nil || provider.RetainLastGoodOnFailure == nil {
		return true
	}
	return *provider.RetainLastGoodOnFailure
}

// UsageHTTPError is a provider response the fetcher wants surfaced: 401/403
// purges the cached report, other statuses are transient.
type UsageHTTPError struct {
	Status  int
	Message string
}

func (e *UsageHTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "usage endpoint returned " + strconv.Itoa(e.Status)
}

// NewUsageHTTPError builds a UsageHTTPError with a status and message.
func NewUsageHTTPError(message string, status int) *UsageHTTPError {
	return &UsageHTTPError{Status: status, Message: message}
}

// UsageUnavailableAccount is one account whose quota could not be read.
type UsageUnavailableAccount struct {
	Provider   string  `json:"provider"`
	Label      string  `json:"label"`
	AccountKey *string `json:"accountKey,omitempty"`
	// Ambient marks an environment/runtime-key probe rather than a stored account.
	Ambient bool   `json:"ambient,omitempty"`
	Reason  string `json:"reason"`
}

// UsageSnapshot is one quota pass over every configured provider account.
type UsageSnapshot struct {
	// Reports holds the successful reports, one per account that answered.
	Reports []UsageReport `json:"reports"`
	// UnavailableAccounts holds accounts that answered with nothing, instead of
	// being silently omitted.
	UnavailableAccounts []UsageUnavailableAccount `json:"unavailableAccounts"`
}

// UsageActiveAccount is the session-routing identity of the account a provider
// currently uses.
type UsageActiveAccount struct {
	AccountKey *string
	Email      string
	AccountID  string
	OrgID      string
	OrgName    string
}

// usageStatusFor maps a used fraction to the upstream thresholds.
func usageStatusFor(usedFraction *float64) UsageStatus {
	if usedFraction == nil {
		return UsageStatusUnknown
	}
	if *usedFraction >= 1 {
		return UsageStatusExhausted
	}
	if *usedFraction >= 0.9 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// UsageUsedFraction resolves a limit's used fraction (0..1; >1 means overage)
// from whichever amount fields the provider populated. Precedence mirrors
// upstream resolveUsedFraction: explicit fraction, used/limit, percent-unit
// used, then inverted remaining fraction.
func UsageUsedFraction(limit UsageLimit) (float64, bool) {
	amount := limit.Amount
	if amount.UsedFraction != nil {
		return *amount.UsedFraction, true
	}
	if amount.Used != nil && amount.Limit != nil && *amount.Limit > 0 {
		return *amount.Used / *amount.Limit, true
	}
	if amount.Unit == UsageUnitPercent && amount.Used != nil {
		return *amount.Used / 100, true
	}
	if amount.RemainingFraction != nil {
		return math.Max(0, 1-*amount.RemainingFraction), true
	}
	return 0, false
}

// IsUsageLimitExhausted reports whether a limit has spent its quota. An
// explicit provider status wins; otherwise the amount fields decide.
func IsUsageLimitExhausted(limit UsageLimit) bool {
	if limit.Status != "" && limit.Status != UsageStatusUnknown {
		return limit.Status == UsageStatusExhausted
	}
	amount := limit.Amount
	if amount.UsedFraction != nil && *amount.UsedFraction >= 1 {
		return true
	}
	if amount.RemainingFraction != nil && *amount.RemainingFraction <= 0 {
		return true
	}
	if amount.Used != nil && amount.Limit != nil && *amount.Used >= *amount.Limit {
		return true
	}
	if amount.Remaining != nil && *amount.Remaining <= 0 {
		return true
	}
	if amount.Unit == UsageUnitPercent && amount.Used != nil && *amount.Used >= 100 {
		return true
	}
	return false
}

// UsageLimitStatus is the display/ranking status of a limit: the provider's
// explicit status when it has one, else the fraction thresholds (warning at
// 90% used).
func UsageLimitStatus(limit UsageLimit) UsageStatus {
	if limit.Status != "" {
		return limit.Status
	}
	fraction, ok := UsageUsedFraction(limit)
	if !ok {
		return UsageStatusUnknown
	}
	return usageStatusFor(&fraction)
}

// SortUsageLimits orders limits for display: exhausted first, then by
// descending used fraction, then by label. The sort is stable.
func SortUsageLimits(limits []UsageLimit) {
	type ranked struct {
		limit     UsageLimit
		exhausted bool
		fraction  float64
	}
	ranks := make([]ranked, len(limits))
	for i, limit := range limits {
		fraction, ok := UsageUsedFraction(limit)
		if !ok {
			fraction = -1
		}
		ranks[i] = ranked{limit: limit, exhausted: IsUsageLimitExhausted(limit), fraction: fraction}
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].exhausted != ranks[j].exhausted {
			return ranks[i].exhausted
		}
		if ranks[i].fraction != ranks[j].fraction {
			return ranks[i].fraction > ranks[j].fraction
		}
		return ranks[i].limit.Label < ranks[j].limit.Label
	})
	for i := range ranks {
		limits[i] = ranks[i].limit
	}
}

// UsageReportMetadataString reads a string identity field from report metadata.
func UsageReportMetadataString(report *UsageReport, key string) (string, bool) {
	if report == nil || report.Metadata == nil {
		return "", false
	}
	value, ok := report.Metadata[key].(string)
	if !ok {
		return "", false
	}
	value = trimJSWhitespace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// UsageBuildAmount builds an amount from absolute counters. Without an
// authoritative usedFraction it derives one from used/limit, capped at 1, and
// derives the remaining fraction from it.
func UsageBuildAmount(used, limit, remaining *float64, usedFraction *float64, unit UsageUnit) UsageAmount {
	if usedFraction == nil && used != nil && limit != nil && *limit > 0 {
		value := math.Min(*used / *limit, 1)
		usedFraction = &value
	}
	var remainingFraction *float64
	if usedFraction != nil {
		value := math.Max(1-*usedFraction, 0)
		remainingFraction = &value
	}
	return UsageAmount{
		Used:              used,
		Limit:             limit,
		Remaining:         remaining,
		UsedFraction:      usedFraction,
		RemainingFraction: remainingFraction,
		Unit:              unit,
	}
}

// usageNumber mirrors toNumber: finite numbers, and numeric strings.
func usageNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return value, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return value, true
	case string:
		trimmed := trimJSWhitespace(typed)
		if trimmed == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil && !math.IsInf(value, 0) {
			return 0, false
		}
		if math.IsNaN(value) {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

// usageRecord reports whether value is a JSON object.
func usageRecord(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	record, ok := value.(map[string]any)
	return record, ok
}

// usageString reads a trimmed non-empty string field.
func usageString(record map[string]any, key string) (string, bool) {
	if record == nil {
		return "", false
	}
	value, ok := record[key].(string)
	if !ok {
		return "", false
	}
	value = trimJSWhitespace(value)
	return value, value != ""
}

// usageBool reads a JSON boolean.
func usageBool(record map[string]any, key string) (bool, bool) {
	if record == nil {
		return false, false
	}
	value, ok := record[key].(bool)
	return value, ok
}

// usageArray reads a JSON array field.
func usageArray(record map[string]any, key string) ([]any, bool) {
	if record == nil {
		return nil, false
	}
	value, ok := record[key].([]any)
	return value, ok
}

// usageObjectArray reads an array of objects.
func usageObjectArray(record map[string]any, key string) []map[string]any {
	items, _ := usageArray(record, key)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := usageRecord(item); ok {
			out = append(out, object)
		}
	}
	return out
}

// usageHasOwn reports whether a parsed object carries key.
func usageHasOwn(record map[string]any, key string) bool {
	if record == nil {
		return false
	}
	_, ok := record[key]
	return ok
}

// usageClampFraction clamps a numeric value into 0..1.
func usageClampFraction(value float64) float64 {
	return math.Min(math.Max(value, 0), 1)
}

// usageSlugify ports pi-utils slugify, for provider labels and ids.
func usageSlugify(input string) string {
	lowered := strings.ToLower(trimJSWhitespace(input))
	var builder strings.Builder
	previousDash := false
	for _, r := range lowered {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			previousDash = false
		default:
			if !previousDash && builder.Len() > 0 {
				builder.WriteByte('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

// usageSecretTag is the non-reversible FNV-derived tag the quota cache uses so
// two secrets never share an entry. It mirrors the upstream 32-bit imul math.
func usageSecretTag(secret string) string {
	first := uint32(0x811c9dc5)
	second := uint32(0x01000193)
	for _, unit := range utf16.Encode([]rune(secret)) {
		first = (first ^ uint32(unit)) * 0x01000193
		second = (second + uint32(unit)) * 0x85ebca6b
	}
	return leftPadHex(first) + leftPadHex(second)
}

func leftPadHex(value uint32) string {
	text := strconv.FormatUint(uint64(value), 16)
	for len(text) < 8 {
		text = "0" + text
	}
	return text
}

// UsageCacheIdentity builds the account identity component of a probe cache
// key. An explicit store scope wins; otherwise stable credential claims, and a
// secret-derived tag only when no stable identifier exists (so an OAuth
// refresh does not orphan the cached report).
func UsageCacheIdentity(credential UsageCredential) string {
	parts := []string{string(credential.Type)}
	if accountID := trimJSWhitespace(credential.AccountID); accountID != "" {
		parts = append(parts, "account:"+accountID)
	}
	if email := strings.ToLower(trimJSWhitespace(credential.Email)); email != "" {
		parts = append(parts, "email:"+email)
	}
	if orgID := trimJSWhitespace(credential.OrgID); orgID != "" {
		parts = append(parts, "org:"+orgID)
	}
	if projectID := trimJSWhitespace(credential.ProjectID); projectID != "" {
		parts = append(parts, "project:"+projectID)
	}
	if enterpriseURL := strings.ToLower(trimJSWhitespace(credential.EnterpriseURL)); enterpriseURL != "" {
		parts = append(parts, "enterprise:"+enterpriseURL)
	}
	if region := credential.Region; region != "" {
		parts = append(parts, "region:"+region)
	}
	if credential.InferenceRegion != "" {
		parts = append(parts, "inference:"+credential.InferenceRegion)
	}
	if active := credential.ActiveOrganizationID; active != "" {
		parts = append(parts, "selected-org:"+active)
	}
	hasStable := credential.AccountID != "" || credential.Email != "" || credential.OrgID != ""
	if !hasStable {
		secret := strings.TrimSpace(credential.APIKey)
		if secret == "" {
			secret = strings.TrimSpace(credential.RefreshToken)
		}
		if secret == "" {
			secret = strings.TrimSpace(credential.AccessToken)
		}
		if secret != "" {
			parts = append(parts, "secret:"+usageSecretTag(secret))
		} else {
			parts = append(parts, "anonymous")
		}
	}
	return strings.Join(parts, ":")
}

// UsageAccountSettings is the non-secret identity of a stored credential.
type UsageAccountSettings struct {
	AccountID string
	ProjectID string
	Email     string
	OrgID     string
	OrgName   string
	Region    string
	// InferenceRegion is "global", "eu" or "us" when the provider partitions
	// inference independently of residency.
	InferenceRegion      string
	ActiveOrganizationID string
	EnterpriseURL        string
	APIEndpoint          string
}

// stringClaim reads a string-valued provider claim from a credential.
func stringClaim(credential Credential, key string) string {
	raw, ok := credential.Extra[key]
	if !ok {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return trimJSWhitespace(value)
}

// AccountCredentialSettings reads a credential's non-secret identity claims.
// An API-key credential carries only what its stored projection holds.
func AccountCredentialSettings(credential Credential) *UsageAccountSettings {
	if credential.Type != CredentialOAuth && credential.Type != CredentialAPIKey {
		return nil
	}
	settings := &UsageAccountSettings{
		AccountID:            trimJSWhitespace(credential.AccountID),
		ProjectID:            trimJSWhitespace(credential.ProjectID),
		EnterpriseURL:        trimJSWhitespace(credential.EnterpriseDomain),
		Email:                stringClaim(credential, "email"),
		OrgID:                stringClaim(credential, "orgId"),
		OrgName:              stringClaim(credential, "orgName"),
		Region:               stringClaim(credential, "region"),
		InferenceRegion:      stringClaim(credential, "inferenceRegion"),
		ActiveOrganizationID: stringClaim(credential, "activeOrganizationId"),
		APIEndpoint:          stringClaim(credential, "apiEndpoint"),
	}
	switch settings.InferenceRegion {
	case "global", "eu", "us":
	default:
		settings.InferenceRegion = ""
	}
	return settings
}

// UsageCredentialFromStored narrows a stored credential to the bundle a usage
// fetcher authenticates with. Fetchers never receive storage details they do
// not use.
func UsageCredentialFromStored(credential *Credential) UsageCredential {
	if credential == nil {
		return UsageCredential{}
	}
	if credential.Type == CredentialAPIKey {
		return UsageCredential{Type: CredentialAPIKey, APIKey: credential.Key}
	}
	out := UsageCredential{
		Type:         CredentialOAuth,
		AccessToken:  credential.Access,
		RefreshToken: credential.Refresh,
	}
	if credential.HasExpires() {
		out.ExpiresAt = new(credential.Expires)
	}
	if settings := AccountCredentialSettings(*credential); settings != nil {
		out.AccountID = settings.AccountID
		out.ProjectID = settings.ProjectID
		out.Email = settings.Email
		out.OrgID = settings.OrgID
		out.OrgName = settings.OrgName
		out.EnterpriseURL = settings.EnterpriseURL
		out.Region = settings.Region
		out.InferenceRegion = settings.InferenceRegion
		out.ActiveOrganizationID = settings.ActiveOrganizationID
		out.APIEndpoint = settings.APIEndpoint
	}
	return out
}

// AttributeUsageReport attaches the stored credential's identity to a report so
// its labels and grouping match the account.
func AttributeUsageReport(report *UsageReport, credential UsageCredential) {
	if report == nil {
		return
	}
	metadata := make(map[string]any, len(report.Metadata)+4)
	for key, value := range report.Metadata {
		metadata[key] = value
	}
	if credential.AccountID != "" {
		if _, exists := metadata["accountId"]; !exists {
			metadata["accountId"] = credential.AccountID
		}
	}
	if credential.Email != "" {
		if _, exists := metadata["email"]; !exists {
			metadata["email"] = credential.Email
		}
	}
	if credential.ProjectID != "" {
		if _, exists := metadata["projectId"]; !exists {
			metadata["projectId"] = credential.ProjectID
		}
	}
	if credential.OrgID != "" {
		sameOrg := true
		if existing, ok := metadata["orgId"].(string); ok {
			sameOrg = existing == credential.OrgID
		} else {
			metadata["orgId"] = credential.OrgID
		}
		if sameOrg && credential.OrgName != "" {
			if _, exists := metadata["orgName"]; !exists {
				metadata["orgName"] = credential.OrgName
			}
		}
	}
	if len(metadata) > 0 {
		report.Metadata = metadata
	}
}
