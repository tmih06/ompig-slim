// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"os"

	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/tui"
)

// stopProfiles writes the PIG_PROFILE profiles that main started. It is a no-op
// when PIG_PROFILE is unset.
var stopProfiles = func() {}

// stopModelServices cancels and drains Services-owned model work while extension transports are still available.
var stopModelServices = func() {}

// profileSyncBeforeExit is configured before runtime startup by the embedder.
var profileSyncBeforeExit func() error

// flushProfileSync keeps failed uploads visible and makes a successful command
// fail rather than claiming that an unsent preference change is synchronized.
func flushProfileSync(code int) int {
	if profileSyncBeforeExit != nil {
		if err := profileSyncBeforeExit(); err != nil {
			fmt.Fprintln(os.Stderr, "profile sync:", err)
			if code == 0 {
				return 1
			}
		}
	}
	return code
}

// exitProcess releases startup extension ownership and writes profiles before
// exiting. os.Exit skips deferred calls, so main uses this on every exit path.
func exitProcess(code int) {
	stopAutomaticExtensionCacheGC()
	stopModelServices()
	stopStartupExtensions()
	codingagent.RestoreStdout()
	stopProfiles()
	// pig additive (D102): every exit through here is requested or prints its reason, so the session marker goes.
	codingagent.EndSessionMarker()
	// Pi's process 'exit' handler restores the terminal modes of a TUI that exits without stop() (9df819de, #11170); os.Exit runs no handlers.
	tui.RestoreTerminalOnExit()
	// pig additive (D102): an emergency exit in flight ends the process with its 129, and a claimed exit.log line is flushed first.
	codingagent.BeforeExit()
	os.Exit(flushProfileSync(code))
}
