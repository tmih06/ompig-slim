package todotool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/coding/extension"

	"ompig-slim/internal/todo"
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

// SettleExtension wires the stop-time reminder.
//
// This is the behavior the todo tool exists for: without it, a tool that merely
// records a list is a notebook, and the agent still ends a turn with open work.
// The handler runs on the agent_before_settle boundary — the last point where the
// harness can still turn a would-be stop into another turn — and returns
// Continue when the list proves the agent is not done.
func SettleExtension() extension.ExtensionFactory {
	return func(api extension.API) error {
		state := &reminderState{}
		api.OnAgentBeforeSettle(func(ctx context.Context, evt *extension.AgentBeforeSettleEvent) (extension.AgentBeforeSettleEventResult, error) {
			return state.check(ctx, api, evt)
		})
		return nil
	}
}

// check decides whether to continue the run for unfinished todos.
//
// It is deliberately conservative. Every path that cannot justify another turn
// returns without continuing, because a wrong continue is worse than a missed
// one: it burns the user's tokens and can restart work they wanted stopped.
func (s *reminderState) check(ctx context.Context, api extension.API, evt *extension.AgentBeforeSettleEvent) (extension.AgentBeforeSettleEventResult, error) {
	// An aborted or failed run was not the agent deciding it was finished.
	if evt == nil || evt.Outcome != extension.AgentActivityCompleted {
		return extension.AgentBeforeSettleEventResult{}, nil
	}
	extCtx := extension.FromContext(ctx)
	if extCtx == nil {
		return extension.AgentBeforeSettleEventResult{}, nil
	}
	phases := latestPhases(branchOf(extCtx))
	if len(phases) == 0 {
		// No todo list: this feature is opt-in by use, and an agent that never
		// created one is judged by its own output, not by a list it never had.
		return extension.AgentBeforeSettleEventResult{}, nil
	}
	open := todo.OpenTasks(phases)
	if !s.shouldRemind(phases) {
		if len(open) == 0 {
			// Either everything settled, or the only open work is blocked. A blocked
			// task is waiting on something the agent cannot supply — the user, another
			// agent, a service — so continuing would spin. This is also the escape
			// hatch that keeps the reminder from fighting a legitimate stop.
			s.reset()
		}
		return extension.AgentBeforeSettleEventResult{}, nil
	}
	if answeredUser(ctx) {
		// The agent asked the user something in its final message. A question is a
		// legitimate stop even with work open: it is the mechanism the reminder
		// itself directs the agent to use when it is stuck. Continuing here would
		// talk over the user.
		return extension.AgentBeforeSettleEventResult{}, nil
	}

	s.recordSent(phases)

	reminder := renderReminder(phases, open, s.sent)
	// A custom message with display false: the model must see it, the user should
	// not have the transcript filled with reminders.
	api.SendMessage(extension.SendMessagePayload{
		CustomType: reminderCustomType,
		Content:    reminder,
		Display:    false,
	}, &extension.SendMessageOptions{
		TriggerTurn: ptr(true),
		DeliverAs:   extension.DeliverAsFollowUp,
	})

	cont := true
	return extension.AgentBeforeSettleEventResult{Continue: &cont}, nil
}

// shouldRemind reports whether the run should continue for the given list, and
// is where every deliberate "do not continue" decision lives.
//
// It is separate from check so the policy can be tested without a live session:
// each guard here is a case where continuing would be wrong, and each is cheap to
// regress silently.
func (s *reminderState) shouldRemind(phases []todo.Phase) bool {
	if len(phases) == 0 {
		// No todo list: this feature is opt-in by use, and an agent that never
		// created one is judged by its own output, not by a list it never had.
		return false
	}
	if len(todo.OpenTasks(phases)) == 0 {
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
	return todo.Fingerprint(phases) != s.fingerprint
}

// recordSent spends one reminder on the given list, recording it so an unchanged
// list is not nudged twice.
func (s *reminderState) recordSent(phases []todo.Phase) {
	s.sent++
	s.fingerprint = todo.Fingerprint(phases)
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
func renderReminder(phases []todo.Phase, open []todo.Task, attempt int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<system-reminder>\nYou stopped with %d open todo item(s):\n", len(open))
	for _, phase := range phases {
		line := ""
		for _, task := range phase.Tasks {
			if task.Status.Settled() || task.Status == todo.StatusBlocked {
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

// answeredUser reports whether the agent's last message asked the user something.
//
// Only the tail of the message is inspected. A question anywhere in a long report
// is usually rhetorical or already answered in the same message, whereas a final
// question is the agent handing control back on purpose. The check is on the last
// non-empty line, which is where a model puts a closing question.
func answeredUser(ctx context.Context) bool {
	extCtx := extension.FromContext(ctx)
	if extCtx == nil {
		return false
	}
	branch := branchOf(extCtx)
	for i := len(branch) - 1; i >= 0; i-- {
		text, ok := lastAssistantText(branch[i])
		if !ok {
			continue
		}
		return endsWithQuestion(text)
	}
	return false
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
func branchOf(extCtx *extension.Context) []extension.SessionEntry {
	manager, err := extCtx.SessionManager()
	if err != nil || manager == nil {
		return nil
	}
	return manager.GetBranch()
}

// ptr returns a pointer to v, for the optional members of the API's option
// structs where the presence of the value carries meaning.
func ptr[T any](v T) *T { return &v }
