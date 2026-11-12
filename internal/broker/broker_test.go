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
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	_ "github.com/MichaelKinsy/PiG/ai/all"
	"github.com/MichaelKinsy/PiG/coding"
)

// testClients creates two independent client profiles and a vault host;
// every fixture uses temporary profiles and no real provider credentials.
func testClients(t testing.TB) (*Client, *Client, *httptest.Server) {
	t.Helper()
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	handler, err := NewHandler(t.TempDir(), "sync-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handler.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := func() *Client {
		return &Client{base: server.URL, token: "sync-secret", dir: t.TempDir(), http: server.Client()}
	}
	return client(), client(), server
}

// openStore gives tests real SQLite credentials while preserving the user's DB.
func openStore(t testing.TB, dir string) *ai.SQLiteAuthStorage {
	t.Helper()
	s, err := ai.OpenLocalCredentialStore(dir)
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
	for _, path := range []string{"/v1/items/settings.json", "/v1/messages"} {
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
	if err := c.request(context.Background(), "GET", "/v1/items/settings.json", "", nil, &item); err != nil {
		t.Fatal(err)
	}
	if err := c.request(context.Background(), "PUT", "/v1/items/settings.json", item.Revision, Item{Data: []byte(`{}`)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.request(context.Background(), "PUT", "/v1/items/settings.json", item.Revision, Item{Data: []byte(`{"stale":true}`)}, nil); err == nil {
		t.Fatal("accepted stale overwrite")
	}
	for _, path := range []string{"/v1/messages", "/v1/items/agent.db", "/v1/items/secrets"} {
		if err := c.request(context.Background(), "GET", path, "", nil, nil); err == nil {
			t.Fatalf("unexpected route %s", path)
		}
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

// TestUnchangedPreferencesKeepAncestry pins idle reconciliation's no-write path
// for both present documents and tombstones; initial sync must still persist all
// ancestors so later edits and deletions retain three-way conflict detection.
func TestUnchangedPreferencesKeepAncestry(t *testing.T) {
	client, _, _ := testClients(t)
	ctx := context.Background()
	mustWrite(t, filepath.Join(client.dir, "settings.json"), `{"base":true}`)
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	before := make(map[string]os.FileInfo, len(SharedFiles))
	for _, name := range SharedFiles {
		info, err := os.Stat(client.statePath(name))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = info
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range SharedFiles {
		after, err := os.Stat(client.statePath(name))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before[name], after) || !before[name].ModTime().Equal(after.ModTime()) {
			t.Errorf("unchanged %s ancestry rewritten", name)
		}
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
	sa := &remoteStore{client: a}
	sa.SaveAccount(ctx, "fixture", nil, ai.Credential{Type: ai.CredentialAPIKey, Key: "provider-secret"})
	for _, c := range []*Client{a, b} {
		if err := c.SyncFiles(ctx); err != nil {
			t.Fatal(err)
		}
	}
	installRemoteStore(b)
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

// TestStartKeepsCredentialsRemote pins that a configured but unreachable broker
// leaves the CLI usable without silently opening the old local credential vault.
func TestStartKeepsCredentialsRemote(t *testing.T) {
	dir := t.TempDir()
	previous := ai.CredentialStoreFactory
	t.Cleanup(func() { ai.CredentialStoreFactory = previous })
	mustWrite(t, filepath.Join(dir, connectionFile), `{"url":"http://127.0.0.1:1","secret":"offline-secret"}`)
	t.Setenv("OMPIG_BROKER_URL", "")
	t.Setenv("OMPIG_BROKER_TOKEN", "")
	var report strings.Builder
	finish, err := Start(context.Background(), dir, &report)
	if err != nil || finish == nil {
		t.Fatalf("unreachable broker must leave startup usable and sync retrying: %v", err)
	}
	defer func() {
		if err := finish(); err != nil {
			t.Error("never-connected broker failed repair-command exit", err)
		}
	}()
	if !strings.Contains(report.String(), "profile sync pending (will retry)") {
		t.Fatal("degradation was silent")
	}
	if !strings.Contains(report.String(), "127.0.0.1:1") {
		t.Fatal("degradation did not name the service")
	}
	if strings.Contains(report.String(), "offline-secret") {
		t.Fatal("degradation leaked the secret")
	}
	store, err := ai.OpenCredentialStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Read(context.Background(), "fixture"); err == nil {
		t.Fatal("unavailable broker fell back to local credentials")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.db")); !os.IsNotExist(err) {
		t.Fatal("remote profile opened a local vault", err)
	}
}

// TestNewHandlerTightensDataDirectory covers the deployed layout: an image or an
// operator may create the directory with a wider mode, and the service must not
// serve plaintext credentials out of a world-readable path.
func TestNewHandlerTightensDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Mkdir is masked by the umask, so assert from a known wide mode.
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(dir, "secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handler.Close() })
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Fatalf("data directory mode %o, want 700", perm)
	}
}
