package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// TestCustomThinkingEffortsOnWire verifies cold-start reference inheritance all
// the way through session clamping and OpenAI-compatible serialization. A tier
// appearing in the selector is insufficient: max must not become high on wire,
// off must omit effort, and an explicit endpoint opt-out must stay authoritative.
func TestCustomThinkingEffortsOnWire(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	const id = "deepseek-ai/deepseek-v4.1-flash-1017"
	for _, test := range []struct {
		level   ai.ModelThinkingLevel
		enabled bool
	}{
		{ai.ThinkingOff, true},
		{ai.ThinkingLow, true},
		{ai.ThinkingHigh, true},
		{ai.ThinkingMax, true},
		{ai.ThinkingHigh, false},
	} {
		t.Run(fmt.Sprintf("%s/effort-enabled=%t", test.level, test.enabled), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model  string
					Effort *string `json:"reasoning_effort"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Model != id || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("custom endpoint, model identity or auth was not preserved")
				}
				if test.enabled && test.level != ai.ThinkingOff {
					if body.Effort == nil || *body.Effort != string(test.level) {
						t.Errorf("wire effort = %v; want %s", body.Effort, test.level)
					}
				} else if body.Effort != nil {
					t.Errorf("disabled/off effort emitted %q", *body.Effort)
				}
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			dir := t.TempDir()
			raw := fmt.Sprintf(`{"providers":{"deploy":{"api":"openai-completions","baseUrl":%q,"apiKey":"fixture","models":[{"id":%q,"reasoning":true,"contextWindow":1000000,"maxTokens":32768,"compat":{"supportsDeveloperRole":false,"supportsReasoningEffort":%t,"maxTokensField":"max_tokens"}}]}}}`, server.URL+"/v1", id, test.enabled)
			if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer services.Close()
			if services.Registry().ModelMetadata().Generation() != 0 {
				t.Fatal("fixture must start without fetched metadata")
			}
			model, err := BuildModel("deploy/"+id, services)
			if err != nil {
				t.Fatal(err)
			}
			session, err := NewSession(services, SessionOptions{Model: model, ThinkingLevel: test.level, NoSession: true, NoTools: "all", ResourceLoader: NoResources})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			if session.ThinkingLevel() != test.level {
				t.Fatalf("selected effort %s clamped to %s", test.level, session.ThinkingLevel())
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := session.Send(ctx, "local effort fixture"); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatalf("inference calls = %d; want one local request", calls.Load())
			}
		})
	}
}
