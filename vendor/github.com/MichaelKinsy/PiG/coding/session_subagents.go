package coding

// ompig-slim built-in: native host for tintinweb/pi-subagents. Child sessions
// use the same model runtime and account router, with their own agent and log.
import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/packagemanager"
	"github.com/MichaelKinsy/PiG/internal/subagents"
)

func (s *Session) Subagents() *subagents.Manager { return s.subagents }

type subagentSession struct{ *Session }

func (s subagentSession) Subscribe(fn func(agent.AgentEvent)) func() {
	return s.Session.Subscribe(AgentSessionEventListener(fn))
}

func (s *Session) initSubagents(opts SessionOptions) error {
	if opts.subagentChild {
		return nil
	}
	if opts.BuiltinEventBuses != nil {
		s.builtinEventBuses = opts.BuiltinEventBuses
	}
	if len(s.builtinEventBuses) == 0 {
		if loader, ok := s.ResourceLoader().(*DefaultResourceLoader); ok {
			if loader.eventBus != nil {
				s.builtinEventBuses = append(s.builtinEventBuses, loader.eventBus)
			}
			if loader.files.host != nil {
				s.builtinEventBuses = append(s.builtinEventBuses, loader.files.host)
			}
		}
	}
	s.subagents = subagents.New(subagents.Host{CWD: s.CWD(), AgentDir: s.services.AgentDir(), SessionID: s.ID(), SessionDir: s.sessionDir, Create: s.createSubagent, History: s.Messages,
		Notify: func(text string) {
			_ = s.SendMessage(extension.CustomMessageRef{CustomType: "subagents:notification", Content: text, Display: true}, &extension.SendMessageOptions{TriggerTurn: new(true), DeliverAs: extension.DeliverAsSteer})
		},
		ParentRunning: s.IsStreaming,
		Mention:       s.orchestrateAgentMention,
		Subscribe:     func(fn func(agent.AgentEvent)) func() { return s.Subscribe(AgentSessionEventListener(fn)) },
		InstalledPackages: func() []subagents.PackageRoot {
			manager := packagemanager.NewPackageManager(packagemanager.PackageManagerOptions{CWD: s.CWD(), AgentDir: s.services.AgentDir(), SettingsManager: s.services.SettingsManager()})
			var roots []subagents.PackageRoot
			for _, pkg := range manager.ListConfiguredPackages() {
				if pkg.InstalledPath != "" {
					roots = append(roots, subagents.PackageRoot{Path: pkg.InstalledPath, Source: pkg.Source.Source})
				}
			}
			return roots
		},
		Persist: func(r subagents.Record) error { _, err := s.inner.AppendCustomEntry("subagents:record", r); return err },
		Restore: func() []subagents.Record {
			var out []subagents.Record
			index := map[string]int{}
			for _, entry := range s.inner.GetBranch() {
				if c, ok := entry.(icodingagent.CustomEntry); ok && c.CustomType == "subagents:record" {
					b, err := json.Marshal(c.Data)
					if err != nil {
						continue
					}
					var r subagents.Record
					if json.Unmarshal(b, &r) != nil || r.ID == "" {
						continue
					}
					if i, ok := index[r.ID]; ok {
						out[i] = r
					} else {
						index[r.ID] = len(out)
						out = append(out, r)
					}
				}
			}
			return out
		},
		Event: func(name string, data any) {
			for _, bus := range s.builtinEventBuses {
				bus.Emit(name, data)
			}
		},
		RefreshTools: s.syncSubagentTools,
	})
	s.subagentToolsAllowed = opts.NoTools != "all" && !opts.SkipBuiltinTools && opts.NoTools != "builtin"
	if opts.subagentBuiltinSelection != nil {
		s.subagentToolsAllowed = *opts.subagentBuiltinSelection
	}
	var unsubscribes []func()
	for _, bus := range s.builtinEventBuses {
		unsubscribes = append(unsubscribes, s.subagents.BindRPC(bus))
	}
	s.subagentRPCUnsubscribe = func() {
		for _, off := range unsubscribes {
			off()
		}
	}

	return s.syncSubagentTools()
}

func (s *Session) orchestrateAgentMention(ctx context.Context, agentType, message string) error {
	clone, err := NewSession(s.services, SessionOptions{Model: s.Model(), ThinkingLevel: s.ThinkingLevel(), ResourceLoader: NoResources, SystemPrompt: s.agent.SystemPrompt(), Tools: s.Tools(), SkipBuiltinTools: true, NoSession: true, subagentChild: true})
	if err != nil {
		return err
	}
	defer clone.Close()
	for _, msg := range s.Messages() {
		if msg.Role() == "system" {
			continue
		}
		if _, err = clone.inner.AppendMessage(msg); err != nil {
			return err
		}
	}
	clone.refreshContext()
	prompt := fmt.Sprintf("<agent_mention>\nThe user addressed @%s with this message:\n%s\nDelegate it now by calling Agent with subagent_type=%q and run_in_background=true. Write a self-contained task using the conversation context. Do not answer the user's task yourself.\n</agent_mention>", agentType, message, agentType)
	_, err = clone.Send(ctx, prompt)
	return err
}

func (s *Session) syncSubagentTools() error {
	if s.subagents == nil {
		return nil
	}
	settings := s.subagents.Settings()
	competing := false
	if runner := s.currentRunner(); runner != nil {
		for _, tool := range runner.Tools() {
			if tool.Definition.Name == "Workflow" || tool.Definition.Name == "SubagentWorkflow" {
				competing = true
			}
		}
	}
	s.toolRegistryMu.Lock()
	for _, entry := range append(slices.Clone(s.toolRegistry.base), s.toolRegistry.custom...) {
		if _, builtin := entry.tool.(*subagents.Tool); !builtin && (entry.tool.Name() == "Workflow" || entry.tool.Name() == "SubagentWorkflow") {
			competing = true
		}
	}
	if competing && !settings.WorkflowsExplicit {
		s.subagents.DisableConflictingWorkflows()
		settings.WorkflowsEnabled = false
	}
	s.toolRegistry.base = slices.DeleteFunc(s.toolRegistry.base, func(e sessionToolEntry) bool { _, builtin := e.tool.(*subagents.Tool); return builtin })
	if settings.Enabled && s.subagentToolsAllowed {
		for _, name := range subagents.ToolNames {
			if name == "SubagentWorkflow" && !settings.WorkflowsEnabled {
				continue
			}
			tool := &subagents.Tool{Manager: s.subagents, ToolName: name}
			definition, err := toolDefinition(tool)
			if err != nil {
				s.toolRegistryMu.Unlock()
				return err
			}
			definition = icodingagent.WithSubagentRenderers(definition)
			s.toolRegistry.base = append(s.toolRegistry.base, sessionToolEntry{tool: tool, registration: extension.RegisteredTool{Definition: definition, SourceInfo: syntheticToolSource(name, "builtin")}})
		}
	}
	s.toolRegistryMu.Unlock()
	return s.RefreshTools()
}

func (s *Session) createSubagent(ctx context.Context, inv subagents.Invocation) (subagents.Child, error) {
	model := s.Model()
	spec := inv.Params.Model
	if spec != "" && spec != "default" {
		var err error
		model, err = subagents.ResolveModel(spec, s.AvailableModelSnapshot())
		if err != nil {
			return nil, err
		}
		if s.subagents.Settings().ScopeModels && inv.Config.Model == "" {
			patterns := s.services.SettingsManager().GetEnabledModels()
			if len(patterns) > 0 {
				allowed := false
				key := model.ProviderMeta.ProviderID + "/" + model.ID
				for _, p := range patterns {
					match := extension.ToolNameMatcher(map[string]struct{}{p: {}})
					if match(key) || match(model.ID) {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil, fmt.Errorf("model %s is outside enabledModels", key)
				}
			}
		}
	}
	if model == nil {
		return nil, fmt.Errorf("select a model before starting subagents")
	}
	services := &AgentSessionServices{cwd: inv.CWD, agentDir: s.services.AgentDir(), credentials: s.services.Credentials(), settings: s.services.SettingsManager(), registry: s.modelRegistry, modelRuntime: s.modelRuntime, sharedRuntime: true}
	memory, err := subagents.MemoryPrompt(s.CWD(), s.services.AgentDir(), inv.Config)
	if err != nil {
		return nil, err
	}
	extras := memory
	for _, skill := range s.ResourceLoader().GetSkills().Skills {
		if subagents.Inherits(inv.Config.Skills, skill.Name) && !skill.DisableModelInvocation {
			if _, preload := inv.Config.Skills.(bool); preload || inv.Config.Skills == nil {
				extras += fmt.Sprintf("\n\nAvailable skill: %s — %s\nRead %s before using it.", skill.Name, skill.Description, skill.FilePath)
			} else {
				extras += "\n# Preloaded Skill: " + skill.Name + "\n" + skill.Body
			}
		}
	}
	worktreeBase := ""
	if inv.Params.Isolation == "worktree" {
		worktreeBase = s.CWD()
	}
	prompt := subagents.BuildPrompt(ctx, inv.Config, inv.CWD, subagents.InheritableParentPrompt(s.agent.SystemPrompt(), s.CWD()), worktreeBase, inv.WorkflowID != "" && len(inv.Params.Schema) == 0, extras)
	var toolList []agent.AgentTool
	if inv.Params.Isolated == nil || !*inv.Params.Isolated {
		if inv.Config.Extensions != false {
			s.toolRegistryMu.RLock()
			for _, entry := range s.toolRegistry.entries {
				source := entry.registration.SourceInfo
				extName := entry.registration.Source
				if extName == "" {
					extName = strings.TrimSuffix(filepath.Base(source.Path), filepath.Ext(source.Path))
				}
				admitted := subagents.Inherits(inv.Config.Extensions, extName) && !slices.ContainsFunc(inv.Config.ExcludeExtensions, func(name string) bool { return strings.EqualFold(name, extName) })
				if len(inv.Config.ExtSelectors) > 0 {
					admitted = admitted && slices.ContainsFunc(inv.Config.ExtSelectors, func(selector string) bool {
						return strings.EqualFold(selector, extName) || strings.EqualFold(selector, extName+"/"+entry.tool.Name())
					})
				}
				if source.Source != "builtin" && !subagents.IsTool(entry.tool.Name()) && admitted {
					toolList = append(toolList, entry.tool)
				}
			}
			s.toolRegistryMu.RUnlock()
		}
	}
	var allowed map[string]struct{}
	if inv.Config.Tools != nil {
		allowed = map[string]struct{}{}
		for _, name := range inv.Config.Tools {
			allowed[name] = struct{}{}
		}
		for _, tool := range toolList {
			if !extension.IsMcpToolName(tool.Name()) {
				allowed[tool.Name()] = struct{}{}
			}
		}
	}
	excluded := map[string]struct{}{}
	for _, name := range inv.Config.DisallowedTools {
		excluded[name] = struct{}{}
	}
	if inv.Depth < s.subagents.Settings().MaxSubagentDepth && len(inv.Config.AllowedSubagents) > 0 {
		for _, name := range []string{"Agent", "get_subagent_result", "steer_subagent"} {
			toolList = append(toolList, &subagents.Tool{Manager: s.subagents, ToolName: name, Owner: inv.ID})
			if allowed != nil {
				allowed[name] = struct{}{}
			}
		}
	}
	if len(inv.Params.Schema) > 0 {
		toolList = append(toolList, &subagentStructuredTool{manager: s.subagents, id: inv.ID, schema: inv.Params.Schema})
		if allowed != nil {
			allowed["StructuredOutput"] = struct{}{}
		}
		prompt += "\n\nReturn your result by calling StructuredOutput with a payload matching its JSON Schema. Your final prose is not a substitute for that tool."
	}
	initial := inv.Config.Tools
	if initial == nil {
		initial = []string{"read", "bash", "edit", "write", "grep", "find", "ls", "codemode"}
	}
	dir := filepath.Join(s.sessionDir, "subagents", s.ID())
	if inv.Config.SessionDir != "" {
		dir = inv.Config.SessionDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(s.CWD(), dir)
		}
	}
	childOpts := SessionOptions{Model: model, ThinkingLevel: ai.ModelThinkingLevel(inv.Params.Thinking), ResourceLoader: NoResources, SystemPrompt: prompt, Tools: toolList, AllowedTools: allowed, ExcludedTools: excluded, InitialActiveToolNames: initial, SessionDir: dir, subagentChild: true}
	if childOpts.ThinkingLevel == "" {
		childOpts.ThinkingLevel = s.ThinkingLevel()
	}
	if inv.Params.Resume != "" {
		records := s.subagents.Records()
		for _, r := range records {
			if r.ID == inv.ID {
				childOpts.ResumePath = r.SessionFile
				break
			}
		}
	}
	if inv.Config.PersistSession != nil && !*inv.Config.PersistSession {
		childOpts.NoSession = true
	}
	child, err := NewSession(services, childOpts)
	if err != nil {
		return nil, err
	}
	if len(inv.History) > 0 && inv.Params.Resume == "" {
		for _, msg := range inv.History {
			if msg.Role() == "system" {
				continue
			}
			if _, err = child.inner.AppendMessage(msg); err != nil {
				child.Close()
				return nil, err
			}
		}
		child.refreshContext()
	}
	s.subagents.SetInvocation(inv.ID, model.DisplayName, model.Capabilities.ContextWindow, string(child.ThinkingLevel()))
	return subagentSession{child}, nil
}

type subagentStructuredTool struct {
	manager *subagents.Manager
	id      string
	schema  json.RawMessage
}

func (t *subagentStructuredTool) Name() string  { return "StructuredOutput" }
func (t *subagentStructuredTool) Label() string { return "Structured Output" }
func (t *subagentStructuredTool) ExecutionMode() agent.ToolExecutionMode {
	return agent.ToolModeSequential
}
func (t *subagentStructuredTool) Schema() ai.ToolSchema {
	var params map[string]any
	_ = json.Unmarshal(t.schema, &params)
	return ai.ToolSchema{Name: t.Name(), Description: "Return the final result matching this JSON Schema.", Parameters: params}
}
func (t *subagentStructuredTool) Execute(ctx context.Context, id string, raw json.RawMessage, update agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	t.manager.SetStructured(t.id, raw)
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Structured output accepted."}}, Terminate: true}, nil
}
