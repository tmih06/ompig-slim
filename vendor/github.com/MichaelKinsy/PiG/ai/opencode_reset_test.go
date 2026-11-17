package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFreeUsageResetAfter(t *testing.T) {
	freeBody := []byte(`{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`)
	goBody := []byte(`{"type":"GoUsageLimitError","message":"Go usage limit reached"}`)
	otherBody := []byte(`{"error":{"message":"server error"}}`)

	for _, tc := range []struct {
		name    string
		status  int
		body    []byte
		headers http.Header
		want    time.Duration
	}{
		{
			name:    "retry-after seconds",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{"Retry-After": {"44951"}},
			want:    44951 * time.Second,
		},
		{
			name:    "retry-after milliseconds",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{"Retry-After-Ms": {"900000"}},
			want:    15 * time.Minute,
		},
		{
			name:    "go tier",
			status:  http.StatusTooManyRequests,
			body:    goBody,
			headers: http.Header{"Retry-After": {"90061"}},
			want:    90061 * time.Second,
		},
		{
			name:    "other error body unchanged",
			status:  http.StatusTooManyRequests,
			body:    otherBody,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    0,
		},
		{
			name:    "non-429 unchanged",
			status:  http.StatusForbidden,
			body:    freeBody,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    0,
		},
		{
			name:    "free error without reset header unchanged",
			status:  http.StatusTooManyRequests,
			body:    freeBody,
			headers: http.Header{},
			want:    0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := freeUsageResetAfter(tc.status, tc.body, tc.headers); got != tc.want {
				t.Fatalf("reset duration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProviderErrorResetCountdownUnits(t *testing.T) {
	for _, tc := range []struct {
		delay time.Duration
		want  string
	}{
		{0, "quota"},
		{time.Second, "quota (resets in 1m)"},
		{59*time.Minute + 59*time.Second, "quota (resets in 59m)"},
		{time.Hour, "quota (resets in 1h00m)"},
		{23*time.Hour + 59*time.Minute, "quota (resets in 23h59m)"},
		{24 * time.Hour, "quota (resets in 1d00h)"},
		{25*time.Hour + time.Minute, "quota (resets in 1d01h)"},
	} {
		t.Run(tc.delay.String(), func(t *testing.T) {
			got := FormatProviderError(NormalizedProviderError{Message: "quota", UsageResetAfter: tc.delay})
			if got != tc.want {
				t.Fatalf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderErrorResetTimeSurvivesBodySubstitution(t *testing.T) {
	raw := []byte(`{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`)
	err := openAIHTTPError(http.StatusTooManyRequests, raw, http.Header{"Retry-After": {"3600"}})
	normalized := NormalizeProviderError(err)
	if normalized.UsageResetAfter != time.Hour {
		t.Fatalf("normalized reset duration = %v, want 1h", normalized.UsageResetAfter)
	}
	message := FormatProviderError(normalized)
	if message != err.Error() {
		t.Fatalf("normalized error = %q, direct error = %q", message, err.Error())
	}
	if !strings.Contains(message, "FreeUsageLimitError") {
		t.Fatalf("body was replaced: %q", message)
	}
	if !strings.HasSuffix(message, " (resets in 1h00m)") {
		t.Fatalf("reset time lost: %q", message)
	}
}

type openCodeQuotaErrorTransport struct {
	body     string
	headers  http.Header
	requests *atomic.Int32
}

func (transport openCodeQuotaErrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     transport.headers.Clone(),
		Body:       io.NopCloser(strings.NewReader(transport.body)),
		Request:    request,
	}, nil
}

func TestQuotaResetSurvivesChatAndClassifierFailures(t *testing.T) {
	for _, quota := range []struct {
		name            string
		body            string
		providerMessage string
	}{
		{
			name:            "free usage",
			body:            `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`,
			providerMessage: "Free usage limit reached",
		},
		{
			name:            "go usage",
			body:            `{"type":"error","error":{"type":"GoUsageLimitError","message":"Go usage limit reached"}}`,
			providerMessage: "Go usage limit reached",
		},
	} {
		for _, surface := range []string{"chat", "classifier"} {
			for _, retries := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/%s/retries-%d", quota.name, surface, retries), func(t *testing.T) {
					requests := &atomic.Int32{}
					message := openCodeQuotaFailureMessage(t, surface, quota.body, http.Header{"Retry-After": {"3600"}}, retries, requests)
					if got := requests.Load(); got != 1 {
						t.Fatalf("request count = %d, want 1", got)
					}
					if count := strings.Count(message, "resets in 1h00m"); count != 1 {
						t.Fatalf("reset countdown count = %d, message = %q", count, message)
					}
					if !strings.Contains(message, quota.providerMessage) {
						t.Fatalf("provider message missing from %q", message)
					}
					capMessage := "Server requested 3600s retry delay (max: 60s)"
					if retries == 1 && !strings.Contains(message, capMessage) {
						t.Fatalf("retry cap missing from %q", message)
					}
					if retries == 0 && strings.Contains(message, capMessage) {
						t.Fatalf("unexpected retry cap in %q", message)
					}
				})
			}
		}
	}
}

func TestQuotaErrorWithoutResetAndNonQuotaErrorRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		surface string
		body    string
		headers http.Header
		retries int
		want    string
	}{
		{
			name:    "chat non-quota despite retry-after",
			surface: "chat",
			body:    `{"error":{"message":"server error"}}`,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    `429: {"message":"server error"}`,
		},
		{
			name:    "chat quota without retry-after",
			surface: "chat",
			body:    `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`,
			want:    `429: {"type":"FreeUsageLimitError","message":"Free usage limit reached"}`,
		},
		{
			name:    "chat quota with invalid retry-after",
			surface: "chat",
			body:    `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`,
			headers: http.Header{"Retry-After": {"invalid"}},
			want:    `429: {"type":"FreeUsageLimitError","message":"Free usage limit reached"}`,
		},
		{
			name:    "classifier non-quota despite retry-after",
			surface: "classifier",
			body:    `{"type":"error","error":{"type":"Error","message":"server error"}}`,
			headers: http.Header{"Retry-After": {"3600"}},
			want:    `System One API error (429): {"type":"error","error":{"type":"Error","message":"server error"}}`,
		},
		{
			name:    "classifier quota without retry-after",
			surface: "classifier",
			body:    `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`,
			want:    `System One API error (429): {"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached"}}`,
		},
		{
			name:    "chat non-quota retry-cap error unchanged",
			surface: "chat",
			body:    `{"error":{"message":"server error"}}`,
			headers: http.Header{"Retry-After": {"3600"}},
			retries: 1,
			want:    "Server requested 3600s retry delay (max: 60s). 429 server error",
		},
		{
			name:    "classifier non-quota retry-cap error unchanged",
			surface: "classifier",
			body:    `{"type":"error","error":{"type":"Error","message":"server error"}}`,
			headers: http.Header{"Retry-After": {"3600"}},
			retries: 1,
			want:    "Server requested 3600s retry delay (max: 60s). System One API returned 429",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := &atomic.Int32{}
			message := openCodeQuotaFailureMessage(t, tc.surface, tc.body, tc.headers, tc.retries, requests)
			if message != tc.want {
				t.Fatalf("message = %q, want %q", message, tc.want)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("request count = %d, want 1", got)
			}
		})
	}
}

func openCodeQuotaFailureMessage(t *testing.T, surface, body string, headers http.Header, retries int, requests *atomic.Int32) string {
	t.Helper()
	fetch := &http.Client{Transport: openCodeQuotaErrorTransport{body: body, headers: headers, requests: requests}}
	switch surface {
	case "chat":
		model := &Model{ID: "fixture", ProviderMeta: ProviderMetadata{ProviderID: "opencode-go", API: APIOpenAICompletions, BaseURL: "https://fixture.invalid/v1"}}
		stream, err := OpenAICompletionsAPI().StreamSimple(context.Background(), model, NormalizeContext(Context{Messages: []Message{UserMessage{Content: UserText("Hello")}}}), StreamOptions{APIKey: "fixture", Fetch: fetch, MaxRetries: new(retries)})
		if err != nil {
			t.Fatal(err)
		}
		return stream.Result().ErrorMessage
	case "classifier":
		model := GetBuiltinClassifierModel("opencode", "jev-1.13-free")
		if model == nil {
			t.Fatal("missing OpenCode classifier fixture")
		}
		model.BaseURL = "https://fixture.invalid/v1"
		result := ClassifyTypesafeSystemOne(context.Background(), *model, ClassifierContext{
			State: JsonObject{"text": "hello"},
			Questions: ClassifierQuestions{{ID: "ok", Question: ClassifierBoolQuestion{
				Instructions: "Is this OK?", Criteria: ClassifierBoolCriteria{True: "yes", False: "no"},
			}}},
		}, ClassifierOptions{APIKey: "fixture", Fetch: fetch, MaxRetries: new(retries)})
		return result.ErrorMessage
	default:
		t.Fatalf("unknown surface %q", surface)
		return ""
	}
}
