package subagents

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestCostDefaultsAndCompactToolDescriptions(t *testing.T) {
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir()})
	defer m.Close()
	if s := m.Settings(); s.AgentMentions != "direct" || s.ToolDescriptionMode != "compact" {
		t.Fatalf("cost-saving defaults missing: mentions=%q descriptions=%q", s.AgentMentions, s.ToolDescriptionMode)
	}
	for _, workflow := range []bool{false, true} {
		compact := m.toolDescription(workflow)
		if len(compact) > 5000 || !strings.Contains(compact, "direct") {
			t.Fatalf("tool description is oversized or omits direct-tool guidance: %d bytes", len(compact))
		}
		m.mu.Lock()
		m.settings.ToolDescriptionMode = "full"
		m.mu.Unlock()
		full := m.toolDescription(workflow)
		if len(compact)*2 >= len(full) {
			t.Fatalf("expected at least 50%% description reduction: compact=%d full=%d", len(compact), len(full))
		}
		t.Logf("workflow=%t description bytes: %d -> %d", workflow, len(full), len(compact))
		m.mu.Lock()
		m.settings.ToolDescriptionMode = "compact"
		m.mu.Unlock()
	}
}

func TestLegacyModelMentionUsesDirectDispatch(t *testing.T) {
	m := testManager(t, func(_ context.Context, inv Invocation) (Child, error) {
		if inv.Params.Prompt != "find the auth handler" {
			t.Error("mention task was rewritten", inv.Params.Prompt)
		}
		return &fakeChild{send: func(context.Context, string) (string, error) { return "auth.go", nil }}, nil
	})
	if err := m.Update(map[string]any{"agentMentions": "model"}); err != nil {
		t.Fatal(err)
	}
	if m.Settings().AgentMentions != "direct" {
		t.Fatal("legacy model mode still permits an orchestration-only model call")
	}
	if err := m.Mention("Explore", "find the auth handler"); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return len(m.Records()) == 1 && !m.Records()[0].Active() })
}

func TestHumanAgentRunsDoNotWakeParentModel(t *testing.T) {
	for _, detached := range []bool{false, true} {
		t.Run(map[bool]string{false: "delegated", true: "human"}[detached], func(t *testing.T) {
			var notifications atomic.Int32
			m := testManager(t, func(context.Context, Invocation) (Child, error) {
				return &fakeChild{send: func(context.Context, string) (string, error) { return "done", nil }}, nil
			})
			m.host.ParentRunning = func() bool { return true } // Own the flush explicitly.
			m.host.Notify = func(string) { notifications.Add(1) }
			if _, err := m.Spawn(context.Background(), Params{Prompt: "task", Background: new(true), Detached: detached}, "", ""); err != nil {
				t.Fatal(err)
			}
			await(t, func() bool { rows := m.Records(); return len(rows) == 1 && !rows[0].Active() })
			m.flushNotifications()
			want := int32(1)
			if detached {
				want = 0
			}
			if notifications.Load() != want {
				t.Fatalf("parent wakeups=%d; want %d", notifications.Load(), want)
			}
		})
	}
}

func TestCompletionNotificationRequestsFetchOnlyForTruncation(t *testing.T) {
	for _, size := range []int{1500, 1501} {
		var notification string
		m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
			return []Record{{ID: "a", Status: "completed", Result: strings.Repeat("界", size), CompletedAt: 1}}
		}, Notify: func(text string) { notification = text }})
		m.mu.Lock()
		m.runs[0].consumed = false
		m.notifications = []completion{{"a", 1}}
		m.mu.Unlock()
		m.flushNotifications()
		m.Close()
		if notification == "" || strings.Contains(notification, "use get_subagent_result") != (size > 1500) {
			t.Fatalf("incorrect retrieval hint for %d-character result", size)
		}
	}
}

func TestAgentResultDoesNotDuplicateIdenticalErrors(t *testing.T) {
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{{ID: "failed", Status: "error", Result: "command failed", Error: "command failed"}}
	}})
	defer m.Close()
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	tool := Tool{Manager: m, ToolName: "get_subagent_result"}
	result, err := tool.Execute(context.Background(), "result", []byte(`{"agent_id":"failed"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := result.Content[0].(ai.TextContent).Text; strings.Count(text, "command failed") != 1 {
		t.Fatal("identical result and error duplicated in model context", text)
	}
}

func TestSmartNotificationPreservesResponsiveness(t *testing.T) {
	for _, test := range []struct {
		name, mode    string
		parentRunning bool
		age           time.Duration
	}{
		{"active parent", "smart", true, 0},
		{"liveness bound", "smart", false, 31 * time.Second},
		{"explicit async", "async", false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			at := time.Now().Add(-test.age).UnixMilli()
			var notifications atomic.Int32
			m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), ParentRunning: func() bool { return test.parentRunning }, Restore: func() []Record {
				return []Record{{ID: "a", Status: "completed", Background: true, CompletedAt: at}, {ID: "b", Status: "completed", Background: true}}
			}, Notify: func(string) { notifications.Add(1) }})
			defer m.Close()
			m.mu.Lock()
			m.settings.DefaultJoinMode = test.mode
			m.runs[0].consumed = false
			m.runs[1].Status = "running"
			m.notifications = []completion{{"a", at}}
			m.mu.Unlock()
			m.flushNotifications()
			if notifications.Load() != 1 {
				t.Fatal("cost batching blocked a notification that should be delivered")
			}
		})
	}
}

func TestSmartNotificationsBatchSiblingCompletions(t *testing.T) {
	now := time.Now().UnixMilli()
	var mu sync.Mutex
	var messages []string
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{{ID: "a", Status: "completed", Background: true, CompletedAt: now, Result: "auth.go"}, {ID: "b", Status: "completed", Background: true}}
	}, Notify: func(text string) { mu.Lock(); messages = append(messages, text); mu.Unlock() }})
	defer m.Close()
	m.mu.Lock()
	m.runs[0].consumed, m.runs[1].consumed = false, false
	m.runs[1].Status = "running"
	m.notifications = []completion{{"a", now}}
	m.mu.Unlock()
	m.flushNotifications()
	mu.Lock()
	count := len(messages)
	mu.Unlock()
	if count != 0 {
		t.Fatal("smart join woke the parent before its sibling completed")
	}
	m.mu.Lock()
	m.runs[1].Status = "completed"
	m.runs[1].CompletedAt = now
	m.runs[1].Result = "config.go"
	m.notifications = append(m.notifications, completion{"b", now})
	m.mu.Unlock()
	m.flushNotifications()
	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 1 || !strings.Contains(messages[0], "auth.go") || !strings.Contains(messages[0], "config.go") {
		t.Fatal("siblings were not delivered together", messages)
	}
	if strings.Contains(messages[0], "Use get_subagent_result") {
		t.Fatal("complete short results unnecessarily request another retrieval")
	}
}
