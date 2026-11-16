package goalext

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoopbackReleaseAllAllowsLateRequests(t *testing.T) {
	model := newLoopback(t)
	// Unblock the old fixture on failure so this regression cannot hang cleanup.
	t.Cleanup(model.releaseOne)
	model.releaseAll()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Post(model.server.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request arriving after releaseAll remained blocked: %v", err)
	}
	response.Body.Close()
}
