package codemode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestModelsJudgeValidatesQuestionsAndImages(t *testing.T) {
	models := &judgeBridgeModels{}
	var execute func(context.Context, json.RawMessage) (json.RawMessage, error)
	for _, global := range (&run{}).modelGlobals(models) {
		if global.Name == "models.judge" {
			execute = global.Execute
		}
	}
	if execute == nil {
		t.Fatal("models.judge is not exposed")
	}
	valid := `{"state":{"large":"data"},"questions":{"bool":{"type":"bool","instructions":"Check","criteria":{"true":"Yes","false":"No"}},"choice":{"type":"choice","instructions":"Pick","criteria":{"a":"First","b":"Second"}},"score":{"type":"score","instructions":"Rate","criteria":["Low","High"]}},"images":[{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}]}`
	if _, err := execute(context.Background(), json.RawMessage("["+valid+"]")); err != nil {
		t.Fatal(err)
	}
	models.called = 0
	for _, test := range []struct {
		name, before, after, problem string
	}{
		{"state", `"state":{"large":"data"}`, `"state":[]`, "context.state must be an object"},
		{"question", `"bool":{"type":"bool","instructions":"Check","criteria":{"true":"Yes","false":"No"}}`, `"bool":null`, "context.questions.bool must be a question object"},
		{"instructions", `"instructions":"Check"`, `"instructions":null`, "context.questions.bool.instructions must be a string"},
		{"bool criteria", `"criteria":{"true":"Yes","false":"No"}`, `"criteria":[]`, `criteria must be { true: string, false: string }`},
		{"bool true", `"true":"Yes"`, `"true":false`, `criteria must be { true: string, false: string }`},
		{"bool false", `"false":"No"`, `"false":null`, `criteria must be { true: string, false: string }`},
		{"choice criteria", `"criteria":{"a":"First","b":"Second"}`, `"criteria":{"a":1}`, "criteria must map each label to its meaning"},
		{"score criteria", `"criteria":["Low","High"]`, `"criteria":[1,2]`, "criteria must list the levels as strings"},
		{"images", `"images":[{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}]`, `"images":{}`, "context.images must be an array"},
		{"image object", `{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}`, `null`, "context.images[0] must be an image block"},
		{"image type", `"type":"image"`, `"type":"text"`, "context.images[0] must be an image block"},
		{"image data", `"data":"aW1hZ2U="`, `"data":null`, "context.images[0] must be an image block"},
		{"image MIME", `"mimeType":"image/png"`, `"mimeType":null`, "context.images[0] must be an image block"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := strings.Replace(valid, test.before, test.after, 1)
			if raw == valid {
				t.Fatal("test mutation did not match its fixture")
			}
			if _, err := execute(context.Background(), json.RawMessage("["+raw+"]")); err == nil || !strings.Contains(err.Error(), test.problem) {
				t.Fatalf("got %v, want %s", err, test.problem)
			}
			if models.called != 0 {
				t.Fatal("invalid context reached the judge")
			}
		})
	}
}
