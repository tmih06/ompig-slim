package codingagent

// Wires the /usage board into the interactive mode: the slash command, the
// fullscreen overlay mount/close lifecycle, the quota service, and the local
// activity scanner. Ports packages/coding-agent/src/modes/interactive/
// interactive-mode.ts showUsageDashboard/closeUsageDashboard and the /usage
// branch of the submit handler.

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/tui"
)

// UsageQuotaService is the quota source the /usage board reads.
// *UsageService satisfies it; the interface keeps the board testable without a
// credential store and lets the host substitute a stub.
type UsageQuotaService interface {
	// FetchSnapshot refreshes every configured account's quota. force bypasses
	// the cache TTL.
	FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error)
	// ActiveAccount reports the read-only identity of the account a provider
	// currently routes to. No accountKey asks for the store's active account;
	// one argument names the exact account, where a nil key is an explicit
	// identity-less account, not the default.
	ActiveAccount(ctx context.Context, provider string, accountKey ...*string) (ai.UsageActiveAccount, bool)
}

// usageDashboardState is the mounted /usage overlay.
type usageDashboardState struct {
	component    *tui.UsageDashboardComponent
	handle       *tui.OverlayHandle
	previousMode string
}

// showUsageDashboard opens the fullscreen quota board, or focuses it when it is
// already open. Primary reports live on the alternate screen, including when
// chat uses normal scrollback, so a regular-mode chat switches to fullscreen
// first, as upstream does.
func (m *InteractiveMode) showUsageDashboard() {
	if m.tuiInst == nil {
		return
	}
	if m.usageDashboard != nil {
		m.usageDashboard.handle.Focus()
		return
	}
	previousMode := "regular"
	if m.altScreen != nil {
		previousMode = "fullscreen"
	}
	if previousMode == "regular" && !m.switchTuiMode("fullscreen", true, true) {
		m.showWarning("Close active overlays before opening usage.")
		return
	}

	if m.usageActivity == nil {
		m.usageActivity = tui.NewUsageActivity(m.usageSessionRoots())
	} else {
		m.usageActivity.SetRoots(m.usageSessionRoots())
	}
	activity := m.usageActivity

	provider := ""
	if m.opts.Model != nil {
		provider = m.opts.Model.ProviderID()
	}
	service := m.opts.UsageService
	if service == nil {
		service = m.usageService
	}
	// activeAccountMu guards the identity the refresh goroutine resolves
	// against the owner loop's read for the detail view.
	var activeAccountMu sync.Mutex
	var activeAccount *ai.UsageActiveAccount

	component := tui.NewUsageDashboard(tui.UsageDashboardOptions{
		InitialRefresh: true,
		ViewportRows:   m.usageViewportRows,
		LoadActivity: func(ctx context.Context, push func([]tui.DailyActivityPoint)) error {
			return activity.Load(ctx, push)
		},
		RefreshProgress: func(ctx context.Context, force bool, push func(tui.UsageDashboardSnapshot)) (tui.UsageDashboardSnapshot, error) {
			if service == nil {
				return tui.UsageDashboardSnapshot{}, nil
			}
			if provider != "" {
				account, ok := m.resolveUsageActiveAccount(ctx, service, provider)
				activeAccountMu.Lock()
				if ok {
					resolved := account
					activeAccount = &resolved
				} else {
					activeAccount = nil
				}
				activeAccountMu.Unlock()
			}
			var snapshot ai.UsageSnapshot
			var err error
			if progressive, ok := service.(interface {
				FetchSnapshotProgress(context.Context, bool, func(ai.UsageSnapshot)) (ai.UsageSnapshot, error)
			}); ok {
				snapshot, err = progressive.FetchSnapshotProgress(ctx, force, func(snapshot ai.UsageSnapshot) { push(tui.NewUsageDashboardSnapshot(snapshot)) })
			} else {
				snapshot, err = service.FetchSnapshot(ctx, force)
			}
			return tui.NewUsageDashboardSnapshot(snapshot), err
		},
		ResolveActiveAccount: func(providerID string) *ai.UsageActiveAccount {
			if providerID != provider {
				return nil
			}
			activeAccountMu.Lock()
			defer activeAccountMu.Unlock()
			return activeAccount
		},
		RequestRender: m.requestRender,
		OnClose:       m.closeUsageDashboard,
	})
	// The board owns the whole screen: 100% width and height with no margin,
	// anchored bottom-center as upstream's showOverlay requests.
	options := tui.OverlaySpec{
		Width:     &tui.OverlayValue{Value: 100, Percent: true},
		MaxHeight: &tui.OverlayValue{Value: 100, Percent: true},
		Anchor:    "bottom-center",
		Margin:    tui.OverlayMarginAll(0),
	}.Options()
	options.Fullscreen = true
	handle := m.tuiInst.ShowOverlay(component, options)
	if handle == nil {
		return
	}
	m.usageDashboard = &usageDashboardState{component: component, handle: handle, previousMode: previousMode}
	// The overlay's first refresh and activity scan run on the owner loop so
	// their pushes land through the mode's render request.
	component.Start(m.backgroundContext())
	m.requestRender()
}

// closeUsageDashboard disposes the board, hides its overlay, and restores the
// chat's previous screen mode.
func (m *InteractiveMode) closeUsageDashboard() {
	dashboard := m.usageDashboard
	if dashboard == nil {
		return
	}
	m.usageDashboard = nil
	dashboard.component.Dispose()
	// Hide removes the entry synchronously (overlayRemoveTarget runs under the
	// overlay lock before apply returns), so HasOverlay is false for the
	// renderer swap and switchTuiMode's own guard passes.
	dashboard.handle.Hide()
	if !m.isShuttingDownMode() && dashboard.previousMode != m.currentTuiMode() {
		m.switchTuiMode(dashboard.previousMode, true, true)
	}
	m.requestRender()
}

// disposeUsageDashboard releases the board when the mode ends: it stops the
// activity scan and refresh goroutines and drops the mounted state without
// touching the renderer, which teardown is already stopping.
func (m *InteractiveMode) disposeUsageDashboard() {
	dashboard := m.usageDashboard
	if dashboard == nil {
		return
	}
	m.usageDashboard = nil
	dashboard.component.Dispose()
	dashboard.handle.Hide()
}

// usageViewportRows reports the terminal's visible rows for the board frame.
func (m *InteractiveMode) usageViewportRows() int {
	if m.tuiInst == nil {
		return 24
	}
	if terminal := m.tuiInst.Terminal(); terminal != nil {
		if rows := terminal.Rows(); rows > 0 {
			return rows
		}
	}
	return 24
}

// usageSessionRoots lists the session directories the activity heatmap scans:
// the agent directory's sessions plus the current session's directory.
func (m *InteractiveMode) usageSessionRoots() []string {
	roots := make([]string, 0, 2)
	if m.opts.AgentDir != "" {
		roots = append(roots, filepath.Join(m.opts.AgentDir, "sessions"))
	}
	session := m.currentSession()
	if session != nil {
		if dir := session.GetSessionDir(); dir != "" {
			roots = append(roots, dir)
		}
	}
	return roots
}

// usageAccountKey reports the account key this session is routed to for
// provider. present is false when the host wires no per-session routing
// decision, which asks the quota service for the store's active account; a
// present result passes the key through, nil included, because a nil key names
// an identity-less account rather than the default.
func (m *InteractiveMode) usageAccountKey(provider string) (*string, bool) {
	if m.opts.UsageAccountKey == nil {
		return nil, false
	}
	return m.opts.UsageAccountKey(provider)
}

// resolveUsageActiveAccount reads the routed account identity for provider,
// passing the session's key only when the host reported one.
func (m *InteractiveMode) resolveUsageActiveAccount(ctx context.Context, service UsageQuotaService, provider string) (ai.UsageActiveAccount, bool) {
	key, present := m.usageAccountKey(provider)
	if !present {
		return service.ActiveAccount(ctx, provider)
	}
	return service.ActiveAccount(ctx, provider, key)
}

// backgroundContext returns the run's background context, or the process
// context when the mode has not started one (component tests).
func (m *InteractiveMode) backgroundContext() context.Context {
	if m.backgroundCtx != nil {
		return m.backgroundCtx
	}
	return context.Background()
}

// isShuttingDownMode reports whether the mode is tearing down or already torn
// down, which keeps the board from switching the renderer back mid-stop.
func (m *InteractiveMode) isShuttingDownMode() bool {
	return m.runEnded.Load() || m.requestExit.Load() || m.inputLoopErr != nil
}

// currentTuiMode reports the mounted renderer's mode.
func (m *InteractiveMode) currentTuiMode() string {
	if m.altScreen != nil {
		return "fullscreen"
	}
	return "regular"
}

// usageHandler runs /usage: bare or `show` opens the quota board, `reset` spends
// a saved rate-limit reset, and anything else is a usage error, as upstream's
// submit handler does.
func usageHandler(sc *SlashContext) error {
	verb, rest := parseUsageSubcommand(sc.Args)
	switch {
	case verb == "" || (verb == "show" && rest == ""):
		if sc.ShowUsage != nil {
			sc.ShowUsage()
		}
		return nil
	case verb == "reset":
		switch {
		case rest != "":
			// A typed target skips the picker: the caller already chose.
			handleUsageResetCommand(sc, rest)
		case sc.ShowResetUsageSelector != nil:
			sc.ShowResetUsageSelector()
		default:
			handleUsageResetCommand(sc, "")
		}
		return nil
	}
	message := "Usage: /usage [show|reset [provider/credential|provider/active]]"
	if sc.ShowError != nil {
		sc.ShowError(message)
	} else if sc.AppendText != nil {
		// Headless dispatch has no status line; the transcript is the only sink.
		sc.AppendText(message)
	}
	return nil
}
