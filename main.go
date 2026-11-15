package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

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
	goalext "ompig-slim/internal/goalext"
	"ompig-slim/internal/loopguard"
	"ompig-slim/internal/selfupdate"
	"ompig-slim/internal/workdesk"
	"ompig-slim/prompts"
)

// brokerStartupNeeded reports whether normal CLI execution can use broker-backed
// credentials. These fast paths are handled without opening the configured broker.
func brokerStartupNeeded(args []string) bool {
	if len(args) >= 2 && args[0] == "durable" && args[1] == "status" {
		return false
	}
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
	// does not own is delegated to the trimmed coding runtime.
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
	//
	// syncedDocuments is handed to the interactive session below: a poll that
	// rewrites settings.json locally must reach a session that already loaded it,
	// or a model role assigned on another machine waits for a restart.
	var finishSync func() error
	var syncedDocuments <-chan struct{}
	var syncConflicts <-chan any
	if brokerStartupNeeded(args) {
		// One buffered slot coalesces signals: the receiver re-reads every document
		// from disk, so a second signal for the same files says nothing new, and a
		// full slot must never stall the worker holding the profile lock.
		documents := make(chan struct{}, 1)
		conflicts := make(chan broker.Conflict, 8)
		conflictEvents := make(chan any, 8)
		go func() {
			for conflict := range conflicts {
				conflictEvents <- conflict
			}
		}()
		var err error
		finishSync, err = broker.Start(context.Background(), coding.DefaultAgentDir(), os.Stderr, func() {
			select {
			case documents <- struct{}{}:
			default:
			}
		}, conflicts)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		syncedDocuments = documents
		syncConflicts = conflictEvents
	}
	if len(args) > 0 && args[0] == "durable" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err := workdesk.Run(ctx, args[1:], os.Stdout)
		stop()
		if finishSync != nil {
			err = errors.Join(err, finishSync())
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// PiG owns the terminal, signals, and delegated arguments. The inline
	// extensions below are hidden program machinery, not user-managed resources.
	cli.Main(args, &cli.MainOptions{
		SystemPromptPreamble: prompts.System(),
		SyncedDocuments:      syncedDocuments,
		SyncConflicts:        syncConflicts,
		ExtensionFactories: []extension.InlineExtension{
			extension.NamedInlineExtension{
				Name: "tool-call-loop-guard", Factory: loopguard.Extension, Hidden: true,
			},
			extension.NamedInlineExtension{
				Name: "goal", Factory: goalext.Extension, Hidden: true,
			},
			extension.NamedInlineExtension{
				Name:    "agent-state",
				Factory: agentstateext.Extension(agentstate.NewPublisher()),
				Hidden:  true,
			},
		},
		BeforeExit: finishSync,
		ProfileLogin: &cli.ProfileLoginOption{
			ID: broker.LoginProviderID, Name: broker.LoginProviderName, Login: broker.Login,
		},
	})
}
