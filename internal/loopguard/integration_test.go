package loopguard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/MichaelKinsy/PiG/ai/all"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/extension"
	_ "github.com/MichaelKinsy/PiG/coding/extension/builtin/codemode"
)

type fixtureCall struct {
	name string
	args string
}

func runSessionFixture(t *testing.T, turns [][]fixtureCall) ([]string, string, string) {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	cwd, dir := t.TempDir(), t.TempDir()
	path := filepath.Join(cwd, "fixture.txt")
	if err := os.WriteFile(path, []byte("same result\nsame result\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, string(body))
		n := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if n <= len(turns) {
			var calls []any
			for i, call := range turns[n-1] {
				calls = append(calls, map[string]any{
					"index": i, "id": fmt.Sprintf("call_%d_%d", n, i), "type": "function",
					"function": map[string]any{"name": call.name, "arguments": call.args},
				})
			}
			delta := map[string]any{"role": "assistant", "tool_calls": calls}
			raw, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
			fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", raw)
			return
		}
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Changed approach.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	config := map[string]any{"providers": map[string]any{"loop-fixture": map[string]any{
		"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key",
		"models": []any{map[string]any{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}},
	}}}
	raw, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	settings := coding.NewInMemorySettingsManager(coding.Settings{})
	services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	model, err := coding.BuildModel("loop-fixture/fixture", services)
	if err != nil {
		t.Fatal(err)
	}
	loader := coding.NewDefaultResourceLoader(coding.DefaultResourceLoaderOptions{
		CWD: cwd, AgentDir: dir, SettingsManager: settings,
		NoExtensions: true, NoContextFiles: true, NoSkills: true, NoThemes: true, NoPromptTemplates: true,
		ExtensionFactories: []extension.InlineExtension{extension.NamedInlineExtension{
			Name: "tool-call-loop-guard", Factory: Extension, Hidden: true,
		}},
	})
	if err := loader.Reload(); err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	session, err := coding.NewSession(services, coding.SessionOptions{
		Model: model, ResourceLoader: loader,
		AllowedTools: map[string]struct{}{"read": {}, "edit": {}, "write": {}, "grep": {}, "bash": {}, "codemode": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := session.Send(ctx, "work on fixture.txt"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(turns)+1 {
		t.Fatalf("got %d requests, want %d", len(requests), len(turns)+1)
	}
	file := session.Inner().GetSessionFile()
	if file == nil {
		t.Fatal("no durable session file")
	}
	log, err := os.ReadFile(*file)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return requests, string(log), string(content)
}

func TestSessionSteersBeforeNextRequest(t *testing.T) {
	turns := make([][]fixtureCall, threshold)
	for i := range turns {
		turns[i] = []fixtureCall{{"read", `{"path":"fixture.txt"}`}}
	}
	requests, log, _ := runSessionFixture(t, turns)
	for i, request := range requests {
		if got := strings.Contains(request, "tool_call_loop_detected"); got != (i == threshold) {
			t.Fatalf("request %d: steer present=%v", i+1, got)
		}
	}
	if !strings.Contains(log, `"customType":"tool-call-loop-redirect"`) || !strings.Contains(log, `"display":false`) {
		t.Fatal("missing hidden durable steer")
	}
}

func TestSessionDeterministicFailureRecovery(t *testing.T) {
	var turns [][]fixtureCall
	for i := 0; i < 3; i++ {
		turns = append(turns, []fixtureCall{{"edit", fmt.Sprintf(`{"path":"fixture.txt","edits":[{"oldText":"absent %d","newText":"replacement"}]}`, i)}})
	}
	turns = append(turns,
		[]fixtureCall{{"read", `{"path":"fixture.txt"}`}},
		[]fixtureCall{{"edit", `{"path":"fixture.txt","edits":[{"oldText":"same result\nsame result\n","newText":"recovered\n"}]}`}},
	)
	requests, log, content := runSessionFixture(t, turns)
	for i, request := range requests {
		if got := strings.Contains(request, "deterministic_tool_failure"); got != (i >= 3) {
			t.Fatalf("request %d: deterministic steer present=%v, want %v", i+1, got, i >= 3)
		}
	}
	if content != "recovered\n" {
		t.Fatalf("fresh read did not recover: %q", content)
	}
	if strings.Count(log, `"failureClass":"edit-unmatched"`) != 1 || !strings.Contains(log, `"display":false`) {
		t.Fatal("missing single hidden durable failure diagnostic")
	}
}

func TestSessionLegitimateProgress(t *testing.T) {
	for _, test := range []struct {
		name  string
		turns [][]fixtureCall
	}{
		{"polling", [][]fixtureCall{
			{{"bash", `{"command":"printf pending"}`}},
			{{"bash", `{"command":"printf pending"}`}},
			{{"bash", `{"command":"printf pending"}`}},
			{{"bash", `{"command":"printf ready"}`}},
		}},
		{"red test and fix", [][]fixtureCall{
			{{"bash", `{"command":"grep -q recovered fixture.txt"}`}},
			{{"bash", `{"command":"grep -q recovered fixture.txt"}`}},
			{{"bash", `{"command":"grep -q recovered fixture.txt"}`}},
			{{"edit", `{"path":"fixture.txt","edits":[{"oldText":"same result\nsame result\n","newText":"recovered\n"}]}`}},
			{{"bash", `{"command":"grep -q recovered fixture.txt"}`}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests, log, _ := runSessionFixture(t, test.turns)
			for _, request := range requests {
				if strings.Contains(request, "deterministic_tool_failure") {
					t.Fatal("legitimate progress received a deterministic-error steer")
				}
			}
			if strings.Contains(log, `"customType":"tool-call-loop-redirect"`) {
				t.Fatal("legitimate progress persisted a steer")
			}
			var failures int
			var lastError bool
			for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
				var entry struct {
					Message struct {
						Role    string `json:"role"`
						IsError bool   `json:"isError"`
					} `json:"message"`
				}
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry.Message.Role == "toolResult" {
					lastError = entry.Message.IsError
					if lastError {
						failures++
					}
				}
			}
			wantFailures := 0
			if test.name == "red test and fix" {
				wantFailures = 3
			}
			if failures != wantFailures || lastError {
				t.Fatalf("got %d failures and last result error=%v, want %d failures and successful recovery", failures, lastError, wantFailures)
			}
		})
	}
}
