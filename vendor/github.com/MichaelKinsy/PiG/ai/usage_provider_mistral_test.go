package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// mistralBudgetBody is the shape the dashboard billing_api_public route answers
// with, budget included.
const mistralBudgetBody = `{"vibe_budget": {"usage_percentage": 42.5, "initial_budget": 300.0, "currency": "EUR",
	"reset_at": "2026-11-01T00:00:00Z", "payg_enabled": false},
	"api_budget": {"usage_percentage": 3.0, "initial_budget": 50.0}}`

// mistralServer serves one billing answer and reports the request it received.
func mistralServer(t *testing.T, status int, body string) (*httptest.Server, func() *http.Request) {
	t.Helper()
	var mu sync.Mutex
	var received *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		received = request.Clone(context.Background())
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, body)
	}))
	t.Cleanup(server.Close)
	return server, func() *http.Request {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
}

// mistralFetch probes through the registered provider, as the usage service does.
func mistralFetch(t *testing.T, baseURL string, client *http.Client, credential UsageCredential) (*UsageReport, error) {
	t.Helper()
	provider := DefaultUsageProvider("mistral")
	if provider == nil {
		t.Fatal("mistral usage provider is not registered")
	}
	return provider.FetchUsage(context.Background(), UsageFetchParams{
		Provider:   "mistral",
		BaseURL:    baseURL,
		Credential: credential,
	}, UsageFetchContext{HTTPClient: client})
}

// mistralVibeCredential is the oauth credential the Vibe sign-in stores.
func mistralVibeCredential() UsageCredential {
	return UsageCredential{Type: CredentialOAuth, AccessToken: "fixture-key"}
}

func TestMistralUsageReadsTheVibeCodeBudget(t *testing.T) {
	server, received := mistralServer(t, http.StatusOK, mistralBudgetBody)
	report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report = %+v, want one limit", report)
	}
	limit := report.Limits[0]
	if limit.ID != "mistral:vibe-credits" || limit.Label != "Vibe Credits" {
		t.Fatalf("limit = %q/%q", limit.ID, limit.Label)
	}
	if limit.Amount.Unit != UsageUnitPercent {
		t.Fatalf("unit = %q, want percent", limit.Amount.Unit)
	}
	if limit.Amount.Used == nil || *limit.Amount.Used != 42.5 ||
		limit.Amount.Limit == nil || *limit.Amount.Limit != 100 ||
		limit.Amount.Remaining == nil || *limit.Amount.Remaining != 57.5 ||
		limit.Amount.UsedFraction == nil || *limit.Amount.UsedFraction != 0.425 {
		t.Fatalf("amount = %+v", limit.Amount)
	}
	if limit.Window == nil || limit.Window.ID != "monthly" || limit.Window.Label != "Monthly" {
		t.Fatalf("window = %+v", limit.Window)
	}
	const wantReset = int64(1_793_491_200_000) // 2026-11-01T00:00:00Z
	if limit.Window.ResetsAt == nil || *limit.Window.ResetsAt != wantReset {
		t.Fatalf("resetsAt = %v, want %d", limit.Window.ResetsAt, wantReset)
	}
	if limit.Status != UsageStatusOK {
		t.Fatalf("status = %q, want ok", limit.Status)
	}
	if limit.Scope.Provider != "mistral" || limit.Scope.WindowID != "monthly" || !limit.Scope.Shared {
		t.Fatalf("scope = %+v", limit.Scope)
	}
	if report.Provider != "mistral" || report.FetchedAt == 0 || report.Raw == nil {
		t.Fatalf("report = %+v, want the provider, a fetch time and the payload", report)
	}
	request := received()
	if request == nil {
		t.Fatal("the billing route was never requested")
	}
	if request.URL.Path != "/v1/billing/budget" {
		t.Fatalf("path = %q, want /v1/billing/budget", request.URL.Path)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer fixture-key" {
		t.Fatalf("authorization = %q, want the fixture key as a bearer", got)
	}
}

// TestMistralVibeCreditsNameStaysOutOfCodingPlanRouting pins the billing-name
// marker coding/session_model_usage_health.go modelReportRemaining skips, so a
// pay-as-you-go account past its allowance is never routed as depleted.
func TestMistralVibeCreditsNameStaysOutOfCodingPlanRouting(t *testing.T) {
	server, _ := mistralServer(t, http.StatusOK, mistralBudgetBody)
	report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report = %+v, want one limit", report)
	}
	limit := report.Limits[0]
	probe := strings.ToLower(limit.ID + " " + limit.Label)
	if !strings.Contains(probe, "credit") {
		t.Fatalf("id and label = %q, want the billing-name marker modelReportRemaining skips", probe)
	}
}

func TestMistralPaygOverageStaysAWarning(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		status    UsageStatus
		wantNotes []string
	}{
		{
			name:      "pay as you go on",
			body:      `{"vibe_budget": {"usage_percentage": 130.0, "initial_budget": 300.0, "payg_enabled": true}}`,
			status:    UsageStatusWarning,
			wantNotes: []string{"Usage past the allowance is billed through pay-as-you-go."},
		},
		{
			name:      "pay as you go off",
			body:      `{"vibe_budget": {"usage_percentage": 130.0, "initial_budget": 300.0, "payg_enabled": false}}`,
			status:    UsageStatusExhausted,
			wantNotes: nil,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _ := mistralServer(t, http.StatusOK, test.body)
			report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if report == nil || len(report.Limits) != 1 {
				t.Fatalf("report = %+v, want one limit", report)
			}
			limit := report.Limits[0]
			if limit.Amount.Used == nil || *limit.Amount.Used != 130 {
				t.Fatalf("used = %v, want the unclamped 130", limit.Amount.Used)
			}
			if limit.Amount.UsedFraction == nil || *limit.Amount.UsedFraction != 1.3 {
				t.Fatalf("fraction = %v, want the unclamped 1.3", limit.Amount.UsedFraction)
			}
			if limit.Amount.Remaining == nil || *limit.Amount.Remaining != 0 ||
				limit.Amount.RemainingFraction == nil || *limit.Amount.RemainingFraction != 0 {
				t.Fatalf("remaining = %+v, want none left", limit.Amount)
			}
			if limit.Status != test.status {
				t.Fatalf("status = %q, want %q", limit.Status, test.status)
			}
			if strings.Join(limit.Notes, " ") != strings.Join(test.wantNotes, " ") {
				t.Fatalf("notes = %q, want %q", limit.Notes, test.wantNotes)
			}
		})
	}
}

func TestMistralNegativeUsageClampsToZero(t *testing.T) {
	server, _ := mistralServer(t, http.StatusOK, `{"vibe_budget": {"usage_percentage": -3.0, "initial_budget": 300.0}}`)
	report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report = %+v, want one limit", report)
	}
	limit := report.Limits[0]
	if limit.Amount.Used == nil || *limit.Amount.Used != 0 ||
		limit.Amount.UsedFraction == nil || *limit.Amount.UsedFraction != 0 {
		t.Fatalf("amount = %+v, want a zeroed meter", limit.Amount)
	}
	if limit.Status != UsageStatusOK {
		t.Fatalf("status = %q, want ok", limit.Status)
	}
	// Nothing drives the prompt quota bar's reset countdown from a guessed month.
	if limit.Window.DurationMs != nil {
		t.Fatalf("durationMs = %d, want none for a calendar month", *limit.Window.DurationMs)
	}
	if limit.Window.ResetsAt != nil {
		t.Fatalf("resetsAt = %d, want none for a body without reset_at", *limit.Window.ResetsAt)
	}
}

func TestMistralUsageWithoutAnAllowanceReportsNothing(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"null budget", `{"vibe_budget": null}`},
		{"no budget", `{}`},
		{"zero allowance", `{"vibe_budget": {"usage_percentage": 10.0, "initial_budget": 0}}`},
		{"missing percentage", `{"vibe_budget": {"initial_budget": 300.0}}`},
		{"not a number", `{"vibe_budget": {"usage_percentage": "NaN", "initial_budget": 300.0}}`},
		{"infinite", `{"vibe_budget": {"usage_percentage": "Infinity", "initial_budget": 300.0}}`},
		{"not json", `not json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _ := mistralServer(t, http.StatusOK, test.body)
			report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
			if report != nil || err != nil {
				t.Fatalf("fetch = %+v, %v; want nothing reported", report, err)
			}
		})
	}
}

func TestMistralUsageSurfacesOnlyARejectedKey(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		wantHTTP bool
	}{
		{"unauthorized", http.StatusUnauthorized, true},
		{"missing permission", http.StatusForbidden, false},
		{"no billing route", http.StatusNotFound, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _ := mistralServer(t, test.status, `{}`)
			report, err := mistralFetch(t, server.URL, server.Client(), mistralVibeCredential())
			if report != nil {
				t.Fatalf("report = %+v, want nothing reported", report)
			}
			var httpErr *UsageHTTPError
			if matched := errors.As(err, &httpErr); matched != test.wantHTTP {
				t.Fatalf("error = %v, want a UsageHTTPError: %v", err, test.wantHTTP)
			}
			if test.wantHTTP && httpErr.Status != test.status {
				t.Fatalf("status = %d, want %d", httpErr.Status, test.status)
			}
		})
	}
}

// mistralRewriteTransport records the endpoint the probe built and sends it to
// the test server, so no table case can reach a real origin.
type mistralRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper

	mu       sync.Mutex
	original *url.URL
}

func (tr *mistralRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	tr.original = request.URL
	tr.mu.Unlock()
	clone := request.Clone(request.Context())
	rewritten := *request.URL
	rewritten.Scheme, rewritten.Host = tr.target.Scheme, tr.target.Host
	clone.URL = &rewritten
	clone.Host = tr.target.Host
	return tr.base.RoundTrip(clone)
}

func (tr *mistralRewriteTransport) endpoint() *url.URL {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.original
}

// TestMistralBaseURLJoinsTheBudgetRouteOnce keeps the configured host and path
// prefix while adding exactly one `/v1`, the segment upstream's api_base carries.
func TestMistralBaseURLJoinsTheBudgetRouteOnce(t *testing.T) {
	for _, test := range []struct {
		name     string
		base     string
		wantHost string
		wantPath string
	}{
		{"empty base", "", "api.mistral.ai", "/v1/billing/budget"},
		{"bare host", "https://api.mistral.ai", "api.mistral.ai", "/v1/billing/budget"},
		{"trailing slash", "https://api.mistral.ai/", "api.mistral.ai", "/v1/billing/budget"},
		{"upstream api base", "https://api.mistral.ai/v1", "api.mistral.ai", "/v1/billing/budget"},
		{"path prefix", "https://proxy.example/mistral/v1", "proxy.example", "/mistral/v1/billing/budget"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, received := mistralServer(t, http.StatusOK, mistralBudgetBody)
			target, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("parse test server URL: %v", err)
			}
			transport := &mistralRewriteTransport{target: target, base: server.Client().Transport}
			report, err := mistralFetch(t, test.base, &http.Client{Transport: transport}, mistralVibeCredential())
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if report == nil {
				t.Fatalf("base %q produced no report", test.base)
			}
			endpoint := transport.endpoint()
			if endpoint == nil {
				t.Fatalf("base %q reached no server", test.base)
			}
			if endpoint.Host != test.wantHost {
				t.Fatalf("host = %q, want %q for base %q", endpoint.Host, test.wantHost, test.base)
			}
			if endpoint.Path != test.wantPath {
				t.Fatalf("path = %q, want %q for base %q", endpoint.Path, test.wantPath, test.base)
			}
			if request := received(); request == nil || request.URL.Path != test.wantPath {
				t.Fatalf("test server received %v, want %q", request, test.wantPath)
			}
		})
	}
}

func TestMistralCredentialShapesShareTheBearer(t *testing.T) {
	for _, test := range []struct {
		name       string
		credential UsageCredential
	}{
		{"browser sign-in", UsageCredential{Type: CredentialOAuth, AccessToken: "fixture-key"}},
		{"pasted key", UsageCredential{Type: CredentialAPIKey, APIKey: "fixture-key"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, received := mistralServer(t, http.StatusOK, mistralBudgetBody)
			report, err := mistralFetch(t, server.URL, server.Client(), test.credential)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if report == nil {
				t.Fatalf("credential %+v produced no report", test.credential)
			}
			request := received()
			if request == nil {
				t.Fatal("the billing route was never requested")
			}
			if got := request.Header.Get("Authorization"); got != "Bearer fixture-key" {
				t.Fatalf("authorization = %q, want the fixture key as a bearer", got)
			}
		})
	}
}

func TestMistralProviderRefusesAnEmptyToken(t *testing.T) {
	provider := DefaultUsageProvider("mistral")
	if provider == nil {
		t.Fatal("mistral usage provider is not registered")
	}
	for _, credential := range []UsageCredential{
		{Type: CredentialOAuth},
		{Type: CredentialAPIKey},
	} {
		if provider.SupportsCredential(UsageFetchParams{Provider: "mistral", Credential: credential}) {
			t.Fatalf("provider accepted %+v, want an empty token refused", credential)
		}
	}
	if provider.SupportsCredential(UsageFetchParams{Provider: "zai", Credential: mistralVibeCredential()}) {
		t.Fatal("provider answered for another provider id")
	}
}

// TestMistralProviderIsRegisteredForAVibeCredential fails when the registry
// line is dropped, which would leave `/usage` and the prompt bar silent.
func TestMistralProviderIsRegisteredForAVibeCredential(t *testing.T) {
	provider := DefaultUsageProvider("mistral")
	if provider == nil {
		t.Fatal("mistral usage provider is not registered")
	}
	if !provider.SupportsCredential(UsageFetchParams{Provider: "mistral", Credential: mistralVibeCredential()}) {
		t.Fatal("the registered provider does not accept the key the Vibe sign-in stores")
	}
	if !provider.ValidatesCredentials {
		t.Fatal("the registered provider does not authenticate its credential")
	}
}

func TestMistralCancelledContextReturnsTheCause(t *testing.T) {
	server, _ := mistralServer(t, http.StatusOK, mistralBudgetBody)
	cause := errors.New("usage pass abandoned")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	provider := DefaultUsageProvider("mistral")
	if provider == nil {
		t.Fatal("mistral usage provider is not registered")
	}
	report, err := provider.FetchUsage(ctx, UsageFetchParams{
		Provider:   "mistral",
		BaseURL:    server.URL,
		Credential: mistralVibeCredential(),
	}, UsageFetchContext{HTTPClient: server.Client()})
	if report != nil || !errors.Is(err, cause) {
		t.Fatalf("fetch = %+v, %v; want %v", report, err, cause)
	}
}
