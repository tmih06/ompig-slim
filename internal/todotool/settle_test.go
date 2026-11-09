package todotool

import (
	"testing"

	"ompig-slim/internal/todo"
)

// TestEndsWithQuestion pins the guard that keeps the reminder from talking over
// the user. The reminder exists to stop a silent stall; a deliberate hand-off must
// survive it, because that is the escape hatch the reminder itself recommends.
func TestEndsWithQuestion(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"plain statement", "Committed as abc1234.", false},
		{"closing question", "Implemented the core port.\n\nWhich preset should I match?", true},
		{"hand-off without a question mark", "Fixed the frame.\nLet me know if the padding looks right.", true},
		{"question then a closing note", "Should I also port presets?\n\nNo changes were made.", true},
		{"trailing blank lines", "Done.\n\nWant me to commit it?\n\n", true},
		{"report with a question in the middle", "Why is this slow? Because the loop copies.\n\nFixed by reusing the buffer.", false},
		{"empty", "", false},
		{"whitespace only", "  \n\t\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := endsWithQuestion(tc.text); got != tc.want {
				t.Fatalf("endsWithQuestion(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestRenderReminder checks that the nudge names the work it is asking for. A
// reminder that just says "keep going" can be satisfied by a summary; one that
// lists the open tasks and their count cannot.
func TestRenderReminder(t *testing.T) {
	phases := []todo.Phase{{
		Name: "Port",
		Tasks: []todo.Task{
			{Content: "read OMP source", Status: todo.StatusCompleted},
			{Content: "port the tool", Status: todo.StatusInProgress},
			{Content: "port the reminder", Status: todo.StatusPending},
			{Content: "wait for user", Status: todo.StatusBlocked, Blocker: "needs a decision"},
		},
	}}
	out := renderReminder(phases, todo.OpenTasks(phases), 1)

	for _, want := range []string{"2 open todo item(s)", "port the tool", "port the reminder", "Reminder 1/3"} {
		if !contains(out, want) {
			t.Fatalf("reminder should contain %q, got:\n%s", want, out)
		}
	}
	// Settled and blocked work must not appear: the agent cannot act on either,
	// and listing them would ask for work the reminder itself calls done/waiting.
	for _, unwanted := range []string{"read OMP source", "wait for user", "needs a decision"} {
		if contains(out, unwanted) {
			t.Fatalf("reminder should not mention %q, got:\n%s", unwanted, out)
		}
	}
}

// TestCheckGuards pins every reason the reminder stays silent. Each guard is a
// deliberate "do not continue" decision, and a regression in any of them turns a
// helpful nudge into a loop that burns the user's budget.
func TestCheckGuards(t *testing.T) {
	open := []todo.Phase{{Name: "P", Tasks: []todo.Task{{Content: "a", Status: todo.StatusPending}}}}
	settled := []todo.Phase{{Name: "P", Tasks: []todo.Task{{Content: "a", Status: todo.StatusCompleted}}}}
	blockedOnly := []todo.Phase{{Name: "P", Tasks: []todo.Task{{Content: "a", Status: todo.StatusBlocked}}}}

	t.Run("no list means no reminder", func(t *testing.T) {
		st := &reminderState{}
		if st.shouldRemind(nil) {
			t.Fatal("a session without a todo list must not be nudged")
		}
	})

	t.Run("a settled list needs no reminder", func(t *testing.T) {
		st := &reminderState{}
		if st.shouldRemind(settled) {
			t.Fatal("a finished list must not be nudged")
		}
	})

	t.Run("blocked-only work needs no reminder", func(t *testing.T) {
		st := &reminderState{}
		if st.shouldRemind(blockedOnly) {
			t.Fatal("work waiting on someone else must not be nudged")
		}
	})

	t.Run("open work is reminded once per list state", func(t *testing.T) {
		st := &reminderState{}
		if !st.shouldRemind(open) {
			t.Fatal("open work should be nudged")
		}
		st.recordSent(open)
		if st.shouldRemind(open) {
			t.Fatal("the same unchanged list must not be nudged twice")
		}
	})

	t.Run("progress earns a fresh reminder", func(t *testing.T) {
		st := &reminderState{}
		st.recordSent(open)
		progressed := []todo.Phase{{Name: "P", Tasks: []todo.Task{
			{Content: "a", Status: todo.StatusCompleted},
			{Content: "b", Status: todo.StatusPending},
		}}}
		if !st.shouldRemind(progressed) {
			t.Fatal("a changed list with open work should be nudged")
		}
	})

	t.Run("the budget is bounded", func(t *testing.T) {
		st := &reminderState{}
		// Each iteration makes progress, so the fingerprint guard never fires and
		// only the count can stop it.
		for i := 0; i < maxReminders; i++ {
			list := []todo.Phase{{Name: "P", Tasks: []todo.Task{{Content: string(rune('a' + i)), Status: todo.StatusPending}}}}
			if !st.shouldRemind(list) {
				t.Fatalf("reminder %d should be allowed", i+1)
			}
			st.recordSent(list)
		}
		if st.shouldRemind(open) {
			t.Fatalf("no reminder may exceed the budget of %d", maxReminders)
		}
	})

	t.Run("reaching a consistent state refills the budget", func(t *testing.T) {
		st := &reminderState{}
		st.sent = maxReminders
		st.fingerprint = "stale"
		st.reset()
		if !st.shouldRemind(open) {
			t.Fatal("a reset should allow a reminder again")
		}
	})
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
