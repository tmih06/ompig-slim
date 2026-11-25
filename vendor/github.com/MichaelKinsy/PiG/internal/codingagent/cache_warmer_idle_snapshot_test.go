package codingagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// pendingIdleSession is the session-manager stub a deadline-only warmer run
// needs: the run never refreshes, so no usage is appended.
type pendingIdleSession struct{}

func (pendingIdleSession) AppendUsage(kind, provider, model string, usage ai.Usage, note string) (UsageEntry, error) {
	return UsageEntry{}, nil
}

func (pendingIdleSession) GetBranch(fromID ...string) []SessionEntry { return nil }

// newPendingIdleWarmer builds a warmer whose run carries only an
// idle-compaction deadline: the model has no cache lifetime, so the run
// anchors its offer at settle with the policy delay.
func newPendingIdleWarmer(t *testing.T, delaySeconds float64, mode CacheWarmingMode, offer func(ctx context.Context, request *CacheWarmRequest) IdleOfferResult) *CacheWarmer {
	t.Helper()
	if offer == nil {
		offer = func(context.Context, *CacheWarmRequest) IdleOfferResult { return IdleOfferDeclined }
	}
	warmer := NewCacheWarmer(
		func(ctx context.Context, model *ai.Model, transcript ai.TranscriptContext, options ai.StreamOptions) (*ai.AssistantMessageEventStream, error) {
			return nil, errors.New("no warm requests expected")
		},
		pendingIdleSession{},
		func() CacheWarmingMode { return mode },
		nil,
	)
	warmer.SetIdleCompaction(offer, func() IdleCompactionPolicy {
		return IdleCompactionPolicy{Enabled: true, DelaySeconds: delaySeconds}
	})
	t.Cleanup(func() {
		warmer.Close()
		warmer.Wait()
	})
	return warmer
}

func pendingIdleRequest() CacheWarmRequest {
	return CacheWarmRequest{
		Model:   &ai.Model{ID: "chat"},
		Context: ai.NormalizeContext(ai.Context{}),
		Options: ai.StreamOptions{},
	}
}

func pendingIdleDeadline(t *testing.T, warmer *CacheWarmer) PendingIdleCompaction {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pending, ok := warmer.PendingIdleCompaction(); ok {
			return pending
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("no pending idle-compaction deadline")
	return PendingIdleCompaction{}
}

func pendingIdleGone(t *testing.T, warmer *CacheWarmer, why string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := warmer.PendingIdleCompaction(); !ok {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("pending offer still reported after %s", why)
}

func TestPendingIdleCompactionArmedAndConsumed(t *testing.T) {
	claimed := make(chan struct{}, 1)
	warmer := newPendingIdleWarmer(t, 0.05, CacheWarmingMode("streaming"), func(context.Context, *CacheWarmRequest) IdleOfferResult {
		claimed <- struct{}{}
		return IdleOfferClaimed
	})
	var current atomic.Bool
	current.Store(true)
	warmer.Start(pendingIdleRequest(), current.Load)
	if _, ok := warmer.PendingIdleCompaction(); ok {
		t.Fatal("deadline pending before the run settled")
	}
	settle := time.Now()
	warmer.OnAgentSettled()
	pending := pendingIdleDeadline(t, warmer)
	if want := settle.Add(50 * time.Millisecond).UnixMilli(); pending.DeadlineAtMs < want-80 || pending.DeadlineAtMs > want+1500 {
		t.Fatalf("deadline %d not anchored at settle plus the policy delay", pending.DeadlineAtMs)
	}

	current.Store(false)
	if _, ok := warmer.PendingIdleCompaction(); ok {
		t.Fatal("stale request still reported a pending offer")
	}
	current.Store(true)
	if _, ok := warmer.PendingIdleCompaction(); !ok {
		t.Fatal("current request lost its pending offer")
	}

	// The deadline fires the offer; a claimed offer latches and is not
	// reported again.
	select {
	case <-claimed:
	case <-time.After(5 * time.Second):
		t.Fatal("offer never reached the idle hook")
	}
	pendingIdleGone(t, warmer, "the claimed offer")
}

func TestPendingIdleCompactionBusyKeepsDeadline(t *testing.T) {
	release := make(chan struct{})
	answered := make(chan IdleOfferResult, 1)
	warmer := newPendingIdleWarmer(t, 0.05, CacheWarmingMode("streaming"), func(ctx context.Context, _ *CacheWarmRequest) IdleOfferResult {
		select {
		case <-release:
		case <-ctx.Done():
			return IdleOfferDeclined
		}
		select {
		case result := <-answered:
			return result
		case <-ctx.Done():
			return IdleOfferDeclined
		}
	})
	warmer.Start(pendingIdleRequest(), func() bool { return true })
	warmer.OnAgentSettled()
	pending := pendingIdleDeadline(t, warmer)

	// Once the deadline passes the offer is in flight and hidden; a busy
	// answer re-pends it with its deadline unchanged.
	time.Sleep(time.Until(time.UnixMilli(pending.DeadlineAtMs)) + 50*time.Millisecond)
	answered <- IdleOfferBusy
	close(release)
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		after, ok := warmer.PendingIdleCompaction()
		if !ok {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		if after.DeadlineAtMs != pending.DeadlineAtMs {
			t.Fatalf("busy answer changed the deadline: %d -> %d", pending.DeadlineAtMs, after.DeadlineAtMs)
		}
		break
	}
	if _, ok := warmer.PendingIdleCompaction(); !ok {
		t.Fatal("busy answer dropped the pending offer")
	}

	// The next idle signal offers the same deadline again; the claimed
	// answer consumes it for good.
	warmer.OnIdle()
	answered <- IdleOfferClaimed
	pendingIdleGone(t, warmer, "the claimed offer")
}

func TestPendingIdleCompactionWarmingOff(t *testing.T) {
	warmer := newPendingIdleWarmer(t, 30, CacheWarmingMode("off"), nil)
	warmer.Start(pendingIdleRequest(), func() bool { return true })
	warmer.OnAgentSettled()
	pendingIdleDeadline(t, warmer)
}

func TestPendingIdleCompactionCancelAndDisable(t *testing.T) {
	warmer := newPendingIdleWarmer(t, 30, CacheWarmingMode("streaming"), nil)
	warmer.Start(pendingIdleRequest(), func() bool { return true })
	warmer.OnAgentSettled()
	pendingIdleDeadline(t, warmer)
	warmer.Cancel()
	if _, ok := warmer.PendingIdleCompaction(); ok {
		t.Fatal("cancelled run still reported pending")
	}

	disabled := newPendingIdleWarmer(t, 30, CacheWarmingMode("streaming"), nil)
	var enabled atomic.Bool
	enabled.Store(true)
	disabled.SetIdleCompaction(func(context.Context, *CacheWarmRequest) IdleOfferResult { return IdleOfferDeclined },
		func() IdleCompactionPolicy { return IdleCompactionPolicy{Enabled: enabled.Load(), DelaySeconds: 30} })
	disabled.Start(pendingIdleRequest(), func() bool { return true })
	disabled.OnAgentSettled()
	pendingIdleDeadline(t, disabled)
	enabled.Store(false)
	if _, ok := disabled.PendingIdleCompaction(); ok {
		t.Fatal("disabled policy still reported pending")
	}
}
