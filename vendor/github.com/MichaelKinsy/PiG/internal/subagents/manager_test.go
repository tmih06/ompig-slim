package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
)

type fakeChild struct {
	mu       sync.Mutex
	listener func(agent.AgentEvent)
	messages []agent.AgentMessage
	steers   []string
	send     func(context.Context, string) (string, error)
	closed   atomic.Bool
}

func TestConsumedResultDoesNotNotifyAndGroupWaits(t *testing.T) {
	var notified atomic.Int32
	first, second := make(chan struct{}), make(chan struct{})
	cwd := t.TempDir()
	m := New(Host{CWD: cwd, AgentDir: t.TempDir(), SessionID: "notification", Notify: func(string) { notified.Add(1) }, Create: func(_ context.Context, inv Invocation) (Child, error) {
		return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
			gate := first
			if p == "second" {
				gate = second
			}
			select {
			case <-gate:
				return p, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}}, nil
	}})
	defer m.Close()
	if err := m.Update(map[string]any{"enabled": true, "defaultJoinMode": "group"}); err != nil {
		t.Fatal(err)
	}
	a, err := m.Spawn(context.Background(), Params{Prompt: "first"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Spawn(context.Background(), Params{Prompt: "second"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	close(first)
	await(t, func() bool { r, _ := m.Find(a.ID); return !r.Active() })
	m.flushNotifications()
	if notified.Load() != 0 {
		t.Fatal("group notified before sibling completion")
	}
	if _, err = m.Consume(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	close(second)
	await(t, func() bool { r, _ := m.Find(b.ID); return !r.Active() })
	m.flushNotifications()
	if notified.Load() != 1 {
		t.Fatalf("group did not emit just the unconsumed result: %d", notified.Load())
	}
}

func TestScheduleSingleOwnerAndPackageBoundary(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	if err := SaveSettings(cwd, map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	first := New(Host{CWD: cwd, AgentDir: dir, SessionID: "same"})
	defer first.Close()
	if _, err := first.Spawn(context.Background(), Params{Prompt: "schedule", Schedule: "+1h"}, "", ""); err != nil {
		t.Fatal(err)
	}
	second := New(Host{CWD: cwd, AgentDir: dir, SessionID: "same"})
	defer second.Close()
	if _, err := second.Spawn(context.Background(), Params{Prompt: "duplicate", Schedule: "+1h"}, "", ""); err == nil {
		t.Fatal("two processes can own the same schedule file")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"@test/agency","pi":{"subagents":{"agents":["./agents"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "review.md"), []byte("---\nname: review\ndescription: package reviewer\n---\nReview."), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "agents", "escape.md")); err != nil {
		t.Fatal(err)
	}
	resources, warnings := packageResources([]PackageRoot{{Path: root, Source: "npm:@test/agency"}}, "agents", []any{"agency"})
	if len(resources) != 1 || len(warnings) != 1 {
		t.Fatalf("package boundary: %+v %v", resources, warnings)
	}
	resources, _ = packageResources([]PackageRoot{{Path: root}}, "agents", false)
	if len(resources) != 0 {
		t.Fatal("disabled package agents still load")
	}
}

func TestCrossExtensionRPCReplyAndOwnership(t *testing.T) {
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { return "rpc complete", nil }}, nil
	})
	bus := extension.CreateEventBus()
	unsubscribe := m.BindRPC(bus)
	defer unsubscribe()
	replies := make(chan map[string]any, 1)
	bus.On("subagents:rpc:ping:reply:ping", func(data any) { replies <- data.(map[string]any) })
	bus.Emit("subagents:rpc:ping", map[string]any{"requestId": "ping"})
	if reply := <-replies; reply["success"] != true {
		t.Fatal(reply)
	}
	bus.On("subagents:rpc:spawn:reply:spawn", func(data any) { replies <- data.(map[string]any) })
	bus.Emit("subagents:rpc:spawn", map[string]any{"requestId": "spawn", "type": "Explore", "prompt": "RPC"})
	select {
	case reply := <-replies:
		if reply["success"] != true {
			t.Fatal(reply)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RPC spawn did not reply")
	}
}

func (c *fakeChild) Send(ctx context.Context, prompt string) ([]agent.AgentMessage, error) {
	c.mu.Lock()
	fn := c.listener
	c.mu.Unlock()
	if fn != nil {
		fn(agent.TurnStartEvent{})
	}
	text, err := c.send(ctx, prompt)
	msg := agent.AgentMessage{Assistant: &agent.AssistantMessage{Role: "assistant", Content: []ai.AssistantContentBlock{ai.TextContent{Text: text}}, Usage: &ai.Usage{Input: 20, Output: 5, CacheRead: 3, Cost: ai.UsageCost{Total: .001}}}}
	c.mu.Lock()
	c.messages = append(c.messages, msg)
	fn = c.listener
	c.mu.Unlock()
	if fn != nil {
		fn(agent.MessageEndEvent{Message: msg})
	}
	return c.Messages(), err
}
func (c *fakeChild) QueueSteer(text string, _ []ai.ImageContent) {
	c.mu.Lock()
	c.steers = append(c.steers, text)
	c.mu.Unlock()
}
func (c *fakeChild) Messages() []agent.AgentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agent.AgentMessage(nil), c.messages...)
}
func (c *fakeChild) Path() string { return "" }
func (c *fakeChild) Close() error { c.closed.Store(true); return nil }
func (c *fakeChild) Subscribe(fn func(agent.AgentEvent)) func() {
	c.mu.Lock()
	c.listener = fn
	c.mu.Unlock()
	return func() { c.mu.Lock(); c.listener = nil; c.mu.Unlock() }
}
func testManager(t *testing.T, create func(context.Context, Invocation) (Child, error)) *Manager {
	t.Helper()
	cwd := t.TempDir()
	m := New(Host{CWD: cwd, AgentDir: t.TempDir(), SessionID: newID("test_"), Create: create})
	if err := m.Update(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestQueuePoolsSteeringAndDisableDrain(t *testing.T) {
	var created atomic.Int32
	var closed atomic.Int32
	m := testManager(t, func(ctx context.Context, inv Invocation) (Child, error) {
		created.Add(1)
		return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
			<-ctx.Done()
			closed.Add(1)
			return "partial", ctx.Err()
		}}, nil
	})
	if err := m.Update(map[string]any{"maxConcurrent": 1}); err != nil {
		t.Fatal(err)
	}
	a, err := m.Spawn(context.Background(), Params{Type: "Explore", Prompt: "first", Name: "audit"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return created.Load() == 1 })
	b, err := m.Spawn(context.Background(), Params{Type: "explore", Prompt: "second"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "queued" || b.Handle != "explore-2" {
		t.Fatalf("second background run: %+v", b)
	}
	fg, err := m.Spawn(context.Background(), Params{Type: "Plan", Prompt: "foreground", Background: new(false)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return created.Load() == 2 })
	if fg.Status != "running" {
		t.Fatal(fg.Status)
	}
	if err = m.Steer("audit", "check RPC too", ""); err != nil {
		t.Fatal(err)
	}
	if err = m.Stop(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return created.Load() == 3 })
	if err = m.Update(map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 3 {
		t.Fatalf("disable returned before drain: %d", closed.Load())
	}
	for _, r := range m.Records() {
		if r.Active() {
			t.Fatalf("active record after disable: %+v", r)
		}
	}
	if _, err = m.Spawn(context.Background(), Params{Prompt: "disabled"}, "", ""); err == nil {
		t.Fatal("disabled feature spawned a child")
	}
}

func TestForegroundHandoffSurvivesCallerCancellation(t *testing.T) {
	release := make(chan struct{})
	child := &fakeChild{send: func(ctx context.Context, p string) (string, error) {
		select {
		case <-release:
			return "finished", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return child, nil })
	ctx, cancel := context.WithCancel(context.Background())
	r, err := m.Spawn(ctx, Params{Prompt: "work", Background: new(false)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Background(r.ID) {
		t.Fatal("handoff unavailable")
	}
	got, err := m.Wait(ctx, r.ID, "", true)
	if err != nil || !got.Background {
		t.Fatalf("handoff: %+v %v", got, err)
	}
	cancel()
	close(release)
	waitCtx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	got, err = m.Wait(waitCtx, r.ID, "", false)
	if err != nil || got.Result != "finished" || got.Status != "completed" {
		t.Fatalf("background run did not survive caller: %+v %v", got, err)
	}
}

func TestResumeAndUsageKeepSession(t *testing.T) {
	var creations atomic.Int32
	child := &fakeChild{send: func(_ context.Context, p string) (string, error) { return p, nil }}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { creations.Add(1); return child, nil })
	r, err := m.Spawn(context.Background(), Params{Prompt: "first", Background: new(false)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	r, err = m.Wait(context.Background(), r.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Tokens != 25 || r.OutputTokens != 5 {
		t.Fatalf("usage: %+v", r)
	}
	resumed, err := m.Spawn(context.Background(), Params{Prompt: "second", Resume: r.ID, Background: new(false)}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err = m.Wait(context.Background(), resumed.ID, "", false)
	if err != nil || resumed.ID != r.ID || resumed.Result != "second" || resumed.Tokens != 50 || creations.Load() != 1 {
		t.Fatalf("resume: %+v %v creations %d", resumed, err, creations.Load())
	}
}

func TestNestedOwnershipAndDepth(t *testing.T) {
	hold := make(chan struct{})
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(ctx context.Context, p string) (string, error) {
			select {
			case <-hold:
				return p, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}}, nil
	})
	path := filepath.Join(m.host.CWD, ".pi", "agents")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "parent.md"), []byte("---\nname: parent\ndescription: Parent\nallowed_subagents: [Explore]\n---\nDelegate."), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	parent, err := m.Spawn(context.Background(), Params{Type: "parent", Prompt: "parent"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := m.Spawn(context.Background(), Params{Type: "Explore", Prompt: "nested"}, parent.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Consume(child.ID, ""); err == nil {
		t.Fatal("main can access ownership-scoped nested child")
	}
	if _, err = m.Spawn(context.Background(), Params{Type: "Plan", Prompt: "outside allowlist"}, parent.ID, ""); err == nil {
		t.Fatal("parent escaped allowlist")
	}
	if _, err = m.Spawn(context.Background(), Params{Type: "Explore", Prompt: "too deep"}, child.ID, ""); err == nil {
		t.Fatal("depth cap did not apply")
	}
	if err = m.Stop(parent.ID, ""); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Wait(context.Background(), parent.ID, "", false)
	r, _ := m.Consume(child.ID, parent.ID)
	if r.Active() {
		t.Fatal("parent completion did not stop its child")
	}
}

func TestWorkflowRealVMAndReplay(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(_ context.Context, p string) (string, error) { calls.Add(1); return strings.ToUpper(p), nil }}, nil
	})
	script := `export const meta={name:'parallel-test',description:'Native orchestration',phases:[{title:'Scan',detail:'search'}]}; phase('Scan'); const items=await pipeline(args, (item)=>agent(item,{label:item}), (result,item,index)=>({result,item,index})); return items;`
	w, err := m.StartWorkflow(context.Background(), WorkflowParams{Script: script, Args: json.RawMessage(`["a","b"]`)})
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { w, _ = m.Workflow(w.ID); return !w.Active() })
	if w.Status != "completed" {
		t.Fatalf("real VM workflow failed: %s", w.ResultText())
	}
	if string(w.Result) != `[{"result":"A","item":"a","index":0},{"result":"B","item":"b","index":1}]` {
		t.Fatal(string(w.Result))
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	second, err := m.StartWorkflow(context.Background(), WorkflowParams{ScriptPath: w.ScriptPath, Args: json.RawMessage(`["a","b"]`), ResumeFrom: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { second, _ = m.Workflow(second.ID); return !second.Active() })
	if second.Status != "completed" || calls.Load() != 2 {
		t.Fatalf("replay reran children: %+v calls=%d", second, calls.Load())
	}
}

func TestWorkflowDeterminismAndMetadata(t *testing.T) {
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return nil, fmt.Errorf("unexpected child") })
	for _, expr := range []string{"Date.now()", "Math.random()", "new Date()", "eval('1')", "Function('return 1')()"} {
		t.Run(expr, func(t *testing.T) {
			w, err := m.StartWorkflow(context.Background(), WorkflowParams{Script: `export const meta={name:'test',description:'determinism'}; return ` + expr})
			if err != nil {
				t.Fatal(err)
			}
			await(t, func() bool { w, _ = m.Workflow(w.ID); return !w.Active() })
			if w.Status != "failed" || !strings.Contains(w.Error, "unavailable") {
				t.Fatalf("not blocked: %s", w.ResultText())
			}
		})
	}
	for _, literal := range []string{`{name: call(), description:'x'}`, `{...other,name:'x',description:'x'}`, `{name:'x',description:variable}`} {
		if _, _, err := ExtractMeta("export const meta=" + literal); err == nil {
			t.Fatal("executable metadata accepted", literal)
		}
	}
}

func TestSettingsAndCustomAgentPrecedence(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "subagents.json"), []byte(`{"maxConcurrent":2,"showModel":true,"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(cwd, map[string]any{"maxConcurrent": 3, "packageAgents": false}); err != nil {
		t.Fatal(err)
	}
	s, w := LoadSettings(cwd, dir)
	if len(w) > 0 || s.MaxConcurrent != 3 || !s.ShowModel || !s.Enabled {
		t.Fatalf("merge: %+v %v", s, w)
	}
	if err := SaveSettings(cwd, map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(cwd, ".pi", "subagents.json"))
	if !strings.Contains(string(b), `"packageAgents": false`) {
		t.Fatal("unknown setting lost")
	}
	c, err := ParseConfig("review.md", []byte("---\nname: review\ndescription: Review\ntools: read, grep\ndisallowedTools: [bash]\nmodel: sonnet\nmax_turns: 4\nrun_in_background: false\n---\nReview carefully."))
	if err != nil {
		t.Fatal(err)
	}
	p := resolveInvocation(c, Params{Model: "haiku", Background: new(true), MaxTurns: new(9)}, DefaultSettings(), false)
	if p.Model != "sonnet" || *p.Background || *p.MaxTurns != 4 || c.SystemPrompt != "Review carefully." || len(c.Tools) != 2 {
		t.Fatalf("config precedence: %+v %+v", c, p)
	}
}

func TestScheduleFormatsAndCancellation(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		spec  string
		after time.Duration
		once  bool
	}{{"5m", 5 * time.Minute, false}, {"+10m", 10 * time.Minute, true}, {"0 */15 * * * *", 15 * time.Minute, false}, {"2026-11-07T11:30:00Z", time.Hour, true}} {
		next, once, err := nextFire(tc.spec, now)
		if err != nil || next.Sub(now) != tc.after || once != tc.once {
			t.Fatalf("%s: %v %t %v", tc.spec, next, once, err)
		}
	}
	var count atomic.Int32
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { count.Add(1); return "scheduled", nil }}, nil
	})
	r, err := m.Spawn(context.Background(), Params{Prompt: "later", Schedule: "+250ms"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "scheduled" {
		t.Fatal(r)
	}
	await(t, func() bool { return count.Load() == 1 })
	time.Sleep(300 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatal("one-shot repeated")
	}
}

func TestWorkflowRetryKeepsAwaitedCallAndCountsAttempts(t *testing.T) {
	var creations atomic.Int32
	var firstStarted atomic.Bool
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		attempt := creations.Add(1)
		return &fakeChild{send: func(ctx context.Context, _ string) (string, error) {
			if attempt == 1 {
				firstStarted.Store(true)
				<-ctx.Done()
				return "", ctx.Err()
			}
			return "retry succeeded", nil
		}}, nil
	})
	w, err := m.StartWorkflow(context.Background(), WorkflowParams{Script: `export const meta={name:'retry',description:'Retry live work'}; return await agent('task');`})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	await(t, func() bool {
		records := m.WorkflowRecords(w.ID)
		if len(records) == 1 && records[0].Status == "running" && firstStarted.Load() {
			id = records[0].ID
			return true
		}
		return false
	})
	if err = m.RetryWorkflowAgent(w.ID, id); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w, err = m.WaitWorkflow(ctx, w.ID)
	if err != nil || w.Status != "completed" || string(w.Result) != `"retry succeeded"` || w.DoneCount != 1 || w.TotalCount != 1 || w.OutputTokens != 10 || creations.Load() != 2 {
		t.Fatalf("retry failed: %+v, %v, creations=%d", w, err, creations.Load())
	}
	if len(m.WorkflowRecords(w.ID)) != 1 {
		t.Fatal("abandoned retry attempt remained in inspector")
	}
}

func TestPackageDisableCreatesLocalOverride(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "review.md")
	source := "---\nname: review\ndescription: Package review\n---\nReview code."
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	m := testManager(t, func(context.Context, Invocation) (Child, error) { return nil, nil })
	c, err := ParseConfig(path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	c.Source = "package"
	target, err := m.EjectConfig(c, true)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != source {
		t.Fatal("package file changed")
	}
	b, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	override, err := ParseConfig(target, b)
	if err != nil || override.Enabled == nil || *override.Enabled {
		t.Fatalf("disabled override invalid: %+v %v", override, err)
	}
	if _, err = m.EjectConfig(c, false); err == nil {
		t.Fatal("eject overwrote an existing local definition")
	}
}

func TestUsageLedgerAndParentPoolAreCountedOnce(t *testing.T) {
	var ledgers atomic.Int32
	m := testManager(t, func(context.Context, Invocation) (Child, error) {
		return &fakeChild{send: func(context.Context, string) (string, error) { return "ok", nil }}, nil
	})
	m.host.Event = func(name string, _ any) {
		if name == "subagents:usage" {
			ledgers.Add(1)
		}
	}
	if err := m.Update(map[string]any{"reportUsage": true}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r, err := m.Spawn(context.Background(), Params{Prompt: "spend"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.Wait(context.Background(), r.ID, "", false); err != nil {
			t.Fatal(err)
		}
	}
	if m.DrainUsage("child") != nil {
		t.Fatal("child drained parent usage")
	}
	usage := m.DrainUsage("")
	if usage == nil || usage.TotalTokens != 56 || ledgers.Load() != 2 {
		t.Fatalf("usage=%+v ledgers=%d", usage, ledgers.Load())
	}
	if m.DrainUsage("") != nil {
		t.Fatal("usage reported twice")
	}
}
