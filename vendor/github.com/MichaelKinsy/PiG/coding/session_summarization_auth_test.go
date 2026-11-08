package coding

import (
	"context"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/compaction"
)

// TestNewSessionSummarizationUsesRuntimeAuthRouting ensures a Session using
// the built-in stream leaves summary authentication to ModelRuntime, where
// account eligibility and fallback routing are applied just as they are for
// ordinary turns.
func TestNewSessionSummarizationUsesRuntimeAuthRouting(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	cwd, agentDir := t.TempDir(), t.TempDir()
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir,
		SettingsManager: icodingagent.NewInMemorySettingsManager(Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()

	session, err := NewSession(services, SessionOptions{
		NoSession: true, NoTools: "all", ResourceLoader: NoResources,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if !session.runtimeStream {
		t.Fatal("a Session with its built-in Agent must use runtime auth for summaries")
	}
	var model *ai.Model
	for _, candidate := range services.Registry().GetAll() {
		if candidate.ProviderMeta.ProviderID == "openai-codex" && candidate.ID == "gpt-6.1-sol" {
			model = candidate
			break
		}
	}
	if model == nil {
		t.Fatal("openai-codex/gpt-6.1-sol is missing from the model catalog")
	}

	request, err := session.prepareSummarizationRequest(context.Background(), model)
	if err != nil {
		t.Fatalf("prepare summary request: %v", err)
	}
	if request.apiKey != "" || len(request.headers) != 0 || len(request.env) != 0 {
		t.Fatalf("summary request pre-resolved credentials: apiKey=%q headers=%v env=%v", request.apiKey, request.headers, request.env)
	}
	completer, ok := request.completer.(compaction.ProviderCompleter)
	if !ok || completer.StreamFn == nil {
		t.Fatal("summary request must use the Session's runtime-backed stream")
	}
}
