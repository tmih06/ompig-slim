package codingagent

import (
	"context"
	"errors"
	"path/filepath"
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
