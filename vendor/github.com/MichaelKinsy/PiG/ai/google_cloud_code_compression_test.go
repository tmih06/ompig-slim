package ai

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCCAAntigravityResponseCompression uses a real transport because Go only
// decodes gzip automatically when the transport itself negotiates compression.
// Discovery JSON, SSE, and rejection bodies must all reach their parsers decoded.
func TestCCAAntigravityResponseCompression(t *testing.T) {
	t.Setenv("PI_AI_ANTIGRAVITY_VERSION", "1.0.0")
	for _, kind := range []string{"discovery", "stream", "error"} {
		for _, compressed := range []bool{false, true} {
			name := kind + "/plain"
			if compressed {
				name = kind + "/gzip"
			}
			t.Run(name, func(t *testing.T) {
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if r.Header.Get("Accept-Encoding") != "gzip" {
						t.Error("transport must negotiate gzip")
					}
					if r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("request must carry the fixture bearer token")
					}
					payload := `{"models":{"gemini-compression-test":{"displayName":"Gemini test","model":"MODEL_TEST","maxOutputTokens":4096}}}`
					status := http.StatusOK
					w.Header().Set("Content-Type", "application/json")
					if kind == "stream" {
						w.Header().Set("Content-Type", "text/event-stream")
						payload = "data: " + `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}}` + "\n\n"
					} else if kind == "error" {
						status = http.StatusForbidden
						payload = `{"error":{"message":"fixture permission denied"}}`
					}
					var body io.Writer = w
					if compressed {
						w.Header().Set("Content-Encoding", "gzip")
						writer := gzip.NewWriter(w)
						defer func() {
							if err := writer.Close(); err != nil {
								t.Errorf("close gzip response: %v", err)
							}
						}()
						body = writer
					}
					w.WriteHeader(status)
					if _, err := io.WriteString(body, payload); err != nil {
						t.Errorf("write response: %v", err)
					}
				}))
				defer server.Close()
				if kind == "discovery" {
					primary, sandbox := googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint
					googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = server.URL, server.URL
					defer func() {
						googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint = primary, sandbox
					}()
					models, err := googleDiscoverAntigravityModels(context.Background(), &Credential{Access: "test-token", ProjectID: "test-project"})
					if err != nil {
						t.Fatal(err)
					}
					if len(models) != 1 || models[0].ModelID() != "gemini-compression-test" {
						t.Fatalf("models = %+v, want discovered Gemini model", models)
					}
				} else {
					model := ccaTestModel(GoogleAntigravityProviderID, "gemini-compression-test", server.URL, false)
					stream, err := StreamSimpleContext(context.Background(), model, ccaTestContext("hi"), StreamOptions{APIKey: ccaTestCredential})
					if err != nil {
						t.Fatal(err)
					}
					result, _ := ccaCollect(t, stream)
					if kind == "error" {
						if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "fixture permission denied") {
							t.Fatalf("result = %+v, want readable rejection", result)
						}
					} else if result.StopReason != StopReasonStop || ccaMessageText(result) != "ok" {
						t.Fatalf("result = %+v, want completed text", result)
					}
				}
				if requests != 1 {
					t.Fatalf("requests = %d, want no retry", requests)
				}
			})
		}
	}
}
