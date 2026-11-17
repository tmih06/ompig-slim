package coding

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// openCodeFreeLimitTransport returns the gateway's real free-tier 429: the
// FreeUsageLimitError body and the Retry-After header that carries the reset.
type openCodeFreeLimitTransport struct {
	requests *atomic.Int32
}

func (transport openCodeFreeLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	body := `{"type":"error","error":{"type":"FreeUsageLimitError","message":"Free usage limit reached. Free models work in OpenCode (https://opencode.ai); subscribe to OpenCode Go to keep going: https://opencode.ai/go"}}`
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Retry-After":  {"3600"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: request,
	}, nil
}

func TestOpenCodeFreeQuotaErrorShowsResetTime(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{CWD: t.TempDir(), AgentDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { services.Close() })
	runtime := services.ModelRuntime()

	entry := ai.ListModels(ai.OpenCodeFreeProviderID)[0]
	model, err := BuildModel(ai.OpenCodeFreeProviderID+"/"+entry.ID, services)
	if err != nil {
		t.Fatal(err)
	}
	request := ai.Context{
		SystemPrompt: "Caller instructions remain authoritative.",
		Messages:     []ai.Message{ai.UserMessage{Content: ai.UserText("Hello"), Timestamp: 1}},
	}
	for _, retries := range []int{0, 1} {
		t.Run(fmt.Sprintf("retries-%d", retries), func(t *testing.T) {
			requests := &atomic.Int32{}
			options := ai.StreamOptions{Fetch: &http.Client{Transport: openCodeFreeLimitTransport{requests: requests}}, MaxRetries: new(retries), MaxTokens: 512}
			result := runtime.StreamSimple(context.Background(), model, request, options).Result()
			if result == nil || result.StopReason != ai.StopReasonError {
				t.Fatalf("quota error not returned: %+v", result)
			}
			if !strings.Contains(result.ErrorMessage, "Free usage limit reached") {
				t.Fatalf("error lost the provider message: %q", result.ErrorMessage)
			}
			if count := strings.Count(result.ErrorMessage, "resets in 1h00m"); count != 1 {
				t.Fatalf("reset countdown count = %d, error = %q", count, result.ErrorMessage)
			}
			if retries == 1 && !strings.Contains(result.ErrorMessage, "Server requested 3600s retry delay (max: 60s)") {
				t.Fatalf("retry cap missing from error: %q", result.ErrorMessage)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("request count = %d, want 1", got)
			}
		})
	}
}
