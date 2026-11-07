package subagents

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

func TestHubIncludesChildrenWithoutDoubleBilling(t *testing.T) {
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{
			{ID: "parent", Type: "general-purpose", Status: "completed", Turns: 2, LifetimeUsage: ai.Usage{Input: 100, Output: 20, CacheRead: 1000, Cost: ai.UsageCost{Total: 3}}},
			{ID: "child", ParentID: "parent", Type: "Explore", Status: "completed", Turns: 1, LifetimeUsage: ai.Usage{Input: 60, Output: 10, CacheRead: 500, Cost: ai.UsageCost{Total: 1}}},
		}
	}})
	defer m.Close()
	rows := m.HubSnapshot()
	if len(rows) != 2 || rows[0].State != "parked" || rows[0].Usage.TotalTokens != 50 || rows[1].Usage.TotalTokens != 70 {
		t.Fatalf("incorrect direct usage: %+v", rows)
	}
	if rows[0].Usage.Cost.Total+rows[1].Usage.Cost.Total != 3 {
		t.Fatal("nested cost billed twice")
	}
	if _, ok := m.Find("child"); ok {
		t.Fatal("human hub weakened the model ownership boundary")
	}
}

func TestHubRevivePromptAndKillOwnSession(t *testing.T) {
	var creates, sends atomic.Int32
	child := &fakeChild{send: func(context.Context, string) (string, error) { sends.Add(1); return "follow-up complete", nil }}
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{{ID: "saved", Type: "general-purpose", Status: "completed", SessionFile: "saved.jsonl", Config: Config{Name: "general-purpose"}}}
	}, Create: func(_ context.Context, inv Invocation) (Child, error) {
		creates.Add(1)
		if inv.Params.Resume != "saved" {
			return nil, fmt.Errorf("resume session was not selected")
		}
		return child, nil
	}})
	defer m.Close()
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	if err := m.ReviveObserved("saved"); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return m.HubSnapshot()[0].State == "idle" })
	if creates.Load() != 1 || sends.Load() != 0 {
		t.Fatal("revive spent a model turn")
	}
	if err := m.PromptObserved("saved", "continue reviewing"); err != nil {
		t.Fatal(err)
	}
	result, err := m.Wait(context.Background(), "saved", "", false)
	if err != nil || result.Result != "follow-up complete" || creates.Load() != 1 || sends.Load() != 1 {
		t.Fatalf("follow-up did not reuse child: %+v %v", result, err)
	}
	if err = m.KillObserved("saved"); err != nil {
		t.Fatal(err)
	}
	await(t, child.closed.Load)
	if row := m.HubSnapshot()[0]; row.State != "aborted" || !row.Killed {
		t.Fatal(row)
	}
	if err = m.PromptObserved("saved", "resurrect"); err == nil {
		t.Fatal("killed session accepted a prompt")
	}
	if err = m.ReviveObserved("saved"); err == nil {
		t.Fatal("killed session revived")
	}
}

func TestHubActivityUpdatesToolsAndStaysBounded(t *testing.T) {
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir()})
	defer m.Close()
	r := &run{Record: Record{ID: "test"}}
	m.onEvent(r, agent.ToolExecutionStartEvent{ToolCallID: "call", ToolName: "read", Args: []byte(`{"path":"auth.go"}`)})
	before := m.HubActivity()
	m.onEvent(r, agent.ToolExecutionEndEvent{ToolCallID: "call", ToolName: "read", IsError: true, Result: agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "not found"}}}})
	after := m.HubActivity()
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].Status != "error" || after[0].Summary != "not found" {
		t.Fatalf("tool activity was not updated: %+v", after)
	}
	m.mu.Lock()
	for range hubActivityLimit + 10 {
		m.hubEventLocked(r, "response", "Response", "text", "completed", 0)
	}
	m.mu.Unlock()
	events := m.HubActivity()
	if len(events) != hubActivityLimit || events[0].ID != 12 || events[len(events)-1].ID != hubActivityLimit+11 {
		t.Fatal("activity ring lost ordering", events[0].ID, events[len(events)-1].ID)
	}
	m.mu.Lock()
	remaining := len(m.hubTools)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatal("evicted tool handler retained")
	}
}

func TestHubCanResumeSettledChildWithoutGrantingModelDelegation(t *testing.T) {
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{
			{ID: "parent", Type: "general-purpose", Status: "completed"},
			{ID: "child", ParentID: "parent", Type: "general-purpose", Status: "completed", Config: Config{Name: "general-purpose"}},
		}
	}, Create: func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { return "human follow-up", nil }}, nil
	}})
	defer m.Close()
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Spawn(context.Background(), Params{Resume: "child", Prompt: "model follow-up"}, "parent", ""); err == nil {
		t.Fatal("model resumed child after parent settled")
	}
	if err := m.PromptObserved("child", "human follow-up"); err != nil {
		t.Fatal(err)
	}
	result, err := m.Wait(context.Background(), "child", "parent", false)
	if err != nil || result.Result != "human follow-up" || result.ParentID != "parent" {
		t.Fatalf("%+v %v", result, err)
	}
}
