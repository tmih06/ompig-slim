package workdesk_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ompig-slim/internal/workdesk"
)

// closedOutput models a client losing its response after admission and execution.
type closedOutput struct{}

// Write rejects the response without influencing the durable assignment.
func (closedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestLostReceiptDoesNotRepeatWork distinguishes output delivery from committed
// execution: resuming after a disconnected output must return the stored result.
func TestLostReceiptDoesNotRepeatWork(t *testing.T) {
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); answer(w, "Committed answer") })
	err := workdesk.Run(context.Background(), []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "lost-receipt", "Finish"}, closedOutput{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("lost response: %v", err)
	}
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !strings.Contains(out.String(), "Committed answer") {
		t.Fatalf("repeated after lost receipt: %d %s", calls.Load(), out.String())
	}
}

// TestProviderFailureIsTerminal ensures a definitive provider rejection is not
// reported as completed or replayed indefinitely by a restart policy.
func TestProviderFailureIsTerminal(t *testing.T) {
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid fixture request","type":"invalid_request_error"}}`)
	})
	for _, args := range [][]string{
		{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "provider-failure", "Finish"},
		{"resume", "--state", state},
	} {
		var out bytes.Buffer
		if err := workdesk.Run(context.Background(), args, &out); err == nil || !strings.Contains(out.String(), "model_error") {
			t.Fatalf("provider rejection: %v %s", err, out.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("terminal failure was retried: %d", calls.Load())
	}
	// Status remains available without credentials or provider configuration.
	if err := os.Remove(filepath.Join(os.Getenv("OMPIG_SLIM_CODING_AGENT_DIR"), "models.json")); err != nil {
		t.Fatal(err)
	}
	if err := workdesk.Run(context.Background(), []string{"status", "--state", state}, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
}

// TestInvalidStateIsRefused prevents a newer or damaged host format from being
// interpreted as a different assignment during recovery.
func TestInvalidStateIsRefused(t *testing.T) {
	for _, raw := range []string{`{`, `{"version":2}`} {
		state := t.TempDir()
		if err := os.WriteFile(filepath.Join(state, "job.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"resume", "status"} {
			if err := workdesk.Run(context.Background(), []string{command, "--state", state}, new(bytes.Buffer)); err == nil {
				t.Fatalf("accepted state %s", raw)
			}
		}
	}
}
