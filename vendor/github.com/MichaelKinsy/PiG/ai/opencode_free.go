package ai

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const OpenCodeFreeProviderID = "opencode-free"
const opencodeFreeBaseURL = "https://opencode.ai/inference/openai/v1"

//go:embed opencode_free_data/system.txt
var opencodeFreePreamble string

func init() {
	GeneratedProviders = append(GeneratedProviders, OpenCodeFreeProviderID)
	builtinProviderNames()[OpenCodeFreeProviderID] = "OpenCode Free"
	builtinProviderBaseURLs()[OpenCodeFreeProviderID] = opencodeFreeBaseURL
	builtinAPIKeyNames()[OpenCodeFreeProviderID] = "Public free tier"
	builtinAPIKeyAuths()[OpenCodeFreeProviderID] = &APIKeyAuth{
		Name: "Public free tier",
		Check: func(context.Context, APIKeyAuthInput) (*AuthCheck, error) {
			return &AuthCheck{Type: CredentialAPIKey, Source: "Built-in"}, nil
		},
		Resolve: func(context.Context, APIKeyAuthInput) (*AuthResult, error) {
			return &AuthResult{Auth: ModelAuth{APIKey: "public"}, Source: "Built-in"}, nil
		},
	}
	GeneratedModels = append(GeneratedModels, opencodeFreeChatModels()...)
	jev := GetBuiltinClassifierModel("opencode", "jev-1.13-free")
	jev.Provider = OpenCodeFreeProviderID
	GeneratedClassifierModels = append(GeneratedClassifierModels, *jev)
	builtinProviderConstructors[OpenCodeFreeProviderID] = OpenCodeFreeProvider
	builtinStreamWrappers[OpenCodeFreeProviderID] = withOpenCodeFreeCompatibility
}

// OpenCodeFreeProvider serves the public free tier without a separate runtime or credentials.
func OpenCodeFreeProvider() *ModelsProvider { return builtinProvider(OpenCodeFreeProviderID) }

func withOpenCodeFreeCompatibility(inner *ProviderStreams) *ProviderStreams {
	wrap := func(next ModelsStreamFunction) ModelsStreamFunction {
		return func(ctx context.Context, model *Model, transcript TranscriptContext, options StreamOptions) (*AssistantMessageEventStream, error) {
			if err := validateProviderRequest(ctx, transcript); err != nil {
				return nil, err
			}
			metadata, supported := LookupModelExact(OpenCodeFreeProviderID + "/" + model.ID)
			if !supported {
				return nil, fmt.Errorf("unsupported OpenCode free chat model %q", model.ID)
			}
			messages := slices.Clone(transcript.borrowed())
			preamble := strings.NewReplacer("{{model_name}}", metadata.DisplayName, "{{model_id}}", metadata.ID).Replace(opencodeFreePreamble)
			compatibility := preamble + "\n\nThe preceding tool inventory describes the compatibility client, not executable tools. Only call tools supplied in this request's tools array. Follow the caller's instructions for current workspace and tool usage.\n\n"
			if len(messages) > 0 {
				if initial, ok := messages[0].(SystemMessage); ok {
					initial.Content = SystemText(compatibility + systemContentText(initial.Content))
					messages[0] = initial
				} else {
					messages = append([]Message{SystemMessage{Content: SystemText(compatibility)}}, messages...)
				}
			}
			options.APIKey = "public"
			if options.MaxTokens == 0 {
				options.MaxTokens = min(16384, metadata.MaxOutputTokens)
			}
			options.Headers = opencodeFreeHeaders(options.Headers)
			if options.Fetch == nil {
				options.Fetch = opencodeFreeHTTPClient
			}
			return next(ctx, model, newTranscriptContext(messages), options)
		}
	}
	return &ProviderStreams{Stream: wrap(inner.Stream), StreamSimple: wrap(inner.StreamSimple)}
}

func opencodeFreeClassifier() *ProviderClassifier {
	return &ProviderClassifier{Classify: func(ctx context.Context, model *ClassifierModel, request ClassifierContext, options ClassifierOptions) (ClassifierResult, error) {
		if model.ID != "jev-1.13-free" {
			return ClassifierErrorResult(model, fmt.Errorf("unsupported OpenCode free judge model %q", model.ID), false), nil
		}
		options.APIKey, options.APIKeySet = "public", true
		options.Headers = opencodeFreeHeaders(options.Headers)
		if options.Fetch == nil {
			options.Fetch = opencodeFreeHTTPClient
		}
		return ClassifyTypesafeSystemOne(ctx, *model, request, options), nil
	}}
}

func opencodeFreeHeaders(headers ProviderHeaders) ProviderHeaders {
	headers = maps.Clone(headers)
	if headers == nil {
		headers = ProviderHeaders{}
	}
	session := opencodeFreeSessionID()
	for key, value := range map[string]string{
		"User-Agent":        "opencode/dev/0.0.0-dev-202611160550/cli",
		"x-opencode-client": "cli", "x-opencode-project": "global",
		"x-opencode-session-id": session, "x-opencode-session": session,
		"x-session-affinity": session, "X-Session-Id": session,
	} {
		headers[key] = new(value)
	}
	return headers
}

// Session identifiers follow OpenCode schema/identifier.ts at b75b158.
func opencodeFreeSessionID() string {
	value := ^(uint64(time.Now().UnixMilli())*4096 + 1)
	id := "ses_"
	for i := 0; i < 6; i++ {
		id += fmt.Sprintf("%02x", byte(value>>uint(40-8*i)))
	}
	var random [14]byte
	_, _ = rand.Read(random[:])
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for _, b := range random {
		id += string(chars[int(b)%len(chars)])
	}
	return id
}
