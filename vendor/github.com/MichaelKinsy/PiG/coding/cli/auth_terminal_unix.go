//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Poll before reading the raw terminal so a completed loopback callback can
// cancel its outstanding paste prompt without leaving a blocked input goroutine.
func readAuthLoginInput(ctx context.Context, buffer []byte) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, context.Cause(ctx)
		}
		descriptors := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
		count, err := unix.Poll(descriptors, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if count == 0 {
			continue
		}
		if ctx.Err() != nil {
			return 0, context.Cause(ctx)
		}
		if descriptors[0].Revents&unix.POLLIN != 0 {
			return os.Stdin.Read(buffer)
		}
		if descriptors[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
	}
}
