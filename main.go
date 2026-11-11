package main

import (
	"context"
	"fmt"
	"os"
	"slices"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/coding/cli"
	"github.com/MichaelKinsy/PiG/coding/extension"
	_ "github.com/MichaelKinsy/PiG/coding/extension/builtin/todo"  // Register the todo built-in and its reminder.
	_ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.
	"github.com/MichaelKinsy/PiG/coding/extension/pigsdk"

	_ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.

	"ompig-slim/internal/agentstate"
	agentstateext "ompig-slim/internal/agentstate/ext"
	"ompig-slim/internal/broker"
	"ompig-slim/internal/loopguard"
	"ompig-slim/internal/selfupdate"
	"ompig-slim/prompts"
)

// brokerStartupNeeded reports whether normal CLI execution can use broker-backed
// credentials. These fast paths are handled without opening the configured broker.
func brokerStartupNeeded(args []string) bool {
	if slices.Contains(args, "--offline") || slices.Contains(args, "--version") || slices.Contains(args, "-v") || slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		return false
	}
	return len(args) == 0 || args[0] != "diagnose"
}

func main() {
	args := os.Args[1:]
	// The updater runs the replacement binary with this private command before
	// swapping it into place. It must happen before normal CLI startup, because
	// loading extensions would otherwise observe the old staged SDK.
	if len(args) == 1 && args[0] == selfupdate.StageReplacementArg {
		if _, _, err := pigsdk.Sync(coding.ConfigRoot(), false); err != nil {
			fmt.Fprintln(os.Stderr, "stage extension SDKs:", err)
			os.Exit(1)
		}
		return
	}
	// Claim the archive-install self-update before PiG parses arguments:
	// PiG has no `upgrade` spelling, so it would read that word as a prompt and
	// fail on the missing model instead of updating. Everything this package
	// does not own is delegated unchanged, so the complete CLI is preserved.
	if delegated, handled, code := selfupdate.Handle(args, coding.Version, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	} else if delegated != nil {
		args = delegated
	}
	// Claimed here for the same reason as the self-updater above: PiG has no
	// `broker` subcommand, so it would read the word as a prompt and fail on the
	// missing model instead of configuring the hosted profile store. This is the
	// non-interactive provisioning path for containers, CI jobs and machines
	// configured by configuration management, which cannot answer /login.
	if handled, code := broker.HandleSetup(context.Background(), args, coding.DefaultAgentDir(), os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	// Load the saved /login connection (or environment override) and synchronize
	// preferences before PiG loads configuration, and install remote credential
	// discovery. Inference stays local; OAuth refresh belongs to the broker.
	// Commands that cannot use credentials must not pay for broker startup or its
	// exit flush; --offline also has to remain offline before PiG parses flags.
	var finishSync func() error
	if brokerStartupNeeded(args) {
		var err error
		finishSync, err = broker.Start(context.Background(), coding.DefaultAgentDir(), os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	// Delegate arguments, terminal ownership, signals, and exit status to PiG.
	// The vendored personal runtime supplies the defaults; keep the complete CLI.
	//
	// The todo feature is native: coding/extension/builtin/todo registers the tool
	// with the built-in registry and the stop-time reminder with the native
	// before-settle hook registry, both from its init. Nothing about the todo
	// feature is an extension: the tool and reminder exist with
	// --no-extensions, the tool is dropped by
	// --no-builtin-tools, and neither appears in the startup Extensions list.
	//
	// Pass `delegated`, not `os.Args[1:]`: the self-updater may rewrite the
	// argument list before handing the rest of the CLI over.
	//
	// SystemPromptPreamble is embedded from prompts/system.md rather than written
	// in Go source, so the prompt is reviewed and diffed as prose. The runtime
	// composes its tool, rules, project-context and skills sections around it.
	//
	// AgentState publishes what the agent is doing as a JSON document under the
	// configuration root, so an external tool (a terminal multiplexer, a status
	// bar, a notifier) can watch it without this binary knowing about any of
	// them. It registers event handlers only, so it adds no tool, command, or
	// flag; a publisher that cannot resolve a configuration root does nothing.
	//
	// The adapters that consume the document live in extensions/, not here: they
	// are ordinary extensions a user installs, so supporting another consumer
	// never means a new binary.
	cli.Main(args, &cli.MainOptions{
		SystemPromptPreamble: prompts.System(),
		ExtensionFactories: []extension.InlineExtension{
			// Hidden program machinery, like agent-state: always active even with
			// --no-extensions, without adding a user-managed resource or tool.
			extension.NamedInlineExtension{
				Name: "tool-call-loop-guard", Factory: loopguard.Extension, Hidden: true,
			},
			extension.NamedInlineExtension{
				Name:    "agent-state",
				Factory: agentstateext.Extension(agentstate.NewPublisher()),
				// Hidden keeps the startup Extensions list as it was: this is a
				// program-level surface, not a resource the user manages, and it
				// loads whether or not extensions are enabled.
				Hidden: true,
			},
		},
		BeforeExit: finishSync,
		ProfileLogin: &cli.ProfileLoginOption{
			ID: broker.LoginProviderID, Name: broker.LoginProviderName, Login: broker.Login,
		},
	})
}
