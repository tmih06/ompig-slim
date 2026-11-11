package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// fixtureOAuth reuses the public provider shape, but never calls a real endpoint.
// The injected refresh runs only on the host and can assert its raw token input.
type fixtureOAuth struct {
	ai.CodexOAuthProvider
	refresh func(context.Context, ai.OAuthCredentials) (ai.OAuthCredentials, error)
}

// ID isolates the fixture from all real provider registrations.
func (fixtureOAuth) ID() string { return "broker-fixture" }

// RefreshTokenContext invokes the test-owned exchange with the vault credential.
func (p fixtureOAuth) RefreshTokenContext(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
	return p.refresh(ctx, c)
}

// RefreshToken supports the registry's non-context entry without real traffic.
func (p fixtureOAuth) RefreshToken(c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
	return p.refresh(context.Background(), c)
}

// registerRefresh scopes a fake provider's lifetime to one nonparallel test.
func registerRefresh(t *testing.T, refresh func(context.Context, ai.OAuthCredentials) (ai.OAuthCredentials, error)) {
	t.Helper()
	ai.RegisterOAuthProvider("broker-fixture", fixtureOAuth{refresh: refresh})
	t.Cleanup(func() { ai.UnregisterOAuthProvider("broker-fixture") })
}

// TestVaultAccountOperations pins live remote credentials, selection, shared
// quota state and logout without a local SQLite credential mirror.
func TestVaultAccountOperations(t *testing.T) {
	a, b, server := testClients(t)
	sa, sb := &remoteStore{client: a}, &remoteStore{client: b}
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, err := sa.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialOAuth, Access: key, Refresh: "private-" + key, Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	credential, err := sb.Read(ctx, "fixture")
	if err != nil || credential == nil || credential.Access != "two" || credential.Refresh != remoteRefreshSentinel {
		t.Fatalf("wrong redacted credential: %+v %v", credential, err)
	}
	if _, err = sb.SelectAccount(ctx, "fixture", "one"); err != nil {
		t.Fatal(err)
	}
	credential, err = sa.Read(ctx, "fixture")
	if err != nil || credential == nil || credential.Access != "one" {
		t.Fatal("selection not shared", err)
	}
	key := "one"
	until := time.Now().Add(time.Hour).UnixMilli()
	if err = sa.BlockAccount(ctx, "fixture", &key, "chat", until); err != nil {
		t.Fatal(err)
	}
	if err = sb.BlockAccount(ctx, "fixture", &key, "chat", until-1000); err != nil {
		t.Fatal(err)
	}
	blocked, err := sb.GetAccountBlockedUntil(ctx, "fixture", &key, []string{"chat"})
	if err != nil || blocked == nil || *blocked != until {
		t.Fatal("quota block not shared or shortened", err)
	}
	if err = sb.ClearAccountBlocks(ctx, "fixture", &key, []string{"chat"}); err != nil {
		t.Fatal(err)
	}
	blocked, err = sa.GetAccountBlockedUntil(ctx, "fixture", &key, []string{"chat"})
	if err != nil || blocked != nil {
		t.Fatal("quota block not cleared", err)
	}
	if _, err = sb.DeleteAccount(ctx, "fixture", "one"); err != nil {
		t.Fatal(err)
	}
	if err = sb.Delete(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if count, err := sa.AccountCount(); err != nil || count != 0 {
		t.Fatal("logout not shared", count, err)
	}
	for _, path := range []string{"/v1/items/agent.db"} {
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer sync-secret")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatalf("private database route %s remains: %d", path, response.StatusCode)
		}
	}
	for _, client := range []*Client{a, b} {
		if _, err := os.Stat(filepath.Join(client.dir, "agent.db")); !os.IsNotExist(err) {
			t.Fatal("remote access created local vault", err)
		}
	}
}

// TestBulkAccountReadPreservesSelectionAndCredentials pins the broker's single-pass account
// export: every enabled account remains visible, the selected flag is preserved, and the host
// still holds the real refresh token while the HTTP snapshot redacts it.
func TestBulkAccountReadPreservesSelectionAndCredentials(t *testing.T) {
	client, _, server := testClients(t)
	host := server.Config.Handler.(*Store)
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, err := host.vault.SaveAccount(ctx, "fixture", &key, ai.Credential{Type: ai.CredentialOAuth, Access: key, Refresh: "private-" + key}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := host.vault.SelectAccount(ctx, "fixture", "one"); err != nil {
		t.Fatal(err)
	}
	accounts, err := host.vault.ReadAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("bulk account read: %d accounts, %v", len(accounts), err)
	}
	for _, account := range accounts {
		if account.Credential.Refresh != "private-"+*account.Info.AccountKey {
			t.Fatalf("bulk read lost host credential: %+v", account)
		}
		if account.Info.Active != (*account.Info.AccountKey == "one") {
			t.Fatalf("bulk read lost selection: %+v", account.Info)
		}
	}
	entries, err := (&remoteStore{client: client}).snapshot(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("remote snapshot: %d entries, %v", len(entries), err)
	}
	for _, entry := range entries {
		if entry.Credential.Refresh != remoteRefreshSentinel {
			t.Fatalf("snapshot leaked host refresh: %+v", entry.Credential)
		}
	}
}

// TestVaultRefreshCoalesces proves the host spends a refresh token once for
// concurrent requests that observed the same access token, and redacts replies.
func TestVaultRefreshCoalesces(t *testing.T) {
	a, b, server := testClients(t)
	sa, sb := &remoteStore{client: a}, &remoteStore{client: b}
	ctx := context.Background()
	var calls atomic.Int32
	registerRefresh(t, func(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
		calls.Add(1)
		if c.Refresh != "private-refresh" {
			return c, fmt.Errorf("stale or sentinel refresh")
		}
		time.Sleep(30 * time.Millisecond)
		c.Refresh = "rotated-private"
		c.Access = "fresh-access"
		c.Expires = time.Now().Add(time.Hour).UnixMilli()
		return c, nil
	})
	if _, err := sa.SaveAccount(ctx, "broker-fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "expired-access", Refresh: "private-refresh", Expires: 1}); err != nil {
		t.Fatal(err)
	}
	observed, err := sb.Read(ctx, "broker-fixture")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, store := range []*remoteStore{sa, sb} {
		wg.Go(func() {
			credential, err := store.RefreshOAuth(ctx, "broker-fixture", nil, true, *observed)
			if err != nil || credential == nil || credential.Access != "fresh-access" || credential.Refresh != remoteRefreshSentinel {
				t.Errorf("refresh: %+v %v", credential, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("spent refresh %d times", calls.Load())
	}
	host := server.Config.Handler.(*Store)
	raw, err := host.vault.ReadAccount(ctx, "broker-fixture", nil)
	if err != nil || raw == nil || raw.Refresh != "rotated-private" {
		t.Fatal("rotation not persisted on host", err)
	}
}

// TestRuntimeRefreshUsesBroker exercises the actual resolver through a runtime
// overlay. The local provider callback must never receive the remote sentinel.
func TestRuntimeRefreshUsesBroker(t *testing.T) {
	a, _, _ := testClients(t)
	store := &remoteStore{client: a}
	ctx := context.Background()
	registerRefresh(t, func(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
		if c.Refresh != "only-on-host" {
			return c, errors.New("refresh escaped host")
		}
		c.Access = "resolved-on-host"
		c.Refresh = "rotated-on-host"
		c.Expires = time.Now().Add(time.Hour).UnixMilli()
		return c, nil
	})
	if _, err := store.SaveAccount(ctx, "broker-fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Refresh: "only-on-host", Access: "old", Expires: 1}); err != nil {
		t.Fatal(err)
	}
	auth := ai.ProviderAuth{OAuth: &ai.OAuthAuth{
		Refresh: func(context.Context, ai.Credential) (ai.Credential, error) {
			t.Error("client refreshed OAuth")
			return ai.Credential{}, errors.New("must use broker")
		},
		ToAuth: func(c ai.Credential) (ai.ModelAuth, error) { return ai.ModelAuth{APIKey: c.Access}, nil },
	}}
	result, err := ai.ResolveProviderAuth(ctx, "broker-fixture", auth, ai.NewRuntimeCredentials(store), ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil || result == nil || result.Auth.APIKey != "resolved-on-host" {
		t.Fatalf("resolver failed: %+v %v", result, err)
	}
}

// TestVaultBackgroundRefresh pins proactive refresh and failure isolation: a
// transient error neither removes secrets nor blocks another account's refresh.
func TestVaultBackgroundRefresh(t *testing.T) {
	a, _, server := testClients(t)
	store := &remoteStore{client: a}
	ctx := context.Background()
	registerRefresh(t, func(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
		if c.Access == "failing" {
			return c, errors.New("temporary private-provider-error")
		}
		c.Access = "proactively-refreshed"
		c.Refresh = "rotated"
		c.Expires = time.Now().Add(time.Hour).UnixMilli()
		return c, nil
	})
	for _, key := range []string{"failing", "working"} {
		if _, err := store.SaveAccount(ctx, "broker-fixture", &key, ai.Credential{Type: ai.CredentialOAuth, Access: key, Refresh: "private", Expires: time.Now().Add(time.Minute).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	host := server.Config.Handler.(*Store)
	if err := host.sweep(ctx); err == nil || strings.Contains(err.Error(), "private-provider-error") {
		t.Fatal("failure not sanitized", err)
	}
	failed, err := host.vault.ReadAccount(ctx, "broker-fixture", new("failing"))
	if err != nil || failed == nil || failed.Refresh != "private" {
		t.Fatal("transient failure destroyed credential", err)
	}
	credential, err := store.ReadAccount(ctx, "broker-fixture", new("working"))
	if err != nil || credential == nil || credential.Access != "proactively-refreshed" {
		t.Fatal("one failure prevented refresh", err)
	}
}

// TestRefreshSurvivesDisconnect proves that a lost HTTP response cannot discard
// the provider's already-rotated token, and retrying reuses the persisted result.
func TestRefreshSurvivesDisconnect(t *testing.T) {
	a, _, server := testClients(t)
	store := &remoteStore{client: a}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	registerRefresh(t, func(ctx context.Context, c ai.OAuthCredentials) (ai.OAuthCredentials, error) {
		calls.Add(1)
		close(started)
		<-release
		c.Refresh = "durable-private"
		c.Access = "durable-access"
		c.Expires = time.Now().Add(time.Hour).UnixMilli()
		return c, nil
	})
	ctx := context.Background()
	if _, err := store.SaveAccount(ctx, "broker-fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Refresh: "old-private", Access: "old", Expires: 1}); err != nil {
		t.Fatal(err)
	}
	expected, err := store.Read(ctx, "broker-fixture")
	if err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := store.RefreshOAuth(requestCtx, "broker-fixture", nil, true, *expected); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("exchange did not start")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled request succeeded")
	}
	close(release)
	// A retry queues behind the original vault mutation; no polling or token
	// replay is needed to recover the lost HTTP response.
	result, err := store.RefreshOAuth(ctx, "broker-fixture", nil, true, *expected)
	if err != nil || result == nil || result.Access != "durable-access" || calls.Load() != 1 {
		t.Fatalf("lost rotation/replayed refresh: %+v %v calls=%d", result, err, calls.Load())
	}
	raw, err := server.Config.Handler.(*Store).vault.ReadAccount(ctx, "broker-fixture", nil)
	if err != nil || raw == nil || raw.Refresh != "durable-private" {
		t.Fatal("lost persisted refresh", err)
	}
}

// TestLocalMigrationIsExplicit proves connection never uploads local secrets;
// an explicit repeatable migration cannot overwrite rotations or revive logout.
func TestLocalMigrationIsExplicit(t *testing.T) {
	a, _, server := testClients(t)
	ctx := context.Background()
	local := openStore(t, a.dir)
	if _, err := local.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "local", Refresh: "local-private"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvURL, server.URL)
	t.Setenv(EnvToken, "sync-secret")
	if _, err := Connect(ctx, a.dir); err != nil {
		t.Fatal(err)
	}
	remote := &remoteStore{client: a}
	if count, err := remote.AccountCount(); err != nil || count != 0 {
		t.Fatal("implicit upload", count, err)
	}
	var stdout, stderr strings.Builder
	if code := runMigrate(ctx, []string{"--from-local"}, a.dir, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if _, err := remote.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "newer", Refresh: "remote-private"}); err != nil {
		t.Fatal(err)
	}
	if code := runMigrate(ctx, []string{"--from-local"}, a.dir, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	credential, err := remote.Read(ctx, "fixture")
	if err != nil || credential == nil || credential.Access != "newer" {
		t.Fatal("migration overwrote rotation", err)
	}
	if err := remote.Delete(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if code := runMigrate(ctx, []string{"--from-local"}, a.dir, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if count, err := remote.AccountCount(); err != nil || count != 0 {
		t.Fatal("migration revived logout", count, err)
	}
}

// TestRemoteCASAndRedaction protects concurrent login and opaque credential
// marshaling: stale callbacks are not replayed, and no old refresh escapes.
func TestRemoteCASAndRedaction(t *testing.T) {
	a, b, server := testClients(t)
	sa, sb := &remoteStore{client: a}, &remoteStore{client: b}
	ctx := context.Background()
	var credential ai.Credential
	if err := json.Unmarshal([]byte(`{"type":"oauth","access":"one","refresh":"opaque-private","expires":1,"extraField":"retained"}`), &credential); err != nil {
		t.Fatal(err)
	}
	if _, err := sa.SaveAccount(ctx, "fixture", nil, credential); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", server.URL+"/v1/snapshot", nil)
	req.Header.Set("Authorization", "Bearer sync-secret")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || strings.Contains(string(data), "opaque-private") || !strings.Contains(string(data), "retained") {
		t.Fatal("snapshot leaked/discarded fields", string(data), err)
	}
	calls := 0
	_, err = sa.Modify(ctx, "fixture", func(current *ai.Credential) (*ai.Credential, error) {
		calls++
		if _, err := sb.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "newer-key"}); err != nil {
			return nil, err
		}
		return &ai.Credential{Type: ai.CredentialAPIKey, Key: "stale-key"}, nil
	})
	if err == nil || calls != 1 {
		t.Fatal("stale callback overwrote or replayed", err, calls)
	}
	current, err := sb.Read(ctx, "fixture")
	if err != nil || current == nil || current.Key != "newer-key" {
		t.Fatal("lost concurrent login", err)
	}
}

// TestSnapshotAuthorization checks vault APIs behind the same bearer gate and
// verifies client requests never follow a redirect carrying secrets elsewhere.
func TestSnapshotAuthorization(t *testing.T) {
	_, _, server := testClients(t)
	for _, path := range []string{"/v1/snapshot", "/v1/credential", "/v1/credential/refresh"} {
		response, err := http.Post(server.URL+path, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal(path, response.StatusCode)
		}
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed secret-bearing redirect") }))
	defer target.Close()
	redirect := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusTemporaryRedirect))
	defer redirect.Close()
	client, err := newClient(t.TempDir(), connection{URL: redirect.URL, Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&remoteStore{client: client}).snapshot(context.Background()); err == nil {
		t.Fatal("accepted redirect")
	}
}
