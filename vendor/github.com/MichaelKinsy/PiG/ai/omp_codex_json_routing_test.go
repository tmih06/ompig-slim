package ai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestCodexJSONNativeFallback runs the real auth store, account router and Codex
// SSE transport. A rejected access-only account is scrubbed after the forced
// refresh and the next account completes the request using its account header.
func TestCodexJSONNativeFallback(t *testing.T) {
	var mu sync.Mutex
	var attempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		token := r.Header.Get("Authorization")
		mu.Lock()
		attempts = append(attempts, token)
		mu.Unlock()
		if r.URL.Path != "/codex/responses" {
			t.Errorf("incorrect Codex path: %s", r.URL.Path)
		}
		if token == "Bearer rejected-fixture" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"message":"invalidated oauth token"}}`)
			return
		}
		if token != "Bearer healthy-fixture" || r.Header.Get("ChatGPT-Account-Id") != "b" {
			t.Errorf("incorrect imported auth headers: %s %s", token, r.Header.Get("ChatGPT-Account-Id"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"fixture-response\",\"status\":\"in_progress\"}}\n\n"+
			"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"fixture-message\",\"role\":\"assistant\",\"content\":[]}}\n\n"+
			"data: {\"type\":\"response.content_part.added\",\"output_index\":0,\"content_index\":0,\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n"+
			"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture-response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
	}))
	defer server.Close()
	store, err := ai.OpenSQLiteAuthStorage(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, account := range []struct{ key, token string }{{"b", "healthy-fixture"}, {"a", "rejected-fixture"}} {
		key := account.key
		if _, err := store.SaveAccount(t.Context(), "codex-json", &key, ai.Credential{Type: ai.CredentialOAuth, Access: account.token, AccountID: key, Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := ai.BuiltinProviderAuth("codex-json")
	if err != nil {
		t.Fatal(err)
	}
	model := &ai.Model{ID: "fixture-model", ProviderMeta: ai.ProviderMetadata{ProviderID: "codex-json", API: ai.APIOpenAICodexResponses, BaseURL: server.URL}, Capabilities: ai.ModelCapabilities{ContextWindow: 8192, MaxOutputTokens: 1024}}
	provider := ai.CreateProvider(ai.CreateProviderOptions{ID: "codex-json", BaseURL: server.URL, Models: []ai.AnyModel{model}, API: ai.OpenAICodexResponsesAPI(), Auth: auth})
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: store})
	models.SetProvider(provider)
	noRetries := 0
	stream := models.StreamSimple(t.Context(), model, ai.Context{Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi"), Timestamp: 1}}}, ai.StreamOptions{Transport: ai.TransportSSE, MaxRetries: &noRetries})
	text, result := collectStream(t, stream)
	if text != "hello" || result.StopReason != ai.StopReasonStop {
		t.Fatalf("fallback stream failed: %q %+v", text, result)
	}
	mu.Lock()
	actual := append([]string(nil), attempts...)
	mu.Unlock()
	if !reflect.DeepEqual(actual, []string{"Bearer rejected-fixture", "Bearer healthy-fixture"}) {
		t.Fatalf("unexpected attempts: %v", actual)
	}
	dead := "a"
	saved, err := store.ReadAccount(t.Context(), "codex-json", &dead)
	if err != nil || saved != nil {
		t.Fatalf("failed account still enabled: %+v %v", saved, err)
	}
	remaining, err := store.ListAccounts(t.Context())
	if err != nil || len(remaining) != 1 || *remaining[0].AccountKey != "b" {
		t.Fatalf("sibling account changed: %+v %v", remaining, err)
	}
}

// TestCodexJSONNeverReplaysAfterOutput verifies that importing accounts does not
// bypass the router's replay barrier after any content has been emitted.
func TestCodexJSONNeverReplaysAfterOutput(t *testing.T) {
	store := newFakeAccountStore(accountInfo("codex-json", keyOf("a"), true), accountInfo("codex-json", keyOf("b"), false))
	for _, key := range []string{"a", "b"} {
		store.put("codex-json", keyOf(key), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("plus")})
	}
	router := ai.NewAccountRouter(store)
	attempts := 0
	text, result := collectStream(t, router.StreamRoutes(t.Context(), okModel("codex-json", "fixture-model"), ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
		attempts++
		observe(ai.ProviderResponse{Status: 401})
		return partialErrorStream("invalidated oauth token"), nil
	}))
	if text != "half" || result.StopReason != ai.StopReasonError || attempts != 1 {
		t.Fatalf("request replayed after output: %q attempts=%d %+v", text, attempts, result)
	}
}
