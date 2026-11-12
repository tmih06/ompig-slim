package codingagent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// usageRemoteStore models broker-owned refresh behind the registry's live view.
// Its inherited ModifyAccount cannot refresh, as the client owns no refresh token.
type usageRemoteStore struct{ *usageTestStore }

// RefreshOAuth updates only the requested account and keeps the refresh redacted.
func (store *usageRemoteStore) RefreshOAuth(ctx context.Context, provider string, key *string, explicit bool, expected ai.Credential) (*ai.Credential, error) {
	if !explicit || expected.Refresh != ai.RemoteRefreshSentinel {
		return nil, fmt.Errorf("usage refresh lost account scope or remote ownership")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for index, candidate := range store.keys[provider] {
		if candidate != usageAccountKeyText(key) {
			continue
		}
		credential := store.entries[provider][index]
		credential.Access = "fresh-" + candidate
		credential.Expires = time.Now().Add(time.Hour).UnixMilli()
		store.entries[provider][index] = credential
		return &credential, nil
	}
	return nil, fmt.Errorf("usage refreshed an unknown account")
}

// TestUsageRefreshesRemoteAccountsThroughRegistry guards expired broker accounts
// appearing unavailable when the session registry hides the owner's capabilities.
// Real quota parsing and exact-account auth resolution must both succeed.
func TestUsageRefreshesRemoteAccountsThroughRegistry(t *testing.T) {
	const providerID = ai.GoogleAntigravityProviderID
	store := &usageRemoteStore{newUsageTestStore()}
	for _, key := range []string{"one", "two"} {
		store.add(providerID, &key, ai.Credential{Type: ai.CredentialOAuth, Access: "expired", Refresh: ai.RemoteRefreshSentinel, Expires: 1, ProjectID: "fixture-project"})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer fresh-one" && token != "Bearer fresh-two" {
			t.Errorf("quota received stale or unscoped access token")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.75}]}]}`)
	}))
	defer server.Close()
	registry := NewModelRegistryWithModelsPath("")
	registry.SetCredentialStore(store)
	var providers []*ai.UsageProvider
	for _, provider := range ai.DefaultUsageProviders() {
		if provider.ID == providerID {
			providers = append(providers, provider)
		}
	}
	service := NewUsageService(UsageServiceOptions{Credentials: store, Accounts: store, Providers: providers, GetAuth: func(ctx context.Context, id string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error) {
		return registry.ResolveRegistryProviderAuth(ctx, id, overrides)
	}, BaseURLFor: func(string) string { return server.URL }, HTTPClient: server.Client()})
	snapshot, err := service.FetchProviderSnapshot(context.Background(), providerID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.UnavailableAccounts) != 0 || len(snapshot.Reports) != 2 {
		t.Fatalf("remote accounts must report usage after owner refresh: reports=%d, unavailable=%+v", len(snapshot.Reports), snapshot.UnavailableAccounts)
	}
	for _, report := range snapshot.Reports {
		if len(report.Limits) == 0 || report.Limits[0].Amount.RemainingFraction == nil || *report.Limits[0].Amount.RemainingFraction != 0.75 {
			t.Fatalf("quota fixture was not parsed: %+v", report)
		}
	}
}
