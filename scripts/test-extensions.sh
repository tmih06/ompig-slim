#!/usr/bin/env bash
# Runs the tests of the extension module under extensions/.
#
# These extensions are not part of the binary's Go module: they are installable
# PiG extensions, built by PiG against the SDK staged for the running version. A
# `replace` line pointing at that staged copy is what makes the module buildable
# here, and it is generated rather than committed so it can never pin a stale SDK.
#
# Usage: scripts/test-extensions.sh [extension-name...]
set -euo pipefail
cd "$(dirname "$0")/.."

binary="${OMPIG_SLIM_BIN:-bin/ompig-slim}"

# Use the toolchain that built the binary rather than whatever `go` an unusual
# PATH resolves to: an extension module carries the `go` directive PiG scaffolds,
# and an older compiler rejects it before running a single test.
toolchain="$("${GO:-go}" env GOROOT)/bin/go"
if [[ ! -x "$toolchain" ]]; then
    printf 'no Go toolchain at %s; set GO to a go command\n' "$toolchain" >&2
    exit 1
fi
if [[ ! -x "$binary" ]]; then
    printf '%s not found; run make build first\n' "$binary" >&2
    exit 1
fi

# PiG stages one SDK directory per language; only the Go one carries go.mod. The
# staged copy is content-addressed, so the newest directory is the current SDK.
config_root="${PIG_HOME:-$HOME/.pi}"
shopt -s nullglob
candidates=("$config_root"/state/pigsdk/sdk-*/go.mod)
shopt -u nullglob
if (( ${#candidates[@]} == 0 )); then
    printf 'no staged Go SDK under %s/state/pigsdk; run one session first\n' "$config_root" >&2
    exit 1
fi
root="$(cd "$(dirname "$(ls -t "${candidates[@]}" | head -1)")" && pwd)"
if [[ ! -f "$root/go.mod" ]]; then
    printf 'staged SDK %s has no go.mod\n' "$root" >&2
    exit 1
fi

names=("$@")
if (( ${#names[@]} == 0 )); then
    shopt -s nullglob
    for dir in extensions/*/; do
        [[ -f "$dir/go.mod" ]] && names+=("$(basename "$dir")")
    done
    shopt -u nullglob
fi

for name in "${names[@]}"; do
    dir="extensions/$name"
    [[ -f "$dir/go.mod" ]] || { printf 'no extension at %s\n' "$dir" >&2; exit 1; }
    # The module's committed go.mod carries the placeholder require; the replace
    # is added here and removed on exit so a failed run leaves no residue.
    cp "$dir/go.mod" "$dir/go.mod.orig"
    trap 'mv -f "$dir/go.mod.orig" "$dir/go.mod"' EXIT
    printf '\nreplace github.com/MichaelKinsy/PiG/extensions/sdk => %s\n' "$root" >> "$dir/go.mod"
    printf '==> %s (SDK %s)\n' "$name" "$(basename "$root")"
    # GOTOOLCHAIN=local keeps the resolved toolchain; the module's `go` directive
    # is the version PiG scaffolds, not a request to download another one.
    (cd "$dir" && GOFLAGS=-mod=mod GOTOOLCHAIN=local "$toolchain" test ./...)
    mv -f "$dir/go.mod.orig" "$dir/go.mod"
    trap - EXIT
done
