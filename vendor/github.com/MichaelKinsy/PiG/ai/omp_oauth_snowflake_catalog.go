package ai

// Snowflake Cortex model roster and provider registration.
//
// Ports pi-catalog's providers/snowflake.kdl seed block: the account-billed Cortex
// REST API's documented models, with the pricing and limits that file states. The
// catalog's base URL is the `.invalid` placeholder origin snowflakeTransport
// replaces with the credential's account host; it parses (models.json invariant)
// and never resolves (RFC 6761), so an unrewritten request fails before reaching
// any host.

import (
	"slices"
)

// snowflakeAnthropicCompat is the Cortex Anthropic route: ephemeral five-minute
// caching and Bedrock-compatible betas only, and no eager tool-input streaming or
// long cache retention.
var snowflakeAnthropicCompat = ModelCompat{
	SupportsEagerToolInputStreaming: new(false),
	SupportsLongCacheRetention:      new(false),
}

// snowflakeOpenAICompat is the Cortex OpenAI route: max_tokens/store are hard
// errors and reasoning_effort is documented.
var snowflakeOpenAICompat = ModelCompat{
	MaxTokensField:          "max_completion_tokens",
	SupportsStore:           new(false),
	SupportsDeveloperRole:   new(false),
	SupportsReasoningEffort: new(true),
}

// snowflakeThinkingAnthropic is the Claude effort ladder the newest Cortex rows
// accept (Opus 4.7+, Sonnet 5).
var snowflakeThinkingAnthropic = map[ModelThinkingLevel]*string{
	ThinkingLow:   new("low"),
	ThinkingHigh:  new("high"),
	ThinkingXHigh: new("xhigh"),
	ThinkingMax:   new("max"),
}

// snowflakeThinkingAnthropicHigh is the ladder of the older Claude rows, which
// reject xhigh and max.
var snowflakeThinkingAnthropicHigh = map[ModelThinkingLevel]*string{
	ThinkingLow:  new("low"),
	ThinkingHigh: new("high"),
}

// snowflakeThinkingOpenAI is Cortex's documented reasoning_effort ladder for the
// prefixed gpt rows below 5.2.
var snowflakeThinkingOpenAI = map[ModelThinkingLevel]*string{
	ThinkingMinimal: new("minimal"),
	ThinkingLow:     new("low"),
	ThinkingMedium:  new("medium"),
	ThinkingHigh:    new("high"),
}

// snowflakeModels is the provider's static roster.
var snowflakeModels = []GeneratedModel{
	{
		ID:                   "claude-opus-5-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 5.5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  4,
		OutputCostPerMTokens: 20,
		CacheReadCost:        0.2,
		CacheWriteCost:       5,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-opus-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  5,
		OutputCostPerMTokens: 25,
		CacheReadCost:        0.5,
		CacheWriteCost:       6.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-opus-4-8",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 4.8",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  5,
		OutputCostPerMTokens: 25,
		CacheReadCost:        0.5,
		CacheWriteCost:       6.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-opus-4-7",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 4.7",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  5,
		OutputCostPerMTokens: 25,
		CacheReadCost:        0.5,
		CacheWriteCost:       6.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-opus-4-6",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 4.6",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  5,
		OutputCostPerMTokens: 25,
		CacheReadCost:        0.5,
		CacheWriteCost:       6.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-opus-4-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Opus 4.5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropicHigh,
		ContextWindow:        200000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  5,
		OutputCostPerMTokens: 25,
		CacheReadCost:        0.5,
		CacheWriteCost:       6.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-sonnet-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Sonnet 5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  2,
		OutputCostPerMTokens: 10,
		CacheReadCost:        0.2,
		CacheWriteCost:       2.5,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-sonnet-4-6",
		Provider:             "snowflake",
		DisplayName:          "Claude Sonnet 4.6",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropic,
		ContextWindow:        1000000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  3,
		OutputCostPerMTokens: 15,
		CacheReadCost:        0.3,
		CacheWriteCost:       3.75,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-sonnet-4-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Sonnet 4.5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropicHigh,
		ContextWindow:        200000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  3,
		OutputCostPerMTokens: 15,
		CacheReadCost:        0.3,
		CacheWriteCost:       3.75,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "claude-haiku-4-5",
		Provider:             "snowflake",
		DisplayName:          "Claude Haiku 4.5",
		API:                  "anthropic-messages",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeAnthropicCompat,
		ThinkingLevelMap:     snowflakeThinkingAnthropicHigh,
		ContextWindow:        200000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  1,
		OutputCostPerMTokens: 5,
		CacheReadCost:        0.1,
		CacheWriteCost:       1.25,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "openai-gpt-5.1",
		Provider:             "snowflake",
		DisplayName:          "GPT-5.1",
		API:                  "openai-completions",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeOpenAICompat,
		ThinkingLevelMap:     snowflakeThinkingOpenAI,
		ContextWindow:        272000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  1.25,
		OutputCostPerMTokens: 10,
		CacheReadCost:        0.13,
		CacheWriteCost:       0,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "openai-gpt-5",
		Provider:             "snowflake",
		DisplayName:          "GPT-5",
		API:                  "openai-completions",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeOpenAICompat,
		ThinkingLevelMap:     snowflakeThinkingOpenAI,
		ContextWindow:        272000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  1.25,
		OutputCostPerMTokens: 10,
		CacheReadCost:        0.13,
		CacheWriteCost:       0,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "openai-gpt-5-mini",
		Provider:             "snowflake",
		DisplayName:          "GPT-5 Mini",
		API:                  "openai-completions",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeOpenAICompat,
		ThinkingLevelMap:     snowflakeThinkingOpenAI,
		ContextWindow:        272000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  0.25,
		OutputCostPerMTokens: 2,
		CacheReadCost:        0.03,
		CacheWriteCost:       0,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "openai-gpt-5-nano",
		Provider:             "snowflake",
		DisplayName:          "GPT-5 Nano",
		API:                  "openai-completions",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeOpenAICompat,
		ThinkingLevelMap:     snowflakeThinkingOpenAI,
		ContextWindow:        272000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  0.06,
		OutputCostPerMTokens: 0.44,
		CacheReadCost:        0.01,
		CacheWriteCost:       0,
		Reasoning:            true,
		Capabilities:         []string{"text", "image"},
	},
	{
		ID:                   "openai-gpt-4.1",
		Provider:             "snowflake",
		DisplayName:          "GPT-4.1",
		API:                  "openai-completions",
		BaseURL:              snowflakeAccountPlaceholder,
		Compat:               &snowflakeOpenAICompat,
		ThinkingLevelMap:     snowflakeThinkingOpenAI,
		ContextWindow:        128000,
		MaxOutputTokens:      16384,
		InputCostPerMTokens:  2,
		OutputCostPerMTokens: 8,
		CacheReadCost:        0.5,
		CacheWriteCost:       0,
		Reasoning:            false,
		Capabilities:         []string{"text", "image"},
	},
}

// snowflakeProvider assembles the Snowflake provider: the catalog above routed
// through the API each row names, over the request transport that swaps the
// placeholder origin for the credential's account host.
func snowflakeProvider() *ModelsProvider {
	models := make([]AnyModel, 0, len(snowflakeModels))
	for index := range snowflakeModels {
		models = append(models, snowflakeModels[index].ToModel())
	}
	auth := ProviderAuth{APIKey: snowflakeAPIKeyAuth()}
	auth.OAuth, _ = OAuthProviderAuth("snowflake")
	return CreateProvider(CreateProviderOptions{
		ID:      "snowflake",
		Name:    new(snowflakeOAuthName),
		BaseURL: snowflakeAccountPlaceholder,
		Auth:    auth,
		Models:  models,
		API: ProviderAPIMap{
			APIAnthropicMessages: snowflakeStreams(AnthropicMessagesAPI()),
			APIOpenAICompletions: snowflakeStreams(OpenAICompletionsAPI()),
		},
	})
}

// registerSnowflakeCatalog appends the roster to the generated catalog so model
// listing, lookup and the model registry see it.
func init() {
	GeneratedModels = append(GeneratedModels, snowflakeModels...)
	if !slices.Contains(GeneratedProviders, "snowflake") {
		GeneratedProviders = append(GeneratedProviders, "snowflake")
		slices.Sort(GeneratedProviders)
	}
}
