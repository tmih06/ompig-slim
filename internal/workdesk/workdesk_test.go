package workdesk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/MichaelKinsy/PiG/ai/all"
	"ompig-slim/internal/workdesk"
)

// fixture isolates credentials and serves the real provider protocol without paid calls.
func fixture(t *testing.T, serve http.HandlerFunc) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PIG_HOME", home)
	t.Setenv("PIG_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	t.Setenv("OMPIG_SLIM_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	t.Setenv("PI_OFFLINE", "1")
	if err := os.MkdirAll(filepath.Join(home, "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(serve)
	t.Cleanup(server.Close)
	raw, _ := json.Marshal(map[string]any{"providers": map[string]any{"desk-fixture": map[string]any{
		"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key",
		"models": []any{map[string]any{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}},
	}}})
	if err := os.WriteFile(filepath.Join(home, "agent", "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return t.TempDir(), t.TempDir()
}

// answer emits an OpenAI SSE final answer for the production provider parser.
func answer(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	delta, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": nil}}})
	fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", delta)
}

// TestRunResumeAndStatus observes one durable assignment through the public command seam.
func TestRunAcceptsControlFileWhileActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PIG_HOME", home)
	t.Setenv("PIG_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	t.Setenv("OMPIG_SLIM_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	t.Setenv("PI_OFFLINE", "1")
	if err := os.MkdirAll(filepath.Join(home, "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-release
		}
		answer(w, "handled")
	}))
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"providers": map[string]any{"desk-fixture": map[string]any{
		"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key",
		"models": []any{map[string]any{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}},
	}}})
	if err := os.WriteFile(filepath.Join(home, "agent", "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	state, cwd := t.TempDir(), t.TempDir()
	control := filepath.Join(state, "control.jsonl")
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "task-1", "--control-file", control, "do the work"}
	result := make(chan error, 1)
	go func() {
		var out bytes.Buffer
		result <- workdesk.Run(context.Background(), args, &out)
	}()
	<-firstStarted
	if err := os.WriteFile(control, []byte(`{"content":"also inspect the failing test"}
`), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("durable run did not finish")
	}
	if calls.Load() < 2 {
		t.Fatalf("provider calls = %d, control input was not executed", calls.Load())
	}
}

func TestRunResumeAndStatus(t *testing.T) {
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); answer(w, "Implemented and verified.") })
	eventsPath := filepath.Join(state, "events.jsonl")
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "ticket-1", "--events-file", eventsPath, "Fix the fixture"}
	for _, command := range [][]string{args, {"resume", "--state", state}, {"status", "--state", state}, args} {
		var out bytes.Buffer
		if err := workdesk.Run(context.Background(), command, &out); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Status string `json:"status"`
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%s: %v", out.String(), err)
		}
		if result.Status != "done" || !strings.Contains(result.Answer, "Implemented") {
			t.Fatalf("result: %s", out.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate execution: %d requests", calls.Load())
	}
	events, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), `"type":"text"`) {
		t.Fatalf("missing structured worker event: %s", events)
	}
}
