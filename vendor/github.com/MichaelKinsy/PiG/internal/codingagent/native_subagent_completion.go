package codingagent

import (
	"context"
	"strings"

	"github.com/MichaelKinsy/PiG/tui"
)

// Embedding the concrete provider retains its async file-search planner and
// cancellation behavior while adding agent rows beside file attachments.
type subagentAutocomplete struct {
	*tui.CombinedProvider
	m *InteractiveMode
}

func (p *subagentAutocomplete) GetSuggestions(ctx context.Context, lines []string, row, col int, options tui.AutocompleteSuggestionOptions) *tui.AutocompleteSuggestions {
	base := p.CombinedProvider.GetSuggestions(ctx, lines, row, col, options)
	return p.merge(base, lines, row, col)
}
func (p *subagentAutocomplete) merge(base *tui.AutocompleteSuggestions, lines []string, row, col int) *tui.AutocompleteSuggestions {
	manager := p.m.subagentManager()
	if manager == nil || !manager.Settings().Enabled || manager.Settings().AgentMentions == "off" || row != 0 || len(lines) == 0 {
		return base
	}
	line := lines[0]
	col = min(max(0, col), len(line))
	prefix := line[:col]
	if !strings.HasPrefix(prefix, "@") || strings.ContainsAny(prefix, " \t\n") {
		return base
	}
	query := strings.ToLower(strings.TrimPrefix(prefix, "@"))
	var items []tui.AutocompleteItem
	used := map[string]bool{}
	add := func(handle, label, description string) {
		if used[handle] || !strings.Contains(strings.ToLower(handle), query) {
			return
		}
		used[handle] = true
		items = append(items, tui.AutocompleteItem{Value: "@" + handle, Label: "@" + label, Description: description})
	}
	for _, r := range manager.Records() {
		if !r.TopLevel() {
			continue
		}
		action := "resume"
		if r.Active() {
			action = "send message"
		}
		handle := r.Handle
		if r.Alias != "" {
			handle = r.Alias
		}
		add(handle, handle, action+" · "+r.Type+" · "+r.Status+" · "+r.Description)
		if r.Alias != "" {
			add(r.Handle, r.Handle, action+" · "+r.Description)
		}
	}
	for _, c := range manager.Configs() {
		if c.Enabled == nil || *c.Enabled {
			handle := strings.ToLower(c.Name)
			add(handle, handle, "start agent · "+c.Description)
		}
	}
	add("main", "main", "send to main agent")
	if base != nil && base.Prefix == prefix {
		items = append(items, base.Items...)
	}
	if len(items) == 0 {
		return base
	}
	return &tui.AutocompleteSuggestions{Items: items, Prefix: prefix}
}

func (p *subagentAutocomplete) SuggestionTask(lines []string, row, col int, force bool) (string, func(context.Context) ([]tui.AutocompleteItem, error), bool) {
	prefix, task, ok := p.CombinedProvider.SuggestionTask(lines, row, col, force)
	extra := p.merge(nil, lines, row, col)
	if extra == nil {
		return prefix, task, ok
	}
	items := append([]tui.AutocompleteItem(nil), extra.Items...)
	return extra.Prefix, func(ctx context.Context) ([]tui.AutocompleteItem, error) {
		if ok && prefix == extra.Prefix {
			files, err := task(ctx)
			if err != nil {
				return nil, err
			}
			return append(items, files...), nil
		}
		return items, ctx.Err()
	}, true
}
