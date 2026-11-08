package cli

import (
	"context"
	"testing"

	_ "github.com/MichaelKinsy/PiG/ai/all"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// TestStartupModelRoles verifies role aliases and missing-primary credential fallback
// before a provider request exists. Fixture auth never leaves the process.
func TestStartupModelRoles(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "fixture-not-a-real-key")
	settings := codingagent.Settings{Config: modelroles.Config{ModelRoles: map[string]string{"default": "missing/no-model", "review": "openai/gpt-4o:off"}}, Retry: &codingagent.RetrySettingsJSON{FallbackChains: map[string][]string{"default": {"openai/gpt-4o-mini:off"}}}}
	services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: codingagent.NewInMemorySettingsManager(settings)})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	for _, tc := range []struct{ selector, role, model string }{{"", "default", "gpt-4o-mini"}, {"@review", "review", "gpt-4o"}} {
		result, err := selectStartupModel(context.Background(), startupModelOptions{CLIModel: tc.selector}, settings, services)
		if err != nil || result.Model == nil {
			t.Fatalf("%s: %v %v", tc.selector, result, err)
		}
		if result.Role != tc.role || result.Model.ID != tc.model || result.Thinking != "off" {
			t.Fatalf("%s: role=%s model=%s effort=%s", tc.selector, result.Role, result.Model.ID, result.Thinking)
		}
	}
}
