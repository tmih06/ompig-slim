package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestOMPCodexFreeTokenTransports proves the free-tier fix reaches actual SSE
// requests and WebSocket handshakes, not only login storage. Two email-only
// credentials in one session must not reuse each other's authenticated socket.
func TestOMPCodexFreeTokenTransports(t *testing.T) {
	for _, transport := range []Transport{TransportSSE, TransportWebSocketCached} {
		t.Run(string(transport), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			session := "omp-codex-free-" + string(transport)
			t.Cleanup(func() { CloseOpenAICodexWebSocketSessions(session) })
			var mu sync.Mutex
			attempts := []string{}
			completed := `{"type":"response.completed","response":{"id":"fixture-response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/codex/responses" || r.Header.Get("originator") != "codex_cli_rs" {
					t.Errorf("invalid Codex endpoint/identity: %s %q", r.URL.Path, r.Header.Get("originator"))
				}
				if _, present := r.Header["Chatgpt-Account-Id"]; present {
					t.Error("workspace-less account sent account header")
				}
				mu.Lock()
				attempts = append(attempts, r.Header.Get("Authorization"))
				mu.Unlock()
				if websocket.IsWebSocketUpgrade(r) {
					conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
					for {
						if _, _, err := conn.ReadMessage(); err != nil {
							return
						}
						if err := conn.WriteMessage(websocket.TextMessage, []byte(completed)); err != nil {
							return
						}
					}
				}
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\n", completed)
			}))
			defer server.Close()
			tokens := []string{
				codexLoginJWT(`{"https://api.openai.com/profile":{"email":"first@example.invalid"}}`),
				codexLoginJWT(`{"https://api.openai.com/profile":{"email":"second@example.invalid"}}`),
			}
			for _, token := range tokens {
				provider := NewOpenAICodexResponsesProvider(OpenAICodexResponsesConfig{APIKey: token, Model: "fixture-model", BaseURL: server.URL, ProviderID: "openai-codex"})
				t.Cleanup(func() { _ = provider.Close() })
				stream, err := provider.Stream(ctx, NormalizeContext(Context{Messages: []Message{UserMessage{Content: UserText("hi"), Timestamp: 1}}}), StreamOptions{Transport: transport, SessionID: session, MaxRetries: new(0)})
				if err != nil {
					t.Fatalf("free token rejected locally: %v", err)
				}
				for range stream.Events(ctx) {
				}
				result, err := stream.ResultContext(ctx)
				if err != nil || result.StopReason != StopReasonStop {
					t.Fatalf("free account request failed: %+v %v", result, err)
				}
			}
			mu.Lock()
			actual := append([]string(nil), attempts...)
			mu.Unlock()
			want := []string{"Bearer " + tokens[0], "Bearer " + tokens[1]}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("requests crossed account identities: %v", actual)
			}
			// Raw bearer material must never become the cache's routing key.
			codexWebSocketSessions.mu.Lock()
			for key := range codexWebSocketSessions.sessions[session] {
				if strings.Contains(key, tokens[0]) || strings.Contains(key, tokens[1]) {
					t.Error("cache key contains raw bearer")
				}
			}
			codexWebSocketSessions.mu.Unlock()
		})
	}
}
