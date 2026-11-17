package codemode

import "strings"

// The model-facing text of the tool, copied from packages/coding-agent/src/extensions/codemode/tool.ts.

// descriptionIntro is DESCRIPTION_INTRO.
const descriptionIntro = "Run JavaScript that calls other tools. The input is raw JavaScript (not JSON, no code fence), run as an async function body in a QuickJS sandbox: top-level `await` and `return` work. No Node, file system, network, or timers.\n" +
	"- `await tools.<name>({ ...args })` resolves to a string, or an object if the tool's declaration says so, and rejects with an Error on failure. A tool whose declaration returns an object resolves on failure too when it returned that object; check its fields. Calls still running when the script ends are cancelled.\n" +
	"- `todo` is direct-only. Call the todo tool directly, outside codemode; nested calls cannot observe intermediate transcript state. Tool discovery cannot make it callable.\n" +
	"- Optional first line: `// @options: {\"max_output_tokens\": 10000, \"timeout_ms\": 60000}`"

// describeGlobals lists the script globals, one line each. The details live in DocsPath.
//
// Ports packages/coding-agent/src/extensions/codemode/tool.ts (describeGlobals).
func describeGlobals(models bool) string {
	lines := []string{
		"Globals:",
		"- `text(value)`, `image(dataUrlOrImageBlock)`, `console.log(...)`, and top-level `return` add output; `exit()` ends the script. With several text items, each starts with a `==> text N/M <==` line, and `console` lines follow the other output in one `<console_output>` block. `image()` also saves the image to a temp file and the result names its path.",
		"- `store(key, value)` and `load(key)` keep JSON values across codemode calls.",
		"- `ALL_TOOLS`, `await searchTools(query, { limit?, namespace? })`, `await describeTool(name)`, `await describeNamespace(name)`: find unlisted tools, such as MCP tools.",
	}
	if models {
		lines = append(lines, "- `models`: inspect models with `getModelsOfType`, `getAvailableOfType`, and `getModelOfType`; generate images with `await models.generateImages(model, context)`.",
			"  Judge state with `await models.judge({state: {...}, questions: {check: {type: \"bool\", instructions: \"...\", criteria: {true: \"...\", false: \"...\"}}}})`. Questions may use `bool`, `choice` with label-to-meaning criteria, or `score` with ordered string levels; add `images` when needed. Read each answer as `{type, value, classifier?}`; native models may provide classifier metadata. The configured `judge` role and its fallbacks choose the judge model. Scripts cannot select its model or role, and `models.classify` is unavailable.")
	}
	return strings.Join(lines, "\n")
}
