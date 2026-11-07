package main

import (
	"os"

	_ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
	"github.com/MichaelKinsy/PiG/coding/cli"
	_ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.

	_ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.
)

func main() {
	// Delegate arguments, terminal ownership, signals, and exit status to PiG.
	// The vendored personal runtime supplies the defaults; keep the complete CLI.
	cli.Main(os.Args[1:], nil)
}
