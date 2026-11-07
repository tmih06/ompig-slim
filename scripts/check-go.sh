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
        flags=()
        if [[ "${1:-check}" == race ]]; then flags+=(-race); fi
        "$compiler" test -mod=vendor "${flags[@]}" ./...
        # Resolve patterns in module mode: GOPATH wildcard discovery ignores
        # symlinked trees, while explicit canonical imports resolve them.
        packages=()
        package_list="$("$compiler" list -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/...)"
        while IFS= read -r package; do
            if [[ -n "$package" ]]; then packages+=("$package"); fi
        done <<< "$package_list"
        if ((${#packages[@]} == 0)); then printf 'No PiG packages found\n' >&2; exit 1; fi
        (cd "$cache_root/gopath/src" && GO111MODULE=off GOPATH="$cache_root/gopath" GOTOOLCHAIN=local \
            "$compiler" test "${flags[@]}" "${packages[@]}")
        ;;
    vet) ;;
    *) printf 'Usage: %s [check|test|race|vet]\n' "$0" >&2; exit 2 ;;
esac
if [[ "${1:-check}" == check || "${1:-check}" == vet ]]; then
    "$compiler" vet -mod=vendor ./... ./vendor/github.com/MichaelKinsy/PiG/...
fi
