//go:build unix

package agentstate

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid names a running process.
//
// Signal 0 performs the permission and existence checks without delivering
// anything. EPERM means the process exists and belongs to someone else, which is
// alive; ESRCH is the only answer that means gone.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
