package codingagent

import (
	"context"
	"fmt"
)

// bindUsageFallbackUI binds reserve questions through the main-loop task queue.
// Background/subagent sessions deliberately have no confirmer and may auto-fallback.
func (m *InteractiveMode) bindUsageFallbackUI() {
	handle, ok := m.opts.SessionHandle.(interface {
		SetUsageFallbackConfirmer(func(context.Context, string, string, float64) bool)
	})
	if !ok {
		return
	}
	handle.SetUsageFallbackConfirmer(func(ctx context.Context, from, to string, remaining float64) bool {
		accepted := false
		err := m.runOnMainAndWait(ctx, func() error {
			accepted = m.askConfirm(ctx, "Model usage reserve", fmt.Sprintf("%s has %.1f%% remaining. Switch to %s?", from, remaining, to))
			return nil
		})
		return err == nil && accepted
	})
}
