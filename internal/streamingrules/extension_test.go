package streamingrules

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

type recordingUI struct {
	extension.UIContext
	notices []string
}

func (u *recordingUI) Notify(message, kind string) {
	u.notices = append(u.notices, kind+": "+message)
}

type captureAPI struct {
	extension.API
	ui       *recordingUI
	renderer extension.EntryRenderer
	settings map[string]any
	start    extension.ExtensionHandlerNoResult[extension.AgentStartEvent]
	update   extension.ExtensionHandlerNoResult[extension.MessageUpdateEvent]
	call     extension.ExtensionHandler[extension.ToolCallEvent, extension.ToolCallEventResult]
	result   extension.ExtensionHandler[extension.ToolResultEvent, extension.ToolResultEventResult]
	end      extension.ExtensionHandler[extension.TurnEndEvent, extension.TurnEndEventResult]
}

func (a *captureAPI) GetSettings() extension.Settings { return a.settings }
func (a *captureAPI) RegisterEntryRenderer(customType string, renderer extension.EntryRenderer) {
	if customType == pokeType {
		a.renderer = renderer
	}
}
func (a *captureAPI) OnAgentStart(h extension.ExtensionHandlerNoResult[extension.AgentStartEvent]) func() {
	a.start = h
	return func() {}
}
func (a *captureAPI) OnMessageUpdate(h extension.ExtensionHandlerNoResult[extension.MessageUpdateEvent]) func() {
	a.update = h
	return func() {}
}
func (a *captureAPI) OnToolCall(h extension.ExtensionHandler[extension.ToolCallEvent, extension.ToolCallEventResult]) func() {
	a.call = h
	return func() {}
}
func (a *captureAPI) OnToolResult(h extension.ExtensionHandler[extension.ToolResultEvent, extension.ToolResultEventResult]) func() {
	a.result = h
	return func() {}
}
func (a *captureAPI) OnTurnEnd(h extension.ExtensionHandler[extension.TurnEndEvent, extension.TurnEndEventResult]) func() {
	a.end = h
	return func() {}
}

func startExtension(t *testing.T, cwd, agentDir string) (*captureAPI, context.Context) {
	t.Helper()
	api := &captureAPI{ui: &recordingUI{}}
	ctx := extension.WithContext(context.Background(), extension.NewContext(cwd, api.ui, func() error { return nil }, extension.ContextActions{}))
	if err := Extension(agentDir)(api); err != nil {
		t.Fatal(err)
	}
	if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
		t.Fatal(err)
	}
	return api, ctx
}

func toolCall(id, name string, args map[string]any) extension.CustomToolCallEvent {
	return extension.CustomToolCallEvent{
		ToolCallEventBase: extension.ToolCallEventBase{ToolCallID: id}, ToolName: name, Input: args,
	}
}

func boundary(outcome extension.AgentActivityOutcome) extension.TurnEndEvent {
	return extension.TurnEndEvent{BoundaryState: &extension.BoundaryState{Outcome: outcome}}
}

func TestBlockingAndFinalArguments(t *testing.T) {
	cwd, agentDir := t.TempDir(), t.TempDir()
	directory := filepath.Join(cwd, ".pi/rules")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "no-bad.md"), []byte("---\ncondition: BAD\nscope: tool:write(*.go)\n---\nUse GOOD instead."), 0o600); err != nil {
		t.Fatal(err)
	}
	api, ctx := startExtension(t, cwd, agentDir)
	for _, text := range []string{"B", "BA", "BAD"} {
		partial := &ai.AssistantMessage{Content: []ai.AssistantContentBlock{ai.ToolCall{
			ID: "streamed", Name: "write", Arguments: ai.JsonObject{"path": "a.go", "content": text},
		}}}
		if api.update != nil {
			if err := api.update(ctx, extension.MessageUpdateEvent{AssistantMessageEvent: ai.ToolCallDeltaEvent{ContentIndex: 0, Partial: partial}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := api.call(ctx, toolCall("streamed", "write", map[string]any{"path": "a.go", "content": "GOOD"}))
	if err != nil || result.Block {
		t.Fatal("stale partial match blocked final arguments", result, err)
	}
	result, err = api.call(ctx, toolCall("no-deltas", "write", map[string]any{"path": "a.go", "content": "BAD"}))
	if err != nil || !result.Block || result.Terminate || !strings.Contains(result.Reason, "Use GOOD instead.") || !strings.Contains(result.Reason, "did not run") {
		t.Fatal("final-only call not blocked with correction", result, err)
	}
	result, err = api.call(ctx, toolCall("retry", "write", map[string]any{"path": "a.go", "content": "BAD"}))
	if err != nil || result.Block {
		t.Fatal("same rule re-fired on its retry", result, err)
	}
	ended, err := api.end(ctx, boundary(extension.AgentActivityCompleted))
	if err != nil || ended.Entries == nil || len(*ended.Entries) != 1 {
		t.Fatal("missing or repeated blocked poke", ended, err)
	}
	assertPoke(t, api, (*ended.Entries)[0], "Tool call blocked", "no-bad", "Use GOOD instead.")
	if len(api.ui.notices) != 0 {
		t.Fatal("poke used warning notifications", api.ui.notices)
	}
	if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
		t.Fatal(err)
	}
	var blocks atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Go(func() {
			result, err := api.call(ctx, toolCall("nested/1", "write", map[string]any{"path": "a.go", "content": "BAD"}))
			if err != nil {
				t.Error(err)
			}
			if result.Block {
				blocks.Add(1)
			}
		})
	}
	workers.Wait()
	if blocks.Load() != 1 {
		t.Fatal("concurrent calls must claim a rule once", blocks.Load())
	}
	ended, err = api.end(ctx, boundary(extension.AgentActivityAborted))
	if err != nil || ended.Entries == nil || len(*ended.Entries) != 1 {
		t.Fatal("concurrent blocks repeated or lost the poke", ended, err)
	}
	assertPoke(t, api, (*ended.Entries)[0], "Tool call blocked", "no-bad", "Use GOOD instead.")
	if again, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || again.Entries != nil {
		t.Fatal("blocked poke leaked to next turn", again, err)
	}
}

func TestChangedExecutionArguments(t *testing.T) {
	cwd, agentDir := t.TempDir(), t.TempDir()
	directory := filepath.Join(agentDir, "rules")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "no-bad.md"), []byte("---\ncondition: BAD\nscope: tool:write(*.go)\n---\nUse GOOD instead."), 0o600); err != nil {
		t.Fatal(err)
	}
	api, ctx := startExtension(t, cwd, agentDir)
	args := map[string]any{"path": "a.go", "content": "GOOD"}
	if result, err := api.call(ctx, toolCall("changed", "write", args)); err != nil || result.Block {
		t.Fatal("blocked valid arguments", result, err)
	}
	args["content"] = "BAD"
	if _, err := api.result(ctx, extension.CustomToolResultEvent{
		ToolResultEventBase: extension.ToolResultEventBase{ToolCallID: "changed", Input: args}, ToolName: "write",
	}); err != nil {
		t.Fatal(err)
	}
	ended, err := api.end(ctx, boundary(extension.AgentActivityCompleted))
	if err != nil || ended.Entries == nil || len(*ended.Entries) != 2 {
		t.Fatal("missed execution arguments changed by a later handler", ended, err)
	}
	assertPoke(t, api, (*ended.Entries)[1], "Reminder sent", "no-bad", "Use GOOD instead.")
}

func TestGenericToolRule(t *testing.T) {
	cwd, agentDir := t.TempDir(), t.TempDir()
	directory := filepath.Join(agentDir, "rules")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "read-path.md"), []byte("---\ncondition: 'danger'\nscope: 'tool:read(*.txt)'\ninterruptMode: never\n---\nCheck the requested path."), 0o600); err != nil {
		t.Fatal(err)
	}
	api, ctx := startExtension(t, cwd, agentDir)
	for _, file := range []string{"danger.go", "safe.txt", "danger.txt"} {
		args := map[string]any{"path": file}
		if result, err := api.call(ctx, toolCall(file, "read", args)); err != nil || result.Block {
			t.Fatal("advisory rule blocked the tool", result, err)
		}
		if _, err := api.result(ctx, extension.CustomToolResultEvent{
			ToolResultEventBase: extension.ToolResultEventBase{ToolCallID: file, Input: args}, ToolName: "read",
		}); err != nil {
			t.Fatal(err)
		}
		ended, err := api.end(ctx, boundary(extension.AgentActivityCompleted))
		if err != nil {
			t.Fatal(err)
		}
		if file != "danger.txt" {
			if ended.Entries != nil {
				t.Fatal("matched outside the scope or condition", ended)
			}
			continue
		}
		if ended.Entries == nil || len(*ended.Entries) != 2 {
			t.Fatal("generic tool arguments did not match", ended)
		}
		assertPoke(t, api, (*ended.Entries)[1], "read-path", "Check the requested path.")
	}
}

func TestPassiveDelivery(t *testing.T) {
	api, ctx := startExtension(t, t.TempDir(), t.TempDir())
	args := map[string]any{"path": "nested/a.go", "content": `import "io/ioutil"`}
	call := toolCall("nested/1", "write", args)
	call.ParentToolCallID = "codemode"
	if result, err := api.call(ctx, call); err != nil || result.Block {
		t.Fatal("advisory rule blocked", result, err)
	}
	result := extension.CustomToolResultEvent{
		ToolResultEventBase: extension.ToolResultEventBase{ToolCallID: call.ToolCallID, Input: args, Content: []any{"original output"}},
		ToolName:            "write",
	}
	result.IsError = true
	if _, err := api.result(ctx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries != nil {
		t.Fatal("warned for failed mutation", got, err)
	}
	if len(api.ui.notices) != 0 {
		t.Fatal("notified for failed mutation", api.ui.notices)
	}
	result.IsError = false
	for i := 0; i < 2; i++ {
		got, err := api.result(ctx, result)
		if err != nil || got.Content != nil || got.IsError != nil {
			t.Fatal("changed original output", got, err)
		}
	}
	prior := extension.SessionBoundaryDraft{Type: extension.SessionBoundaryDraftCustom, CustomType: "other"}
	event := boundary(extension.AgentActivityCompleted)
	event.Entries = []extension.SessionBoundaryDraft{prior}
	got, err := api.end(ctx, event)
	if err != nil || got.Entries == nil || len(*got.Entries) != 3 || got.Continue != nil {
		t.Fatal(got, err)
	}
	entries := *got.Entries
	if entries[0].CustomType != "other" || entries[1].Display || entries[1].CustomType != customType || !strings.Contains(entries[1].Content.(string), "io/ioutil") {
		t.Fatal(entries)
	}
	if len(event.Entries) != 1 {
		t.Fatal("mutated earlier drafts")
	}
	assertPoke(t, api, entries[2], "Reminder sent", "go-ioutil", "Deprecated io/ioutil import. Use io and os.")
	if len(api.ui.notices) != 0 {
		t.Fatal("poke used warning notifications", api.ui.notices)
	}
	if _, err := api.result(ctx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries != nil {
		t.Fatal("duplicate reminder", got, err)
	}
	other, otherCtx := startExtension(t, t.TempDir(), t.TempDir())
	if _, err := other.result(otherCtx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := other.end(otherCtx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries == nil {
		t.Fatal("factories shared delivery state", got, err)
	}
	if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.result(ctx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityAborted)); err != nil || got.Entries != nil {
		t.Fatal("warned after abort", got, err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries != nil {
		t.Fatal("leaked pending warning", got, err)
	}
	if _, err := api.result(ctx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries == nil || len(*got.Entries) != 2 {
		t.Fatal("aborted reminder did not rearm", got, err)
	}
	api.settings = map[string]any{"ttsr": map[string]any{"enabled": false}}
	if err := api.start(ctx, extension.AgentStartEvent{}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.result(ctx, result); err != nil {
		t.Fatal(err)
	}
	if got, err := api.end(ctx, boundary(extension.AgentActivityCompleted)); err != nil || got.Entries != nil {
		t.Fatal("disabled rules still active", got, err)
	}
	if len(api.ui.notices) != 0 {
		t.Fatal("used warning notifications", api.ui.notices)
	}
}
