package todo

import (
	"strings"
	"testing"
)

// TestApplyOps covers each ported op's documented effect. The cases are the ones
// whose behavior the reminder depends on: a task must only leave the open set
// when it settles, and blocked work must stay visible without becoming active.
func TestApplyOps(t *testing.T) {
	t.Run("init replaces the list and starts the first task", func(t *testing.T) {
		phases, errs := Apply(nil, Params{Op: OpInit, List: &[]InitRow{
			{Phase: "P1", Items: []string{"a", "b"}},
		}})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if got := phases[0].Tasks[0].Status; got != StatusInProgress {
			t.Fatalf("first task should auto-start, got %q", got)
		}
		if got := phases[0].Tasks[1].Status; got != StatusPending {
			t.Fatalf("second task should stay pending, got %q", got)
		}
	})

	t.Run("flat items init synthesizes the default phase", func(t *testing.T) {
		phases, errs := Apply(nil, Params{Op: OpInit, Items: []string{"a"}})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if phases[0].Name != defaultInitPhase {
			t.Fatalf("expected phase %q, got %q", defaultInitPhase, phases[0].Name)
		}
	})

	t.Run("duplicate task in init is rejected", func(t *testing.T) {
		_, errs := Apply(nil, Params{Op: OpInit, Items: []string{"a", "a"}})
		if len(errs) == 0 {
			t.Fatal("expected a duplicate-task error")
		}
	})

	t.Run("start demotes the previous active task", func(t *testing.T) {
		phases := seed(t, "a", "b")
		phases, errs := Apply(phases, Params{Op: OpStart, Task: "b"})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if phases[0].Tasks[0].Status != StatusPending {
			t.Fatalf("previous active task should demote to pending, got %q", phases[0].Tasks[0].Status)
		}
		if phases[0].Tasks[1].Status != StatusInProgress {
			t.Fatalf("target should be in progress, got %q", phases[0].Tasks[1].Status)
		}
	})

	t.Run("done advances the active pointer to the next pending task", func(t *testing.T) {
		phases := seed(t, "a", "b")
		phases, errs := Apply(phases, Params{Op: OpDone, Task: "a"})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if phases[0].Tasks[0].Status != StatusCompleted {
			t.Fatalf("target should be completed, got %q", phases[0].Tasks[0].Status)
		}
		if phases[0].Tasks[1].Status != StatusInProgress {
			t.Fatalf("next task should auto-start, got %q", phases[0].Tasks[1].Status)
		}
	})

	t.Run("block keeps a task open but never active", func(t *testing.T) {
		phases := seed(t, "a", "b")
		phases, errs := Apply(phases, Params{Op: OpBlock, Task: "a", Reason: "waiting on user"})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if phases[0].Tasks[0].Status != StatusBlocked {
			t.Fatalf("target should be blocked, got %q", phases[0].Tasks[0].Status)
		}
		if phases[0].Tasks[0].Blocker != "waiting on user" {
			t.Fatalf("reason should be recorded, got %q", phases[0].Tasks[0].Blocker)
		}
		// The blocked task is not open work: OpenTasks must skip it, or the
		// reminder would demand progress on work the agent cannot do.
		if open := OpenTasks(phases); len(open) != 1 || open[0].Content != "b" {
			t.Fatalf("only the unblocked task should be open, got %v", open)
		}
	})

	t.Run("block normalizes a multi-line reason to one line", func(t *testing.T) {
		phases := seed(t, "a")
		phases, _ = Apply(phases, Params{Op: OpBlock, Task: "a", Reason: "line one\n\n  line two"})
		if got := phases[0].Tasks[0].Blocker; got != "line one line two" {
			t.Fatalf("reason should collapse whitespace, got %q", got)
		}
	})

	t.Run("block does not reopen a settled task", func(t *testing.T) {
		phases := seed(t, "a")
		phases, _ = Apply(phases, Params{Op: OpDone, Task: "a"})
		phases, _ = Apply(phases, Params{Op: OpBlock, Phase: "Tasks"})
		if phases[0].Tasks[0].Status != StatusCompleted {
			t.Fatalf("completed task must stay completed, got %q", phases[0].Tasks[0].Status)
		}
	})

	t.Run("unblock returns the task to pending work", func(t *testing.T) {
		phases := seed(t, "a")
		phases, _ = Apply(phases, Params{Op: OpBlock, Task: "a", Reason: "r"})
		phases, errs := Apply(phases, Params{Op: OpUnblock, Task: "a"})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		// Unblocking clears the block, and normalization then makes the task the
		// active one again because it is the only pending task left.
		if phases[0].Tasks[0].Status != StatusInProgress {
			t.Fatalf("expected the task to become active again, got %q", phases[0].Tasks[0].Status)
		}
		if phases[0].Tasks[0].Blocker != "" {
			t.Fatalf("blocker should clear, got %q", phases[0].Tasks[0].Blocker)
		}
	})

	t.Run("append creates a missing phase", func(t *testing.T) {
		phases := seed(t, "a")
		phases, errs := Apply(phases, Params{Op: OpAppend, Phase: "Later", Items: []string{"c"}})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(phases) != 2 || phases[1].Name != "Later" {
			t.Fatalf("expected a new phase, got %+v", phases)
		}
	})

	t.Run("append rejects a task that already exists", func(t *testing.T) {
		phases := seed(t, "a")
		_, errs := Apply(phases, Params{Op: OpAppend, Phase: "Tasks", Items: []string{"a"}})
		if len(errs) == 0 {
			t.Fatal("expected an already-exists error")
		}
	})

	t.Run("unknown operation is an error", func(t *testing.T) {
		_, errs := Apply(seed(t, "a"), Params{Op: "bogus"})
		if len(errs) == 0 {
			t.Fatal("expected an unknown-operation error")
		}
	})
}

// TestFindTaskHints guards the error text a model reads when it addresses a task
// wrongly. Getting this right matters: the model retries immediately, and a hint
// that says what to send instead ends the loop in one turn.
func TestFindTaskHints(t *testing.T) {
	t.Run("generated ids are called out", func(t *testing.T) {
		_, errs := Apply(seed(t, "a"), Params{Op: OpDone, Task: "task-1"})
		if len(errs) != 1 || !strings.Contains(errs[0], "full text, not by ids") {
			t.Fatalf("expected an addressing hint, got %v", errs)
		}
	})

	t.Run("an empty list is explained", func(t *testing.T) {
		_, errs := Apply(nil, Params{Op: OpDone, Task: "a"})
		if len(errs) != 1 || !strings.Contains(errs[0], "todo list is empty") {
			t.Fatalf("expected an empty-list hint, got %v", errs)
		}
	})
}

// TestSummary checks the result text the model reads: settled work must be
// distinguishable from dropped work, or the model cannot tell what it actually
// finished.
func TestSummary(t *testing.T) {
	t.Run("dropped tasks are not rendered as completed", func(t *testing.T) {
		phases := seed(t, "a", "b")
		phases, _ = Apply(phases, Params{Op: OpDone, Task: "a"})
		phases, _ = Apply(phases, Params{Op: OpDrop, Task: "b"})
		out := Summary(phases, nil, false)
		if !strings.Contains(out, "[X] a") {
			t.Fatalf("completed task should render [X], got:\n%s", out)
		}
		if !strings.Contains(out, "[-] b") {
			t.Fatalf("dropped task should render [-], got:\n%s", out)
		}
	})

	t.Run("a fully settled list reports no remaining items", func(t *testing.T) {
		phases := seed(t, "a")
		phases, _ = Apply(phases, Params{Op: OpDone, Task: "a"})
		if out := Summary(phases, nil, false); !strings.Contains(out, "Remaining items: none.") {
			t.Fatalf("expected no remaining items, got:\n%s", out)
		}
	})

	t.Run("a blocked task counts as blocked, not open", func(t *testing.T) {
		phases := seed(t, "a")
		phases, _ = Apply(phases, Params{Op: OpBlock, Task: "a", Reason: "r"})
		out := Summary(phases, nil, false)
		if !strings.Contains(out, "1 blocked") {
			t.Fatalf("expected a blocked count, got:\n%s", out)
		}
	})

	t.Run("errors are reported first", func(t *testing.T) {
		out := Summary(seed(t, "a"), []string{"boom"}, false)
		if !strings.HasPrefix(out, "Errors: boom") {
			t.Fatalf("expected the error first, got:\n%s", out)
		}
	})
}

// TestOpenAndUnsettled pins the two views the reminder switches on. The
// distinction is the whole reason a blocked task does not spin the loop: open
// work means "continue", blocked-only work means "stop".
func TestOpenAndUnsettled(t *testing.T) {
	phases := seed(t, "a", "b")
	phases, _ = Apply(phases, Params{Op: OpBlock, Task: "a", Reason: "r"})
	phases, _ = Apply(phases, Params{Op: OpDone, Task: "b"})

	if open := OpenTasks(phases); len(open) != 0 {
		t.Fatalf("expected no open work, got %v", open)
	}
	if unsettled := UnsettledTasks(phases); len(unsettled) != 1 || unsettled[0].Content != "a" {
		t.Fatalf("expected the blocked task to stay unsettled, got %v", unsettled)
	}
}

// TestClonePhasesIsDeep guards the copy the tool relies on to compute a candidate
// list without mutating the snapshot it read, so a rejected call reports the
// previous list rather than a half-applied one.
func TestClonePhasesIsDeep(t *testing.T) {
	original := seed(t, "a")
	copied := ClonePhases(original)
	copied[0].Tasks[0].Status = StatusCompleted
	if original[0].Tasks[0].Status == StatusCompleted {
		t.Fatal("clone shares task storage with the original")
	}
}

// TestCompletedTransitions checks the per-call progress report, which exists so a
// renderer does not have to diff snapshots.
func TestCompletedTransitions(t *testing.T) {
	before := seed(t, "a", "b")
	after := ClonePhases(before)
	after[0].Tasks[0].Status = StatusCompleted
	got := CompletedTransitions(before, after)
	if len(got) != 1 || got[0].Content != "a" {
		t.Fatalf("expected only task a to transition, got %v", got)
	}
}

// seed builds a single-phase list and lets Apply establish the active pointer, so
// tests assert against the same normalization the tool produces.
func seed(t *testing.T, contents ...string) []Phase {
	t.Helper()
	phases, errs := Apply(nil, Params{Op: OpInit, List: &[]InitRow{{Phase: "Tasks", Items: contents}}})
	if len(errs) > 0 {
		t.Fatalf("seeding the list failed: %v", errs)
	}
	return phases
}
