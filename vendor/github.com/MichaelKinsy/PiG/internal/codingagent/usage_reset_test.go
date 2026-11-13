package codingagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// resetRouteTransport answers the reset routes of both providers from one table
// and records the requests it saw, so a test can assert the wire contract and
// the number of spends without network access.
type resetRouteTransport struct {
	// mu guards the recording slices: one Codex probe issues its usage and
	// entitlement requests concurrently, so RoundTrip runs on several goroutines.
	mu       sync.Mutex
	routes   map[string]resetRouteResponse
	requests []*http.Request
	bodies   []string
}

type resetRouteResponse struct {
	status int
	body   string
}

func (transport *resetRouteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	payload := ""
	if request.Body != nil {
		encoded, _ := io.ReadAll(request.Body)
		payload = string(encoded)
	}
	transport.mu.Lock()
	transport.requests = append(transport.requests, request)
	transport.bodies = append(transport.bodies, payload)
	transport.mu.Unlock()
	key := request.Method + " " + request.URL.Path
	response, ok := transport.routes[key]
	if !ok {
		response = resetRouteResponse{status: 404, body: `{"error":"not found"}`}
	}
	return &http.Response{
		StatusCode: response.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(response.body)),
		Request:    request,
	}, nil
}

// resetRouteRequests counts the requests that used method and path.
func (transport *resetRouteTransport) count(method, path string) int {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	count := 0
	for _, request := range transport.requests {
		if request.Method == method && request.URL.Path == path {
			count++
		}
	}
	return count
}

// resetTestOAuthCredential is an unexpired OAuth credential: a zero expiry
// reads as expired and makes every usage probe decline the credential.
func resetTestOAuthCredential() ai.Credential {
	return ai.Credential{Type: ai.CredentialOAuth, Access: "tok", Expires: time.Now().Add(time.Hour).UnixMilli()}
}

// resetTestService builds a quota service over a single-account store holding
// one credential per provider.
func resetTestService(t *testing.T, transport *resetRouteTransport, providerID string, credential ai.Credential) *UsageService {
	t.Helper()
	store := newUsageTestStore()
	store.add(providerID, nil, credential)
	return NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		HTTPClient:  &http.Client{Transport: transport},
	})
}

func TestAutoResetRedeemsAnAvailableCreditForAnExhaustedAccount(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-auto","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
	}}
	credential := resetTestOAuthCredential()
	store := newUsageTestStore()
	store.add("openai-codex", nil, credential)
	used := 1.0
	service := NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		Providers: []*ai.UsageProvider{{ID: "openai-codex", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
			return &ai.UsageReport{Provider: "openai-codex", Limits: []ai.UsageLimit{{Status: ai.UsageStatusExhausted, Amount: ai.UsageAmount{UsedFraction: &used}}}}, nil
		}}},
		HTTPClient:               &http.Client{Transport: transport},
		AutoResetProviderDefault: func(providerID string) bool { return providerID == "openai-codex" },
	})
	if _, err := service.FetchSnapshot(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(transport.requests) != 2 || transport.requests[1].Method != "POST" {
		t.Fatalf("requests = %d %+v", len(transport.requests), transport.requests)
	}
}

func TestListResetCreditsReportsEveryAccountWithItsSession(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{
			"credits": [{"id": "credit-a", "status": "available", "expires_at": "2099-01-01T00:00:00Z"}],
			"available_count": 1
		}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	entries, err := service.ListResetCredits(context.Background())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	entry := entries[0]
	if entry.ProviderID != "openai-codex" || entry.Err != "" || !entry.Active {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Credits == nil || entry.Credits.AvailableCount != 1 {
		t.Fatalf("credits = %+v", entry.Credits)
	}
	if !strings.Contains(transport.requests[0].Header.Get("Authorization"), "Bearer") {
		t.Fatalf("authorization = %q", transport.requests[0].Header.Get("Authorization"))
	}
}

func TestListResetCreditsIncludesCodexJSON(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{"credits":[{"id":"credit-json","status":"available"}],"available_count":1}`},
	}}
	service := resetTestService(t, transport, "codex-json", resetTestOAuthCredential())
	entries, err := service.ListResetCredits(context.Background())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 1 || entries[0].ProviderID != "codex-json" || entries[0].Err != "" || entries[0].Credits == nil || entries[0].Credits.AvailableCount != 1 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestListResetCreditsSkipsProvidersWithoutResets(t *testing.T) {
	store := newUsageTestStore()
	store.add("openrouter", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "k"})
	service := NewUsageService(UsageServiceOptions{Credentials: store, Accounts: store})
	entries, err := service.ListResetCredits(context.Background())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestListResetCreditsKeepsAnUnreadableAccountVisible(t *testing.T) {
	transport := &resetRouteTransport{}
	store := newUsageTestStore()
	store.add("openai-codex", nil, resetTestOAuthCredential())
	service := NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		HTTPClient:  &http.Client{Transport: transport},
	})
	entries, err := service.ListResetCredits(context.Background())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 1 || entries[0].Err == "" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Credits != nil {
		t.Fatalf("failed account reported credits: %+v", entries[0].Credits)
	}
}

func TestRedeemResetCreditSpendsAndInvalidatesUsage(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{
			"credits": [{"id": "credit-a", "status": "available", "expires_at": "2099-01-01T00:00:00Z"}],
			"available_count": 1
		}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
		"GET /backend-api/wham/usage": {status: 200, body: `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,
			"primary_window":{"used_percent":10,"limit_window_seconds":18000}}}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())

	// Warm the usage cache so the redeem has something to invalidate.
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("usage: %v", err)
	}
	usageReads := transport.count("GET", "/backend-api/wham/usage")
	if usageReads == 0 {
		t.Fatal("usage probe never ran")
	}

	outcome, err := service.RedeemResetCredit(context.Background(), UsageResetTarget{ProviderID: "openai-codex"})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if !outcome.Ok || outcome.Code != "reset" || outcome.CreditID != "credit-a" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if got := transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume"); got != 1 {
		t.Fatalf("consume calls = %d", got)
	}
	// A cached report must not outlive the reset: the next read probes again.
	if _, err := service.FetchSnapshot(context.Background(), false); err != nil {
		t.Fatalf("usage after reset: %v", err)
	}
	if got := transport.count("GET", "/backend-api/wham/usage"); got <= usageReads {
		t.Fatalf("usage cache survived the reset: %d reads, was %d", got, usageReads)
	}
}

func TestRedeemResetCreditSurfacesAProviderRefusal(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"nothing_to_reset"}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	outcome, err := service.RedeemResetCredit(context.Background(), UsageResetTarget{ProviderID: "openai-codex"})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if outcome.Ok || outcome.Code != "nothing_to_reset" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(outcome.Cleared) != 0 {
		t.Fatalf("refusal reported cleared windows: %v", outcome.Cleared)
	}
}

func TestRedeemResetCreditRefusesAnUnnamedProvider(t *testing.T) {
	service := resetTestService(t, &resetRouteTransport{}, "openai-codex", resetTestOAuthCredential())
	outcome, err := service.RedeemResetCredit(context.Background(), UsageResetTarget{ProviderID: "openrouter"})
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if outcome.Code != "unsupported_provider" {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestResetUsageAccountsRankTheSpendableAccountFirst(t *testing.T) {
	entries := []UsageResetCreditAccount{
		{
			ProviderID: "anthropic",
			Label:      "idle@example.com",
			Credits: &ai.UsageResetCredits{
				AvailableCount:  1,
				RedeemableCount: new(0),
				Credits:         []ai.UsageResetCreditDetail{{ID: "g", Title: "Free reset", Status: "unavailable", RemainingCount: 1}},
			},
		},
		{
			ProviderID: "openai-codex",
			Label:      "active@example.com",
			Active:     true,
			Credits: &ai.UsageResetCredits{
				AvailableCount: 2,
				Credits: []ai.UsageResetCreditDetail{
					{ID: "later", Status: "available", ExpiresAt: "2099-06-01T00:00:00Z", RemainingCount: 1, Title: "One free reset"},
					{ID: "sooner", Status: "available", ExpiresAt: "2099-01-01T00:00:00Z", RemainingCount: 1, Title: "One free reset"},
				},
			},
		},
	}
	rows := resetUsageAccounts(entries)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].ProviderID != "openai-codex" {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[0].AvailableCount != 2 || rows[0].RedeemableCount != 2 {
		t.Fatalf("counts = %d/%d", rows[0].AvailableCount, rows[0].RedeemableCount)
	}
	// The summary reports the soonest expiring banked credit as the provider
	// stamped it, so its countdown and the credit agree.
	if rows[0].ExpiresAt != "2099-01-01T00:00:00Z" {
		t.Fatalf("expiry = %q", rows[0].ExpiresAt)
	}
	if rows[1].RedeemableCount != 0 || rows[1].UnavailableReason == "" {
		t.Fatalf("unusable row = %+v", rows[1])
	}
}

// TestResetUsageAccountsPreserveTieOrder pins stable ordering independently of
// the sorting algorithm: equivalent rows retain their inventory order.
func TestResetUsageAccountsPreserveTieOrder(t *testing.T) {
	entries := []UsageResetCreditAccount{
		{ProviderID: "openai-codex", Label: "same", AccountKey: new("first")},
		{ProviderID: "openai-codex", Label: "same", AccountKey: new("second")},
	}
	rows := resetUsageAccounts(entries)
	if len(rows) != 2 || rows[0].TargetRef != "first" || rows[1].TargetRef != "second" {
		t.Fatalf("equal-ranked rows lost inventory order: %+v", rows)
	}
}

// TestResetAccountsKeepProviderAndStoreOrder checks the service-level order
// before presentation ranking: Codex precedes Claude, with each store's order
// retained even when other providers are interleaved in its listing.
func TestResetAccountsKeepProviderAndStoreOrder(t *testing.T) {
	store := newUsageTestStore()
	store.add("anthropic", new("claude-first"), resetTestOAuthCredential())
	store.add("openrouter", new("ignored"), ai.Credential{Type: ai.CredentialAPIKey})
	store.add("openai-codex", new("codex-first"), resetTestOAuthCredential())
	store.add("anthropic", new("claude-second"), resetTestOAuthCredential())
	store.add("openai-codex", new("codex-second"), resetTestOAuthCredential())
	service := NewUsageService(UsageServiceOptions{Credentials: store, Accounts: store})
	accounts, err := service.resetAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex-first", "codex-second", "claude-first", "claude-second"}
	if len(accounts) != len(want) {
		t.Fatalf("accounts = %+v", accounts)
	}
	for index, account := range accounts {
		if account.AccountKey == nil || *account.AccountKey != want[index] {
			t.Fatalf("account %d = %+v, want %s", index, account, want[index])
		}
	}
}

func TestResolveResetTargetAcceptsActiveAndKeepsProblems(t *testing.T) {
	rows := []tui.ResetUsageAccount{
		{Label: "selected@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex", Active: true, AvailableCount: 1, RedeemableCount: 1, TargetRef: "key-1"},
		{Label: "idle@example.com", ProviderID: "anthropic", ProviderLabel: "Claude", AvailableCount: 2, RedeemableCount: 0, TargetRef: "key-2", UnavailableReason: "not usable right now"},
	}
	target, account, problem := resetUsageTargetFromArgs(rows, "codex/active", "/usage reset")
	if problem != "" {
		t.Fatalf("active target: %s", problem)
	}
	if account.Label != "selected@example.com" {
		t.Fatalf("account = %+v", account)
	}
	if target.ProviderID != "openai-codex" || target.AccountKey == nil || *target.AccountKey != "key-1" {
		t.Fatalf("target = %+v", target)
	}

	if _, _, problem := resetUsageTargetFromArgs(rows, "claude/key-2", "/usage reset"); !strings.Contains(problem, "no saved resets usable") {
		t.Fatalf("problem = %q", problem)
	}
	if _, _, problem := resetUsageTargetFromArgs(rows, "nope/key", "/usage reset"); !strings.Contains(problem, "Unknown reset provider") {
		t.Fatalf("problem = %q", problem)
	}
	if _, _, problem := resetUsageTargetFromArgs(rows, "codex", "/usage reset"); !strings.Contains(problem, "<provider>/<credential>") {
		t.Fatalf("problem = %q", problem)
	}
}

func TestDescribeResetOutcomeNamesTheResetScope(t *testing.T) {
	cases := []struct {
		outcome UsageResetOutcome
		want    string
	}{
		{UsageResetOutcome{ProviderID: "anthropic", Code: "reset", Cleared: []string{"anthropic:5h"}}, "Claude's 5h session limit has been refreshed; weekly limits are unchanged"},
		{UsageResetOutcome{ProviderID: "openai-codex", Code: "reset"}, "your rate-limit window has been refreshed"},
		{UsageResetOutcome{ProviderID: "openai-codex", Code: "already_redeemed"}, "that reset was already redeemed"},
		{UsageResetOutcome{ProviderID: "anthropic", Code: "network_error"}, "couldn't confirm whether the reset applied"},
		{UsageResetOutcome{ProviderID: "anthropic", Code: "offer_changed"}, "the reset offer changed"},
	}
	for _, tc := range cases {
		got := describeResetOutcome(tc.outcome, "a@example.com")
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%q does not contain %q", got, tc.want)
		}
		if strings.Contains(got, "\n") {
			t.Fatalf("outcome message spans lines: %q", got)
		}
	}
}

// resetSlashContext builds the minimal dispatch context for a headless
// `/usage` run.
func resetSlashContext(output *[]string) *SlashContext {
	return &SlashContext{
		Append:     func(message string) { *output = append(*output, message) },
		AppendText: func(message string) { *output = append(*output, message) },
		ShowError:  func(message string) { *output = append(*output, message) },
	}
}

func TestUsageHandlerListsResetsWithoutAPicker(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{
			"credits": [{"id": "credit-a", "status": "available", "expires_at": "2099-01-01T00:00:00Z"}],
			"available_count": 1
		}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	var output []string
	sc := resetSlashContext(&output)
	sc.Args = "reset"
	sc.ListResetCredits = service.ListResetCredits
	sc.RedeemResetCredit = func(ctx context.Context, reference string) (UsageResetOutcome, error) {
		t.Fatalf("a bare listing spent %q", reference)
		return UsageResetOutcome{}, nil
	}
	if err := usageHandler(sc); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(output) != 1 {
		t.Fatalf("output = %#v", output)
	}
	for _, want := range []string{"Saved rate-limit resets:", "1 saved, 1 usable now", "openai-codex/"} {
		if !strings.Contains(output[0], want) {
			t.Fatalf("listing lacks %q:\n%s", want, output[0])
		}
	}
	if transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume") != 0 {
		t.Fatal("listing spent a credit")
	}
}

func TestUsageHandlerOpensAutoResetSettings(t *testing.T) {
	opened := 0
	sc := resetSlashContext(new([]string))
	sc.Args = "auto-reset"
	sc.ShowAutoResetSelector = func() { opened++ }
	if err := usageHandler(sc); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if opened != 1 {
		t.Fatalf("auto-reset picker opened %d times", opened)
	}
}

func TestUsageHandlerOpensThePickerWhenTheHostHasOne(t *testing.T) {
	opened := 0
	sc := resetSlashContext(new([]string))
	sc.Args = "reset"
	sc.ShowResetUsageSelector = func() { opened++ }
	if err := usageHandler(sc); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if opened != 1 {
		t.Fatalf("picker opened %d times", opened)
	}

	// A typed target never opens the picker: the caller already chose.
	listed := 0
	components := resetSlashContext(new([]string))
	components.Args = "reset codex/active"
	components.ShowResetUsageSelector = func() { t.Fatal("typed target opened the picker") }
	components.ListResetCredits = func(context.Context) ([]UsageResetCreditAccount, error) {
		listed++
		return nil, nil
	}
	if err := usageHandler(components); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if listed != 1 {
		t.Fatalf("typed target listed %d times", listed)
	}
}

func TestUsageHandlerRedeemsATypedTarget(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	var output []string
	sc := resetSlashContext(&output)
	sc.Args = "reset codex/active"
	sc.ListResetCredits = service.ListResetCredits
	sc.RedeemResetCredit = func(ctx context.Context, reference string) (UsageResetOutcome, error) {
		target, _, problem := resetUsageTargetFromArgs(resetUsageAccounts(mustList(t, service)), reference, "/usage reset")
		if problem != "" {
			t.Fatalf("reference %q: %s", reference, problem)
		}
		return service.RedeemResetCredit(ctx, target)
	}
	if err := usageHandler(sc); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(output) != 1 || !strings.Contains(output[0], "Reset applied") {
		t.Fatalf("output = %#v", output)
	}
	if got := transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume"); got != 1 {
		t.Fatalf("consume calls = %d", got)
	}
}

// mustList reads a service's inventory or fails the test.
func mustList(t *testing.T, service UsageResetQuotaService) []UsageResetCreditAccount {
	t.Helper()
	entries, err := service.ListResetCredits(context.Background())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	return entries
}

func TestUsageHandlerRejectsUnknownArguments(t *testing.T) {
	for _, args := range []string{"resets", "show extra", "reset later"} {
		var output []string
		sc := resetSlashContext(&output)
		sc.Args = args
		if err := usageHandler(sc); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if len(output) == 0 {
			t.Fatalf("%q produced no message", args)
		}
		if args == "resets" && !strings.Contains(output[0], "Usage: /usage") {
			t.Fatalf("%q message = %q", args, output[0])
		}
	}
}

func TestUsageSubcommandSplitsVerbAndRest(t *testing.T) {
	cases := []struct {
		args, verb, rest string
	}{
		{"", "", ""},
		{"show", "show", ""},
		{"show  ", "show", ""},
		{"reset", "reset", ""},
		{"reset  codex/active", "reset", "codex/active"},
		{"  RESET codex/key  ", "reset", "codex/key"},
	}
	for _, tc := range cases {
		verb, rest := parseUsageSubcommand(tc.args)
		if verb != tc.verb || rest != tc.rest {
			t.Fatalf("%q = (%q,%q), want (%q,%q)", tc.args, verb, rest, tc.verb, tc.rest)
		}
	}
}

func TestResetBlockScopesKeepUncoveredBlocks(t *testing.T) {
	// Codex resets its chat meter: an auth or model-policy block must survive.
	if got := resetBlockScopes("openai-codex", nil); strings.Join(got, ",") != "chat,shared" {
		t.Fatalf("codex scopes = %v", got)
	}
	// A Claude session-only reset cannot lift the weekly wall, so the unscoped
	// row stays.
	if got := resetBlockScopes("anthropic", []string{"anthropic:5h"}); strings.Join(got, ",") != "anthropic:5h" {
		t.Fatalf("partial Claude scopes = %v", got)
	}
	// Both shared windows covered means the whole-account wall went too.
	if got := resetBlockScopes("anthropic", []string{"anthropic:5h", "anthropic:7d"}); strings.Join(got, ",") != "anthropic:5h,anthropic:7d," {
		t.Fatalf("full Claude scopes = %v", got)
	}
	// An answer that names no window lifts nothing.
	if got := resetBlockScopes("anthropic", nil); got != nil {
		t.Fatalf("empty Claude scopes = %v", got)
	}
	if got := resetBlockScopes("openrouter", []string{"x"}); got != nil {
		t.Fatalf("unknown provider scopes = %v", got)
	}
}

// TestResetBlockScopesDoNotAliasProviderCoverage protects ownership: callers
// may change the returned scopes without mutating the provider's cleared list.
func TestResetBlockScopesDoNotAliasProviderCoverage(t *testing.T) {
	cleared := []string{"anthropic:5h", "anthropic:7d"}
	scopes := resetBlockScopes("anthropic", cleared)
	scopes[0] = "changed"
	if len(cleared) != 2 || cleared[0] != "anthropic:5h" || cleared[1] != "anthropic:7d" {
		t.Fatalf("scope calculation mutated provider coverage: %v", cleared)
	}
}

// resetBlockStore is a single-account credential store that records the block
// clears it receives.
type resetBlockStore struct {
	*usageTestStore
	cleared [][]string
}

func (store *resetBlockStore) ClearAccountBlocks(ctx context.Context, providerID string, accountKey *string, scopes []string) error {
	store.cleared = append(store.cleared, append([]string{}, scopes...))
	return nil
}

func TestRedeemResetCreditClearsCoveredBlocks(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
	}}
	store := &resetBlockStore{usageTestStore: newUsageTestStore()}
	store.add("openai-codex", nil, resetTestOAuthCredential())
	service := NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		HTTPClient:  &http.Client{Transport: transport},
	})
	outcome, err := service.RedeemResetCredit(context.Background(), UsageResetTarget{ProviderID: "openai-codex"})
	if err != nil || !outcome.Ok {
		t.Fatalf("redeem = %+v err=%v", outcome, err)
	}
	if len(store.cleared) != 1 || strings.Join(store.cleared[0], ",") != "chat,shared" {
		t.Fatalf("cleared = %v", store.cleared)
	}
}

func TestRedeemResetCreditKeepsBlocksOnAProviderRefusal(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"nothing_to_reset"}`},
	}}
	store := &resetBlockStore{usageTestStore: newUsageTestStore()}
	store.add("openai-codex", nil, resetTestOAuthCredential())
	service := NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		HTTPClient:  &http.Client{Transport: transport},
	})
	if _, err := service.RedeemResetCredit(context.Background(), UsageResetTarget{ProviderID: "openai-codex"}); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if len(store.cleared) != 0 {
		t.Fatalf("a refusal cleared blocks: %v", store.cleared)
	}
}

func TestResolveResetTargetHandlesKeylessAccounts(t *testing.T) {
	// One keyless account is nameable through its provider alone.
	single := []tui.ResetUsageAccount{
		{Label: "only@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex", Active: true, AvailableCount: 1, RedeemableCount: 1},
	}
	if target, _, problem := resetUsageTargetFromArgs(single, "codex/", "/usage reset"); problem != "" {
		t.Fatalf("single keyless account: %s", problem)
	} else if target.AccountKey != nil {
		t.Fatalf("keyless target carried a key: %v", *target.AccountKey)
	}
	// Two keyless accounts of one provider are ambiguous: spending either would
	// be a guess about which account the user meant.
	ambiguous := []tui.ResetUsageAccount{
		{Label: "a@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex", Active: true, AvailableCount: 1, RedeemableCount: 1},
		{Label: "b@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex", AvailableCount: 1, RedeemableCount: 1},
	}
	if _, _, problem := resetUsageTargetFromArgs(ambiguous, "codex/", "/usage reset"); problem == "" {
		t.Fatal("an ambiguous target resolved anyway")
	}
	// Three candidates must not resurrect a match from the last one.
	triple := append(ambiguous, tui.ResetUsageAccount{Label: "c@example.com", ProviderID: "openai-codex", ProviderLabel: "Codex", AvailableCount: 1, RedeemableCount: 1})
	if _, _, problem := resetUsageTargetFromArgs(triple, "codex/", "/usage reset"); problem == "" {
		t.Fatal("a three-account provider resolved without a key")
	}
}

// resetWiringTest builds an InteractiveMode with only the state a reset command
// touches: a transcript container, an editor slot, a renderer, and a quota
// service over the fixture transport.
func resetWiringTest(t *testing.T, service UsageQuotaService) *InteractiveMode {
	t.Helper()
	mode := &InteractiveMode{
		opts: InteractiveModeOptions{UsageService: service, CWD: t.TempDir()},
	}
	mode.chatContainer = tui.NewContainer()
	mode.editorContainer = tui.NewContainer()
	mode.editor = tui.NewEditor()
	mode.tuiInst = tui.NewWithOutput(io.Discard, 120, 40)
	// The owner-loop queue a real mode drains every frame; the drain helper
	// below stands in for it so a background outcome is published.
	mode.uiTaskCh = make(chan func(), 16)
	return mode
}

// drainOwnerLoop runs every queued owner-loop task, which is what the input loop
// does between frames.
func drainOwnerLoop(mode *InteractiveMode) {
	for {
		select {
		case task := <-mode.uiTaskCh:
			task()
		default:
			return
		}
	}
}

// renderedTranscript flattens the chat container into plain text for assertion.
func renderedTranscript(mode *InteractiveMode) string {
	lines := mode.chatContainer.Render(120)
	for index, line := range lines {
		lines[index] = tui.SanitizeDisplayLine(line)
	}
	return strings.Join(lines, "\n")
}

func TestShowResetUsageSelectorExplainsMissingResets(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{"credits":[],"available_count":0}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	mode := resetWiringTest(t, service)
	mode.showResetUsageSelector()
	// The progress line is replaced by the verdict, as upstream's status line is
	// reused rather than appended to.
	if transcript := renderedTranscript(mode); !strings.Contains(transcript, "No saved rate-limit resets available") {
		t.Fatalf("no empty explanation: %q", transcript)
	}
}

func TestShowResetUsageSelectorReportsUnreachableAccounts(t *testing.T) {
	// The fixture has no reset route, so the inventory read fails.
	mode := resetWiringTest(t, resetTestService(t, &resetRouteTransport{}, "openai-codex", resetTestOAuthCredential()))
	mode.showResetUsageSelector()
	if transcript := renderedTranscript(mode); !strings.Contains(transcript, "some accounts couldn't be reached") {
		t.Fatalf("transcript = %q", transcript)
	}
}

func TestShowResetUsageSelectorWithoutAQuotaService(t *testing.T) {
	mode := resetWiringTest(t, nil)
	mode.opts.UsageService = nil
	mode.usageService = nil
	mode.showResetUsageSelector()
	if transcript := renderedTranscript(mode); !strings.Contains(transcript, "unavailable in this session") {
		t.Fatalf("transcript = %q", transcript)
	}
}

func TestRedeemResetCreditThroughTheModeReportsTheOutcome(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	mode := resetWiringTest(t, service)
	mode.backgroundCtx = t.Context()
	rows := resetUsageAccounts(mustList(t, service))
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	mode.redeemResetRow(rows[0])
	mode.backgroundTasks.Wait()
	drainOwnerLoop(mode)
	transcript := renderedTranscript(mode)
	if !strings.Contains(transcript, "Reset applied") {
		t.Fatalf("no spend outcome: %q", transcript)
	}
	if strings.Contains(transcript, "Error:") {
		t.Fatalf("a successful spend reported an error: %q", transcript)
	}
	if got := transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume"); got != 1 {
		t.Fatalf("consume calls = %d", got)
	}
}

func TestRedeemResetCreditThroughTheModeKeepsAProblemAwayFromTheProvider(t *testing.T) {
	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits": {status: 200, body: `{"credits":[],"available_count":0}`},
	}}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	mode := resetWiringTest(t, service)
	outcome, err := mode.redeemResetCredit(t.Context(), "codex/active")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if outcome.Ok {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Reason == "" {
		t.Fatalf("outcome carries no explanation: %+v", outcome)
	}
	if got := transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume"); got != 0 {
		t.Fatalf("an ineligible account spent a credit: %d calls", got)
	}
}

func TestListResetCreditsWithoutAServiceIsEmpty(t *testing.T) {
	mode := resetWiringTest(t, nil)
	mode.opts.UsageService = nil
	entries, err := mode.listResetCredits(t.Context())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestRedeemResetCreditJoinsAnExistingSpend(t *testing.T) {
	transport := &resetRouteTransport{}
	service := resetTestService(t, transport, "openai-codex", resetTestOAuthCredential())
	// Hold an admitted operation explicitly rather than relying on scheduler
	// timing: two sequential spends are valid, two simultaneous ones are not.
	flight := &redeemInFlight{creditID: "credit-a", promise: make(chan struct{})}
	service.redeemInFlight = map[string]*redeemInFlight{"openai-codex\x00": flight}
	other, err := service.RedeemResetCredit(t.Context(), UsageResetTarget{ProviderID: "openai-codex", CreditID: "other"})
	if err != nil || other.Code != "reset_in_progress" {
		t.Fatalf("different credit: %+v, %v", other, err)
	}
	flight.outcome = UsageResetOutcome{Ok: true, Code: "reset", CreditID: "credit-a"}
	close(flight.promise)
	joined, err := service.RedeemResetCredit(t.Context(), UsageResetTarget{ProviderID: "openai-codex", CreditID: "credit-a"})
	if err != nil || !joined.Ok || joined.CreditID != "credit-a" {
		t.Fatalf("joined result: %+v, %v", joined, err)
	}
	if len(transport.requests) != 0 {
		t.Fatal("joined redemption issued another request")
	}
}
