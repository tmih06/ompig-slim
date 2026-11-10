package coding

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// branchModelRole restores only the latest explicit selection and only when the
// physical identity still agrees. A direct model_change clears the previous role.
func branchModelRole(branch []icodingagent.SessionEntry, model *ai.Model) string {
	for _, entry := range slices.Backward(branch) {
		if entry.Base().Type != "model_change" {
			continue
		}
		var change struct {
			Role     string `json:"role"`
			Provider string `json:"provider"`
			ModelID  string `json:"modelId"`
		}
		if json.Unmarshal(entry.Raw(), &change) != nil {
			return ""
		}
		if model != nil && change.Provider+"/"+change.ModelID == ai.ModelSelector(model) {
			return change.Role
		}
		return ""
	}
	return ""
}

// AvailableModelSnapshot returns authenticated runtime models bound to their
// backends. Model roles, the picker and subagents share this unfiltered catalog.
func (s *Session) AvailableModelSnapshot() []*ai.Model { return s.modelRuntime.GetAvailableSnapshot() }

// ActiveModelRole reports the logical selection, not the provider reached by fallback.
func (s *Session) ActiveModelRole() string {
	if role := s.activeModelRole.Load(); role != nil {
		return *role
	}
	return ""
}

// ActiveModelRoleColor reports the color token for the active role, resolved
// through the same modelroles metadata the model manager paints role rows and
// browser chips with. One role therefore has one hue across the catalog and the
// footer, including user-authored modelTags overrides, which Config.RoleInfo
// merges. It reads the live settings on each call, so a tag edit repaints with
// the next frame. An empty role reports "" rather than the default role's color.
func (s *Session) ActiveModelRoleColor() string {
	role := s.ActiveModelRole()
	if role == "" {
		return ""
	}
	return s.SettingsManager().Get().Config.RoleInfo(role).Color
}

// beginRoleCycle keeps role identity even when two roles resolve to the same model.
// Unavailable roles are skipped; no configured candidates means no switch, not all models.
func (s *Session) beginRoleCycle(ctx context.Context, direction string, deferThinking bool, order []string, options ...ModelMutationOptions) (*ModelCycleResult, func(), error) {
	type candidate struct {
		role  string
		model *ai.Model
		level ai.ModelThinkingLevel
	}
	var candidates []candidate
	available := s.AvailableModelSnapshot()
	pool := make([]modelroles.Candidate, 0, len(available))
	for _, model := range available {
		pool = append(pool, modelroles.Candidate{Provider: providerID(model), ID: model.ID})
	}
	for _, role := range order {
		selector, err := s.SettingsManager().ResolveModelRole("@"+role, pool)
		if err != nil {
			continue
		}
		spec, level := modelroles.Split(selector)
		if level == "inherit" {
			level = ""
		}
		provider, id, ok := stringsCutModel(spec)
		if !ok {
			continue
		}
		model := s.modelRuntime.GetModel(provider, id)
		if model == nil || !s.modelRuntime.HasConfiguredAuth(provider) {
			continue
		}
		candidates = append(candidates, candidate{role, model, ai.ModelThinkingLevel(level)})
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	index := slices.IndexFunc(candidates, func(c candidate) bool { return c.role == s.ActiveModelRole() })
	if index < 0 {
		current := s.Model()
		index = slices.IndexFunc(candidates, func(c candidate) bool { return ai.ModelsAreEqual(c.model, current) })
	}
	delta := 1
	if direction == "backward" {
		delta = -1
	}
	if index < 0 {
		if delta > 0 {
			index = -1
		} else {
			index = 0
		}
	}
	next := candidates[(index+delta+len(candidates))%len(candidates)]
	if len(candidates) == 1 && next.role == s.ActiveModelRole() {
		return nil, nil, nil
	}
	option := ModelMutationOptions{Role: next.role, ThinkingLevel: next.level}
	if len(options) > 0 {
		option.Persist = options[0].Persist
	}
	complete, err := s.beginModelChangeState(ctx, next.model, extension.ModelSelectSourceCycle, nil, deferThinking, option)
	if err != nil {
		return nil, nil, err
	}
	return &ModelCycleResult{Model: next.model, ThinkingLevel: s.ThinkingLevel()}, complete, nil
}

// stringsCutModel accepts provider-qualified identities, including IDs with slashes.
func stringsCutModel(spec string) (string, string, bool) {
	for i := range spec {
		if spec[i] == '/' {
			return spec[:i], spec[i+1:], i > 0 && i < len(spec)-1
		}
	}
	return "", "", false
}

// tryModelFallback runs only after same-model retries have settled. It preserves the
// role key, never wraps, skips missing credentials, and omits the failed response
// before continuing. Context overflow is left to compaction, not hidden by hopping.
// The caller holds s.mu; notifications are deferred to avoid reentering that lock.
func (s *Session) tryModelFallback(ctx context.Context, message *agent.AssistantMessage, sameModelRetry ...bool) (bool, error) {
	if !s.SettingsManager().GetRetryEnabled() || s.agentRunAborted(ctx) {
		return false, nil
	}
	if ai.IsContextOverflow(message.LLMMessage(), s.contextWindowFor(message)) {
		return false, nil
	}
	model := s.Model()
	if model == nil {
		return false, nil
	}
	settings := s.SettingsManager()
	if policy := settings.Get().Retry; policy != nil {
		if policy.ModelFallback != nil && !*policy.ModelFallback {
			return false, nil
		}
		usage := isModelUsageLimit(message.ErrorMessage)
		if policy.FallbackOn == "usage-limit" && !usage || policy.FallbackOn == "except-usage-limit" && usage {
			return false, nil
		}
	}
	current := s.fallbackSelector()
	if s.fallbackTried == nil {
		s.fallbackTried = map[string]bool{}
	}
	s.fallbackTried[current] = true
	for _, key := range s.fallbackKeys(current) {
		for _, candidate := range s.fallbackCandidates(key, current, false) {
			target := s.modelRuntime.GetModel(candidate.Provider, candidate.ID)
			if len(sameModelRetry) > 0 && sameModelRetry[0] && s.deferSignedThinkingFallback(message, target) {
				continue
			}
			identity := s.fallbackCandidateIdentity(candidate)
			if s.fallbackTried[identity] || s.fallbackSuppressed(identity) {
				continue
			}
			s.fallbackTried[identity] = true
			if target == nil || !s.fallbackFits(target) || s.checkModelAuth(ctx, target) != nil {
				continue
			}
			if err := s.omitRecoveryAttempt(message); err != nil {
				return false, err
			}
			hopped, err := s.applyFallback(ctx, key, candidate, message.ErrorMessage, false, false)
			if err != nil {
				return false, err
			}
			if !hopped {
				continue
			}
			if attempt := s.retryAttempt.Swap(0); attempt > 0 {
				s.emitEvent(agent.AutoRetryEndEvent{Success: false, Attempt: int(attempt), FinalError: "Switching to fallback " + candidate.Raw})
			}
			s.failedResponse.Store(message)
			return true, nil
		}
	}
	return false, nil
}

// isModelUsageLimit classifies provider quota/billing denials rather than short
// transport failures. Matching is intentionally case-insensitive across providers.
func isModelUsageLimit(message string) bool {
	message = strings.ToLower(message)
	for _, term := range []string{"usage limit", "usage_limit", "quota", "billing", "out of budget", "available balance", "credit balance", "out of credits", "usage_not_included", "spending limit"} {
		if strings.Contains(message, term) {
			return true
		}
	}
	return false
}
