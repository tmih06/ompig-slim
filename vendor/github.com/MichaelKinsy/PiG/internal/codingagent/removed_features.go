package codingagent

import (
	"strings"

	"github.com/MichaelKinsy/PiG/internal/pigstrip"
)

// init keeps legacy configuration and help from advertising removed features.
// These exclusions are product boundaries, not optional build-tag defaults.
func init() {
	for _, name := range []string{"pig-login", "llama.cpp"} {
		pigstrip.Strip(pigstrip.ListExtensions, name)
	}
}

// removedBuiltinCommand recognizes retired local commands, including arguments.
// The interactive host rejects them before queueing any model work; ordinary
// prose and similarly prefixed commands remain untouched.
func removedBuiltinCommand(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "/sprite", "/pigsayhi", "/arminsayshi", "/dementedelves", "/share", "/changelog", "/llama":
		return true
	}
	return false
}
