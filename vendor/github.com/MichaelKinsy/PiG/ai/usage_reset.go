package ai

// Ports the redemption half of packages/ai/src/usage/openai-codex-reset.ts and
// packages/ai/src/usage/claude-reset.ts: listing an account's saved rate-limit
// resets and spending one on demand.
//
// Codex banks resets behind two dedicated routes:
//
//	GET  <backend>/wham/rate-limit-reset-credits          list
//	POST <backend>/wham/rate-limit-reset-credits/consume  spend one
//
// Claude banks Cedar grants (per-window) and a Juniper session reset, both spent
// through the organization's first-party route:
//
//	POST <api>/api/organizations/<org>/reset_rate_limits

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"time"
)

const (
	codexResetCreditsPath        = "wham/rate-limit-reset-credits"
	codexResetCreditsConsumePath = "wham/rate-limit-reset-credits/consume"
	// claudeResetRedeemTimeout bounds one reset mutation. A mutation is never
	// retried, so the budget is generous enough to survive a slow organization
	// lookup on the same call.
	claudeResetRedeemTimeout = 25 * time.Second
)

// claudeRedeemRequestIDPattern is the idempotency key shape Claude accepts.
var claudeRedeemRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// UsageResetConsumeResult is the outcome of spending one saved reset.
//
// Code and Ok mirror the provider's business verdict: Ok is true only when a
// window was actually reset. Cleared is the set of limit ids the provider
// confirmed as refreshed, which a caller uses to lift the matching blocks
// without lifting unrelated ones.
type UsageResetConsumeResult struct {
	Ok      bool
	Code    string
	Status  int
	Cleared []string
	Reason  string
	Raw     any
}

// usageResetCodeFromHTTP reports the diagnostic code of a refused mutation.
func usageResetCodeFromHTTP(status int) string {
	switch status {
	case 401, 403:
		return "auth_error"
	case 429:
		return "rate_limited"
	}
	return "http_" + usageFormatCount(int64(status))
}

// usageCodexAccountID resolves the ChatGPT account id of a credential, falling
// back to the bearer's auth claim.
func usageCodexAccountID(credential UsageCredential) string {
	if accountID := trimJSWhitespace(credential.AccountID); accountID != "" {
		return accountID
	}
	claims, ok := usageJWTClaims(credential.AccessToken)
	if !ok {
		return ""
	}
	auth, ok := usageRecord(claims[codexJWTAuthClaim])
	if !ok {
		return ""
	}
	accountID, _ := usageString(auth, "chatgpt_account_id")
	return accountID
}

// codexResetHeaders builds the account-API headers for both reset routes.
func codexResetHeaders(credential UsageCredential, jsonBody bool) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + credential.AccessToken,
		"User-Agent":    usageUserAgent(),
	}
	if accountID := usageCodexAccountID(credential); accountID != "" {
		headers["ChatGPT-Account-Id"] = accountID
	}
	if jsonBody {
		headers["Content-Type"] = "application/json"
	}
	return headers
}

// ListCodexResetCredits lists one account's saved resets from the dedicated
// route. ok is false on a transport, auth or shape failure, so a caller keeps
// the count a usage probe already reported instead of reading it as zero.
func ListCodexResetCredits(ctx context.Context, fctx UsageFetchContext, credential UsageCredential, baseURL string) (*UsageResetCredits, bool) {
	if credential.AccessToken == "" {
		return nil, false
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "GET",
		usageAbsoluteURL(normalizeCodexBaseURL(baseURL), codexResetCreditsPath),
		codexResetHeaders(credential, false), nil)
	if err != nil || !response.OK() {
		return nil, false
	}
	payload, ok := response.JSON()
	if !ok {
		return nil, false
	}
	record, ok := usageRecord(payload)
	if !ok {
		return nil, false
	}
	credits := make([]UsageResetCreditDetail, 0, 4)
	available := 0
	for _, item := range usageObjectArray(record, "credits") {
		id, ok := usageString(item, "id")
		if !ok {
			continue
		}
		detail := UsageResetCreditDetail{ID: id, Program: "codex_rate_limits"}
		if program, ok := usageString(item, "reset_type"); ok {
			detail.Program = program
		}
		detail.Status, _ = usageString(item, "status")
		if detail.Status == "" {
			detail.Status = "available"
		}
		detail.GrantedAt, _, _ = usageOptionalIsoTimestamp(item["granted_at"])
		detail.ExpiresAt, _, _ = usageOptionalIsoTimestamp(item["expires_at"])
		detail.Title, _ = usageString(item, "title")
		if detail.Status == "available" {
			// A Codex credit has no per-grant count: one object is one spendable
			// reset, which the shared summary needs to count its expiry.
			detail.RemainingCount = 1
		}
		credits = append(credits, detail)
	}
	if reported, ok := usageNumber(record["available_count"]); ok {
		available = int(math.Max(0, math.Trunc(reported)))
	} else {
		for _, credit := range credits {
			if credit.Status == "available" {
				available++
			}
		}
	}
	return &UsageResetCredits{AvailableCount: available, Credits: credits}, true
}

// PickSoonestExpiringResetCredit picks the available credit that expires
// soonest, because credits are perishable. Undated credits rank after dated
// ones; when nothing is available the first credit is returned so the consume
// surfaces the provider's own verdict.
func PickSoonestExpiringResetCredit(credits []UsageResetCreditDetail) *UsageResetCreditDetail {
	var best *UsageResetCreditDetail
	bestExpiry := int64(math.MaxInt64)
	var undated *UsageResetCreditDetail
	for index := range credits {
		credit := &credits[index]
		status := credit.Status
		if status == "" {
			status = "available"
		}
		if status != "available" {
			continue
		}
		expiry, ok := usageIsoTimestamp(credit.ExpiresAt)
		if !ok {
			if undated == nil {
				undated = credit
			}
			continue
		}
		if expiry < bestExpiry {
			best, bestExpiry = credit, expiry
		}
	}
	if best != nil {
		return best
	}
	if undated != nil {
		return undated
	}
	if len(credits) == 0 {
		return nil
	}
	return &credits[0]
}

// ConsumeCodexResetCredit spends one saved Codex reset. error reports a
// transport failure; a business refusal comes back in the result's code.
func ConsumeCodexResetCredit(ctx context.Context, fctx UsageFetchContext, credential UsageCredential, baseURL, creditID string) (*UsageResetConsumeResult, error) {
	requestID, err := UUIDv7(nil)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"credit_id": creditID, "redeem_request_id": requestID}
	if accountID := usageCodexAccountID(credential); accountID != "" {
		body["account_id"] = accountID
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	response, err := usageDo(ctx, fctx, usageFollowRedirects, "POST",
		usageAbsoluteURL(normalizeCodexBaseURL(baseURL), codexResetCreditsConsumePath),
		codexResetHeaders(credential, true), encoded)
	if err != nil {
		return nil, err
	}
	payload, _ := response.JSON()
	code := ""
	if record, ok := usageRecord(payload); ok {
		code, _ = usageString(record, "code")
	}
	if !response.OK() && code == "reset" {
		code = usageResetCodeFromHTTP(response.Status)
	}
	if code == "" {
		if response.OK() {
			// A 2xx without a verdict is the shape the endpoint used before the
			// code field existed: no reason to claim a spend the caller cannot
			// confirm.
			code = "malformed_response"
		} else {
			code = usageResetCodeFromHTTP(response.Status)
		}
	}
	return &UsageResetConsumeResult{
		Ok:     response.OK() && code == "reset",
		Code:   code,
		Status: response.Status,
		Raw:    payload,
	}, nil
}

// ListClaudeResetCredits lists one Claude account's saved resets. A nil result
// means discovery failed; a zero-count object is an authoritative answer.
func ListClaudeResetCredits(ctx context.Context, fctx UsageFetchContext, credential UsageCredential, baseURL string) *UsageResetCredits {
	return listClaudeResetCredits(ctx, fctx, credential.AccessToken, credential, baseURL)
}

// claudeResolveOrganizationID resolves the organization a redemption posts to,
// from the credential when it names a valid one, else from the profile route.
func claudeResolveOrganizationID(ctx context.Context, fctx UsageFetchContext, accessToken, configuredOrgID, baseURL string) string {
	if orgID := trimJSWhitespace(configuredOrgID); claudeOrganizationID.MatchString(orgID) {
		return orgID
	}
	probeCtx, cancel := usageDeadline(ctx, 5*time.Second)
	defer cancel()
	response, err := usageDo(probeCtx, fctx, usageFollowRedirects, "GET",
		claudeOAuthBaseURL(baseURL)+"/profile", buildClaudeOAuthHeaders(accessToken, claudeOAuthBeta, true), nil)
	if err != nil || !response.OK() {
		return ""
	}
	payload, ok := response.JSON()
	record, isRecord := usageRecord(payload)
	if !ok || !isRecord {
		return ""
	}
	candidate := ""
	if organization, ok := usageRecord(record["organization"]); ok {
		candidate, _ = usageString(organization, "uuid")
	}
	if candidate == "" {
		candidate, _ = usageString(record, "organization_uuid")
	}
	if !claudeOrganizationID.MatchString(candidate) {
		return ""
	}
	return candidate
}

// claudeNormalizeConsumeCode maps the provider's verdict wording onto the
// shared reset vocabulary.
func claudeNormalizeConsumeCode(result string) string {
	switch result {
	case "already_used":
		return "already_redeemed"
	case "not_limited":
		return "nothing_to_reset"
	}
	return result
}

// ConsumeClaudeResetCredit spends exactly the server-selected Claude reset
// described by credit. A mutation is never retried: a transport failure comes
// back as a result, so the caller can report the spend as unconfirmed rather
// than spending twice.
func ConsumeClaudeResetCredit(ctx context.Context, fctx UsageFetchContext, credential UsageCredential, baseURL string, credit UsageResetCreditDetail, redeemRequestID string) *UsageResetConsumeResult {
	program := credit.Program
	if program != claudeCedarProgram && program != claudeJuniperProgram {
		return &UsageResetConsumeResult{Code: "unknown_program"}
	}
	expiryMs := int64(0)
	if credit.ExpiresAt != "" {
		parsed, ok := usageIsoTimestamp(credit.ExpiresAt)
		if !ok {
			return &UsageResetConsumeResult{Code: "invalid_credit"}
		}
		expiryMs = parsed
	}
	if credit.Usable == nil || !*credit.Usable || credit.RemainingCount <= 0 ||
		(expiryMs != 0 && expiryMs <= timeNowMillis()) {
		return &UsageResetConsumeResult{Code: "unavailable", Reason: "credit_not_usable"}
	}
	if program == claudeCedarProgram && !claudeCedarGrantID.MatchString(credit.ID) {
		return &UsageResetConsumeResult{Code: "invalid_credit"}
	}
	if program == claudeJuniperProgram && credit.ID != claudeJuniperProgram {
		return &UsageResetConsumeResult{Code: "invalid_credit"}
	}
	if program == claudeCedarProgram && redeemRequestID == "" {
		generated, err := UUIDv7(nil)
		if err != nil {
			return &UsageResetConsumeResult{Code: "invalid_request", Raw: err}
		}
		redeemRequestID = generated
	}
	if redeemRequestID != "" && !claudeRedeemRequestIDPattern.MatchString(redeemRequestID) {
		return &UsageResetConsumeResult{Code: "invalid_request"}
	}
	orgID := claudeResolveOrganizationID(ctx, fctx, credential.AccessToken, credential.OrgID, baseURL)
	if orgID == "" {
		return &UsageResetConsumeResult{Code: "organization_unavailable"}
	}
	body := map[string]any{"program": program}
	if program == claudeCedarProgram {
		body["grant_id"] = credit.ID
		if redeemRequestID != "" {
			body["request_id"] = redeemRequestID
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return &UsageResetConsumeResult{Code: "network_error", Raw: err}
	}
	redeemCtx, cancel := usageDeadline(ctx, claudeResetRedeemTimeout)
	defer cancel()
	response, err := usageDo(redeemCtx, fctx, usageFollowRedirects, "POST",
		claudeAPIURL(baseURL, "/api/organizations/"+url.PathEscape(orgID)+"/reset_rate_limits"),
		buildClaudeOAuthHeaders(credential.AccessToken, claudeOAuthBeta, true), encoded)
	if err != nil {
		return &UsageResetConsumeResult{Code: "network_error", Raw: err}
	}
	payload, _ := response.JSON()
	if !response.OK() {
		return &UsageResetConsumeResult{Code: usageResetCodeFromHTTP(response.Status), Status: response.Status, Raw: payload}
	}
	record, ok := usageRecord(payload)
	if !ok {
		return &UsageResetConsumeResult{Code: "malformed_response", Status: response.Status, Raw: payload}
	}
	result, ok := usageString(record, "result")
	if !ok || trimJSWhitespace(result) == "" {
		return &UsageResetConsumeResult{Code: "malformed_response", Status: response.Status, Raw: payload}
	}
	reason, _, reasonOK := usageOptionalString(record["reason"])
	if !reasonOK {
		return &UsageResetConsumeResult{Code: "malformed_response", Status: response.Status, Raw: payload}
	}
	cleared := []string(nil)
	if usageHasOwn(record, "cleared") {
		normalized, ok := normalizeClaudeResetWindowList(record["cleared"])
		if !ok {
			return &UsageResetConsumeResult{Code: "malformed_response", Status: response.Status, Raw: payload}
		}
		cleared = normalized
	} else if trimJSWhitespace(result) == "reset" {
		if program == claudeJuniperProgram {
			cleared = []string{"anthropic:5h"}
		} else {
			cleared = credit.Clears
		}
	}
	code := claudeNormalizeConsumeCode(trimJSWhitespace(result))
	return &UsageResetConsumeResult{
		Ok:      code == "reset",
		Code:    code,
		Status:  response.Status,
		Cleared: cleared,
		Reason:  reason,
		Raw:     payload,
	}
}
