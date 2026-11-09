package cli

// Ports packages/coding-agent/src/core/model-resolver.ts

// Startup resolves an explicit model/role, a resumed session, configured roles,
// then an authenticated saved/provider default or the first available model.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// startupModelRuntime is the ModelRuntime surface startup model selection
// reads: the composed model list and per-provider configured auth.
type startupModelRuntime struct {
	loaded  []codingagent.RuntimeModel
	load    func() []codingagent.RuntimeModel
	hasAuth func(providerID string) bool
	auth    map[string]bool
}

func newStartupModelRuntime(models []codingagent.RuntimeModel, hasAuth func(providerID string) bool) *startupModelRuntime {
	return &startupModelRuntime{loaded: models, hasAuth: hasAuth, auth: map[string]bool{}}
}

// newLazyStartupModelRuntime is newStartupModelRuntime over the models load lists when a selection first reads them: an explicit --model
// resolves through ModelRuntime and reads none (main.ts:803-819 reads modelRuntime only where a step needs it).
func newLazyStartupModelRuntime(load func() []codingagent.RuntimeModel, hasAuth func(providerID string) bool) *startupModelRuntime {
	return &startupModelRuntime{load: load, hasAuth: hasAuth, auth: map[string]bool{}}
}

// models returns the composed models, loading them on the first read.
func (rt *startupModelRuntime) models() []codingagent.RuntimeModel {
	if rt.load != nil {
		rt.loaded, rt.load = rt.load(), nil
	}
	return rt.loaded
}

// GetModels returns every composed model, or one provider's.
func (rt *startupModelRuntime) GetModels(providerID string) []codingagent.RuntimeModel {
	if providerID == "" {
		return slices.Clone(rt.models())
	}
	var models []codingagent.RuntimeModel
	for _, model := range rt.models() {
		if model.Provider == providerID {
			models = append(models, model)
		}
	}
	return models
}

// HasConfiguredAuth reports the provider's configured auth, read once.
func (rt *startupModelRuntime) HasConfiguredAuth(providerID string) bool {
	configured, ok := rt.auth[providerID]
	if !ok {
		configured = rt.hasAuth(providerID)
		rt.auth[providerID] = configured
	}
	return configured
}

// getModel mirrors upstream ModelRuntime.getModel: an exact composed model.
func (rt *startupModelRuntime) getModel(providerID, modelID string) *codingagent.RuntimeModel {
	models := rt.models()
	index := slices.IndexFunc(models, func(model codingagent.RuntimeModel) bool {
		return model.Provider == providerID && model.ID == modelID
	})
	if index < 0 {
		return nil
	}
	return &models[index]
}

// getAvailable mirrors upstream ModelRuntime.getAvailableSnapshot: the
// composed models whose provider has configured auth.
func (rt *startupModelRuntime) getAvailable() []codingagent.RuntimeModel {
	var available []codingagent.RuntimeModel
	for _, model := range rt.models() {
		if rt.HasConfiguredAuth(model.Provider) {
			available = append(available, model)
		}
	}
	return available
}

// GetModel returns an exact composed model without reading authentication.
func (rt *startupModelRuntime) GetModel(providerID, modelID string) *codingagent.RuntimeModel {
	return rt.getModel(providerID, modelID)
}

// GetAvailableSnapshot returns the existing startup adapter's available-model observation.
func (rt *startupModelRuntime) GetAvailableSnapshot() []codingagent.RuntimeModel {
	return rt.getAvailable()
}

// findInitialModel delegates startup's saved-default and available-model selection to the shared resolver. Session construction still owns startup thinking selection.
func findInitialModel(rt *startupModelRuntime, defaultProvider, defaultModelID string) (*codingagent.RuntimeModel, error) {
	result, err := codingagent.FindInitialModel(codingagent.FindInitialModelOptions{
		ModelRuntime: rt, DefaultProvider: defaultProvider, DefaultModelId: defaultModelID,
	})
	return result.Model, err
}

// startupModelOptions carries the CLI and session inputs startup model
// selection reads.
type startupModelOptions struct {
	CLIProvider string
	CLIModel    string
	CLIThinking string
	// SessionManager is the already-opened Session used for restoration before default-model fallback.
	SessionManager *coding.SessionManager
	// APIKey is --api-key: a non-persistent key for the selected model's
	// provider.
	APIKey string
}

// startupModel retains selection diagnostics separately from the resolved model.
// Fallback continues after explicit-model errors so metadata remains available.
type startupModel struct {
	Upstream             string
	Role                 string
	Model                *ai.Model
	Thinking             string
	Warnings             []string
	Errors               []error
	ModelFallbackMessage string
}

// selectStartupModel resolves explicit selection, then the loaded Session's model, then configured/provider defaults. It retains ordered errors in the result and returns their aggregate. A nil Model with nil error means no authenticated model is available.
func selectStartupModel(ctx context.Context, options startupModelOptions, settings codingagent.Settings, services *coding.AgentSessionServices) (startupModel, error) {
	// upstream: main.ts:803-819 resolves against modelRuntime, whose catalog and auth include the virtual models the services registered (agent-session-services.ts:182-193).
	modelRuntime := services.ModelRuntime()
	rt := newLazyStartupModelRuntime(modelRuntime.RuntimeModels, modelRuntime.HasConfiguredAuth)
	var result startupModel
	result.Role, _ = modelroles.Role(options.CLIModel)
	selected, err := selectSessionOptionModel(rt, modelRuntime, options, settings, &result)
	if err != nil {
		result.Errors = append(result.Errors, err)
	}
	if options.APIKey != "" {
		if selected == nil {
			result.Errors = append(result.Errors, errors.New("--api-key requires a model to be specified via --model or --provider/--model"))
		} else {
			// upstream: main.ts:835 awaits modelRuntime.setRuntimeApiKey, whose synchronization failure rejects.
			if err := modelRuntime.SetRuntimeApiKey(ctx, selected.Provider, options.APIKey); err != nil {
				result.Errors = append(result.Errors, err)
			}
		}
	}
	if selected == nil && options.SessionManager != nil {
		// sdk.ts:198-224 restores an authenticated saved model before consulting defaults.
		branch := options.SessionManager.GetContextBranch()
		existing := options.SessionManager.BuildSessionContext()
		// Assistant messages name the physical model that answered, so a virtual selection is only in model_change entries (sdk.ts:201-205).
		sessionModel := coding.GetBranchSelection(branch, func(provider, modelID string) *ai.Model {
			if model := rt.getModel(provider, modelID); model != nil && model.NativeModel != nil {
				return model.NativeModel
			}
			return nil
		})
		if len(existing.Messages) > 0 && sessionModel != nil {
			if !sessionModel.Unresolvable {
				if restored := rt.getModel(sessionModel.Provider, sessionModel.ModelID); restored != nil && rt.HasConfiguredAuth(restored.Provider) {
					selected = restored
				}
			}
			if selected == nil {
				result.ModelFallbackMessage = "Could not restore model " + sessionModel.Provider + "/" + sessionModel.ModelID
			}
		}
	}
	if selected == nil && settings.ModelRoles["default"] != "" {
		resolved, roleErr := settings.Config.Resolve("@default", settings.DefaultProvider+"/"+settings.DefaultModel)
		if roleErr != nil {
			result.Errors = append(result.Errors, roleErr)
		} else {
			chains := map[string][]string{}
			if settings.Retry != nil {
				chains = settings.Retry.FallbackChains
			}
			selectors := append([]string{resolved}, chains["default"]...)
			for _, selector := range selectors {
				value, e := settings.Config.Resolve(selector, settings.DefaultProvider+"/"+settings.DefaultModel)
				if e != nil {
					continue
				}
				base, level := modelroles.Split(value)
				parsed, ok := modelroles.ParseSelector(base, func(provider, id string) bool { return rt.getModel(provider, id) != nil })
				provider, id := parsed.Provider, parsed.ID
				if !ok {
					continue
				}
				candidate := rt.getModel(provider, id)
				if candidate != nil && rt.HasConfiguredAuth(provider) {
					selected = candidate
					result.Role = "default"
					result.Upstream = parsed.Upstream
					if level != "inherit" {
						result.Thinking = level
					}
					break
				}
			}
		}
	}
	if selected == nil {
		selected, err = findInitialModel(rt, settings.DefaultProvider, settings.DefaultModel)
		if err != nil {
			return result, err
		}
		if selected == nil {
			result.ModelFallbackMessage = codingagent.FormatNoModelsAvailableMessage()
			return result, errors.Join(result.Errors...)
		}
		if result.ModelFallbackMessage != "" {
			result.ModelFallbackMessage += ". Using " + selected.Provider + "/" + selected.ID
		}
	}
	result.Model, err = buildModelFromRef(ctx, selected.Provider, selected.ID, services)
	if err == nil && result.Model != nil && result.Upstream != "" {
		result.Model, err = ai.WithModelUpstreamRoute(result.Model, result.Upstream)
	}
	// A virtual model is the runtime's shared catalog entry and carries its reasoning from its definition (virtual-models.ts:159-176 createVirtualModel), so startup does not rewrite it.
	if err == nil && selected.Reasoning && result.Model != nil && !coding.IsVirtualModel(result.Model) {
		result.Model.ProviderMeta.Reasoning = true
		if result.Model.Capabilities.MaxThinking == "" {
			result.Model.Capabilities.MaxThinking = ai.ThinkingLevelHigh
		}
	}
	if err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result, errors.Join(result.Errors...)
}

// selectSessionOptionModel mirrors upstream main.ts buildSessionOptions: the
// explicit --model resolution. It records thinking and diagnostics in result.
func selectSessionOptionModel(rt *startupModelRuntime, modelRuntime *coding.ModelRuntime, options startupModelOptions, settings codingagent.Settings, result *startupModel) (*codingagent.RuntimeModel, error) {
	if model, thinking, ok := testFauxCLIModel(options); ok {
		result.Thinking = thinking
		return model, nil
	}
	var cliErr error
	// Pi 1.0.0 main.ts:469-474 (#10236): --provider only scopes --model, so it fails without one.
	if options.CLIProvider != "" && options.CLIModel == "" {
		cliErr = fmt.Errorf("--provider requires --model (for example: --provider %s --model <pattern>)", options.CLIProvider)
	}
	if options.CLIModel != "" {
		if _, isRole := modelroles.Role(options.CLIModel); isRole {
			value, err := settings.Config.Resolve(options.CLIModel, settings.DefaultProvider+"/"+settings.DefaultModel)
			if err != nil {
				return nil, err
			}
			base, level := modelroles.Split(value)
			options.CLIModel = base
			if options.CLIThinking == "" && level != "inherit" {
				options.CLIThinking = level
			}
		}
		if parsed, ok := modelroles.ParseSelector(options.CLIModel, func(provider, id string) bool { return rt.getModel(provider, id) != nil }); ok && parsed.Upstream != "" {
			options.CLIModel = parsed.Provider + "/" + parsed.ID
			result.Upstream = parsed.Upstream
			if options.CLIThinking == "" && parsed.Thinking != "" {
				options.CLIThinking = parsed.Thinking
			}
		}
		resolved := coding.ResolveCliModel(coding.ResolveCliModelOptions{CliProvider: options.CLIProvider, CliModel: options.CLIModel, CliThinking: ai.ThinkingLevel(options.CLIThinking), InlineApiKey: options.APIKey != "", ModelRuntime: modelRuntime})
		if resolved.Warning != "" {
			result.Warnings = append(result.Warnings, resolved.Warning)
		}
		if resolved.Error != "" {
			cliErr = errors.New(resolved.Error)
		}
		if resolved.Model != nil {
			result.Thinking = string(resolved.ThinkingLevel)
			// A role's explicit :off must not disappear merely because the catalog
			// model is non-reasoning; startup would otherwise reapply the global default.
			if result.Role != "" && result.Thinking == "" {
				result.Thinking = options.CLIThinking
			}
			return runtimeModelOf(resolved.Model), nil
		}
	}
	return nil, cliErr
}

// testFauxCLIModel selects the test-only test-faux provider for a --model naming it, without the registry resolution a real --model takes
// (ResolveCliModel, which builds every runtime model). It exists only when PIG_TEST_FAUX=1. The provider is registered in the model registry
// (registerHarnessTestFaux), so every consumer of the registry sees it as Pi's harness-registered one; only this selection is a shortcut, and
// the startup allocation ceilings (TestStartupAllocationBudget, test/perfgate), which only move down, are measured with it: the registry
// resolution adds about 6,100 allocations and 380 KB to the warm first prompt.
func testFauxCLIModel(options startupModelOptions) (*codingagent.RuntimeModel, string, bool) {
	if !coding.TestFauxProviderEnabled() {
		return nil, "", false
	}
	spec := options.CLIModel
	if options.CLIProvider == "test-faux" && !strings.HasPrefix(spec, "test-faux/") {
		spec = "test-faux/" + spec
	}
	modelID, ok := strings.CutPrefix(spec, "test-faux/")
	if !ok || modelID == "" {
		return nil, "", false
	}
	thinking := ""
	if colon := strings.LastIndex(modelID, ":"); colon != -1 && validThinkingLevels[modelID[colon+1:]] { //portlint:allow utf16units colon is the byte index of ASCII ':' (0x3A), which never occurs inside a multibyte UTF-8 sequence, so the slices split at the same character as lastIndexOf/substring; the suffix is only used when it equals a thinking level (test-faux-only shortcut)
		modelID, thinking = modelID[:colon], modelID[colon+1:] //portlint:allow utf16units colon is the byte index of ASCII ':' (0x3A), which never occurs inside a multibyte UTF-8 sequence, so the slices split at the same character as lastIndexOf/substring; the suffix is only used when it equals a thinking level (test-faux-only shortcut)
	}
	return &codingagent.RuntimeModel{Provider: "test-faux", ID: modelID, Name: "Test Faux"}, thinking, true
}

// runtimeModelOf is the selection metadata of a resolved model: what ModelRuntime.RuntimeModels carries for it.
func runtimeModelOf(model *ai.Model) *codingagent.RuntimeModel {
	return &codingagent.RuntimeModel{NativeModel: model, Provider: model.ProviderMeta.ProviderID, ID: model.ID, Name: model.DisplayName, Reasoning: model.ProviderMeta.Reasoning || model.Capabilities.MaxThinking != "", Headers: ai.ProviderHeadersFromStrings(model.ProviderMeta.Headers)}
}
