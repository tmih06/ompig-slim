package codingagent

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

func promptTestLimit(id string, fraction float64, reset int64) ai.UsageLimit {
	return ai.UsageLimit{ID: id, Scope: ai.UsageScope{WindowID: id}, Window: &ai.UsageWindow{ID: id, ResetsAt: &reset}, Amount: ai.UsageAmount{UsedFraction: &fraction}}
}

func TestPromptQuotaTotalsAndEssentialWindow(t *testing.T) {
	later := time.Now().Add(4 * time.Hour).UnixMilli()
	sooner := time.Now().Add(2 * time.Hour).UnixMilli()
	snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
		{Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .8, sooner), promptTestLimit("7d", .2, later)}},
		{Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .2, later), promptTestLimit("7d", .6, later)}},
		{Provider: "other", Limits: []ai.UsageLimit{promptTestLimit("monthly", 1, sooner)}},
	}}
	windows := promptQuotaWindows(snapshot, "openai-codex", "luna-6")
	if len(windows) != 2 || windows[0].used != .5 || windows[0].resetsAt != sooner {
		t.Fatalf("wrong account total or next reset: %+v", windows)
	}
	m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "luna-6", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
	m.opts.UsageAccountKey = func(string) (*string, bool) { return nil, true }
	label := m.promptQuotaLabel(200)
	if strings.Contains(label, "luna-6") || strings.Contains(strings.ToLower(label), "codex") {
		t.Fatalf("quota label included model/provider: %q", label)
	}
	wantBar := tui.RenderUsageBar(tui.ActiveTheme(), ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: new(.5)}, Status: ai.UsageStatusOK}, 24)
	if !strings.Contains(label, wantBar) {
		t.Fatalf("quota does not use the usage board bar: %q", label)
	}
	if strings.Count(label, "█") == 0 || !strings.Contains(label, "5h") || !strings.Contains(label, "50% left") || !strings.Contains(label, "reset 2h") || strings.Contains(label, "7d") {
		t.Fatalf("wrong essential quota: %q", label)
	}

	minimumWidth := widthx.VisibleWidth(label) - 24 + 12
	if compact := m.promptQuotaLabel(minimumWidth); widthx.VisibleWidth(compact) != minimumWidth {
		t.Fatalf("bar did not shrink to 12 cells: %q", compact)
	}
	if compact := m.promptQuotaLabel(minimumWidth - 1); compact != "" {
		t.Fatalf("bar rendered without enough room for its fields: %q", compact)
	}

	// A freshly fetched weekly bottleneck replaces the short-window bar.
	m.promptQuota.snapshot.Reports[0].Limits[1].Amount.UsedFraction = new(1.0)
	m.promptQuota.snapshot.Reports[1].Limits[1].Amount.UsedFraction = new(.8)
	label = m.promptQuotaLabel(200)
	if strings.Count(label, "█") == 0 || !strings.Contains(label, "7d") || !strings.Contains(label, "10% left") || strings.Contains(label, "5h") {
		t.Fatalf("did not select the new limiting window: %q", label)
	}
	m.promptQuota.snapshot.UnavailableAccounts = []ai.UsageUnavailableAccount{{Provider: "openai-codex"}}
	if label = m.promptQuotaLabel(200); !strings.Contains(label, "partial") {
		t.Fatalf("incomplete totals were presented as complete: %q", label)
	}
	m.editor = tui.NewEditor()
	m.editor.TopRightStatus = m.promptQuotaLabel
	if row := m.editor.Render(200)[0]; !strings.Contains(row, "10% left") {
		t.Fatalf("quota did not reach the prompt border: %q", row)
	}

	// Select a different model and provider without waiting for another network fetch.
	m.opts.Model = &ai.Model{ID: "different", ProviderMeta: ai.ProviderMetadata{ProviderID: "other"}}
	if label = m.promptQuotaLabel(200); strings.Contains(label, "different") || !strings.Contains(label, "monthly") || !strings.Contains(label, "0% left") {
		t.Fatalf("model switch retained the old quota: %q", label)
	}
	m.opts.Model.ProviderMeta.ProviderID = "unsupported"
	if label = m.promptQuotaLabel(200); label != "" {
		t.Fatalf("missing quota should be hidden: %q", label)
	}
	if row := m.editor.Render(200)[0]; strings.Contains(row, "%") || strings.Contains(row, "quota") {
		t.Fatalf("unavailable quota was not cleared from the border: %q", row)
	}
	m.opts.Model.ProviderMeta.ProviderID = "openai-codex"
	m.promptQuota.loaded = false
	if label = m.promptQuotaLabel(200); label != "" {
		t.Fatalf("loading quota should be hidden: %q", label)
	}
	m.promptQuota.loaded, m.promptQuota.failed = true, true
	if label = m.promptQuotaLabel(200); label != "" {
		t.Fatalf("failed quota fetch should be hidden: %q", label)
	}
}

func TestPromptQuotaWeightsCapacityAndFiltersModels(t *testing.T) {
	a, b := promptTestLimit("monthly", 1, 0), promptTestLimit("monthly", 0, 0)
	a.Amount.Limit = new(100.0)
	b.Amount.Limit = new(900.0)
	modelOnly := promptTestLimit("special", 1, 0)
	modelOnly.Scope.ModelID = "another-model"
	invalid := promptTestLimit("invalid", math.NaN(), 0)
	snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "test", Limits: []ai.UsageLimit{a, modelOnly, invalid}}, {Provider: "test", Limits: []ai.UsageLimit{b}}}}
	windows := promptQuotaWindows(snapshot, "test", "selected-model")
	if len(windows) != 1 || math.Abs(windows[0].used-.1) > 1e-9 {
		t.Fatalf("wrong weighted total or model filtering: %+v", windows)
	}
}

type promptQuotaTestService struct {
	fetch func(context.Context) (ai.UsageSnapshot, error)
}

func (s promptQuotaTestService) FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error) {
	return s.fetch(ctx)
}
func (promptQuotaTestService) ActiveAccount(context.Context, string, ...*string) (ai.UsageActiveAccount, bool) {
	return ai.UsageActiveAccount{}, false
}

func TestPromptQuotaRefreshPublishesOnOwnerAndRejectsReplacedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m := &InteractiveMode{uiTaskCh: make(chan func(), 8), tuiInst: tui.NewWithOutput(io.Discard, 100, 30), editor: tui.NewEditor()}
	m.opts.UsageService = promptQuotaTestService{fetch: func(context.Context) (ai.UsageSnapshot, error) {
		return ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "test"}}}, nil
	}}
	m.refreshPromptQuota(ctx)
	var apply func()
	select {
	case apply = <-m.uiTaskCh:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh was not delivered")
	}
	if m.promptQuota.loaded {
		t.Fatal("worker mutated owner state")
	}
	apply()
	if !m.promptQuota.loaded || len(m.promptQuota.snapshot.Reports) != 1 {
		t.Fatal("owner did not publish quota")
	}
	m.refreshPromptQuota(ctx)
	select {
	case apply = <-m.uiTaskCh:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh was not delivered")
	}
	replacement := &promptQuotaState{}
	m.promptQuota = replacement
	apply()
	if replacement.loaded {
		t.Fatal("old session overwrote replacement quota")
	}
	cancel()
	m.backgroundTasks.Wait()
}

func TestPromptQuotaCancellationJoinsFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	m := &InteractiveMode{uiTaskCh: make(chan func(), 8)}
	m.opts.UsageService = promptQuotaTestService{fetch: func(ctx context.Context) (ai.UsageSnapshot, error) {
		close(started)
		<-ctx.Done()
		return ai.UsageSnapshot{}, errors.New("cancelled")
	}}
	m.startPromptQuota(ctx)
	<-started
	cancel()
	done := make(chan struct{})
	go func() { m.backgroundTasks.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("quota tasks outlived shutdown")
	}
}

func TestPromptQuotaKeepsFreeMonthlyAndPlusWeeklyTotalsSeparate(t *testing.T) {
	monthlyReset := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	weeklyReset := time.Now().Add(4 * 24 * time.Hour).UnixMilli()
	freeA, freeB := promptTestLimit("30d", .8, monthlyReset), promptTestLimit("30d", .6, monthlyReset)
	plus := promptTestLimit("7d", .2, weeklyReset)
	// Codex can use the same meter ID for different account window durations.
	freeA.ID, freeB.ID, plus.ID = "openai-codex:primary", "openai-codex:primary", "openai-codex:primary"
	snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
		{Provider: "openai-codex", Metadata: map[string]any{"planType": "free", "accountKey": "free"}, Limits: []ai.UsageLimit{freeA}},
		{Provider: "openai-codex", Metadata: map[string]any{"planType": "free", "accountKey": "free"}, Limits: []ai.UsageLimit{freeB}},
		{Provider: "openai-codex", Metadata: map[string]any{"planType": "plus", "accountKey": "plus"}, Limits: []ai.UsageLimit{plus}},
	}}
	windows := promptQuotaWindows(snapshot, "openai-codex", "gpt-6-luna")
	if len(windows) != 2 || windows[0].label != "30d" || math.Abs(windows[0].used-.7) > 1e-9 || windows[0].resetsAt != monthlyReset || windows[1].label != "7d" || windows[1].used != .2 {
		t.Fatalf("free monthly quota was mixed with Plus weekly quota: %+v", windows)
	}
	m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "gpt-6-luna", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
	m.opts.UsageAccountKey = func(string) (*string, bool) { return new("free"), true }
	label := m.promptQuotaLabel(100)
	if !strings.HasPrefix(label, "30d ") || !strings.Contains(label, "30% left") || !strings.Contains(label, "reset 20d") || strings.Contains(label, "7d") {
		t.Fatalf("wrong monthly window or reset in prompt: %q", label)
	}
}

func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts(t *testing.T) {
	for _, provider := range []string{"openai-codex", "future-quota-provider"} {
		t.Run(provider, func(t *testing.T) {
			monthlyReset := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
			weeklyReset := time.Now().Add(4 * 24 * time.Hour).UnixMilli()
			snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
				{Provider: provider, Metadata: map[string]any{"planType": "plus", "accountKey": "plus"}, Limits: []ai.UsageLimit{promptTestLimit("7d", .95, weeklyReset)}},
				{Provider: provider, Metadata: map[string]any{"planType": "free", "accountKey": "free-a"}, Limits: []ai.UsageLimit{promptTestLimit("30d", .2, monthlyReset)}},
				{Provider: provider, Metadata: map[string]any{"planType": "ChatGPT-Free", "accountKey": "free-b"}, Limits: []ai.UsageLimit{promptTestLimit("30d", .4, monthlyReset)}},
			}}
			m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "unlisted-future-model-a", ProviderMeta: ai.ProviderMetadata{ProviderID: provider}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}, editor: tui.NewEditor()}
			routes := map[string]string{}
			m.opts.UsageAccountKey = func(string) (*string, bool) {
				key, present := routes[m.opts.Model.ID]
				if !present {
					return nil, false
				}
				return &key, true
			}
			m.editor.TopRightStatus = m.promptQuotaLabel
			if label := m.promptQuotaLabel(100); label != "" {
				t.Fatalf("unknown route guessed a subscription tier: %q", label)
			}
			routes[m.opts.Model.ID] = "free-a"
			assertFree := func() {
				t.Helper()
				label := m.promptQuotaLabel(100)
				if !strings.HasPrefix(label, "30d ") || !strings.Contains(label, "70% left") || strings.Contains(label, "7d") {
					t.Fatalf("Plus quota overrode the combined free tier: %q", label)
				}
			}
			assertFree()
			m.editor.Render(100)
			m.editor.NeedsRedraw()
			routes[m.opts.Model.ID] = "plus"
			if !m.editor.IsDirty() {
				t.Fatal("routed account change did not invalidate the border")
			}
			if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
				t.Fatalf("paid fallback retained the free bar: %q", label)
			}
			routes[m.opts.Model.ID] = "free-b"
			assertFree()
			m.editor.Render(100)
			m.editor.NeedsRedraw()
			m.opts.Model.ID = "unlisted-future-model-b"
			if !m.editor.IsDirty() {
				t.Fatal("switching models retained a stale account identity")
			}
			if label := m.promptQuotaLabel(100); label != "" {
				t.Fatalf("new model guessed eligibility from its name: %q", label)
			}
			routes[m.opts.Model.ID] = "plus"
			if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {
				t.Fatalf("new model ignored its actual paid route: %q", label)
			}
			m.opts.Model.ID = "unlisted-future-model-a"
			assertFree()
			routes[m.opts.Model.ID] = "missing-account"
			if label := m.promptQuotaLabel(100); label != "" {
				t.Fatalf("missing account quota fell back to another tier: %q", label)
			}
		})
	}
}
