//go:build !pig_strip_websearch

package websearch

// The web_search definition: schema, settings surface, and the Execute that walks
// the free provider chain.
//
// Ports packages/coding-agent/src/web/search/index.ts (webSearchSchema and
// executeSearch) minus the model-registry layer, which this port does not have:
// every provider here is credential-free.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/internal/webtools"
)

// inputSchema is the model-facing parameter schema: a required query plus the
// optional filters upstream's webSearchSchema declares, with `timeout` replacing
// upstream's model-only knobs (a free chain has no grounding model to configure).
const inputSchema = `{
  "type": "object",
  "required": ["query"],
  "properties": {
    "query": {
      "type": "string",
      "description": "Search query. Supports site:, -site:, after:/before: YYYY-MM-DD, inurl:, intitle:, filetype:, \"quoted phrases\", -term and OR."
    },
    "limit": {
      "type": "number",
      "description": "Number of results to return, 1-20. Defaults to webSearch.results (10)."
    },
    "num_search_results": {
      "type": "number",
      "description": "Provider-native result breadth, 1-20. Defaults to limit."
    },
    "recency": {
      "type": "string",
      "enum": ["day", "week", "month", "year"],
      "description": "Restrict results to this time window."
    },
    "max_tokens": {
      "type": "number",
      "description": "Answer budget for providers that synthesize text. Keyless providers ignore it."
    },
    "timeout": {
      "type": "number",
      "description": "Per-provider timeout in seconds, clamped to 1-300. Defaults to webSearch.timeoutSeconds (60)."
    }
  }
}`

// settings are the `webSearch` namespace values this tool reads. The numeric
// fields are already bounded, so no caller multiplies or converts an unbounded
// configuration value.
type settings struct {
	// chain is the configured provider order; empty selects the free default.
	chain []string
	// timeoutSeconds bounds one provider transport, 1..300; zero selects the
	// default.
	timeoutSeconds float64
	// results is the default result count, 1..20; zero selects the default.
	results int
	// endpoints overrides a provider transport URL by chain id.
	endpoints map[string]string
}

// readSettings extracts the `webSearch` namespace. A missing namespace is the
// default configuration, not an error.
func readSettings(get func() extension.Settings) settings {
	var config settings
	if get == nil {
		return config
	}
	namespace, ok := get()["webSearch"].(map[string]any)
	if !ok {
		return config
	}
	if chain, ok := namespace["chain"].([]any); ok {
		for _, entry := range chain {
			if name, ok := entry.(string); ok {
				config.chain = append(config.chain, name)
			}
		}
	}
	// Clamp before any conversion: a configured 1e20 must not overflow an int or
	// a time.Duration.
	if seconds := number(namespace["timeoutSeconds"]); seconds > 0 {
		config.timeoutSeconds = min(seconds, float64(webtools.MaxSearchTimeout/time.Second))
	}
	if results := number(namespace["results"]); results >= 1 {
		config.results = webtools.ClampLimit(int(min(results, float64(webtools.MaxSearchResults))))
	}
	if endpoints, ok := namespace["endpoints"].(map[string]any); ok {
		for id, value := range endpoints {
			if endpoint, ok := value.(string); ok && strings.TrimSpace(endpoint) != "" {
				if config.endpoints == nil {
					config.endpoints = map[string]string{}
				}
				config.endpoints[strings.ToLower(strings.TrimSpace(id))] = strings.TrimSpace(endpoint)
			}
		}
	}
	return config
}

// number reads a JSON number from the decoded settings object.
func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	}
	return 0
}

// arguments are the call parameters after validation.
type arguments struct {
	query      string
	limit      int
	numResults int
	recency    string
	// timeout is kept in seconds, clamped to the provider-transport range, so no
	// multiplication happens before the bound is applied.
	timeoutSeconds int
}

// parseArguments decodes and validates a call. Bad arguments are a user error, not
// a runtime failure, so they return an error.
func parseArguments(raw json.RawMessage) (arguments, error) {
	var input struct {
		Query      string   `json:"query"`
		Limit      *float64 `json:"limit"`
		NumResults *float64 `json:"num_search_results"`
		Recency    string   `json:"recency"`
		MaxTokens  *float64 `json:"max_tokens"`
		Timeout    *float64 `json:"timeout"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return arguments{}, fmt.Errorf("invalid web_search input: %w", err)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return arguments{}, errors.New("query must not be empty")
	}

	args := arguments{query: query}
	var err error
	if args.limit, err = positiveInt(input.Limit, "limit", webtools.MaxSearchResults); err != nil {
		return arguments{}, err
	}
	if args.numResults, err = positiveInt(input.NumResults, "num_search_results", webtools.MaxSearchResults); err != nil {
		return arguments{}, err
	}
	if input.MaxTokens != nil {
		// max_tokens reaches grounded providers, which this free chain does not
		// have; the value is validated for schema fidelity and then ignored.
		if _, tokenErr := positiveInt(input.MaxTokens, "max_tokens", 0); tokenErr != nil {
			return arguments{}, tokenErr
		}
	}
	if input.Timeout != nil {
		seconds, timeoutErr := positiveInt(input.Timeout, "timeout", int(webtools.MaxSearchTimeout/time.Second))
		if timeoutErr != nil {
			return arguments{}, timeoutErr
		}
		args.timeoutSeconds = seconds
	}
	args.recency = strings.ToLower(strings.TrimSpace(input.Recency))
	switch args.recency {
	case "", "day", "week", "month", "year":
	default:
		return arguments{}, fmt.Errorf("recency must be day, week, month or year, not %q", input.Recency)
	}
	return args, nil
}

// positiveInt validates an optional positive integer parameter and caps it at
// max before the float-to-int conversion, so an absurd value cannot overflow the
// conversion or reach a provider as a huge request. A max of zero means the
// value has no range of its own.
func positiveInt(value *float64, name string, max int) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value != math.Trunc(*value) || *value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	if max > 0 && *value > float64(max) {
		return max, nil
	}
	return int(*value), nil
}

// Definition is the `web_search` tool definition.
func Definition(options tools.ExtendedBuiltinOptions) extension.ToolDefinition {
	readOnly, openWorld := true, true
	return extension.ToolDefinition{
		Name:          ToolName,
		Label:         ToolName,
		Description:   description,
		PromptSnippet: "Search the web using free providers and return cited sources",
		Parameters:    json.RawMessage(inputSchema),
		// Direct exposure: the model calls it, unlike codemode's model-only entry.
		Exposure: extension.ToolExposureDirect,
		Annotations: &extension.ToolAnnotations{
			ReadOnlyHint:  &readOnly,
			OpenWorldHint: &openWorld,
		},
		Execute: func(ctx context.Context, _ string, params json.RawMessage, _ extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
			config := readSettings(options.Settings)
			args, err := parseArguments(params)
			if err != nil {
				return extension.AgentToolResult{}, err
			}

			// RunSearch owns chain validation, defaults, and final bounds.
			limit := config.results
			if args.limit > 0 {
				limit = args.limit
			}
			timeout := time.Duration(config.timeoutSeconds * float64(time.Second))
			if args.timeoutSeconds > 0 {
				timeout = time.Duration(args.timeoutSeconds) * time.Second
			}

			result, err := webtools.RunSearch(ctx, webtools.SearchOptions{
				Query:      args.query,
				Limit:      limit,
				Recency:    args.recency,
				NumResults: args.numResults,
				Timeout:    timeout,
				Chain:      config.chain,
				Endpoints:  config.endpoints,
			})
			if err != nil {
				return extension.AgentToolResult{}, err
			}

			toolResult := agent.AgentToolResult{
				Content: []ai.ToolResultMessageContent{ai.TextContent{Text: result.Text}},
				Details: result.Details,
			}
			if result.IsError {
				toolResult.IsError = true
				toolResult.Preview = result.Details.Error
			}
			return toolResult, nil
		},
	}
}

// description is the model-facing tool description.
const description = "Search the web without an API key. Use it for current information that is not in the repo: news, " +
	"release notes, documentation, API changes, error messages. The query accepts Google-style operators (site:, " +
	"-site:, after:, before:, inurl:, intitle:, filetype:, \"phrase\", -term, OR); unsupported operators are relaxed " +
	"rather than returning nothing, and the result says so. Returns an answer when a provider synthesizes one, plus " +
	"numbered cited sources with a snippet each. Providers are tried in order, so a refusal falls through to the next " +
	"engine and the result names the failure. Known URLs → `web_fetch`."
