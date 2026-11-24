// SPDX-License-Identifier: MIT

package coding

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/compaction"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// Ports packages/coding-agent/src/core/agent-session.ts (_getSummarizationRequestAuth).
type summarizationRequest struct {
	model *ai.Model
	// apiKey, headers and env are the request auth of the summary model; headers omit the ones the auth deletes.
	apiKey    string
	headers   map[string]string
	env       map[string]string
	completer compaction.SimpleCompleter
	streamFn  compaction.StreamFn
	// thinkingLevel is the thinking level of the summary request: the session's, or the router's for a virtual selection.
	thinkingLevel ai.ModelThinkingLevel
}

type compactionFallbackState struct {
	current    modelroles.Selector
	effort     ai.ModelThinkingLevel
	ownerRole  string
	ownerChain string
	tried      map[string]struct{}
}

// compactionModelCollector records the distinct provider/model identities
// one Compact attempt's successful summary responses showed, in encounter
// order. The collector is attempt-local: a failed attempt's identities are
// discarded with it, so fallback losers never reach the completed record.
type compactionModelCollector struct {
	models []string
}

// observe records the identity of a validated summary response: its reported
// (responseModel) or declared model wins over the resolved request identity,
// and a missing provider falls back to the request's.
func (c *compactionModelCollector) observe(request *ai.Model, message *ai.AssistantMessage) {
	provider := message.Provider
	if provider == "" {
		provider = providerID(request)
	}
	model := message.ResponseModel
	if model == "" {
		model = message.Model
	}
	if model == "" && request != nil {
		model = request.ID
	}
	c.add(compactionModelIdentity(provider, model))
}

// addRequest records the resolved request identity, the only attribution a
// text-only injected completer or stream function can carry.
func (c *compactionModelCollector) addRequest(request *ai.Model) {
	c.add(compactionModelIdentity(providerID(request), request.ID))
}

func (c *compactionModelCollector) add(identity string) {
	if identity == "" || slices.Contains(c.models, identity) {
		return
	}
	c.models = append(c.models, identity)
}

func (c *compactionModelCollector) identities() []string {
	if len(c.models) == 0 {
		return nil
	}
	return slices.Clone(c.models)
}

// compactionModelIdentity joins provider and model into the display identity
// used by /session and compaction cards.
func compactionModelIdentity(provider, model string) string {
	if provider == "" {
		return model
	}
	if model == "" {
		return provider
	}
	return provider + "/" + model
}

func (s *Session) runDefaultCompaction(ctx context.Context, prep compaction.CompactionPreparation, model *ai.Model, customInstructions, reason string) (compaction.CompactionResult, error) {
	compactionModel, err := s.compactionModel(model)
	if err != nil {
		return compaction.CompactionResult{}, err
	}

	config, chains, _, lookup := s.fallbackContext()
	settings := s.SettingsManager().Get()
	ownerRole := s.ActiveModelRole()
	compactAssigned := settings.ModelRoles["compact"] != ""
	if compactAssigned {
		ownerRole = "compact"
	}
	current := modelroles.Selector{
		Provider: providerID(compactionModel),
		ID:       compactionModel.ID,
		Upstream: ai.ModelUpstreamRoute(compactionModel),
	}
	effort := s.ThinkingLevel()
	if compactAssigned {
		selector, resolveErr := s.SettingsManager().ResolveModelRole("@compact")
		if resolveErr != nil {
			return compaction.CompactionResult{}, resolveErr
		}
		_, configuredEffort := modelroles.Split(selector)
		if configuredEffort != "" && configuredEffort != "auto" && configuredEffort != "inherit" {
			effort = ai.ModelThinkingLevel(configuredEffort)
		}
	}
	effort = ai.ClampThinkingLevel(compactionModel, effort)
	current.Thinking = string(effort)
	if !compactAssigned {
		if _, configured := chains["compact"]; configured {
			ownerRole = "compact"
		}
	}
	ownerChain := modelroles.ResolveChainKey(config, chains, ownerRole, current.String(), lookup)
	state := compactionFallbackState{
		current:    current,
		effort:     effort,
		ownerRole:  ownerRole,
		ownerChain: ownerChain,
		tried:      map[string]struct{}{current.String(): {}},
	}
	request, err := s.prepareSummarizationRequest(ctx, compactionModel, state.effort)
	if err != nil {
		return compaction.CompactionResult{}, err
	}
	state.effort = request.thinkingLevel
	state.current.Thinking = string(state.effort)
	state.tried[state.current.String()] = struct{}{}

	for {
		retry, retryCallbacks := s.summarizationRetryOptions("compaction", reason, compactionModelLabel(request.model))
		// upstream compact(preparation, model, apiKey, headers, customInstructions, signal, thinkingLevel, this.agent.streamFunction, env, retry, callbacks, sessionId): request.completer holds the Agent's stream function.
		streamFn := request.streamFn
		var identities compactionModelCollector
		if streamFn == nil {
			if providerCompleter, ok := request.completer.(compaction.ProviderCompleter); ok {
				providerCompleter.OnComplete = identities.observe
				streamFn = compaction.CompleterStreamFn(providerCompleter)
			} else if providerCompleter, ok := request.completer.(*compaction.ProviderCompleter); ok && providerCompleter != nil {
				observed := *providerCompleter
				observed.OnComplete = identities.observe
				streamFn = compaction.CompleterStreamFn(observed)
			} else {
				streamFn = compaction.CompleterStreamFn(request.completer)
			}
		}
		result, compactErr := compaction.Compact(ctx, prep, request.model, request.apiKey, request.headers, customInstructions, request.thinkingLevel, streamFn, request.env, retry, retryCallbacks, "")
		if compactErr == nil {
			// A completer or stream override that reports no response identity
			// attributes the summary to the resolved request that produced it.
			if len(identities.models) == 0 {
				identities.addRequest(request.model)
			}
			result.Models = identities.identities()
			return result, nil
		}
		if ctx.Err() != nil || errors.Is(compactErr, context.Canceled) || errors.Is(compactErr, context.DeadlineExceeded) || ai.IsContextOverflow(ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: compactErr.Error()}, 0) {
			return result, compactErr
		}
		policy := s.SettingsManager().Get().Retry
		if !s.SettingsManager().GetRetryEnabled() || policy != nil && policy.ModelFallback != nil && !*policy.ModelFallback {
			return result, compactErr
		}
		usageLimit := isModelUsageLimit(compactErr.Error())
		if policy != nil && (policy.FallbackOn == "usage-limit" && !usageLimit || policy.FallbackOn == "except-usage-limit" && usageLimit) {
			return result, compactErr
		}

		keys := []string{}
		if state.ownerChain != "" {
			keys = append(keys, state.ownerChain)
		}
		modelChain := modelroles.ResolveChainKey(config, chains, state.ownerRole, state.current.String(), lookup)
		if modelChain != "" && modelChain != state.ownerChain {
			keys = append(keys, modelChain)
		}
		found := false
		for _, key := range keys {
			for _, candidate := range s.fallbackCandidates(key, state.current.String(), false) {
				target := s.modelRuntime.GetModel(candidate.Provider, candidate.ID)
				if target == nil {
					continue
				}
				if candidate.Upstream != "" {
					target, err = ai.WithModelUpstreamRoute(target, candidate.Upstream)
					if err != nil {
						continue
					}
				}
				effort := state.effort
				if candidate.Thinking != "" && candidate.Thinking != "auto" && candidate.Thinking != "inherit" {
					effort = ai.ModelThinkingLevel(candidate.Thinking)
				}
				effort = ai.ClampThinkingLevel(target, effort)
				candidate.Thinking = string(effort)
				identity := candidate.String()
				if _, exists := state.tried[identity]; exists {
					continue
				}
				state.tried[identity] = struct{}{}
				if authErr := s.checkModelAuth(ctx, target); authErr != nil {
					if ctx.Err() != nil {
						return result, authErr
					}
					continue
				}
				nextRequest, prepareErr := s.prepareSummarizationRequest(ctx, target, effort)
				if prepareErr != nil {
					if ctx.Err() != nil || errors.Is(prepareErr, context.Canceled) || errors.Is(prepareErr, context.DeadlineExceeded) {
						return result, prepareErr
					}
					continue
				}
				state.current = candidate
				state.effort = nextRequest.thinkingLevel
				state.current.Thinking = string(state.effort)
				state.tried[state.current.String()] = struct{}{}
				request = nextRequest
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return result, compactErr
		}
	}
}

// compactionModel returns the assigned compact role model when one exists. An
// unassigned role preserves the historical behavior of compacting with the
// active session model; assigning @compact is therefore an opt-in override.
func (s *Session) compactionModel(fallback *ai.Model) (*ai.Model, error) {
	if s.SettingsManager().Get().ModelRoles["compact"] == "" {
		return fallback, nil
	}
	selector, err := s.SettingsManager().ResolveModelRole("@compact")
	if err != nil {
		return nil, err
	}
	return BuildModel(selector, s.services)
}

// compactionModelLabel identifies the concrete model used for a summary in the
// interactive status line. DisplayName is the user-facing catalog name; ID is
// the stable fallback for custom models without one.
func compactionModelLabel(model *ai.Model) string {
	if model == nil {
		return ""
	}
	if model.DisplayName != "" {
		return model.DisplayName
	}
	return model.ID
}

// withoutDeletedHeaders drops the headers an auth result deletes; summary requests take only header values.
// upstream: packages/coding-agent/src/core/agent-session.ts:withoutDeletedHeaders
func withoutDeletedHeaders(headers ai.ProviderHeaders) map[string]string {
	if headers == nil {
		return nil
	}
	values := make(map[string]string, len(headers))
	for name, value := range headers {
		if value != nil {
			values[name] = *value
		}
	}
	return values
}

// prepareSummarizationRequest resolves the summary model and its request auth (agent-session.ts:560 _getSummarizationRequestAuth).
func (s *Session) prepareSummarizationRequest(ctx context.Context, model *ai.Model, thinkingLevels ...ai.ModelThinkingLevel) (summarizationRequest, error) {
	thinking := s.ThinkingLevel()
	if len(thinkingLevels) > 0 {
		thinking = thinkingLevels[0]
	}
	// Pi's getAuth rejects with the signal's reason when the signal is already aborted (resolve.ts:33-44), before any request exists. A caller-owned Go provider has no getAuth: it is not called once the signal has aborted, with the same reason (pinned by TestCompactCancelsSynchronouslyFromStartEvent).
	if ctx.Err() != nil {
		return summarizationRequest{model: model, thinkingLevel: thinking}, abortSignalReason(ctx)
	}
	// Route a virtual model first: summaries size their input and output from the model they get. upstream: agent-session.ts:541-548
	if IsVirtualModel(model) {
		route, err := s.modelRuntime.ResolveModel(ctx, model, agent.ConvertToLLM(agent.NormalizeMessages(s.agent.Messages(), model)), ResolveModelOptions{Reason: ModelRouteReasonDirect, ThinkingLevel: thinking})
		if err != nil {
			return summarizationRequest{model: model, thinkingLevel: thinking}, err
		}
		model, thinking = route.Model, route.ThinkingLevel
	}
	request := summarizationRequest{model: model, completer: s.resolveCompleter(), streamFn: s.streamFn, thinkingLevel: thinking}
	// A session CreateAgentSession built sends summaries through its own stream (sdk.ts streamFn, marked with markRuntimeStreamFunction),
	// which resolves auth through the Model Runtime as it does for prompts. Credentials resolved here would reach the runtime as an
	// explicit API key instead of the stored OAuth login (#11520). The check runs after virtual-model routing. A caller's completer,
	// streamFn or replaced agent stream function keeps the pre-resolved credentials.
	if s.runtimeStream && s.completer == nil && s.streamFn == nil && s.agent.StreamFunction() == nil {
		return request, nil
	}
	// Registry-built providers use the stock request path. A directly supplied Go provider is a caller-owned stream, even when it carries provider identity metadata.
	_, registryBuilt := model.Provider.(*requestOptionsProvider)
	_, nativeAuth := model.Provider.(interface{ Auth() ai.ProviderAuth })
	prepared, provider, options := model, model.Provider, ai.StreamOptions{Headers: ai.ProviderHeadersFromStrings(model.ProviderMeta.Headers)}
	var err error
	if registryBuilt || nativeAuth {
		prepared, provider, options, err = s.modelRuntime.prepare(ctx, model, ai.StreamOptions{})
		if ctx.Err() != nil {
			// Pi's getAuth races its signal and rejects with the signal's reason (resolve.ts:33-44), which _getSummarizationRequestAuth rethrows.
			return request, abortSignalReason(ctx)
		}
	}
	if err != nil {
		if (s.streamFn != nil || s.agent.StreamFunction() != nil || !registryBuilt) && ctx.Err() == nil {
			return request, nil
		}
		if missing, ok := errors.AsType[*modelRuntimeAuthMissingError](err); ok {
			return request, errors.New(icodingagent.FormatNoAPIKeyFoundMessage(missing.provider))
		}
		return request, err
	}
	clone := *prepared
	clone.Provider = provider
	request.model = &clone
	request.apiKey, request.headers, request.env = options.APIKey, withoutDeletedHeaders(options.Headers), maps.Clone(options.Env)
	return request, nil
}

// errOperationAborted is DOMException AbortError's message: the reason a plainly aborted AbortSignal rejects with.
var errOperationAborted = errors.New("This operation was aborted")

// abortSignalReason is the reason an aborted signal rejects with: AbortError for a plain abort, TimeoutError for AbortSignal.timeout(), or the caller's cause.
func abortSignalReason(ctx context.Context) error {
	cause := context.Cause(ctx)
	switch {
	case cause == nil || errors.Is(cause, context.Canceled):
		return errOperationAborted
	case errors.Is(cause, context.DeadlineExceeded):
		return errors.New("The operation was aborted due to timeout")
	}
	return cause
}
