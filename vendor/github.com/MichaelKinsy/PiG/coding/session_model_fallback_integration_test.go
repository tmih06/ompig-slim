package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestRoleFallbackStreamsAcrossProviders runs the actual Session/Agent/provider
// recovery loop against local SSE endpoints. It proves configuration is not just
// menu state: quota errors walk in order, the successful response reaches Send,
// and a later prompt restores the primary after its advertised reset.
func TestRoleFallbackStreamsAcrossProviders(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	var mu sync.Mutex
	var requested []string
	primaryHealthy := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		requested = append(requested, body.Model)
		healthy := primaryHealthy
		mu.Unlock()
		if body.Model != "third" && !(body.Model == "primary" && healthy) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"message":"billing quota exceeded; retry-after: 1 seconds","type":"insufficient_quota"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"fallback worked\"},\"finish_reason\":null}]}\n\n", body.Model)
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	cwd, dir := t.TempDir(), t.TempDir()
	config := map[string]any{"providers": map[string]any{}}
	providers := config["providers"].(map[string]any)
	for _, entry := range []struct{ provider, model string }{{"fallback-a", "primary"}, {"fallback-b", "second"}, {"fallback-c", "third"}} {
		providers[entry.provider] = map[string]any{"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key", "models": []any{map[string]any{"id": entry.model, "name": entry.model, "contextWindow": 128000, "maxTokens": 1024}}}
	}
	raw, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	settings := Settings{Config: modelroles.Config{ModelRoles: map[string]string{"review": "fallback-a/primary"}}, Retry: &icodingagent.RetrySettingsJSON{MaxRetries: new(0), FallbackChains: map[string][]string{"review": {"fallback-b/second:off", "fallback-c/third:off"}}}}
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: cwd, AgentDir: dir, SettingsManager: NewInMemorySettingsManager(settings)})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	model, err := BuildModel("@review", services)
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(services, SessionOptions{Model: model, ModelRole: "review", ResourceLoader: NoResources, NoTools: "all", NoSession: true})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	now := time.Now()
	session.fallbackNow = func() time.Time { return now }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = session.Send(ctx, "use the configured role chain"); err != nil {
		t.Fatal(err)
	}
	if session.Model().ID != "third" || session.ActiveModelRole() != "review" {
		t.Fatal(session.Model().ID, session.ActiveModelRole())
	}
	mu.Lock()
	first := append([]string{}, requested...)
	primaryHealthy = true
	mu.Unlock()
	if !reflect.DeepEqual(first, []string{"primary", "second", "third"}) {
		t.Fatal("request order", first)
	}
	now = now.Add(2 * time.Second)
	if _, err = session.Send(ctx, "the primary recovered"); err != nil {
		t.Fatal(err)
	}
	if session.Model().ID != "primary" {
		t.Fatal("primary was not restored", session.Model().ID)
	}
}
