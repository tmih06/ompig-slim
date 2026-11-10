package broker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
)

// SetupUsage is the help text for the setup command. It is kept beside the
// parser so a new flag cannot be documented in one place and forgotten in the
// other.
const SetupUsage = `Usage:
  ompig-slim broker setup --url <origin> [--token-file <path>] [--token-stdin] [--verify] [--force]
  ompig-slim broker show
  ompig-slim broker clear

Configure the hosted profile store without the /login dialog, for containers,
CI jobs and machines provisioned by configuration management.

Options for setup:
  --url <origin>       Broker origin, for example https://sync.example.com.
  --token-file <path>  Read the shared secret from a file (trimmed).
  --token <secret>     The shared secret on the command line. Prefer a file or
                       stdin: argv is visible to every process on the machine.
  --token-stdin        Read the shared secret from stdin.
  --verify             Check the connection before saving it.
  --force              Replace a saved connection that points somewhere else.

The secret is also read from OMPIG_BROKER_TOKEN_FILE, then OMPIG_BROKER_TOKEN,
when no token option is given. 'broker show' prints the saved URL and whether a
secret is stored (never the secret itself). 'broker clear' disconnects this
device; the downloaded profile and credentials remain local.

Restart ompig-slim to activate a changed connection: a running session keeps the
sync worker it started with.
`

// HandleSetup claims the `broker` subcommand from the argument list. It reports
// whether this package handled the invocation, and the process exit status when
// it did.
//
// The command is claimed before PiG parses arguments, in the same place and for
// the same reason as the archive self-updater: PiG has no `broker` subcommand, so
// it would read the word as a prompt and fail on the missing model instead of
// configuring the connection. Everything not named here is delegated unchanged.
func HandleSetup(ctx context.Context, args []string, dir string, stdin io.Reader, stdout, stderr io.Writer) (handled bool, code int) {
	if len(args) == 0 || args[0] != "broker" {
		return false, 0
	}
	rest := args[1:]
	if len(rest) == 0 {
		io.WriteString(stderr, SetupUsage)
		return true, 2
	}
	switch rest[0] {
	case "setup":
		return true, runSetup(ctx, rest[1:], dir, stdin, stdout, stderr)
	case "show":
		return true, runShow(dir, stdout, stderr)
	case "clear":
		return true, runClear(dir, stdout, stderr)
	case "help", "-h", "--help":
		io.WriteString(stdout, SetupUsage)
		return true, 0
	default:
		fmt.Fprintf(stderr, "Unknown broker command %q\n\n", rest[0])
		io.WriteString(stderr, SetupUsage)
		return true, 2
	}
}

// runSetup parses the setup flags and applies them.
//
// Flags are parsed by hand rather than with `flag` because this runs before PiG
// owns the argument list, and a shared FlagSet would print its own errors in a
// style the rest of this command does not use.
func runSetup(ctx context.Context, args []string, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	var options SetupOptions
	options.Stdin = stdin
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url", "-url":
			if i+1 >= len(args) {
				return setupUsageError(stderr, "--url needs a value")
			}
			i++
			options.URL = args[i]
		case "--token", "-token":
			if i+1 >= len(args) {
				return setupUsageError(stderr, "--token needs a value")
			}
			i++
			options.Token = args[i]
		case "--token-file", "-token-file":
			if i+1 >= len(args) {
				return setupUsageError(stderr, "--token-file needs a value")
			}
			i++
			options.TokenFile = args[i]
		case "--token-stdin", "-token-stdin":
			// stdin is already the last fallback in [setupSecret]; the flag exists
			// so a script can state that the secret arrives on stdin instead of
			// relying on that ordering.
			options.Stdin = stdin
		case "--verify", "-verify":
			options.Verify = true
		case "--force", "-force":
			options.Force = true
		case "--help", "-h", "-help":
			io.WriteString(stdout, SetupUsage)
			return 0
		default:
			return setupUsageError(stderr, fmt.Sprintf("unknown option %q", args[i]))
		}
	}
	result, err := Setup(ctx, dir, options)
	if err != nil {
		fmt.Fprintf(stderr, "broker setup: %v\n", err)
		return 1
	}
	switch {
	case !result.Changed:
		fmt.Fprintf(stdout, "Broker connection already set to %s\n", result.URL)
	case result.URL == "":
		fmt.Fprintln(stdout, "Removed the saved broker connection. The downloaded profile stays local.")
	default:
		fmt.Fprintf(stdout, "Saved broker connection to %s\n", result.URL)
		if result.Verified {
			fmt.Fprintln(stdout, "Verified the service answered.")
		} else {
			fmt.Fprintln(stdout, "Not verified; the next startup reports an unreachable service.")
		}
	}
	return 0
}

// runShow prints the saved URL and secret presence, never the secret.
func runShow(dir string, stdout, stderr io.Writer) int {
	saved, err := readSavedConnection(filepath.Join(dir, connectionFile))
	if err != nil {
		fmt.Fprintf(stderr, "broker show: %v\n", err)
		return 1
	}
	if saved.URL == "" {
		fmt.Fprintln(stdout, "No broker connection is saved on this device.")
		return 0
	}
	fmt.Fprintf(stdout, "url: %s\nsecret: stored\n", saved.URL)
	return 0
}

// runClear removes the saved connection.
func runClear(dir string, stdout, stderr io.Writer) int {
	result, err := Setup(context.Background(), dir, SetupOptions{Clear: true})
	if err != nil {
		fmt.Fprintf(stderr, "broker clear: %v\n", err)
		return 1
	}
	if !result.Changed {
		fmt.Fprintln(stdout, "No broker connection was saved.")
		return 0
	}
	fmt.Fprintln(stdout, "Removed the saved broker connection. The downloaded profile stays local.")
	return 0
}

// setupUsageError prints the usage after a usage mistake and returns the exit
// status for it.
func setupUsageError(stderr io.Writer, message string) int {
	fmt.Fprintf(stderr, "broker setup: %s\n\n", message)
	io.WriteString(stderr, SetupUsage)
	return 2
}
