package ai

// Ports packages/ai/src/utils/account-policy.ts: the model entitlements and failure
// scopes used when routing one request across the stored accounts of a provider.

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/lazyregexp"
)

// jsSpaceBody is jsSpaceClass without its brackets, for patterns that add their own
// characters to the class (JavaScript's \s, which Go's \s does not spell).
var jsSpaceBody = jsSpaceClass[1 : len(jsSpaceClass)-1]

// Codex plan tiers, matched against the normalized chatgpt_plan_type claim exactly as
// account-policy.ts does: the tier must sit between the separators, so "pro" does not match
// "professional" and "go" does not match "gpt".
var (
	codexProPlan  = lazyregexp.New(`(?:^|_)pro(?:_|$)`)
	codexPaidPlan = lazyregexp.New(`(?:^|_)(?:plus|business|team|enterprise|edu|education|teacher|teachers|health|gov|government)(?:_|$)`)
	codexFreePlan = lazyregexp.New(`(?:^|_)(?:free|go)(?:_|$)`)
)

// codexPlanSeparators folds the space and hyphen runs of a plan tier into one underscore, as
// account-policy.ts's `replace(/[\s-]+/g, "_")` does.
var codexPlanSeparators = lazyregexp.New(`[` + jsSpaceBody + `-]+`)

// codexPaidModels are the catalog models a ChatGPT paid plan is required for. Node-compatible
// projection of pi-catalog rules/runtime/behavior.kdl (account-policy.ts CODEX_PAID_MODELS).
var codexPaidModels = map[string]bool{
	"gpt-5.6":          true,
	"gpt-5.6-sol":      true,
	"gpt-5.6-sol-pro":  true,
	"gpt-5.6-luna":     true,
	"gpt-5.6-luna-pro": true,
	"gpt-6.1-sol":      true,
}

// AccountPolicyModelID is the canonical bare model identity used by OMP's model-policy
// blocks: the last path segment, JavaScript-trimmed and lowercased.
func AccountPolicyModelID(modelID string) string {
	index := strings.LastIndex(modelID, "/")
	return jsstring.ToLower(jsstring.Trim(modelID[index+1:]))
}

// AccountMeetsModelPlan reports whether a known Codex plan satisfies the catalog requirement
// for modelID. A nil result means the plan cannot be read (an unparseable token or an unknown
// tier): the account stays probeable, ranked after every known-eligible account.
//
// Only openai-codex OAuth credentials carry a plan; every other provider and every API key is
// eligible.
func AccountMeetsModelPlan(provider, modelID string, credential Credential) *bool {
	if provider != "openai-codex" || credential.Type != CredentialOAuth {
		return new(true)
	}
	id := AccountPolicyModelID(modelID)
	requirement := "none"
	switch {
	case strings.Contains(id, "-spark"):
		requirement = "pro"
	case codexPaidModels[id] || strings.HasPrefix(id, "gpt-6.1-terra"):
		requirement = "paid"
	}
	if requirement == "none" {
		return new(true)
	}
	plan, ok := codexPlanClaim(credential.Access)
	if !ok {
		return nil
	}
	switch {
	case plan == "prolite" || plan == "pro_lite":
		return new(requirement == "paid")
	case codexProPlan.MatchString(plan):
		return new(true)
	case codexPaidPlan.MatchString(plan):
		return new(requirement == "paid")
	case codexFreePlan.MatchString(plan):
		return new(false)
	}
	return nil
}

// codexPlanClaim reads payload["https://api.openai.com/auth"].chatgpt_plan_type from the
// stored OAuth access token. Stored JWT claims are an external boundary, not an entitlement
// guarantee from the backend, so every failure reports "unknown" rather than "eligible".
func codexPlanClaim(access string) (string, bool) {
	parts := strings.Split(access, ".")
	if len(parts) < 2 {
		return "", false
	}
	decoded, err := decodeJWTPayload(parts[1])
	if err != nil {
		return "", false
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", false
	}
	authRaw, ok := payload["https://api.openai.com/auth"]
	if !ok {
		return "", false
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(authRaw, &claims); err != nil {
		return "", false
	}
	planRaw, ok := claims["chatgpt_plan_type"]
	if !ok {
		return "", false
	}
	var planValue any
	if err := json.Unmarshal(planRaw, &planValue); err != nil {
		return "", false
	}
	plan, ok := planValue.(string)
	if !ok {
		return "", false
	}
	normalized := codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")
	return strings.TrimPrefix(normalized, "chatgpt_"), true
}

// decodeJWTPayload decodes a base64url JWT segment the way atob does after the caller's
// -/+ and _// mapping: ASCII whitespace is ignored, and missing padding is accepted.
func decodeJWTPayload(segment string) ([]byte, error) {
	text := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, segment)
	text = strings.NewReplacer("-", "+", "_", "/").Replace(text)
	if index := strings.IndexByte(text, '='); index >= 0 {
		text = text[:index]
	}
	return base64.RawStdEncoding.DecodeString(text)
}

// AccountQuotaScope is OMP's quota scope for one model, keeping Codex chat and Spark meters
// separate. Providers without a per-model meter use the unscoped ("") block.
func AccountQuotaScope(provider, modelID string) string {
	if provider != "openai-codex" {
		return ""
	}
	if strings.Contains(AccountPolicyModelID(modelID), "-spark") {
		return "spark"
	}
	return "chat"
}

// AccountRequestBlockScopes lists the persisted OMP block scopes a request must honor.
func AccountRequestBlockScopes(provider, modelID string) []string {
	id := AccountPolicyModelID(modelID)
	scopes := []string{"auth", "account-policy", AccountQuotaScope(provider, modelID)}
	if provider == "openai-codex" {
		scopes = append(scopes, "shared")
	}
	if provider == "openai-codex" || provider == "cursor" {
		scopes = append(scopes, "model-policy:"+id)
	}
	return scopes
}

// MinAccountBlockMs is how long every account-scoped rejection blocks its account at least.
const MinAccountBlockMs = int64(60_000)

var (
	// accountPolicyDeniedModel matches Codex's "The '<model>' model is not supported when
	// using Codex with a ChatGPT account." rejection.
	accountPolicyDeniedModel = lazyregexp.NewJSIgnoreCase(`\bThe ['"]([^'"\r\n]+)['"] model is not supported when using Codex with a ChatGPT account\.`)
	// accountPolicyCursorDenial and accountPolicyCursorDetail match Cursor's changeable
	// rate-limit wording for a model the account's plan cannot use.
	accountPolicyCursorDenial = lazyregexp.NewJSIgnoreCase(`\bERROR_RATE_LIMITED_CHANGEABLE\b`)
	accountPolicyCursorDetail = lazyregexp.NewJSIgnoreCase(`Named models unavailable|Model unavailable on|Free plans can only use`)
	// accountPolicyDeniedAccount matches provider wording for an account the vendor disabled.
	accountPolicyDeniedAccount = lazyregexp.NewJSIgnoreCase(`cyber_policy|account.*(?:deactivated|suspended)|organization.*(?:restricted|disabled)`)
	// accountPolicyConcurrency matches a transient 429: a sibling account would not help.
	accountPolicyConcurrency = lazyregexp.NewJSIgnoreCase(`too many requests|per.?minute|tokens per min|requests per min|concurren`)
	// accountPolicyAuthFailure matches rejections another account can repair.
	accountPolicyAuthFailure = lazyregexp.NewJSIgnoreCase(`invalidated oauth token|invalid_grant|OAuth refresh (?:failed|lease)|token (?:has been )?(?:expired|revoked)|authentication (?:failed|required)|invalid (?:api key|access token)`)
	// accountPolicyRetryMinutes and accountPolicyRetrySeconds read the provider's own wait out
	// of the message, which account-policy.ts spells case-sensitively.
	accountPolicyRetryMinutes = lazyregexp.New(`try again in[` + jsSpaceBody + `]*~?[` + jsSpaceBody + `]*(\d+(?:\.\d+)?)[` + jsSpaceBody + `]*min`)
	accountPolicyRetrySeconds = lazyregexp.New(`retry (?:after|in)[` + jsSpaceBody + `]*(\d+(?:\.\d+)?)[` + jsSpaceBody + `]*s`)
)

// accountQuotaMessagePattern classifies error text that means the account is exhausted rather
// than the request being throttled or the provider failing. It is the full source list
// (retry.ts NON_RETRYABLE_PROVIDER_LIMIT_ERROR_PATTERN).
var accountQuotaMessagePattern = buildProviderErrorPattern([]string{
	"GoUsageLimitError",
	"FreeUsageLimitError",
	"Monthly usage limit reached",
	"available balance",
	"insufficient_quota",
	"out of budget",
	`quota (?:exceeded|exhausted)`,
	"billing",
	"subscription_sharing_usage_limit_exceeded",
	"usage_limit_reached",
	"usage_not_included",
	"You have hit your ChatGPT usage limit",
	"out of credits",
	"subscription_sharing_workplace_individual_usage_limit_exceeded",
	`"code"` + jsSpaceClass + `*:` + jsSpaceClass + `*"?1308([^0-9]|$)`,
})

// IsAccountQuotaErrorMessage reports whether provider error text describes an exhausted account
// rather than transient throttling. These errors do not clear by waiting, so only another
// stored account can repair them.
func IsAccountQuotaErrorMessage(message string) bool {
	return accountQuotaMessagePattern.MatchString(message)
}

// AccountFailureScope is the block an account rejection earns.
type AccountFailureScope struct {
	// Scope is the persisted OMP block scope: "auth", "account-policy", a quota scope ("chat",
	// "spark", or "" for every scope), or "model-policy:<model>".
	Scope string
	// Until is the block deadline in epoch milliseconds.
	Until int64
}

// AccountFailureBlock classifies a failed request as a rejection of the account, or of the
// model under this account, and reports the block it earns. A nil result means a sibling
// credential cannot repair the failure (a transient transport failure, a server error, or an
// invalid request), so the request must not rotate.
//
// status is the response status, or 0 when no response arrived. retryAfterMs is the parsed
// Retry-After delay, or nil.
func AccountFailureBlock(provider, modelID, message string, status int, retryAfterMs *int64) *AccountFailureScope {
	now := nowMillis()
	id := AccountPolicyModelID(modelID)
	if match := accountPolicyDeniedModel.FindStringSubmatch(message); match != nil {
		if provider != "openai-codex" || AccountPolicyModelID(match[1]) != id {
			return nil
		}
		return &AccountFailureScope{Scope: "model-policy:" + id, Until: now + MinAccountBlockMs}
	}
	if provider == "cursor" && accountPolicyCursorDenial.MatchString(message) && accountPolicyCursorDetail.MatchString(message) {
		return &AccountFailureScope{Scope: "model-policy:" + id, Until: now + MinAccountBlockMs}
	}
	if status == 403 || accountPolicyDeniedAccount.MatchString(message) {
		return &AccountFailureScope{Scope: "account-policy", Until: now + MinAccountBlockMs}
	}
	if IsAccountQuotaErrorMessage(message) || (status == 429 && !accountPolicyConcurrency.MatchString(message)) {
		delay := MinAccountBlockMs
		if retryAfterMs != nil && *retryAfterMs > delay {
			delay = *retryAfterMs
		}
		if minutes := accountPolicyRetryMinutes.FindStringSubmatch(message); minutes != nil {
			if value, err := strconv.ParseFloat(minutes[1], 64); err == nil {
				if scaled := int64(value * float64(MinAccountBlockMs)); scaled > delay {
					delay = scaled
				}
			}
		}
		if seconds := accountPolicyRetrySeconds.FindStringSubmatch(message); seconds != nil {
			if value, err := strconv.ParseFloat(seconds[1], 64); err == nil {
				if scaled := int64(value * 1000); scaled > delay {
					delay = scaled
				}
			}
		}
		return &AccountFailureScope{Scope: AccountQuotaScope(provider, modelID), Until: now + delay}
	}
	if status == 401 || accountPolicyAuthFailure.MatchString(message) {
		return &AccountFailureScope{Scope: "auth", Until: now + MinAccountBlockMs}
	}
	return nil
}
