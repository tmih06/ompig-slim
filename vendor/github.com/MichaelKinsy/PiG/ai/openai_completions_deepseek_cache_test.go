package ai_test

// The DeepSeek prompt-cache accounting of the openai-completions stream.
//
// DeepSeek reports prompt-cache hits as `prompt_cache_hit_tokens` and misses as
// `prompt_cache_miss_tokens`, while OpenAI-shaped proxies in front of it add an
// empty `prompt_tokens_details.cached_tokens`. The parser must read the first
// field that actually carries hits, or a proxy's 0 hides the real count and every
// turn is estimated and reported as a full cache miss.
//
// These cases run the real native stream against a local server, so the typed
// chunk decoder and the JavaScript-compatibility reader of mistyped chunks are
// both exercised.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// deepSeekUsageStream serves one chat-completions SSE stream whose single usage
// chunk carries the given usage object verbatim, then a stop chunk.
func deepSeekUsageStream(t *testing.T, usage string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(writer, "data: {\"id\":\"chatcmpl-1\",\"choices\":[],\"usage\":%s}\n\n", usage)
		fmt.Fprint(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server
}

// deepSeekUsageFor streams one DeepSeek request whose usage chunk is usage and
// returns the usage the native stream reports.
func deepSeekUsageFor(t *testing.T, usage string) ai.Usage {
	t.Helper()
	server := deepSeekUsageStream(t, usage)
	provider := ai.NewOpenAIProvider(ai.OpenAIConfig{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		Model:      "deepseek-flash",
		ProviderID: "deepseek",
	})
	stream, err := provider.Stream(context.Background(), ai.NormalizeContext(ai.Context{
		Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi"), Timestamp: 1}},
	}), ai.StreamOptions{})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	var final *ai.AssistantMessage
	for event := range stream.Events(context.Background()) {
		switch typed := event.(type) {
		case ai.DoneEvent:
			final = typed.Message
		case ai.ErrorEvent:
			final = typed.Error
		}
	}
	if final == nil {
		t.Fatal("stream ended without a terminal event")
	}
	if final.StopReason != ai.StopReasonStop {
		t.Fatalf("stop reason = %q (error %q), want stop", final.StopReason, final.ErrorMessage)
	}
	return final.Usage
}

// TestDeepSeekCacheReadTakesFirstPositiveField is the regression: a proxy zero
// must not hide DeepSeek's own hit count, and the uncached input keeps the
// subtraction so the prompt is not billed twice.
func TestDeepSeekCacheReadTakesFirstPositiveField(t *testing.T) {
	usage := deepSeekUsageFor(t, `{"prompt_tokens":1000,"completion_tokens":100,`+
		`"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":800}`)
	if usage.CacheRead != 800 {
		t.Fatalf("CacheRead = %d, want 800 (prompt_cache_hit_tokens)", usage.CacheRead)
	}
	if usage.Input != 200 {
		t.Fatalf("Input = %d, want 200 (1000 prompt tokens - 800 cache hits)", usage.Input)
	}
	if usage.TotalTokens != 1100 {
		t.Fatalf("TotalTokens = %d, want 1100", usage.TotalTokens)
	}
	if usage.Output != 100 {
		t.Fatalf("Output = %d, want 100", usage.Output)
	}
}

// TestDeepSeekCacheReadPrefersDetailFieldWhenPositive keeps the stock OpenAI and
// OpenRouter shape: a positive prompt_tokens_details.cached_tokens is the hit
// count even when a DeepSeek-only field is also present.
func TestDeepSeekCacheReadPrefersDetailFieldWhenPositive(t *testing.T) {
	usage := deepSeekUsageFor(t, `{"prompt_tokens":1000,"completion_tokens":10,`+
		`"prompt_tokens_details":{"cached_tokens":600},"prompt_cache_hit_tokens":500}`)
	if usage.CacheRead != 600 {
		t.Fatalf("CacheRead = %d, want 600 (prompt_tokens_details.cached_tokens)", usage.CacheRead)
	}
	if usage.Input != 400 {
		t.Fatalf("Input = %d, want 400", usage.Input)
	}
}

// TestDeepSeekCacheWriteStaysOutOfCacheRead is the cache-write half: a write
// count is subtracted from the input but is not a read, and it does not make the
// detail cached_tokens zero count as hits.
func TestDeepSeekCacheWriteStaysOutOfCacheRead(t *testing.T) {
	usage := deepSeekUsageFor(t, `{"prompt_tokens":1000,"completion_tokens":10,`+
		`"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":40},"prompt_cache_hit_tokens":800}`)
	if usage.CacheRead != 800 || usage.CacheWrite != 40 {
		t.Fatalf("CacheRead/CacheWrite = %d/%d, want 800/40", usage.CacheRead, usage.CacheWrite)
	}
	if usage.Input != 160 {
		t.Fatalf("Input = %d, want 160 (1000 - 800 read - 40 write)", usage.Input)
	}
	if usage.TotalTokens != 1010 {
		t.Fatalf("TotalTokens = %d, want 1010", usage.TotalTokens)
	}
}

// TestDeepSeekCacheReadFallsBackToTopLevelCachedTokens covers Kimi's documented
// placement, which the same candidate order accepts when the other two are absent.
func TestDeepSeekCacheReadFallsBackToTopLevelCachedTokens(t *testing.T) {
	usage := deepSeekUsageFor(t, `{"prompt_tokens":500,"completion_tokens":5,"cached_tokens":300}`)
	if usage.CacheRead != 300 {
		t.Fatalf("CacheRead = %d, want 300 (top-level cached_tokens)", usage.CacheRead)
	}
	if usage.Input != 200 {
		t.Fatalf("Input = %d, want 200", usage.Input)
	}
}

// TestDeepSeekCacheReadWithoutHitsIsZero is the no-hits case: reported zeros (or
// absent fields) leave the whole prompt uncached rather than inventing hits.
func TestDeepSeekCacheReadWithoutHitsIsZero(t *testing.T) {
	usage := deepSeekUsageFor(t, `{"prompt_tokens":400,"completion_tokens":7,`+
		`"prompt_tokens_details":{"cached_tokens":0},"prompt_cache_hit_tokens":0}`)
	if usage.CacheRead != 0 {
		t.Fatalf("CacheRead = %d, want 0", usage.CacheRead)
	}
	if usage.Input != 400 || usage.TotalTokens != 407 {
		t.Fatalf("Input/TotalTokens = %d/%d, want 400/407", usage.Input, usage.TotalTokens)
	}
}
