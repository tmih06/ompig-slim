// Ports packages/coding-agent/src/core/system-prompt.ts.

// Package prompts builds the default coding-agent system prompt.
//
// It mirrors upstream core/system-prompt.js as of Pi 0.87.1: an untagged
// preamble, then tagged sections in upstream order (tools, rules,
// addendum, project_context, skills, cwd). The model receives this text, so it
// matches upstream byte for byte apart from the product name and the
// omission of bundled self-documentation. BuildDefaultPrompt's callers pass the
// same inputs upstream's agent session collects: selected tools, their prompt
// snippets and guidelines, context files, and skills.
package prompts

import (
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// Skill is the subset of a discovered skill that the prompt lists.
type Skill struct {
	Name        string
	Description string
	Path        string
	// DisableModelInvocation hides the skill from the model, as upstream's
	// disable-model-invocation frontmatter does.
	DisableModelInvocation bool
}

// Options configures BuildDefaultPrompt.
type Options struct {
	// Cwd is the working directory shown to the model.
	Cwd string
	// Tools are the selected tool names in the order the model sees them. Nil selects read, bash, edit, write, grep, and find; an empty slice selects none.
	Tools []string
	// HiddenTools are the selected tools whose declarations requests leave out. They are reachable only through another tool, so the tool list and rules leave them out too.
	HiddenTools []string
	// ToolHints maps a tool name to its prompt snippet. Upstream lists only
	// tools that have a snippet.
	ToolHints map[string]string
	// ToolGuidelines maps a tool name to its prompt guidelines. Guidelines of
	// selected tools become rules, deduplicated in tool order.
	ToolGuidelines map[string][]string
	// PromptGuidelines are rules that do not belong to one tool.
	PromptGuidelines []string
	// Skills lists discovered skills.
	Skills []Skill
	// CustomPrompt is an agent definition's body. AppendMode decides its role.
	CustomPrompt string
	// ForceSystemPrompt replaces the full prompt without sections, including when it points to an empty string.
	ForceSystemPrompt *string
	// AppendSystemPrompt is appended as an addendum after the configured preamble.
	AppendSystemPrompt string
	// ContextFiles are the loaded AGENTS.md and CLAUDE.md files.
	ContextFiles []struct{ Path, Content string }
	// AppendMode is "append" (default: CustomPrompt becomes the addendum
	// section) or "replace" (CustomPrompt replaces the preamble, and the tools,
	// rules sections are omitted, as upstream does for a custom
	// system prompt).
	AppendMode string
}

// Preamble is the base system prompt text: the paragraph that opens the prompt,
// before the tagged tool, rules, project-context, and skills sections.
//
// It is a variable so an embedding program can supply its own prose, which is
// how ompig-slim ships a prompt maintained as a Markdown file rather than Go
// source. The vendored default is upstream's paragraph, so a build that never
// sets it behaves exactly as before. Set it once at startup, before any session
// builds its prompt; a session that is already running keeps the text it read.
//
// The composition around it is unchanged: a program that sets Preamble still
// gets the tool, rules, project-context, and skills sections.
//
// An empty value falls back to the default, so clearing the variable cannot
// produce a prompt with no opening sentence.
var Preamble = defaultPreamble

// defaultPreamble is upstream's opening paragraph, and the value Preamble falls
// back to when it is unset.
const defaultPreamble = "You are a coding assistant running inside ompig-slim, a coding agent harness. Help users inspect files, run commands, and modify code."

// preambleText resolves the paragraph to render.
func preambleText() string {
	if strings.TrimSpace(Preamble) == "" {
		return defaultPreamble
	}
	return Preamble
}

type section struct{ name, content string }

// BuildDefaultPrompt renders the structured prompt or returns an exact forced replacement.
func BuildDefaultPrompt(o Options) string {
	return ai.GetCurrentSystemPrompt([]ai.Message{BuildSystemPromptState(o)})
}

// BuildSystemPromptState returns opaque content for a forced prompt, or empty content with structured sections otherwise.
func BuildSystemPromptState(o Options) ai.SystemMessage {
	if o.ForceSystemPrompt != nil {
		return ai.SystemMessage{Content: ai.SystemText(*o.ForceSystemPrompt)}
	}
	return ai.SystemMessage{Content: ai.SystemText(""), Sections: BuildSystemPromptSections(o)}
}

// BuildSystemPromptSections retains the ordered, independently replaceable prompt sections.
func BuildSystemPromptSections(o Options) ai.OrderedSections {
	if o.Tools == nil {
		o.Tools = []string{"read", "bash", "edit", "write", "grep", "find"}
	}
	head := preambleText()
	declared := make([]string, 0, len(o.Tools))
	for _, name := range o.Tools {
		if !slices.Contains(o.HiddenTools, name) {
			declared = append(declared, name)
		}
	}
	var sections []section
	if o.AppendMode == "replace" && o.CustomPrompt != "" {
		head = o.CustomPrompt
	} else {
		var tools []string
		for _, name := range declared {
			if hint := o.ToolHints[name]; hint != "" {
				tools = append(tools, "- "+name+": "+hint)
			}
		}
		list := "(none)"
		if len(tools) > 0 {
			list = strings.Join(tools, "\n")
		}
		sections = append(sections,
			section{"tools", list + "\n\nAdditional tools may be available."},
			section{"rules", renderRules(declared, o.ToolGuidelines, o.PromptGuidelines)},
		)
		if addendum := strings.TrimSpace(o.CustomPrompt); addendum != "" {
			sections = append(sections, section{"addendum", addendum})
		}
	}
	if o.AppendSystemPrompt != "" {
		if len(sections) > 0 && sections[len(sections)-1].name == "addendum" {
			sections[len(sections)-1].content += "\n\n" + o.AppendSystemPrompt
		} else {
			sections = append(sections, section{"addendum", o.AppendSystemPrompt})
		}
	}
	if len(o.ContextFiles) > 0 {
		parts := []string{"Project-specific instructions and guidelines:"}
		for _, f := range o.ContextFiles {
			parts = append(parts, `<project_instructions path="`+f.Path+`">`+"\n"+f.Content+"\n</project_instructions>")
		}
		sections = append(sections, section{"project_context", strings.Join(parts, "\n\n")})
	}
	// A hidden reader is still reachable through another tool, so skills stay but the hint names no tool.
	if readTool := skillReadTool(declared, o.Tools); readTool != "" {
		if skills := strings.TrimSpace(formatSkills(o.Skills, readTool)); skills != "" {
			sections = append(sections, section{"skills", skills})
		}
	}
	cwd := o.Cwd
	if cwd == "" {
		cwd = "."
	}
	sections = append(sections, section{"cwd", strings.ReplaceAll(cwd, `\`, "/")})
	out := ai.OrderedSections{{Name: "preamble", Value: new(head)}}
	for _, s := range sections {
		out = append(out, ai.PromptSection{Name: s.name, Value: new("<" + s.name + ">\n" + s.content + "\n</" + s.name + ">")})
	}
	return out
}

// PigExamplesLocation is PiG's examples directory. pig additive (D22): PiG's examples are published in its repository, not materialized beside the documentation bundle.
const PigExamplesLocation = "https://github.com/MichaelKinsy/PiG/tree/main/examples"

// renderRules mirrors upstream buildRules.
func renderRules(tools []string, toolGuidelines map[string][]string, promptGuidelines []string) string {
	var lines []string
	for _, rule := range guidelinesFor(tools, toolGuidelines, promptGuidelines) {
		lines = append(lines, "- "+rule)
	}
	return strings.Join(lines, "\n")
}

func guidelinesFor(tools []string, toolGuidelines map[string][]string, promptGuidelines []string) []string {
	has := func(name string) bool { return slices.Contains(tools, name) }
	seen := map[string]bool{}
	var out []string
	add := func(rule string) {
		rule = widthx.JSTrim(rule)
		if rule != "" && !seen[rule] {
			seen[rule] = true
			out = append(out, rule)
		}
	}
	bash, powershell := has("bash"), has("powershell")
	// omp-slim divergence: grep and find are in the default tool set, so a run that has them says so. Stated positively
	// and with the reason: the shell tool truncates its output to 2000 lines / 50KB and spills the rest to a temp file,
	// where these tools cap their own output and report how to widen the search.
	if bash && (has("grep") || has("find")) {
		switch {
		case has("grep") && has("find"):
			add("Search file contents with the grep tool and locate files with the find tool; each caps its output and says how to widen the search")
		case has("grep"):
			add("Search file contents with the grep tool; it caps its output and says how to widen the search")
		default:
			add("Locate files with the find tool; it caps its output and says how to widen the search")
		}
	} else if (bash || powershell) && !has("grep") && !has("find") && !has("ls") {
		switch {
		case bash && powershell:
			add("Use bash or PowerShell for file operations like listing, searching, and finding files")
		case powershell:
			add("Use PowerShell for file operations like listing, searching, and finding files")
		default:
			add("Use bash for file operations like ls, rg, find")
		}
	}
	for _, name := range tools {
		for _, rule := range toolGuidelines[name] {
			add(rule)
		}
	}
	for _, rule := range promptGuidelines {
		add(rule)
	}
	add("Be concise in your responses")
	add("Show file paths clearly when working with files")
	return out
}

// skillReadTool names the tool that loads skill files: the first of read and bash that is declared, "indirect" when only a hidden one is selected, and "" when neither is selected.
func skillReadTool(declared, selected []string) string {
	for _, name := range []string{"read", "bash"} {
		if slices.Contains(declared, name) {
			return name
		}
	}
	for _, name := range []string{"read", "bash"} {
		if slices.Contains(selected, name) {
			return "indirect"
		}
	}
	return ""
}

var skillXMLEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")

// formatSkills lists the model-visible skills as the Agent Skills XML, in order, with every field XML-escaped.
//
// upstream: packages/coding-agent/src/core/skills.ts formatSkillsForPrompt
func formatSkills(skills []Skill, readTool string) string {
	var visible []Skill
	for _, skill := range skills {
		if !skill.DisableModelInvocation {
			visible = append(visible, skill)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	load := "Use the read tool to load a skill's file when the task matches its description."
	switch readTool {
	case "bash":
		load = "Use bash to load a skill's file when the task matches its description."
	case "indirect":
		load = "Load a skill's file when the task matches its description."
	}
	lines := []string{
		"\n\nThe following skills provide specialized instructions for specific tasks.",
		load,
		"Resolve relative paths against the skill's directory (parent of SKILL.md). Use absolute paths in tool calls.",
		"",
		"<available_skills>",
	}
	for _, skill := range visible {
		lines = append(lines,
			"  <skill>",
			"    <name>"+skillXMLEscaper.Replace(skill.Name)+"</name>",
			"    <description>"+skillXMLEscaper.Replace(skill.Description)+"</description>",
			"    <location>"+skillXMLEscaper.Replace(skill.Path)+"</location>",
			"  </skill>",
		)
	}
	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}
