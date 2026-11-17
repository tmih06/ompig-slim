package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

const geminiFixtureKey = "gemini-fixture-key"

func geminiTestModels(t *testing.T, baseURL string) (*ai.Models, *ai.ClassifierModel) {
	t.Helper()
	authContext := ai.AuthContext{
		Env: func(name string) (string, bool) {
			if name == "GEMINI_API_KEY" {
				return geminiFixtureKey, true
			}
			return "", false
		},
		FileExists: func(string) bool { return false },
	}
	models := ai.BuiltinModels(ai.CreateModelsOptions{AuthContext: &authContext})
	model, ok := models.GetModelOfType(ai.ModelTypeClassifier, "google", "gemini-decision-1").(*ai.ClassifierModel)
	if !ok {
		t.Fatal("Gemini Decision 1 is not registered as a Google classifier")
	}
	model.BaseURL = baseURL
	return models, model
}

func geminiFixtureResult() string {
	return `{"answers":{"correct":{"type":"noul","noul":0.95,"confidence":0.95},"route":{"type":"choice","choice":"2","probabilities":{"10":0.2,"2":0.6,"é":0.1,"review option":0.1},"confidence":0.6},"quality":{"type":"score","score":1.4,"confidence":0.8,"probabilities":{"0":0.1,"1":0.4,"2":0.5}},"plain":{"type":"noul","noul":0.6}},"usage":{"total_input_tokens":100,"total_output_tokens":0,"total_tokens":999}}`
}

func geminiRequest() ai.ClassifierContext {
	return ai.ClassifierContext{
		State: ai.JsonObject{"answer": 4, "expected": 4},
		Questions: ai.ClassifierQuestions{
			{ID: "correct", Question: ai.ClassifierBoolQuestion{Instructions: "Does the answer match?", Criteria: ai.ClassifierBoolCriteria{True: "Matches", False: "Does not match"}}},
			{ID: "route", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose a route.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "é", Description: "Unicode option"}, {Key: "10", Description: "Ten"}, {Key: "2", Description: "Two"}, {Key: "review option", Description: "A key with an internal space"}}}},
			{ID: "quality", Question: ai.ClassifierScoreQuestion{Instructions: "Rate the answer.", Criteria: []string{"Incorrect", "Partly correct", "Correct"}}},
			{ID: "plain", Question: ai.ClassifierBoolQuestion{Instructions: "Does this hold?"}},
		},
	}
}

func geminiAnswer(t *testing.T, result ai.ClassifierResult, id string) ai.ClassifierAnswer {
	t.Helper()
	for _, answer := range result.Answers {
		if answer.ID == id {
			return answer.Answer
		}
	}
	t.Fatalf("missing answer %q in %+v", id, result.Answers)
	return nil
}

func TestGeminiDecisionsCatalogAndClassification(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/decisions" || r.URL.RawQuery != "" {
			t.Errorf("request target = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("x-goog-api-key") != geminiFixtureKey || r.Header.Get("Authorization") != "" {
			t.Errorf("request authentication = x-goog-api-key %q, Authorization %q", r.Header.Get("x-goog-api-key"), r.Header.Get("Authorization"))
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if strings.Contains(string(body), geminiFixtureKey) {
			t.Error("API key appeared in request body")
		}
		var payload struct {
			Model     string                     `json:"model"`
			Input     json.RawMessage            `json:"input"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if payload.Model != "gemini-decision-1" {
			t.Errorf("model = %q", payload.Model)
		}
		var input string
		if err := json.Unmarshal(payload.Input, &input); err != nil {
			t.Errorf("input is not serialized state text: %s", payload.Input)
		} else {
			var state map[string]any
			if err := json.Unmarshal([]byte(input), &state); err != nil || state["answer"] != float64(4) || state["expected"] != float64(4) {
				t.Errorf("state input = %q (%v)", input, err)
			}
		}
		var boolQuestion map[string]json.RawMessage
		if err := json.Unmarshal(payload.Questions["correct"], &boolQuestion); err != nil {
			t.Errorf("decode bool question: %v", err)
		} else {
			var questionType string
			_ = json.Unmarshal(boolQuestion["type"], &questionType)
			var criteria map[string]string
			_ = json.Unmarshal(boolQuestion["criteria"], &criteria)
			if questionType != "noul" || criteria["true_description"] != "Matches" || criteria["false_description"] != "Does not match" || criteria["true"] != "" || criteria["false"] != "" {
				t.Errorf("bool wire question = %s", payload.Questions["correct"])
			}
		}
		var plainQuestion map[string]json.RawMessage
		_ = json.Unmarshal(payload.Questions["plain"], &plainQuestion)
		if _, exists := plainQuestion["criteria"]; exists {
			t.Errorf("empty bool criteria were not omitted: %s", payload.Questions["plain"])
		}
		var choiceQuestion map[string]json.RawMessage
		_ = json.Unmarshal(payload.Questions["route"], &choiceQuestion)
		var criteria map[string]string
		_ = json.Unmarshal(choiceQuestion["criteria"], &criteria)
		if criteria["é"] != "Unicode option" || criteria["review option"] != "A key with an internal space" {
			t.Errorf("choice criteria = %v", criteria)
		}
		var scoreQuestion map[string]json.RawMessage
		_ = json.Unmarshal(payload.Questions["quality"], &scoreQuestion)
		var levels []string
		_ = json.Unmarshal(scoreQuestion["criteria"], &levels)
		if len(levels) != 3 || levels[0] != "Incorrect" || levels[2] != "Correct" {
			t.Errorf("score criteria = %v", levels)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, geminiFixtureResult())
	}))
	defer server.Close()

	classifierCatalog := ai.GetBuiltinClassifierModels("google")
	matches := 0
	for _, model := range classifierCatalog {
		if model.ID == "gemini-decision-1" {
			matches++
			if model.API != ai.ClassifierAPI("gemini-decisions") || model.BaseURL != "https://generativelanguage.googleapis.com/v1beta" || model.ContextWindow != 262144 || model.Cost.Input != 0.1 || model.Cost.Output != 0 || len(model.Input) != 2 || model.Input[0] != "text" || model.Input[1] != "image" || model.Compat != nil {
				t.Fatalf("Gemini Decision 1 metadata = %+v", model)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("Google catalog has %d Gemini Decision 1 classifiers, want one", matches)
	}
	if ai.GetBuiltinClassifierModel("google", "gemini-decision-1") == nil {
		t.Fatal("Gemini Decision 1 is missing from the built-in classifier lookup")
	}
	if _, ok := ai.BuiltinClassifiers("google")[ai.ClassifierAPI("gemini-decisions")]; !ok {
		t.Fatal("Google provider does not register Gemini Decisions")
	}
	if _, ok := ai.BuiltinClassifiers("google-vertex")[ai.ClassifierAPI("gemini-decisions")]; ok {
		t.Fatal("Gemini Decisions was registered for Google Vertex")
	}

	models, model := geminiTestModels(t, server.URL+"/v1beta")
	if models.GetModelOfType(ai.ModelTypeChat, "google", model.ID) != nil {
		t.Fatal("Gemini Decision 1 was exposed as a chat model")
	}
	zeroRetries := 0
	result := models.Classify(context.Background(), model, geminiRequest(), ai.ModelsClassifierOptions{ClassifierOptions: ai.ClassifierOptions{MaxRetries: &zeroRetries}})
	if result.StopReason != ai.ClassifierStopReasonStop || result.Provider != "google" || result.Model != model.ID || result.API != ai.ClassifierAPI("gemini-decisions") || len(result.Answers) != 4 {
		t.Fatalf("classification result = %+v", result)
	}
	if answer, ok := geminiAnswer(t, result, "correct").(ai.ClassifierBoolAnswer); !ok || answer.Probability != 0.95 {
		t.Fatalf("bool answer = %#v", geminiAnswer(t, result, "correct"))
	}
	choice, ok := geminiAnswer(t, result, "route").(ai.ClassifierChoiceAnswer)
	if !ok || choice.Choice != "2" || choice.Confidence != 0.6 || len(choice.Probabilities) != 4 {
		t.Fatalf("choice answer = %#v", geminiAnswer(t, result, "route"))
	}
	for i, key := range []string{"é", "10", "2", "review option"} {
		if choice.Probabilities[i].Key != key {
			t.Fatalf("choice probability order = %+v", choice.Probabilities)
		}
	}
	if answer, ok := geminiAnswer(t, result, "quality").(ai.ClassifierScoreAnswer); !ok || answer.Score != 1.4 || answer.Confidence != 0.8 {
		t.Fatalf("score answer = %#v", geminiAnswer(t, result, "quality"))
	}
	if answer, ok := geminiAnswer(t, result, "plain").(ai.ClassifierBoolAnswer); !ok || answer.Probability != 0.6 {
		t.Fatalf("bool answer without criteria = %#v", geminiAnswer(t, result, "plain"))
	}
	if result.Usage == nil || result.Usage.Input != 100 || result.Usage.Output != 0 || result.Usage.TotalTokens != 100 || math.Abs(result.Usage.Cost.Total-0.00001) > 1e-12 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestGeminiDecisionsImageInput(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1beta/decisions" || r.Header.Get("x-goog-api-key") != geminiFixtureKey || r.Header.Get("Authorization") != "" {
			t.Errorf("image request target or auth = %s %v", r.URL, r.Header)
		}
		var payload struct {
			Input []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Data     string `json:"data"`
				MIMEType string `json:"mime_type"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode image request: %v", err)
		}
		if len(payload.Input) != 3 || payload.Input[0].Type != "text" || payload.Input[0].Text != "{}" || payload.Input[1].Type != "image" || payload.Input[1].Data != "aGVsbG8=" || payload.Input[1].MIMEType != "image/png" || payload.Input[2].Type != "image" || payload.Input[2].Data != "d29ybGQ=" || payload.Input[2].MIMEType != "image/jpeg" {
			t.Errorf("image input blocks = %+v", payload.Input)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"answers":{"visible":{"type":"noul","noul":0.9}}}`)
	}))
	defer server.Close()
	models, model := geminiTestModels(t, server.URL+"/v1beta")
	result := models.Classify(context.Background(), model, ai.ClassifierContext{
		Images:    []ai.ImageContent{{Data: "aGVsbG8=", MimeType: "image/png"}, {Data: "d29ybGQ=", MimeType: "image/jpeg"}},
		Questions: ai.ClassifierQuestions{{ID: "visible", Question: ai.ClassifierBoolQuestion{Instructions: "Is the image visible?"}}},
	})
	if result.StopReason != ai.ClassifierStopReasonStop || requests.Load() != 1 {
		t.Fatalf("image result = %+v, requests = %d", result, requests.Load())
	}
}

func TestGeminiDecisionsRetainsUsageAndRejectsMalformedAnswers(t *testing.T) {
	tests := []struct {
		name    string
		request ai.ClassifierContext
		answers string
	}{
		{
			name:    "missing answer",
			request: ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check it."}}}},
			answers: `{"answers":{},"usage":{"total_input_tokens":11,"total_output_tokens":0}}`,
		},
		{
			name:    "out of range bool probability",
			request: ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check it."}}}},
			answers: `{"answers":{"check":{"type":"noul","noul":1.2}},"usage":{"total_input_tokens":11,"total_output_tokens":0}}`,
		},
		{
			name:    "missing choice probability",
			request: ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "a"}, {Key: "b"}}}}}},
			answers: `{"answers":{"check":{"type":"choice","choice":"a","probabilities":{"a":0.8},"confidence":0.8}},"usage":{"total_input_tokens":11,"total_output_tokens":0}}`,
		},
		{
			name:    "unknown selected choice",
			request: ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "a"}, {Key: "b"}}}}}},
			answers: `{"answers":{"check":{"type":"choice","choice":"other","probabilities":{"a":0.8,"b":0.2},"confidence":0.8}},"usage":{"total_input_tokens":11,"total_output_tokens":0}}`,
		},
		{
			name:    "score outside rubric",
			request: ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierScoreQuestion{Instructions: "Rate.", Criteria: []string{"low", "high"}}}}},
			answers: `{"answers":{"check":{"type":"score","score":2,"confidence":0.8}},"usage":{"total_input_tokens":11,"total_output_tokens":0}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.answers)
			}))
			defer server.Close()
			models, model := geminiTestModels(t, server.URL+"/v1beta")
			result := models.Classify(context.Background(), model, test.request)
			if result.StopReason != ai.ClassifierStopReasonError || len(result.Answers) != 0 || result.ErrorMessage == "" {
				t.Fatalf("malformed response result = %+v", result)
			}
			if result.Usage == nil || result.Usage.Input != 11 || result.Usage.Output != 0 || result.Usage.TotalTokens != 11 {
				t.Fatalf("usage was not retained before answer failure: %+v", result.Usage)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests = %d, want 1", requests.Load())
			}
		})
	}
}

func TestGeminiDecisionsDoesNotRetrySafetyErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"safety","message":"blocked by safety policy"}}`)
	}))
	defer server.Close()
	models, model := geminiTestModels(t, server.URL+"/v1beta")
	retries := 2
	request := ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check."}}}}
	result := models.Classify(context.Background(), model, request, ai.ModelsClassifierOptions{ClassifierOptions: ai.ClassifierOptions{MaxRetries: &retries}})
	if result.StopReason != ai.ClassifierStopReasonError || result.ErrorMessage == "" || requests.Load() != 1 {
		t.Fatalf("safety error result = %+v, requests = %d", result, requests.Load())
	}
}

func TestGeminiDecisionsRejectsInvalidRequestsBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"answers":{"check":{"type":"noul","noul":0.8}}}`)
	}))
	defer server.Close()
	models, model := geminiTestModels(t, server.URL+"/v1beta")
	validBool := ai.ClassifierBoolQuestion{Instructions: "Check."}
	nilBool := (*ai.ClassifierBoolQuestion)(nil)
	tooMany := make(ai.ClassifierQuestions, 1001)
	for i := range tooMany {
		tooMany[i] = ai.ClassifierQuestionEntry{ID: "q-" + strconv.Itoa(i), Question: validBool}
	}
	tests := []struct {
		name      string
		questions ai.ClassifierQuestions
	}{
		{name: "no questions"},
		{name: "invalid ID", questions: ai.ClassifierQuestions{{ID: "bad.id", Question: validBool}}},
		{name: "duplicate IDs", questions: ai.ClassifierQuestions{{ID: "same", Question: validBool}, {ID: "same", Question: validBool}}},
		{name: "typed nil question", questions: ai.ClassifierQuestions{{ID: "check", Question: nilBool}}},
		{name: "one choice option", questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "only"}}}}}},
		{name: "unrepresentable option key", questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "ok"}, {Key: string([]byte{0xff})}}}}}},
		{name: "duplicate choice option", questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierChoiceQuestion{Instructions: "Choose.", Criteria: []ai.ClassifierChoiceCriterion{{Key: "same"}, {Key: "same"}}}}}},
		{name: "one score level", questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierScoreQuestion{Instructions: "Rate.", Criteria: []string{"low"}}}}},
		{name: "too many questions", questions: tooMany},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := models.Classify(context.Background(), model, ai.ClassifierContext{Questions: test.questions})
			if result.StopReason != ai.ClassifierStopReasonError || len(result.Answers) != 0 || result.ErrorMessage == "" {
				t.Fatalf("invalid request result = %+v", result)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid requests reached HTTP: %d", requests.Load())
	}
}

func TestGeminiDecisionsRetryHooksAndHeaderOverrides(t *testing.T) {
	var requests, payloadHooks, responseHooks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := requests.Add(1)
		if r.Header.Get("X-GoOg-ApI-KeY") != "header-override" || r.Header.Get("Authorization") != "" {
			t.Errorf("merged authentication = %v", r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		var payload struct {
			Input string `json:"input"`
		}
		_ = json.Unmarshal(body, &payload)
		if payload.Input != "hook replacement" {
			t.Errorf("OnPayload replacement was not sent: %s", body)
		}
		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"retry fixture"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"answers":{"check":{"type":"noul","noul":0.8}}}`)
	}))
	defer server.Close()
	models, model := geminiTestModels(t, server.URL+"/v1beta")
	model.Headers = map[string]string{"x-goog-api-key": "model-header"}
	zeroDelay := 0
	oneRetry := 1
	options := ai.ModelsClassifierOptions{
		ClassifierOptions: ai.ClassifierOptions{
			MaxRetries:      &oneRetry,
			MaxRetryDelayMs: &zeroDelay,
			Headers:         ai.ProviderHeaders{"X-GOOG-Api-Key": ai.ProviderHeader("header-override")},
			OnPayload: func(payload any, requestModel ai.ClassifierModel) (any, bool, error) {
				payloadHooks.Add(1)
				if requestModel.ID != model.ID {
					t.Errorf("OnPayload model = %+v", requestModel)
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					return nil, false, err
				}
				var original map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &original); err != nil || original["questions"] == nil {
					t.Errorf("OnPayload did not see Gemini questions: %s (%v)", encoded, err)
				}
				return map[string]any{"model": requestModel.ID, "input": "hook replacement", "questions": map[string]any{"check": map[string]any{"type": "noul", "instructions": "Check."}}}, true, nil
			},
			OnResponse: func(response ai.ProviderResponse, responseModel ai.ClassifierModel) error {
				responseHooks.Add(1)
				if response.Status != http.StatusOK || responseModel.ID != model.ID {
					t.Errorf("OnResponse = %+v for %+v", response, responseModel)
				}
				return nil
			},
		},
	}
	request := ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check."}}}}
	result := models.Classify(context.Background(), model, request, options)
	if result.StopReason != ai.ClassifierStopReasonStop || requests.Load() != 2 || payloadHooks.Load() != 1 || responseHooks.Load() != 1 {
		t.Fatalf("retry and hook result = %+v, requests=%d payload hooks=%d response hooks=%d", result, requests.Load(), payloadHooks.Load(), responseHooks.Load())
	}
	if answer, ok := geminiAnswer(t, result, "check").(ai.ClassifierBoolAnswer); !ok || answer.Probability != 0.8 {
		t.Fatalf("retry answer = %#v", geminiAnswer(t, result, "check"))
	}
}

func TestGeminiDecisionsRespectsHeaderRemoval(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("x-goog-api-key") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("removed Google auth header was restored: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"answers":{"check":{"type":"noul","noul":0.8}}}`)
	}))
	defer server.Close()
	models, model := geminiTestModels(t, server.URL+"/v1beta")
	result := models.Classify(context.Background(), model, ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check."}}}}, ai.ModelsClassifierOptions{ClassifierOptions: ai.ClassifierOptions{Headers: ai.ProviderHeaders{"X-Goog-Api-Key": nil}}})
	if result.StopReason != ai.ClassifierStopReasonStop || requests.Load() != 1 {
		t.Fatalf("header removal result = %+v, requests = %d", result, requests.Load())
	}
}

func TestGeminiDecisionsTimeoutRetryAndCancellation(t *testing.T) {
	t.Run("attempt timeout retries", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if requests.Add(1) == 1 {
				time.Sleep(100 * time.Millisecond)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"answers":{"check":{"type":"noul","noul":0.8}}}`)
		}))
		defer server.Close()
		models, model := geminiTestModels(t, server.URL+"/v1beta")
		timeoutMs, retries := 30, 1
		result := models.Classify(context.Background(), model, ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check."}}}}, ai.ModelsClassifierOptions{ClassifierOptions: ai.ClassifierOptions{TimeoutMs: &timeoutMs, MaxRetries: &retries}})
		if result.StopReason != ai.ClassifierStopReasonStop || requests.Load() != 2 {
			t.Fatalf("timeout retry result = %+v, requests = %d", result, requests.Load())
		}
	})

	t.Run("caller cancellation", func(t *testing.T) {
		var requests atomic.Int32
		entered := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			close(entered)
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"answers":{"check":{"type":"noul","noul":0.8}}}`)
		}))
		defer server.Close()
		models, model := geminiTestModels(t, server.URL+"/v1beta")
		request := ai.ClassifierContext{Questions: ai.ClassifierQuestions{{ID: "check", Question: ai.ClassifierBoolQuestion{Instructions: "Check."}}}}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan ai.ClassifierResult, 1)
		go func() { done <- models.Classify(ctx, model, request) }()
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("classifier did not reach the loopback server")
		}
		cancel()
		select {
		case result := <-done:
			if result.StopReason != ai.ClassifierStopReasonAborted || len(result.Answers) != 0 || requests.Load() != 1 {
				t.Fatalf("cancellation result = %+v, requests = %d", result, requests.Load())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("classification did not stop after cancellation")
		}
	})
}

func TestPostClassifierRequestKeepsBearerAuthentication(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-bearer" || r.Header.Get("x-goog-api-key") != "" {
			t.Errorf("shared classifier request auth = %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	zeroRetries := 0
	response, err := ai.PostClassifierRequest(context.Background(), "Bearer fixture", server.URL+"/classify", ai.ClassifierModel{ID: "fixture", Provider: "fixture", BaseURL: server.URL}, map[string]any{"input": "fixture"}, ai.ClassifierOptions{APIKey: "fixture-bearer", MaxRetries: &zeroRetries}, nil)
	if err != nil || requests.Load() != 1 {
		t.Fatalf("PostClassifierRequest: response=%s err=%v requests=%d", response, err, requests.Load())
	}
	var decoded map[string]bool
	if err := json.Unmarshal(response, &decoded); err != nil || !decoded["ok"] {
		t.Fatalf("response = %s (%v)", response, err)
	}
}
