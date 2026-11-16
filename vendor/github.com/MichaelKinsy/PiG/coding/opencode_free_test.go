package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

type openCodeFreeTransport struct {
	requests []*http.Request
	bodies   []map[string]any
	status   int
}

func (transport *openCodeFreeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	transport.requests = append(transport.requests, request)
	transport.bodies = append(transport.bodies, payload)
	status := transport.status
	if status == 0 {
		status = http.StatusOK
	}
	contentType := "text/event-stream"
	response := `data: {"choices":[{"index":0,"delta":{"content":"fixture answer"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
	if payload["model"] == "jev-1.13-free" {
		contentType = "application/json"
		response = `{"answers":{"correct":{"type":"noul","noul":0.99}}}`
	}
	if status != http.StatusOK {
		response = `{"error":{"message":"Free usage limit reached"}}`
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d", status), Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
}

func TestOpenCodeFreeNativeRequests(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { services.Close() })
	runtime := services.ModelRuntime()
	auth, err := runtime.CheckAuth(context.Background(), ai.OpenCodeFreeProviderID)
	if err != nil || auth == nil {
		t.Fatalf("keyless availability: auth=%v err=%v", auth, err)
	}
	for _, entry := range ai.ListModels(ai.OpenCodeFreeProviderID) {
		t.Run(entry.ID, func(t *testing.T) {
			model, err := BuildModel("opencode-free/"+entry.ID, services)
			if err != nil {
				t.Fatal(err)
			}
			if model.Capabilities.ContextWindow != entry.ContextWindow || !model.ProviderMeta.Reasoning {
				t.Fatalf("model metadata lost: %+v", model)
			}
			transport := &openCodeFreeTransport{}
			request := ai.Context{
				SystemPrompt: "Caller instructions remain authoritative.",
				Messages:     []ai.Message{ai.UserMessage{Content: ai.UserText("Hello"), Timestamp: 1}},
				Tools:        []ai.ToolSchema{{Name: "read", Description: "Read a file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}},
			}
			headers := ai.ProviderHeaders{"X-Fixture": new("preserved")}
			options := ai.StreamOptions{Fetch: &http.Client{Transport: transport}, Headers: headers, MaxRetries: new(0), Thinking: ai.ThinkingLevelHigh, MaxTokens: 512}
			for range 2 {
				result := runtime.StreamSimple(context.Background(), model, request, options).Result()
				if result == nil || result.StopReason != ai.StopReasonStop || result.ErrorMessage != "" {
					t.Fatalf("stream result: %+v", result)
				}
			}
			if len(transport.requests) != 2 {
				t.Fatalf("requests = %d, want 2", len(transport.requests))
			}
			var lastSession string
			for i, sent := range transport.requests {
				if sent.URL.String() != "https://opencode.ai/inference/openai/v1/chat/completions" || sent.Header.Get("Authorization") != "Bearer public" {
					t.Fatalf("request endpoint/auth: %s %q", sent.URL, sent.Header.Get("Authorization"))
				}
				if !strings.HasPrefix(sent.UserAgent(), "opencode/") || sent.Header.Get("X-Fixture") != "preserved" {
					t.Fatalf("headers: %v", sent.Header)
				}
				session := sent.Header.Get("X-Opencode-Session-Id")
				if !regexp.MustCompile(`^ses_[a-f0-9]{12}[a-zA-Z0-9]{14}$`).MatchString(session) || session == lastSession {
					t.Fatalf("session = %q, previous %q", session, lastSession)
				}
				for _, key := range []string{"X-Opencode-Session", "X-Session-Affinity", "X-Session-Id"} {
					if sent.Header.Get(key) != session {
						t.Fatalf("%s differs from session", key)
					}
				}
				lastSession = session
				body := transport.bodies[i]
				if body["model"] != model.ID || body["stream"] != true || body["max_completion_tokens"] != float64(512) {
					t.Fatalf("stream defaults: model=%v stream=%v tokens=%v", body["model"], body["stream"], body["max_completion_tokens"])
				}
				if *entry.Compat.SupportsReasoningEffort {
					if body["reasoning_effort"] != "high" {
						t.Fatalf("reasoning effort = %v, want high", body["reasoning_effort"])
					}
				} else if _, present := body["reasoning_effort"]; present {
					t.Fatal("unsupported reasoning_effort sent")
				}
				messages := body["messages"].([]any)
				system := messages[0].(map[string]any)["content"].(string)
				if !strings.Contains(system, "# Code Mode") || !strings.Contains(system, request.SystemPrompt) || !strings.Contains(system, "- Model ID: "+entry.ID) || !strings.Contains(system, "- Name: "+entry.DisplayName) || strings.Contains(system, "{{model_") {
					t.Fatal("compatibility preamble, selected identity or caller instructions missing")
				}
				tools := body["tools"].([]any)
				if len(tools) != 1 || tools[0].(map[string]any)["function"].(map[string]any)["name"] != "read" {
					t.Fatalf("caller tools changed: %v", tools)
				}
			}
			if len(headers) != 1 || request.SystemPrompt != "Caller instructions remain authoritative." {
				t.Fatal("provider mutated caller-owned options or context")
			}
			transport.status = http.StatusTooManyRequests
			result := runtime.StreamSimple(context.Background(), model, request, options).Result()
			if result == nil || result.StopReason != ai.StopReasonError || !strings.Contains(result.ErrorMessage, "Free usage limit reached") {
				t.Fatalf("quota error not propagated: %+v", result)
			}
			if len(transport.requests) != 3 {
				t.Fatalf("quota request retried despite MaxRetries=0: %d requests", len(transport.requests))
			}
		})
	}
	t.Run("jev judge", func(t *testing.T) {
		anyModel := runtime.GetModelOfType(ai.ModelTypeClassifier, ai.OpenCodeFreeProviderID, "jev-1.13-free")
		model, ok := anyModel.(*ai.ClassifierModel)
		if !ok {
			t.Fatalf("Jev is not a judge model: %T", anyModel)
		}
		if model.Cost.Input != 0 || model.Cost.Output != 0 || model.ContextWindow != 32000 {
			t.Fatalf("Jev metadata: %+v", model)
		}
		transport := &openCodeFreeTransport{}
		request := ai.ClassifierContext{State: ai.JsonObject{"answer": 4}, Questions: ai.ClassifierQuestions{{ID: "correct", Question: ai.ClassifierBoolQuestion{Instructions: "Is the answer to 2 + 2 correct?", Criteria: ai.ClassifierBoolCriteria{True: "Correct", False: "Incorrect"}}}}}
		options := ai.ModelsClassifierOptions{ClassifierOptions: ai.ClassifierOptions{Fetch: &http.Client{Transport: transport}, MaxRetries: new(0)}}
		result := runtime.Classify(context.Background(), model, request, options)
		if result.StopReason != ai.ClassifierStopReasonStop || len(result.Answers) != 1 {
			t.Fatalf("judge result: %+v", result)
		}
		answer, ok := result.Answers[0].Answer.(ai.ClassifierBoolAnswer)
		if !ok || answer.Probability != 0.99 {
			t.Fatalf("judge answer: %+v", result.Answers)
		}
		if len(transport.requests) != 1 {
			t.Fatalf("judge requests = %d", len(transport.requests))
		}
		sent, body := transport.requests[0], transport.bodies[0]
		if sent.URL.String() != "https://opencode.ai/inference/systemone/v1/systemone" || sent.Header.Get("Authorization") != "Bearer public" || !strings.HasPrefix(sent.UserAgent(), "opencode/") {
			t.Fatalf("judge request URL/headers: %s %v", sent.URL, sent.Header)
		}
		if body["model"] != model.ID || body["state"] == nil || body["questions"] == nil || body["messages"] != nil {
			t.Fatalf("judge payload is not System One: %v", body)
		}
		transport.status = http.StatusTooManyRequests
		result = runtime.Classify(context.Background(), model, request, options)
		if result.StopReason != ai.ClassifierStopReasonError || !strings.Contains(result.ErrorMessage, "Free usage limit reached") {
			t.Fatalf("judge quota error not propagated: %+v", result)
		}
	})
}
