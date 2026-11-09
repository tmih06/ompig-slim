package main

import (
	"os"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/cli"
	"github.com/MichaelKinsy/PiG/coding/extension"
	_ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.

	_ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.

	"ompig-slim/internal/selfupdate"
	"ompig-slim/internal/todotool"
	"ompig-slim/prompts"
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
	//
	// The todo extension is composed in here rather than patched into the
	// vendored runtime, so it survives a future `go mod vendor` and can be
	// removed in one line. It registers both halves of the feature: the tool the
	// model calls, and the settle-boundary handler that continues a run whose
	// todo list still has open work.
	//
	// Pass `delegated`, not `os.Args[1:]`: the self-updater may rewrite the
	// argument list before handing the rest of the CLI over.
	//
	// SystemPromptPreamble is embedded from prompts/system.md rather than written
	// in Go source, so the prompt is reviewed and diffed as prose. The runtime
	// composes its tool, rules, project-context and skills sections around it.
	cli.Main(args, &cli.MainOptions{
		ExtensionFactories: []extension.InlineExtension{
			todotool.Extension(),
			todotool.SettleExtension(),
		},
		SystemPromptPreamble: prompts.System(),
	})
}
