package workdesk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ompig-slim/internal/workdesk"
)

// TestAutonomousEditsAndChecks verifies real coding tools keep working without
// another human prompt, and that their workspace survives opening a new host.
func TestAutonomousEditsAndChecks(t *testing.T) {
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			toolCall(w, "write", map[string]any{"path": "result.txt", "content": "fixed\n"})
		case 2:
			toolCall(w, "bash", map[string]any{"command": "test \"$(cat result.txt)\" = fixed"})
		default:
			answer(w, "Wrote result.txt and ran the check.")
		}
	})
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "edit-1", "Fix and verify"}, &out); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(cwd, "result.txt")); err != nil || string(raw) != "fixed\n" {
		t.Fatalf("workspace: %q %v", raw, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("autonomous requests: %d", calls.Load())
	}
	out.Reset()
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("completed work was repeated")
	}
}

// TestTurnBudgetSurvivesResume bounds a model which otherwise calls tools forever.
func TestTurnBudgetSurvivesResume(t *testing.T) {
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		toolCall(w, "bash", map[string]any{"command": "printf checked > check.txt"})
	})
	var out bytes.Buffer
	err := workdesk.Run(context.Background(), []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "budget-1", "--max-turns", "1", "Keep working"}, &out)
	if err == nil || !strings.Contains(out.String(), "turn_limit") {
		t.Fatalf("unbounded run: %v %s", err, out.String())
	}
	out.Reset()
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err == nil {
		t.Fatal("failed run was reported successful")
	}
	if calls.Load() != 1 {
		t.Fatalf("budget reset on reopen: %d", calls.Load())
	}
}

// TestAdmissionIntentCanResumeWithoutDatabase covers the crash window before
// the first submission: the fsynced host intent is sufficient for recovery.
func TestAdmissionIntentCanResumeWithoutDatabase(t *testing.T) {
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) { answer(w, "Admitted from intent") })
	job := map[string]any{"version": 1, "request_id": "intent-1", "prompt": "Finish", "cwd": cwd, "model": "desk-fixture/fixture", "max_turns": 10}
	raw, _ := json.Marshal(job)
	if err := os.WriteFile(filepath.Join(state, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"status", "--state", state}, &out); err != nil || !strings.Contains(out.String(), "pending") {
		t.Fatalf("intent status: %v %s", err, out.String())
	}
	out.Reset()
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"done"`) {
		t.Fatal(out.String())
	}
}

// TestCancellationPreservesPendingWork checks graceful shutdown is not an abort
// of the assignment and later starts still use its original admission key.
func TestCancellationPreservesPendingWork(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = ioReadBody(r)
		if calls.Add(1) == 1 {
			reached <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		answer(w, "Resumed after shutdown")
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- workdesk.Run(ctx, []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "cancel-1", "Finish"}, new(bytes.Buffer))
	}()
	select {
	case <-reached:
	case <-time.After(15 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancellation reported success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown did not return")
	}
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests: %d", calls.Load())
	}
}

// TestValidationAndPrivacy rejects invalid invocations without admitting work,
// and verifies the state directory does not expose transcripts on a shared volume.
func TestValidationAndPrivacy(t *testing.T) {
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) { answer(w, "done") })
	for _, args := range [][]string{nil, {"unknown"}, {"run"}, {"resume", "--state", state, "extra"}, {"run", "--state", state, "--cwd", cwd, "--model", "x", "--request-id", "id", "work"}} {
		if err := workdesk.Run(context.Background(), args, new(bytes.Buffer)); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := os.Chmod(state, 0755); err != nil {
		t.Fatal(err)
	}
	run := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "private-1", "Finish"}
	if err := workdesk.Run(context.Background(), run, new(bytes.Buffer)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{state, filepath.Join(state, "job.json"), filepath.Join(state, "session.sqlite")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Errorf("world/group readable state: %s (%v)", path, info.Mode().Perm())
		}
	}
	conflicting := append([]string{}, run...)
	conflicting[len(conflicting)-1] = "Different assignment"
	if err := workdesk.Run(context.Background(), conflicting, new(bytes.Buffer)); err == nil {
		t.Fatal("replaced assignment")
	}
}

// TestHelpDoesNotCreateState makes the opt-in mode discoverable without touching
// credentials or creating an assignment.
func TestHelpDoesNotCreateState(t *testing.T) {
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "resume") || !strings.Contains(out.String(), "--request-id") {
		t.Fatal(out.String())
	}
}
