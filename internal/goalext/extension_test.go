package goalext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/factoryload"
	"github.com/MichaelKinsy/PiG/coding/extension/host/inproc"

	"ompig-slim/internal/goal"
)

// TestActiveGoalRequestsContinuationBeforeSettlement pins the boundary contract:
// an unmet goal keeps the run working instead of first reporting idle.
func TestActiveGoalRequestsContinuationBeforeSettlement(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	var sent extension.SendMessagePayload
	result := continueActiveGoal(context.Background(), tracker, func(payload extension.SendMessagePayload, options *extension.SendMessageOptions) {
		sent = payload
		if options == nil || options.DeliverAs != extension.DeliverAsFollowUp {
			t.Fatalf("continuation options = %+v, want followUp", options)
		}
	})
	if result.Continue == nil || !*result.Continue {
		t.Fatal("an unmet active goal must continue before agent_settled")
	}
	if sent.CustomType != "goal-continuation" {
		t.Fatalf("custom message type = %q, want goal-continuation", sent.CustomType)
	}
	if !sent.Display {
		t.Fatal("goal continuation is hidden from the transcript")
	}
	if sent.Content != "Goal poke 1: Continue working toward this goal: finish the work" {
		t.Fatalf("continuation content = %q", sent.Content)
	}
	if got := tracker.Get().Pokes; got != 1 {
		t.Fatalf("goal pokes = %d, want 1", got)
	}
}

type recordingUI struct {
	extension.UIContext
	notices []string
	widget  []string
}

func (u *recordingUI) Notify(message, kind string) { u.notices = append(u.notices, kind+": "+message) }

func (u *recordingUI) SetWidget(key string, content []string, opts *extension.ExtensionWidgetOptions) {
	if key == "goal" {
		u.widget = content
	}
}

type settledGoal struct {
	runner  *inproc.Runner
	tracker *goal.Tracker
	ui      *recordingUI
	sent    []extension.CustomMessageRef
}

func newSettledGoal(t *testing.T, objective string) *settledGoal {
	t.Helper()
	root := t.TempDir()
	tracker := goal.New()
	if objective != "" {
		if err := tracker.Create(objective, 0); err != nil {
			t.Fatal(err)
		}
	}
	runtime := extension.CreateExtensionRuntime()
	loaded, err := factoryload.LoadExtensionFromFactory(
		func(api extension.API) error { return extensionWithTracker(api, tracker) },
		root, extension.CreateEventBus(), runtime, "<inline:goal>",
	)
	if err != nil {
		t.Fatal(err)
	}
	g := &settledGoal{tracker: tracker, ui: &recordingUI{UIContext: extension.NoopUIContext}}
	g.runner = inproc.NewRunner([]extension.Extension{loaded}, root, runtime)
	t.Cleanup(g.runner.Shutdown)
	g.runner.BindCore(extension.ExtensionActions{SendMessage: func(message extension.CustomMessageRef, options *extension.SendMessageOptions) error {
		g.sent = append(g.sent, message)
		return nil
	}}, extension.ContextActions{}, nil)
	g.runner.SetUIContext(g.ui, extension.ModeTUI)
	return g
}

func (g *settledGoal) settle(t *testing.T, outcome extension.AgentActivityOutcome) inproc.BoundaryDispatchResult {
	t.Helper()
	result, err := g.runner.EmitBoundary(
		context.Background(),
		&extension.AgentBeforeSettleEvent{Type: "agent_before_settle", BoundaryState: extension.BoundaryState{Outcome: outcome}},
		func([]extension.SessionBoundaryDraft) (extension.BoundaryContextPreview, error) {
			return extension.BoundaryContextPreview{CanContinue: true}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestModelErrorPausesTheGoalInsteadOfPoking(t *testing.T) {
	g := newSettledGoal(t, "finish the work")
	result := g.settle(t, extension.AgentActivityError)
	if result.Continue {
		t.Fatal("a failed run requested another turn")
	}
	if len(g.sent) != 0 {
		t.Fatalf("a failed run queued %d follow-ups, want none", len(g.sent))
	}
	state := g.tracker.Get()
	if state == nil || state.Status != goal.StatusPaused {
		t.Fatalf("goal after a model error = %+v, want paused", state)
	}
	if state.Pokes != 0 {
		t.Fatalf("goal pokes = %d, want 0", state.Pokes)
	}
	if len(g.ui.widget) != 1 || !strings.Contains(g.ui.widget[0], "paused") {
		t.Fatalf("goal widget = %q, want the paused state", g.ui.widget)
	}
	if len(g.ui.notices) != 1 || !strings.Contains(g.ui.notices[0], "/goal resume") {
		t.Fatalf("notices = %q, want one naming /goal resume", g.ui.notices)
	}

	if result := g.settle(t, extension.AgentActivityCompleted); result.Continue || len(g.sent) != 0 {
		t.Fatal("a paused goal was poked at the next boundary")
	}
}

func TestModelErrorLeavesNonActiveGoalsAlone(t *testing.T) {
	t.Run("no goal", func(t *testing.T) {
		g := newSettledGoal(t, "")
		result := g.settle(t, extension.AgentActivityError)
		if result.Continue || len(g.sent) != 0 {
			t.Fatal("an absent goal requested a turn")
		}
		if len(g.ui.notices) != 0 {
			t.Fatalf("notices = %q, want none without a goal", g.ui.notices)
		}
	})
	t.Run("completed goal", func(t *testing.T) {
		g := newSettledGoal(t, "finish the work")
		if err := g.tracker.Complete(); err != nil {
			t.Fatal(err)
		}
		if result := g.settle(t, extension.AgentActivityError); result.Continue || len(g.sent) != 0 {
			t.Fatal("a completed goal requested a turn")
		}
		if state := g.tracker.Get(); state == nil || state.Status != goal.StatusComplete {
			t.Fatalf("goal after a model error = %+v, want complete", state)
		}
		if len(g.ui.notices) != 0 {
			t.Fatalf("notices = %q, want none for a completed goal", g.ui.notices)
		}
	})
	t.Run("successful run", func(t *testing.T) {
		g := newSettledGoal(t, "finish the work")
		if result := g.settle(t, extension.AgentActivityCompleted); !result.Continue {
			t.Fatal("an active goal stopped continuing after a successful turn")
		}
		if len(g.sent) != 1 || g.sent[0].CustomType != "goal-continuation" {
			t.Fatalf("queued = %+v, want one goal continuation", g.sent)
		}
		if state := g.tracker.Get(); state == nil || state.Status != goal.StatusActive {
			t.Fatalf("goal after a successful turn = %+v, want active", state)
		}
	})
}

func TestAbortedGoalPausesBeforeAnotherPoke(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	pauseAbortedGoal(tracker, true)
	if state := tracker.Get(); state == nil || state.Status != goal.StatusPaused {
		t.Fatalf("aborted goal state = %+v, want paused", state)
	}
	result := continueActiveGoal(context.Background(), tracker, func(extension.SendMessagePayload, *extension.SendMessageOptions) {
		t.Fatal("paused goal was poked after interruption")
	})
	if result.Continue != nil && *result.Continue {
		t.Fatal("paused goal requested continuation")
	}
}

// TestPausedGoalIsNotPokedAgain pins the behavior the blocker policy exists for:
// once a goal pauses, the settlement boundary stops requesting turns.
func TestPausedGoalIsNotPokedAgain(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < goal.BlockerThreshold; i++ {
		if _, err := tracker.Blocked("needs the staging API token"); err != nil {
			t.Fatal(err)
		}
	}
	pokes := tracker.Get().Pokes
	result := continueActiveGoal(context.Background(), tracker, func(extension.SendMessagePayload, *extension.SendMessageOptions) {
		t.Fatal("paused goal was poked after reaching the blocker threshold")
	})
	if result.Continue != nil && *result.Continue {
		t.Fatal("paused goal requested continuation")
	}
	if got := tracker.Get().Pokes; got != pokes {
		t.Fatalf("pokes = %d, want %d", got, pokes)
	}
}

// TestGoalToolRecordsBlockerAndProgress drives the registered tool, the surface
// the model actually calls, rather than the tracker underneath it.
func TestGoalToolRecordsBlockerAndProgress(t *testing.T) {
	root := t.TempDir()
	loaded, err := factoryload.LoadExtensionFromFactory(Extension, root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:goal>")
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := loaded.RegisteredTool("goal")
	if !ok {
		t.Fatal("the goal tool was not registered")
	}
	call := func(args string) (extension.AgentToolResult, error) {
		return definition.Definition.Execute(context.Background(), "call", json.RawMessage(args), nil)
	}
	if _, err := call(`{"op":"create","objective":"ship the fix"}`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= goal.BlockerThreshold; i++ {
		result, err := call(`{"op":"blocked","reason":"needs the staging API token"}`)
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("report %d failed: %+v", i, result.Content)
		}
	}
	if _, err := call(`{"op":"blocked","reason":"needs the staging API token"}`); err != nil {
		t.Fatal(err)
	}
	// Progress and resume are rejected on a paused goal, which keeps a late report
	// from reviving work a human stopped.
	if result, _ := call(`{"op":"progress"}`); !result.IsError {
		t.Fatal("a paused goal accepted a progress report")
	}
	if result, _ := call(`{"op":"resume"}`); result.IsError {
		t.Fatalf("resume failed: %+v", result.Content)
	}
	if result, _ := call(`{"op":"progress"}`); result.IsError {
		t.Fatalf("progress after resume failed: %+v", result.Content)
	}
	if result, _ := call(`{"op":"blocked","reason":" "}`); !result.IsError {
		t.Fatal("a blank blocker was accepted")
	}
}

// TestPauseNoticeNamesTheBlockerAndTheResumeAction pins the one notice a
// threshold pause shows, and that status reports the same explanation without
// repeating the instruction.
func TestPauseNoticeNamesTheBlockerAndTheResumeAction(t *testing.T) {
	tracker := goal.New()
	if err := tracker.Create("finish the work", 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < goal.BlockerThreshold; i++ {
		if _, err := tracker.Blocked("needs the staging API token"); err != nil {
			t.Fatal(err)
		}
	}
	notice := pausedNotice(tracker.Get())
	if !strings.Contains(notice, "needs the staging API token") {
		t.Fatalf("notice = %q, want it to name the missing input", notice)
	}
	if !strings.Contains(notice, "/goal resume") {
		t.Fatalf("notice = %q, want the explicit resume action", notice)
	}
	// A status read reports the blocker and its count, never the instruction again.
	if rendered := render(tracker.Get()); strings.Contains(rendered, "/goal resume") {
		t.Fatalf("status repeated the pause notice:\n%s", rendered)
	} else if !strings.Contains(rendered, "reported 3×") {
		t.Fatalf("status lost the blocker count:\n%s", rendered)
	}
	// The blocker text outlives the streak, so a resumed goal still explains what
	// had stopped it while the count restarts.
	if _, err := tracker.Resume(); err != nil {
		t.Fatal(err)
	}
	rendered := render(tracker.Get())
	if !strings.Contains(rendered, "needs the staging API token") {
		t.Fatalf("the resumed status lost the blocker it explains:\n%s", rendered)
	}
	if strings.Contains(rendered, "reported") {
		t.Fatalf("resume did not clear the count:\n%s", rendered)
	}
}

// TestToolShowsThePauseNoticeOnce pins where the notice belongs: the transition
// that paused the goal returns it, and a later read does not.
func TestToolShowsThePauseNoticeOnce(t *testing.T) {
	root := t.TempDir()
	loaded, err := factoryload.LoadExtensionFromFactory(Extension, root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:goal>")
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := loaded.RegisteredTool("goal")
	call := func(args string) string {
		t.Helper()
		result, err := definition.Definition.Execute(context.Background(), "call", json.RawMessage(args), nil)
		if err != nil {
			t.Fatal(err)
		}
		return result.Content[0].(ai.TextContent).Text
	}
	call(`{"op":"create","objective":"ship the fix"}`)
	for i := 1; i < goal.BlockerThreshold; i++ {
		if text := call(`{"op":"blocked","reason":"needs the staging API token"}`); strings.Contains(text, "/goal resume") {
			t.Fatalf("report %d showed the pause notice early:\n%s", i, text)
		}
	}
	paused := call(`{"op":"blocked","reason":"needs the staging API token"}`)
	if !strings.Contains(paused, "/goal resume") {
		t.Fatalf("the paused report did not show the notice:\n%s", paused)
	}
	if reread := call(`{"op":"get"}`); strings.Contains(reread, "/goal resume") {
		t.Fatalf("a status read repeated the pause notice:\n%s", reread)
	}
}

// TestGoalSubmitsSkillObjective exercises the real command runner. Goal must
// forward slash skill syntax to the host's normal prompt/resource expansion.
func TestGoalSubmitsSkillObjective(t *testing.T) {
	for _, args := range []string{"/skill:review fix this", "set /skill:review fix this", "create /skill:review fix this"} {
		t.Run(args, func(t *testing.T) {
			root := t.TempDir()
			loaded, err := factoryload.LoadExtensionFromFactory(Extension, root, extension.CreateEventBus(), extension.CreateExtensionRuntime(), "<inline:goal>")
			if err != nil {
				t.Fatal(err)
			}
			runner := inproc.NewRunner([]extension.Extension{loaded}, root)
			var submitted any
			var expand *bool
			runner.BindCore(extension.ExtensionActions{SendUserMessage: func(content any, opts *extension.SendUserMessageOptions) error {
				submitted = content
				if opts != nil {
					expand = opts.ExpandPromptTemplates
				}
				return nil
			}}, extension.ContextActions{}, nil)
			if !runner.ExecuteCommand(context.Background(), "goal", args) {
				t.Fatal("goal command not handled")
			}
			if submitted != "/skill:review fix this" {
				t.Fatalf("submitted = %q; want skill objective", submitted)
			}
			// A goal objective is a prompt, so the host must run the same
			// skill/template expansion a typed prompt gets. Without the explicit
			// request, an extension-originated message defaults to no expansion
			// and a `/skill:` objective reaches the model as literal text.
			if expand == nil || !*expand {
				t.Fatalf("ExpandPromptTemplates = %v; want an explicit true", expand)
			}
		})
	}
}

// TestGoalCompletionsOfferEveryControlWord pins the completion list to the parser's vocabulary, so
// a verb the handler accepts cannot be invisible in the picker, and a suggestion cannot parse as a
// prompt.
func TestGoalCompletionsOfferEveryControlWord(t *testing.T) {
	items, err := goalCompletions("")
	if err != nil {
		t.Fatal(err)
	}
	offered := make(map[string]bool, len(items))
	for _, item := range items {
		offered[item.Value] = true
		command, err := parseGoalCommand(item.Value)
		if err != nil {
			// Only a verb that requires an objective may refuse a bare invocation.
			if goalControlWords[item.Value] != goalOpSet {
				t.Fatalf("suggestion %q does not parse: %v", item.Value, err)
			}
			continue
		}
		if command.op == goalOpPrompt {
			t.Fatalf("suggestion %q parses as a prompt instead of a control word", item.Value)
		}
	}
	for word := range goalControlWords {
		if !offered[word] {
			t.Errorf("control word %q is missing from the completion list", word)
		}
	}
	if len(offered) != len(goalControlWords) {
		t.Fatalf("completions offer %d words, vocabulary has %d", len(offered), len(goalControlWords))
	}
	filtered, err := goalCompletions("RE")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Value != "resume" {
		t.Fatalf("prefix match = %+v, want resume only", filtered)
	}
	if spaced, err := goalCompletions("set a goal"); err != nil || spaced != nil {
		t.Fatalf("an argument position offered completions: %+v %v", spaced, err)
	}
}
