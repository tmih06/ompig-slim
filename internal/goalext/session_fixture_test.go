package goalext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Exercise the production SSE provider path.
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/extension"

	"ompig-slim/internal/goal"
)

// codingPromptSteer is the prompt option a slash command typed during a run
// carries, so the command reaches extension dispatch instead of the queue.
func codingPromptSteer() *coding.PromptOptions {
	return &coding.PromptOptions{StreamingBehavior: extension.DeliverAsSteer}
}

// loopback is a model server that records every request and lets a test hold a
// response open, so a queue or interrupt lands at a chosen point in the run.
type loopback struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
	started  chan int
	release  chan struct{}
}

func newLoopback(t *testing.T) *loopback {
	t.Helper()
	l := &loopback{t: t, started: make(chan int, 64), release: make(chan struct{})}
	l.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		l.mu.Lock()
		l.requests = append(l.requests, string(body))
		n := len(l.requests)
		l.mu.Unlock()
		l.started <- n
		<-l.release
		w.Header().Set("Content-Type", "text/event-stream")
		chunk, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{
			map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": fmt.Sprintf("reply %d", n)}, "finish_reason": nil},
		}})
		fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	t.Cleanup(l.server.Close)
	return l
}

// releaseOne unblocks the request the server is holding, once.
func (l *loopback) releaseOne() {
	select {
	case l.release <- struct{}{}:
	case <-time.After(time.Second):
	}
}

// answerNoRelease returns a held request without releasing it, so a test can
// interrupt the run while that request is still in flight.
func (l *loopback) answerNoRelease(timeout time.Duration) int {
	select {
	case n := <-l.started:
		return n
	case <-time.After(timeout):
		l.t.Fatalf("no model request within %s", timeout)
		return 0
	}
}

// answer releases the request the server is holding and returns its number.
func (l *loopback) answer(timeout time.Duration) int {
	select {
	case n := <-l.started:
		l.release <- struct{}{}
		return n
	case <-time.After(timeout):
		l.t.Fatalf("no model request within %s", timeout)
		return 0
	}
}

func (l *loopback) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.requests...)
}

// goalSession drives the production Session, command runner, and settlement
// path over the loopback provider, with the goal tracker's state observable.
type goalSession struct {
	t       *testing.T
	model   *loopback
	track   *goal.Tracker
	cancel  context.CancelFunc
	session *coding.Session
	ctx     context.Context
}

// newGoalSession builds the production Session over the loopback provider, with
// the goal extension bound to a tracker the test can read.
func newGoalSession(t *testing.T) *goalSession {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())
	model := newLoopback(t)
	cwd, dir := t.TempDir(), t.TempDir()
	config := map[string]any{"providers": map[string]any{"goal-fixture": map[string]any{
		"api": "openai-completions", "baseUrl": model.server.URL + "/v1", "apiKey": "fixture-key",
		"models": []any{map[string]any{"id": "fixture", "name": "fixture", "contextWindow": 128000, "maxTokens": 1024}},
	}}}
	raw, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(dir, "models.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	settings := coding.NewInMemorySettingsManager(coding.Settings{})
	services, err := coding.CreateAgentSessionServices(coding.CreateAgentSessionServicesOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(services.Close)
	selected, err := coding.BuildModel("goal-fixture/fixture", services)
	if err != nil {
		t.Fatal(err)
	}
	tracker := goal.New()
	loader := coding.NewDefaultResourceLoader(coding.DefaultResourceLoaderOptions{
		CWD: cwd, AgentDir: dir, SettingsManager: settings,
		NoExtensions: true, NoContextFiles: true, NoSkills: true, NoThemes: true, NoPromptTemplates: true,
		ExtensionFactories: []extension.InlineExtension{extension.NamedInlineExtension{
			Name: "goal", Factory: func(api extension.API) error { return extensionWithTracker(api, tracker) }, Hidden: true,
		}},
	})
	if err := loader.Reload(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loader.Close() })
	session, err := coding.NewSession(services, coding.SessionOptions{Model: selected, ResourceLoader: loader})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() { model.releaseAll() })
	drive := &goalSession{t: t, model: model, track: tracker, cancel: cancel}
	drive.session = session
	drive.ctx = ctx
	return drive
}

// releaseAll unblocks every held response so a failed test cannot hang the suite.
func (l *loopback) releaseAll() {
	l.mu.Lock()
	held := len(l.requests)
	l.mu.Unlock()
	for i := 0; i < held+1; i++ {
		select {
		case <-l.started:
		case <-time.After(10 * time.Millisecond):
		}
		select {
		case l.release <- struct{}{}:
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// run drives one prompt off the test goroutine and returns a channel that
// reports when it settles.
func (g *goalSession) run(prompt string) <-chan error {
	done := make(chan error, 1)
	go func() { done <- g.session.Prompt(g.ctx, prompt) }()
	return done
}

func (g *goalSession) wait(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prompt failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("prompt never settled")
	}
}

// status reports the tracker state the goal tool and slash command report.
func (g *goalSession) status(t *testing.T) goal.Status {
	t.Helper()
	state := g.track.Get()
	if state == nil {
		return ""
	}
	return state.Status
}

// waitStatus waits for the tracker to reach want, so an assertion reads the
// settled lifecycle state instead of racing the settlement handler.
func (g *goalSession) waitStatus(t *testing.T, want goal.Status, timeout time.Duration) goal.Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := g.status(t)
		if got == want {
			return got
		}
		if time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// drain answers up to max held requests, then stops, so any continuation that
// survives an interruption gets the chance to start a turn and be counted.
func (l *loopback) drain(window time.Duration, max int) {
	for i := 0; i < max; i++ {
		select {
		case <-l.started:
			l.release <- struct{}{}
		case <-time.After(window):
			return
		}
	}
}

// messages returns the conversation messages of each recorded request, without
// tool definitions or other request scaffolding, so an assertion about injected
// text cannot match a schema.
func (l *loopback) messages() [][]json.RawMessage {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([][]json.RawMessage, len(l.requests))
	for i, request := range l.requests {
		var decoded struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.Unmarshal([]byte(request), &decoded); err == nil {
			out[i] = decoded.Messages
		}
	}
	return out
}

// lastMessages is the conversation of the most recent request.
func (l *loopback) lastMessages() []json.RawMessage {
	all := l.messages()
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

func countPokes(requests []string) int {
	pokes := 0
	for _, request := range requests {
		if strings.Contains(request, "Goal poke") {
			pokes++
		}
	}
	return pokes
}
