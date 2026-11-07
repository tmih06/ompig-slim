package codingagent

import (
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

func TestFormatElapsedDurationIncludesDays(t *testing.T) {
	for _, test := range []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 24 * time.Hour, want: "1d"},
		{elapsed: 25*time.Hour + 2*time.Minute + 3*time.Second, want: "1d 1h2m3s"},
	} {
		if got := formatElapsedDuration(test.elapsed); got != test.want {
			t.Errorf("formatElapsedDuration(%s) = %q, want %q", test.elapsed, got, test.want)
		}
	}
}

func TestFormatThroughputFormatsLongElapsedTimeReadably(t *testing.T) {
	message := agent.AgentMessage{Assistant: &agent.AssistantMessage{
		Role:   "assistant",
		Usage:  &ai.Usage{Output: 1, TotalTokens: 1},
	}}

	got := formatThroughput([]agent.AgentMessage{message}, 1550700)
	want := "TPS 0.0 tok/s. out 1, in 0, cache r/w 0/0, total 1, 25m50.7s"
	if got != want {
		t.Fatalf("formatThroughput() = %q, want %q", got, want)
	}
}
