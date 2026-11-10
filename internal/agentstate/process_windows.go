//go:build windows

package agentstate

import "syscall"

// processAlive reports whether pid names a running process.
//
// Windows has no signal-0 probe, so this opens the process with the least
// privilege that still reports existence. A handle that opens is a live process;
// ERROR_ACCESS_DENIED means one exists that this process may not open, which is
// alive; ERROR_INVALID_PARAMETER is the documented answer for a pid that no
// longer exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return !isMissingProcessError(err)
	}
	_ = syscall.CloseHandle(handle)
	return true
}

// isMissingProcessError reports whether err is Windows' "no such process".
func isMissingProcessError(err error) bool {
	if pidErr, ok := err.(syscall.Errno); ok {
		// ERROR_INVALID_PARAMETER: the pid is not in use.
		return pidErr == 87
	}
	return false
}
