package ai

import (
	"reflect"
	"testing"
)

// TestJoinTextPartsKeepsAssistantContent checks the rewritten text join against
// the accumulation it replaces: one part must be the part itself, several parts
// their concatenation with no separator, and no text the empty content the
// caller treats as "send nothing".
func TestJoinTextPartsKeepsAssistantContent(t *testing.T) {
	part := func(text string) oaiContentPart { return oaiContentPart{Type: "text", Text: new(text)} }
	for _, test := range []struct {
		name  string
		parts []oaiContentPart
		want  string
	}{
		{name: "no text parts", parts: nil, want: ""},
		{name: "one empty part", parts: []oaiContentPart{part("")}, want: ""},
		{name: "one part", parts: []oaiContentPart{part("answer")}, want: "answer"},
		{name: "two parts concatenate without a separator", parts: []oaiContentPart{part("alpha"), part("beta")}, want: "alphabeta"},
		{name: "an empty part contributes nothing", parts: []oaiContentPart{part(""), part("beta")}, want: "beta"},
		{name: "all parts empty", parts: []oaiContentPart{part(""), part("")}, want: ""},
		{name: "multibyte text is joined as its bytes", parts: []oaiContentPart{part("é"), part("😀")}, want: "é😀"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := joinTextParts(test.parts); got != test.want {
				t.Fatalf("joinTextParts() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestConvertCompletionsAssistantContent asserts the content a converted
// assistant message carries. Blank text and a tool-call-only message are the two
// cases the text join must not change: the first has no content and is dropped,
// the second keeps a nil content beside its tool call.
func TestConvertCompletionsAssistantContent(t *testing.T) {
	assistant := func(blocks ...AssistantContentBlock) Message {
		return Message(AssistantMessage{Content: blocks, API: APIOpenAICompletions})
	}
	for _, test := range []struct {
		name     string
		msg      Message
		messages int
		want     any
	}{
		{name: "single text block", msg: assistant(TextContent{Text: "hello"}), messages: 1, want: "hello"},
		{name: "two text blocks", msg: assistant(
			TextContent{Text: "hello "},
			TextContent{Text: "world"},
		), messages: 1, want: "hello world"},
		{name: "blank text blocks carry no content", msg: assistant(
			TextContent{Text: "  \n"},
		), messages: 0},
		{name: "tool call only", msg: assistant(ToolCall{
			ID: "call_1", Name: "read", Arguments: JsonObject{"path": "a"},
		}), messages: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, err := convertCompletionsMessages([]Message{test.msg}, completionsConvertOptions{instructionRole: "system"})
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != test.messages {
				t.Fatalf("converted %d messages, want %d: %#v", len(out), test.messages, out)
			}
			if test.messages == 0 {
				return
			}
			if !reflect.DeepEqual(out[0].Content, test.want) {
				t.Fatalf("content = %#v, want %#v", out[0].Content, test.want)
			}
		})
	}
}
