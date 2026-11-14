package subagents

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

// Model-facing descriptions adapted from the pinned extension, with dynamic
// values replaced at registration time, as the original template renderer does.
//
//go:embed agent-description.txt
var fullAgentDescription string

//go:embed agent-description-compact.txt
var compactAgentDescription string

//go:embed workflow-description.txt
var fullWorkflowDescription string

//go:embed workflow-description-compact.txt
var compactWorkflowDescription string

func (m *Manager) toolDescription(workflow bool) string {
	s := m.Settings()
	var full, compact []string
	for _, c := range m.Configs() {
		if c.Enabled != nil && !*c.Enabled {
			continue
		}
		tools := "all available tools"
		if c.Tools != nil {
			tools = strings.Join(c.Tools, ", ")
		}
		full = append(full, "- "+c.Name+": "+c.Description+" (Tools: "+tools+")")
		description := c.Description
		for i, r := range description {
			if strings.ContainsRune(".!?", r) && i+1 < len(description) && description[i+1] == ' ' {
				description = description[:i+1]
				break
			}
		}
		compact = append(compact, "- "+c.Name+": "+description+" (Tools: "+tools+")")
	}
	isolation, shortIsolation, schedule := "", "", ""
	if s.WorktreeIsolation {
		isolation = "\n- Use isolation: \"worktree\" to give the agent its own git worktree (safe parallel file modifications); leave it unset, or pass \"off\", for none. The worktree is removed when the agent finishes; if it made changes, they are committed to a branch and the branch is named in the result."
		shortIsolation = "\n- isolation: \"worktree\" gives the agent its own git worktree (removed on completion); changes land on a branch named in the result."
	}
	if s.SchedulingEnabled {
		schedule = "\n- Use `schedule` only when the user explicitly asked for scheduled / recurring / delayed execution (e.g. \"every Monday\", \"in an hour\"). Don't auto-schedule from vague intent like \"monitor X\" — run once now or ask."
	}
	template := fullAgentDescription
	if workflow {
		template = fullWorkflowDescription
		if s.ToolDescriptionMode == "compact" {
			template = compactWorkflowDescription
		}
	} else {
		switch s.ToolDescriptionMode {
		case "compact":
			template = compactAgentDescription
		case "custom":
			for _, path := range []string{filepath.Join(m.host.CWD, ".pi", "agent-tool-description.md"), filepath.Join(m.host.AgentDir, "agent-tool-description.md")} {
				b, err := os.ReadFile(path)
				if err == nil && strings.TrimSpace(string(b)) != "" {
					template = strings.TrimSpace(string(b))
					break
				}
			}
		}
	}
	return strings.NewReplacer("{{typeList}}", strings.Join(full, "\n"), "{{compactTypeList}}", strings.Join(compact, "\n"), "{{agentDir}}", m.host.AgentDir, "{{isolationGuideline}}", isolation, "{{isolationCompactGuideline}}", shortIsolation, "{{scheduleGuideline}}", schedule).Replace(template)
}
