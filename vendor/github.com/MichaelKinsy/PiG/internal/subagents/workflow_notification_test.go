package subagents

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestWorkflowNotificationIncludesTerminalStatus(t *testing.T) {
	for _, status := range []string{"completed", "failed", "killed"} {
		t.Run(status, func(t *testing.T) {
			var notification string
			m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Notify: func(text string) { notification = text }})
			defer m.Close()
			w := &Workflow{ID: "wf_test", Status: status, CompletedAt: 1, Meta: WorkflowMeta{Name: "Audit & report"}}
			m.workflows = []*Workflow{w}
			m.notifications = []completion{{w.ID, w.CompletedAt}}
			m.flushNotifications()
			var envelope struct {
				ID     string `xml:"task-id"`
				Status string `xml:"status"`
				Text   string `xml:",chardata"`
			}
			if err := xml.Unmarshal([]byte(notification), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.ID != w.ID || envelope.Status != status || strings.TrimSpace(envelope.Text) != strings.TrimSpace(w.ResultText()) {
				t.Fatalf("workflow notification lost its status or result: %q", notification)
			}
			notification = ""
			m.flushNotifications()
			if notification != "" {
				t.Fatal("workflow completion notified twice")
			}
		})
	}
}
