package main

import (
	"os"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
	"github.com/MichaelKinsy/PiG/coding/cli"
	"github.com/MichaelKinsy/PiG/coding/extension"
	_ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.

	_ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.

	"ompig-slim/internal/todotool"
)

func main() {
	// Delegate arguments, terminal ownership, signals, and exit status to PiG.
	// The vendored personal runtime supplies the defaults; keep the complete CLI.
	//
	// The todo extension is composed in here rather than patched into the
	// vendored runtime, so it survives a future `go mod vendor` and can be
	// removed in one line. It registers both halves of the feature: the tool the
	// model calls, and the settle-boundary handler that continues a run whose
	// todo list still has open work.
	cli.Main(os.Args[1:], &cli.MainOptions{
		ExtensionFactories: []extension.InlineExtension{
			todotool.Extension(),
			todotool.SettleExtension(),
		},
	})
}
