package subagents

// Ports src/prompts.ts. The inherited prefix stays byte-identical for caching;
// each child adds its own identity, environment, skills and worktree boundary.
import (
	"context"
	"runtime"
	"strings"
	"time"
)

func InheritableParentPrompt(rendered, cwd string) string {
	marker := "\n\n<cwd>\n" + strings.ReplaceAll(cwd, "\\", "/") + "\n</cwd>"
	i := strings.LastIndex(rendered, marker)
	if i < 0 {
		return rendered
	}
	head := rendered[:i]
	if strings.HasSuffix(head, "\n</skills>") {
		if at := strings.LastIndex(head, "\n\n<skills>\n"); at >= 0 {
			return head[:at]
		}
	}
	return head
}
func BuildPrompt(ctx context.Context, c Config, cwd, parent, worktreeBase string, workflow bool, extras string) string {
	gitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	isGit, err := git(gitCtx, cwd, "rev-parse", "--is-inside-work-tree")
	env := "# Environment\nWorking directory: " + cwd + "\n"
	if err == nil && isGit == "true" {
		branch, e := git(gitCtx, cwd, "branch", "--show-current")
		if e != nil {
			branch = "unknown"
		}
		env += "Git repository: yes\nBranch: " + branch
	} else {
		env += "Not a git repository"
	}
	env += "\nPlatform: " + runtime.GOOS
	active := `<active_agent name="` + xmlEscape(c.Name) + `"/>` + "\n\n"
	worktree := ""
	if worktreeBase != "" {
		worktree = "\n\n<worktree_isolation>\nYour working directory is an isolated git worktree copy of " + worktreeBase + ".\nWork only inside it — never in " + worktreeBase + ", even if other instructions name that path as your working directory.\n</worktree_isolation>"
	}
	workflowBlock := ""
	if workflow {
		workflowBlock = "\n\n<workflow_child>\nYour final message IS the return value of this task. A workflow script captures it and passes it to the next stage; no person reads it.\nReturn only the answer, in exactly the shape the prompt asks for — no preamble, no summary of what you did, no offer to continue.\n</workflow_child>"
	}
	if extras != "" {
		extras = "\n\n" + extras
	}
	if c.PromptMode == "append" {
		if parent == "" {
			parent = "# Role\nYou are a general-purpose coding agent for complex, multi-step tasks.\nYou have full access to read, write, edit files, and execute commands.\nDo what has been asked; nothing more, nothing less."
		}
		bridge := `<sub_agent_context>
You are operating as a sub-agent invoked to handle a specific task.
- Use the read tool instead of cat/head/tail
- Use the edit tool instead of sed/awk
- Use the write tool instead of echo/heredoc
- Use the find tool instead of bash find/ls for file search
- Use the grep tool instead of bash grep/rg for content search
- Make independent tool calls in parallel
- Use absolute file paths
- Do not use emojis
- Be concise but complete
</sub_agent_context>`
		custom := ""
		if strings.TrimSpace(c.SystemPrompt) != "" {
			custom = "\n\n<agent_instructions>\n" + c.SystemPrompt + "\n</agent_instructions>"
		}
		return parent + "\n\n" + bridge + "\n\n" + active + env + worktree + workflowBlock + custom + extras
	}
	return active + "You are a pi coding agent sub-agent.\nYou have been invoked to handle a specific task autonomously.\n\n" + env + worktree + workflowBlock + "\n\n" + c.SystemPrompt + extras
}
