package ai

// Devin transport tests: a fake Connect/protobuf server exercises the
// devin-agent provider end to end. Internal package so tests can encode and
// decode the hand-rolled protobuf wire with devinPbWriter/devinPbReader.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Fake server -------------------------------------------------------------

type devinCapturedRequest struct {
	path        string
	header      http.Header
	body        []byte
	connectBody []byte // Connect-envelope payload (unframed)
}

type devinFakeServer struct {
	*httptest.Server
	mu       chan struct{}
	requests []devinCapturedRequest
	// chatFrames are the raw Connect frames the chat handler streams.
	chatFrames   [][]byte
	chatStatus   int
	chatTrailer  string
	assignBody   []byte
	assignStatus int
	authBody     []byte
	authStatus   int
	authURL      string // when set, GetUserJwtResponse carries customApiServerUrl
}

func newDevinFakeServer(t *testing.T) *devinFakeServer {
	t.Helper()
	fake := &devinFakeServer{mu: make(chan struct{}, 1), chatStatus: 200, assignStatus: 200, authStatus: 200}
	fake.mu <- struct{}{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.record(r.URL.Path, r.Header, body)
		switch r.URL.Path {
		case devinAuthPath:
			if fake.authStatus != 200 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fake.authStatus)
				_, _ = w.Write(fake.authBody)
				return
			}
			if fake.authBody != nil {
				w.Header().Set("Content-Type", "application/proto")
				_, _ = w.Write(fake.authBody)
				return
			}
			w.Header().Set("Content-Type", "application/proto")
			_, _ = w.Write(devinTestGetUserJwtResponse("test-user-jwt", fake.authURL))
		case devinAssignModelPath:
			w.Header().Set("Content-Type", "application/proto")
			if fake.assignStatus != 200 {
				w.WriteHeader(fake.assignStatus)
				_, _ = w.Write(fake.assignBody)
				return
			}
			if fake.assignBody != nil {
				_, _ = w.Write(fake.assignBody)
				return
			}
			_, _ = w.Write(devinTestAssignModelResponse("assigned-model-uid", "assign-jwt-1"))
		case devinChatMessagePath:
			if fake.chatStatus != 200 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(fake.chatStatus)
				_, _ = w.Write([]byte(fake.chatTrailer))
				return
			}
			w.Header().Set("Content-Type", "application/connect+proto")
			flusher, _ := w.(http.Flusher)
			for _, frame := range fake.chatFrames {
				_, _ = w.Write(frame)
				if flusher != nil {
					flusher.Flush()
				}
			}
			if fake.chatTrailer != "" {
				_, _ = w.Write(devinFrameConnectMessage([]byte(fake.chatTrailer), devinConnectEndStreamFlag))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	return fake
}

func (f *devinFakeServer) record(path string, header http.Header, body []byte) {
	<-f.mu
	f.requests = append(f.requests, devinCapturedRequest{path: path, header: header.Clone(), body: body})
	f.mu <- struct{}{}
}

func (f *devinFakeServer) captured() []devinCapturedRequest {
	<-f.mu
	out := append([]devinCapturedRequest(nil), f.requests...)
	f.mu <- struct{}{}
	return out
}

func (f *devinFakeServer) requestFor(suffix string) *devinCapturedRequest {
	for i := range f.requests {
		if strings.HasSuffix(f.requests[i].path, suffix) {
			return &f.requests[i]
		}
	}
	return nil
}

// connectPayload unframes a captured Connect request body (handles gzip).
func (f *devinCapturedRequest) connectPayload(t *testing.T) []byte {
	t.Helper()
	body := f.body
	if f.header.Get("connect-content-encoding") == "gzip" {
		reader, err := gzip.NewReader(bytes.NewReader(body[5:]))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		body, err = io.ReadAll(reader)
		if err != nil {
			t.Fatalf("gunzip: %v", err)
		}
		return body
	}
	if len(body) < 5 {
		t.Fatalf("connect frame too short: %d", len(body))
	}
	return body[5:]
}

// --- Response encoders --------------------------------------------------------

func devinTestGetUserJwtResponse(userJWT, customURL string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, userJWT)
	w.appendString(2, customURL)
	return w.buffer
}

func devinTestAssignModelResponse(modelUID, jwt string) []byte {
	assignment := &devinPbWriter{}
	assignment.appendString(1, jwt)
	assignment.appendString(2, modelUID)
	response := &devinPbWriter{}
	response.appendMessage(1, assignment.buffer)
	return response.buffer
}

func devinTestChatToolCall(id, name, args string) []byte {
	return devinChatToolCall(id, name, args)
}

func devinTestChatMessage(fields func(w *devinPbWriter)) []byte {
	w := &devinPbWriter{}
	fields(w)
	return w.buffer
}

func devinTestTextMessage(messageID, text string) []byte {
	return devinTestChatMessage(func(w *devinPbWriter) {
		w.appendString(1, messageID)
		w.appendString(3, text)
	})
}

func devinTestUsageMessage(input, output, cacheWrite, cacheRead uint64) []byte {
	usage := &devinPbWriter{}
	usage.appendVarintField(2, input)
	usage.appendVarintField(3, output)
	usage.appendVarintField(4, cacheWrite)
	usage.appendVarintField(5, cacheRead)
	return usage.buffer
}

func devinTestStreamFrames(messages ...[]byte) [][]byte {
	frames := make([][]byte, len(messages))
	for i, message := range messages {
		frames[i] = devinFrameConnectMessage(message, 0)
	}
	return frames
}

// --- Request decoders ----------------------------------------------------------

type devinTestMetadata struct {
	apiKey  string
	userJWT string
	ideName string
	ideVer  string
	ideType string
	session string
}

type devinTestPrompt struct {
	messageID  string
	source     uint64
	prompt     string
	toolCallID string
	toolCalls  [][3]string
	thinking   string
	signature  string
	images     int
}

type devinTestChatRequest struct {
	metadata      devinTestMetadata
	systemPrompt  string
	prompts       []devinTestPrompt
	chatModelUID  string
	cascadeID     string
	executionID   string
	assignmentJWT string
	requestType   uint64
	disablePar    bool
	toolDefs      [][]byte
	configuration []byte
}

func devinTestReadStringField(reader *devinPbReader, wire int) (string, bool) {
	if wire != 2 {
		return "", false
	}
	payload, ok := reader.readBytes()
	if !ok {
		return "", false
	}
	return devinReadString(payload), true
}

func devinTestReadMetadata(data []byte) devinTestMetadata {
	reader := &devinPbReader{data: data}
	var meta devinTestMetadata
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				meta.ideName = devinReadString(payload)
			case 3:
				meta.apiKey = devinReadString(payload)
			case 5:
				meta.session = devinReadString(payload)
			case 7:
				meta.ideVer = devinReadString(payload)
			case 10:
				// session_id; payload already consumed
			case 12:
			case 21:
				meta.userJWT = devinReadString(payload)
			case 28:
				meta.ideType = devinReadString(payload)
			}
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	return meta
}

func devinTestReadPrompt(data []byte) devinTestPrompt {
	reader := &devinPbReader{data: data}
	var prompt devinTestPrompt
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				prompt.messageID = devinReadString(payload)
			case 3:
				prompt.prompt = devinReadString(payload)
			case 6:
				call := devinTestReadToolCall(payload)
				prompt.toolCalls = append(prompt.toolCalls, call)
			case 7:
				prompt.toolCallID = devinReadString(payload)
			case 10:
				prompt.images++
			case 11:
				prompt.thinking = devinReadString(payload)
			case 12:
				prompt.signature = devinReadString(payload)
			}
			continue
		}
		if field == 2 && wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				break
			}
			prompt.source = value
			continue
		}
		if field == 9 && wire == 0 {
			_, _ = reader.readVarint()
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	return prompt
}

func devinTestReadToolCall(data []byte) [3]string {
	reader := &devinPbReader{data: data}
	var out [3]string
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				out[0] = devinReadString(payload)
			case 2:
				out[1] = devinReadString(payload)
			case 3:
				out[2] = devinReadString(payload)
			}
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	return out
}

func devinTestReadChatRequest(t *testing.T, data []byte) devinTestChatRequest {
	t.Helper()
	reader := &devinPbReader{data: data}
	var req devinTestChatRequest
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				req.metadata = devinTestReadMetadata(payload)
			case 2:
				req.systemPrompt = devinReadString(payload)
			case 3:
				req.prompts = append(req.prompts, devinTestReadPrompt(payload))
			case 8:
				req.configuration = payload
			case 10:
				req.toolDefs = append(req.toolDefs, payload)
			case 12:
			case 13:
			case 16:
				req.cascadeID = devinReadString(payload)
			case 21:
				req.chatModelUID = devinReadString(payload)
			case 22:
				req.executionID = devinReadString(payload)
			case 26:
				req.assignmentJWT = devinReadString(payload)
			}
			continue
		}
		if field == 7 && wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				break
			}
			req.requestType = value
			continue
		}
		if field == 11 && wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				break
			}
			req.disablePar = value != 0
			continue
		}
		if field == 20 && wire == 0 {
			_, _ = reader.readVarint()
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	return req
}

// --- Provider / stream helpers -------------------------------------------------

func devinTestModel(baseURL string, compat map[string]any) *Model {
	meta := ProviderMetadata{API: APIDevinAgent, ProviderID: "devin", BaseURL: baseURL}
	if compat != nil {
		extra := make(map[string]json.RawMessage, len(compat))
		for key, value := range compat {
			raw, _ := json.Marshal(value)
			extra[key] = raw
		}
		meta.Compat = &ModelCompat{Extra: extra}
	}
	return &Model{
		ID:           "swe-1-6",
		DisplayName:  "SWE-1.6",
		Input:        []string{ModelInputText},
		ProviderMeta: meta,
		Capabilities: ModelCapabilities{SupportsToolUse: true, ContextWindow: 200000, MaxOutputTokens: 64000},
	}
}

func devinTestProvider(t *testing.T, fake *devinFakeServer, model *Model, apiKey string) Provider {
	t.Helper()
	provider, err := NewDevinProvider(DevinConfig{
		ModelMetadata: model,
		BaseURL:       fake.URL,
		APIKey:        apiKey,
		Model:         model.ID,
		ProviderID:    "devin",
	})
	if err != nil {
		t.Fatalf("NewDevinProvider: %v", err)
	}
	return provider
}

type devinTestResult struct {
	events    []AssistantMessageEvent
	final     *AssistantMessage
	streamErr *AssistantMessage
}

func devinTestRunStream(t *testing.T, provider Provider, transcript TranscriptContext, opts StreamOptions) devinTestResult {
	t.Helper()
	stream, err := provider.Stream(context.Background(), transcript, opts)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var result devinTestResult
	for event := range stream.Events(context.Background()) {
		result.events = append(result.events, event)
		switch e := event.(type) {
		case DoneEvent:
			result.final = e.Message
		case ErrorEvent:
			result.streamErr = e.Error
		}
	}
	return result
}

func devinTestTranscript() TranscriptContext {
	return NormalizeContext(Context{
		SystemPrompt: "You are helpful.",
		Messages: []Message{
			UserMessage{Content: UserText("hi"), Timestamp: 2},
		},
	})
}

// --- Tests ---------------------------------------------------------------------

func TestDevinStreamTextUsageAndRequestFields(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendString(1, "msg-1")
			w.appendString(3, "Hello")
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendString(3, " world")
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(7, devinTestUsageMessage(11, 7, 3, 5))
		}),
	)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "aaa.bbb.ccc")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{SessionID: "sess-1"})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	if result.final == nil {
		t.Fatal("no final message")
	}
	if result.final.StopReason != StopReasonStop {
		t.Fatalf("stop reason %v", result.final.StopReason)
	}
	var text strings.Builder
	for _, block := range result.final.Content {
		if b, ok := block.(TextContent); ok {
			text.WriteString(b.Text)
		}
	}
	if text.String() != "Hello world" {
		t.Fatalf("text %q", text.String())
	}
	if result.final.ResponseID != "msg-1" {
		t.Fatalf("response id %q", result.final.ResponseID)
	}
	if result.final.Usage.Input != 11 || result.final.Usage.Output != 7 || result.final.Usage.CacheRead != 5 || result.final.Usage.CacheWrite != 3 {
		t.Fatalf("usage %#v", result.final.Usage)
	}

	chat := fake.requestFor("/GetChatMessage")
	if chat == nil {
		t.Fatal("no chat request captured")
	}
	if chat.header.Get("connect-content-encoding") != "" {
		t.Fatalf("unexpected gzip for small request")
	}
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	if req.metadata.apiKey != "devin-session-token$aaa.bbb.ccc" {
		t.Fatalf("apiKey %q", req.metadata.apiKey)
	}
	if req.metadata.userJWT != "test-user-jwt" {
		t.Fatalf("userJwt %q", req.metadata.userJWT)
	}
	if req.metadata.ideName != "devin-cli" || req.metadata.ideVer != devinCliVersion || req.metadata.ideType != "chisel" {
		t.Fatalf("metadata %#v", req.metadata)
	}
	if req.chatModelUID != "swe-1-6" {
		t.Fatalf("chatModelUid %q", req.chatModelUID)
	}
	if req.cascadeID != "sess-1" {
		t.Fatalf("cascadeId %q", req.cascadeID)
	}
	if req.systemPrompt != "You are helpful." {
		t.Fatalf("system prompt %q", req.systemPrompt)
	}
	if len(req.prompts) != 1 || req.prompts[0].source != 1 || req.prompts[0].prompt != "hi" {
		t.Fatalf("prompts %#v", req.prompts)
	}
	if req.requestType != 5 {
		t.Fatalf("requestType %d", req.requestType)
	}
	topP, stops := devinTestReadConfiguration(t, req.configuration)
	if topP != float64(float32(0.95)) {
		t.Fatalf("default topP %v", topP)
	}
	if len(stops) != 0 {
		t.Fatalf("unexpected stopPatterns %v", stops)
	}
}

func TestDevinStreamRawAPIKeyUnprefixed(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "ok"))
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "sk-ws-abc123")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	chat := fake.requestFor("/GetChatMessage")
	if chat == nil {
		t.Fatal("no chat request")
	}
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	if req.metadata.apiKey != "sk-ws-abc123" {
		t.Fatalf("apiKey %q", req.metadata.apiKey)
	}
}

func TestDevinStreamThinkingAndToolCallCumulative(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendString(1, "msg-t")
			w.appendString(9, "let me ")
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendString(9, "think")
			w.appendString(10, "sig-1")
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(6, devinTestChatToolCall("call_1", "read_file", `{"path":"/a`))
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(6, devinTestChatToolCall("call_1", "", `{"path":"/a/b"`))
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(6, devinTestChatToolCall("call_1", "", `{"path":"/a/b"}`))
			w.appendVarintField(5, 2) // tool use-ish stop; tool call wins anyway
		}),
	)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	if result.final == nil || result.final.StopReason != StopReasonToolUse {
		t.Fatalf("final %#v", result.final)
	}
	var thinking, toolName, toolArgs strings.Builder
	for _, block := range result.final.Content {
		switch b := block.(type) {
		case ThinkingContent:
			thinking.WriteString(b.Thinking)
		case ToolCall:
			toolName.WriteString(b.Name)
			args, _ := json.Marshal(b.Arguments)
			toolArgs.Write(args)
		}
	}
	if thinking.String() != "let me think" {
		t.Fatalf("thinking %q", thinking.String())
	}
	if toolName.String() != "read_file" {
		t.Fatalf("tool name %q", toolName.String())
	}
	if !strings.Contains(toolArgs.String(), `"path":"/a/b"`) && !strings.Contains(toolArgs.String(), `"path": "/a/b"`) {
		t.Fatalf("tool args %s", toolArgs.String())
	}
}

func TestDevinStreamToolCallIncremental(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(6, devinTestChatToolCall("call_9", "run", `{"x":`))
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			// Incremental fragment: not a prefix of the previous text.
			w.appendMessage(6, devinTestChatToolCall("call_9", "", `1}`))
		}),
	)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	var args string
	for _, block := range result.final.Content {
		if b, ok := block.(ToolCall); ok {
			raw, _ := json.Marshal(b.Arguments)
			args = string(raw)
		}
	}
	if !strings.Contains(args, `"x":1`) && !strings.Contains(args, `"x": 1`) {
		t.Fatalf("args %s", args)
	}
}

func TestDevinStreamMaxTokensStopReason(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendString(3, "partial")
			w.appendVarintField(5, 3) // MAX_TOKENS
		}),
	)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	if result.final == nil || result.final.StopReason != StopReasonLength {
		t.Fatalf("stop reason %#v", result.final)
	}
}

func TestDevinStreamTrailerError(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "x"))
	fake.chatTrailer = `{"error":{"code":"resource_exhausted","message":"quota exceeded","details":[]}}`
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr == nil {
		t.Fatal("expected trailer error")
	}
	if !strings.Contains(result.streamErr.ErrorMessage, "quota exceeded") {
		t.Fatalf("error %v", result.streamErr)
	}
	if result.final != nil {
		t.Fatalf("unexpected final %#v", result.final)
	}
}

func TestDevinHTTPErrorJSONDetail(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.authStatus = 401
	fake.authBody = []byte(`{"code":16,"message":"invalid session token"}`)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.streamErr.ErrorMessage, "invalid session token") {
		t.Fatalf("error %v", result.streamErr)
	}
}

func TestDevinHTTPErrorHTMLSuppressed(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.authStatus = 502
	fake.authBody = []byte(`<html><body>Bad Gateway <script>alert(1)</script></body></html>`)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr == nil {
		t.Fatal("expected error")
	}
	errText := result.streamErr.ErrorMessage
	if strings.Contains(errText, "<html") || strings.Contains(errText, "<script") {
		t.Fatalf("html leaked: %v", errText)
	}
	if !strings.Contains(errText, "502") {
		t.Fatalf("error %v", errText)
	}
}

func TestDevinRouterAssignModel(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "routed"))
	model := devinTestModel(fake.URL, map[string]any{
		"modelRouter":    true,
		"requestModelId": "router-swe",
	})
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{SessionID: "sess-r"})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}

	assign := fake.requestFor("/AssignModel")
	if assign == nil {
		t.Fatal("no AssignModel request")
	}
	assignPayload := assign.body
	// AssignModel is a unary proto body, not a Connect frame.
	assignReader := &devinPbReader{data: assignPayload}
	var routerUID, cascadeID string
	for assignReader.index < len(assignReader.data) {
		field, wire, ok := assignReader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := assignReader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				// metadata
			case 2:
				routerUID = devinReadString(payload)
			case 3:
				cascadeID = devinReadString(payload)
			case 5:
				// prompt
			}
			continue
		}
		if !assignReader.skip(wire) {
			break
		}
	}
	if routerUID != "router-swe" {
		t.Fatalf("router uid %q", routerUID)
	}
	if cascadeID != "sess-r" {
		t.Fatalf("cascade %q", cascadeID)
	}

	chat := fake.requestFor("/GetChatMessage")
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	if req.chatModelUID != "assigned-model-uid" {
		t.Fatalf("chatModelUid %q", req.chatModelUID)
	}
	if req.assignmentJWT != "assign-jwt-1" {
		t.Fatalf("assignment jwt %q", req.assignmentJWT)
	}
}

func TestDevinFrameCap(t *testing.T) {
	fake := newDevinFakeServer(t)
	// Frame header declares a payload above the 16 MiB cap.
	header := []byte{0, 0xFF, 0xFF, 0xFF, 0xFF}
	fake.chatFrames = [][]byte{append(header, []byte("x")...)}
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr == nil {
		t.Fatal("expected frame-cap error")
	}
	if !strings.Contains(result.streamErr.ErrorMessage, "cap") {
		t.Fatalf("error %v", result.streamErr)
	}
}

func TestDevinCompressedFrameCap(t *testing.T) {
	fake := newDevinFakeServer(t)
	// A small compressed frame that inflates past the 16 MiB cap.
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write(make([]byte, devinMaxConnectFrameBytes+1))
	_ = writer.Close()
	fake.chatFrames = [][]byte{devinFrameConnectMessage(compressed.Bytes(), devinConnectCompressedFlag)}
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr == nil {
		t.Fatal("expected decompressed-size cap error")
	}
	if !strings.Contains(result.streamErr.ErrorMessage, "cap") {
		t.Fatalf("error %v", result.streamErr)
	}
}

func TestDevinGzipRequestAboveThreshold(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "ok"))
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	big := strings.Repeat("x", 4096)
	transcript := NormalizeContext(Context{Messages: []Message{
		UserMessage{Content: UserText(big), Timestamp: 1},
	}})
	result := devinTestRunStream(t, provider, transcript, StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	chat := fake.requestFor("/GetChatMessage")
	if chat == nil {
		t.Fatal("no chat request")
	}
	if chat.header.Get("connect-content-encoding") != "gzip" {
		t.Fatalf("expected gzip content encoding, headers %v", chat.header)
	}
	payload := chat.connectPayload(t)
	req := devinTestReadChatRequest(t, payload)
	if len(req.prompts) != 1 || !strings.HasPrefix(req.prompts[0].prompt, big[:100]) {
		t.Fatalf("gzip payload did not decode: %#v", req.prompts)
	}
}

func TestDevinCustomAPIBaseURL(t *testing.T) {
	fake := newDevinFakeServer(t)
	// Second server stands in for the custom API base returned by GetUserJwt.
	chatFake := newDevinFakeServer(t)
	chatFake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "custom"))
	fake.authURL = chatFake.URL
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	if chatFake.requestFor("/GetChatMessage") == nil {
		t.Fatal("chat did not hit custom base URL")
	}
	if fake.requestFor("/GetChatMessage") != nil {
		t.Fatal("chat hit original base URL")
	}
}

func TestDevinToolSchemaOnWire(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "ok"))
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	transcript := NormalizeContext(Context{
		Messages: []Message{UserMessage{Content: UserText("hi"), Timestamp: 1}},
		Tools: []ToolSchema{{
			Name:        "lookup",
			Description: "look things up",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
		}},
	})
	result := devinTestRunStream(t, provider, transcript, StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	chat := fake.requestFor("/GetChatMessage")
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	if len(req.toolDefs) != 1 {
		t.Fatalf("tool defs %#v", req.toolDefs)
	}
	// toolDefs entries are raw ChatToolDefinition protobuf; decode name/schema.
	reader := &devinPbReader{data: req.toolDefs[0]}
	var name, schema string
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		if wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				break
			}
			switch field {
			case 1:
				name = devinReadString(payload)
			case 3:
				schema = devinReadString(payload)
			}
			continue
		}
		if !reader.skip(wire) {
			break
		}
	}
	if name != "lookup" {
		t.Fatalf("tool name %q", name)
	}
	if !strings.Contains(schema, `"q"`) {
		t.Fatalf("schema %q", schema)
	}
}

func TestDevinTemperatureFloor(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "ok"))
	model := devinTestModel(fake.URL, map[string]any{"temperatureFloor": 0.5})
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{Temperature: 0.1, TemperatureSet: true})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	chat := fake.requestFor("/GetChatMessage")
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	// CompletionConfiguration field 5 is temperature (double, wire 1).
	configReader := &devinPbReader{data: req.configuration}
	var temperature float64
	for configReader.index < len(configReader.data) {
		field, wire, ok := configReader.readTag()
		if !ok {
			break
		}
		if field == 5 && wire == 1 {
			raw, ok := configReader.readFixed64()
			if !ok {
				break
			}
			temperature = math.Float64frombits(raw)
			continue
		}
		if !configReader.skip(wire) {
			break
		}
	}
	if temperature != 0.5 {
		t.Fatalf("temperature %v", temperature)
	}
}

// devinTestReadConfiguration reads topP (field 8, double) and stopPatterns
// (field 9, strings) out of an encoded CompletionConfiguration.
func devinTestReadConfiguration(t *testing.T, data []byte) (float64, []string) {
	t.Helper()
	reader := &devinPbReader{data: data}
	var topP float64
	var stops []string
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			break
		}
		switch {
		case field == 8 && wire == 1:
			raw, ok := reader.readFixed64()
			if !ok {
				return topP, stops
			}
			topP = math.Float64frombits(raw)
		case field == 9 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return topP, stops
			}
			stops = append(stops, devinReadString(payload))
		default:
			if !reader.skip(wire) {
				return topP, stops
			}
		}
	}
	return topP, stops
}

func TestDevinStreamSamplingParams(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(devinTestTextMessage("m", "ok"))
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{
		SamplingParams: SamplingParams{"topP": 0.123456789, "stopSequences": []any{"<<END>>", 7, "<<"}},
	})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	chat := fake.requestFor("/GetChatMessage")
	req := devinTestReadChatRequest(t, chat.connectPayload(t))
	topP, stops := devinTestReadConfiguration(t, req.configuration)
	// topP is sent float32-rounded, as Devin's CLI does.
	if topP != float64(float32(0.123456789)) {
		t.Fatalf("topP %v", topP)
	}
	if len(stops) != 2 || stops[0] != "<<END>>" || stops[1] != "<<" {
		t.Fatalf("stopPatterns %v", stops)
	}
}

func TestDevinStreamToolCallLateName(t *testing.T) {
	fake := newDevinFakeServer(t)
	fake.chatFrames = devinTestStreamFrames(
		devinTestChatMessage(func(w *devinPbWriter) {
			w.appendMessage(6, devinTestChatToolCall("call_late", "", `{"a":`))
		}),
		devinTestChatMessage(func(w *devinPbWriter) {
			// A later delta may carry the name (omp: `if (tc.name) block.name = tc.name`).
			w.appendMessage(6, devinTestChatToolCall("call_late", "late_tool", `1}`))
		}),
	)
	model := devinTestModel(fake.URL, nil)
	provider := devinTestProvider(t, fake, model, "key")
	defer provider.Close()

	result := devinTestRunStream(t, provider, devinTestTranscript(), StreamOptions{})
	if result.streamErr != nil {
		t.Fatalf("stream error: %v", result.streamErr)
	}
	var name, args string
	for _, block := range result.final.Content {
		if b, ok := block.(ToolCall); ok {
			name = b.Name
			raw, _ := json.Marshal(b.Arguments)
			args = string(raw)
		}
	}
	if name != "late_tool" {
		t.Fatalf("tool name %q", name)
	}
	if !strings.Contains(args, `"a":1`) && !strings.Contains(args, `"a": 1`) {
		t.Fatalf("args %s", args)
	}
}
