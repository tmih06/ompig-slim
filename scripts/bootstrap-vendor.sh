#!/usr/bin/env bash
# Restore disposable dependencies without ever regenerating the patched PiG tree.
# modules.txt selects every platform's packages; Go verifies downloads with go.sum.
# Requires Go and network access on first use (or a populated module cache).
set -euo pipefail
cd "$(dirname "$0")/.."
if cmp -s vendor/modules.txt vendor/.bootstrap-modules.txt; then exit 0; fi

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
cp go.mod go.sum "$stage/"
# Import the locked non-PiG packages explicitly, including internal packages:
# go mod vendor copies their sources, embed assets and license notices without
# compiling them or applying internal-import restrictions. Use a scratch module
# so upstream PiG can neither replace personal patches nor change the manifest.
{
    printf 'package main\nimport (\n'
    awk '!/^#/ && NF && $0 !~ /^github[.]com\/MichaelKinsy\/PiG(\/|$)/ { printf "_ \"%s\"\n", $0 }' vendor/modules.txt
    printf ')\nfunc main() {}\n'
} > "$stage/main.go"
printf 'Restoring pinned dependencies (preserving patched PiG sources)\n'
(cd "$stage" && GOFLAGS= "${GO:-go}" mod vendor)
rm "$stage/vendor/modules.txt"

# Replace whole generated namespaces so upgrades cannot leave obsolete Go files.
# github.com is shared with maintained PiG; only replace the generated owners.
# Publish the completion marker last, so an interrupted copy is retried.
for source in "$stage/vendor"/*; do
    name="${source##*/}"
    if [[ "$name" == github.com ]]; then
        mkdir -p vendor/github.com
        for owner in "$source"/*; do
            target="vendor/github.com/${owner##*/}"
            rm -rf "$target"
            mv "$owner" "$target"
        done
    else
        rm -rf "vendor/$name"
        mv "$source" "vendor/$name"
    fi
done
cp vendor/modules.txt vendor/.bootstrap-modules.txt
