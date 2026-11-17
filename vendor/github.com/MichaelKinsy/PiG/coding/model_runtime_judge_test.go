package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

func judgeTestServices(t *testing.T, providers map[string]any, role string, fallbacks []string) *AgentSessionServices {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	for _, name := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "TYPESAFE_API_KEY"} {
		t.Setenv(name, "")
	}
	dir := t.TempDir()
	catalog, err := json.Marshal(map[string]any{"providers": providers})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.json"), catalog, 0600); err != nil {
		t.Fatal(err)
	}
	settings := NewInMemorySettingsManager(Settings{
		Config: modelroles.Config{ModelRoles: map[string]string{"judge": role}},
		Retry:  &icodingagent.RetrySettingsJSON{FallbackChains: map[string][]string{"judge": fallbacks}},
	})
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: dir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { services.Close() })
	return services
}

func judgeQuestionSet() ai.ClassifierContext {
	return ai.ClassifierContext{
		State: ai.JsonObject{"answer": "4", "expected": "4"},
		Questions: ai.ClassifierQuestions{
			{ID: "correct", Question: ai.ClassifierBoolQuestion{Instructions: "Does the answer match?", Criteria: ai.ClassifierBoolCriteria{True: "Matches", False: "Does not match"}}},
			{ID: "route", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose a route.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "accept", Description: "Accept it"}, {Key: "review", Description: "Review it"}}}},
			{ID: "quality", Question: ai.ClassifierScoreQuestion{Instructions: "Rate the answer.", Criteria: []string{"Incorrect", "Partly correct", "Correct"}}},
		},
	}
}

func nativeJudgeServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/decisions" || r.Header.Get("x-goog-api-key") != "judge-fixture-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("native judge request path/auth: %s %v", r.URL, r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(string(body), "judge-fixture-key") {
			t.Error("fixture key appeared in native judge request body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
}

func chatJudgeServer(t *testing.T, response string, usage string, onRequest func(map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode chat request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if onRequest != nil {
			onRequest(payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":null}]}\n\n", mustJSON(t, response))
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":%s}\n\n", usage)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestModelRuntimeJudgeUsesConfiguredNativeClassifier(t *testing.T) {
	server := nativeJudgeServer(t, `{"answers":{"correct":{"type":"noul","noul":0.8,"confidence":0.9},"route":{"type":"choice","choice":"accept","probabilities":{"accept":0.8,"review":0.2},"confidence":0.8},"quality":{"type":"score","score":1.5,"confidence":0.7}},"usage":{"total_input_tokens":4,"total_output_tokens":2,"total_tokens":6}}`)
	defer server.Close()
	services := judgeTestServices(t, map[string]any{
		"google": map[string]any{"baseUrl": server.URL + "/v1beta", "apiKey": "judge-fixture-key"},
	}, "google/gemini-decision-1", nil)

	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonStop || result.Provider != "google" || result.Model != "gemini-decision-1" {
		t.Fatalf("judge result identity/stop: %+v", result)
	}
	if got := result.Answers["correct"].(ai.JudgeBoolAnswer); !got.Value || got.Probability == nil || *got.Probability != 0.8 {
		t.Fatalf("bool answer: %+v", got)
	}
	choice := result.Answers["route"].(ai.JudgeChoiceAnswer)
	if choice.Value != "accept" || choice.Classifier == nil || choice.Classifier.Confidence != 0.8 || len(choice.Classifier.Probabilities) != 2 {
		t.Fatalf("choice answer: %+v", choice)
	}
	score := result.Answers["quality"].(ai.JudgeScoreAnswer)
	if score.Value != 1.5 || score.Classifier == nil || score.Classifier.Confidence != 0.7 {
		t.Fatalf("score answer: %+v", score)
	}
	if result.Usage == nil || result.Usage.Input != 4 || result.Usage.Output != 2 {
		t.Fatalf("native usage: %+v", result.Usage)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	answers := wire["answers"].(map[string]any)
	boolAnswer := answers["correct"].(map[string]any)
	if boolAnswer["type"] != "bool" || boolAnswer["value"] != true || boolAnswer["classifier"].(map[string]any)["probability"] != 0.8 {
		t.Fatalf("native bool JSON: %s", encoded)
	}
}

func TestModelRuntimeJudgeFallsBackAndAggregatesUsage(t *testing.T) {
	native := nativeJudgeServer(t, `{"answers":{},"usage":{"total_input_tokens":4,"total_output_tokens":2,"total_tokens":6}}`)
	defer native.Close()
	var chatModel atomic.Value
	chat := chatJudgeServer(t, `{"correct":true,"route":"accept","quality":2}`, `{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}`, func(payload map[string]any) {
		chatModel.Store(payload["model"])
		messages, _ := payload["messages"].([]any)
		tools, _ := payload["tools"].([]any)
		if len(messages) != 2 || len(tools) != 0 {
			t.Errorf("judge chat included history or tools: %+v", payload)
			return
		}
		if !strings.Contains(messages[0].(map[string]any)["content"].(string), "untrusted data") {
			t.Errorf("missing state-data instruction: %+v", messages[0])
		}
		if payload["reasoning_effort"] != nil || payload["reasoning"] != nil {
			t.Errorf("judge chat default effort was not off: %+v", payload)
		}
		var user string
		switch content := messages[1].(map[string]any)["content"].(type) {
		case string:
			user = content
		case []any:
			user = content[0].(map[string]any)["text"].(string)
		default:
			t.Errorf("unexpected judge user content: %#v", content)
			return
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(user), &input); err != nil || input["state"].(map[string]any)["answer"] != "4" {
			t.Errorf("chat did not receive judge state as JSON data: %s (%v)", user, err)
		}
	})
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"google":     map[string]any{"baseUrl": native.URL + "/v1beta", "apiKey": "judge-fixture-key"},
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{map[string]any{"id": "fallback", "input": []string{"text"}}}},
	}, "google/gemini-decision-1", []string{"judge-chat/fallback"})

	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	servedModel, _ := chatModel.Load().(string)
	if result.StopReason != ai.JudgeStopReasonStop || result.Provider != "judge-chat" || result.Model != "fallback" || servedModel != "fallback" {
		t.Fatalf("fallback result: %+v, chat model=%q", result, servedModel)
	}
	if result.Usage == nil || result.Usage.Input != 7 || result.Usage.Output != 3 || result.Usage.TotalTokens != 10 {
		t.Fatalf("combined usage: %+v", result.Usage)
	}
	if got := result.Answers["correct"].(ai.JudgeBoolAnswer); !got.Value || got.Probability != nil {
		t.Fatalf("chat bool answer fabricated classifier metadata: %+v", got)
	}
	if got := result.Answers["route"].(ai.JudgeChoiceAnswer); got.Value != "accept" || got.Classifier != nil {
		t.Fatalf("chat choice answer fabricated classifier metadata: %+v", got)
	}
}

func TestModelRuntimeJudgeTriesNestedFallbacksPastFour(t *testing.T) {
	var mu sync.Mutex
	var requested []string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode chat request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		model, _ := payload["model"].(string)
		mu.Lock()
		requested = append(requested, model)
		mu.Unlock()
		answer := `{"correct":"not a boolean"}`
		if model == "j5" {
			answer = `{"correct":true,"route":"accept","quality":2}`
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":null}]}\n\n", mustJSON(t, answer))
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{
			map[string]any{"id": "j0"}, map[string]any{"id": "j1"}, map[string]any{"id": "j2"},
			map[string]any{"id": "j3"}, map[string]any{"id": "j4"}, map[string]any{"id": "j5"},
		}},
	}, "judge-chat/j0", []string{"judge-chat/j1", "@audit"})
	if err := services.SettingsManager().SetFallbackChain("audit", []string{"judge-chat/j2", "judge-chat/j3", "judge-chat/j4", "judge-chat/j5"}, "global"); err != nil {
		t.Fatal(err)
	}

	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonStop || result.Model != "j5" {
		t.Fatalf("judge did not reach the fifth fallback: %+v", result)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	want := []string{"j0", "j1", "j2", "j3", "j4", "j5"}
	if len(got) != len(want) {
		t.Fatalf("candidate requests: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate requests: %v, want %v", got, want)
		}
	}
	if result.Usage == nil || result.Usage.Input != 6 || result.Usage.Output != 6 || result.Usage.TotalTokens != 12 {
		t.Fatalf("usage from all failed candidates was not retained: %+v", result.Usage)
	}
}

func TestModelRuntimeJudgeUsesLiveRoleReassignment(t *testing.T) {
	var servedModel atomic.Value
	chat := chatJudgeServer(t, `{"correct":true,"route":"accept","quality":2}`, `{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`, func(payload map[string]any) {
		servedModel.Store(payload["model"])
	})
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{map[string]any{"id": "first"}, map[string]any{"id": "second"}}},
	}, "judge-chat/first", nil)

	first := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if first.StopReason != ai.JudgeStopReasonStop || first.Model != "first" || servedModel.Load() != "first" {
		t.Fatalf("initial judge role: result=%+v served=%v", first, servedModel.Load())
	}
	if err := services.SettingsManager().SetModelRole("judge", "judge-chat/second", "global"); err != nil {
		t.Fatal(err)
	}
	second := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if second.StopReason != ai.JudgeStopReasonStop || second.Model != "second" || servedModel.Load() != "second" {
		t.Fatalf("reassigned judge role: result=%+v served=%v", second, servedModel.Load())
	}
}

func TestModelRuntimeJudgeIncludesVirtualRoleModels(t *testing.T) {
	var servedModel atomic.Value
	chat := chatJudgeServer(t, `{"correct":true,"route":"accept","quality":2}`, `{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`, func(payload map[string]any) {
		servedModel.Store(payload["model"])
	})
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{map[string]any{"id": "physical"}}},
	}, "judge-chat/physical", nil)

	runtime := services.ModelRuntime()
	physical := runtime.GetModel("judge-chat", "physical")
	if physical == nil {
		t.Fatal("physical judge model is unavailable")
	}
	var routed atomic.Bool
	if err := runtime.RegisterVirtualModel(VirtualModelDefinition{
		Provider: "judge-chat", ID: "virtual", Route: func(_ context.Context, _ ModelRouteRequest) (ModelRoute, error) {
			routed.Store(true)
			return ModelRoute{Model: physical, ThinkingLevel: ai.ThinkingOff}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := services.SettingsManager().SetModelRole("judge", "judge-chat/virtual", "global"); err != nil {
		t.Fatal(err)
	}

	result := runtime.Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonStop || result.Provider != "judge-chat" || result.Model != "physical" || servedModel.Load() != "physical" || !routed.Load() {
		t.Fatalf("virtual judge role did not route to its physical model: result=%+v served=%v routed=%v", result, servedModel.Load(), routed.Load())
	}
}

func TestModelRuntimeJudgePreservesUpstreamAndThinkingEffort(t *testing.T) {
	chat := chatJudgeServer(t, `{"correct":true,"route":"accept","quality":2}`, `{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`, func(payload map[string]any) {
		reasoning, _ := payload["reasoning"].(map[string]any)
		if payload["model"] != "route-model" || reasoning["effort"] != "high" {
			t.Errorf("judge route or effort was lost: %+v", payload)
		}
		provider, _ := payload["provider"].(map[string]any)
		only, _ := provider["only"].([]any)
		if len(only) != 1 || only[0] != "fireworks" {
			t.Errorf("judge upstream route was lost: %+v", payload)
		}
	})
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"openrouter": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{
			map[string]any{"id": "route-model", "reasoning": true, "compat": map[string]any{"supportsReasoningEffort": true}},
		}},
	}, "openrouter/route-model@fireworks:high", nil)

	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonStop || result.Provider != "openrouter" || result.Model != "route-model" {
		t.Fatalf("routed judge result: %+v", result)
	}
}

func TestModelRuntimeJudgeRejectsUnsupportedNativeRoute(t *testing.T) {
	services := judgeTestServices(t, map[string]any{
		"google": map[string]any{"baseUrl": "http://127.0.0.1:1/v1beta", "apiKey": "judge-fixture-key"},
	}, "google/gemini-decision-1@fireworks", nil)

	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonError || !strings.Contains(result.ErrorMessage, "does not support @upstream routing") {
		t.Fatalf("unsupported native route was not reported: %+v", result)
	}
}

func TestModelRuntimeJudgeSkipsImageIncompatibleCandidates(t *testing.T) {
	var requestedModel atomic.Value
	chat := chatJudgeServer(t, `{"correct":true,"route":"accept","quality":2}`, `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}`, func(payload map[string]any) {
		requestedModel.Store(payload["model"])
		messages := payload["messages"].([]any)
		content := messages[1].(map[string]any)["content"].([]any)
		if len(content) != 2 {
			t.Errorf("judge image was not forwarded: %+v", content)
			return
		}
		image := content[1].(map[string]any)
		imageURL := image["image_url"].(map[string]any)["url"]
		if image["type"] != "image_url" || imageURL != "data:image/png;base64,aW1hZ2U=" {
			t.Errorf("judge image was not forwarded: %+v", image)
		}
	})
	defer chat.Close()
	providers := map[string]any{
		"vision-fixture": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{
			map[string]any{"id": "text-only", "input": []string{"text"}},
			map[string]any{"id": "vision", "input": []string{"text", "image"}},
		}},
	}
	services := judgeTestServices(t, providers, "vision-fixture/text-only", []string{"vision-fixture/vision"})
	request := judgeQuestionSet()
	request.Images = []ai.ImageContent{{Data: "aW1hZ2U=", MimeType: "image/png"}}
	result := services.ModelRuntime().Judge(context.Background(), request)
	servedModel, _ := requestedModel.Load().(string)
	if result.StopReason != ai.JudgeStopReasonStop || result.Model != "vision" || servedModel != "vision" {
		t.Fatalf("image-compatible fallback: %+v, requested=%q", result, servedModel)
	}
}

func TestModelRuntimeJudgeRejectsMalformedChatAnswers(t *testing.T) {
	chat := chatJudgeServer(t, `{"correct":"yes","route":"accept","quality":2}`, `{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`, nil)
	defer chat.Close()
	services := judgeTestServices(t, map[string]any{
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{map[string]any{"id": "only"}}},
	}, "judge-chat/only", nil)
	result := services.ModelRuntime().Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonError || !strings.Contains(result.ErrorMessage, "must be a boolean") {
		t.Fatalf("malformed answer accepted: %+v", result)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 2 {
		t.Fatalf("failed attempt usage missing: %+v", result.Usage)
	}
}

func TestModelRuntimeJudgeCancellationStopsFallback(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var calls atomic.Int32
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Error(err)
		}
		startOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		chat.Close()
	}()
	services := judgeTestServices(t, map[string]any{
		"judge-chat": map[string]any{"api": "openai-completions", "baseUrl": chat.URL + "/v1", "apiKey": "chat-fixture-key", "models": []any{map[string]any{"id": "first"}, map[string]any{"id": "second"}}},
	}, "judge-chat/first", []string{"judge-chat/second"})
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan ai.JudgeResult, 1)
	go func() { resultCh <- services.ModelRuntime().Judge(ctx, judgeQuestionSet()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("judge request did not start")
	}
	cancel()
	select {
	case result := <-resultCh:
		if result.StopReason != ai.JudgeStopReasonAborted {
			t.Fatalf("cancelled judge result: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("judge did not return after cancellation")
	}
	if calls.Load() != 1 {
		t.Fatalf("cancellation started a fallback request: %d calls", calls.Load())
	}
}

func TestChatJudgeAnswersRejectsDuplicatesAndNonScalarValues(t *testing.T) {
	boolQuestions := ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check"}}}
	answers, err := chatJudgeAnswers(boolQuestions, []byte(`{"check":true}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(answers["check"])
	if err != nil {
		t.Fatal(err)
	}
	var answer map[string]any
	if err := json.Unmarshal(encoded, &answer); err != nil {
		t.Fatal(err)
	}
	if _, exists := answer["classifier"]; exists {
		t.Fatalf("chat answer fabricated classifier metadata: %s", encoded)
	}
	if _, err := chatJudgeAnswers(boolQuestions, []byte(`{"check":true,"check":false}`)); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate answer keys accepted: %v", err)
	}
	if _, err := chatJudgeAnswers(boolQuestions, []byte(`{"check":true} {}`)); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	if _, err := chatJudgeAnswers(boolQuestions, []byte(`{"check":null}`)); err == nil {
		t.Fatal("null bool answer was accepted")
	}
	scoreQuestions := ai.ClassifierQuestions{{ID: "score", Question: ai.ClassifierScoreQuestion{Instructions: "Rate", Criteria: []string{"low", "high"}}}}
	if _, err := chatJudgeAnswers(scoreQuestions, []byte(`{"score":null}`)); err == nil {
		t.Fatal("null score answer was accepted")
	}
}

func TestModelRuntimeJudgeNoCandidateError(t *testing.T) {
	var runtime *ModelRuntime
	result := runtime.Judge(context.Background(), judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonError || !strings.Contains(result.ErrorMessage, "Configure modelRoles.judge") || result.Answers == nil {
		t.Fatalf("no-candidate result is not actionable: %+v", result)
	}
}

func TestModelRuntimeJudgePreCanceledWithoutCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var runtime *ModelRuntime
	result := runtime.Judge(ctx, judgeQuestionSet())
	if result.StopReason != ai.JudgeStopReasonAborted || result.ErrorMessage != context.Canceled.Error() || result.Answers == nil {
		t.Fatalf("pre-cancelled judge result: %+v", result)
	}
}
