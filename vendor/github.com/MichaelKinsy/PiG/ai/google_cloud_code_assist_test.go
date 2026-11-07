package ai

// Behavioral tests of the Cloud Code Assist transport against a deterministic fixture server: request
// building, streamed text/thinking/tool-call folds, usage, stop reasons, in-band errors, endpoint failover,
// empty-response re-requests, cancellation, markup healing and the Antigravity envelope.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const ccaTestCredential = `{"token":"test-token","projectId":"test-project"}`

type ccaRecordedRequest struct {
	path   string
	query  string
	header http.Header
	body   string
}

type ccaFixture struct {
	*httptest.Server
	mu       sync.Mutex
	requests []ccaRecordedRequest
	// statuses answers the n-th request with that status; zero serves the events.
	statuses []int
	// events serves the stream body; handler overrides both when set.
	events  []string
	handler func(w http.ResponseWriter, r *http.Request, attempt int)
}

func newCCAFixture(t *testing.T) *ccaFixture {
	t.Helper()
	fixture := &ccaFixture{}
	fixture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fixture.mu.Lock()
		fixture.requests = append(fixture.requests, ccaRecordedRequest{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: string(body)})
		attempt := len(fixture.requests) - 1
		status := 0
		if attempt < len(fixture.statuses) {
			status = fixture.statuses[attempt]
		}
		events := fixture.events
		handler := fixture.handler
		fixture.mu.Unlock()
		if handler != nil {
			handler(w, r, attempt)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"fixture failure"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, event := range events {
			_, _ = io.WriteString(w, "data: "+event+"\n\n")
		}
	}))
	t.Cleanup(fixture.Close)
	return fixture
}

func ccaTestModel(providerID, modelID, baseURL string, reasoning bool) *Model {
	return &Model{
		ID:    modelID,
		Input: []ModelInputModality{ModelInputText},
		ProviderMeta: ProviderMetadata{
			ProviderID: providerID, API: APIGoogleGeminiCLI, BaseURL: baseURL, Reasoning: reasoning,
		},
		Capabilities: ModelCapabilities{ContextWindow: 100000, MaxOutputTokens: 8192, MaxThinking: ThinkingLevelHigh},
	}
}

func ccaTestContext(text string) Context {
	return Context{Messages: []Message{UserMessage{Content: UserText(text), Timestamp: 1}}}
}

func ccaCollect(t *testing.T, stream *AssistantMessageEventStream) (*AssistantMessage, []AssistantMessageEvent) {
	t.Helper()
	var events []AssistantMessageEvent
	for event := range stream.Events(context.Background()) {
		events = append(events, event)
	}
	return stream.Result(), events
}

func ccaMessageText(message *AssistantMessage) string {
	text := ""
	for _, block := range message.Content {
		if value, ok := block.(TextContent); ok {
			text += value.Text
		}
	}
	return text
}

func ccaMessageThinking(message *AssistantMessage) string {
	thinking := ""
	for _, block := range message.Content {
		if value, ok := block.(ThinkingContent); ok {
			thinking += value.Thinking
		}
	}
	return thinking
}

func TestCCATransportTextThinkingToolCallsUsage(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Let me look.","thought":true,"thoughtSignature":"c2ln"}]}}],"responseId":"resp-1"}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":" world","thoughtSignature":"dGV4dA=="}]}}]}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"a.txt"},"id":"call-1"}}]}}]}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":5,"totalTokenCount":125,"cachedContentTokenCount":10}}}`,
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)

	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, events := ccaCollect(t, stream)
	if result.StopReason != StopReasonToolUse {
		t.Fatalf("stop reason = %q, want toolUse (%s)", result.StopReason, result.ErrorMessage)
	}
	if text := ccaMessageText(result); text != "Hello world" {
		t.Fatalf("text = %q", text)
	}
	if thinking := ccaMessageThinking(result); thinking != "Let me look." {
		t.Fatalf("thinking = %q", thinking)
	}
	if result.Usage.Input != 90 || result.Usage.Output != 25 || result.Usage.CacheRead != 10 || result.Usage.TotalTokens != 125 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if result.Usage.Reasoning == nil || *result.Usage.Reasoning != 5 {
		t.Fatalf("reasoning usage = %v", result.Usage.Reasoning)
	}
	var call *ToolCall
	for _, block := range result.Content {
		if value, ok := block.(ToolCall); ok {
			call = &value
		}
	}
	if call == nil || call.ID != "call-1" || call.Name != "read" || call.Arguments["path"] != "a.txt" {
		t.Fatalf("tool call = %#v", call)
	}
	lifecycle := map[string]bool{}
	for _, event := range events {
		switch event.(type) {
		case ThinkingDeltaEvent:
			lifecycle["thinking"] = true
		case TextDeltaEvent:
			lifecycle["text"] = true
		case ToolCallEndEvent:
			lifecycle["tool"] = true
		case DoneEvent:
			lifecycle["done"] = true
		}
	}
	for _, want := range []string{"thinking", "text", "tool", "done"} {
		if !lifecycle[want] {
			t.Fatalf("missing %q event: %v", want, lifecycle)
		}
	}

	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.requests) != 1 {
		t.Fatalf("requests = %d", len(fixture.requests))
	}
	sent := fixture.requests[0]
	if sent.path != "/v1internal:streamGenerateContent" || sent.query != "alt=sse" {
		t.Fatalf("target = %s?%s", sent.path, sent.query)
	}
	if got := sent.header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("authorization = %q", got)
	}
	if got := sent.header.Get("User-Agent"); !strings.HasPrefix(got, "GeminiCLI/") {
		t.Fatalf("user agent = %q", got)
	}
	if got := sent.header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("accept = %q", got)
	}
	var body struct {
		Project string `json:"project"`
		Model   string `json:"model"`
		Request struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(sent.body), &body); err != nil {
		t.Fatalf("request body: %v (%s)", err, sent.body)
	}
	if body.Project != "test-project" || body.Model != "gemini-3.1-pro-preview" {
		t.Fatalf("identity = %q/%q", body.Project, body.Model)
	}
	if len(body.Request.Contents) != 1 || body.Request.Contents[0].Parts[0].Text != "hi" {
		t.Fatalf("contents = %#v", body.Request.Contents)
	}
	if strings.Contains(sent.body, "labels") {
		t.Fatalf("gemini-cli request carries antigravity labels: %s", sent.body)
	}
}

func TestCCATransportAntigravityEnvelope(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}}`,
	}
	model := ccaTestModel(GoogleAntigravityProviderID, "claude-sonnet-4-6", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonStop || ccaMessageText(result) != "ok" {
		t.Fatalf("result = %+v", result)
	}

	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	sent := fixture.requests[0]
	if got := sent.header.Get("User-Agent"); !strings.HasPrefix(got, "antigravity/hub/") {
		t.Fatalf("user agent = %q", got)
	}
	if got := sent.header.Get("Accept-Encoding"); got != "gzip" {
		t.Fatalf("accept-encoding = %q", got)
	}
	if got := sent.header.Get("anthropic-beta"); got != ccaClaudeThinkingBetaHeader {
		t.Fatalf("claude beta = %q", got)
	}
	var body struct {
		Model       string `json:"model"`
		UserAgent   string `json:"userAgent"`
		RequestID   string `json:"requestId"`
		RequestType string `json:"requestType"`
		Request     struct {
			SessionID  string    `json:"sessionId"`
			Labels     ccaLabels `json:"labels"`
			ToolConfig *struct {
				FunctionCallingConfig struct {
					Mode string `json:"mode"`
				} `json:"functionCallingConfig"`
			} `json:"toolConfig"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(sent.body), &body); err != nil {
		t.Fatalf("request body: %v (%s)", err, sent.body)
	}
	if body.UserAgent != "antigravity" || body.Model != "claude-sonnet-4-6" {
		t.Fatalf("identity = %q/%q", body.UserAgent, body.Model)
	}
	if !strings.HasPrefix(body.Request.SessionID, "-") {
		t.Fatalf("session id = %q, want signed decimal", body.Request.SessionID)
	}
	if !strings.HasPrefix(body.RequestID, "agent/") || !strings.HasSuffix(body.RequestID, "/1") {
		t.Fatalf("request id = %q", body.RequestID)
	}
	labels := body.Request.Labels
	if labels.UsedClaude != "true" || labels.UsedClaudeConservative != "true" || labels.UsedNonGeminiModel != "true" {
		t.Fatalf("usage labels = %+v, want JSON strings", labels)
	}
	if labels.CascadeID == "" || labels.CascadeID != labels.RootCascadeID || labels.TrajectoryID == "" {
		t.Fatalf("cascade labels = %+v", labels)
	}
	if labels.LastStepIndex != "0" || !strings.HasSuffix(labels.RequestID, "-0") {
		t.Fatalf("step labels = %+v", labels)
	}
	// Claude on Antigravity always forces VALIDATED, even without tools.
	if body.Request.ToolConfig == nil || body.Request.ToolConfig.FunctionCallingConfig.Mode != "VALIDATED" {
		t.Fatalf("tool config = %+v", body.Request.ToolConfig)
	}
	if strings.Contains(sent.body, `"used_claude":{`) {
		t.Fatalf("used_claude was serialized as an object: %s", sent.body)
	}
}

func TestCCATransportFailsOverToSandbox(t *testing.T) {
	primary := newCCAFixture(t)
	primary.statuses = []int{http.StatusServiceUnavailable}
	sandbox := newCCAFixture(t)
	sandbox.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"recovered"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	previousPrimary, previousSandbox := googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint
	googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = primary.URL, sandbox.URL
	t.Cleanup(func() {
		googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = previousPrimary, previousSandbox
	})
	model := ccaTestModel(GoogleAntigravityProviderID, "gemini-3.1-pro", "", true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonStop || ccaMessageText(result) != "recovered" {
		t.Fatalf("result = %+v", result)
	}
	primary.mu.Lock()
	primaryCount := len(primary.requests)
	primary.mu.Unlock()
	sandbox.mu.Lock()
	sandboxCount := len(sandbox.requests)
	sandbox.mu.Unlock()
	if primaryCount == 0 || sandboxCount == 0 {
		t.Fatalf("attempts: primary=%d sandbox=%d", primaryCount, sandboxCount)
	}
}

func TestCCATransportInBandError(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"partial"}]}}]}}`,
		`{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED","details":[{"reason":"RATE_LIMIT"}]}}`,
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonError {
		t.Fatalf("stop reason = %q", result.StopReason)
	}
	for _, want := range []string{"Cloud Code Assist stream error", "Resource exhausted", "RESOURCE_EXHAUSTED", "RATE_LIMIT"} {
		if !strings.Contains(result.ErrorMessage, want) {
			t.Fatalf("error %q lacks %q", result.ErrorMessage, want)
		}
	}
}

func TestCCATransportHTTPValidationInstruction(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.handler = func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://example.test/verify"}}]}}`)
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	noRetries := 0
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, MaxRetries: &noRetries})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonError {
		t.Fatalf("stop reason = %q", result.StopReason)
	}
	if !strings.Contains(result.ErrorMessage, "Account verification required") || !strings.Contains(result.ErrorMessage, "https://example.test/verify") {
		t.Fatalf("error = %q", result.ErrorMessage)
	}
	if !strings.Contains(result.ErrorMessage, "Cloud Code Assist API error (403)") {
		t.Fatalf("status missing: %q", result.ErrorMessage)
	}
}

func TestCCATransportRejectsBadCredential(t *testing.T) {
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", "http://127.0.0.1:1", true)
	invalid, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: `not json`})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	rejected, _ := ccaCollect(t, invalid)
	if rejected.StopReason != StopReasonError || !strings.Contains(rejected.ErrorMessage, "Invalid Google Cloud Code Assist credentials") {
		t.Fatalf("invalid credential result = %+v", rejected)
	}
	fixture := newCCAFixture(t)
	model.ProviderMeta.BaseURL = fixture.URL
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: `{"projectId":"p"}`})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "Missing token or projectId") {
		t.Fatalf("result = %+v", result)
	}
}

func TestCCATransportHealsLeakedThinkingMarkup(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"<thinking>hidden reasoning</thinking>visible answer"}]}}]}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}}`,
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if text := ccaMessageText(result); text != "visible answer" {
		t.Fatalf("visible text = %q", text)
	}
	if thinking := ccaMessageThinking(result); thinking != "hidden reasoning" {
		t.Fatalf("thinking = %q", thinking)
	}
}

func TestCCATransportRetriesEmptyStream(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.handler = func(w http.ResponseWriter, _ *http.Request, attempt int) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if attempt == 0 {
			_, _ = io.WriteString(w, "data: "+`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"   "}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":0,"totalTokenCount":4}}}`+"\n\n")
			return
		}
		_, _ = io.WriteString(w, "data: "+`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"real answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6}}}`+"\n\n")
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonStop || ccaMessageText(result) != "real answer" {
		t.Fatalf("result = %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.requests) != 2 {
		t.Fatalf("attempts = %d, want 2", len(fixture.requests))
	}
}

func TestCCATransportCancellation(t *testing.T) {
	blocked := make(chan struct{})
	fixture := &ccaFixture{}
	fixture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-blocked
	}))
	ctx, cancel := context.WithCancel(context.Background())
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(ctx, model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		close(blocked)
		fixture.Close()
		t.Fatalf("stream setup: %v", err)
	}
	cancel()
	result, _ := ccaCollect(t, stream)
	close(blocked)
	fixture.Close()
	if result.StopReason != StopReasonAborted {
		t.Fatalf("stop reason = %q (%s)", result.StopReason, result.ErrorMessage)
	}
}

func TestCCATransportSafetyFinishIsError(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"nope"}]},"finishReason":"SAFETY"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "Generation failed with finish reason: SAFETY") {
		t.Fatalf("result = %+v", result)
	}
}

func TestCCACredentialBlobAliases(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: `{"token":"t","project_id":"p"}`})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
		t.Fatalf("result = %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	var body struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal([]byte(fixture.requests[0].body), &body); err != nil || body.Project != "p" {
		t.Fatalf("project = %q (%v)", body.Project, err)
	}
}

func TestCCANormalizeSchemaForCCAStripsUnsupportedKeywords(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "minLength": 1, "pattern": "^/"},
			"unit": map[string]any{"type": []any{"string", "null"}, "nullable": true},
			"opts": map[string]any{"anyOf": []any{
				map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}},
				map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "number"}}},
			}},
		},
		"required": []any{"path", "unit"},
	}
	normalized, ok := ccaNormalizeSchemaForCCA(schema, nil).(map[string]any)
	if !ok {
		t.Fatalf("normalized schema has the wrong shape")
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	text := string(encoded)
	for _, banned := range []string{`"anyOf"`, `"nullable"`, `"minLength"`, `"pattern"`, `["string"`} {
		if strings.Contains(text, banned) {
			t.Fatalf("normalized schema still carries %s: %s", banned, text)
		}
	}
	if !strings.Contains(text, "minLength") || !strings.Contains(text, "pattern") {
		t.Fatalf("lifted keywords missing from the description spill: %s", text)
	}
	properties, _ := normalized["properties"].(map[string]any)
	opts, _ := properties["opts"].(map[string]any)
	optsProperties, _ := opts["properties"].(map[string]any)
	if _, ok := optsProperties["a"]; !ok {
		t.Fatalf("object combiner branches not merged: %s", text)
	}
}

func TestCCAPlanningBufferDropsLeakedEnvelope(t *testing.T) {
	tools := map[string]struct{}{"read": {}}
	leak := `{"thought":"I should read the file","call":"read","path":"a.txt"}`
	result := ccaConsumePlanningBuffer(leak+"rest", tools, false)
	if result.kind != ccaPlanningLeak || result.visibleText != "rest" {
		t.Fatalf("leak scan = %+v", result)
	}
	plain := ccaConsumePlanningBuffer(`{"answer":"hello"}`, tools, false)
	if plain.kind != ccaPlanningPlain || plain.visibleText != `{"answer":"hello"}` {
		t.Fatalf("plain scan = %+v", plain)
	}
	if incomplete := ccaConsumePlanningBuffer(`{"thou`, tools, false); incomplete.kind != ccaPlanningIncomplete {
		t.Fatalf("partial scan = %+v", incomplete)
	}
}

func TestCCATransportEndpointModesAndLastGoodHost(t *testing.T) {
	production, sandbox := newCCAFixture(t), newCCAFixture(t)
	production.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"p"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	sandbox.events = production.events
	previousProduction, previousSandbox := googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint
	googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = production.URL, sandbox.URL
	t.Cleanup(func() {
		googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = previousProduction, previousSandbox
	})
	model := ccaTestModel(GoogleAntigravityProviderID, "gemini-3.1-pro", "", true)
	session := &CCAAntigravitySessionState{}

	// Sandbox mode pins the sandbox host and forgets the last good one.
	sandboxOnly := &GoogleCloudCodeOptions{EndpointMode: "sandbox", Session: session}
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, GoogleCloudCode: sandboxOnly})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
		t.Fatalf("result = %+v", result)
	}
	production.mu.Lock()
	productionCount := len(production.requests)
	production.mu.Unlock()
	sandbox.mu.Lock()
	sandboxCount := len(sandbox.requests)
	sandbox.mu.Unlock()
	if productionCount != 0 || sandboxCount != 1 {
		t.Fatalf("sandbox mode attempts: production=%d sandbox=%d", productionCount, sandboxCount)
	}

	// Auto mode starts from the remembered host.
	session.LastGoodEndpoint = production.URL
	auto := &GoogleCloudCodeOptions{Session: session}
	stream, err = StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, GoogleCloudCode: auto})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
		t.Fatalf("result = %+v", result)
	}
	production.mu.Lock()
	autoCount := len(production.requests)
	production.mu.Unlock()
	if autoCount != 1 {
		t.Fatalf("auto mode did not prefer the last good host: production=%d", autoCount)
	}
	if session.LastGoodEndpoint != production.URL {
		t.Fatalf("last good endpoint = %q", session.LastGoodEndpoint)
	}
}

func TestCCATransportRequestModelIDAndSessionEnvelope(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"responseId":"resp-9","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	session := &CCAAntigravitySessionState{}
	options := &GoogleCloudCodeOptions{RequestModelID: "gemini-3.8-flash-low", Session: session}
	model := ccaTestModel(GoogleAntigravityProviderID, "gemini-3.8-flash-tiered", fixture.URL, true)
	for attempt := 0; attempt < 2; attempt++ {
		stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, GoogleCloudCode: options})
		if err != nil {
			t.Fatalf("stream setup: %v", err)
		}
		if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
			t.Fatalf("result = %+v", result)
		}
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.requests) != 2 {
		t.Fatalf("requests = %d", len(fixture.requests))
	}
	var first struct {
		Model     string `json:"model"`
		RequestID string `json:"requestId"`
		Request   struct {
			Labels ccaLabels `json:"labels"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(fixture.requests[0].body), &first); err != nil {
		t.Fatalf("body: %v", err)
	}
	if first.Model != "gemini-3.8-flash-low" {
		t.Fatalf("wire model = %q, want the override", first.Model)
	}
	var second struct {
		Model     string `json:"model"`
		RequestID string `json:"requestId"`
		Request   struct {
			Labels ccaLabels `json:"labels"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(fixture.requests[1].body), &second); err != nil {
		t.Fatalf("body: %v", err)
	}
	// The session advances the step and echoes the prior response id; the request id keeps its shape.
	if session.StepIndex != 2 || session.RequestCount != 2 {
		t.Fatalf("session counters = %+v", session)
	}
	if !strings.HasSuffix(second.RequestID, "/2") {
		t.Fatalf("second request id = %q", second.RequestID)
	}
	if second.Request.Labels.LastExecutionID != "resp-9" {
		t.Fatalf("last execution id = %q", second.Request.Labels.LastExecutionID)
	}
	if second.Request.Labels.LastStepIndex != "1" || !strings.HasSuffix(second.Request.Labels.RequestID, "-1") {
		t.Fatalf("step labels = %+v", second.Request.Labels)
	}
	if session.LastExecutionID != "resp-9" {
		t.Fatalf("session execution id = %q", session.LastExecutionID)
	}
}

func TestCCATransportHideThinkingAndUsageLabel(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	level := GoogleThinkingLevelHigh
	label := "custom-bucket"
	options := &GoogleCloudCodeOptions{HideThinkingSummary: true, UsageLabel: &label}
	model := ccaTestModel(GoogleAntigravityProviderID, "gemini-3.1-pro", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{
		APIKey:          ccaTestCredential,
		GoogleCloudCode: options,
		GoogleThinking:  &GoogleThinkingOptions{Enabled: true, Level: level},
	})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
		t.Fatalf("result = %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	var body struct {
		Request struct {
			GenerationConfig struct {
				ThinkingConfig struct {
					IncludeThoughts *bool  `json:"includeThoughts"`
					ThinkingLevel   string `json:"thinkingLevel"`
				} `json:"thinkingConfig"`
			} `json:"generationConfig"`
			Labels ccaLabels `json:"labels"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(fixture.requests[0].body), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body.Request.GenerationConfig.ThinkingConfig.IncludeThoughts == nil || *body.Request.GenerationConfig.ThinkingConfig.IncludeThoughts {
		t.Fatalf("includeThoughts = %v, want false", body.Request.GenerationConfig.ThinkingConfig.IncludeThoughts)
	}
	if body.Request.GenerationConfig.ThinkingConfig.ThinkingLevel != "HIGH" {
		t.Fatalf("thinking level = %q", body.Request.GenerationConfig.ThinkingConfig.ThinkingLevel)
	}
	if body.Request.Labels.UsedClaude != "custom-bucket" || body.Request.Labels.UsedClaudeConservative != "custom-bucket" {
		t.Fatalf("usage label = %+v", body.Request.Labels)
	}
}

func TestCCATransportExtraBodyAndSSEObserver(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.events = []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}}`,
	}
	var observed []string
	options := &GoogleCloudCodeOptions{
		ExtraBody: map[string]any{"requestType": "agent"},
		OnSSEEvent: func(_ context.Context, event RawSSEEvent, _ *Model) error {
			observed = append(observed, event.Data)
			return nil
		},
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, GoogleCloudCode: options})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	if result, _ := ccaCollect(t, stream); result.StopReason != StopReasonStop {
		t.Fatalf("result = %+v", result)
	}
	if len(observed) != 1 || !strings.Contains(observed[0], "finishReason") {
		t.Fatalf("observed SSE events = %#v", observed)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if !strings.Contains(fixture.requests[0].body, `"requestType":"agent"`) {
		t.Fatalf("extra body not merged: %s", fixture.requests[0].body)
	}
}

func TestCCATransportAcceptsEmptyResponse(t *testing.T) {
	fixture := newCCAFixture(t)
	fixture.handler = func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: "+`{"response":{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}}`+"\n\n")
	}
	model := ccaTestModel(GoogleGeminiCliProviderID, "gemini-3.1-pro-preview", fixture.URL, true)
	// Without the option the empty stream is a failure.
	stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ := ccaCollect(t, stream)
	if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "empty response") {
		t.Fatalf("result without acceptEmptyResponse = %+v", result)
	}
	// With it, the response stands.
	options := &GoogleCloudCodeOptions{AcceptEmptyResponse: true}
	stream, err = StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential, GoogleCloudCode: options})
	if err != nil {
		t.Fatalf("stream setup: %v", err)
	}
	result, _ = ccaCollect(t, stream)
	if result.StopReason != StopReasonStop {
		t.Fatalf("result with acceptEmptyResponse = %+v", result)
	}
}

func TestCCACollapseTierFamiliesAndRouting(t *testing.T) {
	ids := []string{"gemini-3.8-flash-high", "gemini-3.8-flash-low", "gemini-3.8-flash-medium", "gemini-3.1-pro", "gemini-3.8-flash-tiered"}
	keep, families := ccaCollapseTierFamilies(ids)
	family, ok := families["gemini-3.8-flash"]
	if !ok {
		t.Fatalf("no family collapsed: %v", families)
	}
	if family.Tiers[ThinkingHigh] != "gemini-3.8-flash-high" || family.Tiers[ThinkingLow] != "gemini-3.8-flash-low" || family.Tiers[ThinkingMax] != "gemini-3.8-flash-tiered" {
		t.Fatalf("family tiers = %v", family.Tiers)
	}
	if family.Default != "gemini-3.8-flash-medium" {
		t.Fatalf("family default = %q", family.Default)
	}
	want := []string{"gemini-3.1-pro", "gemini-3.8-flash"}
	if len(keep) != len(want) || keep[0] != want[0] || keep[1] != want[1] {
		t.Fatalf("keep = %v, want %v", keep, want)
	}
	ccaRecordAccountFamily("acct", family)
	if wire, reviewed := ccaRouteWireID("acct", "gemini-3.8-flash", nil, ThinkingLow); wire != "gemini-3.8-flash-low" || !reviewed {
		t.Fatalf("routed wire = %q reviewed=%v", wire, reviewed)
	}
	if wire, reviewed := ccaRouteWireID("acct", "gemini-3.8-flash", nil, ""); wire != family.Default || !reviewed {
		t.Fatalf("default wire = %q reviewed=%v", wire, reviewed)
	}
	if wire, reviewed := ccaRouteWireID("other", "gemini-3.8-flash", nil, ThinkingLow); wire != "gemini-3.8-flash" || reviewed {
		t.Fatalf("cross-account wire = %q reviewed=%v", wire, reviewed)
	}
	if wire, _ := ccaRouteWireID("acct", "gemini-3.8-flash", &GoogleCloudCodeOptions{RequestModelID: "gemini-3.8-flash-tiered"}, ""); wire != "gemini-3.8-flash-tiered" {
		t.Fatalf("override wire = %q", wire)
	}
}

func TestCCAAccountWireProfilesAreIsolated(t *testing.T) {
	ccaRecordAccountProfile("account-a", "gemini-3.8-flash-low", ccaAntigravityWireProfile{ModelEnum: "MODEL_A", MaxOutputTokens: 1111})
	ccaRecordAccountProfile("account-b", "gemini-3.8-flash-low", ccaAntigravityWireProfile{ModelEnum: "MODEL_B", MaxOutputTokens: 2222})
	first, ok := ccaAccountProfile("account-a", "gemini-3.8-flash-low")
	if !ok || first.ModelEnum != "MODEL_A" || first.MaxOutputTokens != 1111 {
		t.Fatalf("account a profile = %+v", first)
	}
	second, ok := ccaAccountProfile("account-b", "gemini-3.8-flash-low")
	if !ok || second.ModelEnum != "MODEL_B" || second.MaxOutputTokens != 2222 {
		t.Fatalf("account b profile = %+v", second)
	}
	if _, ok := ccaAccountProfile("account-c", "gemini-3.8-flash-low"); ok {
		t.Fatal("an unknown account must not inherit another account's constants")
	}
	if got := ccaWireProfileForAccount("account-a", "gemini-3.8-flash-low"); got.ModelEnum != "MODEL_A" {
		t.Fatalf("account-scoped lookup = %+v", got)
	}
}

func TestCCAImageCapsForClaudeOnAntigravity(t *testing.T) {
	claude := ccaImageCapsFor(ccaIdentityAnthropic, true, 1)
	if claude.payload != int(float64(ccaAntigravityImagePayload)*ccaImagePayloadHeadroom) || claude.maxDimension != ccaMaxImageDimension {
		t.Fatalf("claude antigravity caps = %+v", claude)
	}
	many := ccaImageCapsFor(ccaIdentityAnthropic, true, ccaManyImageThreshold+1)
	if many.maxDimension != ccaManyImageMaxDimension {
		t.Fatalf("many-image caps = %+v", many)
	}
	// Gemini on the same gateway keeps the larger contract.
	gemini := ccaImageCapsFor(ccaIdentityGemini, true, 1)
	if gemini.payload != int(float64(ccaMaxImagePayloadBytes)*ccaImagePayloadHeadroom) {
		t.Fatalf("gemini caps = %+v", gemini)
	}
}
