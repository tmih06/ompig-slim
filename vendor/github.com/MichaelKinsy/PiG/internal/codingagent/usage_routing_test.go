package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestFetchAccountSessionUsageUsesOnlyFreshCodexOAuthFacts(t *testing.T) {
	store := newUsageTestStore()
	oauthCredential := ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-access", Expires: time.Now().Add(time.Hour).UnixMilli()}
	ai.SetCredentialAutoReset(&oauthCredential, true)
	store.add("codex-json", new("oauth"), oauthCredential)
	store.add("codex-json", new("api-key"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fixture-key"})
	var calls, autoResetChecks atomic.Int64
	provider := &ai.UsageProvider{
		ID: "codex-json",
		Supports: func(params ai.UsageFetchParams) bool {
			return params.Credential.Type == ai.CredentialOAuth
		},
		FetchUsage: func(_ context.Context, _ ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
			calls.Add(1)
			remaining := 0.6
			used := 0.4
			resetAt := time.Now().Add(3 * time.Hour).UnixMilli()
			duration := int64(5 * time.Hour / time.Millisecond)
			return &ai.UsageReport{
				Provider: "codex-json", FetchedAt: time.Now().UnixMilli(),
				Metadata: map[string]any{"planType": "Free", "email": "private@example.test", "token": "private-token"},
				Limits: []ai.UsageLimit{
					{ID: "codex-json:primary", Scope: ai.UsageScope{Provider: "codex-json", WindowID: "5h"}, Window: &ai.UsageWindow{ID: "5h", DurationMs: &duration, ResetsAt: &resetAt}, Amount: ai.UsageAmount{UsedFraction: &used, RemainingFraction: &remaining, Unit: ai.UsageUnitPercent}, Status: ai.UsageStatusOK},
					{ID: "codex-json:spark:primary", Scope: ai.UsageScope{Provider: "codex-json", Tier: "spark", WindowID: "5h"}, Window: &ai.UsageWindow{ID: "5h", ResetsAt: &resetAt}, Amount: ai.UsageAmount{Unit: ai.UsageUnitPercent}, Status: ai.UsageStatusOK},
					{ID: "codex-json:spend-cap", Scope: ai.UsageScope{Provider: "codex-json", Tier: "credits"}, Window: &ai.UsageWindow{ID: "month"}, Amount: ai.UsageAmount{Unit: ai.UsageUnitCredits}, Status: ai.UsageStatusExhausted},
					{ID: "codex-json:other:primary", Scope: ai.UsageScope{Provider: "codex-json", Tier: "other", ModelID: "other-model"}, Window: &ai.UsageWindow{ID: "5h"}, Amount: ai.UsageAmount{Unit: ai.UsageUnitPercent}, Status: ai.UsageStatusOK},
				},
				Raw: map[string]any{"access_token": "private-token"},
			}, nil
		},
	}
	service := NewUsageService(UsageServiceOptions{
		Credentials: store, Accounts: store, Providers: []*ai.UsageProvider{provider},
		DisableAutoReset:         true,
		AutoResetProviderDefault: func(string) bool { autoResetChecks.Add(1); return true },
		Random:                   func() float64 { return 0.5 },
	})
	facts, err := service.FetchAccountSessionUsage(t.Context(), "codex-json", "gpt-6-luna")
	if err != nil || len(facts) != 1 {
		t.Fatalf("usage facts = %+v, err=%v", facts, err)
	}
	fact := facts[0]
	if fact.AccountKey == nil || *fact.AccountKey != "oauth" || !fact.AccountKeySet || fact.Plan != "free" || len(fact.Windows) != 1 {
		t.Fatalf("unexpected narrow route fact: %+v", fact)
	}
	window := fact.Windows[0]
	if window.ID != "primary" || window.RemainingFraction == nil || *window.RemainingFraction != 0.6 || window.ResetsAt == nil || window.DurationMs == nil || window.Exhausted {
		t.Fatalf("unexpected primary window: %+v", window)
	}
	encoded, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private@example.test", "private-token", "metadata", "raw", "spark", "spend-cap"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("route fact contains unrelated usage data %q: %s", secret, encoded)
		}
	}
	if calls.Load() != 1 || autoResetChecks.Load() != 0 {
		t.Fatalf("probes=%d auto-reset checks=%d, want one OAuth probe and no reset", calls.Load(), autoResetChecks.Load())
	}
	if cached, err := service.FetchAccountSessionUsage(t.Context(), "codex-json", "gpt-6-luna"); err != nil || len(cached) != 1 || calls.Load() != 1 {
		t.Fatalf("routing did not reuse the shared cache: facts=%+v probes=%d err=%v", cached, calls.Load(), err)
	}
	if facts, err := service.FetchAccountSessionUsage(t.Context(), "mistral", "model"); err != nil || len(facts) != 0 {
		t.Fatalf("non-Codex provider produced route facts: %+v %v", facts, err)
	}
}

func TestFetchAccountSessionUsageFailsOpenOnUnavailableOrStaleReports(t *testing.T) {
	for _, test := range []struct {
		name  string
		fetch func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error)
	}{
		{name: "unavailable", fetch: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
			return nil, errors.New("fixture endpoint unavailable")
		}},
		{name: "stale", fetch: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
			return &ai.UsageReport{Provider: "openai-codex", FetchedAt: time.Now().Add(-UsageReportTTL - time.Second).UnixMilli(), Metadata: map[string]any{"planType": "free"}}, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newUsageTestStore()
			store.add("openai-codex", new("account"), ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-access", Expires: time.Now().Add(time.Hour).UnixMilli()})
			service := usageTestService(store, &ai.UsageProvider{ID: "openai-codex", FetchUsage: test.fetch})
			facts, err := service.FetchAccountSessionUsage(t.Context(), "openai-codex", "gpt-6-luna")
			if err != nil || len(facts) != 0 {
				t.Fatalf("unusable usage did not fail open: %+v %v", facts, err)
			}
		})
	}
}

func TestFetchAccountSessionUsagePropagatesCallerCancellation(t *testing.T) {
	store := newUsageTestStore()
	store.add("codex-json", new("account"), ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-access", Expires: time.Now().Add(time.Hour).UnixMilli()})
	started := make(chan struct{})
	provider := &ai.UsageProvider{ID: "codex-json", FetchUsage: func(ctx context.Context, _ ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		close(started)
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}}
	service := usageTestService(store, provider)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := service.FetchAccountSessionUsage(ctx, "codex-json", "gpt-6-luna")
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation was swallowed: %v", err)
	}
}

func TestFetchAccountSessionUsagePropagatesStoreErrors(t *testing.T) {
	base := newUsageTestStore()
	base.add("codex-json", new("account"), ai.Credential{Type: ai.CredentialOAuth, Access: "fixture-access", Expires: time.Now().Add(time.Hour).UnixMilli()})
	store := usageReadErrorStore{usageTestStore: base}
	service := usageTestService(store, &ai.UsageProvider{ID: "codex-json", FetchUsage: func(context.Context, ai.UsageFetchParams, ai.UsageFetchContext) (*ai.UsageReport, error) {
		return nil, errors.New("usage endpoint must not run after the credential store failed")
	}})
	if _, err := service.FetchAccountSessionUsage(t.Context(), "codex-json", "gpt-6-luna"); err == nil || !strings.Contains(err.Error(), "fixture store failure") {
		t.Fatalf("store error was swallowed: %v", err)
	}
}

type usageReadErrorStore struct{ *usageTestStore }

func (store usageReadErrorStore) ReadAccount(context.Context, string, *string) (*ai.Credential, error) {
	return nil, errors.New("fixture store failure")
}

func TestSavedResetRefreshesRouteUsageWithoutDroppingPins(t *testing.T) {
	store := &resetBlockStore{usageTestStore: newUsageTestStore()}
	credential := func(access, plan string) ai.Credential {
		return ai.Credential{
			Type: ai.CredentialOAuth, Access: access, Expires: time.Now().Add(time.Hour).UnixMilli(),
			Extra: map[string]json.RawMessage{"orgName": json.RawMessage(`"` + plan + `"`)},
		}
	}
	freeCredential := credential("free-token", "free")
	ai.SetCredentialAutoReset(&freeCredential, true)
	store.add("openai-codex", new("free"), freeCredential)
	store.add("openai-codex", new("plus"), credential("plus-token", "plus"))
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(store)
	router := registry.AccountRouter()

	// A paid-only route pins Plus before usage facts are installed.
	router.SetSessionUsageSource(nil)
	pinnedModel := &ai.Model{ID: "gpt-6.1-sol", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}
	_, err := router.ImagesRoutes(t.Context(), pinnedModel, ai.AccountRouteRequest{SessionID: "sticky"}, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if key, present := router.PeekAccountKey("openai-codex", "sticky", pinnedModel.ID); !present || key == nil || *key != "plus" {
		t.Fatalf("paid-only route did not pin Plus: key=%v present=%t", key, present)
	}

	routeService, _ := registry.accountSessionUsageService()
	var freeProbes atomic.Int64
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	provider := &ai.UsageProvider{ID: "openai-codex", FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		if params.Credential.AccessToken == "free-token" {
			switch freeProbes.Add(1) {
			case 1:
				return routeUsageReport("free", true), nil
			case 2:
				close(probeStarted)
				// Model a stale provider response that completes after reset cancellation.
				<-releaseProbe
				return routeUsageReport("free", true), nil
			}
			return routeUsageReport("free", false), nil
		}
		return routeUsageReport("plus", false), nil
	}}
	routeService.providers = []*ai.UsageProvider{provider}
	router.SetSessionUsageSource(routeService.FetchAccountSessionUsage)
	warmed, err := routeService.FetchAccountSessionUsage(t.Context(), "openai-codex", "future-model")
	if err != nil {
		t.Fatal(err)
	}
	foundExhaustion := false
	for _, fact := range warmed {
		if fact.AccountKey != nil && *fact.AccountKey == "free" {
			foundExhaustion = len(fact.Windows) > 0 && fact.Windows[0].Exhausted
		}
	}
	if !foundExhaustion {
		t.Fatalf("route service did not cache the exhausted Free report: %+v", warmed)
	}
	preflight := make(chan error, 1)
	go func() {
		_, err := routeService.FetchProviderSnapshot(t.Context(), "openai-codex", true)
		preflight <- err
	}()
	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("route usage probe did not start")
	}

	transport := &resetRouteTransport{routes: map[string]resetRouteResponse{
		"GET /backend-api/wham/rate-limit-reset-credits":          {status: 200, body: `{"credits":[{"id":"credit-a","status":"available"}],"available_count":1}`},
		"POST /backend-api/wham/rate-limit-reset-credits/consume": {status: 200, body: `{"code":"reset"}`},
	}}
	resetService := NewUsageService(UsageServiceOptions{
		Credentials: store, Accounts: store, AccountRouter: router,
		HTTPClient: &http.Client{Transport: transport},
	})
	outcome, err := resetService.RedeemResetCredit(t.Context(), UsageResetTarget{ProviderID: "openai-codex", AccountKey: new("free")})
	if err != nil || !outcome.Ok {
		t.Fatalf("reset = %+v err=%v", outcome, err)
	}
	if key, present := router.PeekAccountKey("openai-codex", "sticky", pinnedModel.ID); !present || key == nil || *key != "plus" {
		t.Fatalf("reset invalidation dropped the valid Plus pin: key=%v present=%t", key, present)
	}

	close(releaseProbe)
	select {
	case err := <-preflight:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pre-reset usage probe did not drain")
	}

	model := &ai.Model{ID: "future-model", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}
	var attempted string
	_, err = router.ImagesRoutes(t.Context(), model, ai.AccountRouteRequest{SessionID: "after-reset"}, func(_ context.Context, selection ai.AccountSelection, _ ai.AccountObserve, _ bool) (ai.AssistantImages, error) {
		attempted = *selection.AccountKey
		return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
	})
	if err != nil || attempted != "free" {
		t.Fatalf("next routed attempt used %q, want refreshed Free account: err=%v", attempted, err)
	}
	if freeProbes.Load() != 3 {
		t.Fatalf("Free usage probes = %d, want cached, stale in-flight, and refreshed probes", freeProbes.Load())
	}
	if got := transport.count("POST", "/backend-api/wham/rate-limit-reset-credits/consume"); got != 1 {
		t.Fatalf("route preflight spent saved resets: consume calls=%d", got)
	}
}

func routeUsageReport(plan string, exhausted bool) *ai.UsageReport {
	remaining := 0.5
	status := ai.UsageStatusOK
	if exhausted {
		remaining = 0
		status = ai.UsageStatusExhausted
	}
	resetAt := time.Now().Add(time.Hour).UnixMilli()
	duration := int64(5 * time.Hour / time.Millisecond)
	return &ai.UsageReport{
		Provider: "openai-codex", FetchedAt: time.Now().UnixMilli(), Metadata: map[string]any{"planType": plan},
		Limits: []ai.UsageLimit{{
			ID: "openai-codex:primary", Scope: ai.UsageScope{Provider: "openai-codex", WindowID: "5h"},
			Window: &ai.UsageWindow{ID: "5h", DurationMs: &duration, ResetsAt: &resetAt},
			Amount: ai.UsageAmount{RemainingFraction: &remaining, Unit: ai.UsageUnitPercent}, Status: status,
		}},
	}
}

func TestModelRegistryRebuildsUsageCacheAfterStoreReplacement(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	registry := NewModelRegistry(t.TempDir())
	defer registry.CloseModelTasks()
	registry.SetCredentialStore(newUsageTestStore())
	router := registry.AccountRouter()
	if facts, err := registry.accountSessionUsage(t.Context(), "openai-codex", "gpt-6-luna"); err != nil || len(facts) != 0 || registry.accountUsageService != nil {
		t.Fatalf("offline routing did not skip usage service access: %+v %v", facts, err)
	}
	first, firstGeneration := registry.accountSessionUsageService()
	registry.SetCredentialStore(newUsageTestStore())
	second, secondGeneration := registry.accountSessionUsageService()
	if first == second || firstGeneration == secondGeneration {
		t.Fatal("credential store replacement retained the previous usage cache")
	}
	if registry.AccountRouter() != router {
		t.Fatal("credential store replacement replaced the live account router")
	}
}
