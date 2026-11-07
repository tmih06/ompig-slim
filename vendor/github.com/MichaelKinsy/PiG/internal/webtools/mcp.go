package webtools

// Minimal JSON-RPC 2.0 client for keyless MCP search endpoints.
//
// Ports packages/coding-agent/src/mcp/json-rpc.ts: one POST carrying
// {"jsonrpc":"2.0","id":…,"method":"tools/call"}, an Accept that allows either a
// JSON body or a text/event-stream body, and result selection by request id.
// Server notifications, server requests and responses for other ids do not
// satisfy the call; malformed messages fail it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// mcpRequestID is the JSON-RPC id of one call: a process-wide counter, so a
// response for another call's id is never accepted as this call's answer.
var mcpRequestID atomic.Int64

// mcpRPCError is a JSON-RPC error member.
type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// mcpEnvelope is a JSON-RPC message: a response carries exactly one of
// Result/Error, a request or notification carries Method.
type mcpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   *mcpRPCError    `json:"error"`
}

// mcpResult is a tools/call result as the keyless search paths read it.
type mcpResult struct {
	StructuredContent json.RawMessage `json:"structuredContent"`
	Content           []mcpContent    `json:"content"`
	IsError           bool            `json:"isError"`
}

// mcpContent is one content block of a tools/call result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callMCPTool posts a tools/call and returns its result payload. A JSON-RPC
// error or an isError result becomes an error whose text the chain quotes. The
// caller's context already carries the per-provider deadline.
func callMCPTool(ctx context.Context, client *http.Client, endpoint, provider, tool string, args map[string]any) (mcpResult, error) {
	return callMCPToolWithHeaders(ctx, client, endpoint, provider, tool, args, nil)
}

// callMCPToolWithHeaders is callMCPTool with extra request headers.
func callMCPToolWithHeaders(ctx context.Context, client *http.Client, endpoint, provider, tool string, args map[string]any, headers http.Header) (mcpResult, error) {
	if endpoint == "" {
		return mcpResult{}, &SearchError{Provider: provider, Message: provider + " MCP endpoint is configured empty"}
	}
	requestID := strconv.FormatInt(mcpRequestID.Add(1), 10)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return mcpResult{}, &SearchError{Provider: provider, Message: provider + " MCP request could not be encoded"}
	}

	requestHeaders := make(http.Header)
	requestHeaders.Set("Content-Type", "application/json")
	requestHeaders.Set("Accept", "application/json, text/event-stream")
	for name, values := range headers {
		for _, value := range values {
			requestHeaders.Add(name, value)
		}
	}

	response, err := Load(ctx, client, http.MethodPost, endpoint, body, requestHeaders, mcpMaxBytes)
	if err != nil {
		if ctx.Err() != nil {
			return mcpResult{}, ctx.Err()
		}
		return mcpResult{}, &SearchError{Provider: provider, Message: classifyProviderMessage(provider, err), Status: response.StatusCode}
	}

	envelope, err := readMCPResponse(response, requestID)
	if err != nil {
		return mcpResult{}, &SearchError{Provider: provider, Message: provider + " MCP response was not understood (" + err.Error() + ")"}
	}
	if envelope.Error != nil {
		message := normalizeSearchText(envelope.Error.Message)
		if message == "" {
			message = fmt.Sprintf("JSON-RPC error %d", envelope.Error.Code)
		}
		return mcpResult{}, &SearchError{Provider: provider, Message: provider + " MCP error: " + message}
	}

	var result mcpResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		// A server may answer with the payload itself rather than a wrapped
		// tools/call result; hand it back as structured content.
		result = mcpResult{StructuredContent: envelope.Result}
	}
	if result.IsError {
		message := ""
		for _, item := range result.Content {
			if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
				message = normalizeSearchText(item.Text)
				break
			}
		}
		if message == "" {
			message = provider + " MCP returned an error"
		}
		return mcpResult{}, &SearchError{Provider: provider, Message: truncateRunes(message, MaxFailureReasonRunes)}
	}
	return result, nil
}

const mcpMaxBytes int64 = 8 * 1024 * 1024

// readMCPResponse selects the response matching requestID from a JSON or SSE
// HTTP body. SSE events are read in order; "[DONE]" ends the stream.
func readMCPResponse(response HTTPResponse, requestID string) (mcpEnvelope, error) {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		var sawUnmatched bool
		for _, event := range sseDataEvents(string(response.Body)) {
			if event == "" {
				continue
			}
			if event == "[DONE]" {
				break
			}
			envelope, matched, err := decodeMCPPayload([]byte(event), requestID)
			if err != nil {
				return mcpEnvelope{}, err
			}
			if matched {
				return envelope, nil
			}
			sawUnmatched = true
		}
		if sawUnmatched {
			return mcpEnvelope{}, errors.New("response id did not match the request id")
		}
		return mcpEnvelope{}, errors.New("the event stream carried no response")
	}

	envelope, matched, err := decodeMCPPayload(response.Body, requestID)
	if err != nil {
		return mcpEnvelope{}, err
	}
	if !matched {
		return mcpEnvelope{}, errors.New("the response id did not match the request id")
	}
	return envelope, nil
}

// sseDataEvents extracts the data lines of each SSE event, joining a
// multi-line data field with newlines.
func sseDataEvents(body string) []string {
	events := make([]string, 0, 4)
	var current []string
	flush := func() {
		if len(current) > 0 {
			events = append(events, strings.Join(current, "\n"))
			current = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if data, ok := strings.CutPrefix(line, "data:"); ok {
			current = append(current, strings.TrimPrefix(data, " "))
		}
	}
	flush()
	return events
}

// decodeMCPPayload decodes one JSON payload, which may be a single message or a
// batch, and reports whether it holds the response to requestID.
func decodeMCPPayload(payload []byte, requestID string) (mcpEnvelope, bool, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return mcpEnvelope{}, false, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var batch []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &batch); err != nil {
			return mcpEnvelope{}, false, errors.New("malformed JSON-RPC batch")
		}
		for _, message := range batch {
			if envelope, matched, err := decodeMCPMessage(message, requestID); err != nil || matched {
				return envelope, matched, err
			}
		}
		return mcpEnvelope{}, false, nil
	}
	return decodeMCPMessage([]byte(trimmed), requestID)
}

// decodeMCPMessage validates one JSON-RPC message and matches its id.
func decodeMCPMessage(raw []byte, requestID string) (mcpEnvelope, bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return mcpEnvelope{}, false, errors.New("malformed JSON-RPC message")
	}
	version, ok := probe["jsonrpc"]
	if !ok || string(version) != `"2.0"` {
		return mcpEnvelope{}, false, errors.New("malformed JSON-RPC message")
	}
	_, hasResult := probe["result"]
	_, hasError := probe["error"]
	if _, isRequest := probe["method"]; isRequest {
		// A server request or notification is not the caller's response.
		if hasResult || hasError {
			return mcpEnvelope{}, false, errors.New("malformed JSON-RPC request")
		}
		return mcpEnvelope{}, false, nil
	}
	if hasResult == hasError {
		return mcpEnvelope{}, false, errors.New("malformed JSON-RPC response")
	}
	id, ok := probe["id"]
	if !ok {
		return mcpEnvelope{}, false, errors.New("malformed JSON-RPC response")
	}
	idText := strings.TrimSpace(string(id))
	matches := idText == requestID || idText == `"`+requestID+`"`
	if !matches {
		return mcpEnvelope{}, false, nil
	}
	var envelope mcpEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return mcpEnvelope{}, false, errors.New("malformed JSON-RPC response")
	}
	return envelope, true, nil
}

// classifyProviderMessage normalizes a transport error into a one-line reason.
func classifyProviderMessage(provider string, err error) string {
	message := normalizeSearchText(err.Error())
	message = strings.TrimSuffix(message, ".")
	if message == "" {
		message = provider + " request failed"
	}
	return truncateRunes(message, MaxFailureReasonRunes)
}

// mcpToolText returns the first non-empty text block of a tools/call result.
func mcpToolText(result mcpResult) string {
	for _, item := range result.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

// mcpJSONPayload decodes the first JSON payload a tools/call result carries:
// structuredContent, then a JSON text block.
func mcpJSONPayload(result mcpResult) (json.RawMessage, bool) {
	if len(result.StructuredContent) > 0 && string(result.StructuredContent) != "null" {
		return result.StructuredContent, true
	}
	for _, item := range result.Content {
		if item.Type != "text" {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			if json.Valid([]byte(text)) {
				return json.RawMessage(text), true
			}
		}
	}
	return nil, false
}
