package goalext

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ompig-slim/internal/goal"
)

// TestInterruptDuringContinuationStartsNoFurtherTurn covers the interruption box
// of #76: a control that lands while a goal continuation turn is running leaves
// the goal in the human's chosen state and starts no further autonomous turn.
func TestInterruptDuringContinuationStartsNoFurtherTurn(t *testing.T) {
	for _, control := range []struct {
		name string
		act  func(g *goalSession)
		want string
	}{
		{"escape", func(g *goalSession) { g.session.RequestAbort() }, "paused"},
		{"pause", func(g *goalSession) { steer(t, g, "/goal pause") }, "paused"},
		{"complete", func(g *goalSession) { steer(t, g, "/goal complete") }, "complete"},
		{"drop", func(g *goalSession) { steer(t, g, "/goal drop") }, ""},
	} {
		t.Run(control.name, func(t *testing.T) {
			g := newGoalSession(t)
			go func() { _ = g.session.Prompt(g.ctx, "/goal set finish the fixture") }()
			g.model.answer(5 * time.Second)                // goal turn 1 settles, queues poke 1
			g.model.answerNoRelease(5 * time.Second)       // poke 1 is now in flight
			control.act(g)
			g.model.releaseOne()
			pokesAtInterrupt := countPokes(g.model.snapshot())

			// Any surviving continuation gets every chance to start a turn.
			g.model.drain(1200*time.Millisecond, 3)
			if got := countPokes(g.model.snapshot()); got != pokesAtInterrupt {
				t.Fatalf("%s started %d more continuation turns", control.name, got-pokesAtInterrupt)
			}
			if got := string(g.waitStatus(t, goalStatus(control.want), 3*time.Second)); got != control.want {
				t.Fatalf("%s left goal status %q, want %q", control.name, got, control.want)
			}
		})
	}
}

// TestFollowUpAfterInterruptDoesNotResume covers the resumed-objective box: an
// ordinary question after interruption runs on its own text, without the paused
// goal's poke or objective.
func TestFollowUpAfterInterruptDoesNotResume(t *testing.T) {
	g := newGoalSession(t)
	turn := g.run("/goal set finish the fixture")
	g.model.answerNoRelease(5 * time.Second)
	g.session.RequestAbort()
	g.model.releaseOne()
	g.wait(t, turn)
	if got := string(g.waitStatus(t, goal.StatusPaused, 3*time.Second)); got != "paused" {
		t.Fatalf("Escape left goal status %q, want paused", got)
	}

	question := g.run("what is the status")
	g.model.answer(5 * time.Second)
	g.wait(t, question)
	g.model.drain(400*time.Millisecond, 2)

	// The follow-up's own turn is its trailing messages: the question last, and
	// nothing goal-originated after the paused goal's own transcript.
	messages := g.model.lastMessages()
	final := string(messages[len(messages)-1])
	if !strings.Contains(final, "what is the status") {
		t.Fatalf("follow-up did not reach the model as its own text: %s", excerpt(final))
	}
	trailing := string(joinMessages(messages[len(messages)-2:]))
	if strings.Contains(trailing, "Goal poke") {
		t.Fatalf("follow-up carried a goal continuation: %s", excerpt(trailing))
	}
	if got := string(g.status(t)); got != "paused" {
		t.Fatalf("follow-up changed goal status to %q, want paused", got)
	}
}

// TestReplacementGoalDoesNotInheritContinuation covers the replacement box: a
// new goal must not run as a continuation of the previous objective.
func TestReplacementGoalDoesNotInheritContinuation(t *testing.T) {
	g := newGoalSession(t)
	firstTurn := g.run("/goal set first objective")
	g.model.answerNoRelease(5 * time.Second)
	g.session.RequestAbort()
	g.model.releaseOne()
	g.wait(t, firstTurn)
	g.waitStatus(t, goal.StatusPaused, 3*time.Second)

	steer(t, g, "/goal drop")
	before := len(g.model.snapshot())
	second := g.run("/goal set second objective")
	g.model.answer(5 * time.Second)
	g.wait(t, second)
	g.model.drain(1200*time.Millisecond, 3)

	// Only the replacement goal's own turns matter: earlier requests are the
	// first goal's legitimate transcript.
	for _, conversation := range g.model.messages()[before:] {
		if text := string(joinMessages(conversation)); !strings.Contains(text, "second objective") {
			t.Fatalf("replacement request never carried its own objective: %s", excerpt(text))
		}
	}
	if got := string(g.waitStatus(t, goal.StatusActive, 3*time.Second)); got != "active" {
		t.Fatalf("replacement goal status = %q, want active", got)
	}
}

// TestExplicitResumeRestartsOnce covers the resume box: resume continues the
// same objective, and a repeated pause/resume cycle adds no duplicate turn.
func TestExplicitResumeRestartsOnce(t *testing.T) {
	g := newGoalSession(t)
	firstTurn := g.run("/goal set finish the fixture")
	g.model.answerNoRelease(5 * time.Second)
	g.session.RequestAbort()
	g.model.releaseOne()
	g.wait(t, firstTurn)
	g.waitStatus(t, goal.StatusPaused, 3*time.Second)
	before := len(g.model.snapshot())

	// Resume restarts the objective once. The goal then continues normally, so
	// the restart is the only turn that carries the objective as a plain prompt.
	resume := g.run("/goal resume")
	g.model.answer(5 * time.Second)
	g.wait(t, resume)
	g.model.drain(600*time.Millisecond, 2)

	restarts := 0
	for _, conversation := range g.model.messages()[before:] {
		if isRestart(string(conversation[len(conversation)-1])) {
			restarts++
		}
	}
	if restarts != 1 {
		t.Fatalf("resume restarted the objective %d times, want 1", restarts)
	}
	if got := string(g.waitStatus(t, goal.StatusActive, 3*time.Second)); got != "active" {
		t.Fatalf("resumed goal status = %q, want active", got)
	}

	// A resume of the already-active goal adds no restart and no duplicate turn.
	pokesBefore := countPokes(g.model.snapshot())
	steer(t, g, "/goal resume")
	g.model.drain(500*time.Millisecond, 1)
	if got := countPokes(g.model.snapshot()); got > pokesBefore+1 {
		t.Fatalf("a repeated resume duplicated continuation turns: %d -> %d", pokesBefore, got)
	}
}

// TestActiveGoalKeepsContinuing is the control: interrupt handling must leave
// normal continuation of an untouched goal alone.
func TestActiveGoalKeepsContinuing(t *testing.T) {
	g := newGoalSession(t)
	go func() { _ = g.session.Prompt(g.ctx, "/goal set finish the fixture") }()
	for i := 0; i < 4; i++ {
		g.model.answer(5 * time.Second)
	}
	if got := countPokes(g.model.snapshot()); got < 3 {
		t.Fatalf("active goal continued %d times over 3 model turns", got)
	}
	if got := string(g.status(t)); got != "active" {
		t.Fatalf("uninterrupted goal status = %q, want active", got)
	}
	g.session.RequestAbort()
}

// isRestart reports whether a request's final message is the goal objective
// sent as an ordinary user prompt, which is how a resumed goal starts again. A
// continuation carries the poke prefix instead.
func isRestart(message string) bool {
	var decoded struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(message), &decoded); err != nil || decoded.Role != "user" || len(decoded.Content) == 0 {
		return false
	}
	text := decoded.Content[0].Text
	return text == "finish the fixture"
}

// goalStatus maps an expected status name onto the tracker constant.
func goalStatus(name string) goal.Status {
	return goal.Status(name)
}

// steer delivers an extension command while the run is active, as the TUI does
// when a slash command is typed during work.
func steer(t *testing.T, g *goalSession, prompt string) {
	t.Helper()
	if err := g.session.Prompt(g.ctx, prompt, codingPromptSteer()); err != nil {
		t.Fatalf("%s: %v", prompt, err)
	}
}

// joinMessages concatenates message bodies for a content assertion.
func joinMessages(messages []json.RawMessage) []byte {
	joined := make([]byte, 0, 1024)
	for _, message := range messages {
		joined = append(joined, message...)
	}
	return joined
}

func excerpt(request string) string {
	const limit = 400
	if len(request) <= limit {
		return request
	}
	return request[len(request)-limit:]
}
