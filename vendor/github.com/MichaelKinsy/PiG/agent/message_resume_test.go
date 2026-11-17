package agent

import (
	"bytes"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
	"github.com/MichaelKinsy/PiG/internal/jsonstringify"
)

func TestResumeMessageDecodePreservesSource(t *testing.T) {
	for _, source := range []string{
		`{ "role": "user", "content": "saved prompt", "timestamp": 1 }`,
		`{"role":"assistant","content":[{"type":"text","text":"saved answer"}],"provider":"fixture","model":"one","timestamp":1,"future":{"keep":true}}`,
		`{"role":"toolResult","toolCallId":"r","toolName":"read","content":[{"type":"text","text":"file output"}],"isError":false,"timestamp":1}`,
		`{"role":"user","content":"\ud800","timestamp":1.0}`,
		`{"role":"assistant","content":[{"type":"thinking","thinking":"saved thought","thinkingSignature":"signature"},{"type":"toolCall","id":"r","name":"read","arguments":{"path":"a.go"}}],"timestamp":1}`,
		`{"role":"user","content":"old","content":"new","timestamp":1e0}`,
		`{"role":"assistant","content":[{"type":"text","text":"kept"},false],"timestamp":1,"durationMs":"wrong-type"}`,
	} {
		t.Run(source, func(t *testing.T) {
			var message AgentMessage
			if err := json.Unmarshal([]byte(source), &message); err != nil {
				t.Fatal(err)
			}
			if message.Role() == "" {
				t.Fatal("decoded message lost its role")
			}
			want, err := jsonstringify.Canonicalize([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.AppendStringify(nil, message)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("round trip = %s, %v; want %s", got, err, want)
			}
		})
	}
}

func TestResumeMessageDecodeRejectsInvalidJSON(t *testing.T) {
	for _, source := range []string{
		`{"role":"user","content":[{"type":"text","text":"incomplete"}]`,
		`{"role":"user","content":"one"} trailing`,
		`{"role":"user","content":[true,]}`,
		`null`,
	} {
		var message AgentMessage
		if err := json.Unmarshal([]byte(source), &message); err == nil {
			t.Fatalf("invalid message accepted: %s", source)
		}
	}
}
