package coding

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// openCodeFreeLimitTransport returns the gateway's real free-tier 429: the
// FreeUsageLimitError body and the Retry-After header that carries the reset.
type openCodeFreeLimitTransport struct{}

func (openCodeFreeLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
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
	options := ai.StreamOptions{Fetch: &http.Client{Transport: openCodeFreeLimitTransport{}}, MaxRetries: new(0), MaxTokens: 512}
	result := runtime.StreamSimple(context.Background(), model, request, options).Result()
	if result == nil || result.StopReason != ai.StopReasonError {
		t.Fatalf("quota error not returned: %+v", result)
	}
	if !strings.Contains(result.ErrorMessage, "Free usage limit reached") {
		t.Fatalf("error lost the provider message: %q", result.ErrorMessage)
	}
	if !strings.Contains(result.ErrorMessage, "resets in") || !strings.Contains(result.ErrorMessage, "1h") {
		t.Fatalf("error did not show time left to reset: %q", result.ErrorMessage)
	}
}
