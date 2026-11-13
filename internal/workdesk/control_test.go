package workdesk

import "testing"

func TestDecodeControlMessagesDoesNotConsumePartialJSONL(t *testing.T) {
	messages, consumed := decodeControlMessages([]byte(`{"content":"first"}
{"content":"partial"`))
	if len(messages) != 1 || messages[0].Content != "first" || consumed != int64(len(`{"content":"first"}
`)) {
		t.Fatalf("messages=%+v consumed=%d", messages, consumed)
	}
}
