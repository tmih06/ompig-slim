package ai_test

import (
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestProviderRetryHintDelayParsesStatedWaits(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    time.Duration
	}{
		{"retry-after: 2", 2 * time.Second},
		{"Retry-After: 30", 30 * time.Second},
		{"retry-after-ms: 2500", 2500 * time.Millisecond},
		{"try again in 5 min", 5 * time.Minute},
		{"Try again in ~90 min.", 90 * time.Minute},
		{"Try again in 2s when the quota resets.", 2 * time.Second},
		{"retry in 30 seconds", 30 * time.Second},
		{"retry in 1 hour", time.Hour},
		{"usage limit reached; wait", 0},
		{"", 0},
	} {
		if got := ai.ProviderRetryHintDelay(tc.message); got != tc.want {
			t.Fatalf("ProviderRetryHintDelay(%q) = %v, want %v", tc.message, got, tc.want)
		}
	}
	// Several hints take the longest, as a provider offering two resets means the later one.
	if got := ai.ProviderRetryHintDelay("try again in 2s; retry-after: 5"); got != 5*time.Second {
		t.Fatalf("the longest stated wait must win, got %v", got)
	}
}

func TestRetryDelayForMessageHonorsQuotaResets(t *testing.T) {
	policy := ai.RetryPolicy{BaseDelayMs: 1000}
	cap := 100
	capped := ai.RetryPolicy{BaseDelayMs: 1000, MaxAgentDelayMs: &cap}
	// An exhausted account's stated reset is the real deadline: it is honored at full length
	// even past the agent cap, since retrying early only meets the same block.
	before := time.Now().UnixMilli()
	got := ai.RetryDelayForAssistant(capped, 1, ai.AssistantMessage{ErrorMessage: "All eligible accounts are temporarily unavailable. Try again in 2 min when the quota resets."})
	elapsed := time.Now().UnixMilli() - before
	if got < 120_000-int(elapsed) || got > 120_000 {
		t.Fatalf("a quota reset past the cap must still be honored, got %d", got)
	}
	before = time.Now().UnixMilli()
	got = ai.RetryDelayForAssistant(capped, 1, ai.AssistantMessage{ErrorMessage: "provider returned error; retry in 5 min"})
	elapsed = time.Now().UnixMilli() - before
	if got < 300_000-int(elapsed) || got > 300_000 {
		t.Fatalf("a provider deadline must not be shortened, got %d", got)
	}
	// A hint shorter than the backoff yields to the backoff.
	if got := ai.RetryDelayForAssistant(policy, 3, ai.AssistantMessage{ErrorMessage: "retry in 1s"}); got != 4_000 {
		t.Fatalf("the backoff must win over a shorter hint, got %d", got)
	}
	// No hint is plain backoff.
	if got := ai.RetryDelayForAssistant(policy, 2, ai.AssistantMessage{ErrorMessage: "connection reset"}); got != 2_000 {
		t.Fatalf("an unstated failure must use the backoff, got %d", got)
	}
}

func TestIsRetryableAssistantErrorClassifiesTheUnavailablePool(t *testing.T) {
	retryable := ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: "All eligible accounts for p/m are temporarily unavailable. Try again in 2s when the quota resets."}
	if !ai.IsRetryableAssistantError(retryable) {
		t.Fatal("a pool naming its quota reset must be retryable")
	}
	terminal := ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: "All eligible accounts for p/m are temporarily unavailable."}
	if ai.IsRetryableAssistantError(terminal) {
		t.Fatal("a reset-less pool error must stay terminal")
	}
}
