package codingagent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestUsageRetiresExpiredCodexJSONAccounts exercises usage through the real
// auth resolver and SQLite lifecycle, including two dead imports and a live
// sibling. Retired accounts must disappear in the same snapshot, not be probed
// with their old tokens or reported as unavailable.
func TestUsageRetiresExpiredCodexJSONAccounts(t *testing.T) {
	store, err := ai.OpenSQLiteAuthStorage(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, key := range []string{"expired-a", "expired-b", "live"} {
		expiry := time.Now().Add(-time.Hour)
		if key == "live" {
			expiry = time.Now().Add(time.Hour)
		}
		credential := ai.Credential{Type: ai.CredentialOAuth, Access: key, AccountID: key, Expires: expiry.UnixMilli()}
		if _, err := store.SaveAccount(t.Context(), "codex-json", new(key), credential); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := ai.BuiltinProviderAuth("codex-json")
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int64
	service := NewUsageService(UsageServiceOptions{
		Credentials: store,
		Accounts:    store,
		GetAuth: func(ctx context.Context, provider string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
			return ai.ResolveProviderAuth(ctx, provider, auth, store, ai.AuthContext{}, overrides)
		},
		Providers: []*ai.UsageProvider{{ID: "codex-json", FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
			probes.Add(1)
			if params.Credential.AccessToken != "live" {
				return nil, nil
			}
			return usageReportFor("codex-json"), nil
		}}},
	})
	for range 2 {
		snapshot, err := service.FetchSnapshot(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("snapshot: reports=%d unavailable=%d cumulative probes=%d", len(snapshot.Reports), len(snapshot.UnavailableAccounts), probes.Load())
		if len(snapshot.Reports) != 1 || len(snapshot.UnavailableAccounts) != 0 {
			t.Fatalf("expired imports remain in usage: reports=%d unavailable=%d", len(snapshot.Reports), len(snapshot.UnavailableAccounts))
		}
		accounts, err := store.ListAccounts(t.Context())
		if err != nil || len(accounts) != 1 || accounts[0].AccountKey == nil || *accounts[0].AccountKey != "live" {
			t.Fatalf("expected only the live account in auth storage: accounts=%+v err=%v", accounts, err)
		}
	}
	if probes.Load() != 2 {
		t.Fatalf("probed retired credentials: calls=%d, want 2 live probes", probes.Load())
	}
}

// usageCodexJSONTokenTransport serves fixture refresh responses without allowing
// any network access; calls counts exchanges through the real auth resolver.
type usageCodexJSONTokenTransport struct {
	status int
	body   string
	calls  atomic.Int64
}

// RoundTrip accepts only the fixed OAuth endpoint and never forwards credentials.
func (transport *usageCodexJSONTokenTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != "https://auth.openai.com/oauth/token" {
		return nil, errors.New("unexpected token endpoint")
	}
	transport.calls.Add(1)
	return &http.Response{StatusCode: transport.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(transport.body)), Request: request}, nil
}

// TestUsageRecoversRejectedCodexJSONBeforeExpiry covers revocation before the
// imported expiry: only a 401 triggers one scoped refresh. A definitive token
// rejection retires the account, a rotation is reprobed once, and transient or
// entitlement failures never delete credentials. All tokens are inert fixtures.
func TestUsageRecoversRejectedCodexJSONBeforeExpiry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		provider    string
		usageStatus int
		status      int
		body        string
		retired     bool
		recovered   bool
		refreshes   int64
	}{
		{"revoked", "codex-json", 401, 400, `{"error":"invalid_grant"}`, true, false, 1},
		{"transient", "codex-json", 401, 503, `{"error":"invalid_grant"}`, false, false, 1},
		{"unclassified", "codex-json", 401, 401, `{"error":"unknown"}`, false, false, 1},
		{"rotation rejected", "codex-json", 401, 200, `{"access_token":"fresh-fixture","refresh_token":"rotated-fixture","expires_in":3600}`, false, false, 1},
		{"rotation", "codex-json", 401, 200, `{"access_token":"fresh-fixture","refresh_token":"rotated-fixture","expires_in":3600}`, false, true, 1},
		{"forbidden", "codex-json", 403, 400, `{"error":"invalid_grant"}`, false, false, 0},
		{"server failure", "codex-json", 500, 400, `{"error":"invalid_grant"}`, false, false, 0},
		{"browser unchanged", "openai-codex", 401, 400, `{"error":"invalid_grant"}`, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &usageCodexJSONTokenTransport{status: tc.status, body: tc.body}
			previous := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: transport}
			t.Cleanup(func() { http.DefaultClient = previous })
			store, err := ai.OpenSQLiteAuthStorage(filepath.Join(t.TempDir(), "auth.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			key := new("rejected")
			credential := ai.Credential{Type: ai.CredentialOAuth, Access: "rejected-fixture", Refresh: "refresh-fixture", AccountID: "fixture-account", Expires: time.Now().Add(24 * time.Hour).UnixMilli()}
			if _, err := store.SaveAccount(t.Context(), tc.provider, key, credential); err != nil {
				t.Fatal(err)
			}
			auth, err := ai.BuiltinProviderAuth(tc.provider)
			if err != nil {
				t.Fatal(err)
			}
			var probes atomic.Int64
			service := NewUsageService(UsageServiceOptions{
				Credentials: store, Accounts: store,
				GetAuth: func(ctx context.Context, provider string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
					if !overrides.AccountKeySet || overrides.AccountKey == nil || *overrides.AccountKey != *key || !overrides.ForceOAuthRefresh {
						t.Error("rejection refresh lost account scope or force flag")
					}
					return ai.ResolveProviderAuth(ctx, provider, auth, store, ai.AuthContext{}, overrides)
				},
				Providers: []*ai.UsageProvider{{ID: tc.provider, FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
					probes.Add(1)
					if params.Credential.AccessToken == "fresh-fixture" && tc.recovered {
						return usageReportFor(tc.provider), nil
					}
					return nil, ai.NewUsageHTTPError("fixture rejection", tc.usageStatus)
				}}},
			})
			snapshot, err := service.FetchSnapshot(t.Context(), true)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := store.ReadAccount(t.Context(), tc.provider, key)
			if err != nil || (stored == nil) != tc.retired {
				t.Fatalf("retired=%t want=%t err=%v", stored == nil, tc.retired, err)
			}
			wantReports, wantUnavailable, wantProbes := 0, 1, int64(1)
			if tc.retired {
				wantUnavailable = 0
			}
			if tc.status == 200 {
				wantProbes = 2
			}
			if tc.recovered {
				wantReports, wantUnavailable = 1, 0
			}
			if len(snapshot.Reports) != wantReports || len(snapshot.UnavailableAccounts) != wantUnavailable || probes.Load() != wantProbes || transport.calls.Load() != tc.refreshes {
				t.Fatalf("reports=%d unavailable=%d probes=%d refreshes=%d; want %d/%d/%d/%d", len(snapshot.Reports), len(snapshot.UnavailableAccounts), probes.Load(), transport.calls.Load(), wantReports, wantUnavailable, wantProbes, tc.refreshes)
			}
		})
	}
}

// TestUsageRetainsCodexJSONAfterTransientRefreshFailure ensures an advisory
// refresh error alone does not hide or delete an account still in the store.
func TestUsageRetainsCodexJSONAfterTransientRefreshFailure(t *testing.T) {
	store := newUsageTestStore()
	store.add("codex-json", new("retry"), ai.Credential{
		Type: ai.CredentialOAuth, Access: "fixture", Refresh: "refresh-fixture",
		AccountID: "retry", Expires: time.Now().Add(-time.Hour).UnixMilli(),
	})
	var refreshes, probes atomic.Int64
	service := usageTestService(store, &ai.UsageProvider{ID: "codex-json", FetchUsage: func(_ context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
		probes.Add(1)
		if params.Credential.AccessToken != "fixture" {
			t.Error("retained credential was not used")
		}
		return nil, nil
	}})
	service.options.GetAuth = func(context.Context, string, ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
		refreshes.Add(1)
		return nil, errors.New("temporary refresh failure")
	}
	snapshot, err := service.FetchSnapshot(t.Context(), true)
	if err != nil || len(snapshot.Reports) != 0 || len(snapshot.UnavailableAccounts) != 1 {
		t.Fatalf("retained account must remain unavailable: snapshot=%+v err=%v", snapshot, err)
	}
	credential, err := store.ReadAccount(t.Context(), "codex-json", new("retry"))
	if err != nil || credential == nil || refreshes.Load() != 1 || probes.Load() != 1 {
		t.Fatalf("transient failure lost credential or skipped retry: retained=%t refreshes=%d probes=%d err=%v", credential != nil, refreshes.Load(), probes.Load(), err)
	}
}
