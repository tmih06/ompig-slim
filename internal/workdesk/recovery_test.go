package workdesk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ompig-slim/internal/workdesk"
)

// TestWorkdeskProcess is the crashable host. The parent test owns timeouts and
// kills, and uses the same Run entry point as the executable.
func TestWorkdeskProcess(t *testing.T) {
	if os.Getenv("OMPIG_WORKDESK_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			if err := workdesk.Run(context.Background(), os.Args[i+1:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

// startHost launches an isolated real process; cleanup kills it if a test fails.
func startHost(t *testing.T, args []string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestWorkdeskProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "OMPIG_WORKDESK_HELPER=1")
	output := new(bytes.Buffer)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd, output
}

// waitForFile orders a kill after a shell side effect, not after an arbitrary delay.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// toolCall emits a complete tool turn over the production SSE protocol.
func toolCall(w http.ResponseWriter, name string, args map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	raw, _ := json.Marshal(args)
	delta, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "fixture-call", "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}}}, "finish_reason": nil}}})
	fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", delta)
}

// TestKillDuringGeneration tests process death, exclusive ownership, pending
// inspection without credentials, and recovery of the original submission.
func TestKillDuringGeneration(t *testing.T) {
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
		answer(w, "Recovered answer")
	})
	t.Cleanup(func() { close(release) })
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "crash-1", "Finish the assignment"}
	first, _ := startHost(t, args)
	select {
	case <-reached:
	case <-time.After(15 * time.Second):
		t.Fatal("model was not called")
	}
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"resume", "--state", state}, &out); err == nil || !strings.Contains(err.Error(), "active owner") {
		t.Fatalf("competing owner: %v", err)
	}
	if err := workdesk.Run(context.Background(), []string{"status", "--state", state}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"placed"`) {
		t.Fatalf("status while working: %s", out.String())
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait()
	second, output := startHost(t, []string{"resume", "--state", state})
	if err := second.Wait(); err != nil {
		t.Fatalf("recovery: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), `"status":"done"`) || calls.Load() != 2 {
		t.Fatalf("recovery calls=%d: %s", calls.Load(), output.String())
	}
}

// TestKillDuringUnsafeTool proves a started bash is not replayed. Releasing its
// original shell also models the fact that process death is not sandbox teardown.
func TestKillDuringUnsafeTool(t *testing.T) {
	var calls atomic.Int32
	effects := t.TempDir()
	marker, release := filepath.Join(effects, "effect"), filepath.Join(effects, "release")
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioReadBody(r)
		if calls.Add(1) == 1 {
			toolCall(w, "bash", map[string]any{"command": fmt.Sprintf("printf x >> '%s'; while [ ! -f '%s' ]; do sleep 0.05; done", marker, release), "timeout": 20})
			return
		}
		if !strings.Contains(body, "interrupted") {
			t.Errorf("model was not informed about the interrupted tool: %s", body)
		}
		answer(w, "Inspected interrupted work")
	})
	t.Cleanup(func() { _ = os.WriteFile(release, []byte("release"), 0600) })
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "unsafe-1", "Run the command"}
	first, _ := startHost(t, args)
	waitForFile(t, marker)
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait()
	if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	second, output := startHost(t, []string{"resume", "--state", state})
	if err := second.Wait(); err != nil {
		t.Fatalf("tool recovery: %v\n%s", err, output.String())
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "x" {
		t.Fatalf("unsafe tool replayed: %q %v", raw, err)
	}
	if !strings.Contains(output.String(), `"status":"done"`) {
		t.Fatalf("result: %s", output.String())
	}
}

// TestCrashLoopEnds verifies persisted model retry accounting bounds repeated
// host deaths. A resume is recovery, not permission to reset the retry budget.
func TestCrashLoopEnds(t *testing.T) {
	reached := make(chan struct{}, 8)
	release := make(chan struct{})
	var calls atomic.Int32
	state, cwd := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = ioReadBody(r)
		calls.Add(1)
		reached <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "crash-loop", "Finish"}
	for attempt := 0; attempt < 4; attempt++ {
		host, _ := startHost(t, args)
		select {
		case <-reached:
		case <-time.After(20 * time.Second):
			t.Fatalf("attempt %d did not reach the model", attempt+1)
		}
		if err := host.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = host.Wait()
		args = []string{"resume", "--state", state}
	}
	host, output := startHost(t, args)
	if err := host.Wait(); err == nil {
		t.Fatalf("crash loop reported success: %s", output.String())
	}
	if !strings.Contains(output.String(), `"status":"unanswered"`) || !strings.Contains(output.String(), "interrupted") || calls.Load() != 4 {
		t.Fatalf("retry budget reset: calls=%d %s", calls.Load(), output.String())
	}
}

// ioReadBody captures a small fixture request for assertions.
func ioReadBody(r *http.Request) (string, error) {
	var body json.RawMessage
	err := json.NewDecoder(r.Body).Decode(&body)
	return string(body), err
}
