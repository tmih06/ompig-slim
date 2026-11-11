package tools

// Built-in tools that are not plain AgentTools: their registration carries the extension metadata a plain tool has no
// place for (prepareLoadout, exposure, reachesTools, output schema and renderers). Upstream builds these in core/tools
// (tools/index.ts createAllToolDefinitions spreads codemodeRenderers into the codemode definition), so they exist whether
// or not any extension loads: `codemode` stays available with --no-extensions.
//
// Go mechanic (not a divergence): upstream's core/tools package has no equivalent of this registry, because its built-in
// definitions are importable from anywhere. PiG's codemode implementation lives in coding/extension/builtin/codemode and
// imports internal/codingagent, which imports this package, so the definition builder registers itself here from that
// package (the compiledin pattern). A build that links no codemode (pig_strip_codemode) registers nothing, as upstream's
// stripped builds do.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/configroot"
	"github.com/MichaelKinsy/PiG/internal/pigstrip"
)

// ExtendedBuiltinOptions are the per-session inputs an extended built-in definition is built with.
type ExtendedBuiltinOptions struct {
	// CWD is the session's working directory.
	CWD string
	// Settings returns the effective settings, read on every use. Nil means the tool's defaults.
	Settings func() extension.Settings
	// CacheDir holds a sandbox's on-disk compilation cache; empty compiles in memory.
	CacheDir string
}

// ExtendedBuiltinTool is one extended built-in: its full definition, which a session registers as a built-in tool.
type ExtendedBuiltinTool struct {
	Definition extension.ToolDefinition
}

// ExtendedBuiltinFactory builds an extended built-in definition for a session. A definition is built from source
// literals, so a factory does not fail.
type ExtendedBuiltinFactory func(options ExtendedBuiltinOptions) ExtendedBuiltinTool

type registeredExtendedBuiltin struct {
	name    string
	factory ExtendedBuiltinFactory
}

// extendedBuiltins holds the registered extended built-ins in registration order. The list is short (one entry), so a
// slice is searched; a map would allocate its table at the first registration.
var extendedBuiltins []registeredExtendedBuiltin

// RegisterExtendedBuiltinTool records the factory of an extended built-in tool, replacing an earlier one of that name.
func RegisterExtendedBuiltinTool(name string, factory ExtendedBuiltinFactory) {
	// A new factory invalidates the cached static definition.
	staticMu.Lock()
	staticDefinitions = nil
	staticMu.Unlock()
	for i := range extendedBuiltins {
		if extendedBuiltins[i].name == name {
			extendedBuiltins[i].factory = factory
			return
		}
	}
	extendedBuiltins = append(extendedBuiltins, registeredExtendedBuiltin{name, factory})
}

// staticDefinitions caches the definition of each extended built-in built with no session options: the metadata every
// host callback reads (description, parameters, exposure, reachesTools) is the same for every session, and the codemode
// description is expensive to build. staticMu guards it; the map is small (one entry).
var (
	staticMu          sync.Mutex
	staticDefinitions map[string]extension.ToolDefinition
)

// StaticExtendedBuiltinDefinition returns the definition of the extended built-in name built with no session options,
// cached: the exposure, reachesTools, parameters and description of a tool that are not the session's. The zero
// definition means no build linked it.
func StaticExtendedBuiltinDefinition(name string) extension.ToolDefinition {
	factory, ok := extendedBuiltinFactory(name)
	if !ok {
		return extension.ToolDefinition{}
	}
	staticMu.Lock()
	defer staticMu.Unlock()
	if definition, cached := staticDefinitions[name]; cached {
		return definition
	}
	definition := factory(ExtendedBuiltinOptions{}).Definition
	if staticDefinitions == nil {
		staticDefinitions = map[string]extension.ToolDefinition{}
	}
	staticDefinitions[name] = definition
	return definition
}

// extendedBuiltinFactory returns the factory of name. A runtime strip of the built-in (a Piglet's strip list) removes it
// from every registry, as the strip of an extension removes its row.
func extendedBuiltinFactory(name string) (ExtendedBuiltinFactory, bool) {
	if pigstrip.Has(pigstrip.ListExtensions, name) {
		return nil, false
	}
	for _, entry := range extendedBuiltins {
		if entry.name == name {
			return entry.factory, true
		}
	}
	return nil, false
}

// extendedBuiltinNames returns the registered extended built-in names a build keeps, in registration order.
func extendedBuiltinNames() []string {
	names := make([]string, 0, len(extendedBuiltins))
	for _, entry := range extendedBuiltins {
		if !pigstrip.Has(pigstrip.ListExtensions, entry.name) {
			names = append(names, entry.name)
		}
	}
	return names
}

// IsExtendedBuiltinTool reports whether name is a registered extended built-in.
func IsExtendedBuiltinTool(name string) bool {
	_, ok := extendedBuiltinFactory(name)
	return ok
}

// CreateExtendedBuiltinTools builds every registered extended built-in for a session, in registration order.
func CreateExtendedBuiltinTools(options ExtendedBuiltinOptions) []ExtendedBuiltinTool {
	out := make([]ExtendedBuiltinTool, 0, len(extendedBuiltins))
	for _, name := range extendedBuiltinNames() {
		factory, _ := extendedBuiltinFactory(name)
		out = append(out, factory(options))
	}
	return out
}

// ExtendedBuiltinDefinitions builds the registered extended built-ins by name, so a session registers each with the
// definition metadata the plain-tool projection cannot carry.
func ExtendedBuiltinDefinitions(options ExtendedBuiltinOptions) map[string]extension.ToolDefinition {
	definitions := make(map[string]extension.ToolDefinition, len(extendedBuiltins))
	for _, tool := range CreateExtendedBuiltinTools(options) {
		definitions[tool.Definition.Name] = tool.Definition
	}
	return definitions
}

// ExtendedBuiltinDefinition builds the definition of the extended built-in name, or the zero definition when no build
// linked it (a stripped codemode). A session that registers the tool reads this definition, not the projection the agent
// tool interface carries: prepareLoadout, exposure, reachesTools and the output schema live only on the definition.
func ExtendedBuiltinDefinition(name string, options ExtendedBuiltinOptions) extension.ToolDefinition {
	// No session inputs: the cached static definition carries every field the caller reads.
	if options.CWD == "" && options.CacheDir == "" && options.Settings == nil {
		return StaticExtendedBuiltinDefinition(name)
	}
	if factory, ok := extendedBuiltinFactory(name); ok {
		return factory(options).Definition
	}
	return extension.ToolDefinition{}
}

// WazeroCacheDir is where the codemode sandbox keeps wazero's compilation cache: `cache/wazero` under PIG_HOME, else
// under the user cache directory. It is an optimization only; an unusable directory is ignored, and so is an empty
// result.
func WazeroCacheDir() string {
	if os.Getenv("PIG_HOME") != "" {
		if root, err := configroot.Resolve(); err == nil {
			return filepath.Join(root, "cache", "wazero")
		}
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "ompig-slim", "wazero")
	}
	return ""
}

// DefinitionAgentTool adapts an extension tool definition to agent.AgentTool, so a definition-driven built-in can sit in
// a plain tool set. Unlike a registered extension tool it reports RunsInExtension false: its Execute runs in this process.
func DefinitionAgentTool(definition extension.ToolDefinition) agent.AgentTool {
	var declaration ai.ToolSchema
	if len(definition.Parameters) > 0 {
		envelope, err := json.Marshal(struct {
			Parameters json.RawMessage `json:"parameters"`
		}{definition.Parameters})
		if err == nil {
			err = json.Unmarshal(envelope, &declaration)
		}
		if err != nil {
			// Parameters are source literals, not user input.
			panic(fmt.Sprintf("built-in tool %q: parse parameters: %v", definition.Name, err))
		}
	}
	tool := &definitionAgentTool{definition: definition, declaration: declaration}
	// Constrained sampling is immutable tool source, while Schema is read on every model turn: decode it once here.
	switch strings.TrimSpace(string(definition.ConstrainedSampling)) {
	case "", "null":
	case "false":
		tool.constrainedSamplingDisabled = true
	default:
		var config ai.ConstrainedSamplingConfig
		if err := json.Unmarshal(definition.ConstrainedSampling, &config); err != nil {
			panic(fmt.Sprintf("built-in tool %q: parse constrained sampling: %v", definition.Name, err))
		}
		tool.constrainedSampling = &config
	}
	return tool
}

type definitionAgentTool struct {
	definition          extension.ToolDefinition
	declaration         ai.ToolSchema
	constrainedSampling *ai.ConstrainedSamplingConfig
	// constrainedSamplingDisabled records an explicit `constrainedSampling: false`, kept for transcript declarations.
	constrainedSamplingDisabled bool
}

func (t *definitionAgentTool) Name() string  { return t.definition.Name }
func (t *definitionAgentTool) Label() string { return t.definition.Label }

// Schema returns the declaration the definition carries, with the parameters decoded once.
func (t *definitionAgentTool) Schema() ai.ToolSchema {
	schema := t.declaration
	schema.Name = t.definition.Name
	schema.Description = t.definition.Description
	schema.PromptGuidelines = t.definition.PromptGuidelines
	schema.ConstrainedSampling = t.constrainedSampling
	schema.ConstrainedSamplingDisabled = t.constrainedSamplingDisabled
	return schema
}

// OutputSchema is the schema of the structured content in successful results.
func (t *definitionAgentTool) OutputSchema() json.RawMessage { return t.definition.OutputSchema }

func (t *definitionAgentTool) Execute(ctx context.Context, toolCallID string, params json.RawMessage, onUpdate agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	if t.definition.Execute == nil {
		return agent.AgentToolResult{}, fmt.Errorf("built-in tool %q has no Execute", t.definition.Name)
	}
	return t.definition.Execute(ctx, toolCallID, params, onUpdate)
}

func (t *definitionAgentTool) ExecutionMode() agent.ToolExecutionMode {
	if t.definition.ExecutionMode == "sequential" {
		return agent.ToolModeSequential
	}
	return agent.ToolModeParallel
}

// PrepareArguments normalizes input before validation, as the definition does.
func (t *definitionAgentTool) PrepareArguments(params json.RawMessage) (json.RawMessage, error) {
	if t.definition.PrepareArguments == nil {
		return params, nil
	}
	return t.definition.PrepareArguments(params)
}
