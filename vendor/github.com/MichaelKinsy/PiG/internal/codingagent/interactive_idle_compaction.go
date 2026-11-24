package codingagent

// Ports packages/coding-agent/src/modes/interactive/interactive-mode.ts.

import (
	"context"
	"fmt"
	"time"

	"github.com/MichaelKinsy/PiG/tui"
)

// idleCompactionReader is the countdown's private capability on the Session
// handle: the warmer's armed idle-compaction offer. InteractiveSessionHandle
// does not name it; a handle without it shows no row.
type idleCompactionReader interface {
	PendingIdleCompaction() (PendingIdleCompaction, bool)
}

// idleCompactionLabel is the countdown row's text: the armed offer's
// deadline as "check in Ns", or "pending" once it passed, because the
// deadline schedules an eligibility check, not a guaranteed compaction.
func idleCompactionLabel(deadlineAtMs, nowMs int64) string {
	remaining := (deadlineAtMs - nowMs + 999) / 1000
	if remaining <= 0 {
		return "Idle compaction pending"
	}
	return fmt.Sprintf("Idle compaction check in %ds", remaining)
}

// startIdleCompactionCountdown owns the one-second observation of the armed
// idle offer. Every queued closure re-reads the live session handle and
// snapshot, so a stale tick cannot revive a consumed offer or a replaced
// session; the row never wakes the 80ms spinner.
func (m *InteractiveMode) startIdleCompactionCountdown(ctx context.Context) {
	m.backgroundTasks.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !m.idleCountdownQueued.CompareAndSwap(false, true) {
					continue
				}
				if !m.postUITaskQuiet(func() {
					m.idleCountdownQueued.Store(false)
					m.refreshIdleCompactionCountdown(time.Now())
				}) {
					m.idleCountdownQueued.Store(false)
				}
			}
		}
	})
}

// refreshIdleCompactionCountdown repaints the countdown row. It runs on the
// owner loop. The row is hidden while a status indicator owns the row above
// the editor or no offer is pending; a past deadline keeps its pending label
// because busy waits do not re-arm it.
func (m *InteractiveMode) refreshIdleCompactionCountdown(now time.Time) {
	label := ""
	if m.activeStatusIndicator == nil {
		if reader, ok := m.opts.SessionHandle.(idleCompactionReader); ok {
			if pending, available := reader.PendingIdleCompaction(); available {
				label = idleCompactionLabel(pending.DeadlineAtMs, now.UnixMilli())
			}
		}
	}
	m.setIdleCompactionLabel(label)
}

// hideIdleCompactionCountdown clears the row immediately, for submissions,
// foreground work and session replacement, ahead of the next refresh.
func (m *InteractiveMode) hideIdleCompactionCountdown() {
	m.setIdleCompactionLabel("")
}

// setIdleCompactionLabel writes a changed label and requests its frame;
// unchanged text renders nothing.
func (m *InteractiveMode) setIdleCompactionLabel(label string) {
	if m.idleCountdownContainer == nil || label == m.idleCountdownShown {
		return
	}
	m.idleCountdownShown = label
	m.idleCountdownContainer.Clear()
	if label != "" {
		m.idleCountdownContainer.Add(tui.NewText(tui.ActiveTheme().Fg("dim", label)))
	}
	if m.tuiInst != nil {
		m.tuiInst.RequestRender()
	}
}
