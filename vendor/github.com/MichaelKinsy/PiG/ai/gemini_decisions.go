package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

const (
	ClassifierAPIGeminiDecisions ClassifierAPI = "gemini-decisions"
	geminiDecisionsLabel                       = "Gemini Decisions"
)

func init() {
	for i := range GeneratedClassifierModels {
		if GeneratedClassifierModels[i].Provider == "google" && GeneratedClassifierModels[i].ID == "gemini-decision-1" {
			return
		}
	}
	GeneratedClassifierModels = append(GeneratedClassifierModels, ClassifierModel{
		ID:            "gemini-decision-1",
		Name:          "Gemini Decision 1",
		API:           ClassifierAPIGeminiDecisions,
		Provider:      "google",
		BaseURL:       "https://generativelanguage.googleapis.com/v1beta",
		Input:         []string{"text", "image"},
		Cost:          ModelCost{Input: 0.1},
		ContextWindow: 262144,
	})
}

type geminiDecisionsRequest struct {
	Model     string                  `json:"model"`
	Input     any                     `json:"input"`
	Questions geminiDecisionQuestions `json:"questions"`
}

type geminiDecisionQuestions ClassifierQuestions

type geminiDecisionImageBlock struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MIMEType string `json:"mime_type"`
}

type geminiDecisionTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type geminiDecisionNoulCriteria struct {
	TrueDescription  string `json:"true_description,omitempty"`
	FalseDescription string `json:"false_description,omitempty"`
}

func (questions geminiDecisionQuestions) MarshalJSON() ([]byte, error) {
	ordered := ClassifierQuestions(questions).inJSOrder()
	return writeJSONObject(len(ordered), func(i int) string { return ordered[i].ID }, func(i int) ([]byte, error) {
		return geminiDecisionQuestionJSON(ordered[i].Question)
	})
}

func geminiDecisionQuestionJSON(question ClassifierQuestion) ([]byte, error) {
	switch question := question.(type) {
	case ClassifierBoolQuestion:
		encoded := fmt.Appendf(nil, `{"type":"noul","instructions":%s`, marshalJSONString(question.Instructions))
		if question.Criteria.True != "" || question.Criteria.False != "" {
			criteria, err := json.Marshal(geminiDecisionNoulCriteria{TrueDescription: question.Criteria.True, FalseDescription: question.Criteria.False})
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, `,"criteria":`...)
			encoded = append(encoded, criteria...)
		}
		return append(encoded, '}'), nil
	case ClassifierChoiceQuestion:
		return question.MarshalJSON()
	case ClassifierScoreQuestion:
		return question.MarshalJSON()
	default:
		return nil, fmt.Errorf("%s received an unsupported question type %T", geminiDecisionsLabel, question)
	}
}

func geminiDecisionsInput(request ClassifierContext) (any, error) {
	state := request.State
	if state == nil {
		state = JsonObject{}
	}
	encoded, err := marshalJSONValue(state)
	if err != nil {
		return nil, err
	}
	if len(request.Images) == 0 {
		return string(encoded), nil
	}
	content := make([]any, 0, len(request.Images)+1)
	content = append(content, geminiDecisionTextBlock{Type: "text", Text: string(encoded)})
	for _, image := range request.Images {
		content = append(content, geminiDecisionImageBlock{Type: "image", Data: image.Data, MIMEType: image.MimeType})
	}
	return content, nil
}

func normalizeGeminiDecisionQuestion(question ClassifierQuestion) (ClassifierQuestion, error) {
	if question == nil {
		return nil, fmt.Errorf("%s received a nil question", geminiDecisionsLabel)
	}
	switch question := question.(type) {
	case ClassifierBoolQuestion:
		return question, nil
	case *ClassifierBoolQuestion:
		if question == nil {
			return nil, fmt.Errorf("%s received a nil question", geminiDecisionsLabel)
		}
		return *question, nil
	case ClassifierChoiceQuestion:
		return question, nil
	case *ClassifierChoiceQuestion:
		if question == nil {
			return nil, fmt.Errorf("%s received a nil question", geminiDecisionsLabel)
		}
		return *question, nil
	case ClassifierScoreQuestion:
		return question, nil
	case *ClassifierScoreQuestion:
		if question == nil {
			return nil, fmt.Errorf("%s received a nil question", geminiDecisionsLabel)
		}
		return *question, nil
	default:
		return nil, fmt.Errorf("%s received an unsupported question type %T", geminiDecisionsLabel, question)
	}
}

func validateGeminiDecisionQuestions(questions ClassifierQuestions) (ClassifierQuestions, error) {
	if len(questions) < 1 || len(questions) > 1000 {
		return nil, fmt.Errorf("%s accepts 1 to 1000 questions, got %d", geminiDecisionsLabel, len(questions))
	}
	validated := make(ClassifierQuestions, len(questions))
	seenIDs := make(map[string]struct{}, len(questions))
	for i, entry := range questions {
		if !validGeminiDecisionQuestionID(entry.ID) {
			return nil, fmt.Errorf("%s received an invalid question ID %q", geminiDecisionsLabel, entry.ID)
		}
		if _, exists := seenIDs[entry.ID]; exists {
			return nil, fmt.Errorf("%s received duplicate question ID %q", geminiDecisionsLabel, entry.ID)
		}
		seenIDs[entry.ID] = struct{}{}
		question, err := normalizeGeminiDecisionQuestion(entry.Question)
		if err != nil {
			return nil, err
		}
		switch question := question.(type) {
		case ClassifierChoiceQuestion:
			if len(question.Criteria) < 2 || len(question.Criteria) > 255 {
				return nil, fmt.Errorf("%s accepts 2 to 255 choice options for %s, got %d", geminiDecisionsLabel, entry.ID, len(question.Criteria))
			}
			seenOptions := make(map[string]struct{}, len(question.Criteria))
			for _, criterion := range question.Criteria {
				if !utf8.ValidString(criterion.Key) {
					return nil, fmt.Errorf("%s received an unrepresentable option key for %s", geminiDecisionsLabel, entry.ID)
				}
				if _, exists := seenOptions[criterion.Key]; exists {
					return nil, fmt.Errorf("%s received duplicate option key %q for %s", geminiDecisionsLabel, criterion.Key, entry.ID)
				}
				seenOptions[criterion.Key] = struct{}{}
			}
		case ClassifierScoreQuestion:
			if len(question.Criteria) < 2 || len(question.Criteria) > 255 {
				return nil, fmt.Errorf("%s accepts 2 to 255 score levels for %s, got %d", geminiDecisionsLabel, entry.ID, len(question.Criteria))
			}
		}
		validated[i] = ClassifierQuestionEntry{ID: entry.ID, Question: question}
	}
	return validated, nil
}

func validGeminiDecisionQuestionID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := range id {
		character := id[i]
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func parseGeminiDecisionsAnswers(raw json.RawMessage, request ClassifierContext) (ClassifierAnswers, error) {
	answers, err := parseAnswers(geminiDecisionsLabel, raw, request)
	if err != nil {
		return nil, err
	}
	questions := make(map[string]ClassifierQuestion, len(request.Questions))
	for _, entry := range request.Questions {
		questions[entry.ID] = entry.Question
	}
	for i := range answers {
		question := questions[answers[i].ID]
		switch question := question.(type) {
		case ClassifierBoolQuestion:
			answer, ok := answers[i].Answer.(ClassifierBoolAnswer)
			if !ok || answer.Probability < 0.01 || answer.Probability > 0.99 {
				return nil, fmt.Errorf("%s returned an invalid bool probability for %s", geminiDecisionsLabel, answers[i].ID)
			}
		case ClassifierChoiceQuestion:
			answer, ok := answers[i].Answer.(ClassifierChoiceAnswer)
			if !ok || answer.Confidence < 0 || answer.Confidence > 1 {
				return nil, fmt.Errorf("%s returned an invalid choice answer for %s", geminiDecisionsLabel, answers[i].ID)
			}
			probabilities := make(map[string]float64, len(answer.Probabilities))
			for _, probability := range answer.Probabilities {
				if probability.Probability < 0 || probability.Probability > 1 {
					return nil, fmt.Errorf("%s returned an out-of-range probability for %s.%s", geminiDecisionsLabel, answers[i].ID, probability.Key)
				}
				probabilities[probability.Key] = probability.Probability
			}
			ordered := make([]ClassifierProbability, 0, len(question.Criteria))
			for _, criterion := range question.Criteria {
				probability, ok := probabilities[criterion.Key]
				if !ok {
					return nil, fmt.Errorf("%s did not return a probability for %s.%s", geminiDecisionsLabel, answers[i].ID, criterion.Key)
				}
				ordered = append(ordered, ClassifierProbability{Key: criterion.Key, Probability: probability})
				delete(probabilities, criterion.Key)
			}
			if len(probabilities) != 0 {
				return nil, fmt.Errorf("%s returned an unknown probability key for %s", geminiDecisionsLabel, answers[i].ID)
			}
			selected := false
			for _, criterion := range question.Criteria {
				if answer.Choice == criterion.Key {
					selected = true
					break
				}
			}
			if !selected {
				return nil, fmt.Errorf("%s returned an unknown choice for %s", geminiDecisionsLabel, answers[i].ID)
			}
			answers[i].Answer = ClassifierChoiceAnswer{Choice: answer.Choice, Probabilities: ordered, Confidence: answer.Confidence}
		case ClassifierScoreQuestion:
			answer, ok := answers[i].Answer.(ClassifierScoreAnswer)
			if !ok || answer.Score < 0 || answer.Score > float64(len(question.Criteria)-1) || answer.Confidence < 0 || answer.Confidence > 1 {
				return nil, fmt.Errorf("%s returned an out-of-range score answer for %s", geminiDecisionsLabel, answers[i].ID)
			}
		default:
			return nil, fmt.Errorf("%s received an unsupported question type for %s", geminiDecisionsLabel, answers[i].ID)
		}
	}
	return answers, nil
}

func parseGeminiDecisionsUsage(raw json.RawMessage, model ClassifierModel) *Usage {
	usage, ok := jsonObjectOf(raw)
	if !ok {
		return nil
	}
	normalized := make(map[string]json.RawMessage, 2)
	if input, exists := usage["total_input_tokens"]; exists {
		normalized["input_tokens"] = input
	}
	if output, exists := usage["total_output_tokens"]; exists {
		normalized["output_tokens"] = output
	}
	if len(normalized) == 0 {
		return nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil
	}
	return ParseClassifierUsage(encoded, model)
}

// ClassifyGeminiDecisions classifies through Google's Gemini Decisions API.
func ClassifyGeminiDecisions(ctx context.Context, model ClassifierModel, request ClassifierContext, options ClassifierOptions) ClassifierResult {
	//portlint:allow clock gemini-decisions API returns one synchronous decision response and uses the classifier timestamp convention
	output := ClassifierResult{API: model.API, Provider: model.Provider, Model: model.ID, Answers: ClassifierAnswers{}, StopReason: ClassifierStopReasonStop, Timestamp: time.Now().UnixMilli()}
	fail := func(err error) ClassifierResult {
		output.Answers = ClassifierAnswers{}
		output.StopReason = ClassifierStopReasonError
		if ctx.Err() != nil {
			output.StopReason = ClassifierStopReasonAborted
		}
		output.ErrorMessage = classifierErrorMessage(err, geminiDecisionsLabel)
		return output
	}
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	if model.API != ClassifierAPIGeminiDecisions {
		return fail(fmt.Errorf("Unsupported classifier API: %s", model.API))
	}
	questions, err := validateGeminiDecisionQuestions(request.Questions)
	if err != nil {
		return fail(err)
	}
	request.Questions = questions
	input, err := geminiDecisionsInput(request)
	if err != nil {
		return fail(err)
	}
	payload := geminiDecisionsRequest{Model: model.ID, Input: input, Questions: geminiDecisionQuestions(questions)}
	authDefaults := ProviderHeadersFromStrings(map[string]string{"x-goog-api-key": options.APIKey, "content-type": "application/json"})
	body, err := postClassifierRequest(ctx, geminiDecisionsLabel, serviceURL(model.BaseURL, "decisions"), model, payload, options, authDefaults, nil)
	if err != nil {
		return fail(err)
	}
	response, ok := jsonObjectOf(body)
	if !ok {
		return fail(fmt.Errorf("%s returned an unexpected response", geminiDecisionsLabel))
	}
	output.Usage = parseGeminiDecisionsUsage(response["usage"], model)
	answers, err := parseGeminiDecisionsAnswers(response["answers"], request)
	if err != nil {
		return fail(err)
	}
	output.Answers = answers
	return output
}

// GeminiDecisionsAPI is the Gemini Decisions classifier module.
func GeminiDecisionsAPI() *ProviderClassifier { return classifierModule(ClassifyGeminiDecisions) }
