package coding

import (
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

func TestLastAssistantTextSkipsNonStopTextlessResponses(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(Settings{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer services.Close()
	session, err := NewSession(services, SessionOptions{NoSession: true, NoTools: "all", ResourceLoader: NoResources})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	previous := agent.AgentMessage{Assistant: &agent.AssistantMessage{
		Role: "assistant", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "previous answer"}}, StopReason: ai.StopReasonStop,
	}}
	for _, reason := range []ai.StopReason{ai.StopReasonAborted, ai.StopReasonError, ai.StopReasonToolUse, ai.StopReasonLength} {
		t.Run(string(reason), func(t *testing.T) {
			session.Agent().SetMessages([]agent.AgentMessage{
				previous,
				{Assistant: &agent.AssistantMessage{Role: "assistant", StopReason: reason}},
			})
			got := session.LastAssistantText()
			if got == nil || *got != "previous answer" {
				t.Fatalf("LastAssistantText() = %v, want previous answer", got)
			}
		})
	}

	session.Agent().SetMessages([]agent.AgentMessage{{
		Assistant: &agent.AssistantMessage{Role: "assistant", StopReason: ai.StopReasonStop},
	}})
	if got := session.LastAssistantText(); got != nil {
		t.Fatalf("normally stopped textless reply should remain the last reply, got %q", *got)
	}
}
