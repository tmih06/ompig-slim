package ai

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"unicode/utf8"
)

// ASCII case folding preserves byte length. Grouping the fixed modeled names
// avoids allocating a lowercase copy of every opaque key on every model read.
var compatNamesByLength = sync.OnceValue(func() map[int][]string {
	names := map[int][]string{}
	for name := range openAICompatMembers() {
		names[len(name)] = append(names[len(name)], name)
	}
	return names
})

// cloneCompatBool detaches a scalar pointer without reflection or interface
// conversion. The pointed-to value is immutable in the catalog after cloning.
func cloneCompatBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	return new(*value)
}

// cloneCompatFloat detaches the only floating-point scalar pointer in compat.
func cloneCompatFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

// normalizeCompatExtra validates and canonicalizes opaque members once for an
// immutable catalog row. The returned bytes are never exposed to callers.
func normalizeCompatExtra(extra map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(extra) == 0 {
		return nil, true
	}
	normalized := make(map[string]json.RawMessage, len(extra))
	for key, value := range extra {
		if len(value) == 0 {
			return nil, false
		}
		for i := 0; i < len(key); i++ {
			if key[i] >= utf8.RuneSelf {
				return nil, false
			}
		}
		for _, name := range compatNamesByLength()[len(key)] {
			if strings.EqualFold(key, name) {
				return nil, false
			}
		}
		trimmed := bytes.TrimSpace(value)
		switch string(trimmed) {
		case "true", "false", "null":
			normalized[key] = bytes.Clone(trimmed)
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, false
			}
			normalized[key] = encoded
		}
	}
	return normalized, true
}

// cloneSerializedJSONValue copies the canonical tree encoding/json produces
// for a JSON object member. Non-canonical programmatic values use the
// whole-object serialization fallback, which owns their normalization.
func cloneSerializedJSONValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil, bool:
		return value, true
	case string:
		return value, utf8.ValidString(typed)
	case float64:
		return value, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case map[string]any:
		if typed == nil {
			return nil, false
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if !utf8.ValidString(key) {
				return nil, false
			}
			copied, ok := cloneSerializedJSONValue(item)
			if !ok {
				return nil, false
			}
			out[key] = copied
		}
		return out, true
	case []any:
		if typed == nil {
			return nil, false
		}
		out := make([]any, len(typed))
		for index, item := range typed {
			copied, ok := cloneSerializedJSONValue(item)
			if !ok {
				return nil, false
			}
			out[index] = copied
		}
		return out, true
	default:
		return nil, false
	}
}

func cloneSerializedJSONObject(in map[string]any) (map[string]any, bool) {
	if in == nil {
		return nil, true
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if !utf8.ValidString(key) {
			return nil, false
		}
		copied, ok := cloneSerializedJSONValue(value)
		if !ok {
			return nil, false
		}
		out[key] = copied
	}
	return out, true
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneCostTiers(in []CostTier) []CostTier {
	if in == nil {
		return nil
	}
	out := make([]CostTier, len(in))
	copy(out, in)
	return out
}

// cloneScalarCompat preserves scalar fields, canonical JSON-valued routing
// fields, and opaque extension JSON without serializing every modeled field.
// Non-canonical programmatic values retain the whole-object fallback. Mutable
// user-authored Extra values pass through
// marshaling so invalid JSON and whitespace/HTML normalization match the
// serialized copy.
// Empty raw values and colliding/non-ASCII keys use the fallback because
// whole-object validation, JSON field matching and Unicode repair can change
// modeled fields or key spelling.
func cloneScalarCompat(in *ModelCompat) (*ModelCompat, bool) {
	// Keep this hot path explicit instead of reflecting over the compat struct.
	// Catalog composition clones hundreds of these values per request; the
	// fallback remains responsible for non-scalar nested values and preserves
	// the serializer's exact normalization semantics.
	if !utf8.ValidString(in.MaxTokensField) ||
		!utf8.ValidString(in.ThinkingFormat) ||
		!utf8.ValidString(in.CacheControlFormat) ||
		!utf8.ValidString(in.DeferredToolsMode) ||
		!utf8.ValidString(in.PromptCacheKeyMode) ||
		!utf8.ValidString(string(in.SessionAffinityFormat)) ||
		!utf8.ValidString(string(in.ThinkingTokenBudgetField)) ||
		in.VLLMPriority != nil && (math.IsNaN(*in.VLLMPriority) || math.IsInf(*in.VLLMPriority, 0)) {
		return nil, false
	}
	out := *in
	out.Extra = nil
	if in.OpenRouterRouting != nil {
		cloned, ok := cloneSerializedJSONObject(in.OpenRouterRouting)
		if !ok {
			return nil, false
		}
		out.OpenRouterRouting = cloned
	}
	if in.ChatTemplateKwargs != nil {
		cloned, ok := cloneSerializedJSONObject(in.ChatTemplateKwargs)
		if !ok {
			return nil, false
		}
		out.ChatTemplateKwargs = cloned
	}
	if in.ChatTemplateArgs != nil {
		cloned, ok := cloneSerializedJSONObject(in.ChatTemplateArgs)
		if !ok {
			return nil, false
		}
		out.ChatTemplateArgs = cloned
	}
	if in.VercelGatewayRouting != nil {
		routing := *in.VercelGatewayRouting
		routing.Only = cloneStringSlice(routing.Only)
		routing.Order = cloneStringSlice(routing.Order)
		out.VercelGatewayRouting = &routing
	}
	if in.AllowedFallbackModels != nil {
		out.AllowedFallbackModels = make([]AnthropicAllowedFallbackModel, len(in.AllowedFallbackModels))
		for index, fallback := range in.AllowedFallbackModels {
			fallback.Cost.Tiers = cloneCostTiers(fallback.Cost.Tiers)
			out.AllowedFallbackModels[index] = fallback
		}
	}
	out.SupportsDeveloperRole = cloneCompatBool(in.SupportsDeveloperRole)
	out.SupportsReasoningEffort = cloneCompatBool(in.SupportsReasoningEffort)
	out.SupportsStore = cloneCompatBool(in.SupportsStore)
	out.SupportsUsageInStreaming = cloneCompatBool(in.SupportsUsageInStreaming)
	out.SupportsFinishReason = cloneCompatBool(in.SupportsFinishReason)
	out.RequiresToolResultName = cloneCompatBool(in.RequiresToolResultName)
	out.RequiresAssistantAfterToolResult = cloneCompatBool(in.RequiresAssistantAfterToolResult)
	out.RequiresNonNullAssistantContent = cloneCompatBool(in.RequiresNonNullAssistantContent)
	out.RequiresToolsWithToolHistory = cloneCompatBool(in.RequiresToolsWithToolHistory)
	out.RequiresThinkingAsText = cloneCompatBool(in.RequiresThinkingAsText)
	out.RequiresReasoningContentOnAssistantMessages = cloneCompatBool(in.RequiresReasoningContentOnAssistantMessages)
	out.SupportsOpenAIGrammarTools = cloneCompatBool(in.SupportsOpenAIGrammarTools)
	out.SupportsExplicitPromptCacheMode = cloneCompatBool(in.SupportsExplicitPromptCacheMode)
	out.SupportsStrictTools = cloneCompatBool(in.SupportsStrictTools)
	out.SendSessionAffinityHeaders = cloneCompatBool(in.SendSessionAffinityHeaders)
	out.SupportsStrictMode = cloneCompatBool(in.SupportsStrictMode)
	out.VLLMPriority = cloneCompatFloat(in.VLLMPriority)
	out.SupportsLongCacheRetention = cloneCompatBool(in.SupportsLongCacheRetention)
	out.SendSessionIdHeader = cloneCompatBool(in.SendSessionIdHeader)
	out.SupportsEagerToolInputStreaming = cloneCompatBool(in.SupportsEagerToolInputStreaming)
	out.SupportsCacheControlOnTools = cloneCompatBool(in.SupportsCacheControlOnTools)
	out.ForceAdaptiveThinking = cloneCompatBool(in.ForceAdaptiveThinking)
	out.AllowEmptySignature = cloneCompatBool(in.AllowEmptySignature)
	out.SupportsTemperature = cloneCompatBool(in.SupportsTemperature)
	out.ZaiToolStream = cloneCompatBool(in.ZaiToolStream)
	out.SupportsToolSearch = cloneCompatBool(in.SupportsToolSearch)
	out.SupportsMaxOutputTokens = cloneCompatBool(in.SupportsMaxOutputTokens)
	out.SupportsThinkingTokenBudget = cloneCompatBool(in.SupportsThinkingTokenBudget)
	out.SupportsAdditionalTools = cloneCompatBool(in.SupportsAdditionalTools)
	out.SupportsMidConvoEffort = cloneCompatBool(in.SupportsMidConvoEffort)
	out.SupportsMidConvoSystemMessages = cloneCompatBool(in.SupportsMidConvoSystemMessages)
	out.SupportsMidConvoToolAdditions = cloneCompatBool(in.SupportsMidConvoToolAdditions)
	out.SupportsMidConvoToolChanges = cloneCompatBool(in.SupportsMidConvoToolChanges)
	if len(in.Extra) > 0 {
		normalized, ok := normalizeCompatExtra(in.Extra)
		if !ok {
			return nil, false
		}
		out.Extra = make(map[string]json.RawMessage, len(normalized))
		for key, value := range normalized {
			out.Extra[key] = bytes.Clone(value)
		}
	}
	return &out, true
}
