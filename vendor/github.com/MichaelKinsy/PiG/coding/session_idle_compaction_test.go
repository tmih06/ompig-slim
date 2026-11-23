package coding

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/coding/extension/host/inproc"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

type idleCapturedRequest struct {
	Model    string
	Messages []json.RawMessage
	Raw      []byte
}

type idleFixture struct {
	session   *Session
	sessionID string
	dir       string

	mu       sync.Mutex
	requests []idleCapturedRequest

	summaryStarted  chan struct{}
	summaryStartOne sync.Once
	blockSummary    chan struct{} // non-nil: summary request blocks until closed

	// pauseResponses parks the next non-summary provider request until
	// releaseResponses (or request cancellation); responseHeld closes when it
	// reached the provider.
	pauseResponses atomic.Bool
	responseHeld   chan struct{}
	responseHeldDo sync.Once
	release        chan struct{}
	releaseOnce    sync.Once

	compactionEnd chan agent.CompactionEndEvent
}

func newIdleFixture(t *testing.T, promptCache map[string]float64, idleDelay float64, cacheWarming icodingagent.CacheWarmingMode, blockSummary bool) *idleFixture {
	return newIdleFixtureRunner(t, promptCache, idleDelay, cacheWarming, blockSummary, nil)
}

func newIdleFixtureRunner(t *testing.T, promptCache map[string]float64, idleDelay float64, cacheWarming icodingagent.CacheWarmingMode, blockSummary bool, runner *inproc.Runner) *idleFixture {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())

	f := &idleFixture{
		summaryStarted: make(chan struct{}),
		responseHeld:   make(chan struct{}),
		release:        make(chan struct{}),
		compactionEnd:  make(chan agent.CompactionEndEvent, 4),
	}
	if blockSummary {
		f.blockSummary = make(chan struct{})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string            `json:"model"`
			Messages []json.RawMessage `json:"messages"`
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(raw, &body) != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		summary := len(body.Messages) > 0 && strings.Contains(string(body.Messages[len(body.Messages)-1]), "conversation to summarize")
		f.mu.Lock()
		f.requests = append(f.requests, idleCapturedRequest{Model: body.Model, Messages: body.Messages, Raw: raw})
		f.mu.Unlock()
		if summary {
			f.summaryStartOne.Do(func() { close(f.summaryStarted) })
			if f.blockSummary != nil {
				select {
				case <-f.blockSummary:
				case <-r.Context().Done():
					return
				}
			}
			writeCompactionFallbackSSE(w, body.Model, "idle summary")
			return
		}
		if f.pauseResponses.Load() {
			f.responseHeldDo.Do(func() { close(f.responseHeld) })
			select {
			case <-f.release:
			case <-r.Context().Done():
				return
			}
		}
		writeCompactionFallbackSSE(w, body.Model, "seeded")
	}))
	t.Cleanup(server.Close)

	modelEntry := map[string]any{
		"id": "chat", "name": "chat", "reasoning": false, "contextWindow": 128000, "maxTokens": 1024,
	}
	if promptCache != nil {
		modelEntry["promptCache"] = promptCache
	}
	config, err := json.Marshal(map[string]any{"providers": map[string]any{
		"fixture": map[string]any{
			"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key",
			"models": []any{modelEntry},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	f.dir = dir

	keepRecentTokens := float64(11)
	settings := Settings{
		Config:       modelroles.Config{ModelRoles: map[string]string{"default": "fixture/chat"}},
		CacheWarming: cacheWarming,
		Retry:        &icodingagent.RetrySettingsJSON{MaxRetries: new(0)},
		Compaction: &icodingagent.CompactionSettingsJSON{
			KeepRecentTokens: &keepRecentTokens,
			IdleDelaySeconds: &idleDelay,
		},
	}
	services, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: dir, SettingsManager: NewInMemorySettingsManager(settings),
	})
	if err != nil {
		t.Fatal(err)
	}
	model, err := BuildModel("@default", services)
	if err != nil {
		services.Close()
		t.Fatal(err)
	}
	sessionDir := t.TempDir()
	f.session, err = NewSession(services, SessionOptions{
		Model: model, ModelRole: "default", SessionDir: sessionDir,
		NoTools: "all", ResourceLoader: NoResources, Runner: runner,
	})
	if err != nil {
		services.Close()
		t.Fatal(err)
	}
	f.sessionID = f.session.ID()
	f.session.Subscribe(func(event agent.AgentEvent) {
		if end, ok := event.(agent.CompactionEndEvent); ok && end.Reason == "idle" {
			select {
			case f.compactionEnd <- end:
			default:
			}
		}
	})
	t.Cleanup(func() {
		f.session.Close()
		services.Close()
	})
	return f
}

func (f *idleFixture) send(t *testing.T, ctx context.Context) {
	t.Helper()
	for _, prompt := range []string{
		"older conversation details for summary",
		"recent conversation details to retain",
	} {
		if _, err := f.session.Send(ctx, prompt); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *idleFixture) captured() []idleCapturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]idleCapturedRequest(nil), f.requests...)
}

func (f *idleFixture) summaryRequest(t *testing.T) idleCapturedRequest {
	t.Helper()
	for _, req := range f.captured() {
		if len(req.Messages) > 0 && strings.Contains(string(req.Messages[len(req.Messages)-1]), "conversation to summarize") {
			return req
		}
	}
	t.Fatal("no summary request captured")
	return idleCapturedRequest{}
}

func (f *idleFixture) compactionEntry() *icodingagent.CompactionEntry {
	return icodingagent.GetLatestCompactionEntry(f.session.Inner().GetBranch())
}

// holdResponses parks the next non-summary provider request until
// releaseResponses. The returned channel closes once the request reached the
// provider, so a test knows the run is in flight.
func (f *idleFixture) holdResponses() <-chan struct{} {
	f.pauseResponses.Store(true)
	return f.responseHeld
}

func (f *idleFixture) releaseResponses() {
	f.pauseResponses.Store(false)
	f.releaseOnce.Do(func() { close(f.release) })
}

func (f *idleFixture) assertNoSummary(t *testing.T, why string) {
	t.Helper()
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatalf("idle summary request ran after %s", why)
		}
	}
	if f.compactionEntry() != nil {
		t.Fatalf("idle compaction persisted after %s", why)
	}
}

func waitIdleCompactionEnd(t *testing.T, f *idleFixture, d time.Duration) agent.CompactionEndEvent {
	t.Helper()
	select {
	case end := <-f.compactionEnd:
		return end
	case <-time.After(d):
		t.Fatal("no idle compaction_end event")
		return agent.CompactionEndEvent{}
	}
}

func assertPrefixSame(t *testing.T, prior, summary idleCapturedRequest) {
	t.Helper()
	if len(summary.Messages) != len(prior.Messages)+2 {
		t.Fatalf("summary carries %d messages, want prefix %d + final response + instruction", len(summary.Messages), len(prior.Messages))
	}
	for i, m := range prior.Messages {
		if string(summary.Messages[i]) != string(m) {
			t.Fatalf("summary message %d differs from preserved prefix:\n got %s\nwant %s", i, summary.Messages[i], m)
		}
	}
	var penultimate struct{ Role string }
	if err := json.Unmarshal(summary.Messages[len(summary.Messages)-2], &penultimate); err != nil || penultimate.Role != "assistant" {
		t.Fatalf("appended message before instruction is not the assistant response: %s", summary.Messages[len(summary.Messages)-2])
	}
	var last struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(summary.Messages[len(summary.Messages)-1], &last); err != nil || last.Role != "user" {
		t.Fatalf("summary instruction missing: %s", summary.Messages[len(summary.Messages)-1])
	}
	if !strings.Contains(last.Content[0].Text, "conversation to summarize") {
		t.Fatalf("summary instruction text unexpected: %q", last.Content[0].Text)
	}
}

func TestSessionIdleCompactionMetadataDeadline(t *testing.T) {
	f := newIdleFixture(t, map[string]float64{"short": 11}, 240, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
	entry := f.compactionEntry()
	if entry == nil || !strings.Contains(entry.Summary, "idle summary") {
		t.Fatalf("no persisted idle compaction entry: %+v", entry)
	}
	assertPrefixSame(t, f.captured()[1], f.summaryRequest(t))

	if _, err := f.session.Send(ctx, "continue after idle compaction"); err != nil {
		t.Fatal(err)
	}
	last := f.captured()[len(f.captured())-1]
	if !strings.Contains(string(last.Raw), "idle summary") {
		t.Fatal("next prompt did not carry the compacted context")
	}
	if n := len(f.session.Messages()); n < 2 || n > 6 {
		t.Fatalf("post-compaction context has %d messages, want a compacted context", n)
	}
}

func TestSessionIdleCompactionFallbackDelay(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
	if f.compactionEntry() == nil {
		t.Fatal("no persisted idle compaction entry")
	}
	assertPrefixSame(t, f.captured()[1], f.summaryRequest(t))
}

func TestSessionIdleCompactionAfterFirstSettleAtMostOnce(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)
	waitIdleCompactionEnd(t, f, 10*time.Second)
	before := len(f.captured())

	time.Sleep(1200 * time.Millisecond)
	if _, err := f.session.Send(ctx, "another turn"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	summaries := 0
	for _, req := range f.captured()[before:] {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			summaries++
		}
	}
	if summaries > 1 {
		t.Fatalf("idle compaction ran %d times for one request, want at most 1", summaries)
	}
}

func TestSessionIdleCompactionInputPreemptsCommit(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	select {
	case <-f.summaryStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("idle summary request never started")
	}
	if !f.session.IsCompacting() {
		t.Fatal("expected the idle claim to report IsCompacting")
	}
	close(f.blockSummary)
	if _, err := f.session.Send(ctx, "input during idle compaction"); err != nil {
		t.Fatalf("prompt during idle compaction failed: %v", err)
	}
	if entry := f.compactionEntry(); entry != nil {
		t.Fatalf("stale idle compaction committed on top of user input: %+v", entry)
	}
	branch := f.session.Inner().GetBranch()
	var lastType string
	if n := len(branch); n > 0 {
		lastType = branch[n-1].Base().Type
	}
	if lastType == "compaction" {
		t.Fatal("compaction landed after the user message")
	}
}

func TestSessionIdleCompactionDisabled(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	if err := f.session.SettingsManager().SetCompactionIdleEnabled(false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.send(t, ctx)

	time.Sleep(1500 * time.Millisecond)
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("idle summary request ran with idleEnabled=false")
		}
	}
	if f.compactionEntry() != nil {
		t.Fatal("idle compaction persisted with idleEnabled=false")
	}
}

func TestSessionIdleCompactionCacheWarmingOff(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("off"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
}

func TestSessionIdleCompactionCompactionDisabled(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	if err := f.session.SettingsManager().SetCompactionEnabled(false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.send(t, ctx)

	time.Sleep(1500 * time.Millisecond)
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("idle summary request ran with compaction.enabled=false")
		}
	}
}

func TestSessionIdleCompactionTinySessionSkipped(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := f.session.Send(ctx, "tiny session"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("idle summary request ran on a session with nothing removable")
		}
	}
	if f.compactionEntry() != nil {
		t.Fatal("idle compaction persisted on a too-small session")
	}
}

func TestSessionIdleCompactionCompactRoleSkips(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := f.session.SettingsManager().UpdateGlobal(func(s *Settings) {
		s.ModelRoles["compact"] = "fixture/chat" // same model → control case below
	}); err != nil {
		t.Fatal(err)
	}
	f.send(t, ctx)
	waitIdleCompactionEnd(t, f, 10*time.Second)

	f2 := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	if err := f2.session.SettingsManager().UpdateGlobal(func(s *Settings) {
		s.ModelRoles["compact"] = "fixture/other-model"
	}); err != nil {
		t.Fatal(err)
	}
	f2.send(t, ctx)
	time.Sleep(1500 * time.Millisecond)
	for _, req := range f2.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("idle summary request ran for a different @compact model")
		}
	}
}

func TestSessionIdleCompactionEndListenerCanPrompt(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var once sync.Once
	var claimHeldAtEnd atomic.Bool
	promptDone := make(chan error, 1)
	f.session.Subscribe(func(event agent.AgentEvent) {
		end, ok := event.(agent.CompactionEndEvent)
		if !ok || end.Reason != "idle" || end.Aborted {
			return
		}
		claimHeldAtEnd.Store(f.session.IsCompacting())
		once.Do(func() {
			go func() {
				_, err := f.session.Send(ctx, "prompt submitted from compaction_end listener")
				promptDone <- err
			}()
		})
	})
	f.send(t, ctx)
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("listener prompt failed: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("listener prompt deadlocked on the unreleased compaction claim")
	}
	if claimHeldAtEnd.Load() {
		t.Fatal("idle claim was still held while the success end event was emitted")
	}
	branch := f.session.Inner().GetBranch()
	var types []string
	for _, entry := range branch {
		types = append(types, entry.Base().Type)
	}
	compacted := false
	for i, typ := range types {
		if typ == "compaction" && i+1 < len(types) {
			compacted = true
		}
	}
	if !compacted {
		t.Fatalf("listener prompt was not appended after the compaction entry: %v", types)
	}
}

func TestSessionIdleCompactionAbortedEndListenerCanPrompt(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), true)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var once sync.Once
	var claimHeldAtEnd atomic.Bool
	promptDone := make(chan error, 1)
	f.session.Subscribe(func(event agent.AgentEvent) {
		end, ok := event.(agent.CompactionEndEvent)
		if !ok || end.Reason != "idle" || !end.Aborted {
			return
		}
		claimHeldAtEnd.Store(f.session.IsCompacting())
		once.Do(func() {
			go func() {
				_, err := f.session.Send(ctx, "prompt submitted from aborted listener")
				promptDone <- err
			}()
		})
	})
	f.send(t, ctx)
	select {
	case <-f.summaryStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("idle summary request never started")
	}
	f.session.AbortCompaction()
	close(f.blockSummary)
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("listener prompt failed: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("listener prompt deadlocked on the unreleased compaction claim")
	}
	if claimHeldAtEnd.Load() {
		t.Fatal("idle claim was still held while the aborted end event was emitted")
	}
}

func TestSettingsCompactionIdleRoundTrip(t *testing.T) {
	var c icodingagent.CompactionSettingsJSON
	if err := json.Unmarshal([]byte(`{"enabled":false,"idleEnabled":false,"idleDelaySeconds":1.5,"reserveTokens":100,"unknownKey":1}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Enabled == nil || *c.Enabled || c.IdleEnabled == nil || *c.IdleEnabled {
		t.Fatal("enabled/idleEnabled did not parse")
	}
	if c.IdleDelaySeconds == nil || *c.IdleDelaySeconds != 1.5 {
		t.Fatalf("idleDelaySeconds = %v", c.IdleDelaySeconds)
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"idleEnabled":false`, `"idleDelaySeconds":1.5`, `"unknownKey":1`} {
		if !strings.Contains(string(out), key) {
			t.Fatalf("marshal lost %s: %s", key, out)
		}
	}

	settings := Settings{Compaction: &icodingagent.CompactionSettingsJSON{
		IdleEnabled:      new(false),
		IdleDelaySeconds: new(float64(2)),
	}}
	sm := NewInMemorySettingsManager(settings)
	if sm.GetCompactionIdleEnabled() {
		t.Fatal("idleEnabled=false not honored")
	}
	if got := sm.GetCompactionIdleDelaySeconds(); got != 2 {
		t.Fatalf("idleDelaySeconds = %v", got)
	}
	if err := sm.SetCompactionIdleEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !sm.GetCompactionIdleEnabled() {
		t.Fatal("SetCompactionIdleEnabled did not take effect")
	}
	if err := sm.SetCompactionIdleDelaySeconds(-5); err != nil {
		t.Fatal(err)
	}
	if got := sm.GetCompactionIdleDelaySeconds(); got != 240 {
		t.Fatalf("invalid idleDelaySeconds = %v, want default 240", got)
	}
	if err := sm.SetCompactionIdleDelaySeconds(0); err != nil {
		t.Fatal(err)
	}
	if got := sm.GetCompactionIdleDelaySeconds(); got != 240 {
		t.Fatalf("zero idleDelaySeconds = %v, want default 240", got)
	}
	sm2 := NewInMemorySettingsManager(Settings{})
	if !sm2.GetCompactionIdleEnabled() {
		t.Fatal("idleEnabled defaults to true")
	}
	if got := sm2.GetCompactionIdleDelaySeconds(); got != 240 {
		t.Fatalf("default idleDelaySeconds = %v, want 240", got)
	}
}

// Idle compaction must decline once the live context is no longer the
// captured request context plus only the successful response, because the
// summary request never saw anything else.
func TestSessionIdleCompactionDeclinesPostSettleCustomMessage(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	if err := f.session.SendCustomMessage(ctx, extension.CustomMessageRef{CustomType: "note", Content: "appended after the run"}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	f.assertNoSummary(t, "an appended custom message")

	if _, err := f.session.Send(ctx, "continue after the custom message"); err != nil {
		t.Fatal(err)
	}
	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("next request's idle compaction end = %+v", end)
	}
}

func TestSessionIdleCompactionDeclinesPostSettleBashRecord(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	exit := 0
	if err := f.session.RecordBashResult("echo hi", BashResult{Output: "hi", ExitCode: &exit}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	f.assertNoSummary(t, "a recorded bash result")
}

func TestSessionIdleCompactionDeclinesContextEdit(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	var targetID string
	for _, entry := range f.session.Inner().GetBranch() {
		if entry.Base().Type == "message" {
			targetID = entry.Base().ID
		}
	}
	if targetID == "" {
		t.Fatal("no message entry to edit")
	}
	if _, err := f.session.Inner().AppendContextEdit(targetID, nil); err != nil {
		t.Fatal(err)
	}
	f.session.RefreshContext()
	time.Sleep(1500 * time.Millisecond)
	f.assertNoSummary(t, "a context edit")
}

func TestSessionIdleCompactionDeclinesAbortedResponse(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := f.session.Send(ctx, "older conversation details for summary"); err != nil {
		t.Fatal(err)
	}

	held := f.holdResponses()
	sendDone := make(chan error, 1)
	go func() {
		_, err := f.session.Send(ctx, "recent conversation details to retain")
		sendDone <- err
	}()
	select {
	case <-held:
	case <-time.After(10 * time.Second):
		t.Fatal("second prompt never reached the provider")
	}
	if err := f.session.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	f.releaseResponses()
	select {
	case <-sendDone:
	case <-time.After(10 * time.Second):
		t.Fatal("aborted send never returned")
	}

	time.Sleep(1500 * time.Millisecond)
	f.assertNoSummary(t, "an aborted final response")
}

// A settled event listener that outlives the idle deadline leaves the session
// busy: the offer must wait for the idle signal after settlement instead of
// latching a declined attempt.
func TestSessionIdleCompactionSettledHandlerCrossingDeadline(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var settleCount atomic.Int32
	releaseSettle := make(chan struct{})
	f.session.Subscribe(func(event agent.AgentEvent) {
		if _, ok := event.(agent.AgentSettledEvent); ok && settleCount.Add(1) == 2 {
			<-releaseSettle
		}
	})
	if _, err := f.session.Send(ctx, "older conversation details for summary"); err != nil {
		t.Fatal(err)
	}
	sendDone := make(chan error, 1)
	go func() {
		_, err := f.session.Send(ctx, "recent conversation details to retain")
		sendDone <- err
	}()
	select {
	case <-sendDone:
		t.Fatal("send returned while its settled listener was parked")
	case <-time.After(1200 * time.Millisecond):
		// The 1s fallback deadline fired during the parked settle: the offer
		// found the session busy and stays pending.
	}
	close(releaseSettle)
	select {
	case <-sendDone:
	case <-time.After(10 * time.Second):
		t.Fatal("send never returned after the settled listener finished")
	}
	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
	if f.compactionEntry() == nil {
		t.Fatal("no persisted idle compaction entry")
	}
}

// A known cache deadline anchors at the request, not at settle: a request
// whose stream outlived it still offers the deadline once the run settles.
func TestSessionIdleCompactionMissedDeadlineOffersAfterSettle(t *testing.T) {
	f := newIdleFixture(t, map[string]float64{"short": 11}, 240, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := f.session.Send(ctx, "older conversation details for summary"); err != nil {
		t.Fatal(err)
	}
	held := f.holdResponses()
	sendStarted := time.Now()
	go func() {
		_, _ = f.session.Send(ctx, "recent conversation details to retain")
	}()
	select {
	case <-held:
	case <-time.After(10 * time.Second):
		t.Fatal("second prompt never reached the provider")
	}
	// Hold the response across the request-anchored deadline (~1s after the
	// request); the settled-idle anchor would schedule the offer later.
	time.Sleep(700 * time.Millisecond)
	f.releaseResponses()

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
	if elapsed := time.Since(sendStarted); elapsed > 3*time.Second {
		t.Fatalf("offer anchored at settle, not the missed request deadline (%v)", elapsed)
	}
	if f.compactionEntry() == nil {
		t.Fatal("no persisted idle compaction entry")
	}
}

// A listener for compaction_start may preempt or prompt without deadlock: the
// start event is queued while the claim is held, so it cannot wait on a
// listener that waits on the claim.
func TestSessionIdleCompactionStartListenerPreempts(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), true)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var once sync.Once
	var claimHeldAtStart atomic.Bool
	cancelled := make(chan bool, 1)
	promptDone := make(chan error, 1)
	f.session.Subscribe(func(event agent.AgentEvent) {
		if _, ok := event.(agent.CompactionStartEvent); !ok {
			return
		}
		claimHeldAtStart.Store(f.session.IsCompacting())
		once.Do(func() {
			cancelled <- f.session.CancelIdleCompaction(ctx)
			go func() {
				_, err := f.session.Send(ctx, "prompt submitted from the start listener")
				promptDone <- err
			}()
		})
	})
	f.send(t, ctx)

	select {
	case ok := <-cancelled:
		if !ok {
			t.Fatal("start listener could not preempt the idle claim")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("CancelIdleCompaction deadlocked on the claim-held start event")
	}
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("listener prompt failed: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("prompt deadlocked behind the claim-held start event")
	}
	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if !end.Aborted {
		t.Fatalf("preempted idle compaction end = %+v, want aborted", end)
	}
	if !claimHeldAtStart.Load() {
		t.Fatal("the idle claim was not held while the start event was emitted")
	}
}

// session_before_compact sees the idle compaction and can cancel it.
func TestSessionIdleCompactionBeforeCompactCancel(t *testing.T) {
	var sawReason atomic.Value
	runner := NewExtensionRunner([]extension.Extension{{
		Name: "cancel-idle",
		Handlers: map[string][]extension.HandlerFn{
			icodingagent.EventSessionBeforeCompact: {func(args ...any) (any, error) {
				event, _ := args[0].(extension.SessionBeforeCompactEvent)
				sawReason.Store(event.Reason)
				return extension.SessionBeforeCompactResult{Cancel: true}, nil
			}},
		},
	}, nil, t.TempDir(), nil, nil)
	f := newIdleFixtureRunner(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if !end.Aborted {
		t.Fatalf("cancelled idle compaction end = %+v", end)
	}
	if f.compactionEntry() != nil {
		t.Fatal("cancelled idle compaction persisted")
	}
	f.assertNoSummary(t, "a cancelled session_before_compact")
	if reason, _ := sawReason.Load().(string); reason != "idle" {
		t.Fatalf("session_before_compact reason = %q, want idle", reason)
	}
}

// A session_before_compact result replaces the provider summary: no summary
// request runs, and the result persists with fromHook set.
func TestSessionIdleCompactionBeforeCompactCustomResult(t *testing.T) {
	runner := NewExtensionRunner([]extension.Extension{{
		Name: "custom-idle",
		Handlers: map[string][]extension.HandlerFn{
			icodingagent.EventSessionBeforeCompact: {func(args ...any) (any, error) {
				event, _ := args[0].(extension.SessionBeforeCompactEvent)
				return extension.SessionBeforeCompactResult{Compaction: &extension.CompactionResult{
					Summary:          "extension idle summary",
					FirstKeptEntryID: event.Preparation.FirstKeptEntryID,
					TokensBefore:     event.Preparation.TokensBefore,
				}}, nil
			}},
		},
	}, nil, t.TempDir(), nil, nil)
	f := newIdleFixtureRunner(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	end := waitIdleCompactionEnd(t, f, 10*time.Second)
	if end.Aborted || end.Summary != "extension idle summary" {
		t.Fatalf("idle compaction end = %+v", end)
	}
	entry := f.compactionEntry()
	if entry == nil || entry.Summary != "extension idle summary" || !entry.FromHook {
		t.Fatalf("extension idle compaction entry = %+v", entry)
	}
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("provider summary ran although the extension answered the compaction")
		}
	}
}

// Deferred custom messages still pending make the offer busy, not declined:
// once they flush the context no longer matches and the offer declines.
func TestSessionIdleCompactionPendingCustomDefersThenDeclines(t *testing.T) {
	f := newIdleFixture(t, nil, 1, icodingagent.CacheWarmingMode("streaming"), false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f.send(t, ctx)

	f.session.runState.mu.Lock()
	f.session.runState.pendingCustom = append(f.session.runState.pendingCustom, agent.AgentMessage{Custom: map[string]any{
		"role": agent.RoleCustom, "customType": "note", "content": "held", "display": true, "timestamp": time.Now().UnixMilli(),
	}})
	f.session.runState.mu.Unlock()

	f.session.notifyWarmerIdle()
	time.Sleep(200 * time.Millisecond)
	for _, req := range f.captured() {
		if strings.Contains(string(req.Raw), "conversation to summarize") {
			t.Fatal("idle summary ran while deferred custom messages were pending")
		}
	}
	if err := f.session.FlushDeferredCustomMessages(); err != nil {
		t.Fatal(err)
	}
	f.session.notifyWarmerIdle()
	time.Sleep(1200 * time.Millisecond)
	f.assertNoSummary(t, "a flushed deferred custom message")
}
