package ai

// Ports the request building of @oh-my-pi/pi-ai src/providers/google-gemini-cli.ts (buildRequest,
// partsToContents, the Antigravity request envelope) and the message conversion of
// src/providers/google-shared.ts for the Cloud Code Assist wire.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

// ─── Wire types ──────────────────────────────────────────────────────────────
//
// Members are declared in the order the source's object literals create them, so the encoded body keeps
// Pi's member order.

// ccaPart is one content part of the Cloud Code Assist request. The text/thought pair is written in the
// order the source builds it: thought first for a thinking part, text first for a visible part.
type ccaPart struct {
	Text             *string           `json:"text,omitempty"`
	Thought          *bool             `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *ccaFunctionCall  `json:"functionCall,omitempty"`
	FunctionResponse *ccaFunctionReply `json:"functionResponse,omitempty"`
	InlineData       *geminiInlineData `json:"inlineData,omitempty"`
}

// ccaFunctionCall is a function call part's call: name, args, then the optional id.
type ccaFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
	ID   string          `json:"id,omitempty"`
}

// ccaFunctionReply is a function response part's reply: name, response, the multimodal parts, then the id.
type ccaFunctionReply struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
	Parts    []ccaPart      `json:"parts,omitempty"`
	ID       string         `json:"id,omitempty"`
}

// ccaContent is one turn: role and parts.
type ccaContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []ccaPart `json:"parts"`
}

// ccaSystemInstruction is the request's system instruction; Antigravity tags it with role "user".
type ccaSystemInstruction struct {
	Role  string            `json:"role,omitempty"`
	Parts []ccaTextPartOnly `json:"parts"`
}

// ccaTextPartOnly is a system instruction part.
type ccaTextPartOnly struct {
	Text string `json:"text"`
}

// ccaGenerationConfig is the request's generation configuration, in the source's member order.
type ccaGenerationConfig struct {
	MaxOutputTokens *int                  `json:"maxOutputTokens,omitempty"`
	Temperature     *float64              `json:"temperature,omitempty"`
	TopP            *float64              `json:"topP,omitempty"`
	TopK            *int                  `json:"topK,omitempty"`
	PresencePenalty *float64              `json:"presencePenalty,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

// ccaToolDecl is the tools array's single object: the function declarations.
type ccaToolDecl struct {
	FunctionDeclarations []ccaFunctionDeclaration `json:"functionDeclarations"`
}

// ccaFunctionDeclaration is one tool declaration: name, description, then the schema under the field the
// target accepts (`parameters` for Claude and every Antigravity row, `parametersJsonSchema` for Gemini CLI
// Gemini rows).
type ccaFunctionDeclaration struct {
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Parameters           any    `json:"parameters,omitempty"`
	ParametersJSONSchema any    `json:"parametersJsonSchema,omitempty"`
}

// ccaFunctionCallingConfig is the toolConfig's calling configuration.
type ccaFunctionCallingConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// ccaToolConfig is the request's tool configuration.
type ccaToolConfig struct {
	FunctionCallingConfig ccaFunctionCallingConfig `json:"functionCallingConfig"`
}

// ccaRequestEnvelope is the `request` member of the Cloud Code Assist body.
type ccaRequestEnvelope struct {
	Contents          []ccaContent          `json:"contents"`
	Labels            *ccaLabels            `json:"labels,omitempty"`
	SessionID         string                `json:"sessionId,omitempty"`
	SystemInstruction *ccaSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *ccaGenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []ccaToolDecl         `json:"tools,omitempty"`
	ToolConfig        *ccaToolConfig        `json:"toolConfig,omitempty"`
}

// ccaRequestBody is the body: project, model, request, then the Antigravity request id, type and user agent.
type ccaRequestBody struct {
	Project     string             `json:"project"`
	Model       string             `json:"model"`
	Request     ccaRequestEnvelope `json:"request"`
	RequestID   string             `json:"requestId,omitempty"`
	RequestType string             `json:"requestType,omitempty"`
	UserAgent   string             `json:"userAgent,omitempty"`
}

// ccaLabels is the Antigravity envelope's labels, written in alphabetical key order as the native client
// orders them. used_claude and used_claude_conservative are ALWAYS JSON strings: the backend answers 403
// "You do not have a valid license of this product" for an object there.
type ccaLabels struct {
	CascadeID              string `json:"cascade_id"`
	LastExecutionID        string `json:"last_execution_id,omitempty"`
	LastStepIndex          string `json:"last_step_index"`
	ModelEnum              string `json:"model_enum,omitempty"`
	RequestID              string `json:"request_id"`
	RootCascadeID          string `json:"root_cascade_id"`
	TrajectoryID           string `json:"trajectory_id"`
	UsedClaude             string `json:"used_claude"`
	UsedClaudeConservative string `json:"used_claude_conservative"`
	UsedNonGeminiModel     string `json:"used_non_gemini_model"`
}

// ─── Request building ────────────────────────────────────────────────────────

// ccaRequestPlan is everything one request build needs beyond the transcript.
type ccaRequestPlan struct {
	providerID  string
	projectID   string
	accountKey  string
	wireModelID string
	// reviewed marks a wire id selected by an effort-routed family.
	reviewed      bool
	isAntigravity bool
	class         ccaIdentityClass
	options       StreamOptions
	ccaOptions    *GoogleCloudCodeOptions
}

// ccaBuildRequestBody builds the Cloud Code Assist request for one turn.
func ccaBuildRequestBody(ctx context.Context, model *Model, transcript TranscriptContext, plan ccaRequestPlan) (ccaRequestBody, error) {
	messages := collapsedMessages(transcript)
	systemPrompts := ccaSystemPrompts(messages)
	contents, err := ccaConvertMessages(model, transcript, plan.providerID)
	if err != nil {
		return ccaRequestBody{}, err
	}
	class := plan.class
	request := ccaRequestEnvelope{Contents: contents}

	if generation, err := ccaGenerationConfigFor(model, plan.options, plan.ccaOptions); err != nil {
		return ccaRequestBody{}, err
	} else if generation != nil {
		request.GenerationConfig = generation
	}
	if len(systemPrompts) > 0 {
		instruction := &ccaSystemInstruction{Parts: make([]ccaTextPartOnly, len(systemPrompts))}
		for i, text := range systemPrompts {
			instruction.Parts[i] = ccaTextPartOnly{Text: text}
		}
		if plan.isAntigravity {
			instruction.Role = "user"
		}
		request.SystemInstruction = instruction
	}

	tools := borrowedCurrentTools(messages)
	if len(tools) > 0 {
		declarations, err := ccaConvertTools(tools, class, plan.isAntigravity)
		if err != nil {
			return ccaRequestBody{}, err
		}
		request.Tools = []ccaToolDecl{{FunctionDeclarations: declarations}}
		request.Contents = ccaApplyToolChoice(&request, request.Contents, plan.options.ToolChoice, class, plan.isAntigravity)
		// Antigravity's default tool mode is VALIDATED; an explicit non-auto choice above wins.
		if plan.isAntigravity && request.ToolConfig == nil {
			request.ToolConfig = &ccaToolConfig{FunctionCallingConfig: ccaFunctionCallingConfig{Mode: "VALIDATED"}}
		}
	}
	// Claude on Antigravity always forces VALIDATED, even with no tools declared.
	if plan.isAntigravity && class == ccaIdentityAnthropic {
		request.ToolConfig = &ccaToolConfig{FunctionCallingConfig: ccaFunctionCallingConfig{Mode: "VALIDATED"}}
	}

	if plan.isAntigravity {
		profile := ccaWireProfileForAccount(plan.accountKey, plan.wireModelID)
		ccaApplyWireProfile(&request, plan.options, profile, plan.reviewed)
		envelope := ccaBuildAntigravityEnvelope(ctx, model, transcript, class, profile, plan.ccaOptions)
		request.Labels = envelope.labels
		request.SessionID = envelope.sessionID
		return ccaRequestBody{Project: plan.projectID, Model: plan.wireModelID, Request: request, RequestID: envelope.requestID, UserAgent: "antigravity"}, nil
	}
	return ccaRequestBody{Project: plan.projectID, Model: plan.wireModelID, Request: request}, nil
}

// ccaSystemPrompts is normalizeSystemPrompts(context.systemPrompt): the current instruction text with its
// sections, split into parts.
func ccaSystemPrompts(messages []Message) []string {
	prompt := GetCurrentSystemPrompt(messages[:min(1, len(messages))])
	if prompt == "" {
		return nil
	}
	if parts := splitSystemPromptParts(prompt); len(parts) > 0 {
		return parts
	}
	return []string{prompt}
}

// splitSystemPromptParts keeps the instruction as one part: Pi's normalizeSystemPrompts returns the
// sections joined by two newlines and the transport sends that text as one part.
func splitSystemPromptParts(prompt string) []string {
	return []string{sanitizeSurrogates(prompt)}
}

// ccaGenerationConfigFor builds generationConfig from the request options.
func ccaGenerationConfigFor(model *Model, options StreamOptions, ccaOptions *GoogleCloudCodeOptions) (*ccaGenerationConfig, error) {
	generation := &ccaGenerationConfig{}
	empty := true
	if options.TemperatureSet || options.Temperature != 0 {
		temperature := options.Temperature
		generation.Temperature = &temperature
		empty = false
	}
	if options.MaxTokens > 0 {
		maxTokens := options.MaxTokens
		generation.MaxOutputTokens = &maxTokens
		empty = false
	}
	if sampling := options.SamplingParams; sampling != nil {
		if value, ok := jsonNumberValue(sampling["topP"]); ok {
			generation.TopP = &value
			empty = false
		}
		if value, ok := jsonNumberValue(sampling["topK"]); ok && value == float64(int(value)) {
			topK := int(value)
			generation.TopK = &topK
			empty = false
		}
		if value, ok := jsonNumberValue(sampling["presencePenalty"]); ok {
			generation.PresencePenalty = &value
			empty = false
		}
	}
	thinking, err := ccaThinkingConfigFor(model, options, ccaOptions)
	if err != nil {
		return nil, err
	}
	if thinking != nil {
		generation.ThinkingConfig = thinking
		empty = false
	}
	if empty {
		return nil, nil
	}
	return generation, nil
}

// jsonNumberValue reads a sampling value as a JavaScript number.
func jsonNumberValue(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	}
	return 0, false
}

// ccaThinkingConfigFor maps the request's reasoning options onto Google's thinking configuration. Omitted
// reasoning sends nothing here: Cloud Code Assist re-applies the model's own server default.
func ccaThinkingConfigFor(model *Model, options StreamOptions, ccaOptions *GoogleCloudCodeOptions) (*geminiThinkingConfig, error) {
	reasoning := model.ProviderMeta.Reasoning || options.IsReasoning
	if !reasoning {
		return nil, nil
	}
	hideSummary := ccaOptions != nil && ccaOptions.HideThinkingSummary
	if raw := options.GoogleThinking; raw != nil {
		if raw.Enabled {
			thinking := &geminiThinkingConfig{IncludeThoughts: new(!hideSummary)}
			if raw.Level != "" {
				thinking.ThinkingLevel = string(raw.Level)
			} else if raw.BudgetTokens != nil {
				thinking.ThinkingBudget = raw.BudgetTokens
			}
			return thinking, nil
		}
		// Explicit off: omitting thinkingConfig re-applies the per-id baked server default, so the request
		// sends includeThoughts false plus the strongest suppression the model allows.
		disabled, err := getDisabledGoogleThinkingConfig(model)
		if err != nil {
			return nil, err
		}
		if disabled == nil {
			return nil, nil
		}
		disabled.IncludeThoughts = new(false)
		return disabled, nil
	}
	if options.Thinking == "" {
		return nil, nil
	}
	// Explicit simple reasoning: a Google level or the model's token budget.
	return buildGeminiThinkingConfig(model, ModelThinkingLevel(options.Thinking), reasoning, options.ThinkingBudgets)
}

// ccaApplyToolChoice maps the provider-neutral tool choice onto toolConfig. Cloud Code Assist drops
// toolConfig on Antigravity's Gemini routes, so a forced choice is restated in the transcript there.
func ccaApplyToolChoice(request *ccaRequestEnvelope, contents []ccaContent, choice any, class ccaIdentityClass, isAntigravity bool) []ccaContent {
	if choice == nil {
		return contents
	}
	if named, ok := toolChoiceName(choice); ok {
		mode := mapToolChoice(named)
		if mode == "AUTO" {
			return contents
		}
		request.ToolConfig = &ccaToolConfig{FunctionCallingConfig: ccaFunctionCallingConfig{Mode: mode}}
		return contents
	}
	if forced, ok := choice.(map[string]any); ok {
		names := []string{}
		if list, ok := forced["allowedFunctionNames"].([]any); ok {
			for _, name := range list {
				if text, ok := name.(string); ok {
					names = append(names, text)
				}
			}
		}
		if list, ok := forced["allowedFunctionNames"].([]string); ok {
			names = append(names, list...)
		}
		request.ToolConfig = &ccaToolConfig{FunctionCallingConfig: ccaFunctionCallingConfig{Mode: "ANY", AllowedFunctionNames: names}}
		if isAntigravity && class != ccaIdentityAnthropic {
			contents = append(contents, ccaContent{Role: "user", Parts: []ccaPart{{Text: ptrString(ccaForcedToolDirective)}}})
		}
	}
	return contents
}

// ccaConvertTools builds the function declarations. Claude through Cloud Code Assist needs the legacy
// `parameters` field, which the API translates into Anthropic's input_schema, and every Antigravity row
// normalizes its schema for Cloud Code Assist; Gemini CLI's Gemini rows keep `parametersJsonSchema`.
func ccaConvertTools(tools []ToolSchema, class ccaIdentityClass, isAntigravity bool) ([]ccaFunctionDeclaration, error) {
	declarations := make([]ccaFunctionDeclaration, len(tools))
	legacyParameters := class == ccaIdentityAnthropic || isAntigravity
	for i, tool := range tools {
		parameters, err := getJSONSchemaToolParameters(tool, nil)
		if err != nil {
			return nil, err
		}
		schema, order := parameters.Schema, parameters.order
		if legacyParameters {
			normalized := ccaNormalizeSchemaForCCA(schema, order)
			declarations[i] = ccaFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  normalized,
			}
			continue
		}
		rewritten, _ := rewriteConstAsEnum(schema, order)
		declarations[i] = ccaFunctionDeclaration{
			Name:                 tool.Name,
			Description:          tool.Description,
			ParametersJSONSchema: rewritten,
		}
	}
	return declarations, nil
}

// ─── Antigravity request constants ───────────────────────────────────────────

// ccaAntigravityWireProfile is the request constants one SKU carries: the model_enum label, the fixed
// output cap, and the SKU's own thinking setting.
type ccaAntigravityWireProfile struct {
	ModelEnum       string
	MaxOutputTokens int
	ThinkingLevel   string
	ThinkingBudget  *int
}

// ccaAntigravityWireProfiles is the offline fallback table of per-SKU request constants, keyed by wire id.
// Discovery owns current SKUs; this table is the last resort when the account roster says nothing.
var ccaAntigravityWireProfiles = map[string]ccaAntigravityWireProfile{
	"gemini-3.5-flash-extra-low": {ModelEnum: "MODEL_PLACEHOLDER_M187", MaxOutputTokens: 65536},
	"gemini-3.5-flash-low":       {ModelEnum: "MODEL_PLACEHOLDER_M20", MaxOutputTokens: 65536},
	"gemini-3-flash-agent":       {ModelEnum: "MODEL_PLACEHOLDER_M132", MaxOutputTokens: 65536},
	"gemini-3.1-pro-low":         {ModelEnum: "MODEL_PLACEHOLDER_M36", MaxOutputTokens: 65535},
	"gemini-pro-agent":           {ModelEnum: "MODEL_PLACEHOLDER_M16", MaxOutputTokens: 65535},
	// Claude on daily-cloudcode-pa rejects maxOutputTokens above 64000 with a 400.
	"claude-sonnet-4-6":        {MaxOutputTokens: 64000},
	"claude-opus-4-6-thinking": {MaxOutputTokens: 64000},
}

// ccaWireProfileForAccount is the wire id's request constants: the serving account's discovered profile,
// then any account's discovered profile, then the offline table. Per-account lookup keeps one account's SKU
// constants from leaking into another's requests.
func ccaWireProfileForAccount(accountKey, wireID string) ccaAntigravityWireProfile {
	if profile, ok := ccaAccountProfile(accountKey, wireID); ok {
		return profile
	}
	if profile, ok := ccaStoredWireProfileFor(wireID); ok {
		return profile
	}
	return ccaAntigravityWireProfiles[wireID]
}

// ccaApplyWireProfile applies the SKU's fixed output cap, then — for a reviewed effort-tier variant — the
// SKU's own thinking strength in place of the requested effort.
func ccaApplyWireProfile(request *ccaRequestEnvelope, options StreamOptions, profile ccaAntigravityWireProfile, reviewed bool) {
	if profile.MaxOutputTokens > 0 {
		if request.GenerationConfig == nil {
			request.GenerationConfig = &ccaGenerationConfig{}
		}
		maxTokens := profile.MaxOutputTokens
		request.GenerationConfig.MaxOutputTokens = &maxTokens
	}
	thinkingRequested := options.Thinking != "" || (options.GoogleThinking != nil && options.GoogleThinking.Enabled)
	if !thinkingRequested {
		return
	}
	if request.GenerationConfig == nil {
		return
	}
	request.GenerationConfig.ThinkingConfig = ccaReviewedTierThinking(request.GenerationConfig.ThinkingConfig, profile, reviewed)
}

// ─── Antigravity envelope ────────────────────────────────────────────────────

// ccaAntigravityEnvelope is one request's identity on Antigravity's ingress.
type ccaAntigravityEnvelope struct {
	sessionID string
	requestID string
	labels    *ccaLabels
}

// ccaAntigravitySession is the per-conversation envelope identity the native client keeps: UUIDs, a
// signed-decimal session id and a step counter. A stream keeps one for its request; the values are the
// ones the client would send for a fresh trajectory.
type ccaAntigravitySession struct {
	AgentID         string
	TrajectoryID    string
	SessionID       string
	StepIndex       int
	RequestCount    int
	LastExecutionID string
}

// ccaAntigravitySessionKey is the request-context key of a conversation's envelope identity.
type ccaAntigravitySessionKey struct{}

// ccaSessionOf reads the conversation identity: the request options' own state, else the one carried by the
// request context.
func ccaSessionOf(ctx context.Context, ccaOptions *GoogleCloudCodeOptions) *CCAAntigravitySessionState {
	if ccaOptions != nil && ccaOptions.Session != nil {
		return ccaOptions.Session
	}
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(ccaAntigravitySessionKey{}).(*CCAAntigravitySessionState)
	return state
}

// WithCCAAntigravitySession binds a conversation's Antigravity envelope identity to ctx, so consecutive
// requests of one trajectory advance the same step counter and echo the prior response id.
func WithCCAAntigravitySession(ctx context.Context, state *CCAAntigravitySessionState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, ccaAntigravitySessionKey{}, state)
}

// ccaBuildAntigravityEnvelope advances the session state and builds the request identity. The step never
// moves backwards, so a compacted or resent context still advances it.
func ccaBuildAntigravityEnvelope(ctx context.Context, model *Model, transcript TranscriptContext, class ccaIdentityClass, profile ccaAntigravityWireProfile, ccaOptions *GoogleCloudCodeOptions) ccaAntigravityEnvelope {
	messages := collapsedMessages(transcript)
	contextSteps := max(1, len(messages))
	requestIndex := 0
	state := ccaSessionOf(ctx, ccaOptions)
	if state != nil {
		if state.AgentID == "" {
			state.AgentID = ccaRandomUUID()
		}
		if state.TrajectoryID == "" {
			state.TrajectoryID = ccaRandomUUID()
		}
		if state.SessionID == "" {
			state.SessionID = ccaRandomSignedDecimalSessionID()
		}
		state.StepIndex = max(contextSteps, state.StepIndex+1)
		requestIndex = state.RequestCount
		state.RequestCount = requestIndex + 1
	}
	agentID, trajectoryID, sessionID := "", "", ""
	step := contextSteps
	var lastExecutionID string
	if state != nil {
		agentID, trajectoryID, sessionID = state.AgentID, state.TrajectoryID, state.SessionID
		step = state.StepIndex
		lastExecutionID = state.LastExecutionID
	}
	if agentID == "" {
		agentID = ccaRandomUUID()
	}
	if trajectoryID == "" {
		trajectoryID = ccaRandomUUID()
	}
	if sessionID == "" {
		sessionID = ccaDeriveAntigravitySessionID(transcript)
	}
	requestID := "agent/" + agentID + "/" + strconv.FormatInt(nowMillis(), 10) + "/" + trajectoryID + "/" + strconv.Itoa(step)
	usageLabel := ccaUsageLabel(class, ccaOptions)
	labels := &ccaLabels{
		CascadeID:              agentID,
		LastExecutionID:        lastExecutionID,
		LastStepIndex:          strconv.Itoa(step - 1),
		ModelEnum:              profile.ModelEnum,
		RequestID:              trajectoryID + "-" + strconv.Itoa(requestIndex),
		RootCascadeID:          agentID,
		TrajectoryID:           trajectoryID,
		UsedClaude:             usageLabel,
		UsedClaudeConservative: usageLabel,
		UsedNonGeminiModel:     strconv.FormatBool(class != ccaIdentityGemini),
	}
	return ccaAntigravityEnvelope{sessionID: sessionID, requestID: requestID, labels: labels}
}

// ccaDeriveAntigravitySessionID derives the signed-decimal session id from the first user text, else picks
// a random one.
func ccaDeriveAntigravitySessionID(transcript TranscriptContext) string {
	if text, ok := ccaFirstUserText(transcript); ok && trimJSWhitespace(text) != "" {
		return ccaSignedDecimalFromHash(text)
	}
	return ccaRandomSignedDecimalSessionID()
}

// ccaFirstUserText is getFirstUserTextForAntigravitySession.
func ccaFirstUserText(transcript TranscriptContext) (string, bool) {
	for _, message := range collapsedMessages(transcript) {
		user, ok := message.(UserMessage)
		if !ok {
			continue
		}
		switch content := user.Content.(type) {
		case UserText:
			return string(content), true
		case UserContentBlocks:
			for _, block := range content {
				if text, ok := block.(TextContent); ok {
					return text.Text, true
				}
			}
			return "", false
		}
		return "", false
	}
	return "", false
}

const ccaInt63Mask = uint64(1<<63 - 1)

// ccaSignedDecimalFromHash folds the SHA-256 of text into a signed decimal session id.
func ccaSignedDecimalFromHash(text string) string {
	digest := sha256.Sum256([]byte(text))
	value := binary.BigEndian.Uint64(digest[:8]) & ccaInt63Mask
	return "-" + strconv.FormatUint(value, 10)
}

// ccaRandomSignedDecimalSessionID is a random signed-decimal session id below 9e18.
func ccaRandomSignedDecimalSessionID() string {
	bound := new(big.Int).SetUint64(9_000_000_000_000_000_000)
	value, err := rand.Int(rand.Reader, bound)
	if err != nil {
		return "-0"
	}
	return "-" + value.String()
}

// ccaRandomUUID is a v4-shaped random UUID: the envelope ids only have to be unique per trajectory.
func ccaRandomUUID() string {
	if uuid, err := UUIDv7(nil); err == nil {
		return uuid
	}
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

func cmpOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// ─── Message conversion ──────────────────────────────────────────────────────

// ccaConvertMessages is google-shared.ts convertMessages for Cloud Code Assist: system messages collapse
// into the request's instruction, Gemini pairs a result's images after the function-response turn, and
// id-less function responses are sorted back into their calls' order.
func ccaConvertMessages(model *Model, transcript TranscriptContext, providerID string) ([]ccaContent, error) {
	conversionModel := *model
	conversionModel.ProviderMeta.ProviderID = providerID
	conversionModel.ProviderMeta.API = APIGoogleGeminiCLI
	messages := TransformMessages(collapsedMessages(transcript), &conversionModel, func(id string, _ *Model, _ AssistantMessage) string {
		return normalizeToolCallId(model.ID, id)
	})
	supportsImages := slicesContainsModelInput(model.Input, ModelInputImage)
	multimodal := ccaSupportsMultimodalFunctionResponse(model)
	class := ccaIdentityForModelID(model.ID)
	includeID := ccaSupportsFunctionPartID(class)
	requiresSkip := ccaRequiresFirstCallSignature(class, model.ID)

	contents := []ccaContent{}
	// responseOrdinals parallels contents: per content, each part's call ordinal (-1 for unmatched).
	responseOrdinals := [][]int{}
	emittedToolNames := map[string]string{}
	callOrder := map[string]int{}
	nextCall := 0
	var pendingToolImages []ccaPendingImage

	flushPendingImages := func() {
		if len(pendingToolImages) == 0 {
			return
		}
		sortCCAImages(pendingToolImages)
		parts := []ccaPart{}
		for _, group := range pendingToolImages {
			parts = append(parts, group.parts...)
		}
		contents = append(contents, ccaContent{Role: "user", Parts: parts})
		responseOrdinals = append(responseOrdinals, make([]int, len(parts)))
		pendingToolImages = nil
	}

	for index := 0; index < len(messages); index++ {
		message := messages[index]
		if _, isResult := message.(ToolResultMessage); !isResult {
			flushPendingImages()
		}
		switch message := message.(type) {
		case UserMessage:
			parts := ccaUserParts(message.Content, supportsImages)
			if len(parts) == 0 {
				continue
			}
			contents = append(contents, ccaContent{Role: "user", Parts: parts})
			responseOrdinals = append(responseOrdinals, make([]int, len(parts)))
		case AssistantMessage:
			sameProviderAndModel := message.Provider == providerID && message.Model == model.ID
			parts, err := ccaAssistantParts(message, model, sameProviderAndModel, includeID, requiresSkip, emittedToolNames, callOrder, &nextCall)
			if err != nil {
				return nil, err
			}
			if len(parts) == 0 {
				continue
			}
			contents = append(contents, ccaContent{Role: "model", Parts: parts})
			responseOrdinals = append(responseOrdinals, make([]int, len(parts)))
		case ToolResultMessage:
			for index < len(messages) {
				result, ok := messages[index].(ToolResultMessage)
				if !ok {
					break
				}
				textParts := []string{}
				imageParts := []ccaPart{}
				for _, block := range result.Content {
					switch block := block.(type) {
					case TextContent:
						textParts = append(textParts, block.Text)
					case ImageContent:
						if supportsImages {
							imageParts = append(imageParts, ccaPart{InlineData: &geminiInlineData{MimeType: block.MimeType, Data: block.Data}})
						}
					}
				}
				text := strings.Join(textParts, "\n")
				if trimJSWhitespace(text) == "" {
					text = ""
					if len(imageParts) > 0 {
						text = "(see attached image)"
					}
				}
				omittedImages := false
				if !supportsImages {
					for _, block := range result.Content {
						if _, ok := block.(ImageContent); ok {
							omittedImages = true
							break
						}
					}
				}
				if omittedImages {
					if text != "" {
						text = text + "\n" + googleNonVisionImagePlaceholder
					} else {
						text = googleNonVisionImagePlaceholder
					}
				}
				text = sanitizeSurrogates(text)
				response := map[string]any{"output": text}
				if result.IsError {
					response = map[string]any{"error": text}
				}
				reply := &ccaFunctionReply{Name: cmpOr(emittedToolNames[result.ToolCallID], result.ToolName), Response: response}
				if len(imageParts) > 0 && multimodal {
					reply.Parts = imageParts
				}
				if includeID {
					reply.ID = result.ToolCallID
				}
				part := ccaPart{FunctionResponse: reply}
				ordinal, matched := callOrder[result.ToolCallID]
				if last := len(contents) - 1; last >= 0 && contents[last].Role == "user" && len(contents[last].Parts) > 0 && contents[last].Parts[0].FunctionResponse != nil {
					contents[last].Parts = append(contents[last].Parts, part)
					responseOrdinals[last] = append(responseOrdinals[last], ordinalFor(matched, ordinal))
				} else {
					contents = append(contents, ccaContent{Role: "user", Parts: []ccaPart{part}})
					responseOrdinals = append(responseOrdinals, []int{ordinalFor(matched, ordinal)})
				}
				if len(imageParts) > 0 && !multimodal {
					pendingToolImages = append(pendingToolImages, ccaPendingImage{ordinal: ordinal, hasOrd: matched, parts: append([]ccaPart{{Text: ptrString("Tool result image:")}}, imageParts...)})
				}
				index++
			}
			index--
		}
	}
	flushPendingImages()
	ccaSortResponsesIntoCallOrder(contents, responseOrdinals)
	return contents, nil
}

// ccaPendingImage is a Gemini 2.x tool result's image turn waiting for its function-response turn.
type ccaPendingImage struct {
	ordinal int
	hasOrd  bool
	parts   []ccaPart
}

func sortCCAImages(images []ccaPendingImage) {
	for i := 1; i < len(images); i++ {
		for j := i; j > 0 && images[j].hasOrd && images[j-1].hasOrd && images[j].ordinal < images[j-1].ordinal; j-- {
			images[j], images[j-1] = images[j-1], images[j]
		}
	}
}

// ordinalFor is a call ordinal, or -1 when the result matched no recorded call.
func ordinalFor(matched bool, ordinal int) int {
	if !matched {
		return -1
	}
	return ordinal
}

// ccaSortResponsesIntoCallOrder sorts each turn's matched function responses back into their calls' order
// within the slots they already occupy, so id-less responses pair with their calls by position.
func ccaSortResponsesIntoCallOrder(contents []ccaContent, ordinals [][]int) {
	for index := range contents {
		content, order := contents[index], ordinals[index]
		if len(content.Parts) < 2 || len(order) != len(content.Parts) {
			continue
		}
		type keyed struct {
			position int
			ordinal  int
			part     ccaPart
		}
		known := make([]keyed, 0, len(order))
		for position, ordinal := range order {
			if ordinal >= 0 {
				known = append(known, keyed{position: position, ordinal: ordinal, part: content.Parts[position]})
			}
		}
		if len(known) < 2 {
			continue
		}
		ordered := slices.Clone(known)
		for i := 1; i < len(ordered); i++ {
			for j := i; j > 0 && ordered[j].ordinal < ordered[j-1].ordinal; j-- {
				ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
			}
		}
		for i, slot := range known {
			content.Parts[slot.position] = ordered[i].part
		}
	}
}

func ccaUserParts(content UserContent, supportsImages bool) []ccaPart {
	parts := []ccaPart{}
	omittedImages := false
	switch content := content.(type) {
	case UserText:
		text := string(content)
		if trimJSWhitespace(text) == "" {
			return nil
		}
		return []ccaPart{{Text: ptrString(sanitizeSurrogates(text))}}
	case UserContentBlocks:
		for _, block := range content {
			switch block := block.(type) {
			case TextContent:
				text := sanitizeSurrogates(block.Text)
				if trimJSWhitespace(text) == "" {
					continue
				}
				parts = append(parts, ccaPart{Text: ptrString(text)})
			case ImageContent:
				if supportsImages {
					parts = append(parts, ccaPart{InlineData: &geminiInlineData{MimeType: block.MimeType, Data: block.Data}})
				} else {
					omittedImages = true
				}
			}
		}
	}
	if omittedImages {
		parts = append(parts, ccaPart{Text: ptrString(googleNonVisionImagePlaceholder)})
	}
	return parts
}

// googleNonVisionImagePlaceholder is google-shared.ts NON_VISION_IMAGE_PLACEHOLDER.
const googleNonVisionImagePlaceholder = "[image omitted: model does not support vision]"

// ccaSkipThoughtSignature is the bypass sentinel a Gemini function call carries when it has no signature
// (google-shared.ts SKIP_THOUGHT_SIGNATURE).
const ccaSkipThoughtSignature = "skip_thought_signature_validator"

// ccaAssistantParts builds one assistant turn's parts.
func ccaAssistantParts(message AssistantMessage, model *Model, sameProviderAndModel, includeID, requiresSkip bool, emittedToolNames map[string]string, callOrder map[string]int, nextCall *int) ([]ccaPart, error) {
	parts := []ccaPart{}
	dropsUnsigned := ccaIdentityForModelID(model.ID) == ccaIdentityAnthropic
	isFirstToolCall := true
	for _, block := range message.Content {
		switch block := block.(type) {
		case TextContent:
			if trimJSWhitespace(block.Text) == "" {
				continue
			}
			signature := ""
			if sameProviderAndModel && isValidThoughtSignature(block.TextSignature) {
				signature = block.TextSignature
			}
			parts = append(parts, ccaPart{Text: ptrString(sanitizeSurrogates(block.Text)), ThoughtSignature: signature})
		case ThinkingContent:
			signature := ""
			if sameProviderAndModel && isValidThoughtSignature(block.ThinkingSignature) {
				signature = block.ThinkingSignature
			}
			if trimJSWhitespace(block.Thinking) == "" && signature == "" {
				continue
			}
			if dropsUnsigned && signature == "" {
				continue
			}
			if sameProviderAndModel {
				thought := true
				parts = append(parts, ccaPart{Text: ptrString(sanitizeSurrogates(block.Thinking)), Thought: &thought, ThoughtSignature: signature})
			} else if trimJSWhitespace(block.Thinking) != "" {
				parts = append(parts, ccaPart{Text: ptrString(sanitizeSurrogates(renderCCADemotedThinking(model.ID, block.Thinking)))})
			}
		case ToolCall:
			emittedToolNames[block.ID] = block.Name
			callOrder[block.ID] = *nextCall
			*nextCall = *nextCall + 1
			signature := ""
			if sameProviderAndModel && isValidThoughtSignature(block.ThoughtSignature) {
				signature = block.ThoughtSignature
			}
			requiresFallback := requiresSkip && isFirstToolCall
			effectiveSignature := signature
			if effectiveSignature == "" && requiresFallback {
				effectiveSignature = ccaSkipThoughtSignature
			}
			isFirstToolCall = false
			call := &ccaFunctionCall{Name: block.Name, Args: block.argumentsJSONOrEmpty()}
			if includeID {
				call.ID = block.ID
			}
			parts = append(parts, ccaPart{FunctionCall: call, ThoughtSignature: effectiveSignature})
		}
	}
	return parts, nil
}

// renderCCADemotedThinking replays another model's reasoning as visible text: Gemini's dialect fence for a
// Gemini target, bare prose for Claude (Anthropic refuses wrapped reasoning in history).
func renderCCADemotedThinking(modelID, text string) string {
	if text == "" {
		return ""
	}
	if ccaIdentityForModelID(modelID) == ccaIdentityAnthropic {
		return text
	}
	return "```thinking\n" + text + "\n```"
}

// ccaSupportsMultimodalFunctionResponse reports a target that accepts images nested inside a
// functionResponse part: Gemini 3 and later.
func ccaSupportsMultimodalFunctionResponse(model *Model) bool {
	if ccaIdentityForModelID(model.ID) != ccaIdentityGemini {
		return false
	}
	return supportsMultimodalFunctionResponse(model.ID)
}

func slicesContainsModelInput(input []ModelInputModality, want ModelInputModality) bool {
	for _, value := range input {
		if value == want {
			return true
		}
	}
	return false
}

// ─── Schema normalization for Cloud Code Assist ──────────────────────────────

// ccaUnsupportedSchemaFields are the JSON Schema keywords the Cloud Code Assist wire has no field for
// (google/pi-ai utils/schema/fields.ts UNSUPPORTED_SCHEMA_FIELDS).
var ccaUnsupportedSchemaFields = map[string]struct{}{
	"$schema": {}, "$id": {}, "$anchor": {}, "$ref": {}, "$defs": {}, "$dynamicRef": {}, "$dynamicAnchor": {},
	"examples": {}, "prefixItems": {}, "unevaluatedProperties": {}, "unevaluatedItems": {}, "patternProperties": {},
	"additionalProperties": {}, "propertyNames": {}, "minItems": {}, "maxItems": {}, "uniqueItems": {},
	"contains": {}, "minContains": {}, "maxContains": {}, "additionalItems": {}, "minLength": {}, "maxLength": {},
	"minimum": {}, "maximum": {}, "exclusiveMinimum": {}, "exclusiveMaximum": {}, "multipleOf": {}, "pattern": {},
	"format": {}, "contentEncoding": {}, "contentMediaType": {}, "contentSchema": {}, "dependencies": {},
	"dependentSchemas": {}, "dependentRequired": {}, "x-mcp-header": {}, "deprecated": {}, "readOnly": {},
	"writeOnly": {}, "$comment": {},
}

// ccaLiftableSchemaFields are the stripped keywords whose value is preserved in the sibling description.
var ccaLiftableSchemaFields = map[string]struct{}{
	"pattern": {}, "format": {}, "minLength": {}, "maxLength": {}, "minimum": {}, "maximum": {},
	"exclusiveMinimum": {}, "exclusiveMaximum": {}, "multipleOf": {}, "minItems": {}, "maxItems": {},
	"uniqueItems": {}, "minProperties": {}, "maxProperties": {}, "default": {}, "examples": {},
}

// ccaSchemaFieldRenames maps the snake_case spellings google's protojson accepts back to JSON Schema names.
var ccaSchemaFieldRenames = map[string]string{
	"additional_properties": "additionalProperties",
	"any_of":                "anyOf",
	"prefix_items":          "prefixItems",
	"property_ordering":     "propertyOrdering",
}

// ccaCCAFallbackSchema is the per-tool fallback for a schema Cloud Code Assist cannot express: an open
// object, so one bad tool never fails the whole request.
var ccaCCAFallbackSchema = map[string]any{"type": "object", "properties": map[string]any{}}

// ccaNormalizeSchemaForCCA ports normalizeSchemaForCCA: Google keyword stripping with description spill,
// snake_case field renaming, scalarized type arrays, nullable removal, object-combiner merging, residual
// combiner stripping, and the object fallback when anything incompatible remains.
func ccaNormalizeSchemaForCCA(schema map[string]any, order schemaObjectOrder) any {
	if schema == nil {
		return ccaCCAFallbackSchema
	}
	normalized, ok := ccaNormalizeSchemaNode(schema, true).(map[string]any)
	if !ok || normalized == nil {
		return ccaCCAFallbackSchema
	}
	if _, ensured := normalized["type"]; !ensured {
		normalized["type"] = "object"
	}
	if normalized["type"] == "object" {
		if _, ok := normalized["properties"].(map[string]any); !ok {
			normalized["properties"] = map[string]any{}
		}
	}
	if ccaHasResidualIncompatibility(normalized) {
		return ccaCCAFallbackSchema
	}
	return normalized
}

// ccaHasResidualIncompatibility is the hard stop of the CCA contract: a type array, a null type, a
// nullable keyword, or a residual combiner.
func ccaHasResidualIncompatibility(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, member := range value {
			switch key {
			case "type":
				if _, isArray := member.([]any); isArray {
					return true
				}
				if text, ok := member.(string); ok && text == "null" {
					return true
				}
			case "nullable":
				return true
			case "anyOf", "oneOf", "allOf", "not":
				return true
			}
			if ccaHasResidualIncompatibility(member) {
				return true
			}
		}
	case []any:
		for _, member := range value {
			if ccaHasResidualIncompatibility(member) {
				return true
			}
		}
	}
	return false
}

// ccaNormalizeSchemaNode normalizes one schema node. isRoot marks the tool's parameter schema, whose type
// must be object.
func ccaNormalizeSchemaNode(value any, isRoot bool) any {
	switch value := value.(type) {
	case bool:
		// A boolean subschema has no object form the CCA wire accepts: `false` becomes the denied form the
		// normalizer's standard coercion uses, `true` the empty schema.
		if value {
			return map[string]any{"type": "object", "properties": map[string]any{}}
		}
		return map[string]any{"type": "object", "properties": map[string]any{}, "not": map[string]any{}}
	case []any:
		out := make([]any, len(value))
		for i, member := range value {
			out[i] = ccaNormalizeSchemaNode(member, false)
		}
		return out
	case map[string]any:
		return ccaNormalizeSchemaObject(value, isRoot)
	default:
		return value
	}
}

// ccaNormalizeSchemaObject normalizes one object node.
func ccaNormalizeSchemaObject(node map[string]any, isRoot bool) any {
	renamed := make(map[string]any, len(node))
	keys := make([]string, 0, len(node))
	for key, value := range node {
		name := key
		if replacement, ok := ccaSchemaFieldRenames[key]; ok {
			name = replacement
		}
		if _, exists := renamed[name]; !exists {
			keys = append(keys, name)
		}
		renamed[name] = value
	}
	out := make(map[string]any, len(renamed))
	description := ""
	if value, ok := renamed["description"].(string); ok {
		description = value
	}
	spilled := []string{}
	for _, key := range keys {
		value := renamed[key]
		switch key {
		case "description":
			continue
		case "$schema", "$id", "$anchor", "$ref", "$defs", "$dynamicRef", "$dynamicAnchor", "$comment",
			"prefixItems", "unevaluatedProperties", "unevaluatedItems", "patternProperties", "propertyNames",
			"contains", "minContains", "maxContains", "additionalItems", "contentEncoding", "contentMediaType",
			"contentSchema", "dependencies", "dependentSchemas", "dependentRequired", "x-mcp-header",
			"deprecated", "readOnly", "writeOnly", "nullable":
			if _, liftable := ccaLiftableSchemaFields[key]; liftable {
				spilled = append(spilled, key+": "+ccaJSONForDescription(value))
			}
			continue
		case "additionalProperties":
			// CCA has no additionalProperties field; the schema shape (properties and required) carries the
			// contract instead.
			continue
		case "anyOf", "oneOf":
			merged, ok := ccaMergeObjectCombiners(value)
			if ok {
				child := ccaNormalizeSchemaNode(merged, isRoot).(map[string]any)
				for childKey, childValue := range child {
					out[childKey] = childValue
				}
				continue
			}
			// A mixed or same-type union collapses to its first representable branch, which is what the
			// normalizer's mixed-type collapse leaves behind after the residual combiners are stripped.
			if branch, ok := ccaFirstCombinableBranch(value); ok {
				child := ccaNormalizeSchemaNode(branch, isRoot).(map[string]any)
				for childKey, childValue := range child {
					out[childKey] = childValue
				}
			}
			continue
		case "allOf":
			branch, ok := ccaFirstCombinableBranch(value)
			if !ok {
				continue
			}
			child := ccaNormalizeSchemaNode(branch, isRoot).(map[string]any)
			for childKey, childValue := range child {
				out[childKey] = childValue
			}
			continue
		case "not":
			continue
		case "type":
			ccaApplySchemaType(out, value)
			continue
		case "properties":
			properties := map[string]any{}
			if members, ok := value.(map[string]any); ok {
				for name, member := range members {
					properties[name] = ccaNormalizeSchemaNode(member, false)
				}
			}
			out["properties"] = properties
			continue
		case "items":
			out["items"] = ccaNormalizeSchemaNode(value, false)
			continue
		case "enum":
			out["enum"] = value
			if _, hasType := out["type"]; !hasType {
				if inferred := ccaInferEnumType(value); inferred != "" {
					out["type"] = inferred
				}
			}
			continue
		case "required":
			if list, ok := value.([]any); ok {
				kept := make([]any, 0, len(list))
				for _, member := range list {
					if text, ok := member.(string); ok {
						kept = append(kept, text)
					}
				}
				if len(kept) > 0 {
					out["required"] = kept
				}
			}
			continue
		case "minItems", "maxItems", "uniqueItems", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
			"multipleOf", "pattern", "format", "minLength", "maxLength", "default", "examples",
			"minProperties", "maxProperties":
			spilled = append(spilled, key+": "+ccaJSONForDescription(value))
			continue
		}
		out[key] = ccaNormalizeSchemaNode(value, false)
	}
	if description != "" {
		out["description"] = description
	}
	if len(spilled) > 0 {
		block := "{" + strings.Join(spilled, ", ") + "}"
		if description != "" {
			out["description"] = description + "\n\n" + block
		} else {
			out["description"] = block
		}
	}
	if _, ok := out["type"]; !ok && isRoot {
		out["type"] = "object"
	}
	if _, ok := out["type"]; !ok {
		if _, hasProperties := out["properties"]; hasProperties {
			out["type"] = "object"
		}
	}
	if out["type"] == "object" {
		if _, ok := out["properties"].(map[string]any); !ok {
			out["properties"] = map[string]any{}
		}
		// A nullable property is not required: CCA has no null representation.
		if required, ok := out["required"].([]any); ok {
			kept := make([]any, 0, len(required))
			for _, member := range required {
				name, _ := member.(string)
				if properties, ok := out["properties"].(map[string]any); ok && name != "" {
					if _, present := properties[name]; !present {
						continue
					}
				}
				kept = append(kept, member)
			}
			if len(kept) > 0 {
				out["required"] = kept
			} else {
				delete(out, "required")
			}
		}
	}
	return out
}

// ccaApplySchemaType writes a scalar type, dropping `null` and collapsing a multi-type array to its first
// scalar member.
func ccaApplySchemaType(out map[string]any, value any) {
	switch value := value.(type) {
	case string:
		if value == "null" {
			return
		}
		out["type"] = value
	case []any:
		for _, member := range value {
			if text, ok := member.(string); ok && text != "null" {
				out["type"] = text
				return
			}
		}
	}
}

// ccaMergeObjectCombiners merges an object-only anyOf/oneOf union into one object shape: branch properties
// merge, and a field stays required only when every branch requires it.
func ccaMergeObjectCombiners(value any) (map[string]any, bool) {
	branches, ok := value.([]any)
	if !ok || len(branches) == 0 {
		return nil, false
	}
	merged := map[string]any{"type": "object", "properties": map[string]any{}}
	properties := merged["properties"].(map[string]any)
	branchRequired := make([]map[string]bool, 0, len(branches))
	for _, branch := range branches {
		member, ok := branch.(map[string]any)
		if !ok {
			return nil, false
		}
		if kind, _ := member["type"].(string); kind != "" && kind != "object" {
			return nil, false
		}
		branchProperties, _ := member["properties"].(map[string]any)
		for name, property := range branchProperties {
			properties[name] = property
		}
		required := map[string]bool{}
		if list, ok := member["required"].([]any); ok {
			for _, member := range list {
				if name, ok := member.(string); ok {
					required[name] = true
				}
			}
		}
		branchRequired = append(branchRequired, required)
	}
	required := []string{}
	for name := range branchRequired[0] {
		everyBranch := true
		for _, set := range branchRequired[1:] {
			if !set[name] {
				everyBranch = false
				break
			}
		}
		if everyBranch {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		slices.Sort(required)
		list := make([]any, len(required))
		for i, name := range required {
			list[i] = name
		}
		merged["required"] = list
	}
	// The node's own non-structural keywords survive the merge.
	for key, member := range branches[0].(map[string]any) {
		switch key {
		case "type", "properties", "required":
			continue
		}
		merged[key] = member
	}
	return merged, true
}

// ccaFirstCombinableBranch picks the first branch a scalar representation can take.
func ccaFirstCombinableBranch(value any) (any, bool) {
	branches, ok := value.([]any)
	if !ok {
		return nil, false
	}
	for _, branch := range branches {
		if member, ok := branch.(map[string]any); ok {
			if kind, _ := member["type"].(string); kind == "null" {
				continue
			}
			return member, true
		}
	}
	return nil, false
}

// ccaInferEnumType infers the scalar type of a bare enum.
func ccaInferEnumType(value any) string {
	list, ok := value.([]any)
	if !ok {
		return ""
	}
	for _, member := range list {
		switch member.(type) {
		case string:
			return "string"
		case float64, int, int64:
			return "number"
		case bool:
			return "boolean"
		}
	}
	return ""
}

// ccaJSONForDescription formats a lifted keyword value the way the spill format writes it.
func ccaJSONForDescription(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(fmt.Sprint(value))
	}
	return string(encoded)
}
