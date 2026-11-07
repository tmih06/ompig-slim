package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

func TestHubActivitySurvivesRecordPersistenceAndResume(t *testing.T) {
	var saved []byte
	host := Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Persist: func(r Record) error {
		var err error
		saved, err = json.Marshal(r)
		return err
	}}
	m := New(host)
	defer m.Close()
	r := &run{Record: Record{ID: "saved", Status: "completed"}}
	m.runs = append(m.runs, r)
	m.onEvent(r, agent.ToolExecutionStartEvent{ToolCallID: "call", ToolName: "read", Args: []byte(`{"path":"auth.go"}`)})
	m.onEvent(r, agent.ToolExecutionEndEvent{ToolCallID: "call", ToolName: "read", Result: agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "file contents"}}}})
	if err := m.persistRecord(r.Record); err != nil {
		t.Fatal(err)
	}
	before := m.HubActivity()
	before[0].toolKey = "" // Runtime tool-slot bookkeeping is not persisted.
	host.Restore = func() []Record {
		var record Record
		if err := json.Unmarshal(saved, &record); err != nil {
			t.Fatal(err)
		}
		return []Record{record}
	}
	restored := New(host)
	defer restored.Close()
	if after := restored.HubActivity(); !reflect.DeepEqual(before, after) {
		t.Fatalf("resume lost tool activity: before=%+v after=%+v", before, after)
	}
	restored.mu.Lock()
	restored.hubEventLocked(restored.runs[0], "response", "Response", "continued", "completed", 1)
	restored.mu.Unlock()
	events := restored.HubActivity()
	if len(events) != 2 || events[1].ID <= events[0].ID {
		t.Fatal("resumed activity did not continue in order", events)
	}
}

func TestHubRestoresBoundedChronologicalActivityAcrossAgents(t *testing.T) {
	var records [2]Record
	records[0].ID, records[1].ID = "a", "b"
	for i := 1; i <= hubActivityLimit+10; i++ {
		r := &records[i%2]
		r.HubActivity = append(r.HubActivity, HubActivity{ID: uint64(i), AgentID: r.ID, Status: "pending", Title: "read"})
	}
	m := New(Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record { return records[:] }})
	defer m.Close()
	events := m.HubActivity()
	if len(events) != hubActivityLimit || events[0].ID != 11 || events[len(events)-1].ID != hubActivityLimit+10 {
		t.Fatal("restored activity lost chronological order or exceeded its limit")
	}
	for _, e := range events {
		if e.Status != "aborted" {
			t.Fatal("restored event still appears in progress")
		}
	}
	m.mu.Lock()
	m.hubEventLocked(m.runs[0], "response", "Response", "new", "completed", 0)
	m.mu.Unlock()
	events = m.HubActivity()
	if len(events) != hubActivityLimit || events[0].ID != 12 || events[len(events)-1].ID != hubActivityLimit+11 {
		t.Fatal("restored activity ring did not rotate correctly")
	}
}

func TestHubRebuildsActivityFromSavedChildToolCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "child.jsonl")
	log := `{"type":"session","id":"child"}
{"type":"message","message":{"role":"user","content":"inspect auth","timestamp":100}}
{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"call","name":"read","arguments":{"path":"auth.go"}}],"timestamp":200,"stopReason":"toolUse"}}
{"type":"message","message":{"role":"toolResult","toolCallId":"call","toolName":"read","content":[{"type":"text","text":"not found"}],"isError":true,"timestamp":300}}
{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Try config.go instead"}],"timestamp":400,"stopReason":"stop"}}
`
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	host := Host{CWD: t.TempDir(), AgentDir: t.TempDir(), Restore: func() []Record {
		return []Record{{ID: "saved", Status: "completed", SessionFile: path}}
	}}
	m := New(host)
	defer m.Close()
	events := m.HubActivity()
	if len(events) != 2 || events[0].Title != "read" || events[0].Summary != "not found" || events[0].Status != "error" || events[0].Timestamp != 200 || events[0].MessageIndex != 1 {
		t.Fatalf("saved tool call/result not reconstructed: %+v", events)
	}
	if events[1].Summary != "Try config.go instead" || events[1].Timestamp != 400 || events[1].MessageIndex != 3 {
		t.Fatal("saved response/anchor not reconstructed", events[1])
	}
	// Once a record has durable Hub events, do not read or duplicate its log.
	host.Restore = func() []Record {
		return []Record{{ID: "saved", Status: "completed", SessionFile: path, HubActivity: events}}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	restored := New(host)
	defer restored.Close()
	if !reflect.DeepEqual(events, restored.HubActivity()) {
		t.Fatal("persisted activity was duplicated or replaced by transcript fallback")
	}
}

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
