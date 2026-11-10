package coding

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// sessionModelFallback remembers the primary before the first hop. It survives
// prompts, not explicit selection. Mu protects it independently of Session.mu so
// preflight and the TUI can inspect/reset it without reentrant notification locks.
type sessionModelFallback struct {
	Primary                string
	OriginalThinking       ai.ModelThinkingLevel
	LastThinking           ai.ModelThinkingLevel
	Role                   string
	Chain                  string
	Pinned                 bool
	MissingCredential      bool
	OriginalAuto, LastAuto bool
}

var fallbackRetryHint = regexp.MustCompile(`(?i)(?:retry[-_ ]after(?:[-_ ]ms)?["'\s:=]+|try again in\s+|retry in\s+)([0-9]+(?:\.[0-9]+)?)\s*(milliseconds?|ms|seconds?|s|minutes?|m|hours?|h)?`)

// fallbackTime is the injectable monotonic clock; test clocks are set before a run.
func (s *Session) fallbackTime() time.Time {
	if s.fallbackNow != nil {
		return s.fallbackNow()
	}
	return time.Now()
}

// fallbackCooldown honors the longest provider wait; unknown failures receive
// OMP's five-minute suppression so the next prompt does not thrash back immediately.
func fallbackCooldown(message string) time.Duration {
	longest := fallbackRetryDelay(message)
	if longest > 0 {
		return longest
	}
	return fallbackReasonDelay(message)
}

// fallbackRetryDelay parses explicit provider retry timing; zero means absent.
func fallbackRetryDelay(message string) time.Duration {
	longest := time.Duration(0)
	for _, match := range fallbackRetryHint.FindAllStringSubmatch(message, -1) {
		number, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		unit := strings.ToLower(match[2])
		scale := time.Second
		if unit == "ms" || strings.HasPrefix(unit, "millisecond") {
			scale = time.Millisecond
		} else if unit == "m" || strings.HasPrefix(unit, "minute") {
			scale = time.Minute
		} else if unit == "h" || strings.HasPrefix(unit, "hour") {
			scale = time.Hour
		} else if strings.Contains(strings.NewReplacer("_", "-", " ", "-").Replace(strings.ToLower(match[0])), "after-ms") {
			scale = time.Millisecond
		}
		delay := time.Duration(number * float64(scale))
		if delay > longest {
			longest = delay
		}
	}
	return longest
}

// fallbackReasonDelay is OMP's conservative reason-specific suppression policy.
func fallbackReasonDelay(message string) time.Duration {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "concurrent"):
		return 5 * time.Second
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit") || strings.Contains(lower, "429"):
		return 30 * time.Second
	case ai.IsAccountQuotaErrorMessage(message) || isModelUsageLimit(message):
		return 30 * time.Minute
	case strings.Contains(lower, "server error"):
		return 20 * time.Second
	}
	return 5 * time.Minute
}

// fallbackSuppressed reads one exact selector cooldown without leaking state.
func (s *Session) fallbackSuppressed(selector string) bool {
	s.retryFallbackMu.Lock()
	defer s.retryFallbackMu.Unlock()
	return s.fallbackCooldowns[selector].After(s.fallbackTime())
}

// fallbackSelector records the actual effort with each model so retry keys can
// distinguish two entries sharing a model with different reasoning settings.
func (s *Session) fallbackSelector() string {
	model := s.Model()
	if model == nil {
		return ""
	}
	value := ai.ModelSelector(model)
	if level := s.ThinkingLevel(); level != "" {
		value += ":" + string(level)
	}
	return value
}

// fallbackContext returns catalog-aware resolver inputs and the legacy default.
func (s *Session) fallbackContext() (modelroles.Config, map[string][]string, string, modelroles.Lookup) {
	settings := s.SettingsManager().Get()
	return settings.Config, s.SettingsManager().ModelFallbackChains(), settings.DefaultProvider + "/" + settings.DefaultModel, func(provider, id string) bool { return s.modelRuntime.GetModel(provider, id) != nil }
}

// fallbackKeys keeps the owning chain first, then allows a model at its tail to
// protect itself with another configured chain. Explicit-empty role ownership is
// retained, so a shared default model never captures a subagent's routing.
func (s *Session) fallbackKeys(current string) []string {
	config, chains, _, lookup := s.fallbackContext()
	key := modelroles.ResolveChainKey(config, chains, s.ActiveModelRole(), current, lookup)
	pinned := s.fallbackChain
	s.retryFallbackMu.Lock()
	if pinned == "" && s.retryFallback != nil {
		pinned = s.retryFallback.Chain
	}
	s.retryFallbackMu.Unlock()
	if pinned == "" {
		if key == "" {
			return nil
		}
		return []string{key}
	}
	if key != "" && key != pinned {
		return []string{pinned, key}
	}
	return []string{pinned}
}

// fallbackCandidates resolves the configured primary once, permits a missing role
// primary to use the current model, flattens role references in the tail so a
// role destination contributes its model and then its own recovery plan, and
// canonicalizes aliases before tail lookup. prewalk wraps a chain that stopped at
// a missing credential or exhausted quota; ChainEntries has already collapsed
// reference loops, so wrapping cannot repeat a role.
func (s *Session) fallbackCandidates(key, current string, wrap bool) []modelroles.Selector {
	config, chains, legacy, lookup := s.fallbackContext()
	primary := key
	if !strings.Contains(key, "/") {
		if config.ModelRoles[key] == "" {
			primary = current
		} else {
			value, err := config.Resolve("@"+key, legacy)
			if err == nil {
				primary = value
			} else {
				primary = current
			}
		}
	} else if strings.HasSuffix(key, "/*") {
		primary = current
	}
	return modelroles.EffectiveCandidates(config, primary, current, legacy, modelroles.ChainEntries(chains, key), lookup, wrap)
}

// fallbackCandidateIdentity fills inherited effort before cycle/cooldown checks;
// a bare primary and its explicit :off spelling must not count as two models.
func (s *Session) fallbackCandidateIdentity(candidate modelroles.Selector) string {
	level := s.ThinkingLevel()
	if candidate.Thinking != "" && candidate.Thinking != "auto" && candidate.Thinking != "inherit" {
		level = ai.ModelThinkingLevel(candidate.Thinking)
	}
	if model := s.modelRuntime.GetModel(candidate.Provider, candidate.ID); model != nil {
		level = ai.ClampThinkingLevel(model, level)
	}
	candidate.Thinking = string(level)
	return candidate.String()
}

// fallbackFits bars context-overflow-shaped fallbacks before they can cause an
// oversized paid request. Unknown windows fail open, as catalog discovery may lag.
func (s *Session) fallbackFits(model *ai.Model) bool {
	if model.Capabilities.ContextWindow <= 0 {
		return true
	}
	usage := s.ContextUsage()
	return usage == nil || usage.Tokens == nil || *usage.Tokens < model.Capabilities.ContextWindow
}

// applyFallback commits one provider swap, pinning the original primary once. The
// caller holds Session.mu; notifications run outside it. Inherit preserves the
// current effort, rather than reapplying the fallback model's global preference.
func (s *Session) applyFallback(ctx context.Context, key string, candidate modelroles.Selector, reason string, pinned, missing bool) (bool, error) {
	current := s.fallbackSelector()
	model := s.modelRuntime.GetModel(candidate.Provider, candidate.ID)
	if candidate.Upstream != "" {
		var err error
		model, err = ai.WithModelUpstreamRoute(model, candidate.Upstream)
		if err != nil {
			return false, nil
		}
	}
	if model == nil || !s.fallbackFits(model) {
		return false, nil
	}
	if err := s.checkModelAuth(ctx, model); err != nil {
		return false, nil
	}
	level := s.ThinkingLevel()
	if candidate.Thinking != "" && candidate.Thinking != "auto" && candidate.Thinking != "inherit" {
		level = ai.ModelThinkingLevel(candidate.Thinking)
	}
	level = ai.ClampThinkingLevel(model, level)
	if providerID(s.Model()) == candidate.Provider && s.Model().ID == candidate.ID && ai.ModelUpstreamRoute(s.Model()) == candidate.Upstream && s.ThinkingLevel() == level {
		return false, nil
	}
	role := s.ActiveModelRole()
	s.retryFallbackMu.Lock()
	if s.retryFallback == nil {
		s.retryFallback = &sessionModelFallback{Primary: current, OriginalThinking: s.ThinkingLevel(), Role: role, Chain: key, MissingCredential: missing, OriginalAuto: s.autoThinking.Load()}
	}
	if candidate.Thinking != "" && candidate.Thinking != "auto" && candidate.Thinking != "inherit" {
		s.autoThinking.Store(false)
	}
	s.retryFallback.LastAuto = s.autoThinking.Load()
	s.retryFallback.LastThinking = level
	s.retryFallback.Pinned = s.retryFallback.Pinned || pinned
	if !missing {
		if s.fallbackCooldowns == nil {
			s.fallbackCooldowns = map[string]time.Time{}
		}
		s.fallbackCooldowns[current] = s.fallbackTime().Add(fallbackCooldown(reason))
	}
	s.retryFallbackMu.Unlock()
	complete, err := s.beginModelChangeState(ctx, model, "fallback", &level, true)
	if err != nil {
		return false, err
	}
	s.fallbackChain = key
	if complete != nil {
		s.mu.Unlock()
		complete()
		s.mu.Lock()
	}
	return true, nil
}

// restoreFallbackPrimary runs once per prompt before credential validation. Manual
// effort changes made on a fallback survive restoration; unchanged efforts return
// to the original. A missing-credential hop restores as soon as credentials return.
func (s *Session) restoreFallbackPrimary(ctx context.Context) error {
	settings := s.SettingsManager().Get()
	if settings.Retry != nil && settings.Retry.FallbackRevertPolicy == "never" {
		return nil
	}
	s.retryFallbackMu.Lock()
	state := s.retryFallback
	if state == nil || state.Pinned {
		s.retryFallbackMu.Unlock()
		return nil
	}
	copy := *state
	s.retryFallbackMu.Unlock()
	if !copy.MissingCredential && s.fallbackSuppressed(copy.Primary) {
		return nil
	}
	_, _, _, lookup := s.fallbackContext()
	selector, ok := modelroles.ParseSelector(copy.Primary, lookup)
	if !ok {
		return nil
	}
	model := s.modelRuntime.GetModel(selector.Provider, selector.ID)
	if selector.Upstream != "" {
		model, _ = ai.WithModelUpstreamRoute(model, selector.Upstream)
	}
	if model == nil || s.checkModelAuth(ctx, model) != nil {
		return nil
	}
	level := s.ThinkingLevel()
	restoreAuto := s.autoThinking.Load()
	if level == copy.LastThinking && restoreAuto == copy.LastAuto {
		level = copy.OriginalThinking
		restoreAuto = copy.OriginalAuto
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryFallbackMu.Lock()
	if s.retryFallback != state {
		s.retryFallbackMu.Unlock()
		return nil
	}
	s.retryFallback = nil
	s.autoThinking.Store(restoreAuto)
	s.retryFallbackMu.Unlock()
	complete, err := s.beginModelChangeState(ctx, model, "fallback", &level, true)
	if err != nil {
		return err
	}
	if complete != nil {
		s.mu.Unlock()
		complete()
		s.mu.Lock()
	}
	return nil
}

// prepareFallbackModel is the per-prompt missing-auth and quota prewalk. Missing
// auth requires no fake failed assistant response and no backoff; errors retain
// ordinary prompt failure semantics when no eligible fallback exists.
func (s *Session) prepareFallbackModel(ctx context.Context) error {
	if err := s.restoreFallbackPrimary(ctx); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	model := s.Model()
	if model == nil {
		return nil
	}
	policy := s.SettingsManager().Get().Retry
	if policy != nil && policy.ModelFallback != nil && !*policy.ModelFallback {
		return nil
	}
	missing := s.checkModelAuth(ctx, model) != nil
	quotaReason := ""
	pinned := false
	requireConfirm := false
	reserveRemaining := 0.0
	if !missing && policy != nil && policy.UsageAwareFallback != nil && *policy.UsageAwareFallback {
		health, remaining := s.modelUsageHealth(ctx, model)
		if health == "reserve" || health == "depleted" {
			reservePolicy := policy.UsageReservePolicy
			if reservePolicy == "fail-closed" {
				return fmt.Errorf("usage preflight blocked: %s for %s", health, s.fallbackSelector())
			}
			requireConfirm = health == "reserve" && reservePolicy != "auto"
			reserveRemaining = remaining
			quotaReason = "usage " + health
			pinned = true
		}
	}
	if !missing && quotaReason == "" {
		return nil
	}
	current := s.fallbackSelector()
	for _, key := range s.fallbackKeys(current) {
		for _, candidate := range s.fallbackCandidates(key, current, false) {
			if s.fallbackSuppressed(s.fallbackCandidateIdentity(candidate)) {
				continue
			}
			target := s.modelRuntime.GetModel(candidate.Provider, candidate.ID)
			if target == nil || !s.fallbackFits(target) || s.checkModelAuth(ctx, target) != nil {
				continue
			}
			if policy != nil && policy.UsageAwareFallback != nil && *policy.UsageAwareFallback {
				health, _ := s.modelUsageHealth(ctx, target)
				if health == "reserve" || health == "depleted" {
					continue
				}
			}
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			if !ai.ModelsAreEqual(s.Model(), model) {
				return nil
			}
			if requireConfirm && !s.confirmUsageFallback(ctx, candidate.Raw, reserveRemaining) {
				return nil
			}
			s.mu.Lock()
			if !ai.ModelsAreEqual(s.Model(), model) {
				s.mu.Unlock()
				return nil
			}
			hopped, err := s.applyFallback(ctx, key, candidate, quotaReason, pinned, missing)
			s.mu.Unlock()
			if err != nil {
				return err
			}
			if hopped {
				return nil
			}
		}
	}
	return nil
}

// deferSignedThinkingFallback preserves OMP's signer retry preference only for a
// same-provider Claude sibling while another attempt on the signer is possible.
// Failures on a different model, non-retryable denials, and spent budgets must
// still try every sibling. Cross-provider normalization can demote signatures.
func (s *Session) deferSignedThinkingFallback(failed *agent.AssistantMessage, target *ai.Model) bool {
	if target == nil || s.rateLimitWalksChain(failed) {
		return false
	}
	source := s.Model()
	if source == nil || target.ProviderMeta.API != ai.APIAnthropicMessages {
		return false
	}
	for _, message := range slices.Backward(s.agent.Messages()) {
		assistant := message.Assistant
		if assistant == nil || assistant == failed {
			continue
		}
		if assistant.Provider != providerID(source) || assistant.ModelID != source.ID || assistant.Provider != providerID(target) || assistant.ModelID == target.ID {
			return false
		}
		for _, block := range assistant.Content {
			if thinking, ok := block.(ai.ThinkingContent); ok && (strings.TrimSpace(thinking.ThinkingSignature) != "" || thinking.Redacted) {
				return true
			}
		}
		return false
	}
	return false
}

// rateLimitWalksChain skips exhausted-model backoff when requested. Signed thinking
// otherwise stays on the current model until its retry budget is spent; cross-model
// transcript normalization strips foreign signatures before a fallback request.
func (s *Session) rateLimitWalksChain(message *agent.AssistantMessage) bool {
	policy := s.SettingsManager().Get().Retry
	text := strings.ToLower(message.ErrorMessage)
	return policy != nil && policy.RateLimitWalksChain != nil && *policy.RateLimitWalksChain && (strings.Contains(text, "429") || strings.Contains(text, "rate limit") || strings.Contains(text, "rate_limit"))
}
