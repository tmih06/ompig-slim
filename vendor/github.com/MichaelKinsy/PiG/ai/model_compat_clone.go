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

// cacheCompatExtra prepares the immutable extension payload retained by a
// bundled catalog row. Mutable user-authored compat values never use this.
func cacheCompatExtra(compat *ModelCompat) {
	if compat == nil || len(compat.Extra) == 0 {
		return
	}
	compat.normalizedExtra, _ = normalizeCompatExtra(compat.Extra)
}

// cloneScalarCompat preserves scalar fields and opaque extension JSON without
// serializing every modeled field. Complex typed fields retain the existing
// whole-object fallback. Mutable user-authored Extra values pass through
// marshaling so invalid JSON and whitespace/HTML normalization match the
// serialized copy; immutable bundled rows reuse their prepared form.
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
		!utf8.ValidString(string(in.SessionAffinityFormat)) ||
		!utf8.ValidString(string(in.ThinkingTokenBudgetField)) ||
		in.VLLMPriority != nil && (math.IsNaN(*in.VLLMPriority) || math.IsInf(*in.VLLMPriority, 0)) ||
		len(in.OpenRouterRouting) != 0 || len(in.ChatTemplateKwargs) != 0 || len(in.ChatTemplateArgs) != 0 ||
		in.VercelGatewayRouting != nil || len(in.AllowedFallbackModels) != 0 {
		return nil, false
	}
	out := *in
	out.Extra = nil
	out.normalizedExtra = nil
	out.OpenRouterRouting = nil
	out.ChatTemplateKwargs = nil
	out.ChatTemplateArgs = nil
	if in.AllowedFallbackModels != nil {
		out.AllowedFallbackModels = make([]AnthropicAllowedFallbackModel, 0)
	}
	out.SupportsDeveloperRole = cloneCompatBool(in.SupportsDeveloperRole)
	out.SupportsReasoningEffort = cloneCompatBool(in.SupportsReasoningEffort)
	out.SupportsStore = cloneCompatBool(in.SupportsStore)
	out.SupportsUsageInStreaming = cloneCompatBool(in.SupportsUsageInStreaming)
	out.SupportsFinishReason = cloneCompatBool(in.SupportsFinishReason)
	out.RequiresToolResultName = cloneCompatBool(in.RequiresToolResultName)
	out.RequiresAssistantAfterToolResult = cloneCompatBool(in.RequiresAssistantAfterToolResult)
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
		normalized := in.normalizedExtra
		if normalized == nil {
			var ok bool
			normalized, ok = normalizeCompatExtra(in.Extra)
			if !ok {
				return nil, false
			}
		}
		out.Extra = make(map[string]json.RawMessage, len(normalized))
		for key, value := range normalized {
			out.Extra[key] = bytes.Clone(value)
		}
	}
	return &out, true
}
