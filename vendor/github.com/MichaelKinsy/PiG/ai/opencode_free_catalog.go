package ai

// opencodeFreeCatalog is the free chat tier as https://opencode.ai/inference/v1/models publishes
// it. Nemotron 3 Ultra is documented by OpenCode but absent from discovery; keep conservative limits.
var opencodeFreeCatalog = []opencodeFreeModel{
	{ID: "big-pickle", DisplayName: "Big Pickle", ContextWindow: 200000, MaxTokens: 32000},
	{ID: "mimo-v2.6-flash-free", DisplayName: "MiMo-V2.6-Flash Free", ContextWindow: 200000, MaxTokens: 32000, Image: true},
	{ID: "deepseek-v4.1-flash-free", DisplayName: "DeepSeek V4.1 Flash Free", ContextWindow: 200000, MaxTokens: 32000, Image: true,
		Efforts: []string{"low", "high", "max"}, RequiresReasoningContent: true},
	{ID: "glm-5.3-flash-free", DisplayName: "GLM-5.3-Flash Free", ContextWindow: 200000, MaxTokens: 32000, Image: true,
		Efforts: []string{"low", "high", "max"}},
	{ID: "longcat-2.5-free", DisplayName: "LongCat 2.5 Free", ContextWindow: 1000000, MaxTokens: 262144, Image: true},
	{ID: "iceman-free", DisplayName: "Iceman Free", ContextWindow: 262144, MaxTokens: 131072},
	{ID: "inkling-2-small-free", DisplayName: "Inkling 2 Small Free", ContextWindow: 1048576, MaxTokens: 131072, Image: true,
		Efforts: []string{"none", "minimal", "low", "medium", "high", "max"}},
	{ID: "beam-free", DisplayName: "Beam Free", ContextWindow: 1048576, MaxTokens: 131072,
		Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	{ID: "flint-40b-free", DisplayName: "Flint 40B Free", ContextWindow: 262144, MaxTokens: 65536,
		Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	{ID: "nemotron-3.5-super-vl-free", DisplayName: "Nemotron 3.5 Super VL Free", ContextWindow: 1048576, MaxTokens: 131072, Image: true,
		Efforts: []string{"none", "minimal", "low", "medium", "high", "xhigh"}},
	{ID: "nemotron-3.5-lightning-free", DisplayName: "Nemotron 3.5 Lightning Free", ContextWindow: 262144, MaxTokens: 262144},
	{ID: "minimax-m3.1-flash-free", DisplayName: "MiniMax M3.1 Flash Free", ContextWindow: 1000000, MaxTokens: 512000, Image: true,
		Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	{ID: "hy4-flash-free", DisplayName: "Hy4 flash Free", ContextWindow: 1048576, MaxTokens: 65536, Image: true,
		Efforts: []string{"none", "high"}},
	{ID: "nemotron-3-ultra-free", DisplayName: "Nemotron 3 Ultra Free", ContextWindow: 128000, MaxTokens: 16384,
		Description: "Documented by OpenCode; absent from current upstream discovery. Limits are conservative."},
}

// opencodeFreeModel is one free chat model as OpenCode publishes it. Efforts is
// capabilities.reasoning.effort: the effort values the upstream route accepts. An empty Efforts
// means the model always reasons and exposes no effort control.
type opencodeFreeModel struct {
	ID            string
	DisplayName   string
	ContextWindow int
	MaxTokens     int
	Image         bool
	Efforts       []string
	// RequiresReasoningContent mirrors compat.requiresReasoningContentOnAssistantMessages, which
	// the DeepSeek route needs on every assistant message it is sent back.
	RequiresReasoningContent bool
	Description              string
}

func opencodeFreeChatModels() []GeneratedModel {
	models := make([]GeneratedModel, 0, len(opencodeFreeCatalog))
	for _, model := range opencodeFreeCatalog {
		models = append(models, model.generated())
	}
	return models
}

func (m opencodeFreeModel) generated() GeneratedModel {
	input := []string{"text"}
	if m.Image {
		input = append(input, "image")
	}
	compat := &ModelCompat{SupportsReasoningEffort: new(len(m.Efforts) > 0)}
	if m.RequiresReasoningContent {
		compat.RequiresReasoningContentOnAssistantMessages = new(true)
	}
	return GeneratedModel{
		ID: m.ID, DisplayName: m.DisplayName, Provider: OpenCodeFreeProviderID,
		API: APIOpenAICompletions, BaseURL: opencodeFreeBaseURL,
		ContextWindow: m.ContextWindow, MaxOutputTokens: m.MaxTokens,
		Reasoning: true, Capabilities: input, Compat: compat,
		ThinkingLevelMap: m.thinkingLevelMap(), Description: m.Description,
	}
}

// thinkingLevelMap pins the advertised efforts and marks every other level unsupported. A level
// absent from the map reads as supported and sends its own name as the effort, which a route that
// publishes a narrower list rejects.
func (m opencodeFreeModel) thinkingLevelMap() ThinkingLevelMap {
	if len(m.Efforts) == 0 {
		return nil
	}
	levels := make(ThinkingLevelMap, len(extendedThinkingLevels))
	for _, level := range extendedThinkingLevels {
		levels[level] = nil
	}
	for _, effort := range m.Efforts {
		levels[effortLevel(effort)] = new(effort)
	}
	return levels
}

// effortLevel is the runtime level an advertised effort value names. "none" is the wire value of
// the off level, not a level of its own.
func effortLevel(effort string) ModelThinkingLevel {
	if effort == "none" {
		return ThinkingOff
	}
	return ModelThinkingLevel(effort)
}
