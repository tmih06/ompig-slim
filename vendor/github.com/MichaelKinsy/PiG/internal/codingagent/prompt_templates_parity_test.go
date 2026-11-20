package codingagent

import (
	"reflect"
	"testing"
)

func TestParsePromptArgsMatchesPi151QuoteRules(t *testing.T) {
	for _, test := range []struct {
		input string
		want  []string
	}{
		{input: "doesn't merge words", want: []string{"doesn't", "merge", "words"}},
		{input: `say "hi there"`, want: []string{"say", "hi there"}},
		{input: `"say \"hi\""`, want: []string{`say "hi"`}},
		{input: `it's fine`, want: []string{"it's", "fine"}},
		{input: `"unclosed quote`, want: []string{`"unclosed`, "quote"}},
		{input: `""`, want: []string{""}},
	} {
		t.Run(test.input, func(t *testing.T) {
			if got := ParsePromptArgs(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParsePromptArgs(%q) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}
