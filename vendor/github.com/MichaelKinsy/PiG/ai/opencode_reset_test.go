package ai

import (
	"net/http"
	"strings"
	"testing"
)

func TestFreeUsageResetSuffix(t *testing.T) {
	freeBody := []byte(`{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`)
	goBody := []byte(`{"type":"GoUsageLimitError","message":"Go usage limit reached"}`)
	otherBody := []byte(`{"error":{"message":"server error"}}`)

	for _, tc := range []struct {
		name    string
		status  int
		body    []byte
		headers http.Header
		want    string
	}{
		{
			name:    "retry-after seconds",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{"Retry-After": {"44951"}},
			want:    " (resets in 12h29m)",
		},
		{
			name:    "retry-after milliseconds",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{"Retry-After-Ms": {"900000"}},
			want:    " (resets in 15m)",
		},
		{
			name:    "go tier",
			status:  http.StatusTooManyRequests,
			body:    goBody,
			headers: http.Header{"Retry-After": {"90061"}},
			want:    " (resets in 1d01h)",
		},
		{
			name:    "other error body unchanged",
			status:  http.StatusTooManyRequests,
			body:    otherBody,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    "",
		},
		{
			name:    "non-429 unchanged",
			status:  http.StatusForbidden,
			body:    freeBody,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    "",
		},
		{
			name:    "free error without reset header unchanged",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{},
			want:    "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := freeUsageResetSuffix(tc.status, tc.body, tc.headers); got != tc.want {
				t.Fatalf("suffix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderErrorResetHintSurvivesBodySubstitution(t *testing.T) {
	raw := []byte(`{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`)
	err := openAIHTTPError(http.StatusTooManyRequests, raw, http.Header{"Retry-After": {"3600"}})
	message := err.Error()
	if !strings.Contains(message, "FreeUsageLimitError") {
		t.Fatalf("body was replaced: %q", message)
	}
	if !strings.HasSuffix(message, " (resets in 1h00m)") {
		t.Fatalf("reset hint lost: %q", message)
	}
}
