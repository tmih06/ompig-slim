package ai

import "encoding/json"

// The bundled OMP 18.14.2 snapshot predates the free Hy4 route on the Go
// gateway. The gateway publishes it as a chat model with zero cost; this row
// keeps the catalog in step without regenerating the whole upstream snapshot.
// Source: https://opencode.ai/inference/go/v1/models.
const opencodeGoHy4FlashFreeRow = `{"id":"hy4-flash-free","provider":"opencode-go","name":"Hy4 flash Free","api":"openai-completions","baseUrl":"https://opencode.ai/inference/go/v1","reasoning":true,"input":["text","image"],"contextWindow":1048576,"maxTokens":65536,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"thinkingLevelMap":{"off":"none","minimal":null,"low":null,"medium":null,"high":"high","xhigh":null,"max":null},"compat":{"supportsDeveloperRole":false,"supportsReasoningEffort":true,"supportsStore":false,"supportsUsageInStreaming":true,"maxTokensField":"max_completion_tokens","supportsStrictMode":false,"thinkingFormat":"openai"}}`

// ompCatalogAdditions returns locally maintained rows for an OMP-owned provider.
// It runs inside the snapshot loader so listing, exact lookup and bare lookup
// all see the same row.
func ompCatalogAdditions(provider string) []json.RawMessage {
	if provider != "opencode-go" {
		return nil
	}
	return []json.RawMessage{json.RawMessage(opencodeGoHy4FlashFreeRow)}
}
