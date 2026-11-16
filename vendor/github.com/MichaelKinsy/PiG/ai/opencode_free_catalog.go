package ai

// Chat metadata from https://opencode.ai/inference/v1/models.
// Nemotron 3 Ultra is documented but absent from discovery; keep conservative limits.
func opencodeFreeChatModels() []GeneratedModel {
	models := []GeneratedModel{
		{ID: "big-pickle", DisplayName: "Big Pickle", ContextWindow: 200000, MaxOutputTokens: 32000,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
		},
		{ID: "mimo-v2.6-flash-free", DisplayName: "MiMo-V2.6-Flash Free", ContextWindow: 200000, MaxOutputTokens: 32000,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
		},
		{ID: "deepseek-v4.1-flash-free", DisplayName: "DeepSeek V4.1 Flash Free", ContextWindow: 200000, MaxOutputTokens: 32000,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"low": new("low"), "high": new("high"), "xhigh": new("max")},
		},
		{ID: "glm-5.3-flash-free", DisplayName: "GLM-5.3-Flash Free", ContextWindow: 200000, MaxOutputTokens: 32000,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"low": new("low"), "high": new("high"), "xhigh": new("max")},
		},
		{ID: "longcat-2.5-free", DisplayName: "LongCat 2.5 Free", ContextWindow: 1000000, MaxOutputTokens: 262144,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
		},
		{ID: "iceman-free", DisplayName: "Iceman Free", ContextWindow: 262144, MaxOutputTokens: 131072,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
		},
		{ID: "inkling-2-small-free", DisplayName: "Inkling 2 Small Free", ContextWindow: 1048576, MaxOutputTokens: 131072,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"off": new("none"), "minimal": new("minimal"), "low": new("low"), "medium": new("medium"), "high": new("high"), "xhigh": new("max")},
		},
		{ID: "beam-free", DisplayName: "Beam Free", ContextWindow: 1048576, MaxOutputTokens: 131072,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"low": new("low"), "medium": new("medium"), "high": new("high"), "xhigh": new("xhigh")},
		},
		{ID: "flint-40b-free", DisplayName: "Flint 40B Free", ContextWindow: 262144, MaxOutputTokens: 65536,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"low": new("low"), "medium": new("medium"), "high": new("high"), "xhigh": new("xhigh")},
		},
		{ID: "nemotron-3.5-super-vl-free", DisplayName: "Nemotron 3.5 Super VL Free", ContextWindow: 1048576, MaxOutputTokens: 131072,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"off": new("none"), "minimal": new("minimal"), "low": new("low"), "medium": new("medium"), "high": new("high"), "xhigh": new("xhigh")},
		},
		{ID: "nemotron-3.5-lightning-free", DisplayName: "Nemotron 3.5 Lightning Free", ContextWindow: 262144, MaxOutputTokens: 262144,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
		},
		{ID: "minimax-m3.1-flash-free", DisplayName: "MiniMax M3.1 Flash Free", ContextWindow: 1000000, MaxOutputTokens: 512000,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"low": new("low"), "medium": new("medium"), "high": new("high"), "xhigh": new("xhigh")},
		},
		{ID: "hy4-flash-free", DisplayName: "Hy4 flash Free", ContextWindow: 1048576, MaxOutputTokens: 65536,
			Reasoning: true, Capabilities: []string{"text", "image"}, Compat: &ModelCompat{SupportsReasoningEffort: new(true)},
			ThinkingLevelMap: map[ModelThinkingLevel]*string{"off": new("none"), "high": new("high")},
		},
		{ID: "nemotron-3-ultra-free", DisplayName: "Nemotron 3 Ultra Free", ContextWindow: 128000, MaxOutputTokens: 16384,
			Reasoning: true, Capabilities: []string{"text"}, Compat: &ModelCompat{SupportsReasoningEffort: new(false)},
			Description: "Documented by OpenCode; absent from current upstream discovery. Limits are conservative.",
		},
	}
	for i := range models {
		models[i].Provider = OpenCodeFreeProviderID
		models[i].API = APIOpenAICompletions
		models[i].BaseURL = opencodeFreeBaseURL
	}
	return models
}
