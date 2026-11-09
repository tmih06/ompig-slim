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

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestRoutedRolePreservesUpstreamOnWireAndResume exercises exact @upstream
// selection through the provider, metadata refresh, and persisted role audit.
func TestRoutedRolePreservesUpstreamOnWireAndResume(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	t.Setenv("OPENROUTER_API_KEY", "fixture-key")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Provider struct{ Only []string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Model != "route-fixture" || len(body.Provider.Only) != 1 || body.Provider.Only[0] != "fireworks" {
			t.Errorf("route lost on wire: %+v", body)
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"route\",\"object\":\"chat.completion.chunk\",\"created\":1,\"choices\":[{\"index\":0,\"delta\":{\"content\":\"routed\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"route\",\"object\":\"chat.completion.chunk\",\"created\":1,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"providers": map[string]any{"openrouter": map[string]any{"baseUrl": server.URL + "/v1", "api": "openai-completions", "apiKey": "fixture-key", "models": []any{map[string]any{"id": "route-fixture", "contextWindow": 128000, "maxTokens": 1024}}}}})
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	settings := Settings{Config: modelroles.Config{ModelRoles: map[string]string{"review": "openrouter/route-fixture@fireworks:off"}}}
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir, SettingsManager: NewInMemorySettingsManager(settings)})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	model, err := BuildModel("@review", services)
	if err != nil {
		t.Fatal(err)
	}
	if ai.ModelSelector(model) != "openrouter/route-fixture@fireworks" {
		t.Fatal(ai.ModelSelector(model))
	}
	s, err := NewSession(services, SessionOptions{Model: model, ModelRole: "review", NoSession: true, NoTools: "all", ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Send(context.Background(), "test routed role"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("request count", calls.Load())
	}
	selection := GetBranchSelection(s.inner.GetContextBranch(), services.ModelRuntime().GetModel)
	if selection == nil || selection.ModelID != "route-fixture@fireworks" {
		t.Fatalf("route lost after response: %+v", selection)
	}
	if role := branchModelRole(s.inner.GetContextBranch(), s.Model()); role != "review" {
		t.Fatal(role)
	}
}
