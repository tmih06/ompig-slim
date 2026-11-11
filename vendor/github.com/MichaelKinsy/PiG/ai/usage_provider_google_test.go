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

// TestRemoteBrokerRefreshMarkerIsNotUsedForCopilotUsage ensures a broker's
// redacted refresh token never becomes the bearer sent to GitHub.
func TestRemoteBrokerRefreshMarkerIsNotUsedForCopilotUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("authorization = %q, want access token only", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	stored := &Credential{
		Type:    CredentialOAuth,
		Access:  "access-token",
		Refresh: RemoteRefreshSentinel,
	}
	report, err := fetchGitHubCopilotUsage(context.Background(), UsageFetchParams{
		Provider:   "github-copilot",
		BaseURL:    server.URL,
		Credential: UsageCredentialFromStored(stored),
	}, UsageFetchContext{HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if report == nil {
		t.Fatal("remote OAuth credential produced no Copilot usage report")
	}
}

// TestAntigravityUsageResponseCompression mirrors OMP fetch's transparent
// decompression for both quota endpoints. A real HTTP transport is required:
// a mock RoundTripper would bypass Go's automatic gzip negotiation and decoding.
func TestAntigravityUsageResponseCompression(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, compressed := range []bool{false, true} {
			name := "summary"
			if legacy {
				name = "legacy"
			}
			if compressed {
				name += "/gzip"
			} else {
				name += "/plain"
			}
			t.Run(name, func(t *testing.T) {
				var paths []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					paths = append(paths, r.URL.Path)
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("quota request must POST with the fixture bearer token")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if legacy && r.URL.Path == antigravityQuotaSummaryPath {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					payload := `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.75}]}]}`
					if legacy {
						payload = `{"models":{"gemini-test":{"modelProvider":"MODEL_PROVIDER_GOOGLE","weeklyQuotaInfo":{"remainingFraction":0.75}}}}`
					}
					w.Header().Set("Content-Type", "application/json")
					var body io.Writer = w
					if compressed {
						if r.Header.Get("Accept-Encoding") != "gzip" {
							t.Error("transport must negotiate gzip")
						}
						w.Header().Set("Content-Encoding", "gzip")
						writer := gzip.NewWriter(w)
						defer func() {
							if err := writer.Close(); err != nil {
								t.Errorf("close gzip response: %v", err)
							}
						}()
						body = writer
					}
					if _, err := io.WriteString(body, payload); err != nil {
						t.Errorf("write quota response: %v", err)
					}
				}))
				defer server.Close()

				report, err := fetchAntigravityUsage(context.Background(), UsageFetchParams{
					Provider: "google-antigravity",
					BaseURL:  server.URL,
					Credential: UsageCredential{
						AccessToken: "test-token",
						ProjectID:   "test-project",
					},
				}, UsageFetchContext{HTTPClient: server.Client()})
				if err != nil {
					t.Fatal(err)
				}
				if report == nil || len(report.Limits) != 1 {
					t.Fatalf("report = %+v, want one quota limit", report)
				}
				limit := report.Limits[0]
				if limit.Amount.RemainingFraction == nil || *limit.Amount.RemainingFraction != 0.75 || limit.Window == nil || limit.Window.ID != "weekly" {
					t.Fatalf("limit = %+v, want 75%% remaining in weekly window", limit)
				}
				wantPaths := antigravityQuotaSummaryPath
				if legacy {
					wantPaths += "," + antigravityAvailableModelsPath
				}
				if got := strings.Join(paths, ","); got != wantPaths {
					t.Fatalf("request paths = %q, want %q", got, wantPaths)
				}
			})
		}
	}
}
