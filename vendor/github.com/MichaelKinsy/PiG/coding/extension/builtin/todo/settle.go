package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
)

// reminderCustomType names the custom message the reminder appends, so a
// renderer can style it and a transcript can be searched for it.
const reminderCustomType = "todo-reminder"

// maxReminders is how many times one run may be continued for unfinished todos
// before the harness gives up and yields to the user.
//
// OMP uses 3. The cap matters because the reminder is a judgement call on model
// output: an agent that genuinely cannot proceed (a broken environment, a task
// it cannot understand) would otherwise be restarted forever. Three attempts
// gives a mis-stopped agent two more chances after the first nudge while bounding
// the wasted work to a few turns.
const maxReminders = 3

// reminderState is the budget of one run. It resets whenever the agent makes
// progress, so a long task earns a fresh reminder per stall instead of spending a
// session-wide allowance on the first one.
type reminderState struct {
	// sent is how many reminders this run has injected.
	sent int
	// fingerprint is the todo list as of the last reminder. A reminder already
	// sent for this exact list would repeat itself, so it is not sent again until
	// the list changes.
	fingerprint string
}

// reminderState is the budget of one run. Package-level, because the hook has no
// per-extension object to hang it on: one process runs one session at a time here,
// and the state guards against repeating an identical nudge within a run.
var reminder reminderState

// registerSettleHook wires the stop-time reminder.
//
// This is the behavior the todo tool exists for: without it, a tool that merely
// records a list is a notebook, and the agent still ends a turn with open work.
// The hook runs on the agent_before_settle boundary — the last point where the
// harness can still turn a would-be stop into another turn — and reports whether to
// continue when the list proves the agent is not done.
//
// It is a native hook, not an extension: the reminder is machinery the harness
// needs, not a capability the user installed, so it never appears in the startup
// Extensions list and is unaffected by --no-extensions.
func registerSettleHook() {
	codingagent.RegisterNativeSettleHook(func(ctx context.Context, state codingagent.NativeSettleState) bool {
		return reminder.check(state)
	})
}

// check decides whether to continue the run for unfinished todos.
//
// It is deliberately conservative. Every path that cannot justify another turn
// returns false, because a wrong continue is worse than a missed one: it burns the
// user's tokens and can restart work they wanted stopped.
func (s *reminderState) check(state codingagent.NativeSettleState) bool {
	// An aborted or failed run was not the agent deciding it was finished.
	if state.Outcome != string(extension.AgentActivityCompleted) {
		return false
	}
	phases := latestPhases(state.Branch)
	if len(phases) == 0 {
		// No todo list: this feature is opt-in by use, and an agent that never
		// created one is judged by its own output, not by a list it never had.
		return false
	}
	return s.remind(phases, state.LastAssistantText, state.Deliver)
}

// remind applies the reminder policy to one list and reports whether the run
// should continue. It takes the list, the run's final assistant text and the
// delivery function instead of a session state, so the policy is testable without
// a live session: every guard here is a case where continuing would be wrong, and
// each is cheap to regress silently.
func (s *reminderState) remind(phases []Phase, lastAssistantText string, deliver func(customType, content string)) bool {
	open := OpenTasks(phases)
	if !s.shouldRemind(phases) {
		if len(open) == 0 {
			// Either everything settled, or the only open work is blocked. A blocked
			// task is waiting on something the agent cannot supply — the user, another
			// agent, a service — so continuing would spin. This is also the escape
			// hatch that keeps the reminder from fighting a legitimate stop.
			s.reset()
		}
		return false
	}
	if endsWithQuestion(lastAssistantText) {
		// The agent asked the user something in its final message. A question is a
		// legitimate stop even with work open: it is the mechanism the reminder
		// itself directs the agent to use when it is stuck. Continuing here would
		// talk over the user.
		return false
	}
	if deliver == nil {
		return false
	}

	s.recordSent(phases)
	// A hidden custom message: the model must see it, the user should not have the
	// transcript filled with reminders.
	deliver(reminderCustomType, renderReminder(phases, open, s.sent))
	return true
}

// shouldRemind reports whether the run should continue for the given list, and
// is where every deliberate "do not continue" decision lives.
//
// It is separate from check so the policy can be tested without a live session:
// each guard here is a case where continuing would be wrong, and each is cheap to
// regress silently.
func (s *reminderState) shouldRemind(phases []Phase) bool {
	if len(phases) == 0 {
		// No todo list: this feature is opt-in by use, and an agent that never
		// created one is judged by its own output, not by a list it never had.
		return false
	}
	if len(OpenTasks(phases)) == 0 {
		// Everything settled, or the only open work is blocked. Blocked work waits on
		// something the agent cannot supply, so continuing would spin.
		return false
	}
	if s.sent >= maxReminders {
		// The budget is spent. Stop nudging and let the user decide.
		return false
	}
	// A reminder already sent for this exact list would repeat itself. The agent
	// either cannot see a way forward or is ignoring the list; either way another
	// identical nudge only spends a turn.
	return Fingerprint(phases) != s.fingerprint
}

// recordSent spends one reminder on the given list, recording it so an unchanged
// list is not nudged twice.
func (s *reminderState) recordSent(phases []Phase) {
	s.sent++
	s.fingerprint = Fingerprint(phases)
}

// reset clears the budget. It runs when the agent reaches a consistent state, so
// the next stall starts from a full allowance rather than inheriting a spent one.
func (s *reminderState) reset() {
	s.sent = 0
	s.fingerprint = ""
}

// renderReminder is the model-facing nudge.
//
// It names the open work explicitly and gives one instruction. It does not
// restate the tool's contract or explain the feature: the agent already has the
// tool description, and a reminder that argues costs the same tokens as the work
// it is asking for. The count is included because a bare "keep going" is easy to
// satisfy with a summary, while "3 open items" is falsifiable.
func renderReminder(phases []Phase, open []Task, attempt int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<system-reminder>\nYou stopped with %d open todo item(s):\n", len(open))
	for _, phase := range phases {
		line := ""
		for _, task := range phase.Tasks {
			if task.Status.Settled() || task.Status == StatusBlocked {
				continue
			}
			if line == "" {
				line = fmt.Sprintf("- %s:", phase.Name)
			}
			line += fmt.Sprintf("\n  - %s", task.Content)
		}
		if line != "" {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\nContinue with these, or mark each one done, dropped or blocked with a reason. ")
	b.WriteString("Do not stop again while a task you can still work on is open.\n")
	fmt.Fprintf(&b, "(Reminder %d/%d)\n</system-reminder>", attempt, maxReminders)
	return b.String()
}

// endsWithQuestion reports whether text ends by handing control back to the user.
//
// It checks the last paragraph, then the one before it, because models routinely
// end with a one-line status note after a question ("Should I port presets?\n\nNo
// changes were made."). That note does not withdraw the question, and treating it
// as a plain stop would resume work the user was just asked to veto.
func endsWithQuestion(text string) bool {
	paragraphs := splitParagraphs(text)
	if len(paragraphs) == 0 {
		return false
	}
	last := paragraphs[len(paragraphs)-1]
	if paragraphAsks(last) {
		return true
	}
	// Only a short single-line trailing note can sit under a question without
	// being a report of its own; anything longer is the agent's real answer.
	if len(paragraphs) >= 2 && !strings.Contains(last, "\n") && paragraphAsks(paragraphs[len(paragraphs)-2]) {
		return true
	}
	return false
}

// splitParagraphs splits text on blank lines and drops empty paragraphs.
func splitParagraphs(text string) []string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		trimmed := strings.TrimSpace(block)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// paragraphAsks reports whether a paragraph's final line is a question or a cue
// that hands the decision to the user.
func paragraphAsks(paragraph string) bool {
	lines := strings.Split(paragraph, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "?") {
			return true
		}
		lower := strings.ToLower(line)
		for _, cue := range []string{
			"let me know", "tell me which", "which one", "should i", "want me to",
			"do you want", "would you like", "say the word", "your call", "pick one",
			"which would you", "confirm",
		} {
			if strings.Contains(lower, cue) {
				return true
			}
		}
		return false
	}
	return false
}

// lastAssistantText returns the text blocks of an entry that is a final
// assistant message, reporting false for any other entry.
func lastAssistantText(entry extension.SessionEntry) (string, bool) {
	if entry == nil {
		return "", false
	}
	raw := entry.Raw()
	if len(raw) == 0 {
		return "", false
	}
	var record struct {
		Message *struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &record) != nil || record.Message == nil {
		return "", false
	}
	if record.Message.Role != "assistant" {
		return "", false
	}
	var parts []string
	for _, block := range record.Message.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

// branchOf reads the session branch the reminder judges.
