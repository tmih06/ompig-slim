package codingagent

// Ports the subset of packages/coding-agent/src/modes/interactive/interactive-mode.ts
// handleReloadCommand that a background settings-document change needs. Unlike
// /reload it never rebuilds resources or extensions, so nothing the user is
// running is interrupted.

import (
	"context"

	"github.com/MichaelKinsy/PiG/tui"
)

// startSyncedDocumentWatcher forwards the embedder's synchronization signals and
// re-reads the configuration those writes replaced.
//
// The runtime re-reads configuration only for /reload, and an embedder's
// synchronization worker writes files behind a live session's back. Without this
// watcher the pull is invisible: the file carries another machine's model role
// while the session keeps answering from the copy it read at startup, so the role
// appears only after a restart.
//
// The worker signals from its own goroutine, and rendering plus the settings
// manager are owner-loop state, so the apply step is posted to the owner loop and
// runs single-threaded with keystrokes. Close ends delivery; a signal still queued
// checks its context and does nothing after shutdown.
func (m *InteractiveMode) startSyncedDocumentWatcher(ctx context.Context) func() {
	signals := SyncedDocuments
	if signals == nil {
		return func() {}
	}
	stopCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	m.backgroundTasks.Go(func() {
		defer close(done)
		for {
			select {
			case <-stopCtx.Done():
				return
			case <-signals:
				_ = m.postToMain(stopCtx, m.applySyncedDocuments)
			}
		}
	})
	return func() {
		stop()
		<-done
	}
}

// applySyncedDocuments re-reads the configuration an embedder rewrote on disk.
// It runs on the owner loop.
//
// Only the steps /reload performs for settings and keybindings run: the settings
// manager re-reads its layers, the mode re-reads its derived view, the keybindings
// manager re-loads its file, and theming and output padding follow. A document is
// what the session already trusts, so this re-reads rather than re-discovers:
// extensions, skills and prompts stay as they are.
//
// The model in use is deliberately left alone. A synchronization can change which
// model a role resolves to while a turn is streaming, and switching models mid-run
// would replay a request after streamed output; the new assignment applies from
// the next selection, exactly as a /reload of settings does today.
func (m *InteractiveMode) applySyncedDocuments() {
	if m.runCtx == nil || m.runCtx.Err() != nil || m.runEnded.Load() {
		return
	}
	if m.opts.SettingsManager != nil {
		// The Session reloads the settings it shares so tools added to
		// defaultTools activate on the next refresh, matching /reload; a mode
		// without a Session handle still refreshes the manager.
		if session, ok := m.opts.SessionHandle.(interface{ ReloadSettings() }); ok {
			session.ReloadSettings()
		} else if m.opts.SessionHandle == nil {
			m.opts.SettingsManager.Reload()
		}
		m.opts.Settings = m.opts.SettingsManager.Get()
		m.refreshSettingsDerivedState()
	}
	// The keybindings file is reloaded unconditionally: a poll that reported a
	// change does not say which document it was, and re-reading one small file is
	// cheaper than tracking that state across the synchronization worker.
	if m.keybindings != nil {
		m.keybindings.Reload()
		m.publishExtensionKeybindings()
	}
	m.requestRender()
}

// refreshSettingsDerivedState re-reads the mode-level values it caches from the
// settings it just reloaded. A value only Render reads is left out; these drive
// state that decorates composited output, terminal capabilities, or the palette.
func (m *InteractiveMode) refreshSettingsDerivedState() {
	m.hideThinking = m.opts.Settings.GetHideThinkingBlock()
	m.outputPad = int(m.opts.Settings.GetOutputPad())
	tui.SetCapabilityOverrides(m.opts.Settings.GetTerminalCapabilityOverrides())
	// A profile `themes/*.json` document synchronizes too, and a received one is
	// not in the registry this mode loaded at startup, so its selection would fall
	// back to a bundled palette until the next restart.
	m.loadThemes()
	m.applyThemeFromSettings(m.runCtx)
}
