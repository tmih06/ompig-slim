package coding

import (
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

	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelmetadata"
)

// TestDiscoveredModelStreamsAndRestartsOffline exercises a model ID absent from
// the executable end-to-end: shared source -> persisted snapshot -> role/model
// resolution -> native HTTP inference -> a new process-equivalent services
// instance with the source unavailable. Only inference receives credentials.
func TestDiscoveredModelStreamsAndRestartsOffline(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	const id = "future-wire-model-not-bundled"
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("API key sent to public source")
		}
		fmt.Fprintf(w, `{"openai":{"models":{%q:{"name":"Future Wire Model","tool_call":true,"modalities":{"input":["text"]},"limit":{"context":128000,"output":4096},"cost":{"input":1,"output":2}}}}}`, id)
	}))
	defer source.Close()
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer future-fixture-key" {
			t.Error("inference missing provider credentials", r.Header.Get("Authorization"))
		}
		var body struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != id {
			t.Error("wrong future ID on wire", body.Model)
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"future-message","type":"message","role":"assistant","content":[]}}

data: {"type":"response.output_text.delta","output_index":0,"delta":"future response"}

data: {"type":"response.completed","response":{"id":"future","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}

`)
	}))
	defer inference.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"providers": map[string]any{"openai": map[string]any{"baseUrl": inference.URL + "/v1", "apiKey": "future-fixture-key"}}})
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir, SettingsManager: NewInMemorySettingsManager(Settings{Retry: &icodingagent.RetrySettingsJSON{MaxRetries: new(0)}})})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	service := modelmetadata.New(modelmetadata.Options{Store: modelmetadata.NewFileStore(filepath.Join(dir, modelmetadata.FileName)), URL: source.URL})
	services.Registry().SetModelMetadata(service)
	if result, err := services.Registry().RefreshModelMetadata(context.Background(), true, true); err != nil || result.Error != "" {
		t.Fatal(result.Error, err)
	}
	model, err := BuildModel("openai/"+id, services)
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(services, SessionOptions{Model: model, NoSession: true, NoTools: "all", ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	messages, err := session.Send(context.Background(), "test future model")
	if err != nil {
		t.Fatal(err)
	}
	rawResult, _ := json.Marshal(messages)
	if calls.Load() != 1 || !strings.Contains(string(rawResult), "future response") {
		t.Fatal("inference result", calls.Load(), string(rawResult))
	}
	source.Close()
	restored, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err = BuildModel("openai/"+id, restored); err != nil {
		t.Fatal("cached future model unavailable offline", err)
	}
}
