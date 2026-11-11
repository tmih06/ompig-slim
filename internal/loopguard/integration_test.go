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

	_ "github.com/MichaelKinsy/PiG/ai/all" // Exercise the production SSE provider path.
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

// TestSessionSteersBeforeNextRequest runs real read calls through a Session and
// local SSE provider. It proves the hidden boundary draft reaches both the next
// model request and the durable session log, even with extensions disabled.
func TestSessionSteersBeforeNextRequest(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	cwd, dir := t.TempDir(), t.TempDir()
	path := filepath.Join(cwd, "fixture.txt")
	if err := os.WriteFile(path, []byte("same result\n"), 0600); err != nil {
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
		if n <= threshold {
			args, _ := json.Marshal(map[string]string{"path": path})
			delta := map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": fmt.Sprintf("call_%d", n), "type": "function",
				"function": map[string]any{"name": "read", "arguments": string(args)},
			}}}
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
		Model: model, ResourceLoader: loader, AllowedTools: map[string]struct{}{"read": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := session.Send(ctx, "read the fixture repeatedly"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != threshold+1 {
		t.Fatalf("got %d requests, want %d", len(requests), threshold+1)
	}
	for i, request := range requests {
		if got := strings.Contains(request, "tool_call_loop_detected"); got != (i == threshold) {
			t.Fatalf("request %d: steer present=%v", i+1, got)
		}
	}
	file := session.Inner().GetSessionFile()
	if file == nil {
		t.Fatal("no durable session file")
	}
	log, err := os.ReadFile(*file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), `"customType":"tool-call-loop-redirect"`) || !strings.Contains(string(log), `"display":false`) {
		t.Fatal("missing hidden durable steer")
	}
}
