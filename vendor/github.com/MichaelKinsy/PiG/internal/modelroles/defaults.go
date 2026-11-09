package modelroles

// RoleDefaults returns OMP's catalog priority chain, copied from
// packages/catalog/src/compat/rules/runtime/behavior.kdl. The returned slice is
// detached so UI rendering and runtime resolution cannot mutate shared policy.
func RoleDefaults(role string) []string {
	switch role {
	case "advisor":
		role = "slow"
	case "tiny", "memory", "widget":
		role = "smol"
	}
	switch role {
	case "smol":
		return []string{"claude-haiku-5-5", "claude-haiku-5.5", "haiku-5-5", "haiku-5.5", "google-antigravity/gemini-3.8-flash", "gemini-3.8-flash", "gemini-3-8-flash", "gemini-3.7-flash", "gemini-3-7-flash", "gemini-3.6-flash", "gemini-3-6-flash", "gemini-3.5-flash", "gemini-3-5-flash", "openai-codex/gpt-5.3-codex-spark", "gpt-5.3-codex-spark", "5.3-spark", "spark", "zai/glm-5.3-flash", "glm-5.3-flash", "gemini-3.1-flash-lite", "gemini-3-1-flash-lite", "flash-lite", "cerebras/gpt-oss-120b", "gpt-oss-120b", "oss-120b", "cerebras/zai-glm-4.7", "cerebras/zai-glm-4.6", "claude-haiku-4-5", "claude-haiku-4.5", "haiku-4-5", "haiku-4.5", "haiku", "openai-codex/gpt-5.6-luna", "gpt-5.6-luna", "openai-codex/gpt-5.4-mini", "gpt-5.4-mini", "-mini"}
	case "slow":
		return []string{"openai-codex/gpt-5.6-sol", "gpt-5.6-sol", "anthropic/claude-fable-5-1", "claude-fable-5-1", "claude-fable-5.1", "fable-5.1", "fable-5-1", "anthropic/claude-fable-5", "claude-fable-5", "fable-5", "kimi-code/k3", "kimi-k3", "moonshotai/kimi-k3", "k3", "zai/glm-5.3", "glm-5.3", "anthropic/claude-opus-5-5", "claude-opus-5-5", "opus-5-5", "anthropic/claude-opus-5", "claude-opus-5", "opus-5", "gpt-5.5", "claude-opus-4-8", "claude-opus-4.8", "opus-4-8", "opus-4.8"}
	case "image":
		return []string{"openai/gpt-image-2", "openai-codex/gpt-image-2", "google-antigravity/gemini-3-pro-image", "xai/grok-imagine-image", "xai-oauth/grok-imagine-image", "openrouter/google/gemini-3-pro-image", "google/gemini-3-pro-image", "deepinfra/black-forest-labs/FLUX-2-pro", "factory-droid/gpt-image-2.5-sunburst", "factory-droid/gpt-image-2.5-flare", "factory-droid/gemini-3-pro-image-preview"}
	case "web":
		return []string{"web/parallel", "web/hosted", "web/exa", "web/firecrawl", "web/searxng", "web/startpage", "web/duckduckgo", "web/ecosia", "web/google", "web/mojeek", "web/public"}
	case "speech":
		return []string{"local/kokoro", "xai/grok-tts", "xai-oauth/grok-tts"}
	case "dictation":
		return []string{"local/parakeet-tdt-0.6b-v3"}
	case "judge":
		return []string{"typesafe/jev-1.14.0", "openrouter/typesafe/jev-1.14", "@tiny", "@smol", "@default"}
	}
	return nil
}
