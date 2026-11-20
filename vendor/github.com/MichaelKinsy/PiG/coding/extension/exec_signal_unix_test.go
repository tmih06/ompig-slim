//go:build unix

package extension

import (
	"context"
	"syscall"
	"testing"
)

func TestExecCommandReportsSignalExitCode(t *testing.T) {
	result, err := ExecCommand(context.Background(), t.TempDir(), "/bin/sh", []string{"-c", "kill -TERM $$"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := 128 + int(syscall.SIGTERM); result.Code != want {
		t.Fatalf("code = %d, want %d", result.Code, want)
	}
	if result.Killed || result.Truncated {
		t.Fatalf("ordinary child signal was reported as a Pi kill: %+v", result)
	}
}
