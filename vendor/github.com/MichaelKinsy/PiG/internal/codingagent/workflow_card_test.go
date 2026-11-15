package codingagent

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func TestWorkflowNotificationCardMarksOutcome(t *testing.T) {
	for status, wantMark := range map[string]string{"completed": "✓", "failed": "✗", "killed": "✗"} {
		card := &subagentNotificationCard{text: `<task-notification><task-id>wf_ab12</task-id><status>` + status + `</status>Workflow wf_ab12 (smoke): ` + status + `</task-notification>`}
		rendered := widthx.StripAnsi(strings.Join(card.Render(60), "\n"))
		if !strings.HasPrefix(rendered, wantMark+" ") {
			t.Fatalf("workflow status %q rendered %q, want the %s mark", status, rendered, wantMark)
		}
		if strings.Contains(rendered, "task-notification") || strings.Contains(rendered, "<status>") {
			t.Fatalf("workflow status %q leaked its envelope: %q", status, rendered)
		}
	}
}
