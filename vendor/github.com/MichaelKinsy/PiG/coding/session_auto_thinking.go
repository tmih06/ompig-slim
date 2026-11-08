package coding

import (
	"context"
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

// autoThinkingQuestion is OMP's rubric, kept verbatim so difficulty meanings do
// not diverge between classifier and chat-backed judge candidates.
//
//go:embed auto_thinking_question.json
var autoThinkingQuestion []byte

// autoQuestion is one named question from the upstream rubric.
type autoQuestion struct {
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// ConfiguredThinkingLevel exposes auto separately from the concrete request
// effort. Provider requests always use ThinkingLevel(), never the auto sentinel.
func (s *Session) ConfiguredThinkingLevel() ai.ModelThinkingLevel {
	if s.autoThinking.Load() {
		return "auto"
	}
	return s.ThinkingLevel()
}

// provisionalAutoLevel starts at high, bounded below max. Sparse ladders with
// only max retain the model's own floor rather than inventing an unsupported tier.
func provisionalAutoLevel(model *ai.Model) ai.ModelThinkingLevel {
	levels := autoThinkingLevels(model, false)
	if len(levels) == 0 {
		return ai.ClampThinkingLevel(model, ai.ThinkingHigh)
	}
	for _, level := range levels {
		if ai.CompareThinkingLevels(level, ai.ThinkingHigh) >= 0 {
			return level
		}
	}
	return levels[len(levels)-1]
}

// autoThinkingLevels admits supported low-or-higher efforts, with OMP's xhigh
// default ceiling unless providers.autoThinkingMaxEffort opts into max.
func autoThinkingLevels(model *ai.Model, allowMax bool) []ai.ModelThinkingLevel {
	var levels []ai.ModelThinkingLevel
	for _, level := range ai.GetSupportedThinkingLevels(model) {
		if ai.CompareThinkingLevels(level, ai.ThinkingLow) < 0 || !allowMax && level == ai.ThinkingMax {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}

// applyAutoThinking classifies once per prompt with a four-second total budget.
// Zero/one effort tiers need no judge call; failures retain the last concrete
// effort. A superseded model/selector never accepts an old classification result.
func (s *Session) applyAutoThinking(ctx context.Context, prompt string) {
	if !s.autoThinking.Load() {
		return
	}
	model := s.Model()
	if model == nil || !s.SupportsThinking() {
		return
	}
	levels := autoThinkingLevels(model, s.SettingsManager().GetAutoThinkingMaxEffort() == "max")
	if len(levels) == 0 {
		return
	}
	chosen := ai.ModelThinkingLevel("")
	if strings.Contains(strings.ToLower(prompt), "ultrathink") {
		all := ai.GetSupportedThinkingLevels(model)
		chosen = all[len(all)-1]
	} else if len(levels) == 1 {
		chosen = levels[0]
	} else {
		classifyCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		var questions map[string]autoQuestion
		if json.Unmarshal(autoThinkingQuestion, &questions) != nil {
			return
		}
		question := questions["level"]
		replacement := ""
		if slices.Contains(levels, ai.ThinkingMax) {
			replacement = ", except that a request meeting the max conditions takes max"
		}
		question.Instructions = strings.ReplaceAll(question.Instructions, "{{#if withMax}}, except that a request meeting the max conditions takes max{{/if}}", replacement)
		for _, candidate := range s.autoThinkingJudges() {
			if classifyCtx.Err() != nil {
				break
			}
			level := s.askAutoThinkingJudge(classifyCtx, candidate, prompt, question, questions["bucket"], levels)
			if slices.Contains(levels, level) {
				chosen = level
				break
			}
		}
	}
	if chosen == "" || ctx.Err() != nil || !s.autoThinking.Load() || !ai.ModelsAreEqual(model, s.Model()) {
		return
	}
	// This is an effective per-prompt resolution, never a persisted preference.
	s.agent.SetThinkingLevel(ai.ClampThinkingLevel(model, chosen))
	s.notifyReplicatedState()
}

// autoThinkingJudges follows the live judge role, its explicit fallback tail, then
// OMP's built-in priority list. Missing auth and unavailable candidates are skipped.
func (s *Session) autoThinkingJudges() []ai.AnyModel {
	config := s.SettingsManager().Get().Config
	all := s.services.Registry().GetAllProviderModelData()
	pool := []modelroles.Candidate{}
	for _, model := range all {
		if s.modelRuntime.HasConfiguredAuth(model.ProviderID()) {
			pool = append(pool, modelroles.Candidate{Provider: model.ProviderID(), ID: model.ModelID(), Kind: string(model.ModelType())})
		}
	}
	patterns := []string{"@judge"}
	if chain, ok := s.SettingsManager().ModelFallbackChains()["judge"]; ok {
		patterns = append(patterns, chain...)
	} else {
		patterns = append(patterns, modelroles.RoleDefaults("judge")...)
	}
	seen := map[string]bool{}
	var out []ai.AnyModel
	for _, pattern := range patterns {
		selector, err := config.ResolveAvailable(pattern, "", pool)
		if err != nil {
			continue
		}
		base, _ := modelroles.Split(selector)
		if seen[base] {
			continue
		}
		seen[base] = true
		for _, model := range all {
			if model.ProviderID()+"/"+model.ModelID() == base {
				out = append(out, model)
				break
			}
		}
	}
	return out
}

// askAutoThinkingJudge uses native choice classifiers where available and a
// constrained chat answer otherwise. Usage is audited outside conversation context.
func (s *Session) askAutoThinkingJudge(ctx context.Context, candidate ai.AnyModel, prompt string, question, bucket autoQuestion, levels []ai.ModelThinkingLevel) ai.ModelThinkingLevel {
	attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	choice := ""
	switch model := candidate.(type) {
	case *ai.ClassifierModel:
		local := model.Provider == "local" || model.Provider == "llama-cpp" || model.Provider == "lm-studio"
		q := question
		labels := []string{}
		for _, level := range levels {
			labels = append(labels, string(level))
		}
		if local {
			q = bucket
			labels = []string{"trivial", "moderate", "hard"}
		}
		criteria := []ai.ClassifierChoiceCriterion{}
		for _, label := range labels {
			criteria = append(criteria, ai.ClassifierChoiceCriterion{Key: label, Description: q.Criteria[label]})
		}
		result := s.modelRuntime.Classify(attempt, model, ai.ClassifierContext{State: ai.JsonObject{"request": prompt}, Questions: ai.ClassifierQuestions{{ID: "level", Question: ai.ClassifierChoiceQuestion{Instructions: q.Instructions, Criteria: criteria}}}})
		if result.Usage != nil {
			s.auditAutoThinkingUsage(model.Provider, model.ID, *result.Usage)
		}
		if result.StopReason != ai.ClassifierStopReasonStop {
			return ""
		}
		for _, answer := range result.Answers {
			if value, ok := answer.Answer.(ai.ClassifierChoiceAnswer); ok {
				choice = value.Choice
				break
			}
		}
		if local {
			switch choice {
			case "trivial":
				choice = "low"
			case "moderate":
				choice = "high"
			case "hard":
				choice = "xhigh"
			}
		}
	case *ai.Model:
		if bound := s.modelRuntime.GetModel(model.ProviderID(), model.ID); bound != nil {
			model = bound
		}
		var instructions strings.Builder
		instructions.WriteString(question.Instructions + "\nReturn exactly one supported level, with no other text.\n")
		for _, level := range levels {
			instructions.WriteString(string(level) + ": " + question.Criteria[string(level)] + "\n")
		}
		stream := s.modelRuntime.StreamSimple(attempt, model, ai.Context{SystemPrompt: instructions.String(), Messages: []ai.Message{ai.UserMessage{Content: ai.UserText(prompt), Timestamp: time.Now().UnixMilli()}}}, ai.StreamOptions{MaxTokens: 256, Thinking: ai.ThinkingOff.ReasoningOption()})
		result := stream.Result()
		if result == nil || result.StopReason == ai.StopReasonError || result.StopReason == ai.StopReasonAborted {
			return ""
		}
		s.auditAutoThinkingUsage(model.ProviderID(), model.ID, result.Usage)
		for _, block := range result.Content {
			if text, ok := block.(ai.TextContent); ok {
				choice += text.Text
			}
		}
	}
	choice = strings.ToLower(strings.Trim(strings.TrimSpace(choice), "\"'`"))
	selected := ai.ModelThinkingLevel(choice)
	if slices.Contains(levels, selected) {
		return selected
	}
	// Local three-bucket choices clamp upward only within the target's ceiling.
	if choice == "low" || choice == "high" || choice == "xhigh" {
		for _, level := range levels {
			if ai.CompareThinkingLevels(level, selected) >= 0 {
				return level
			}
		}
		return levels[len(levels)-1]
	}
	return ""
}

// auditAutoThinkingUsage keeps judge tokens/cost in the session ledger without
// adding its rubric or answer to the conversation the primary model sees.
func (s *Session) auditAutoThinkingUsage(provider, model string, usage ai.Usage) {
	if s.inner == nil {
		return
	}
	entry, err := s.inner.AppendUsage("auto-thinking", provider, model, usage, "Per-prompt effort classification")
	if err == nil {
		s.emitEvent(agent.EntryAppendedEvent{Entry: entry.Raw()})
	}
}
