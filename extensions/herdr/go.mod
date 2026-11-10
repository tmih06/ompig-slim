module ompig-slim/extensions/herdr

go 1.26

// PiG resolves this requirement to the version-matched staged SDK at build time
// (see `ompig-slim extension init`), so the version below is a placeholder that
// the loader rewrites. `go test` in this directory needs the replace line, which
// points at the SDK staged by the running harness; run `make test-extensions` to
// generate it, or `ompig-slim extension init` for a scaffolded copy.
require github.com/MichaelKinsy/PiG/extensions/sdk v0.0.0
