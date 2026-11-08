package main

import (
	"os"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/cli"
	_ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.

	_ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.

	"ompig-slim/internal/selfupdate"
)

func main() {
	args := os.Args[1:]
	// Claim the archive-install self-update before PiG parses arguments:
	// PiG has no `upgrade` spelling, so it would read that word as a prompt and
	// fail on the missing model instead of updating. Everything this package
	// does not own is delegated unchanged, so the complete CLI is preserved.
	if delegated, handled, code := selfupdate.Handle(args, coding.Version, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	} else if delegated != nil {
		args = delegated
	}
	// Delegate arguments, terminal ownership, signals, and exit status to PiG.
	// The vendored personal runtime supplies the defaults; keep the complete CLI.
	cli.Main(args, nil)
}
