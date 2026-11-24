package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestProviderFailureClassification(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		body    string
		headers http.Header
		kind    string
		retry   bool
		wait    time.Duration
	}{
		{"openai quota", 429, `{"error":{"code":"usage_limit_reached"}}`, http.Header{"Retry-After": {"2"}}, "quota", true, 2 * time.Second},
		{"anthropic rate limit", 429, `{"error":{"type":"rate_limit_error"}}`, http.Header{"Retry-After-Ms": {"2500"}}, "rate_limit", true, 2500 * time.Millisecond},
		{"google retry info", 429, `{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"3s"}]}}`, nil, "rate_limit", true, 3 * time.Second},
		{"unknown quota", 429, `{"error":{"code":"insufficient_quota"}}`, nil, "quota", false, 0},
		{"auth", 401, `{"error":{"message":"bad key"}}`, http.Header{"Retry-After": {"1"}}, "auth", false, time.Second},
		{"permission", 403, "permission denied", nil, "permission", false, 0},
		{"billing", 402, "payment required", nil, "billing", false, 0},
		{"invalid", 422, "bad request", nil, "invalid", false, 0},
		{"invalid mentioning server", 400, "invalid request describing a 503 server error", nil, "invalid", false, 0},
		{"numeric quota code", 429, `{"error":{"code":1308,"message":"limit"}}`, nil, "quota", false, 0},
		{"overflow hint with valid header", 429, "rate limit", http.Header{"Retry-After-Ms": {"10000000000000"}, "Retry-After": {"2"}}, "rate_limit", true, 2 * time.Second},
		{"server", 503, "service unavailable", nil, "transient", true, 0},
		{"no retry", 503, "service unavailable", http.Header{"X-Should-Retry": {"false"}}, "permanent", false, 0},
		{"malformed delay", 429, "rate_limit_error", http.Header{"Retry-After": {"NaN"}}, "rate_limit", true, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := time.Now().UnixMilli()
			err := fmt.Errorf("adapter: %w", openAIHTTPError(test.status, []byte(test.body), test.headers))
			message := AssistantMessage{StopReason: StopReasonError, ErrorMessage: err.Error()}
			RecordProviderFailure(&message, err)
			encoded, marshalErr := json.Marshal(message)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var restored AssistantMessage
			if decodeErr := json.Unmarshal(encoded, &restored); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			failure := ProviderFailureForMessage(restored)
			if failure.Kind != test.kind || failure.Status != test.status || IsRetryableAssistantError(restored) != test.retry {
				t.Fatalf("failure=%+v retry=%v, want kind=%s retry=%v", failure, IsRetryableAssistantError(restored), test.kind, test.retry)
			}
			if test.wait > 0 && failure.RetryAt < before+test.wait.Milliseconds() {
				t.Fatalf("deadline too early: %+v", failure)
			}
			if test.wait == 0 && failure.RetryAt != 0 {
				t.Fatalf("invented reset: %+v", failure)
			}
		})
	}
	if failure := ClassifyProviderFailure(context.Canceled); failure.Kind != "cancelled" || failure.Retryable() {
		t.Fatalf("cancelled failure=%+v", failure)
	}
}

func TestProviderRequestHonorsBodyOnlyReset(t *testing.T) {
	calls := 0
	status := 429
	failure := &ClassifierHTTPError{Status: &status, Body: `{"error":{"status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"3s"}]}}`, Message: "rate limited"}
	_, err := RetryProviderRequest(t.Context(), func() (string, error) {
		calls++
		return "", failure
	}, ProviderRetryOptions{MaxRetries: new(3), MaxRetryDelayMs: new(1)})
	if err == nil || calls != 1 || ClassifyProviderFailure(err).RetryAt < time.Now().Add(2*time.Second).UnixMilli() {
		t.Fatalf("body reset shortened: calls=%d err=%v", calls, err)
	}
}

func TestProviderFailurePreservesCodexAbsoluteReset(t *testing.T) {
	reset := time.Now().Add(2 * time.Hour).Unix()
	err := codexHTTPError(429, "Too Many Requests", []byte(fmt.Sprintf(`{"error":{"code":"usage_limit_reached","resets_at":%d}}`, reset)))
	failure := ClassifyProviderFailure(err)
	if failure.Kind != "quota" || failure.RetryAt != reset*1000 {
		t.Fatalf("absolute reset lost: %+v", failure)
	}
}

func TestRetryLongQuotaWaitAndCancellation(t *testing.T) {
	if got := retryTimerDuration(30 * 24 * 60 * 60 * 1000); got != 30*24*time.Hour {
		t.Fatalf("long wait shortened: %v", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	message, err := RetryAssistantCall(ctx, func() (AssistantMessage, error) {
		calls++
		return AssistantMessage{StopReason: StopReasonError, ErrorMessage: "quota exceeded; try again in 720 hours"}, nil
	}, &RetryPolicy{Enabled: true, MaxRetries: 2, BaseDelayMs: 1}, RetryCallbacks{})
	if err != nil || message.StopReason != StopReasonAborted || calls != 1 {
		t.Fatalf("cancelled retry calls=%d message=%+v err=%v", calls, message, err)
	}
}
