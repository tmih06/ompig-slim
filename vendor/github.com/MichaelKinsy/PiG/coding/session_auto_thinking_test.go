package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestAutoThinkingUsesJudgeWithoutChangingPreference exercises real streamed judge
// output with a local fixture. The provider sees concrete low, never the auto
// sentinel, and classification usage never enters the conversation context.
func TestAutoThinkingUsesJudgeWithoutChangingPreference(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if request.Model != "judge" {
			t.Errorf("unexpected judge %q", request.Model)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"judge\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"judge\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"low\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"judge\",\"object\":\"chat.completion.chunk\",\"created\":1,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"providers": map[string]any{"auto-fixture": map[string]any{"api": "openai-completions", "apiKey": "fixture-key", "baseUrl": server.URL + "/v1", "models": []any{map[string]any{"id": "target", "reasoning": true, "contextWindow": 128000, "maxTokens": 4096}, map[string]any{"id": "judge", "contextWindow": 128000, "maxTokens": 1024}}}}})
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	sm := NewInMemorySettingsManager(Settings{Config: modelroles.Config{ModelRoles: map[string]string{"default": "auto-fixture/target:auto", "judge": "auto-fixture/judge"}}})
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir, SettingsManager: sm})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	model, err := BuildModel("auto-fixture/target", services)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(services, SessionOptions{Model: model, ModelRole: "default", ThinkingLevel: "auto", NoSession: true, NoTools: "all", ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.ConfiguredThinkingLevel() != "auto" {
		t.Fatal("auto not retained")
	}
	if judges := s.autoThinkingJudges(); len(judges) == 0 {
		t.Fatalf("no judge candidates: available=%v", services.ModelRuntime().GetAvailableSnapshot())
	}
	s.applyAutoThinking(context.Background(), "rename the specified local constant")
	if s.ThinkingLevel() != ai.ThinkingLow || s.ConfiguredThinkingLevel() != "auto" {
		t.Fatal("judge result", s.ThinkingLevel(), s.ConfiguredThinkingLevel())
	}
	if sm.Get().ModelRoles["default"] != "auto-fixture/target:auto" {
		t.Fatal("classification overwrote role")
	}
	if err = s.SetThinkingLevel(ai.ThinkingHigh); err != nil {
		t.Fatal(err)
	}
	if s.ConfiguredThinkingLevel() == "auto" {
		t.Fatal("manual pin did not disable auto")
	}
}

// TestAutoThinkingSingleTierNeedsNoJudge covers sparse supported effort ladders.
func TestAutoThinkingSingleTierNeedsNoJudge(t *testing.T) {
	model := &ai.Model{ProviderMeta: ai.ProviderMetadata{Reasoning: true}, Capabilities: ai.ModelCapabilities{MaxThinking: "high"}, ThinkingLevelMap: ai.ThinkingLevelMap{ai.ThinkingHigh: new("high"), ai.ThinkingLow: nil, ai.ThinkingMedium: nil, ai.ThinkingMinimal: nil}}
	levels := autoThinkingLevels(model, false)
	if len(levels) != 1 || levels[0] != ai.ThinkingHigh {
		t.Fatal(levels)
	}
}
