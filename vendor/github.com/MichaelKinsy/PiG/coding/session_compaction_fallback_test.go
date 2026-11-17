package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/modelroles"
)

func TestSessionCompactFallbackIntegration(t *testing.T) {
	fallbacks := []string{"fallback-b/second:off", "fallback-c/third:off"}
	cases := []compactionFallbackCase{
		{
			name:         "compact role chain",
			thirdHealthy: true,
			wantSummary:  true,
			wantModels:   []string{"chat", "chat", "primary", "second", "third"},
		},
		{
			name:         "retry disabled",
			disableRetry: true,
			thirdHealthy: true,
			wantModels:   []string{"chat", "chat", "primary"},
		},
		{
			name:                 "model fallback disabled",
			disableModelFallback: true,
			thirdHealthy:         true,
			wantModels:           []string{"chat", "chat", "primary"},
		},
		{
			name:         "fallback on usage limit",
			fallbackOn:   "usage-limit",
			thirdHealthy: true,
			wantSummary:  true,
			wantModels:   []string{"chat", "chat", "primary", "second", "third"},
		},
		{
			name:         "exclude usage limit",
			fallbackOn:   "except-usage-limit",
			thirdHealthy: true,
			wantModels:   []string{"chat", "chat", "primary"},
		},
		{
			name:          "unassigned compact role uses active role chain",
			noCompactRole: true,
			thirdHealthy:  true,
			wantSummary:   true,
			wantModels:    []string{"primary", "primary", "primary", "second", "third"},
		},
		{
			name:              "empty compact chain prevents inheritance",
			emptyCompactChain: true,
			thirdHealthy:      true,
			wantModels:        []string{"chat", "chat", "primary"},
		},
		{
			name:            "empty model chain overrides unassigned compact chain",
			noCompactRole:   true,
			emptyModelChain: true,
			thirdHealthy:    true,
			wantModels:      []string{"primary", "primary", "primary"},
		},
		{
			name:       "chain exhausted",
			wantModels: []string{"chat", "chat", "primary", "second", "third"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCompactionFallbackFixture(t, tc, fallbacks, false)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, prompt := range []string{
				"older conversation details for summary",
				"recent conversation details to retain",
			} {
				if _, err := fixture.session.Send(ctx, prompt); err != nil {
					t.Fatal(err)
				}
			}

			beforeModel := fixture.session.Model().ID
			beforeRole := fixture.session.ActiveModelRole()
			beforeThinking := fixture.session.ThinkingLevel()
			fixture.beginCompaction()
			result, err := fixture.session.Compact(ctx, "")
			if tc.wantSummary {
				if err != nil {
					t.Fatal(err)
				}
				if result == nil || result.Summary != "summary from third" {
					t.Fatalf("summary result=%+v", result)
				}
			} else if err == nil || result != nil {
				t.Fatalf("expected compaction failure, result=%+v err=%v", result, err)
			}
			if got := fixture.models(); !reflect.DeepEqual(got, tc.wantModels) {
				t.Fatalf("model order=%v, want %v", got, tc.wantModels)
			}
			assertCompactionFallbackSessionUnchanged(t, fixture.session, beforeModel, beforeRole, beforeThinking)
		})
	}

	t.Run("cancellation stops fallback", func(t *testing.T) {
		tc := cases[0]
		fixture := newCompactionFallbackFixture(t, tc, fallbacks, true)
		for _, prompt := range []string{
			"older conversation details for summary",
			"recent conversation details to retain",
		} {
			if _, err := fixture.session.Send(context.Background(), prompt); err != nil {
				t.Fatal(err)
			}
		}
		beforeModel := fixture.session.Model().ID
		beforeRole := fixture.session.ActiveModelRole()
		beforeThinking := fixture.session.ThinkingLevel()
		fixture.beginCompaction()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		type compactOutcome struct {
			result *CompactionResult
			err    error
		}
		done := make(chan compactOutcome, 1)
		go func() {
			result, err := fixture.session.Compact(ctx, "")
			done <- compactOutcome{result: result, err: err}
		}()
		select {
		case <-fixture.secondStarted:
			cancel()
		case <-time.After(3 * time.Second):
			cancel()
			outcome := <-done
			t.Fatalf("second fallback was not requested: err=%v models=%v", outcome.err, fixture.models())
		}
		select {
		case outcome := <-done:
			if outcome.err == nil || outcome.result != nil {
				t.Fatalf("expected cancelled compaction, result=%+v err=%v", outcome.result, outcome.err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("compaction did not stop after cancellation")
		}
		if got, want := fixture.models(), []string{"chat", "chat", "primary", "second"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("model order=%v, want %v", got, want)
		}
		assertCompactionFallbackSessionUnchanged(t, fixture.session, beforeModel, beforeRole, beforeThinking)
	})
}

type compactionFallbackCase struct {
	name                 string
	noCompactRole        bool
	emptyCompactChain    bool
	emptyModelChain      bool
	disableRetry         bool
	disableModelFallback bool
	fallbackOn           string
	thirdHealthy         bool
	wantSummary          bool
	wantModels           []string
}

type compactionFallbackFixture struct {
	session       *Session
	compacting    atomic.Bool
	requestedMu   sync.Mutex
	requested     []string
	secondStarted chan struct{}
}

func newCompactionFallbackFixture(t *testing.T, tc compactionFallbackCase, fallbacks []string, blockSecond bool) *compactionFallbackFixture {
	t.Helper()
	t.Setenv("PI_OFFLINE", "1")
	t.Setenv("PIG_HOME", t.TempDir())

	fixture := &compactionFallbackFixture{}
	if blockSecond {
		fixture.secondStarted = make(chan struct{})
	}
	var secondOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fixture.requestedMu.Lock()
		fixture.requested = append(fixture.requested, body.Model)
		fixture.requestedMu.Unlock()

		if !fixture.compacting.Load() && (body.Model == "chat" || body.Model == "primary") {
			writeCompactionFallbackSSE(w, body.Model, "seeded")
			return
		}
		switch body.Model {
		case "primary":
			writeCompactionFallbackQuotaError(w)
		case "second":
			if blockSecond {
				secondOnce.Do(func() { close(fixture.secondStarted) })
				<-r.Context().Done()
				return
			}
			writeCompactionFallbackQuotaError(w)
		case "third":
			if tc.thirdHealthy {
				writeCompactionFallbackSSE(w, body.Model, "summary from third")
				return
			}
			writeCompactionFallbackQuotaError(w)
		default:
			t.Errorf("unexpected model request %q", body.Model)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	providers := map[string]any{}
	for provider, ids := range map[string][]string{
		"seed":       {"chat", "primary"},
		"fallback-a": {"primary"},
		"fallback-b": {"second"},
		"fallback-c": {"third"},
	} {
		models := make([]any, 0, len(ids))
		for _, id := range ids {
			models = append(models, map[string]any{
				"id": id, "name": id, "reasoning": true, "contextWindow": 128000, "maxTokens": 1024,
			})
		}
		providers[provider] = map[string]any{
			"api": "openai-completions", "baseUrl": server.URL + "/v1", "apiKey": "fixture-key", "models": models,
		}
	}
	config, err := json.Marshal(map[string]any{"providers": providers})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.json"), config, 0600); err != nil {
		t.Fatal(err)
	}

	activeModel := "seed/chat"
	roles := map[string]string{"default": activeModel}
	chainKey := "compact"
	if tc.noCompactRole {
		activeModel = "seed/primary"
		roles["default"] = activeModel
		chainKey = "default"
	} else {
		roles["compact"] = "fallback-a/primary"
	}
	chains := map[string][]string{chainKey: append([]string(nil), fallbacks...)}
	if tc.emptyCompactChain {
		chains["default"] = append([]string(nil), fallbacks...)
		chains["compact"] = []string{}
	}
	if tc.emptyModelChain {
		chains["compact"] = append([]string(nil), fallbacks...)
		chains[activeModel] = []string{}
	}
	retryEnabled := !tc.disableRetry
	retry := &icodingagent.RetrySettingsJSON{
		Enabled:        &retryEnabled,
		MaxRetries:     new(0),
		FallbackChains: chains,
		FallbackOn:     tc.fallbackOn,
	}
	if tc.disableModelFallback {
		retry.ModelFallback = new(false)
	}
	keepRecentTokens := float64(11)
	settings := Settings{
		Config: modelroles.Config{ModelRoles: roles},
		Retry:  retry,
		Compaction: &icodingagent.CompactionSettingsJSON{
			KeepRecentTokens: &keepRecentTokens,
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
	fixture.session, err = NewSession(services, SessionOptions{
		Model: model, ModelRole: "default", ThinkingLevel: "high", NoSession: true,
		NoTools: "all", ResourceLoader: NoResources,
	})
	if err != nil {
		services.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fixture.session.Close()
		services.Close()
	})
	return fixture
}

func (f *compactionFallbackFixture) beginCompaction() {
	f.compacting.Store(true)
}

func (f *compactionFallbackFixture) models() []string {
	f.requestedMu.Lock()
	defer f.requestedMu.Unlock()
	return append([]string(nil), f.requested...)
}

func writeCompactionFallbackQuotaError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprint(w, `{"error":{"message":"billing quota exceeded","type":"insufficient_quota"}}`)
}

func writeCompactionFallbackSSE(w http.ResponseWriter, model, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":null}]}\n\n", model, content)
	fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
}

func assertCompactionFallbackSessionUnchanged(t *testing.T, session *Session, model, role string, thinking ai.ModelThinkingLevel) {
	t.Helper()
	if session.Model().ID != model || session.ActiveModelRole() != role || session.ThinkingLevel() != thinking {
		t.Fatalf("session state changed: model=%s role=%s thinking=%s", session.Model().ID, session.ActiveModelRole(), session.ThinkingLevel())
	}
}
