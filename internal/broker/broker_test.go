package broker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	_ "github.com/MichaelKinsy/PiG/ai/all"
	"github.com/MichaelKinsy/PiG/coding"
)

// testClients creates two independent local machines and a storage-only host;
// every fixture uses temporary profiles and no real provider credentials.
func testClients(t *testing.T) (*Client, *Client, *httptest.Server) {
	t.Helper()
	handler, err := NewHandler(t.TempDir(), "sync-secret")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := func() *Client {
		return &Client{base: server.URL, token: "sync-secret", dir: t.TempDir(), http: server.Client()}
	}
	return client(), client(), server
}

// openStore gives tests real SQLite credentials while preserving the user's DB.
func openStore(t *testing.T, dir string) *ai.SQLiteAuthStorage {
	t.Helper()
	s, err := ai.OpenCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mustWrite keeps fixture setup failures visible rather than masking sync bugs.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestStorageSecurity pins auth, path allowlisting, CAS and the absence of model
// proxy routes. The server cannot be used as an inference gateway.
func TestStorageSecurity(t *testing.T) {
	c, _, server := testClients(t)
	for _, path := range []string{"/v1/items/settings.json", "/v1/messages", "/v1/auth/lease"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal(response.StatusCode)
		}
	}
	var item Item
	if err := c.request(context.Background(), "GET", "/v1/items/settings.json", "", "", nil, &item); err != nil {
		t.Fatal(err)
	}
	if err := c.request(context.Background(), "PUT", "/v1/items/settings.json", "", item.Revision, Item{Data: []byte(`{}`)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.request(context.Background(), "PUT", "/v1/items/settings.json", "", item.Revision, Item{Data: []byte(`{"stale":true}`)}, nil); err == nil {
		t.Fatal("accepted stale overwrite")
	}
	for _, path := range []string{"/v1/messages", "/v1/items/agent.db", "/v1/items/secrets"} {
		if err := c.request(context.Background(), "GET", path, "", "", nil, nil); err == nil {
			t.Fatalf("unexpected route %s", path)
		}
	}
	if err := c.request(context.Background(), "PUT", "/v1/items/credentials.json", "", hashData(nil), Item{Data: []byte(`[]`)}, nil); err == nil {
		t.Fatal("auth update without lease")
	}
}

// TestPreferencesSync proves original custom provider URLs, roles and keybinds
// synchronize both ways, including deletions, without replacing model APIs.
func TestPreferencesSync(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	settings := `{"modelRoles":{"smol":"custom/my-model"}}`
	models := `{"providers":{"custom":{"api":"openai-completions","baseUrl":"https://provider.example/v1","apiKey":"provider-secret"}}}`
	mustWrite(t, filepath.Join(a.dir, "settings.json"), settings)
	mustWrite(t, filepath.Join(a.dir, "models.json"), models)
	mustWrite(t, filepath.Join(a.dir, "keybindings.json"), `{"app.interrupt":"ctrl+c"}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range SharedFiles {
		first, _ := os.ReadFile(filepath.Join(a.dir, name))
		second, _ := os.ReadFile(filepath.Join(b.dir, name))
		if string(first) != string(second) {
			t.Fatalf("different %s", name)
		}
	}
	changed := `{"modelRoles":{"smol":"custom/new-model"}}`
	mustWrite(t, filepath.Join(b.dir, "settings.json"), changed)
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(a.dir, "settings.json"))
	if string(data) != changed {
		t.Fatal("client edit not synchronized")
	}
	os.Remove(filepath.Join(b.dir, "keybindings.json"))
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "keybindings.json")); !os.IsNotExist(err) {
		t.Fatal("deletion not propagated")
	}
}

// TestConfigConflict preserves both independently edited copies and protects
// existing preferences with a private backup on first remote adoption.
func TestConfigConflict(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"base":true}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(b.dir, "settings.json"), `{"original":true}`)
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(b.statePath("settings.json.backup"))
	if err != nil || string(backup) != `{"original":true}` {
		t.Fatal("missing first-connect backup")
	}
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"a":true}`)
	mustWrite(t, filepath.Join(b.dir, "settings.json"), `{"b":true}`)
	if err = a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err = b.SyncFiles(ctx); err == nil {
		t.Fatal("lost conflicting edit")
	}
	data, _ := os.ReadFile(filepath.Join(b.dir, "settings.json"))
	if string(data) != `{"b":true}` {
		t.Fatal("overwrote conflict")
	}
}

// TestCredentialsRoundTrip exercises logical account snapshots, selected-account
// portability and logout tombstones without transferring SQLite caches/leases.
func TestCredentialsRoundTrip(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	sa, sb := openStore(t, a.dir), openStore(t, b.dir)
	first, second := "one", "two"
	for _, key := range []*string{&first, &second} {
		if _, err := sa.SaveAccount(ctx, "fixture", key, ai.Credential{Type: ai.CredentialOAuth, Access: *key, Refresh: "refresh-" + *key, Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	noop := func(context.Context) error { return nil }
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.syncCredentials(ctx, sb, noop); err != nil {
		t.Fatal(err)
	}
	account, err := sb.Read(ctx, "fixture")
	if err != nil || account == nil || account.Access != "two" {
		t.Fatalf("wrong selected account: %+v %v", account, err)
	}
	err = b.syncCredentials(ctx, sb, func(ctx context.Context) error { _, err := sb.DeleteAccount(ctx, "fixture", "two"); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err = a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	accounts, _ := sa.ListAccounts(ctx)
	if len(accounts) != 1 || *accounts[0].AccountKey != "one" {
		t.Fatal("logout resurrected")
	}
}

// TestImportPreservesUnrelatedState pins the narrow import boundaries: local
// rows the snapshot omits are tombstoned once, quota blocks survive, and a
// repeated import leaves updated_at untouched instead of churning observers.
func TestImportPreservesUnrelatedState(t *testing.T) {
	a, _, _ := testClients(t)
	ctx := context.Background()
	sa := openStore(t, a.dir)
	first := "one"
	if _, err := sa.SaveAccount(ctx, "fixture", &first, ai.Credential{Type: ai.CredentialAPIKey, Key: "key-one"}); err != nil {
		t.Fatal(err)
	}
	if err := sa.BlockAccount(ctx, "fixture", &first, "", time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := sa.ExportCredentials(ctx)
	// An account another machine removed must retire, not resurrect.
	if _, err := sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "local-only"}); err != nil {
		t.Fatal(err)
	}
	if err := sa.ImportCredentials(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if value, err := sa.ReadAccount(ctx, "fixture", nil); err != nil || value != nil {
		t.Fatal("removed account survived import", err)
	}
	blocked, err := sa.GetAccountBlockedUntil(ctx, "fixture", &first, []string{""})
	if err != nil || blocked == nil {
		t.Fatal("local quota block lost", err)
	}
	// An unchanged import must be a no-op. Row timestamps are unexported, so the
	// observable contract is that re-importing the state it just produced yields
	// the same export again.
	baseline, err := sa.ExportCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = sa.ImportCredentials(ctx, baseline); err != nil {
		t.Fatal(err)
	}
	again, err := sa.ExportCredentials(ctx)
	if err != nil || string(again) != string(baseline) {
		t.Fatal("unchanged import rewrote stored state", err)
	}
}

// TestConcurrentRefresh serializes credential mutation only. Each client must
// see the previous client's rotation before executing its own local callback.
func TestConcurrentRefresh(t *testing.T) {
	a, b, _ := testClients(t)
	sa, sb := openStore(t, a.dir), openStore(t, b.dir)
	ctx := context.Background()
	if _, err := sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "initial", Refresh: "r0"}); err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context) error { return nil }
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.syncCredentials(ctx, sb, noop); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, pair := range []struct {
		client *Client
		store  *ai.SQLiteAuthStorage
	}{{a, sa}, {b, sb}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- pair.client.syncCredentials(ctx, pair.store, func(ctx context.Context) error {
				_, err := pair.store.Modify(ctx, "fixture", func(current *ai.Credential) (*ai.Credential, error) {
					time.Sleep(100 * time.Millisecond)
					copy := *current
					copy.Refresh += "-next"
					return &copy, nil
				})
				return err
			})
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	credential, _ := sa.Read(ctx, "fixture")
	if credential.Refresh != "r0-next-next" {
		t.Fatalf("stale refresh replay: %s", credential.Refresh)
	}
}

// TestInterruptedRotation keeps a locally committed token if publication fails,
// then recovers it without calling the single-use refresh callback again.
func TestInterruptedRotation(t *testing.T) {
	a, _, _ := testClients(t)
	s := openStore(t, a.dir)
	ctx := context.Background()
	noop := func(context.Context) error { return nil }
	s.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialOAuth, Access: "old", Refresh: "old-refresh"})
	if err := a.syncCredentials(ctx, s, noop); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between local SQLite commit and hosted publication.
	if _, err := s.Modify(ctx, "fixture", func(c *ai.Credential) (*ai.Credential, error) {
		copy := *c
		copy.Access = "rotated"
		copy.Refresh = "rotated-refresh"
		return &copy, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.syncCredentials(ctx, s, noop); err != nil {
		t.Fatal(err)
	}
	var remote Item
	if err := a.request(ctx, "GET", "/v1/items/credentials.json", "", "", nil, &remote); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remote.Data), "rotated-refresh") {
		t.Fatal("lost local rotation")
	}
}

// TestDirectProviderTraffic proves synchronized configuration and credentials
// drive the normal client provider transport, with no broker inference hop.
func TestDirectProviderTraffic(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("lost synchronized auth")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"direct\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	a, b, _ := testClients(t)
	ctx := context.Background()
	config := map[string]any{"providers": map[string]any{"fixture": map[string]any{"api": "openai-completions", "baseUrl": upstream.URL, "models": []any{map[string]any{"id": "test", "contextWindow": 32000, "maxTokens": 1000}}}}}
	data, _ := json.Marshal(config)
	mustWrite(t, filepath.Join(a.dir, "models.json"), string(data))
	sa, sb := openStore(t, a.dir), openStore(t, b.dir)
	sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "provider-secret"})
	noop := func(context.Context) error { return nil }
	for _, c := range []*Client{a, b} {
		if err := c.SyncFiles(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.syncCredentials(ctx, sb, noop); err != nil {
		t.Fatal(err)
	}
	services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{AgentDir: b.dir, CWD: b.dir, ProjectTrusted: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	if e := services.Registry().GetError(); e != "" {
		t.Fatal(e)
	}
	model := services.ModelRuntime().GetModel("fixture", "test")
	if model == nil || model.ProviderMeta.API != ai.APIOpenAICompletions {
		t.Fatal("provider transport was replaced")
	}
	result := services.ModelRuntime().CompleteSimple(ctx, model, ai.Context{Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hello")}}})
	if result.StopReason != ai.StopReasonStop || calls != 1 {
		t.Fatalf("direct request failed: %+v calls=%d", result, calls)
	}
}

// TestInstalledCredentialHook proves ordinary runtime store writes publish
// synchronously, including direct login saves and logout, with no explicit sync.
func TestInstalledCredentialHook(t *testing.T) {
	a, b, server := testClients(t)
	previous := ai.CredentialStoreSync
	t.Cleanup(func() { ai.CredentialStoreSync = previous })
	t.Setenv("OMPIG_BROKER_URL", server.URL)
	t.Setenv("OMPIG_BROKER_TOKEN", "sync-secret")
	client, err := Connect(context.Background(), a.dir)
	if err != nil {
		t.Fatal(err)
	}
	s := openStore(t, client.dir)
	if _, err = s.SaveAccount(context.Background(), "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "hook-secret"}); err != nil {
		t.Fatal(err)
	}
	other := openStore(t, b.dir)
	if err = b.syncCredentials(context.Background(), other, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	value, err := other.Read(context.Background(), "fixture")
	if err != nil || value == nil || value.Key != "hook-secret" {
		t.Fatal("store write was not synchronized", err)
	}
	if err = s.Delete(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	if err = b.syncCredentials(context.Background(), other, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	value, err = other.Read(context.Background(), "fixture")
	if err != nil || value != nil {
		t.Fatal("logout was not synchronized", err)
	}
}

// TestCredentialConflict refuses to replay a callback when both machines have
// unshared credential changes; the local secret remains available for recovery.
func TestCredentialConflict(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	sa, sb := openStore(t, a.dir), openStore(t, b.dir)
	noop := func(context.Context) error { return nil }
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.syncCredentials(ctx, sb, noop); err != nil {
		t.Fatal(err)
	}
	sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "a-secret"})
	sb.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "b-secret"})
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	called := false
	err := b.syncCredentials(ctx, sb, func(context.Context) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("conflict replayed callback")
	}
	value, _ := sb.Read(ctx, "fixture")
	if value == nil || value.Key != "b-secret" {
		t.Fatal("lost conflicting local credential")
	}
}

// TestIdlePullDoesNotLease proves a periodic reconcile never occupies the
// credential lease: inference-only sessions must not block a peer's rotation.
func TestIdlePullDoesNotLease(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	sa, sb := openStore(t, a.dir), openStore(t, b.dir)
	noop := func(context.Context) error { return nil }
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.syncCredentials(ctx, sb, noop); err != nil {
		t.Fatal(err)
	}
	if _, err := sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "published"}); err != nil {
		t.Fatal(err)
	}
	if err := a.syncCredentials(ctx, sa, noop); err != nil {
		t.Fatal(err)
	}
	// Hold the lease the way a mid-refresh peer does, then reconcile b's idle pull.
	var lease struct {
		Lease string `json:"lease"`
	}
	if err := b.request(ctx, "POST", "/v1/auth/lease", "", "", nil, &lease); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.request(ctx, "DELETE", "/v1/auth/lease", lease.Lease, "", nil, nil); err != nil {
			t.Error(err)
		}
	}()
	if err := b.pullCredentials(ctx, sb); err != nil {
		t.Fatal(err)
	}
	value, err := sb.Read(ctx, "fixture")
	if err != nil || value == nil || value.Key != "published" {
		t.Fatal("idle pull missed published credential", err)
	}
}

// TestIdlePullPreservesUnsentLocalWrites keeps a pending rotation on disk until
// it can be published instead of letting a pull overwrite it.
func TestIdlePullPreservesUnsentLocalWrites(t *testing.T) {
	a, _, _ := testClients(t)
	ctx := context.Background()
	s := openStore(t, a.dir)
	if err := a.syncCredentials(ctx, s, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "local-pending"}); err != nil {
		t.Fatal(err)
	}
	if err := a.pullCredentials(ctx, s); err == nil {
		t.Fatal("pull ignored unsent local credentials")
	}
	value, _ := s.Read(ctx, "fixture")
	if value == nil || value.Key != "local-pending" {
		t.Fatal("pending local credential lost")
	}
}

// TestStartDegradesToLocal pins that a configured but unreachable service does
// not block CLI startup or the /login repair path.
func TestStartDegradesToLocal(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, connectionFile), `{"url":"http://127.0.0.1:1","secret":"offline-secret"}`)
	t.Setenv("OMPIG_BROKER_URL", "")
	t.Setenv("OMPIG_BROKER_TOKEN", "")
	var report strings.Builder
	finish, err := Start(context.Background(), dir, &report)
	if err != nil || finish != nil {
		t.Fatalf("unreachable broker blocked startup: %v", err)
	}
	if !strings.Contains(report.String(), "profile sync inactive") {
		t.Fatal("degradation was silent")
	}
	if !strings.Contains(report.String(), "127.0.0.1:1") {
		t.Fatal("degradation did not name the service")
	}
	if strings.Contains(report.String(), "offline-secret") {
		t.Fatal("degradation leaked the secret")
	}
}
