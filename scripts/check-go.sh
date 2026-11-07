#!/usr/bin/env bash
# Use canonical GOPATH imports for cacheable vendored tests. This also preserves
# assembly/linkname identities; testing vendor/... with a prefixed import path
# breaks wazero's native backend. Production builds and vet use module mode.
set -euo pipefail
cd "$(dirname "$0")/.."
root="$PWD"
compiler="$("${GO:-go}" env GOROOT)/bin/go"
cache_root="${OMPIG_CHECK_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/ompig-check}"
mkdir -p "$cache_root/gopath/src" "$cache_root/profile/agent" "$cache_root/profile/library"
cache_root="$(cd "$cache_root" && pwd)"
for source in "$root"/vendor/*; do
    [[ -d "$source" ]] || continue
    link="$cache_root/gopath/src/${source##*/}"
    if [[ -L "$link" ]]; then
        if [[ "$(readlink "$link")" != "$source" ]]; then rm "$link"; fi
    elif [[ -e "$link" ]]; then
        printf 'Refusing to replace non-symlink %s\n' "$link" >&2
        exit 1
    fi
    if [[ ! -L "$link" ]]; then ln -s "$source" "$link"; fi
done
export PIG_CODING_AGENT_DIR="$cache_root/profile/agent"
export PI_CODING_AGENT_DIR="$PIG_CODING_AGENT_DIR"
export OMPIG_SLIM_CODING_AGENT_DIR="$PIG_CODING_AGENT_DIR"
export PIG_HOME="$cache_root/profile/library" PI_HOME="$cache_root/profile/library"
export PI_SKIP_VERSION_CHECK=1 PI_TELEMETRY=0
case "${1:-check}" in
    test|race|check)
        # Bash 3.2 (macOS) treats an empty array as unset under nounset.
        # Keep the command nonempty, including when no flags are requested.
        test_command=(test)
        if [[ "${1:-check}" == race ]]; then test_command+=(-race); fi
        "$compiler" "${test_command[@]}" -mod=vendor ./...
        # Resolve patterns in module mode: GOPATH wildcard discovery ignores
        # symlinked trees, while explicit canonical imports resolve them.
        packages=()
        package_list="$("$compiler" list -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/...)"
        if [[ -z "$package_list" ]]; then printf 'No PiG packages found\n' >&2; exit 1; fi
        while IFS= read -r package; do
            if [[ -n "$package" ]]; then packages+=("$package"); fi
        done <<< "$package_list"
        (cd "$cache_root/gopath/src" && GO111MODULE=off GOPATH="$cache_root/gopath" GOTOOLCHAIN=local \
            "$compiler" "${test_command[@]}" "${packages[@]}")
        ;;
    vet) ;;
    *) printf 'Usage: %s [check|test|race|vet]\n' "$0" >&2; exit 2 ;;
esac
if [[ "${1:-check}" == check || "${1:-check}" == vet ]]; then
    "$compiler" vet -mod=vendor ./... ./vendor/github.com/MichaelKinsy/PiG/...
fi
