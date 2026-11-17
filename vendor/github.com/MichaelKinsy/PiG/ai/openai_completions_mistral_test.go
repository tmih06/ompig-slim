package ai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestMistralCompletionsContentParts(t *testing.T) {
	for _, test := range []struct {
		name, delta, text, thinking string
	}{
		{"text parts", `{"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}`, "hello world!", ""},
		{"thinking parts", `{"content":[{"type":"thinking","thinking":[{"type":"text","text":"think "},{"type":"text","text":"carefully"}]},{"type":"text","text":"answer"}]}`, "answer!", "think carefully"},
		{"empty and unknown parts", `{"content":[null,{"type":"text","text":""},{"type":"citation","url":"https://example.com"}]}`, "!", ""},
		{"plain string", `{"content":"hello"}`, "hello!", ""},
		{"mixed string and text parts", `{"content":["hello ",{"type":"text","text":"é😀"}]}`, "hello é😀!", ""},
		{"tool call beside text", `{"content":[{"type":"text","text":"reading"}],"tool_calls":[{"index":0,"id":"call12345","type":"function","function":{"name":"read","arguments":"{\"path\":\"hello.txt\"}"}}]}`, "reading!", ""},
		{"reasoning field", `{"content":[{"type":"text","text":"answer"}],"reasoning_content":"reason"}`, "answer!", "reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("request path = %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":%s}]}\n\n", test.delta)
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"!\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			provider := ai.NewOpenAIProvider(ai.OpenAIConfig{
				BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "devstral-medium-latest", ProviderID: "mistral",
			})
			defer provider.Close()
			stream, err := provider.Stream(context.Background(), ai.NormalizeContext(ai.Context{
				Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi"), Timestamp: 1}},
			}), ai.StreamOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var final *ai.AssistantMessage
			var streamedText, streamedThinking strings.Builder
			for event := range stream.Events(context.Background()) {
				switch event := event.(type) {
				case ai.TextDeltaEvent:
					streamedText.WriteString(event.Delta)
				case ai.ThinkingDeltaEvent:
					streamedThinking.WriteString(event.Delta)
				case ai.DoneEvent:
					final = event.Message
				case ai.ErrorEvent:
					t.Fatalf("stream failed: %s", event.Error.ErrorMessage)
				}
			}
			if final == nil || final.StopReason != ai.StopReasonStop {
				t.Fatalf("missing successful terminal message: %#v", final)
			}
			var text, thinking strings.Builder
			var toolCalls int
			for _, block := range final.Content {
				switch block := block.(type) {
				case ai.TextContent:
					text.WriteString(block.Text)
				case ai.ThinkingContent:
					thinking.WriteString(block.Thinking)
				case ai.ToolCall:
					toolCalls++
					if block.Name != "read" || block.Arguments["path"] != "hello.txt" {
						t.Fatalf("tool call changed: %#v", block)
					}
				}
			}
			if strings.Contains(test.delta, `"tool_calls"`) && toolCalls != 1 {
				t.Fatalf("got %d tool calls, want 1", toolCalls)
			}
			if text.String() != test.text || thinking.String() != test.thinking {
				t.Fatalf("final text/thinking = %q/%q, want %q/%q", text.String(), thinking.String(), test.text, test.thinking)
			}
			if streamedText.String() != test.text || streamedThinking.String() != test.thinking {
				t.Fatalf("streamed text/thinking = %q/%q, want %q/%q", streamedText.String(), streamedThinking.String(), test.text, test.thinking)
			}
		})
	}
}
