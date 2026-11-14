package codingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// TestTodoRestoreIndexedBranch preserves pre-compaction plans, view/error
// semantics and fork isolation for both the streaming probe and decoder fallback.
func TestTodoRestoreIndexedBranch(t *testing.T) {
	for _, name := range []string{`"toolName":"todo"`, `"toolName":"to\u0064o"`, `"tool\u004eame":"todo"`, `"toolName":"read","toolName":"todo"`, `"ToolName":"todo"`} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			var data strings.Builder
			fmt.Fprintln(&data, `{"type":"session","version":3,"id":"widget","cwd":"/tmp"}`)
			result := func(id, parent, op, task string, failed bool) {
				fmt.Fprintf(&data, `{"type":"message","id":%q,"parentId":%q,"message":{"role":"toolResult",%s,"toolCallId":%q,"content":[{"type":"text","text":%q}],"details":{"op":%q,"phases":[{"name":"Plan","tasks":[{"content":%q,"status":"pending"}]}]},"isError":%t,"timestamp":1}}`+"\n", id, parent, name, id, task, op, task, failed)
			}
			result("plan", "", "init", "kept plan", false)
			fmt.Fprintln(&data, `{"type":"message","id":"read","parentId":"plan","message":{"role":"toolResult","toolName":"read","toolCallId":"r","content":[{"type":"text","text":"unrelated"}],"timestamp":1}}`)
			fmt.Fprintln(&data, `{"type":"compaction","id":"compact","parentId":"read","firstKeptEntryId":"read","summary":"summary","tokensBefore":100}`)
			result("view", "compact", "view", "not authoritative", false)
			result("failure", "view", "done", "failed operation", true)
			result("fork", "plan", "init", "other branch", false)
			fmt.Fprintln(&data, `{"type":"session_info","id":"leaf","parentId":"failure","name":"fixture"}`)
			if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
				t.Fatal(err)
			}
			s := loadFixtureSession(t, path)
			m := &InteractiveMode{opts: InteractiveModeOptions{SessionHandle: widgetSession{session: s}}}
			before := s.EntryReads()
			m.restoreTodoWidget(nil)
			if m.todoWidget == nil || m.todoWidgetError != "failed operation" {
				t.Fatal("lost plan or last error")
			}
			if read := s.EntryReads() - before; read != 1 {
				t.Fatalf("decoded %d cold records, want only pre-compaction todo", read)
			}
			if text := m.todoWidget.Output; text != "kept plan" {
				t.Fatalf("restored %q", text)
			}
			leaf := "fork"
			if err := s.SetLeafID(&leaf); err != nil {
				t.Fatal(err)
			}
			m.restoreTodoWidget(nil)
			if m.todoWidget == nil || m.todoWidget.Output != "other branch" || m.todoWidgetError != "" {
				t.Fatal("fork leaked widget state")
			}
			_, err := s.AppendMessage(agent.AgentMessage{ToolResult: &agent.ToolResultMessage{Role: "toolResult", ToolName: "todo", ToolCallID: "clear", Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "clear"}}, Details: map[string]any{"op": "init", "phases": []any{}}}})
			if err != nil {
				t.Fatal(err)
			}
			m.restoreTodoWidget(nil)
			if m.todoWidget != nil || m.todoWidgetError != "" {
				t.Fatal("appended clear was not indexed")
			}
		})
	}
}
