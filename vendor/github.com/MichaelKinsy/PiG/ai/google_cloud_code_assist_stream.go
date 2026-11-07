package ai

// Ports the stream body of @oh-my-pi/pi-ai src/providers/google-gemini-cli.ts (streamGoogleGeminiCli): the
// attempt/failover loop, the SSE chunk folding, the empty-response re-requests and the terminal event of the
// Cloud Code Assist transport.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── Response wire types ─────────────────────────────────────────────────────

// ccaResponseChunk is one SSE JSON event: the response payload, an in-band error, or a trace id.
type ccaResponseChunk struct {
	Response *ccaResponsePayload `json:"response,omitempty"`
	Error    *ccaStreamError     `json:"error,omitempty"`
	TraceID  string              `json:"traceId,omitempty"`
}

// ccaResponsePayload is the chunk's response object.
type ccaResponsePayload struct {
	Candidates     []ccaCandidate     `json:"candidates,omitempty"`
	UsageMetadata  *ccaUsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion   string             `json:"modelVersion,omitempty"`
	ResponseID     string             `json:"responseId,omitempty"`
	PromptFeedback *ccaPromptFeedback `json:"promptFeedback,omitempty"`
}

// ccaCandidate is one generated candidate.
type ccaCandidate struct {
	Content      *ccaCandidateContent `json:"content,omitempty"`
	FinishReason string               `json:"finishReason,omitempty"`
}

// ccaCandidateContent is a candidate's content.
type ccaCandidateContent struct {
	Role  string            `json:"role,omitempty"`
	Parts []ccaResponsePart `json:"parts,omitempty"`
}

// ccaResponsePart is one response part. Text is raw so a non-string value can be told from an absent one.
type ccaResponsePart struct {
	Text             json.RawMessage   `json:"text,omitempty"`
	Thought          *bool             `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *ccaResponseCall  `json:"functionCall,omitempty"`
	InlineData       *geminiInlineData `json:"inlineData,omitempty"`
}

// ccaResponseCall is a response function call.
type ccaResponseCall struct {
	Name json.RawMessage `json:"name,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
	ID   string          `json:"id,omitempty"`
}

// ccaStreamError is a runtime failure delivered as a final JSON event.
type ccaStreamError struct {
	Code    *int            `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Status  string          `json:"status,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

// ccaUsageMetadata is the chunk's token accounting.
type ccaUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

// ccaPromptFeedback is the request-level block explanation.
type ccaPromptFeedback struct {
	BlockReason        string `json:"blockReason,omitempty"`
	BlockReasonMessage string `json:"blockReasonMessage,omitempty"`
}

// ccaPartText reads a part's text and whether the part carried a text member at all. A non-string value is
// delivered as its JavaScript string form rather than dropped.
func ccaPartText(part ccaResponsePart) (string, bool) {
	if len(part.Text) == 0 || string(part.Text) == "null" {
		return "", false
	}
	var text string
	if json.Unmarshal(part.Text, &text) == nil {
		return text, true
	}
	return strings.TrimSpace(string(part.Text)), true
}

// ccaPartIsThinking is isThinkingPart: `thought: true`.
func ccaPartIsThinking(part ccaResponsePart) bool { return part.Thought != nil && *part.Thought }

// ccaFunctionCallName is `part.functionCall.name || ""`.
func ccaFunctionCallName(call *ccaResponseCall) string {
	if call == nil || len(call.Name) == 0 || string(call.Name) == "null" {
		return ""
	}
	var decoded string
	if json.Unmarshal(call.Name, &decoded) == nil {
		return decoded
	}
	return ""
}

// ccaNextToolCallID generates a tool call id: name_<epoch ms>_<counter>.
func ccaNextToolCallID(name string) string {
	//portlint:allow clock Pi's Google adapters build the id with Date.now() (google-generative-ai.ts:200).
	return name + "_" + fmt.Sprintf("%d", nowMillis()) + "_" + fmt.Sprintf("%d", ccaStreamToolCounter.Add(1))
}

// ─── Attempt loop ────────────────────────────────────────────────────────────

// run drives the endpoints until one attempt produces a complete response, then finishes the stream.
func (state *ccaStream) run() {
	defer state.builder.stream.End()
	if err := state.runAttempts(); err != nil {
		// Pi's catch block pushes the error onto the message as it stands: open blocks keep their
		// streamed state and get no synthesized end event.
		reason := StopReasonError
		if state.ctx.Err() != nil {
			reason = StopReasonAborted
		}
		state.builder.failUnfinished(reason, err)
	}
}

// runAttempts is the endpoint failover loop. Once any stream event has started the endpoint is committed
// downstream, so a later failure surfaces instead of failing over.
func (state *ccaStream) runAttempts() error {
	var last error
	for index, endpoint := range state.endpoints {
		isLast := index == len(state.endpoints)-1
		state.resetAttempt()
		err := state.attempt(endpoint, isLast)
		if err == nil {
			return nil
		}
		last = err
		status := ccaStatusOf(err)
		if !isLast && !state.builder.started {
			var emptyOutput *ccaEmptyOutputError
			if ccaTransientStatus(status) || (status == 0 && !errors.As(err, &emptyOutput)) {
				continue
			}
		}
		return err
	}
	if last == nil {
		last = errors.New("Cloud Code Assist request failed")
	}
	return last
}

// attempt sends one endpoint's request and re-requests an empty stream up to the retry bound.
func (state *ccaStream) attempt(endpoint string, isLast bool) error {
	requestURL, err := ccaRequestURL(endpoint)
	if err != nil {
		return err
	}
	response, err := state.send(requestURL, isLast)
	if err != nil {
		return err
	}
	if err := observeProviderResponse(state.ctx, state.options, response, state.model); err != nil {
		_ = response.Body.Close()
		return err
	}
	if consumeErr := state.consumeStream(response, state.firstEventTimeout()); consumeErr != nil {
		_ = response.Body.Close()
		return consumeErr
	}
	_ = response.Body.Close()

	for emptyAttempt := 0; ; emptyAttempt++ {
		meaningful := ccaHasMeaningfulContent(state.builder.partial.Content)
		thoughtOnly := ccaHasThinkingOutput(state.builder.partial.Content)
		acceptedSilence := state.acceptsEmptyResponse() && !state.strippedLeak && (isLast || thoughtOnly)
		if state.builder.partial.StopReason != StopReasonStop || meaningful || acceptedSilence {
			state.receivedContent = meaningful || acceptedSilence
			break
		}
		// A thought-only STOP is a complete provider response, not a transiently empty transport.
		if ccaHasThinkingOutput(state.builder.partial.Content) {
			break
		}
		if emptyAttempt >= ccaMaxEmptyStreamRetries {
			break
		}
		state.resetAttempt()
		if err := abortableSleep(state.ctx, time.Duration(ccaEmptyStreamBaseDelayMs<<emptyAttempt)*time.Millisecond); err != nil {
			return errors.New("Request was aborted")
		}
		retry, err := state.post(requestURL)
		if err != nil {
			return &ccaEmptyOutputError{message: err.Error()}
		}
		if retry.StatusCode < 200 || retry.StatusCode >= 300 {
			err := state.httpError(retry)
			_ = retry.Body.Close()
			return err
		}
		consumeErr := state.consumeStream(retry, state.firstEventTimeout())
		_ = retry.Body.Close()
		if consumeErr != nil {
			return consumeErr
		}
	}

	if state.ctx.Err() != nil {
		return errors.New("Request was aborted")
	}
	if state.builder.partial.StopReason == StopReasonError {
		return errors.New(state.builder.partial.ErrorMessage)
	}
	if !state.receivedContent {
		if ccaHasThinkingOutput(state.builder.partial.Content) {
			return &ccaEmptyOutputError{message: "Cloud Code Assist API returned a thought-only response without final output"}
		}
		return &ccaEmptyOutputError{message: "Cloud Code Assist API returned an empty response"}
	}
	if !state.sawFinishReason {
		return &ccaEmptyOutputError{message: "Cloud Code Assist stream ended without a finish reason (connection dropped or response truncated)"}
	}
	ccaRecordEndpointSuccess(state.ccaOptions, endpoint)
	if state.session != nil {
		state.session.LastExecutionID = state.lastResponseID
	}
	usage := state.usage
	state.builder.done(state.builder.partial.StopReason, &usage, "")
	return nil
}

// firstEventTimeout is the wait for the first stream event.
func (state *ccaStream) firstEventTimeout() time.Duration {
	if state.ccaOptions != nil && state.ccaOptions.FirstEventTimeoutMs != nil && *state.ccaOptions.FirstEventTimeoutMs > 0 {
		return time.Duration(*state.ccaOptions.FirstEventTimeoutMs) * time.Millisecond
	}
	if state.options.TimeoutMs != nil && *state.options.TimeoutMs > 0 {
		return time.Duration(*state.options.TimeoutMs) * time.Millisecond
	}
	if ccaFlashStreamLeakWorkaround(state.providerID, state.model.ID) {
		return ccaFlashFirstEventTimeoutMs * time.Millisecond
	}
	return ccaDefaultFirstEventTimeoutMs * time.Millisecond
}

// send performs the request through the shared retry policy. Only the last endpoint retries; a failed
// earlier endpoint returns so the caller can fail over.
func (state *ccaStream) send(requestURL string, isLast bool) (*http.Response, error) {
	maxRetries := 0
	if isLast {
		maxRetries = ccaMaxRetries
	}
	maxDelay := ccaRateLimitBudgetMs
	if state.options.MaxRetryDelayMs != nil {
		maxDelay = *state.options.MaxRetryDelayMs
	}
	return RetryProviderRequest(state.ctx, func() (*http.Response, error) {
		response, err := state.post(requestURL)
		if err != nil {
			return nil, err
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, nil
		}
		rejection := state.httpError(response)
		_ = response.Body.Close()
		return nil, rejection
	}, ProviderRetryOptions{MaxRetries: &maxRetries, MaxRetryDelayMs: &maxDelay})
}

// post sends the request body and returns the response.
func (state *ccaStream) post(requestURL string) (*http.Response, error) {
	requestCtx, cancel := context.WithCancel(state.ctx)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, strings.NewReader(string(state.body)))
	if err != nil {
		cancel()
		return nil, err
	}
	state.applyHeaders(request, requestURL)
	client := providerHTTPClient(state.client, state.options.Fetch)
	response, err := client.Do(request)
	if err != nil {
		cancel()
		if state.ctx.Err() != nil {
			return nil, errors.New("Request was aborted")
		}
		// The Go client's "Post <url>:" decoration is not part of what fetch rejects with.
		var requestError *url.Error
		if errors.As(err, &requestError) {
			return nil, requestError.Err
		}
		return nil, err
	}
	response.Body = &ccaCancelOnClose{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

// applyHeaders sets the request's headers.
func (state *ccaStream) applyHeaders(request *http.Request, requestURL string) {
	// The client identity is the lowest layer: the transport's own client headers (GeminiCLI/... and the
	// Antigravity client string) identify the app to the backend and must win.
	applyIdentityHeaders(request.Header, ClientIdentityTarget{Provider: state.providerID, BaseURL: requestURL}, state.options.Identity)
	ccaApplyHeaders(request, state.credentials.AccessToken, state.clientHeaders, state.claudeBeta, nil, state.options.Headers)
}

// httpError composes the status rejection of one response.
func (state *ccaStream) httpError(response *http.Response) error {
	body := ccaReadErrorBody(response.Body)
	return &ccaHTTPError{
		message: "Cloud Code Assist API error (" + fmt.Sprintf("%d", response.StatusCode) + "): " + ccaErrorText(response.StatusCode, body, state.credentials.Email),
		status:  response.StatusCode,
		headers: response.Header.Clone(),
	}
}

// ccaCancelOnClose cancels the attempt's request context when the body is closed.
type ccaCancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (body *ccaCancelOnClose) Close() error {
	err := body.ReadCloser.Close()
	body.cancel()
	return err
}

// ─── SSE consumption ─────────────────────────────────────────────────────────

// consume folds every SSE event of one response into the builder. The first-event watchdog closes a silent
// response instead of hanging on it, and is disarmed by the first event so an active body is never cut.
func (state *ccaStream) consumeStream(response *http.Response, firstEventTimeout time.Duration) error {
	if response.Body == nil {
		return errors.New("No response body")
	}
	timedOut := make(chan struct{}, 1)
	timer := time.AfterFunc(firstEventTimeout, func() {
		timedOut <- struct{}{}
		_ = response.Body.Close()
	})
	defer timer.Stop()
	decoder := newSSEDecoder(response.Body)
	for {
		if err := state.ctx.Err(); err != nil {
			return errors.New("Request was aborted")
		}
		if !decoder.Next() {
			break
		}
		event := decoder.Event()
		if event.Data == "" {
			continue
		}
		timer.Stop()
		if observer := state.sseObserver(); observer != nil {
			raw := RawSSEEvent{Event: event.Event, Data: event.Data, Raw: append([]string(nil), event.Raw...)}
			if err := observer(state.ctx, raw, state.eventModel); err != nil {
				return err
			}
		}
		if err := state.handleEvent([]byte(event.Data)); err != nil {
			return err
		}
	}
	timer.Stop()
	select {
	case <-timedOut:
		return errors.New(ccaStreamTimeoutMessage)
	default:
	}
	if err := state.ctx.Err(); err != nil {
		return errors.New("Request was aborted")
	}
	if err := decoder.Err(); err != nil {
		return err
	}
	state.finishResponse()
	return nil
}

// handleEvent folds one SSE JSON event into the message.
func (state *ccaStream) handleEvent(data []byte) error {
	if err := state.builder.observeProviderEvent(data); err != nil {
		return err
	}
	var chunk ccaResponseChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return googleChunkJSONError(string(data), err)
	}
	if chunk.Error != nil {
		return state.streamError(chunk.Error)
	}
	payload := chunk.Response
	if payload == nil {
		return nil
	}
	if payload.ResponseID != "" {
		state.lastResponseID = payload.ResponseID
		state.builder.setResponseMetadata(payload.ResponseID, "", "", "", nil)
	}
	if len(payload.Candidates) == 0 && payload.PromptFeedback != nil && payload.PromptFeedback.BlockReason != "" {
		message := "Request blocked by Google (" + payload.PromptFeedback.BlockReason + ")"
		if detail := payload.PromptFeedback.BlockReasonMessage; detail != "" {
			message += ": " + detail
		}
		return errors.New(message)
	}
	var candidate ccaCandidate
	if len(payload.Candidates) > 0 {
		candidate = payload.Candidates[0]
	}
	if candidate.Content != nil {
		for partIndex, part := range candidate.Content.Parts {
			if err := state.handlePart(partIndex, part); err != nil {
				return err
			}
		}
	}
	if candidate.FinishReason != "" {
		state.sawFinishReason = true
		state.finishReason = candidate.FinishReason
		state.builder.setResponseMetadata("", "", candidate.FinishReason, "", nil)
		mapped := mapStopReasonString(candidate.FinishReason)
		// Only a benign finish upgrades to toolUse; an error finish must surface even with tool calls.
		if (mapped == StopReasonStop || mapped == StopReasonLength) && ccaContentHasToolCall(state.builder.partial.Content) {
			state.builder.partial.StopReason = StopReasonToolUse
		} else {
			state.builder.partial.StopReason = mapped
			if mapped == StopReasonError {
				state.errorMessage = "Generation failed with finish reason: " + candidate.FinishReason
				return errors.New(state.errorMessage)
			}
		}
	}
	if payload.UsageMetadata != nil {
		state.usage = ccaMapUsage(payload.UsageMetadata)
		state.builder.calculateCost(&state.usage)
		state.builder.setUsage(&state.usage)
	}
	return nil
}

func ccaContentHasToolCall(content []AssistantContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(ToolCall); ok {
			return true
		}
	}
	return false
}

// handlePart folds one response part: thinking, visible text (healed), a signature-only part, or a function
// call.
func (state *ccaStream) handlePart(partIndex int, part ccaResponsePart) error {
	text, hasText := ccaPartText(part)
	if hasText && text != "" {
		if ccaPartIsThinking(part) {
			state.flushVisibleText("")
			state.startThinkingBlock()
			state.retainThinkingSignature(part.ThoughtSignature)
			state.emitVisibleThinking(text)
			return nil
		}
		if state.buffering {
			state.textBuffer += text
			state.bufferSig = RetainThoughtSignature(state.bufferSig, part.ThoughtSignature)
		} else if state.flashLeak && strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), "{") {
			state.buffering = true
			state.textBuffer = text
			state.bufferSig = part.ThoughtSignature
		} else {
			state.feedVisibleText(text, part.ThoughtSignature)
		}
		if state.buffering {
			buffered := ccaConsumePlanningBuffer(state.textBuffer, state.toolNames, false)
			if buffered.kind != ccaPlanningIncomplete {
				if buffered.kind == ccaPlanningLeak {
					state.strippedLeak = true
				}
				signature := state.bufferSig
				state.buffering = false
				state.textBuffer = ""
				state.bufferSig = ""
				if buffered.visibleText != "" {
					state.feedVisibleText(buffered.visibleText, signature)
				}
			}
		}
	} else if hasText && part.ThoughtSignature != "" && part.FunctionCall == nil {
		state.retainCurrentBlockSignature(part.ThoughtSignature)
	}
	if part.FunctionCall != nil {
		state.flushVisibleText("")
		state.endCurrentBlock()
		state.buffering = false
		state.textBuffer = ""
		name := ccaFunctionCallName(part.FunctionCall)
		providedID := part.FunctionCall.ID
		needsNewID := providedID == "" || ccaContentHasToolCallID(state.builder.partial.Content, providedID)
		if needsNewID {
			callName := name
			if callName == "" {
				callName = "tool"
			}
			providedID = ccaNextToolCallID(callName)
		}
		arguments := JsonObject{}
		if len(part.FunctionCall.Args) > 0 && string(part.FunctionCall.Args) != "null" {
			if parsed := parseStreamingJsonObject(string(part.FunctionCall.Args)); parsed != nil {
				arguments = parsed
			}
		}
		state.handledToolCall(partIndex, ToolCall{ID: providedID, Name: name, Arguments: arguments, ThoughtSignature: part.ThoughtSignature})
	}
	return nil
}

// retainThinkingSignature keeps a thinking part's signature on the open thinking block.
func (state *ccaStream) retainThinkingSignature(signature string) {
	if signature == "" || state.activeThinking < 0 {
		return
	}
	index := state.activeThinking
	block := state.builder.partial.Content[index].(ThinkingContent)
	block.ThinkingSignature = RetainThoughtSignature(block.ThinkingSignature, signature)
	state.builder.partial.Content[index] = block
}

func ccaContentHasToolCallID(content []AssistantContentBlock, id string) bool {
	for _, block := range content {
		if call, ok := block.(ToolCall); ok && call.ID == id {
			return true
		}
	}
	return false
}

// streamError composes the in-band runtime failure: the message, the status in parentheses and the error
// details as the JSON residue Google's event carries.
func (state *ccaStream) streamError(failure *ccaStreamError) error {
	detail := failure.Message
	if detail != "" && failure.Status != "" {
		detail += " (" + failure.Status + ")"
	}
	if detail == "" {
		detail = failure.Status
	}
	if detail == "" {
		detail = "unknown error"
	}
	residue := ""
	if details := strings.TrimSpace(string(failure.Details)); details != "" && details != "null" && strings.HasPrefix(details, "[") && details != "[]" {
		status, _ := json.Marshal(failure.Status)
		residue = ` {"error":{"status":` + string(status) + `,"details":` + details + `}}`
	}
	message := "Cloud Code Assist stream error: " + detail + residue
	if failure.Code != nil && *failure.Code >= 400 {
		return &ccaAPIError{message: message, status: *failure.Code}
	}
	return errors.New(message)
}

// ─── Planning-leak filter ────────────────────────────────────────────────────

// ccaPlanningKind is the outcome of scanning a buffered visible-text prefix.
type ccaPlanningKind int

const (
	ccaPlanningIncomplete ccaPlanningKind = iota
	ccaPlanningPlain
	ccaPlanningLeak
)

// ccaPlanningResult is a scan's visible text and kind.
type ccaPlanningResult struct {
	kind        ccaPlanningKind
	visibleText string
}

// ccaIsPlanningLeakPrefix reports whether a prefix could still become the leaked planning object.
func ccaIsPlanningLeakPrefix(text string) bool {
	trimmed := strings.TrimLeft(text, " \t\n\r")
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	afterBrace := strings.TrimLeft(strings.TrimPrefix(trimmed, "{"), " \t\n\r")
	if afterBrace == "" {
		return len(trimmed) <= 100
	}
	if afterBrace[0] != '"' {
		return false
	}
	nextQuote := strings.IndexByte(afterBrace[1:], '"')
	if nextQuote < 0 {
		return strings.HasPrefix("thought", afterBrace[1:]) && len(trimmed) <= 100
	}
	if afterBrace[1:nextQuote+1] != "thought" {
		return false
	}
	afterKey := strings.TrimLeft(afterBrace[nextQuote+2:], " \t\n\r")
	if afterKey == "" {
		return len(trimmed) <= 100
	}
	return afterKey[0] == ':'
}

// ccaSplitLeadingJSONObject slices the first balanced JSON object off text, respecting strings.
func ccaSplitLeadingJSONObject(text string) (jsonText, rest string, ok bool) {
	prefix := len(text) - len(strings.TrimLeft(text, " \t\n\r"))
	trimmed := text[prefix:]
	if !strings.HasPrefix(trimmed, "{") {
		return "", "", false
	}
	depth := 0
	inString := false
	escaped := false
	for index := range len(trimmed) {
		ch := trimmed[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[:index+1], trimmed[index+1:], true
			}
		}
	}
	return "", "", false
}

// ccaSplitLeadingJSONObjectIgnoringQuotes slices the first brace-balanced object even with unescaped quotes.
func ccaSplitLeadingJSONObjectIgnoringQuotes(text string) (jsonText, rest string, ok bool) {
	trimmed := strings.TrimLeft(text, " \t\n\r")
	if !strings.HasPrefix(trimmed, "{") {
		return "", "", false
	}
	depth := 0
	for index := range len(trimmed) {
		switch trimmed[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[:index+1], trimmed[index+1:], true
			}
		}
	}
	return "", "", false
}

// ccaIsPlanningLeakObject reports a parsed leading object that is a leaked planning envelope.
func ccaIsPlanningLeakObject(parsed any, toolNames map[string]struct{}) bool {
	record, ok := parsed.(map[string]any)
	if !ok {
		return false
	}
	if _, hasThought := record["thought"].(string); hasThought {
		return true
	}
	if name, ok := record["call"].(string); ok {
		if _, known := toolNames[name]; known {
			return true
		}
	}
	for _, key := range []string{"_i", "paths", "command"} {
		if _, ok := record[key]; ok {
			return true
		}
	}
	_, hasPath := record["path"]
	_, hasContent := record["content"]
	return hasPath && hasContent
}

// ccaConsumePlanningBuffer decides what a buffered visible-text prefix is.
func ccaConsumePlanningBuffer(text string, toolNames map[string]struct{}, final bool) ccaPlanningResult {
	if !ccaIsPlanningLeakPrefix(text) {
		return ccaPlanningResult{kind: ccaPlanningPlain, visibleText: text}
	}
	jsonText, rest, ok := ccaSplitLeadingJSONObject(text)
	if !ok {
		jsonText, rest, ok = ccaSplitLeadingJSONObjectIgnoringQuotes(text)
	}
	if !ok {
		if final {
			if ccaBufferLooksLikeLeak(text, toolNames) {
				return ccaPlanningResult{kind: ccaPlanningLeak}
			}
			return ccaPlanningResult{kind: ccaPlanningPlain, visibleText: text}
		}
		return ccaPlanningResult{kind: ccaPlanningIncomplete}
	}
	var parsed any
	if json.Unmarshal([]byte(jsonText), &parsed) == nil {
		if ccaIsPlanningLeakObject(parsed, toolNames) {
			return ccaPlanningResult{kind: ccaPlanningLeak, visibleText: rest}
		}
		return ccaPlanningResult{kind: ccaPlanningPlain, visibleText: text}
	}
	if ccaBufferLooksLikeLeak(jsonText, toolNames) {
		return ccaPlanningResult{kind: ccaPlanningLeak, visibleText: rest}
	}
	return ccaPlanningResult{kind: ccaPlanningPlain, visibleText: text}
}

// ccaBufferLooksLikeLeak is the substring fallback for a prefix that is not valid JSON.
func ccaBufferLooksLikeLeak(text string, toolNames map[string]struct{}) bool {
	if strings.Contains(text, `"thought"`) {
		return true
	}
	for name := range toolNames {
		if strings.Contains(text, `"`+name+`"`) {
			return true
		}
	}
	if strings.Contains(text, `"_i"`) || strings.Contains(text, `"paths"`) || strings.Contains(text, `"command"`) {
		return true
	}
	return strings.Contains(text, `"path"`) && strings.Contains(text, `"content"`)
}

// finishResponse drains the planning buffer and the healing scanner at the end of a response.
func (state *ccaStream) finishResponse() {
	if state.buffering && state.textBuffer != "" {
		buffered := ccaConsumePlanningBuffer(state.textBuffer, state.toolNames, true)
		if buffered.kind == ccaPlanningLeak {
			state.strippedLeak = true
		}
		if buffered.visibleText != "" {
			state.feedVisibleText(buffered.visibleText, state.bufferSig)
		}
		state.bufferSig = ""
		state.buffering = false
		state.textBuffer = ""
	}
	state.flushVisibleText(state.bufferSig)
	state.endCurrentBlock()
}

// sseObserver is the request's raw SSE observer, or nil.
func (state *ccaStream) sseObserver() func(context.Context, RawSSEEvent, *Model) error {
	if state.ccaOptions == nil {
		return nil
	}
	return state.ccaOptions.OnSSEEvent
}

// acceptsEmptyResponse reports whether a blank completion is the final answer instead of a failure.
func (state *ccaStream) acceptsEmptyResponse() bool {
	return state.ccaOptions != nil && state.ccaOptions.AcceptEmptyResponse
}
