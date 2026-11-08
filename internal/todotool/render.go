package todotool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"

	"ompig-slim/internal/todo"
)

// This file ports oh-my-pi's todo card, packages/tui/src/tools/todo.ts
// (todoToolRenderer), and the helpers it draws through: renderStatusLine,
// renderTreeList, formatTodoLine, selectCollapsedTodos, formatPhaseSummary and
// framedToolCard.
//
// The card is reproduced line for line rather than approximated, because the
// card is the only place a user watches a plan advance: a layout that differs
// from OMP by a tree gutter, a roman numeral, or a collapsed-window rule reads as
// a different tool, and the differences are easy to mistake for a bug in the
// agent's progress.
//
// Symbols, colors, counts and thresholds come from OMP's theme and preview
// limits. They are constants here instead of theme lookups because the vendored
// runtime's Theme exposes foreground tokens but not OMP's symbol table; the
// glyphs below are OMP's values, so a user comparing the two sees one design.

// Card symbols, from OMP's theme symbol table (packages/tui/src/theme/symbols.ts).
const (
	symCheckboxChecked   = "☑"
	symCheckboxUnchecked = "☐"
	symTreeBranch        = "├─"
	symTreeLast          = "└─"
	symSepDot            = " · "
	// symTodoTool is OMP's theme symbol "tool.todo" in the default symbol set.
	symTodoTool = "☑"
)

// collapsedItems is OMP's PREVIEW_LIMITS.COLLAPSED_ITEMS: how many task rows a
// collapsed phase previews before summarizing the rest.
const collapsedItems = 8

// collapsedClosedContext is OMP's COLLAPSED_CLOSED_CONTEXT: closed rows kept
// directly above the open window, so finishing a task is visible as it happens
// instead of silently removing a row.
const collapsedClosedContext = 1

// stripForHeader flattens model-authored text for a single-line header. OMP's
// flattenForHeader does the same for status lines: a raw newline would break the
// line the title rides on.
func stripForHeader(text string) string {
	replacer := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")
	return strings.TrimSpace(replacer.Replace(text))
}

// fg is the theme lookup the card uses throughout.
func fg(token tui.ThemeColor, text string) string {
	return tui.ActiveTheme().Fg(string(token), text)
}

// renderStatusLine is OMP's renderStatusLine: `icon title · meta` with the icon
// and title colored and the meta dim.
func renderStatusLine(icon, title, titleToken string, meta []string) string {
	line := fg(tui.ThemeColorAccent, stripForHeader(title))
	if fgIcon := icon; fgIcon != "" {
		line = fgIcon + " " + line
	}
	kept := make([]string, 0, len(meta))
	for _, value := range meta {
		if flattened := stripForHeader(value); flattened != "" {
			kept = append(kept, flattened)
		}
	}
	if len(kept) > 0 {
		line += " " + fg(tui.ThemeColorDim, strings.Join(kept, symSepDot))
	}
	return line
}

// romanNumeral is OMP's phaseRomanNumeral: one-based ASCII roman numbering for
// phase headers, display only. State and prompts never see it.
func romanNumeral(oneBased int) string {
	if oneBased <= 0 {
		return ""
	}
	pairs := []struct {
		value int
		sym   string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"},
		{90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"},
		{5, "V"}, {4, "IV"}, {1, "I"},
	}
	var b strings.Builder
	rem := oneBased
	for _, pair := range pairs {
		for rem >= pair.value {
			b.WriteString(pair.sym)
			rem -= pair.value
		}
	}
	return b.String()
}

// phaseDisplayName is OMP's formatPhaseDisplayName: `I. Foundation`.
func phaseDisplayName(name string, oneBased int) string {
	return fmt.Sprintf("%s. %s", romanNumeral(oneBased), stripForHeader(name))
}

// phaseProgress is OMP's formatPhaseProgress: the dim `closed/total` suffix a
// phase header carries. It counts closed tasks, not just completed ones, because
// the collapsed viewport hides both, so an abandoned task must still move the
// counter or its phase reads as permanently stuck.
func phaseProgress(phase todo.Phase) string {
	closed := 0
	for _, task := range phase.Tasks {
		if task.Status.Settled() {
			closed++
		}
	}
	return fg(tui.ThemeColorDim, fmt.Sprintf("  %d/%d", closed, len(phase.Tasks)))
}

// formatPhaseSummary is OMP's one-line row for a phase this update did not
// touch: a dim bold header plus progress, so a single task flip does not redraw
// every phase's full task list.
func formatPhaseSummary(phase todo.Phase, oneBased int) string {
	name := fg(tui.ThemeColorDim, tui.ChalkBold(phaseDisplayName(phase.Name, oneBased)))
	return name + phaseProgress(phase)
}

// isClosedTask mirrors OMP's isClosedTodo.
func isClosedTask(task todo.Task) bool { return task.Status.Settled() }

// isActiveTask mirrors OMP's isActiveTodo for the paths this port supports. OMP
// also counts a pending task a live subagent is executing as active; ompig-slim
// has no equivalent registry wired into a renderer, so only the literal
// in_progress task is active. A pending task therefore stays dim.
func isActiveTask(task todo.Task) bool { return task.Status == todo.StatusInProgress }

// formatTodoLine is OMP's formatTodoLine: one checkbox row, colored by status.
func formatTodoLine(item todo.Task) string {
	text := stripForHeader(item.Content)
	switch item.Status {
	case todo.StatusCompleted:
		return fg(tui.ThemeColorSuccess, fmt.Sprintf("%s %s", symCheckboxChecked, strikethrough(text)))
	case todo.StatusInProgress:
		return fg(tui.ThemeColorAccent, fmt.Sprintf("%s %s", symCheckboxUnchecked, text))
	case todo.StatusAbandoned:
		return fg(tui.ThemeColorError, fmt.Sprintf("%s %s", symCheckboxUnchecked, strikethrough(text)))
	case todo.StatusBlocked:
		note := "blocked"
		if item.Blocker != "" {
			note = "blocked: " + stripForHeader(item.Blocker)
		}
		return fg(tui.ThemeColorWarning, fmt.Sprintf("%s %s (%s)", symCheckboxUnchecked, text, note))
	default:
		return fg(tui.ThemeColorDim, fmt.Sprintf("%s %s", symCheckboxUnchecked, text))
	}
}

// strikethrough applies SGR 9/29 around text, as OMP's strikethroughText does.
func strikethrough(text string) string {
	return "\x1b[9m" + text + "\x1b[29m"
}

// collapsedSelection is OMP's CollapsedTodoSelection: the rows to draw plus an
// optional summary line ("" means no summary row).
type collapsedSelection struct {
	items   []todo.Task
	summary string
}

// formatMoreItems is OMP's formatMoreItems.
func formatMoreItems(remaining int, itemType string) string {
	noun := itemType
	if remaining != 1 {
		noun = itemType + "s"
	}
	return fmt.Sprintf("… %d more %s", remaining, noun)
}

// selectWithinCap is OMP's selectWithinCap: the rows to show for a list already
// reduced to the relevant tasks.
//
//  1. Every active task leads, in todo order, and is never dropped for lying
//     outside an ordinary window.
//  2. Remaining rows fill with the tasks following the first active one, so a
//     freshly promoted task leads the preview.
//  3. When active tasks alone exceed the cap, only the first cap are shown and
//     the summary counts the hidden active tasks.
//
// The summary occupies a row, so a summary that would hide exactly one task is
// replaced by that task: the selection spends at most cap+1 rows.
func selectWithinCap(base []todo.Task, cap int) collapsedSelection {
	if len(base) <= cap+1 {
		return collapsedSelection{items: base}
	}
	var active []todo.Task
	for _, task := range base {
		if isActiveTask(task) {
			active = append(active, task)
		}
	}
	if len(active) > cap {
		hidden := len(active) - cap
		noun := "todo"
		if hidden != 1 {
			noun = "todos"
		}
		return collapsedSelection{
			items:   active[:cap],
			summary: fmt.Sprintf("… %d more active %s", hidden, noun),
		}
	}
	firstActive := 0
	if len(active) > 0 {
		for i, task := range base {
			if task.Content == active[0].Content && task.Status == active[0].Status {
				firstActive = i
				break
			}
		}
	}
	fill := make([]todo.Task, 0, cap)
	for i := firstActive; i < len(base) && len(active)+len(fill) < cap; i++ {
		if isActiveTask(base[i]) {
			continue
		}
		fill = append(fill, base[i])
	}
	items := append(append([]todo.Task{}, active...), fill...)
	hidden := len(base) - len(items)
	selection := collapsedSelection{items: items}
	if hidden > 0 {
		selection.summary = formatMoreItems(hidden, "todo")
	}
	return selection
}

// selectCollapsedTodos is OMP's selectCollapsedTodos: the walking viewport for
// one phase's collapsed preview.
//
// The open tasks run through selectWithinCap, led by the last
// collapsedClosedContext closed tasks so a checked row stays visible. The lead is
// additive and never costs an open row; a phase with no open work falls back to
// its closed tasks so a settled phase still draws something.
func selectCollapsedTodos(tasks []todo.Task, cap int) collapsedSelection {
	var open []todo.Task
	for _, task := range tasks {
		if !isClosedTask(task) {
			open = append(open, task)
		}
	}
	if len(open) == 0 {
		return selectWithinCap(tasks, cap)
	}
	var closed []todo.Task
	for _, task := range tasks {
		if isClosedTask(task) {
			closed = append(closed, task)
		}
	}
	// The last closed task only. A phase that ended with a drop therefore leads
	// with the drop, which is what OMP shows: the lead is a record of the most
	// recent state change, not a summary of everything closed.
	lead := closed
	if len(lead) > collapsedClosedContext {
		lead = lead[len(lead)-collapsedClosedContext:]
	}
	selected := selectWithinCap(open, cap)
	items := append(append([]todo.Task{}, lead...), selected.items...)
	return collapsedSelection{items: items, summary: selected.summary}
}

// renderTreeRows draws rows with OMP's tree gutters: every row but the last gets
// `├─ `, the last `└─ `. renderTreeListRows in OMP delegates this to TreeView
// with an empty child list, which leaves exactly those two prefixes.
func renderTreeRows(rows []treeRow) []string {
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		if row.summary {
			out = append(out, row.text)
			continue
		}
		prefix := symTreeBranch
		if i == len(rows)-1 {
			prefix = symTreeLast
		}
		out = append(out, fg(tui.ThemeColorDim, prefix)+" "+row.text)
	}
	return out
}

// treeRow is one line renderTreeRows draws: an item's rendered text, or the
// muted summary row.
type treeRow struct {
	text    string
	summary bool
}

// touchedPhases is OMP's computeTouchedPhases: the phases the latest update
// touched, plus the phase holding the active task. A nil result means "draw every
// phase fully", which preserves the legacy view and the manual-expand path.
func touchedPhases(p todo.Params, phases []todo.Phase, completed []todo.CompletedTask) map[string]bool {
	touched := map[string]bool{}
	for _, phase := range phases {
		for _, task := range phase.Tasks {
			if task.Status == todo.StatusInProgress {
				touched[phase.Name] = true
				break
			}
		}
	}
	for _, transition := range completed {
		touched[transition.Phase] = true
	}
	// `init` replaces the whole list, so the entire plan is fresh and every phase
	// counts as touched.
	if p.Op == todo.OpInit {
		for _, phase := range phases {
			touched[phase.Name] = true
		}
		return touched
	}
	if p.Phase != "" {
		if phase := findPhaseNamed(phases, p.Phase); phase != nil {
			touched[phase.Name] = true
		}
	}
	if p.Task != "" {
		for _, phase := range phases {
			for _, task := range phase.Tasks {
				if task.Content == p.Task {
					touched[phase.Name] = true
					break
				}
			}
		}
	}
	if len(touched) == 0 {
		return nil
	}
	return touched
}

func findPhaseNamed(phases []todo.Phase, name string) *todo.Phase {
	for i := range phases {
		if phases[i].Name == name {
			return &phases[i]
		}
	}
	return nil
}

// renderCall draws the card's call region.
//
// OMP's todo renderer merges call and result: once a result exists the call
// draws nothing, so the transcript shows one `Todo` line instead of a stale
// "op" line above the task list (packages/tui/src/chat/tool-execution.ts:1837,
// `shouldRenderCall = !this.#result || !mergeCallAndResult`). The vendored card
// has no merge flag and substitutes a name-and-args fallback for a nil call, so
// the call region cannot be dropped from here. It therefore returns nothing but
// the card's own zero-width spacer once the call is settled, which leaves the
// result region as the only visible content.
//
// While the call is still streaming there is no result yet, so the header is the
// whole card: a lone status line naming the operation, as OMP draws it.
func renderCall(args json.RawMessage, _ extension.Theme, context extension.ToolRenderContext) extension.Component {
	// IsPartial is set while the tool runs and cleared when its result settles, so
	// it is the reliable "does a result exist" signal (ExecutionStarted stays false
	// on this card's path). Only the streaming window draws a header.
	if !context.IsPartial {
		return blankComponent()
	}
	ops := describeOps(args)
	header := renderStatusLine(fg(tui.ThemeColorWarning, "●"), "Todo", "", ops)
	return tui.NewPaddedText(header, 0, 0, nil)
}

// blankComponent renders no rows, for a region that must not draw.
func blankComponent() tui.Component { return tui.NewPaddedText("", 0, 0, nil) }

// describeOps makes the header's meta from the call's arguments: `init 3 items`,
// `done <task>`, and so on. It reads the partially parsed arguments of a
// streaming call, so every field is treated as optional and possibly the wrong
// type.
func describeOps(args json.RawMessage) []string {
	fields := decodeArgs(args)
	if len(fields) == 0 {
		return nil
	}
	parts := []string{strings.ToLower(stringOr(fields, "op", "update"))}
	if task := stringOr(fields, "task", ""); task != "" {
		parts = append(parts, stripForHeader(task))
	}
	if phase := stringOr(fields, "phase", ""); phase != "" {
		parts = append(parts, stripForHeader(phase))
	}
	if items, ok := fields["items"].([]any); ok && len(items) > 0 {
		noun := "items"
		if len(items) == 1 {
			noun = "item"
		}
		parts = append(parts, fmt.Sprintf("%d %s", len(items), noun))
	}
	return []string{strings.Join(parts, " ")}
}

// renderResult draws the settled card: a status line naming the task count, then
// the phase tree.
func renderResult(result extension.AgentToolResult, options extension.ToolRenderResultOptions, _ extension.Theme, context extension.ToolRenderContext) extension.Component {
	container := cardContainer(context.LastComponent)

	if result.IsError {
		header := renderStatusLine(fg(tui.ThemeColorError, "✗"), "Todo", "", nil)
		container.Add(textLine(header))
		for _, line := range strings.Split(toolResultText(result), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			container.Add(textLine("  " + fg(tui.ThemeColorError, stripForHeader(line))))
		}
		return container
	}

	details := decodeDetails(result)
	phases := make([]todo.Phase, 0, len(details.Phases))
	for _, phase := range details.Phases {
		if len(phase.Tasks) > 0 {
			phases = append(phases, phase)
		}
	}
	total := 0
	for _, phase := range phases {
		total += len(phase.Tasks)
	}
	icon := fg(tui.ThemeColorAccent, symTodoTool)
	header := renderStatusLine(icon, "Todo", "", []string{fmt.Sprintf("%d tasks", total)})
	container.Add(textLine(header))

	if total == 0 {
		fallback := toolResultText(result)
		if strings.TrimSpace(fallback) == "" {
			fallback = "No todos"
		}
		container.Add(textLine("  " + fg(tui.ThemeColorDim, stripForHeader(fallback))))
		return container
	}

	// Everything below is fixed for this result; only the expand toggle varies.
	args := decodeArgs(context.Args)
	var params todo.Params
	if op, ok := args["op"].(string); ok {
		params.Op = todo.Op(op)
	}
	params.Task = stringOr(args, "task", "")
	params.Phase = stringOr(args, "phase", "")

	multiPhase := len(phases) > 1
	indent := ""
	if multiPhase {
		indent = "  "
	}
	// Collapse phases this update did not touch to one line, so a single task flip
	// does not redraw every phase's full list. The manual expand toggle still shows
	// all.
	var touched map[string]bool
	if !options.Expanded && multiPhase {
		touched = touchedPhases(params, phases, details.CompletedTasks)
	}

	for i, phase := range phases {
		if touched != nil && !touched[phase.Name] {
			container.Add(textLine(formatPhaseSummary(phase, i+1)))
			continue
		}
		if multiPhase {
			name := fg(tui.ThemeColorAccent, tui.ChalkBold(phaseDisplayName(phase.Name, i+1)))
			container.Add(textLine(name + phaseProgress(phase)))
		}
		var rows []treeRow
		if options.Expanded {
			for _, task := range phase.Tasks {
				rows = append(rows, treeRow{text: formatTodoLine(task)})
			}
		} else {
			selection := selectCollapsedTodos(phase.Tasks, collapsedItems)
			for _, task := range selection.items {
				rows = append(rows, treeRow{text: formatTodoLine(task)})
			}
			if selection.summary != "" {
				rows = append(rows, treeRow{text: fg(tui.ThemeColorMuted, selection.summary), summary: true})
			}
		}
		for _, line := range renderTreeRows(rows) {
			container.Add(textLine(indent + line))
		}
	}

	return container
}

// cardContainer reuses the component a card already has, clearing it so a
// re-render after an expand or a progress update redraws in place instead of
// stacking.
func cardContainer(last extension.Component) *tui.Container {
	container, ok := last.(*tui.Container)
	if !ok || container == nil {
		container = tui.NewContainer()
	}
	container.Clear()
	return container
}

func textLine(text string) *tui.Text { return tui.NewPaddedText(text, 0, 0, nil) }

// toolResultText joins a result's text blocks, the provider text OMP reads from
// content[0].text.
func toolResultText(result extension.AgentToolResult) string {
	parts := make([]string, 0, len(result.Content))
	for _, block := range result.Content {
		if text, ok := block.(ai.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// decodeDetails reads the todo details a card draws. A card may hold them as the
// value the tool returned or as decoded JSON after a resume, so they go through
// JSON either way.
func decodeDetails(result extension.AgentToolResult) todo.Details {
	var details todo.Details
	raw, err := jsonMarshal(result.Details)
	if err != nil {
		return details
	}
	_ = jsonUnmarshal(raw, &details)
	return details
}

// decodeArgs reads a streaming call's arguments, which arrive as raw JSON while
// completing and as a decoded object once complete.
func decodeArgs(args any) map[string]any {
	switch value := args.(type) {
	case map[string]any:
		return value
	case []byte:
		var fields map[string]any
		if jsonUnmarshal(value, &fields) == nil {
			return fields
		}
	case string:
		var fields map[string]any
		if jsonUnmarshal([]byte(value), &fields) == nil {
			return fields
		}
	}
	return nil
}

func stringOr(fields map[string]any, key, fallback string) string {
	if value, ok := fields[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

// jsonMarshal and jsonUnmarshal keep the JSON dependency of the drawing code in
// one place; the tool and the snapshot reader already depend on it.
func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }

func jsonUnmarshal(raw []byte, target any) error { return json.Unmarshal(raw, target) }
