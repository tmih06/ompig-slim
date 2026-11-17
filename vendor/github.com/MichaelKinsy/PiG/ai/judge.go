package ai

import "encoding/json"

// JudgeAnswer is the closed union of values returned by a judge operation.
type JudgeAnswer interface {
	judgeAnswer()
	AnswerType() string
}

// JudgeBoolAnswer is the answer to a bool question. Probability is present only
// when a native classifier supplies it.
type JudgeBoolAnswer struct {
	Value       bool
	Probability *float64
}

func (JudgeBoolAnswer) judgeAnswer()       {}
func (JudgeBoolAnswer) AnswerType() string { return "bool" }
func (answer JudgeBoolAnswer) MarshalJSON() ([]byte, error) {
	type classifierMetadata struct {
		Probability float64 `json:"probability"`
	}
	var classifier *classifierMetadata
	if answer.Probability != nil {
		classifier = &classifierMetadata{Probability: *answer.Probability}
	}
	return json.Marshal(struct {
		Type       string              `json:"type"`
		Value      bool                `json:"value"`
		Classifier *classifierMetadata `json:"classifier,omitempty"`
	}{answer.AnswerType(), answer.Value, classifier})
}

// JudgeChoiceProbability is one native classifier's probability for a choice.
type JudgeChoiceProbability struct {
	Key         string  `json:"key"`
	Probability float64 `json:"probability"`
}

// JudgeChoiceClassifierMetadata is native classifier metadata for a choice.
type JudgeChoiceClassifierMetadata struct {
	Probabilities []JudgeChoiceProbability `json:"probabilities,omitempty"`
	Confidence    float64                  `json:"confidence"`
}

// JudgeChoiceAnswer is the answer to a choice question.
type JudgeChoiceAnswer struct {
	Value      string
	Classifier *JudgeChoiceClassifierMetadata
}

func (JudgeChoiceAnswer) judgeAnswer()       {}
func (JudgeChoiceAnswer) AnswerType() string { return "choice" }
func (answer JudgeChoiceAnswer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       string                         `json:"type"`
		Value      string                         `json:"value"`
		Classifier *JudgeChoiceClassifierMetadata `json:"classifier,omitempty"`
	}{answer.AnswerType(), answer.Value, answer.Classifier})
}

// JudgeScoreClassifierMetadata is native classifier metadata for a score.
type JudgeScoreClassifierMetadata struct {
	Confidence float64 `json:"confidence"`
}

// JudgeScoreAnswer is the answer to a score question.
type JudgeScoreAnswer struct {
	Value      float64
	Classifier *JudgeScoreClassifierMetadata
}

func (JudgeScoreAnswer) judgeAnswer()       {}
func (JudgeScoreAnswer) AnswerType() string { return "score" }
func (answer JudgeScoreAnswer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       string                        `json:"type"`
		Value      float64                       `json:"value"`
		Classifier *JudgeScoreClassifierMetadata `json:"classifier,omitempty"`
	}{answer.AnswerType(), answer.Value, answer.Classifier})
}

// JudgeStopReason is why a judge request ended.
type JudgeStopReason string

const (
	JudgeStopReasonStop    JudgeStopReason = "stop"
	JudgeStopReasonError   JudgeStopReason = "error"
	JudgeStopReasonAborted JudgeStopReason = "aborted"
)

// JudgeResult is the final result from the configured judge role. Usage sums
// every candidate attempt, including failed attempts.
type JudgeResult struct {
	Provider     string                 `json:"provider"`
	Model        string                 `json:"model"`
	Answers      map[string]JudgeAnswer `json:"answers"`
	Usage        *Usage                 `json:"usage,omitempty"`
	StopReason   JudgeStopReason        `json:"stopReason"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
}
