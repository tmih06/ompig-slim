package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestUsageDashboardRendersProgressAndOnlyForcesManualRefresh(t *testing.T) {
	started, published, released := make(chan bool, 2), make(chan struct{}), make(chan struct{})
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		InitialRefresh: true,
		RefreshProgress: func(ctx context.Context, force bool, push func(UsageDashboardSnapshot)) (UsageDashboardSnapshot, error) {
			started <- force
			snapshot := UsageDashboardSnapshot{Reports: []ai.UsageReport{{Provider: "fast-provider", Metadata: map[string]any{"email": "fast@example.com"}}}}
			push(snapshot)
			if !force {
				close(published)
				select {
				case <-released:
				case <-ctx.Done():
					return snapshot, context.Cause(ctx)
				}
			}
			return snapshot, nil
		},
	})
	defer dashboard.Dispose()
	dashboard.Start(t.Context())
	if force := <-started; force {
		t.Fatal("opening the board bypassed fresh quota caches")
	}
	<-published
	rendered := plainLines(dashboard.Render(110))
	if !strings.Contains(strings.ToLower(rendered), "fast") {
		t.Fatalf("partial usage is not visible while refresh is pending:\n%s", rendered)
	}
	dashboard.mu.Lock()
	pending := dashboard.refreshing
	dashboard.mu.Unlock()
	if !pending {
		t.Fatal("partial report completed a still-pending refresh")
	}
	close(released)
	dashboard.background.Wait()
	dashboard.Refresh()
	if force := <-started; !force {
		t.Fatal("manual refresh did not bypass freshness")
	}
	dashboard.background.Wait()
}

func TestUsageDashboardCloseCancelsProgressAndRejectsLatePush(t *testing.T) {
	started, done := make(chan struct{}), make(chan struct{})
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		InitialRefresh: true,
		RefreshProgress: func(ctx context.Context, _ bool, push func(UsageDashboardSnapshot)) (UsageDashboardSnapshot, error) {
			close(started)
			<-ctx.Done()
			push(UsageDashboardSnapshot{Reports: []ai.UsageReport{{Provider: "late"}}})
			close(done)
			return UsageDashboardSnapshot{}, context.Cause(ctx)
		},
	})
	dashboard.Start(t.Context())
	<-started
	dashboard.Dispose()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the dashboard did not cancel the refresh")
	}
	dashboard.background.Wait()
	dashboard.mu.Lock()
	defer dashboard.mu.Unlock()
	if len(dashboard.reports) != 0 {
		t.Fatal("a closed dashboard accepted a late quota report")
	}
}

func TestUsageRenderingCannotCacheAReportReplacedDuringRender(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	dashboard := NewUsageDashboard(UsageDashboardOptions{
		Reports: []ai.UsageReport{{Provider: "old-provider"}},
		ResolveActiveAccount: func(string) *ai.UsageActiveAccount {
			once.Do(func() { close(started); <-release })
			return nil
		},
	})
	defer dashboard.Dispose()
	go func() { defer close(done); dashboard.detailLines(100) }()
	<-started
	dashboard.mu.Lock()
	dashboard.applySnapshotLocked(UsageDashboardSnapshot{Reports: []ai.UsageReport{{Provider: "new-provider"}}})
	dashboard.mu.Unlock()
	dashboard.changed()
	close(release)
	<-done
	if lines := strings.ToLower(plainLines(dashboard.detailLines(100))); !strings.Contains(lines, "new") || strings.Contains(lines, "old") {
		t.Fatalf("an old render hid the newly published quota report: %s", lines)
	}
}
