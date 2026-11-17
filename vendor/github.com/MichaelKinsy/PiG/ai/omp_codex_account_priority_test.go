package ai_test

import (
	"encoding/json"
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

func TestCodexModelRoutingUsesStoredFreePlan(t *testing.T) {
	ctx := t.Context()
	free := ai.Credential{
		Type: ai.CredentialOAuth, Access: codexToken(""), AccountID: "free",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
		Extra:   map[string]json.RawMessage{"orgName": json.RawMessage(`"free"`)},
	}
	plus := ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("plus"), AccountID: "plus", Expires: free.Expires}
	var mu sync.Mutex
	var attempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		account := r.Header.Get("ChatGPT-Account-Id")
		if r.URL.Path != "/codex/responses" {
			t.Errorf("incorrect Codex path: %s", r.URL.Path)
		}
		credential := free
		if account == "plus" {
			credential = plus
		} else if account != "free" {
			t.Errorf("unexpected request account: %q", account)
		}
		if r.Header.Get("Authorization") != "Bearer "+credential.Access {
			t.Error("request bearer did not match routed account")
		}
		mu.Lock()
		attempts = append(attempts, account)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture-response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
	}))
	defer server.Close()
	store, err := ai.OpenSQLiteAuthStorage(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, credential := range []ai.Credential{free, plus} {
		if _, err := store.SaveAccount(ctx, "openai-codex", new(credential.AccountID), credential); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := ai.BuiltinProviderAuth("openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	router := ai.NewAccountRouter(store)
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: store, AccountRouter: router})
	defer models.Close()
	model := &ai.Model{
		ID:           "gpt-6-luna",
		ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex", API: ai.APIOpenAICodexResponses, BaseURL: server.URL},
		Capabilities: ai.ModelCapabilities{ContextWindow: 8192, MaxOutputTokens: 1024},
	}
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{ID: "openai-codex", BaseURL: server.URL, Models: []ai.AnyModel{model}, API: ai.OpenAICodexResponsesAPI(), Auth: auth}))
	request := ai.AccountRouteRequest{SessionID: "stored-plan-session"}
	if err := store.SetSessionAccount(ctx, "openai-codex", ai.AccountSessionRoute{SessionID: request.SessionID, ModelID: model.ID}, new("plus")); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ model, account string }{
		{"gpt-6-luna", "free"}, {"gpt-6.1-sol", "plus"}, {"gpt-6-luna", "free"},
	} {
		selected := new(*model)
		selected.ID = step.model
		key, present, err := router.ResolveAccount(ctx, selected, request)
		if err != nil || !present || key == nil || *key != step.account {
			t.Fatalf("account preview for %s did not choose %s: %v", step.model, step.account, err)
		}
		stream := models.StreamSimple(ctx, selected, ai.Context{Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi"), Timestamp: 1}}}, ai.StreamOptions{SessionID: request.SessionID, Transport: ai.TransportSSE, MaxRetries: new(0)})
		_, result := collectStream(t, stream)
		if result == nil || result.StopReason != ai.StopReasonStop {
			t.Fatalf("request for %s failed: %+v", step.model, result)
		}
		key, present = router.PeekRoutedAccountKey("openai-codex", request.SessionID, selected.ID)
		if !present || key == nil || *key != step.account {
			t.Fatalf("usage identity for %s did not follow %s", step.model, step.account)
		}
	}
	mu.Lock()
	actual := append([]string(nil), attempts...)
	mu.Unlock()
	if !reflect.DeepEqual(actual, []string{"free", "plus", "free"}) {
		t.Fatalf("model switches used incorrect accounts: %v", actual)
	}
	selected, err := store.Read(ctx, "openai-codex")
	if err != nil || selected == nil || selected.AccountID != "plus" {
		t.Fatal("request routing changed the provider's persistent selection")
	}
}
