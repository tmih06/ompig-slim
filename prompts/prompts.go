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

//
//go:embed system.md
var systemMD string

var system = strings.TrimSpace(systemMD)

// System returns the embedded base system prompt with surrounding whitespace trimmed.
func System() string {
	return system
}
