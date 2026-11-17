package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

type benchmarkJudgeModels struct{ judgeBridgeModels }

func (*benchmarkJudgeModels) Judge(_ context.Context, request ai.ClassifierContext) ai.JudgeResult {
	answers := make(map[string]ai.JudgeAnswer, len(request.Questions))
	for _, question := range request.Questions {
		answers[question.ID] = ai.JudgeBoolAnswer{Value: true}
	}
	return ai.JudgeResult{Provider: "fixture", Model: "judge", Answers: answers, StopReason: ai.JudgeStopReasonStop}
}

func BenchmarkJudgeContext(b *testing.B) {
	for _, fixture := range []struct {
		name      string
		stateSize int
		questions int
	}{
		{"small", 128, 1},
		{"large", 64 * 1024, 32},
	} {
		questions := make(map[string]any, fixture.questions)
		for i := 0; i < fixture.questions; i++ {
			questions[fmt.Sprintf("check%d", i)] = map[string]any{
				"type": "bool", "instructions": "Does the result match?",
				"criteria": map[string]string{"true": "Matches", "false": "Does not match"},
			}
		}
		raw, err := json.Marshal(map[string]any{"state": map[string]string{"text": strings.Repeat("x", fixture.stateSize)}, "questions": questions})
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fixture.name+"/validate", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := checkJudgeContext(raw); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fixture.name+"/bridge", func(b *testing.B) {
			args := json.RawMessage("[" + string(raw) + "]")
			models := &benchmarkJudgeModels{}
			b.ReportAllocs()
			for b.Loop() {
				r := &run{}
				for _, global := range r.modelGlobals(models) {
					if global.Name != "models.judge" {
						continue
					}
					result, err := global.Execute(context.Background(), args)
					if err != nil || !json.Valid(result) || r.snapshot()[0].Status != StatusOK {
						b.Fatalf("judge bridge failed: %s (%v)", result, err)
					}
				}
			}
		})
	}
}
