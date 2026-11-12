package codingagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// remoteQuotaTransport rejects an obsolete bearer without any live provider
// traffic. A forced broker refresh must preserve each account's identity.
type remoteQuotaTransport struct {
	status      int
	rejectFresh bool
	legacy      bool
	fresh       atomic.Int32
}

// RoundTrip serves real quota shapes; entitlement probes are irrelevant here.
func (transport *remoteQuotaTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, `{}`
	if strings.HasSuffix(request.URL.Path, "verified_access") || transport.legacy && strings.HasSuffix(request.URL.Path, "retrieveUserQuotaSummary") {
		status = http.StatusNotFound
	} else if request.Header.Get("Authorization") == "Bearer obsolete" {
		status = transport.status
	} else {
		transport.fresh.Add(1)
		if request.Header.Get("Authorization") != "Bearer fresh-one" && request.Header.Get("Authorization") != "Bearer fresh-two" {
			status = http.StatusUnauthorized
		} else if transport.rejectFresh {
			status = http.StatusUnauthorized
		} else if strings.HasSuffix(request.URL.Path, "wham/usage") {
			body = `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":25,"limit_window_seconds":18000}}}`
		} else if transport.legacy {
			body = `{"models":{"gemini-test":{"modelProvider":"MODEL_PROVIDER_GOOGLE","weeklyQuotaInfo":{"remainingFraction":0.75}}}}`
		} else {
			body = `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.75}]}]}`
		}
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// TestRemoteUsageRecoversRejectedBearer reproduces unavailable Antigravity
// quota and an unknown Codex plan when a broker token is rejected before expiry.
// Only 401 may refresh; a second rejection must terminate without logging out.
func TestRemoteUsageRecoversRejectedBearer(t *testing.T) {
	for _, providerID := range []string{ai.GoogleAntigravityProviderID, "openai-codex"} {
		for _, scenario := range []struct {
			name        string
			status      int
			rejectFresh bool
			legacy      bool
		}{
			{"rotated", http.StatusUnauthorized, false, false},
			{"legacy-rotated", http.StatusUnauthorized, false, true},
			{"still-rejected", http.StatusUnauthorized, true, false},
			{"forbidden", http.StatusForbidden, false, false},
			{"server-error", http.StatusInternalServerError, false, false},
		} {
			t.Run(providerID+"/"+scenario.name, func(t *testing.T) {
				store := &usageRemoteStore{newUsageTestStore()}
				for _, key := range []string{"one", "two"} {
					store.add(providerID, &key, ai.Credential{Type: ai.CredentialOAuth, Access: "obsolete", Refresh: ai.RemoteRefreshSentinel, Expires: time.Now().Add(time.Hour).UnixMilli(), ProjectID: "fixture-project", AccountID: key})
				}
				registry := NewModelRegistryWithModelsPath("")
				registry.SetCredentialStore(store)
				transport := &remoteQuotaTransport{status: scenario.status, rejectFresh: scenario.rejectFresh, legacy: scenario.legacy && providerID == ai.GoogleAntigravityProviderID}
				var refreshes atomic.Int32
				service := NewUsageService(UsageServiceOptions{Credentials: store, Accounts: store, HTTPClient: &http.Client{Transport: transport}, GetAuth: func(ctx context.Context, id string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
					if !overrides.ForceOAuthRefresh || !overrides.AccountKeySet {
						t.Error("recovery must force refresh of the exact account")
					}
					refreshes.Add(1)
					return registry.ResolveRegistryProviderAuth(ctx, id, overrides)
				}})
				snapshot, err := service.FetchProviderSnapshot(context.Background(), providerID, true)
				if err != nil {
					t.Fatal(err)
				}
				wantReports := 0
				if scenario.status == http.StatusUnauthorized && !scenario.rejectFresh {
					wantReports = 2
				}
				if len(snapshot.Reports) != wantReports || len(snapshot.UnavailableAccounts) != 2-wantReports {
					t.Fatalf("reports=%d unavailable=%+v; want %d reports", len(snapshot.Reports), snapshot.UnavailableAccounts, wantReports)
				}
				for _, report := range snapshot.Reports {
					if len(report.Limits) == 0 {
						t.Fatal("missing parsed quota")
					}
					if providerID == "openai-codex" && report.Metadata["planType"] != "plus" {
						t.Fatalf("unknown plan: %+v", report.Metadata)
					}
				}
				for _, key := range []string{"one", "two"} {
					credential, err := store.ReadAccount(context.Background(), providerID, &key)
					if err != nil || credential == nil {
						t.Fatal("quota failure retired account", err)
					}
					wantAccess := "obsolete"
					if scenario.status == http.StatusUnauthorized {
						wantAccess = "fresh-" + key
					}
					if credential.Access != wantAccess || credential.Refresh != ai.RemoteRefreshSentinel {
						t.Fatal("refresh lost scope or remote ownership")
					}
				}
				wantRefreshes := int32(0)
				if scenario.status == http.StatusUnauthorized {
					wantRefreshes = 2
				}
				if got := refreshes.Load(); got != wantRefreshes {
					t.Fatalf("refreshes=%d, want %d (one per rejected account)", got, wantRefreshes)
				}
				if got := transport.fresh.Load(); got != wantRefreshes {
					t.Fatalf("refreshed quota requests=%d, want %d", got, wantRefreshes)
				}
			})
		}
	}
}
