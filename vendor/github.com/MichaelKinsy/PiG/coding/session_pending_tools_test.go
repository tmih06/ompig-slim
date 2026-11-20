package coding

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

func newSessionWithPendingTool(t *testing.T) *Session {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: icodingagent.NewInMemorySettingsManager(Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { services.Close() })
	session, err := NewSession(services, SessionOptions{
		NoSession: true, ResourceLoader: NoResources, InitialActiveToolNames: []string{"read", "bash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	transcript := []agent.AgentMessage{{System: &ai.SystemMessage{ToolsAdded: []ai.ToolSchema{
		{Name: "read"}, {Name: "mcp__fixture__search"},
	}}}}
	session.applyTranscriptTools(transcript, nil)
	if !slices.Contains(session.ActiveToolNames(), "read") {
		t.Fatalf("restored tools omit read: %v", session.ActiveToolNames())
	}
	// A newly selected tool that was not in the restored loadout can be removed
	// without replacing the restored loadout's still-pending MCP tool.
	session.SetActiveToolsByName([]string{"read", "bash"})
	if !slices.Contains(session.ActiveToolNames(), "bash") {
		t.Fatalf("fixture tools omit bash: %v", session.ActiveToolNames())
	}
	return session
}

func TestSetActiveToolsKeepsPendingToolsWhenNewToolDeactivated(t *testing.T) {
	session := newSessionWithPendingTool(t)
	session.SetActiveToolsByName([]string{"read"})
	session.toolRegistryMu.RLock()
	pending := slices.Clone(session.pendingToolNames)
	session.toolRegistryMu.RUnlock()
	if !slices.Equal(pending, []string{"mcp__fixture__search"}) {
		t.Fatalf("deactivating non-restored tool dropped pending tools: %v", pending)
	}
}

func TestSetActiveToolsDropsPendingToolsWhenRestoredToolDeactivated(t *testing.T) {
	session := newSessionWithPendingTool(t)
	session.SetActiveToolsByName([]string{"bash"})
	session.toolRegistryMu.RLock()
	pending := slices.Clone(session.pendingToolNames)
	session.toolRegistryMu.RUnlock()
	if len(pending) != 0 {
		t.Fatalf("deactivating restored tool kept pending tools: %v", pending)
	}
}
