// Package prompts embeds this program's system prompt.
//
// The prompt lives in system.md rather than in Go source so it can be read,
// reviewed, and diffed as prose. It is compiled into the binary, so an installed
// ompig-slim never depends on the file at runtime.
//
// The vendored PiG runtime builds the system prompt itself. This package only
// supplies the base text: main.go hands it to prompts.SetPreamble before the CLI
// runs, and the runtime composes its tool, rules, project-context, and skills
// sections around it.
package prompts

import (
	_ "embed"
	"strings"
)

// systemMD is the embedded prompt. The blank import of embed is what turns the
// directive into the file's contents at build time.
//
//go:embed system.md
var systemMD string

// system is systemMD trimmed once at startup. The prompt is read on every
// session's first turn, so the trim is not repeated per call.
var system = strings.TrimSpace(systemMD)

// System returns the base system prompt: the embedded file with surrounding
// blank lines trimmed.
//
// The returned string is shared and must not be modified; strings are immutable
// in Go, so a caller only ever receives a copy of the header.
func System() string {
	return system
}
