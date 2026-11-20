package ai

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCodexConfigurationUpdateEffortUsesModelMapping(t *testing.T) {
	provider := NewOpenAICodexResponsesProvider(OpenAICodexResponsesConfig{
		APIKey: "fixture-token",
		Model:  "fixture-model",
		ModelMetadata: &Model{
			ID: "fixture-model",
			ProviderMeta: ProviderMetadata{
				ProviderID: "openai-codex",
				API:        APIOpenAICodexResponses,
				Reasoning:  true,
			},
			ThinkingLevelMap: ThinkingLevelMap{
				ThinkingOff: nil,
				ThinkingLow: new("low-provider"),
			},
		},
		Compat: &OpenAIResponsesCompat{SupportsMidConvoEffort: new(true)},
	}).(*openAIResponsesProvider)

	compat := getResponsesCompat(provider.cfg.Compat, true)
	if got := provider.configurationUpdateEffort(compat, StreamOptions{ReasoningEffort: "low"}, "fixture-token"); got != "low-provider" {
		t.Fatalf("mapped Codex effort = %q, want low-provider", got)
	}
	if got := provider.configurationUpdateEffort(compat, StreamOptions{ReasoningEffort: "none"}, "fixture-token"); got != "" {
		t.Fatalf("unavailable none effort = %q, want empty", got)
	}
	if got := provider.configurationUpdateEffort(compat, StreamOptions{Thinking: ThinkingLevelLow}, "fixture-token"); got != "low-provider" {
		t.Fatalf("thinking-level Codex effort = %q, want low-provider", got)
	}
}

func TestMistralStrictToolsFollowModelCompatibility(t *testing.T) {
	tool := ToolSchema{
		Name: "lookup",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
		ConstrainedSampling: &ConstrainedSamplingConfig{Type: ConstrainedSamplingJSONSchema},
	}
	for _, test := range []struct {
		name       string
		compat     *ModelCompat
		wantStrict bool
	}{
		{name: "default disabled"},
		{name: "opted in", compat: &ModelCompat{SupportsStrictMode: new(true)}, wantStrict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &mistralProvider{cfg: MistralConfig{ModelMetadata: &Model{
				ProviderMeta: ProviderMetadata{Compat: test.compat},
			}}}
			tools, err := provider.convertTools([]ToolSchema{tool})
			if err != nil {
				t.Fatal(err)
			}
			if got := tools[0].Function.Strict; got != test.wantStrict {
				t.Fatalf("strict = %t, want %t", got, test.wantStrict)
			}
		})
	}
}

func TestQwenTokenPlanIndividualRegionalCatalogParity(t *testing.T) {
	for _, provider := range []struct {
		id, baseURL string
	}{
		{"qwen-token-plan-individual", qwenTokenPlanUSBaseURL},
		{qwenTokenPlanIndividualCNProviderID, qwenTokenPlanCNBaseURL},
	} {
		models := ListModels(provider.id)
		byID := make(map[string]GeneratedModel, len(models))
		for _, model := range models {
			byID[model.ID] = model
			if model.BaseURL != provider.baseURL {
				t.Errorf("%s/%s base URL = %q, want %q", provider.id, model.ID, model.BaseURL, provider.baseURL)
			}
		}
		for _, id := range []string{
			"deepseek-v4-flash-0731", "deepseek-v4-pro", "deepseek-v4-pro-0813",
			"deepseek-v4.1-flash", "deepseek-v4.1-flash-1017", "glm-5.2", "glm-5.3",
			"qwen3.6-flash", "qwen3.7-max", "qwen3.7-plus", "qwen3.8-flash", "qwen3.8-max",
			"qwen4-max-preview",
		} {
			model, ok := byID[id]
			if !ok {
				t.Errorf("%s catalog missing %s", provider.id, id)
				continue
			}
			if !strings.Contains(model.Record, `"provider":"`+provider.id+`"`) || !strings.Contains(model.Record, `"baseUrl":"`+provider.baseURL+`"`) {
				t.Errorf("%s/%s serialized record does not match catalog metadata: %s", provider.id, id, model.Record)
			}
			if id == "qwen4-max-preview" {
				if model.ContextWindow != 1_000_000 || model.MaxOutputTokens != 393_216 || !model.Reasoning || len(model.Capabilities) != 2 {
					t.Errorf("preview metadata = %+v", model)
				}
				if model.ThinkingLevelMap[ThinkingLow] == nil || *model.ThinkingLevelMap[ThinkingLow] != "low" ||
					model.ThinkingLevelMap[ThinkingMedium] == nil || *model.ThinkingLevelMap[ThinkingMedium] != "medium" ||
					model.ThinkingLevelMap[ThinkingXHigh] == nil || *model.ThinkingLevelMap[ThinkingXHigh] != "xhigh" {
					t.Errorf("preview thinking map = %#v", model.ThinkingLevelMap)
				}
			}
		}
	}
	if got := ProviderBaseURL(qwenTokenPlanIndividualCNProviderID); got != qwenTokenPlanCNBaseURL {
		t.Errorf("CN individual provider base URL = %q", got)
	}
	if got := ProviderDisplayName(qwenTokenPlanIndividualCNProviderID); got != "Qwen Token Plan Individual CN" {
		t.Errorf("CN individual display name = %q", got)
	}
	if !slices.Contains(GeneratedProviders, qwenTokenPlanIndividualCNProviderID) {
		t.Fatal("CN individual provider is absent from provider list")
	}
}

func TestQwenTokenPlanKeyVariablesUsePlanSpecificPrecedence(t *testing.T) {
	for _, test := range []struct {
		provider string
		want     []string
	}{
		{"qwen-token-plan", []string{"QWEN_TOKEN_PLAN_TEAM_API_KEY", "QWEN_TOKEN_PLAN_API_KEY"}},
		{"qwen-token-plan-cn", []string{"QWEN_TOKEN_PLAN_TEAM_CN_API_KEY", "QWEN_TOKEN_PLAN_CN_API_KEY"}},
		{"qwen-token-plan-individual", []string{"QWEN_TOKEN_PLAN_INDIVIDUAL_API_KEY", "QWEN_TOKEN_PLAN_API_KEY"}},
		{qwenTokenPlanIndividualCNProviderID, []string{"QWEN_TOKEN_PLAN_INDIVIDUAL_CN_API_KEY", "QWEN_TOKEN_PLAN_CN_API_KEY"}},
	} {
		got := getAPIKeyEnvVars(test.provider)
		if !slices.Equal(got, test.want) {
			t.Errorf("%s environment keys = %v, want %v", test.provider, got, test.want)
		}
	}
}

func TestResponsesCompatDecodesPromptCacheKeyMode(t *testing.T) {
	var compat OpenAIResponsesCompat
	if err := json.Unmarshal([]byte(`{"promptCacheKeyMode":"disabled"}`), &compat); err != nil {
		t.Fatal(err)
	}
	if got := getResponsesCompat(&compat, false).PromptCacheKeyMode; got != "disabled" {
		t.Fatalf("prompt cache key mode = %q", got)
	}
}

func TestTransientNetworkRetryPatternsIncludeNodeErrors(t *testing.T) {
	for _, message := range []string{
		"connection closed", "read ECONNRESET", "write EPIPE", "connect ETIMEDOUT 10.0.0.1:443",
		"Connection reset by peer", "Responses websocket connection limit reached",
		"Claude Code's stream from Anthropic was cut short after the reply had begun",
	} {
		if !retryableProviderErrorPattern.MatchString(message) {
			t.Errorf("retry pattern did not match %q", message)
		}
	}
	// The bare-code pattern must not match an identifier that merely contains the code.
	if retryableProviderErrorPattern.MatchString("SOMEECONNRESETX") {
		t.Error("retry pattern matched an embedded error code")
	}
}

func TestStrictSchemaRejectsUnsupportedUntypedRoot(t *testing.T) {
	tool := ToolSchema{
		Name: "lookup",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"nested": map[string]any{"description": "no type, anyOf or enum"}},
		},
		ConstrainedSampling: &ConstrainedSamplingConfig{Type: ConstrainedSamplingJSONSchema, Strict: ConstrainedSamplingStrictRequire},
	}
	if _, err := resolveJSONSchemaStrictSampling(tool, true, nil); err == nil || !strings.Contains(err.Error(), "schemas without a type are unsupported") {
		t.Fatalf("untyped root error = %v", err)
	}
}
