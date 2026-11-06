# ompig-slim

A personal coding harness built on [PiG](https://github.com/MichaelKinsy/PiG) as a Go library. PiG v0.7.0 (Pi 1.5.0 behavior) supplies the complete stock application through `coding/cli.Main`, with all stock providers and the Lisp extension builder linked.

## Build and run

```sh
make build
./bin/pig --version
./bin/pig
```

Go 1.26 or newer is required. `go.mod` selects Go 1.27.3; Go's automatic toolchain download must be enabled or that toolchain must be installed locally. Dependencies are pinned in `go.mod` and verified by `go.sum`.

```sh
make run ARGS='--help'
make install                    # ~/.local/bin/pig
make install PREFIX=/some/path  # /some/path/bin/pig
make check                      # local Go tests and vet
```

Run `/login` interactively or set a provider API key. All stock flags, commands, TUI modes, JSON and RPC modes, tools, sessions, extensions, skills, themes, MCP, and Piglets are handled by PiG itself. There is no subprocess dependency on an installed `pig` executable and no stripped-down agent loop.

Stock defaults currently apply: configuration lives in `~/.pig`, with `PIG_HOME` and `PIG_CODING_AGENT_DIR` overrides. PiG's tools and extensions run with your user permissions; this is not a sandbox. Upstream self-update behavior is retained and can replace the executable with a stock release; rebuild this project to retain personal customizations.

## Verification

The baseline has been built and checked with `go test ./...` (no local test files) and `go vet ./...`. The actual executable reports `0.7.0+1.5.0`, displays stock help, and accepts an RPC `get_state` request with a successful correlated response. This is startup/RPC evidence, not a claim that every provider has been live-tested.

## Attribution

PiG is maintained by Michael Kinsy and contributors under the MIT license. PiG follows [Pi](https://github.com/earendil-works/pi), by Mario Zechner and contributors. Personal feature behavior is being migrated from tmih06's `omp-slim` fork. This project is independent of PiG, Pi, and oh-my-pi.
