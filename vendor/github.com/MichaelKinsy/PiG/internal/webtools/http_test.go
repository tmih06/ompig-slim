package webtools

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadRetriesRefusalsAndBoundsBody(t *testing.T) {
	var agents []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.UserAgent())
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("response compression must be disabled")
		}
		if len(agents) == 1 {
			w.WriteHeader(999)
			return
		}
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()
	response, err := Load(context.Background(), server.Client(), "GET", server.URL, nil, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Body) != "abcde" || !response.Truncated {
		t.Fatalf("body cap: %+v", response)
	}
	if len(agents) != 2 || agents[0] == agents[1] {
		t.Fatalf("UA rotation: %v", agents)
	}
}

func TestLoadExplicitAgentRefusalAndErrorRedaction(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("secret echoed from provider"))
	}))
	defer server.Close()
	_, err := Load(context.Background(), server.Client(), "GET", server.URL+"?key=secret", nil, http.Header{"User-Agent": {"custom"}}, 10)
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("custom user agent retried %d times", calls)
	}
}

func TestLoadCancelsRetryAfter(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "99999")
		w.WriteHeader(http.StatusTooManyRequests)
		close(started)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-started; cancel() }()
	_, err := Load(ctx, server.Client(), "GET", server.URL, nil, nil, 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became provider failure: %v", err)
	}
}

func TestLoadCancelsStalledBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Load(ctx, server.Client(), "GET", server.URL, nil, nil, 100)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("body cancellation: %v", err)
	}
}
