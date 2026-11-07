package ai_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// A 401 must be observed before the native API turns it into an empty error
// event. The normal OnResponse hook runs only for accepted responses, so merely
// composing that hook silently rotates past a still-refreshable account.
func TestNativeHTTP401RefreshesSameAccountAndPersistsRotation(t *testing.T) {
	var mu sync.Mutex
	var attempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		key := r.Header.Get("Authorization")
		mu.Lock()
		attempts = append(attempts, key)
		mu.Unlock()
		if key == "Bearer rejected-a" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"invalidated oauth token"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"refresh-ok\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	store, err := ai.OpenSQLiteAuthStorage(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, account := range []struct{ key, token string }{{"b", "healthy-b"}, {"a", "rejected-a"}} {
		key := account.key
		_, err = store.SaveAccount(ctx, "refresh-fixture", &key, ai.Credential{Type: ai.CredentialOAuth, AccountID: key, Access: account.token, Refresh: "refresh-" + key, Expires: time.Now().Add(time.Hour).UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
	}
	var refreshes, acceptedResponses atomic.Int32
	model := &ai.Model{ID: "fixture-model", ProviderMeta: ai.ProviderMetadata{ProviderID: "refresh-fixture", API: ai.APIOpenAICompletions, BaseURL: server.URL + "/v1"}, Capabilities: ai.ModelCapabilities{ContextWindow: 8192, MaxOutputTokens: 1024}}
	provider := ai.CreateProvider(ai.CreateProviderOptions{ID: "refresh-fixture", BaseURL: server.URL + "/v1", Models: []ai.AnyModel{model}, API: ai.OpenAICompletionsAPI(), Auth: ai.ProviderAuth{OAuth: &ai.OAuthAuth{Refresh: func(ctx context.Context, credential ai.Credential) (ai.Credential, error) {
		refreshes.Add(1)
		credential.Access = "fresh-a"
		credential.Refresh = "rotated-a"
		return credential, nil
	}, ToAuth: func(credential ai.Credential) (ai.ModelAuth, error) {
		return ai.ModelAuth{APIKey: credential.Access}, nil
	}}}})
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: store})
	models.SetProvider(provider)
	noRetries := 0
	stream := models.StreamSimple(ctx, model, ai.Context{Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi"), Timestamp: 1}}}, ai.StreamOptions{MaxRetries: &noRetries, OnResponse: func(_ context.Context, response ai.ProviderResponse, _ *ai.Model) error {
		if response.Status != 200 {
			return fmt.Errorf("public hook received rejected status %d", response.Status)
		}
		acceptedResponses.Add(1)
		return nil
	}})
	for range stream.Events(ctx) {
	}
	result, err := stream.ResultContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != ai.StopReasonStop {
		t.Fatalf("stream: %+v", result)
	}
	mu.Lock()
	actual := append([]string(nil), attempts...)
	mu.Unlock()
	if refreshes.Load() != 1 || len(actual) != 2 || actual[0] != "Bearer rejected-a" || actual[1] != "Bearer fresh-a" {
		t.Fatalf("refreshes=%d attempts=%v", refreshes.Load(), actual)
	}
	if acceptedResponses.Load() != 1 {
		t.Fatalf("accepted response callbacks=%d", acceptedResponses.Load())
	}
	key := "a"
	saved, err := store.ReadAccount(ctx, "refresh-fixture", &key)
	if err != nil || saved == nil || saved.Access != "fresh-a" || saved.Refresh != "rotated-a" {
		t.Fatalf("persisted=%+v err=%v", saved, err)
	}
}
