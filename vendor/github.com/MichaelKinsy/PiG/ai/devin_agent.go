package ai

// Devin (devin-agent) provider: Cascade chat over the Connect/protobuf
// transport. Ports packages/ai/src/providers/devin.ts onto the shared codec in
// devin_proto.go and PiG's assistantStreamBuilder.
//
// Ports packages/ai/src/providers/devin.ts (streamDevin, buildDevinChatRequest,
// buildChatMessagePrompts, fetchDevinAuthMetadata, assignDevinModel,
// readConnectTrailerError, devinErrorDetail, clampDevinTemperature).

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DevinConfig configures the Devin provider.
type DevinConfig struct {
	ModelMetadata *Model
	BaseURL       string
	APIKey        string
	// GetAPIKey resolves the request credential per call so a stored OAuth
	// credential (or a refreshed one) is used for each turn.
	GetAPIKey    func(context.Context) (string, error)
	Model        string
	ProviderID   string
	ExtraHeaders map[string]string
	Client       *http.Client
}

type devinAgentProvider struct {
	cfg    DevinConfig
	client *http.Client
	id     string
}

// NewDevinProvider builds the devin-agent provider.
func NewDevinProvider(cfg DevinConfig) (Provider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("devin: model is required")
	}
	return newDevinProvider(cfg), nil
}

func newDevinProvider(cfg DevinConfig) *devinAgentProvider {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	providerID := cfg.ProviderID
	if providerID == "" {
		providerID = devinProviderName
	}
	return &devinAgentProvider{cfg: cfg, client: client, id: providerID}
}

func init() {
	builtInProviders[APIDevinAgent] = func(apiKey, model, baseURL string) Provider {
		return newDevinProvider(DevinConfig{
			APIKey:     apiKey,
			Model:      model,
			BaseURL:    baseURL,
			ProviderID: devinProviderName,
		})
	}
}

func (p *devinAgentProvider) ID() string { return p.id }

func (p *devinAgentProvider) Close() error { return nil }

func (p *devinAgentProvider) resolveModel() *Model {
	if p.cfg.ModelMetadata != nil {
		return new(*p.cfg.ModelMetadata)
	}
	if generated, ok := LookupModelExact(p.cfg.ProviderID + "/" + p.cfg.Model); ok {
		return generated.ToModel()
	}
	return &Model{
		ID: p.cfg.Model, Input: []string{ModelInputText},
		ProviderMeta: ProviderMetadata{API: APIDevinAgent, ProviderID: p.cfg.ProviderID, BaseURL: p.cfg.BaseURL, Headers: p.cfg.ExtraHeaders},
		Capabilities: ModelCapabilities{SupportsToolUse: true, ContextWindow: 200000, MaxOutputTokens: 64000},
	}
}

// devinCompat reads one member of the model's compat object: the catalog's
// unmodeled members arrive through Compat.Extra as raw JSON. ok is false when
// the member is absent or not a T.
func devinCompat[T any](model *Model, key string) (value T, ok bool) {
	if model.ProviderMeta.Compat == nil {
		return value, false
	}
	raw, present := model.ProviderMeta.Compat.Extra[key]
	if !present || json.Unmarshal(raw, &value) != nil {
		var zero T
		return zero, false
	}
	return value, true
}

// devinNormalizeToolCallID ports normalizeToolCallId for the Devin wire:
// Responses-family composites keep their call_ component; the result is
// sanitized to [a-zA-Z0-9_-] and capped at 40 chars.
func devinNormalizeToolCallID(id string) string {
	if pipe := strings.Index(id, "|"); pipe > 0 {
		id = id[:pipe]
	}
	alphanumeric := func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' }
	illegal := func(r rune) bool { return !alphanumeric(r) && r != '_' && r != '-' }
	if id != "" && len(id) <= devinToolCallIDMaxLength && strings.IndexFunc(id, illegal) < 0 {
		return id
	}
	if filtered := strings.Map(func(r rune) rune {
		if illegal(r) {
			return -1
		}
		return r
	}, id); filtered != "" && len(filtered) <= devinToolCallIDMaxLength {
		return filtered
	}
	var out strings.Builder
	for _, r := range ShortHash(id) {
		if alphanumeric(r) {
			out.WriteRune(r)
			if out.Len() >= devinToolCallIDMaxLength {
				break
			}
		}
	}
	if out.Len() == 0 {
		return "call"
	}
	return out.String()
}

// devinContentParts splits user or tool-result blocks into text and image parts.
func devinContentParts[B ContentBlock](blocks []B) (text string, images [][2]string) {
	for _, block := range blocks {
		switch b := any(block).(type) {
		case TextContent:
			text += b.Text
		case ImageContent:
			if b.Data != "" {
				images = append(images, [2]string{b.Data, b.MimeType})
			}
		}
	}
	return text, images
}

// devinBuildPrompts maps the transformed history onto ChatMessagePrompts
// (buildChatMessagePrompts). messageId seeds are cascadeId\0index\0role[...].
func devinBuildPrompts(messages []Message, cascadeID string, model *Model) []devinChatMessagePrompt {
	prompts := make([]devinChatMessagePrompt, 0, len(messages))
	for index, msg := range messages {
		switch m := msg.(type) {
		case UserMessage:
			var text string
			var images [][2]string
			switch content := m.Content.(type) {
			case UserText:
				text = string(content)
			case UserContentBlocks:
				text, images = devinContentParts(content)
			}
			prompts = append(prompts, devinChatMessagePrompt{
				messageID: devinDeterministicUUID(fmt.Sprintf("%s\x00%d\x00%s", cascadeID, index, m.messageRole())),
				source:    1, // CHAT_MESSAGE_SOURCE_USER
				prompt:    text,
				images:    images,
			})
		case AssistantMessage:
			isNative := m.API == model.ProviderMeta.API && m.Provider == model.ProviderMeta.ProviderID && m.Model == model.ID
			var promptText, thinkingText, signature string
			var toolCalls [][3]string
			for _, block := range m.Content {
				switch b := block.(type) {
				case TextContent:
					promptText += b.Text
				case ThinkingContent:
					thinkingText += b.Thinking
					if isNative && signature == "" {
						signature = b.ThinkingSignature
					}
				case ToolCall:
					args, _ := json.Marshal(b.Arguments)
					toolCalls = append(toolCalls, [3]string{b.ID, b.Name, string(args)})
				}
			}
			if promptText == "" && thinkingText == "" && signature == "" && len(toolCalls) == 0 {
				continue
			}
			messageID := devinDeterministicUUID(fmt.Sprintf("%s\x00%d\x00assistant", cascadeID, index))
			if isNative && m.ResponseID != "" {
				messageID = m.ResponseID
			}
			prompts = append(prompts, devinChatMessagePrompt{
				messageID: messageID,
				source:    2, // CHAT_MESSAGE_SOURCE_SYSTEM
				prompt:    promptText,
				thinking:  thinkingText,
				signature: signature,
				toolCalls: toolCalls,
			})
		case ToolResultMessage:
			text, images := devinContentParts(m.Content)
			prompts = append(prompts, devinChatMessagePrompt{
				messageID:       devinDeterministicUUID(fmt.Sprintf("%s\x00%d\x00tool\x00%s", cascadeID, index, m.ToolCallID)),
				source:          4, // CHAT_MESSAGE_SOURCE_TOOL
				toolCallID:      m.ToolCallID,
				toolResultIsErr: m.IsError,
				prompt:          text,
				images:          images,
			})
		}
	}
	return prompts
}

// devinLastUserPrompt finds the last USER prompt for AssignModel's router prompt.
func devinLastUserPrompt(prompts []devinChatMessagePrompt) devinChatMessagePrompt {
	for index := len(prompts) - 1; index >= 0; index-- {
		if prompts[index].source == 1 {
			return prompts[index]
		}
	}
	return devinChatMessagePrompt{}
}

// devinIsGeminiModel reports whether the wire uid routes through a Gemini
// backend needing Google-normalized tool schemas.
func devinIsGeminiModel(modelID, chatModelUID string) bool {
	lower := strings.ToLower(modelID + " " + chatModelUID)
	return strings.Contains(lower, "gemini") || strings.HasPrefix(chatModelUID, "MODEL_GOOGLE_GEMINI_")
}

// devinNormalizeSchemaForGoogle is the Devin-needed subset of
// normalizeSchemaForGoogle: JSON-Schema type arrays collapse to a scalar type
// plus nullable, recursing through every value. Devin's Gemini backend rejects
// type arrays as an opaque invalid_argument.
func devinNormalizeSchemaForGoogle(value any) any {
	switch node := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for key, member := range node {
			out[key] = devinNormalizeSchemaForGoogle(member)
		}
		if typeValue, present := out["type"]; present {
			if types, ok := typeValue.([]any); ok {
				var scalar string
				nullable := false
				var branches []any
				for _, entry := range types {
					text, _ := entry.(string)
					switch text {
					case "null":
						nullable = true
					case "":
					default:
						if scalar == "" {
							scalar = text
						} else {
							branches = append(branches, map[string]any{"type": text})
						}
					}
				}
				switch {
				case scalar != "" && len(branches) == 0:
					out["type"] = scalar
					if nullable {
						out["nullable"] = true
					} else {
						delete(out, "nullable")
					}
				default:
					delete(out, "type")
					delete(out, "nullable")
					if scalar != "" {
						branches = append([]any{map[string]any{"type": scalar}}, branches...)
					}
					if nullable {
						branches = append(branches, map[string]any{"type": "null"})
					}
					if len(branches) > 0 {
						out["anyOf"] = branches
					}
				}
			}
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, member := range node {
			out[i] = devinNormalizeSchemaForGoogle(member)
		}
		return out
	default:
		return value
	}
}

// devinToolSchemaJSON resolves a tool's wire schema, Google-normalized for
// Gemini-routed models. strict mirrors tool.strict.
func devinToolSchemaJSON(tool ToolSchema, google bool) (string, bool) {
	schema := any(tool.Parameters)
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if google {
		schema = devinNormalizeSchemaForGoogle(schema)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return "{}", false
	}
	return string(encoded), tool.ConstrainedSampling != nil
}

// devinUnary posts a protobuf unary RPC and decodes the response.
func devinUnary[T any](ctx context.Context, client *http.Client, baseURL, path string, body []byte, decode func([]byte) *T) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/proto")
	req.Header.Set("connect-protocol-version", "1")
	req.Header.Set("accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, devinMaxConnectFrameBytes+1))
	if err != nil {
		return nil, err
	}
	op := path[strings.LastIndex(path, "/")+1:]
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, devinHTTPError(op, resp.StatusCode, resp.Status, resp.Header.Get("Content-Type"), payload)
	}
	decoded := decodeDevinUnaryMessage(payload, decode)
	if decoded == nil {
		return nil, fmt.Errorf("Devin %s error: response could not be decoded", op)
	}
	return decoded, nil
}

// devinFetchAuthMetadata is fetchDevinAuthMetadata: GetUserJwt returns the
// user JWT and an optional custom API server URL.
func devinFetchAuthMetadata(ctx context.Context, client *http.Client, apiKey, baseURL string) (userJWT, chatBaseURL string, err error) {
	response, err := devinUnary(ctx, client, baseURL, devinAuthPath, encodeDevinMetadataRequest(apiKey), decodeDevinGetUserJwtResponse)
	if err != nil {
		return "", "", err
	}
	if response.userJWT == "" {
		return "", "", fmt.Errorf("Devin auth error: GetUserJwt returned an empty user JWT")
	}
	chatBaseURL = baseURL
	if custom := strings.TrimRight(strings.TrimSpace(response.customAPIBaseURL), "/"); custom != "" {
		chatBaseURL = custom
	}
	return response.userJWT, chatBaseURL, nil
}

// devinAssignModel is assignDevinModel: resolve a router uid into a concrete
// model uid plus the assignment JWT. A failed assignment fails the turn.
func devinAssignModel(ctx context.Context, client *http.Client, model *Model, apiKey, cascadeID, chatBaseURL string, prompt devinChatMessagePrompt) (*devinModelAssignment, error) {
	routerUID, _ := devinCompat[string](model, "requestModelId")
	if routerUID == "" {
		routerUID = model.ID
	}
	assignment, err := devinUnary(ctx, client, chatBaseURL, devinAssignModelPath,
		devinAssignModelRequest(apiKey, routerUID, cascadeID, prompt), decodeDevinAssignModelResponse)
	if err != nil {
		return nil, err
	}
	if assignment.assignmentJWT == "" || assignment.modelUID == "" {
		return nil, fmt.Errorf("Devin AssignModel error: response carried no assignment JWT and model uid")
	}
	return assignment, nil
}

// devinClampTemperature is clampDevinTemperature: the wire floors temperature
// at a positive value (the catalog temperatureFloor compat member).
func devinClampTemperature(temperature float64, temperatureSet bool, floor float64, hasFloor bool) float64 {
	value := temperature
	if !temperatureSet {
		value = 1
	}
	if hasFloor && value < floor {
		return floor
	}
	return value
}

// devinStopPatterns reads the stopSequences sampling value (a string or a
// list of strings) into the CompletionConfiguration stop_patterns list.
func devinStopPatterns(value any) []string {
	switch value := value.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []string:
		return value
	case []any:
		var patterns []string
		for _, item := range value {
			if pattern, ok := item.(string); ok && pattern != "" {
				patterns = append(patterns, pattern)
			}
		}
		return patterns
	}
	return nil
}

// Stream runs one Devin turn: GetUserJwt, optional AssignModel, then the
// Connect-framed GetChatMessage stream.
func (p *devinAgentProvider) Stream(ctx context.Context, transcript TranscriptContext, opts StreamOptions) (*AssistantMessageEventStream, error) {
	if opts.RepairToolFlow {
		transcript = transcript.withRepairedToolFlow()
	}
	if err := validateProviderRequest(ctx, transcript); err != nil {
		return nil, fmt.Errorf("devin: invalid transcript: %w", err)
	}
	model := p.resolveModel()

	apiKey := opts.APIKey
	if apiKey == "" && p.cfg.GetAPIKey != nil {
		resolved, err := p.cfg.GetAPIKey(ctx)
		if err != nil {
			return nil, fmt.Errorf("devin: resolve API key: %w", err)
		}
		apiKey = resolved
	}
	if apiKey == "" {
		apiKey = p.cfg.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key for Devin provider")
	}

	resolved := resolvedMessages(transcript, false)
	messages := TransformMessages(resolved, model, func(id string, _ *Model, _ AssistantMessage) string {
		return devinNormalizeToolCallID(id)
	})
	systemPrompt := ""
	if len(messages) > 0 {
		systemPrompt = GetCurrentSystemPrompt(messages[:1])
	}

	cascadeID := opts.SessionID
	if cascadeID == "" {
		cascadeID = randomUUIDString()
	}
	prompts := devinBuildPrompts(messages, cascadeID, model)

	modelRouter, _ := devinCompat[bool](model, "modelRouter")
	chatModelUID := model.ID
	if !modelRouter {
		if requestModel, _ := devinCompat[string](model, "requestModelId"); requestModel != "" {
			chatModelUID = requestModel
		}
	}
	googleSchema := devinIsGeminiModel(model.ID, chatModelUID)
	tools := borrowedCurrentTools(messages)
	toolDefs := make([][]byte, 0, len(tools))
	for _, tool := range tools {
		schema, strict := devinToolSchemaJSON(tool, googleSchema)
		toolDefs = append(toolDefs, devinToolDefinition(tool.Name, tool.Description, schema, strict))
	}

	baseURL := strings.TrimRight(firstNonEmptyString(p.cfg.BaseURL, model.ProviderMeta.BaseURL, devinDefaultBaseURL), "/")
	client := providerHTTPClient(p.client, opts.Fetch)

	builder := newObservedProviderBuilder(ctx, outputAPI(model, APIDevinAgent), p.id, model.ID)
	builder.modelCost = opts.ModelCost
	builder.setProviderEventObserver(opts, model)

	go func() {
		p.runTurn(ctx, client, opts, model, apiKey, baseURL, systemPrompt, prompts, toolDefs, cascadeID, chatModelUID, modelRouter, builder)
	}()
	return builder.stream, nil
}

// runTurn executes auth, optional router assignment, and the chat stream,
// driving builder events. Every failure after stream creation is an error
// event, matching streamDevin's async IIFE.
func (p *devinAgentProvider) runTurn(ctx context.Context, client *http.Client, opts StreamOptions, model *Model, apiKey, baseURL, systemPrompt string, prompts []devinChatMessagePrompt, toolDefs [][]byte, cascadeID, chatModelUID string, modelRouter bool, builder *assistantStreamBuilder) {
	fail := func(err error) {
		reason := StopReasonError
		if ctx.Err() != nil {
			reason = StopReasonAborted
		}
		builder.fail(reason, err)
	}

	userJWT, chatBaseURL, err := devinFetchAuthMetadata(ctx, client, apiKey, baseURL)
	if err != nil {
		fail(err)
		return
	}

	assignmentJWT := ""
	if modelRouter {
		assignment, err := devinAssignModel(ctx, client, model, apiKey, cascadeID, chatBaseURL, devinLastUserPrompt(prompts))
		if err != nil {
			fail(err)
			return
		}
		assignmentJWT = assignment.assignmentJWT
		chatModelUID = assignment.modelUID
		builder.partial.ResponseModel = assignment.modelUID
	}

	maxTokens := uint64(64000)
	if opts.MaxTokens > 0 {
		maxTokens = uint64(opts.MaxTokens)
	} else if model.Capabilities.MaxOutputTokens > 0 {
		maxTokens = uint64(model.Capabilities.MaxOutputTokens)
	}
	floor, hasFloor := devinCompat[float64](model, "temperatureFloor")
	topP := float64(float32(0.95))
	if value, ok := jsonNumberValue(opts.SamplingParams["topP"]); ok {
		topP = float64(float32(value))
	}
	config := devinCompletionConfiguration{
		maxTokens:    maxTokens,
		temperature:  devinClampTemperature(opts.Temperature, opts.TemperatureSet, floor, hasFloor),
		topP:         topP,
		stopPatterns: devinStopPatterns(opts.SamplingParams["stopSequences"]),
	}

	parallelToolCalls, _ := devinCompat[bool](model, "supportsParallelToolCalls")
	requestBytes := devinGetChatMessageRequest(apiKey, userJWT, systemPrompt, chatModelUID, cascadeID, randomUUIDString(), prompts, toolDefs, !parallelToolCalls, assignmentJWT, config)

	compress := len(requestBytes) >= devinMinCompressedBody
	body := requestBytes
	flags := 0
	if compress {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(requestBytes); err != nil {
			fail(err)
			return
		}
		if err := writer.Close(); err != nil {
			fail(err)
			return
		}
		body = compressed.Bytes()
		flags = devinConnectCompressedFlag
	}
	frame := devinFrameConnectMessage(body, flags)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatBaseURL+devinChatMessagePath, bytes.NewReader(frame))
	if err != nil {
		fail(err)
		return
	}
	req.Header.Set("content-type", "application/connect+proto")
	req.Header.Set("connect-protocol-version", "1")
	req.Header.Set("accept-encoding", "identity")
	req.Header.Set("user-agent", "connect-go/1.18.1 (go1.26.3)")
	req.Header.Set("connect-accept-encoding", "gzip")
	if compress {
		req.Header.Set("connect-content-encoding", "gzip")
	}
	for name, value := range p.cfg.ExtraHeaders {
		req.Header.Set(name, value)
	}
	applyIdentityHeaders(req.Header, ClientIdentityTarget{Provider: p.id, BaseURL: baseURL}, opts.Identity)
	applyProviderHeaders(req, opts.Headers)

	resp, err := client.Do(req)
	if err != nil {
		fail(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if err := observeProviderResponse(ctx, opts, resp, model); err != nil {
		fail(err)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, devinMaxConnectFrameBytes+1))
		fail(devinHTTPError("API", resp.StatusCode, resp.Status, resp.Header.Get("Content-Type"), payload))
		return
	}
	if resp.Body == nil {
		fail(fmt.Errorf("Devin API error: response body is empty"))
		return
	}

	_, builder.managed = resp.Body.(*observedResponseBody)
	builder.start()
	state := &devinTurnState{builder: builder, toolCallIndex: map[string]int{}}
	consumeErr := builder.responseTurn(func() error {
		return p.consumeFrames(ctx, resp.Body, state)
	})
	if consumeErr != nil {
		fail(consumeErr)
		return
	}
	if ctx.Err() != nil {
		fail(context.Cause(ctx))
		return
	}

	// Terminal: any tool call -> toolUse; MAX_TOKENS(3) -> length; else stop.
	reason := StopReasonStop
	if len(state.toolCallIndex) > 0 {
		reason = StopReasonToolUse
	} else if state.latestStop == 3 {
		reason = StopReasonLength
	}
	usage := state.usage
	usage.TotalTokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	builder.calculateCost(&usage)
	builder.done(reason, &usage, "")
}

// devinTurnState accumulates one streamed response's mutable state.
type devinTurnState struct {
	builder       *assistantStreamBuilder
	toolCallIndex map[string]int
	activeToolID  string
	latestStop    int
	usage         Usage
}

// consumeFrames reads the Connect stream and drives builder events.
func (p *devinAgentProvider) consumeFrames(ctx context.Context, body io.ReadCloser, state *devinTurnState) error {
	defer func() { _ = body.Close() }()
	decoder := newDevinConnectFrameDecoder()
	buffer := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		n, err := body.Read(buffer)
		if n > 0 {
			frames, decodeErr := decoder.decode(buffer[:n])
			if decodeErr != nil {
				return decodeErr
			}
			for _, frame := range frames {
				payload := frame.payload
				if frame.flags&devinConnectCompressedFlag != 0 {
					decompressed, gunzipErr := devinGunzip(payload)
					if gunzipErr != nil {
						return gunzipErr
					}
					payload = decompressed
				}
				if frame.flags&devinConnectEndStreamFlag != 0 {
					if trailer := readDevinConnectTrailerError(string(payload)); trailer != nil {
						return trailer
					}
				} else if message := decodeDevinChatMessage(payload); message != nil {
					state.applyMessage(message)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// applyMessage folds one GetChatMessageResponse into builder events. The
// builder's Start/Delta methods own block boundaries: thinkingStart ends text
// and textStart ends thinking, matching Pi's block model.
func (s *devinTurnState) applyMessage(message *devinChatMessage) {
	builder := s.builder
	if message.messageID != "" && builder.partial.ResponseID == "" {
		builder.partial.ResponseID = message.messageID
	}
	if message.actualModelUID != "" {
		builder.partial.ResponseModel = message.actualModelUID
	}
	if message.deltaThinking != "" {
		builder.thinkingDelta(message.deltaThinking, false)
	}
	if message.deltaSignature != "" {
		builder.thinkingSignature(message.deltaSignature)
	}
	if message.deltaText != "" {
		builder.textDelta(message.deltaText)
	}
	for _, call := range message.toolCallDeltas {
		s.applyToolCallDelta(call)
	}
	if message.stopReason != 0 {
		s.latestStop = message.stopReason
		builder.partial.RawStopReason = fmt.Sprintf("%d", message.stopReason)
	}
	if message.usage != nil {
		s.usage.Input = int(message.usage.inputTokens)
		s.usage.Output = int(message.usage.outputTokens)
		s.usage.CacheWrite = int(message.usage.cacheWriteTokens)
		s.usage.CacheRead = int(message.usage.cacheReadTokens)
	}
}

// applyToolCallDelta accumulates one ChatToolCall delta. The server may send
// cumulative or incremental argumentsJson; normalize to the accumulated text.
func (s *devinTurnState) applyToolCallDelta(call devinChatToolCallDelta) {
	builder := s.builder
	toolCallID := call.id
	if toolCallID == "" {
		toolCallID = s.activeToolID
	}
	if toolCallID == "" {
		return
	}
	index, exists := s.toolCallIndex[toolCallID]
	if !exists {
		index = len(s.toolCallIndex)
		s.toolCallIndex[toolCallID] = index
		builder.toolCallStart(streamToolCallDelta{index: index, id: toolCallID, name: call.name})
	} else if call.name != "" {
		// A later delta may carry the name (omp: `if (tc.name) block.name = tc.name`).
		builder.toolCallDelta(streamToolCallDelta{index: index, name: call.name, skipEmpty: true})
	}
	s.activeToolID = toolCallID
	if call.argumentsJSON == "" {
		return
	}
	// The builder records the accumulated text; reading it is free, while a
	// copy per delta made long incremental arguments quadratic.
	delta := call.argumentsJSON
	if previous := builder.toolCalls[index].args.Text(); strings.HasPrefix(delta, previous) {
		delta = delta[len(previous):]
	}
	builder.toolCallDelta(streamToolCallDelta{index: index, argumentsDelta: delta, recordOnly: true})
}
