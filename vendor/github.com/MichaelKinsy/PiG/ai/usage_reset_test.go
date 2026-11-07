package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// resetTestTransport answers every request with a fixed status and body,
// recording the requests it saw so the wire contract can be asserted.
type resetTestTransport struct {
	// mu guards the recording slices: one Codex probe issues its usage and
	// entitlement requests concurrently, so RoundTrip runs on several goroutines.
	mu       sync.Mutex
	status   int
	body     string
	requests []*http.Request
	bodies   []string
}

func (transport *resetTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	payload := ""
	if request.Body != nil {
		encoded, _ := io.ReadAll(request.Body)
		payload = string(encoded)
	}
	transport.mu.Lock()
	transport.requests = append(transport.requests, request)
	transport.bodies = append(transport.bodies, payload)
	transport.mu.Unlock()
	return &http.Response{
		StatusCode: transport.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(transport.body)),
		Request:    request,
	}, nil
}

// resetTestFetchContext wires a transport into a usage fetch context.
func resetTestFetchContext(transport http.RoundTripper) UsageFetchContext {
	return UsageFetchContext{HTTPClient: &http.Client{Transport: transport}}
}

// resetTestCredential is an OAuth credential for the reset routes.
func resetTestCredential() UsageCredential {
	return UsageCredential{Type: CredentialOAuth, AccessToken: "tok", AccountID: "acct-1"}
}

// TestResetClearsOnlyCoveredLocalBlocks ensures a persisted clear is not
// defeated by the same router's in-memory backoff, while policy blocks survive.
func TestResetClearsOnlyCoveredLocalBlocks(t *testing.T) {
	router := NewAccountRouter(nil)
	key := new("account")
	until := timeNowMillis() + 60_000
	router.rememberBlock("openai-codex", key, "chat", until)
	router.rememberBlock("openai-codex", key, "auth", until)
	router.ClearResetBlocks("openai-codex", key, []string{"chat", "shared"})
	if got := router.localBlockedUntil("openai-codex", key, []string{"chat"}, timeNowMillis()); got != 0 {
		t.Fatalf("quota block survived reset: %d", got)
	}
	if got := router.localBlockedUntil("openai-codex", key, []string{"auth"}, timeNowMillis()); got != until {
		t.Fatalf("independent auth block was lost: %d", got)
	}
}

func TestListCodexResetCreditsReadsTheDedicatedRoute(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{
		"credits": [
			{"id": "RateLimitResetCredit_abc", "reset_type": "codex_rate_limits", "status": "available",
			 "granted_at": "2026-06-12T02:11:50Z", "expires_at": "2026-07-12T02:11:50Z",
			 "title": "One free rate limit reset"},
			{"id": "RateLimitResetCredit_def", "status": "redeemed"}
		],
		"available_count": 1
	}`}
	list, ok := ListCodexResetCredits(context.Background(), resetTestFetchContext(transport), resetTestCredential(), "")
	if !ok {
		t.Fatal("listing reported a failure")
	}
	if list.AvailableCount != 1 || len(list.Credits) != 2 {
		t.Fatalf("inventory = %+v", list)
	}
	first := list.Credits[0]
	if first.ID != "RateLimitResetCredit_abc" || first.Status != "available" || first.RemainingCount != 1 {
		t.Fatalf("first credit = %+v", first)
	}
	if first.Program != "codex_rate_limits" || first.Title != "One free rate limit reset" {
		t.Fatalf("first credit metadata = %+v", first)
	}
	if first.ExpiresAt != "2026-07-12T02:11:50.000Z" {
		t.Fatalf("expiry = %q", first.ExpiresAt)
	}
	request := transport.requests[0]
	if got := request.URL.String(); got != "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits" {
		t.Fatalf("url = %q", got)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("authorization = %q", got)
	}
	if got := request.Header.Get("ChatGPT-Account-Id"); got != "acct-1" {
		t.Fatalf("account header = %q", got)
	}
	// A redeemed credit keeps a zero count so the shared summary does not treat
	// it as a banked reset.
	if list.Credits[1].RemainingCount != 0 {
		t.Fatalf("redeemed credit count = %d", list.Credits[1].RemainingCount)
	}
}

func TestListCodexResetCreditsCountsWhenReportAbsent(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{"credits":[{"id":"a","status":"available"},{"id":"b","status":"redeemed"}]}`}
	list, ok := ListCodexResetCredits(context.Background(), resetTestFetchContext(transport), resetTestCredential(), "")
	if !ok || list.AvailableCount != 1 {
		t.Fatalf("inventory = %+v ok=%v", list, ok)
	}
}

func TestListCodexResetCreditsReportsFailure(t *testing.T) {
	for _, status := range []int{401, 404, 500} {
		transport := &resetTestTransport{status: status, body: `{"detail":"nope"}`}
		if list, ok := ListCodexResetCredits(context.Background(), resetTestFetchContext(transport), resetTestCredential(), ""); ok {
			t.Fatalf("status %d produced %+v", status, list)
		}
	}
}

func TestPickSoonestExpiringResetCreditPrefersDated(t *testing.T) {
	available := func(id, expiry string) UsageResetCreditDetail {
		return UsageResetCreditDetail{ID: id, Status: "available", ExpiresAt: expiry}
	}
	credits := []UsageResetCreditDetail{
		available("later", "2026-08-01T00:00:00Z"),
		{ID: "spent", Status: "redeemed", ExpiresAt: "2026-01-01T00:00:00Z"},
		available("undated", ""),
		available("sooner", "2026-07-01T00:00:00Z"),
	}
	picked := PickSoonestExpiringResetCredit(credits)
	if picked == nil || picked.ID != "sooner" {
		t.Fatalf("picked = %+v", picked)
	}
	// With no dated credit the undated one is still spendable.
	if picked := PickSoonestExpiringResetCredit([]UsageResetCreditDetail{available("undated", "")}); picked == nil || picked.ID != "undated" {
		t.Fatalf("undated pick = %+v", picked)
	}
	// Nothing available falls back to the first credit so the provider's own
	// verdict decides.
	if picked := PickSoonestExpiringResetCredit([]UsageResetCreditDetail{{ID: "spent", Status: "redeemed"}}); picked == nil || picked.ID != "spent" {
		t.Fatalf("fallback pick = %+v", picked)
	}
	if picked := PickSoonestExpiringResetCredit(nil); picked != nil {
		t.Fatalf("empty pick = %+v", picked)
	}
}

func TestConsumeCodexResetCreditPostsAnIdempotencyKey(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{"code":"reset"}`}
	result, err := ConsumeCodexResetCredit(context.Background(), resetTestFetchContext(transport), resetTestCredential(), "", "RateLimitResetCredit_abc")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if !result.Ok || result.Code != "reset" {
		t.Fatalf("result = %+v", result)
	}
	request := transport.requests[0]
	if request.Method != "POST" {
		t.Fatalf("method = %q", request.Method)
	}
	if got := request.URL.String(); got != "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume" {
		t.Fatalf("url = %q", got)
	}
	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(transport.bodies[0]), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["credit_id"] != "RateLimitResetCredit_abc" {
		t.Fatalf("credit_id = %v", body["credit_id"])
	}
	if body["account_id"] != "acct-1" {
		t.Fatalf("account_id = %v", body["account_id"])
	}
	requestID, _ := body["redeem_request_id"].(string)
	if requestID == "" {
		t.Fatal("no idempotency key")
	}
}

func TestConsumeCodexResetCreditRequiresSuccessfulHTTPStatus(t *testing.T) {
	transport := &resetTestTransport{status: 500, body: `{"code":"reset"}`}
	result, err := ConsumeCodexResetCredit(t.Context(), resetTestFetchContext(transport), resetTestCredential(), "", "c1")
	if err != nil || result.Ok {
		t.Fatalf("non-2xx must not confirm a spend: %+v, %v", result, err)
	}
}

func TestConsumeClaudeResetCreditGeneratesRequestID(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{"result":"reset"}`}
	credit := UsageResetCreditDetail{ID: "cedar-grant", Program: claudeCedarProgram, RemainingCount: 1, Usable: new(true)}
	result := ConsumeClaudeResetCredit(t.Context(), resetTestFetchContext(transport), UsageCredential{AccessToken: "tok", OrgID: "org-1"}, "", credit, "")
	if !result.Ok {
		t.Fatalf("consume: %+v", result)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}
	if id, _ := body["request_id"].(string); id == "" {
		t.Fatal("Cedar mutation omitted its idempotency key")
	}
}

func TestConsumeCodexResetCreditReportsBusinessCodes(t *testing.T) {
	for _, code := range []string{"already_redeemed", "no_credit", "nothing_to_reset"} {
		transport := &resetTestTransport{status: 200, body: `{"code":"` + code + `"}`}
		result, err := ConsumeCodexResetCredit(context.Background(), resetTestFetchContext(transport), resetTestCredential(), "", "c1")
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if result.Ok || result.Code != code {
			t.Fatalf("result = %+v", result)
		}
	}
	// A 2xx without a verdict is not a confirmed spend.
	transport := &resetTestTransport{status: 200, body: `{}`}
	result, err := ConsumeCodexResetCredit(context.Background(), resetTestFetchContext(transport), resetTestCredential(), "", "c1")
	if err != nil || result.Ok || result.Code != "malformed_response" {
		t.Fatalf("result = %+v err=%v", result, err)
	}
}

func TestConsumeClaudeResetCreditPostsTheSelectedGrant(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{"result":"reset","cleared":["five_hour"]}`}
	usable := true
	credit := UsageResetCreditDetail{
		ID:             "cedar-grant",
		Program:        claudeCedarProgram,
		RemainingCount: 2,
		Usable:         &usable,
		Clears:         []string{"anthropic:5h"},
	}
	credential := UsageCredential{Type: CredentialOAuth, AccessToken: "tok", OrgID: "org-1"}
	result := ConsumeClaudeResetCredit(context.Background(), resetTestFetchContext(transport), credential, "", credit, "req-1")
	if !result.Ok || result.Code != "reset" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Cleared) != 1 || result.Cleared[0] != "anthropic:5h" {
		t.Fatalf("cleared = %v", result.Cleared)
	}
	request := transport.requests[0]
	if got := request.URL.String(); got != "https://api.anthropic.com/api/organizations/org-1/reset_rate_limits" {
		t.Fatalf("url = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(transport.bodies[0]), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["program"] != claudeCedarProgram || body["grant_id"] != "cedar-grant" || body["request_id"] != "req-1" {
		t.Fatalf("body = %v", body)
	}
}

func TestConsumeClaudeResetCreditRefusesAnUnusableCredit(t *testing.T) {
	unusable := false
	cases := []struct {
		name   string
		credit UsageResetCreditDetail
		code   string
	}{
		{"unknown program", UsageResetCreditDetail{ID: "x", Program: "other", RemainingCount: 1}, "unknown_program"},
		{"not usable", UsageResetCreditDetail{ID: "x", Program: claudeCedarProgram, RemainingCount: 1, Usable: &unusable}, "unavailable"},
		{"no count", UsageResetCreditDetail{ID: "x", Program: claudeCedarProgram}, "unavailable"},
		{"bad grant id", UsageResetCreditDetail{ID: "NOT VALID", Program: claudeCedarProgram, RemainingCount: 1, Usable: new(true)}, "invalid_credit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &resetTestTransport{status: 200, body: `{"result":"reset"}`}
			result := ConsumeClaudeResetCredit(context.Background(), resetTestFetchContext(transport), UsageCredential{Type: CredentialOAuth, AccessToken: "tok", OrgID: "org-1"}, "", tc.credit, "")
			if result.Code != tc.code {
				t.Fatalf("code = %q, want %q", result.Code, tc.code)
			}
			if len(transport.requests) != 0 {
				t.Fatal("refused credit still reached the provider")
			}
		})
	}
}

func TestConsumeClaudeResetCreditMapsProviderWording(t *testing.T) {
	for providerResult, want := range map[string]string{
		"already_used": "already_redeemed",
		"not_limited":  "nothing_to_reset",
	} {
		transport := &resetTestTransport{status: 200, body: `{"result":"` + providerResult + `"}`}
		usable := true
		credit := UsageResetCreditDetail{ID: "cedar-grant", Program: claudeCedarProgram, RemainingCount: 1, Usable: &usable}
		result := ConsumeClaudeResetCredit(context.Background(), resetTestFetchContext(transport), UsageCredential{Type: CredentialOAuth, AccessToken: "tok", OrgID: "org-1"}, "", credit, "")
		if result.Code != want || result.Ok {
			t.Fatalf("%s produced %+v, want %q", providerResult, result, want)
		}
	}
}

func TestConsumeClaudeResetCreditResolvesOrganizationFromProfile(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{"result":"reset"}`}
	usable := true
	credit := UsageResetCreditDetail{ID: "juniper_tide", Program: claudeJuniperProgram, RemainingCount: 1, Usable: &usable}
	// No configured org: the mutation cannot run, and the reason reaches the
	// caller instead of a guessed organization id.
	result := ConsumeClaudeResetCredit(context.Background(), resetTestFetchContext(transport), UsageCredential{Type: CredentialOAuth, AccessToken: "tok"}, "", credit, "")
	if result.Code != "organization_unavailable" {
		t.Fatalf("code = %q", result.Code)
	}
	for _, request := range transport.requests {
		if request.Method != http.MethodGet {
			t.Fatalf("mutation ran without an organization: %s %s", request.Method, request.URL)
		}
	}
}

func TestConsumeClaudeResetCreditReportsTransportFailure(t *testing.T) {
	transport := &resetTestTransport{status: 429, body: `{"error":"slow down"}`}
	usable := true
	credit := UsageResetCreditDetail{ID: "cedar-grant", Program: claudeCedarProgram, RemainingCount: 1, Usable: &usable}
	result := ConsumeClaudeResetCredit(context.Background(), resetTestFetchContext(transport), UsageCredential{Type: CredentialOAuth, AccessToken: "tok", OrgID: "org-1"}, "", credit, "")
	if result.Ok || result.Code != "rate_limited" || result.Status != 429 {
		t.Fatalf("result = %+v", result)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("a refused mutation was retried: %d requests", len(transport.requests))
	}
}

func TestListClaudeResetCreditsReadsTheCedarProgramQuery(t *testing.T) {
	transport := &resetTestTransport{status: 200, body: `{
		"cedar_ember": {
			"eligible": true,
			"next_grant_id": "grant-a",
			"grants": [{"id": "grant-a", "resets_left": 2, "label": "One free reset",
				"clears": ["five_hour"], "blocking": [], "usable_now": true, "use_requires_limit": false}]
		}
	}`}
	credential := UsageCredential{Type: CredentialOAuth, AccessToken: "tok"}
	list := ListClaudeResetCredits(context.Background(), resetTestFetchContext(transport), credential, "")
	if list == nil {
		t.Fatal("discovery failed")
	}
	if list.AvailableCount != 2 || list.NextCreditID != "grant-a" {
		t.Fatalf("inventory = %+v", list)
	}
	if list.RedeemableCount == nil || *list.RedeemableCount != 2 {
		t.Fatalf("redeemable = %v", list.RedeemableCount)
	}
	if len(list.Credits) != 1 || list.Credits[0].Clears[0] != "anthropic:5h" {
		t.Fatalf("credits = %+v", list.Credits)
	}
	if list.Credits[0].RequiresLimit == nil || *list.Credits[0].RequiresLimit {
		t.Fatalf("requiresLimit = %v", list.Credits[0].RequiresLimit)
	}
	if got := transport.requests[0].URL.String(); !strings.Contains(got, "cedar_ember=1") {
		t.Fatalf("url = %q", got)
	}
}
