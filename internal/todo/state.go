// Package todo ports oh-my-pi's `todo` tool and its stop-time completion
// reminder to ompig-slim.
//
// The port keeps OMP's data model (phases holding tasks) and its list-mutation
// rules, because the reminder's correctness depends on them: a task only stops
// a reminder when it reaches a settled status, and the single-active-task
// invariant is what makes "the agent is working on X" meaningful.
//
// State lives in the tool result's `details`, which the session log already
// stores on every call. There is no side table: resuming, rewinding, or forking
// a session re-derives the list from the branch exactly as OMP's
// getLatestTodoPhasesFromEntries does. Keeping no state is also why the tool
// needs no persistence hooks and cannot disagree with the transcript.
package todo

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Status is a task's lifecycle state. The set is OMP's TodoStatus.
type Status string

const (
	// StatusPending is open and not started.
	StatusPending Status = "pending"
	// StatusInProgress is the one task the agent claims to be working on.
	StatusInProgress Status = "in_progress"
	// StatusCompleted is finished work. The reminder treats it as settled.
	StatusCompleted Status = "completed"
	// StatusAbandoned is work deliberately dropped. Also settled: the user
	// agreed it will not be done, so reminding about it would be noise.
	StatusAbandoned Status = "abandoned"
	// StatusBlocked is open work waiting on something the agent cannot supply
	// (a user answer, another agent, a service). Deliberately NOT settled, so
	// it keeps the list open, but the reminder skips it: an agent blocked on
	// the user must stop, not spin.
	StatusBlocked Status = "blocked"
)

// Settled reports whether the status ends this task's life. A settled task
// never triggers the completion reminder.
func (s Status) Settled() bool {
	return s == StatusCompleted || s == StatusAbandoned
}

// Task is one unit of work.
type Task struct {
	Content string `json:"content"`
	Status  Status `json:"status"`
	// Blocker is the one-line reason a blocked task is waiting. Empty on every
	// other status.
	Blocker string `json:"blocker,omitempty"`
}

// Phase is a named group of tasks. Phases order the work; the reminder walks
// them in order so it can name the earliest unfinished phase.
type Phase struct {
	Name  string `json:"name"`
	Tasks []Task `json:"tasks"`
}

// Op is the discriminating operation of one call. The params object is a single
// op, as in OMP: there is no ops array, so one call applies one mutation and its
// summary reports the resulting list.
type Op string

const (
	OpInit    Op = "init"
	OpStart   Op = "start"
	OpDone    Op = "done"
	OpRemove  Op = "rm"
	OpDrop    Op = "drop"
	OpBlock   Op = "block"
	OpUnblock Op = "unblock"
	OpAppend  Op = "append"
	OpView    Op = "view"
)

// Details is the tool result's details value: the whole list after the call.
// It is the only state the tool persists, so a later call (or the reminder,
// after a resume) reconstructs the list by reading back the latest one.
type Details struct {
	// Op is the operation that produced this snapshot. Absent on entries
	// written before the field existed.
	Op Op `json:"op,omitempty"`
	// Phases is the full list after the call; unchanged when a call fails.
	Phases []Phase `json:"phases"`
	// CompletedTasks names tasks that moved to completed during this call, so
	// a renderer can highlight progress without diffing.
	CompletedTasks []CompletedTask `json:"completedTasks,omitempty"`
}

// CompletedTask is one task that became completed in a call.
type CompletedTask struct {
	Phase   string `json:"phase"`
	Content string `json:"content"`
}

// Params is one call's arguments. Every field except Op is meaningful only for
// the ops that name it; a field that a given op ignores is not an error, because
// models routinely send the whole set with unused members empty.
//
// Pointers distinguish "absent" from "present but empty", which matters:
// `list: []` is a legal way to clear the todo list, while an absent list is a
// missing argument.
type Params struct {
	Op     Op         `json:"op"`
	List   *[]InitRow `json:"list,omitempty"`
	Task   string     `json:"task,omitempty"`
	Phase  string     `json:"phase,omitempty"`
	Items  []string   `json:"items,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

// InitRow is one phase of an init payload.
type InitRow struct {
	Phase string   `json:"phase"`
	Items []string `json:"items"`
}

// defaultInitPhase names the phase synthesized for a flat init payload that
// names none. OMP uses the same name so a transcript reads the same either way.
const defaultInitPhase = "Tasks"

// ClonePhases deep-copies phases so a call can compute a candidate list without
// touching the snapshot it read, and so a failed call can report the previous
// list by keeping it rather than restoring it.
func ClonePhases(phases []Phase) []Phase {
	if phases == nil {
		return []Phase{}
	}
	out := make([]Phase, len(phases))
	for i, phase := range phases {
		out[i] = Phase{Name: phase.Name, Tasks: make([]Task, len(phase.Tasks))}
		copy(out[i].Tasks, phase.Tasks)
	}
	return out
}

// OpenTasks returns the tasks that still need action: pending or in_progress.
// Blocked tasks are excluded because an agent waiting on the user or a service
// cannot make progress on them, and a reminder that demands progress the agent
// cannot make would fight the user instead of serving them.
func OpenTasks(phases []Phase) []Task {
	var open []Task
	for _, phase := range phases {
		for _, task := range phase.Tasks {
			if task.Status == StatusPending || task.Status == StatusInProgress {
				open = append(open, task)
			}
		}
	}
	return open
}

// UnsettledTasks returns every task that has not settled, blocked ones
// included. The reminder stays silent only when this is empty: a list whose
// last open item is blocked is finished for now, but one holding a blocked item
// beside an open item is not.
func UnsettledTasks(phases []Phase) []Task {
	var unsettled []Task
	for _, phase := range phases {
		for _, task := range phase.Tasks {
			if !task.Status.Settled() {
				unsettled = append(unsettled, task)
			}
		}
	}
	return unsettled
}

// Fingerprint renders the list as a comparable string. The stop-time reminder
// uses it to tell "the agent made progress since the last reminder" from "the
// agent is ignoring the reminder", so it spends its budget on the second case.
func Fingerprint(phases []Phase) string {
	raw, err := json.Marshal(phases)
	if err != nil {
		return ""
	}
	return string(raw)
}

// Apply mutates phases by one op, collecting every problem it finds instead of
// stopping at the first. A caller discards the whole mutation when any error is
// reported, so a partially applied batch can never be persisted: the natural
// retry would otherwise collide with the half that landed.
//
// Executing an op is not the same as committing one. Read-only ops return the
// list untouched, so `view` reports what is stored instead of a normalized copy
// the transcript never accepted.
//
// phases is mutated in place and returned; callers that need the previous list
// pass a ClonePhases copy.
func Apply(phases []Phase, p Params) ([]Phase, []string) {
	var errors []string
	next := applyOp(phases, p, &errors)
	if !p.Op.readOnly() {
		normalizeInProgress(next)
	}
	return next, errors
}

// readOnly reports whether an op inspects the list without changing it. A
// read-only op must not run normalizeInProgress: normalization is a write, and
// a snapshot that never became state must not appear to have been rewritten.
func (op Op) readOnly() bool { return op == OpView }

// normalizeInProgress enforces the single-active-task invariant: at most one
// task is in_progress, and while any pending task exists exactly one of them
// (the earliest) is. Blocked tasks never become active, so a list whose only
// open work is blocked legitimately has no active task.
func normalizeInProgress(phases []Phase) {
	var active *Task
	var firstPending *Task
	for i := range phases {
		for j := range phases[i].Tasks {
			task := &phases[i].Tasks[j]
			switch task.Status {
			case StatusInProgress:
				if active == nil {
					active = task
				} else {
					// A second active task would make "what is the agent doing"
					// ambiguous and let a stale pointer survive a rewrite.
					task.Status = StatusPending
					if firstPending == nil {
						firstPending = task
					}
				}
			case StatusPending:
				if firstPending == nil {
					firstPending = task
				}
			}
		}
	}
	if active == nil && firstPending != nil {
		firstPending.Status = StatusInProgress
	}
}

// applyOp is the op switch. It reports problems through errors and must leave
// phases reconcilable with what it reports; a caller throws the result away when
// errors is non-empty.
func applyOp(phases []Phase, p Params, errors *[]string) []Phase {
	switch p.Op {
	case OpInit:
		return initPhases(p, errors)
	case OpStart:
		hit := findTask(phases, p.Task, errors)
		if hit == nil {
			return phases
		}
		// Demote every other active task so the invariant holds without relying
		// on normalizeInProgress to pick a winner.
		for i := range phases {
			for j := range phases[i].Tasks {
				if &phases[i].Tasks[j] == hit {
					continue
				}
				if phases[i].Tasks[j].Status == StatusInProgress {
					phases[i].Tasks[j].Status = StatusPending
				}
			}
		}
		hit.Status = StatusInProgress
		return phases
	case OpDone:
		for _, task := range targets(phases, p, errors) {
			task.Status = StatusCompleted
			task.Blocker = ""
		}
		return phases
	case OpDrop:
		for _, task := range targets(phases, p, errors) {
			task.Status = StatusAbandoned
			task.Blocker = ""
		}
		return phases
	case OpBlock:
		if p.Task == "" && p.Phase == "" {
			*errors = append(*errors, "block requires a task or phase target")
			return phases
		}
		reason := strings.Join(strings.Fields(p.Reason), " ")
		for _, task := range targets(phases, p, errors) {
			// Only open work can be blocked. Reopening a settled task here would
			// erase finished progress; an already-blocked task stays eligible so a
			// later call can attach or refine the reason.
			if task.Status.Settled() {
				continue
			}
			task.Status = StatusBlocked
			task.Blocker = reason
		}
		return phases
	case OpUnblock:
		if p.Task == "" && p.Phase == "" {
			*errors = append(*errors, "unblock requires a task or phase target")
			return phases
		}
		for _, task := range targets(phases, p, errors) {
			if task.Status == StatusBlocked {
				task.Status = StatusPending
				task.Blocker = ""
			}
		}
		return phases
	case OpRemove:
		return removeTasks(phases, p, errors)
	case OpAppend:
		return appendItems(phases, p, errors)
	case OpView:
		// A view is a read: no normalization, no write.
		return phases
	default:
		*errors = append(*errors, fmt.Sprintf("Unknown todo operation %q", string(p.Op)))
		return phases
	}
}

// initPhases rebuilds the list from scratch. Duplicate phase or task names are
// rejected because every targeting op resolves the first match, so a duplicate
// would be permanently unreachable.
func initPhases(p Params, errors *[]string) []Phase {
	rows := initRows(p)
	if rows == nil {
		*errors = append(*errors, "Missing list for init operation")
		return []Phase{}
	}
	seenPhases := map[string]bool{}
	seenTasks := map[string]bool{}
	out := make([]Phase, 0, len(rows))
	for _, row := range rows {
		if seenPhases[row.Phase] {
			*errors = append(*errors, fmt.Sprintf("Duplicate phase %q in init list", row.Phase))
		}
		seenPhases[row.Phase] = true
		phase := Phase{Name: row.Phase, Tasks: make([]Task, 0, len(row.Items))}
		for _, content := range row.Items {
			if seenTasks[content] {
				*errors = append(*errors, fmt.Sprintf("Duplicate task %q in init list", content))
			}
			seenTasks[content] = true
			phase.Tasks = append(phase.Tasks, Task{Content: content, Status: StatusPending})
		}
		out = append(out, phase)
	}
	return out
}

// initRows resolves the two accepted init shapes. Models routinely flatten a
// single-phase init to `items` (+ optional `phase`) instead of the canonical
// `list`, and they fill unused members with empty arrays, so a non-empty `items`
// must win over an empty `list`. Returns nil when neither shape is usable.
func initRows(p Params) []InitRow {
	if len(p.Items) > 0 {
		phase := p.Phase
		if phase == "" {
			phase = defaultInitPhase
		}
		return []InitRow{{Phase: phase, Items: p.Items}}
	}
	if p.List == nil {
		return nil
	}
	return *p.List
}

// appendItems adds tasks to a phase, creating the phase when it is missing. The
// batch is validated before any task lands so a failing call leaves nothing
// half-applied.
func appendItems(phases []Phase, p Params, errors *[]string) []Phase {
	if p.Phase == "" {
		*errors = append(*errors, "Missing phase name for append operation")
		return phases
	}
	if len(p.Items) == 0 {
		*errors = append(*errors, "Missing items for append operation")
		return phases
	}
	seen := map[string]bool{}
	duplicate := false
	for _, content := range p.Items {
		if seen[content] || findTaskByContent(phases, content) != nil {
			*errors = append(*errors, fmt.Sprintf("Task %q already exists", content))
			duplicate = true
		}
		seen[content] = true
	}
	if duplicate {
		return phases
	}
	phase := findPhase(phases, p.Phase)
	if phase == nil {
		phases = append(phases, Phase{Name: p.Phase})
		phase = &phases[len(phases)-1]
	}
	for _, content := range p.Items {
		phase.Tasks = append(phase.Tasks, Task{Content: content, Status: StatusPending})
	}
	return phases
}

// removeTasks removes one task, one phase's tasks, or every task.
func removeTasks(phases []Phase, p Params, errors *[]string) []Phase {
	switch {
	case p.Task != "":
		hit := findTask(phases, p.Task, errors)
		if hit == nil {
			return phases
		}
		for i := range phases {
			kept := phases[i].Tasks[:0]
			for j := range phases[i].Tasks {
				if &phases[i].Tasks[j] != hit {
					kept = append(kept, phases[i].Tasks[j])
				}
			}
			phases[i].Tasks = kept
		}
		return phases
	case p.Phase != "":
		phase := findPhaseOrError(phases, p.Phase, errors)
		if phase == nil {
			return phases
		}
		phase.Tasks = nil
		return phases
	default:
		for i := range phases {
			phases[i].Tasks = nil
		}
		return phases
	}
}

// targets resolves the tasks a task/phase/everything op acts on. An op that
// names neither targets every task in every phase.
func targets(phases []Phase, p Params, errors *[]string) []*Task {
	switch {
	case p.Task != "":
		if hit := findTask(phases, p.Task, errors); hit != nil {
			return []*Task{hit}
		}
		return nil
	case p.Phase != "":
		phase := findPhaseOrError(phases, p.Phase, errors)
		if phase == nil {
			return nil
		}
		out := make([]*Task, 0, len(phase.Tasks))
		for j := range phase.Tasks {
			out = append(out, &phase.Tasks[j])
		}
		return out
	default:
		var out []*Task
		for i := range phases {
			for j := range phases[i].Tasks {
				out = append(out, &phases[i].Tasks[j])
			}
		}
		return out
	}
}

func findPhase(phases []Phase, name string) *Phase {
	for i := range phases {
		if phases[i].Name == name {
			return &phases[i]
		}
	}
	return nil
}

func findPhaseOrError(phases []Phase, name string, errors *[]string) *Phase {
	phase := findPhase(phases, name)
	if phase == nil {
		*errors = append(*errors, fmt.Sprintf("Phase %q not found", name))
	}
	return phase
}

func findTaskByContent(phases []Phase, content string) *Task {
	for i := range phases {
		for j := range phases[i].Tasks {
			if phases[i].Tasks[j].Content == content {
				return &phases[i].Tasks[j]
			}
		}
	}
	return nil
}

// findTask resolves a task by its verbatim content. Tasks are addressed by
// content and never by id, so the error text says so: a model that invents
// `task-1` gets told the addressing rule instead of retrying the same guess.
func findTask(phases []Phase, content string, errors *[]string) *Task {
	if content == "" {
		*errors = append(*errors, "Missing task content")
		return nil
	}
	if hit := findTaskByContent(phases, content); hit != nil {
		return hit
	}
	if looksLikeTaskID(content) {
		*errors = append(*errors, fmt.Sprintf(
			"Task %q not found. Tasks are referenced by their full text, not by ids — pass the task's content from the previous result.", content))
		return nil
	}
	hint := ""
	if len(UnsettledTasks(phases)) == 0 {
		hint = " (the todo list is empty — was it replaced, or not created yet?)"
	}
	*errors = append(*errors, fmt.Sprintf("Task %q not found%s", content, hint))
	return nil
}

// looksLikeTaskID reports whether content is a generated id such as `task-1`,
// which models emit when they treat the list as a database.
func looksLikeTaskID(content string) bool {
	rest, ok := strings.CutPrefix(content, "task-")
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CompletedTransitions names the tasks that became completed in one call, so a
// renderer can show progress without diffing two snapshots itself.
func CompletedTransitions(before, after []Phase) []CompletedTask {
	previous := map[string]Status{}
	for _, phase := range before {
		for _, task := range phase.Tasks {
			previous[transitionKey(phase.Name, task.Content)] = task.Status
		}
	}
	var out []CompletedTask
	for _, phase := range after {
		for _, task := range phase.Tasks {
			if task.Status != StatusCompleted {
				continue
			}
			if status, existed := previous[transitionKey(phase.Name, task.Content)]; existed && status != StatusCompleted {
				out = append(out, CompletedTask{Phase: phase.Name, Content: task.Content})
			}
		}
	}
	return out
}

// transitionKey identifies one task across two snapshots. A NUL byte separates
// the parts because neither a phase name nor task content may contain one.
func transitionKey(phase, content string) string { return phase + "\x00" + content }

// Summary renders the tool result text. It is model-facing, so it carries the
// two things a model needs to act correctly and nothing else: which tasks are
// still open, and what the active phase pointer means. The tree is included
// because the model addresses tasks by their verbatim content, and a summary
// that omitted the text would force a `view` call to recover it.
func Summary(phases []Phase, errors []string, readOnly bool) string {
	var tasks []Task
	for _, phase := range phases {
		tasks = append(tasks, phase.Tasks...)
	}
	if len(tasks) == 0 {
		if len(errors) > 0 {
			return "Errors: " + strings.Join(errors, "; ")
		}
		if readOnly {
			return "Todo list is empty."
		}
		return "Todo list cleared."
	}

	var b strings.Builder
	if len(errors) > 0 {
		fmt.Fprintf(&b, "Errors: %s\n", strings.Join(errors, "; "))
	}

	open := UnsettledTasks(phases)
	if len(open) == 0 {
		b.WriteString("Remaining items: none.\n")
	} else {
		fmt.Fprintf(&b, "Remaining items (%d):\n", len(open))
		for _, phase := range phases {
			for _, task := range phase.Tasks {
				if task.Status.Settled() {
					continue
				}
				fmt.Fprintf(&b, "  - %s [%s] (%s)\n", task.Content, task.Status, phase.Name)
			}
		}
	}

	settled := len(tasks) - len(open)
	blocked := 0
	for _, task := range tasks {
		if task.Status == StatusBlocked {
			blocked++
		}
	}
	line := fmt.Sprintf("Overall: %d/%d done, %d open", settled, len(tasks), len(open))
	if blocked > 0 {
		line += fmt.Sprintf(", %d blocked", blocked)
	}
	b.WriteString(line + ".\n")

	for _, phase := range phases {
		fmt.Fprintf(&b, "  %s:\n", phase.Name)
		for _, task := range phase.Tasks {
			if task.Status.Settled() {
				fmt.Fprintf(&b, "    - [%s] %s\n", statusMarker(task.Status), task.Content)
				continue
			}
			tag := ""
			switch task.Status {
			case StatusInProgress:
				tag = " (in progress)"
			case StatusAbandoned:
				tag = " (dropped)"
			case StatusBlocked:
				if task.Blocker != "" {
					tag = fmt.Sprintf(" (blocked: %s)", task.Blocker)
				} else {
					tag = " (blocked)"
				}
			}
			fmt.Fprintf(&b, "    - [%s] %s%s\n", statusMarker(task.Status), task.Content, tag)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// statusMarker is the checkbox glyph for a status: done, in progress, open, or
// one of the two ways a task can leave the list without being a done task.
func statusMarker(status Status) string {
	switch status {
	case StatusCompleted:
		return "X"
	case StatusInProgress:
		return "/"
	case StatusAbandoned:
		return "-"
	case StatusBlocked:
		return "!"
	default:
		return " "
	}
}
