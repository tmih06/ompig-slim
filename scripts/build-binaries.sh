#!/usr/bin/env bash
# Build the personal harness for the fork's four supported native platforms.
set -euo pipefail
cd "$(dirname "$0")/.."

platform=""
out="bin/releases"
while (($#)); do
    case "$1" in
        --platform|--out)
            if (($# < 2)); then printf 'Missing value for %s\n' "$1" >&2; exit 2; fi
            if [[ "$1" == --platform ]]; then platform="$2"; else out="$2"; fi
            shift 2
            ;;
        --help|-h)
            printf 'Usage: bash scripts/build-binaries.sh [--platform linux-x64|linux-arm64|darwin-x64|darwin-arm64] [--out directory]\n'
            exit 0
            ;;
        *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
    esac
done

platforms=(linux-x64 linux-arm64 darwin-x64 darwin-arm64)
if [[ -n "$platform" ]]; then
    case "$platform" in
        linux-x64|linux-arm64|darwin-x64|darwin-arm64) platforms=("$platform") ;;
        *) printf 'Unsupported platform: %s\n' "$platform" >&2; exit 2 ;;
    esac
fi
mkdir -p "$out"
out="$(cd "$out" && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

for platform in "${platforms[@]}"; do
    os="${platform%-*}"
    arch="${platform##*-}"
    if [[ "$arch" == x64 ]]; then arch=amd64; fi
    package="$stage/$platform/ompig-slim"
    mkdir -p "$package"
    printf 'Building %s\n' "$platform"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" "${GO:-go}" build -mod=vendor -trimpath -ldflags='-s -w' -o "$package/ompig-slim" .
    cp README.md "$package/README.md"
    cp LICENSE NOTICE "$package/"
    while IFS= read -r -d '' license; do
        target="$package/licenses/${license#vendor/}"
        mkdir -p "$(dirname "$target")"
        cp "$license" "$target"
    done < <(find vendor -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' -o -iname 'COPYRIGHT*' -o -iname 'NOTICE*' -o -iname '*NOTICES*' -o -iname 'PATENTS*' \) -print0)
    cp vendor/github.com/MichaelKinsy/PiG/LICENSE "$package/PiG-LICENSE"
    cp vendor/github.com/MichaelKinsy/PiG/NOTICE "$package/PiG-NOTICE"
    tar -C "$stage/$platform" -czf "$out/ompig-slim-$platform.tar.gz" ompig-slim
    if command -v sha256sum >/dev/null 2>&1; then
        (cd "$out" && sha256sum "ompig-slim-$platform.tar.gz" > "ompig-slim-$platform.tar.gz.sha256")
    else
        (cd "$out" && shasum -a 256 "ompig-slim-$platform.tar.gz" > "ompig-slim-$platform.tar.gz.sha256")
    fi
    printf '%s\n' "$out/ompig-slim-$platform.tar.gz"
done
