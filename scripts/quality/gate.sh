#!/usr/bin/env bash
# Single quality-gate entry point. Runs the cheapest checks first and stops at
# the first failure so the agent gets one concrete thing to fix. Each failing
# check prints file:line, rule id, and a fix hint.
#
# Usage:
#   scripts/quality/gate.sh            # full local gate
#   scripts/quality/gate.sh --quick    # build + vet + lint-diff only (pre-commit)
#   PR_BASE=origin/main gate.sh        # diff checks + new-code lint vs that base
#
# Modes:
#   New code is gated strictly by `new-from-rev` against the PR base.
#   Existing debt is held by scripts/quality/ratchet.py (per-rule totals).
set -uo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"
export PATH="$PATH:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin"

QUICK=0
[ "${1:-}" = "--quick" ] && QUICK=1

# Resolve the diff base once: PR base, else merge-base with main, else HEAD.
BASE="${PR_BASE:-}"
if [ -z "$BASE" ]; then
    for ref in origin/main main; do
        if git rev-parse --verify "$ref" >/dev/null 2>&1; then
            BASE="$(git merge-base "$ref" HEAD 2>/dev/null || true)"
            [ -n "$BASE" ] && break
        fi
    done
fi
[ -z "$BASE" ] && BASE="HEAD"

step() { printf '\n\033[1m== gate: %s ==\033[0m\n' "$*" >&2; }
fail() { printf '\n\033[31mGATE FAIL\033[0m — %s\n' "$*" >&2; }
note() { printf '   %s\n' "$*" >&2; }

FAILED=0
run() { # run <name> <cmd...>
    local name="$1"; shift
    step "$name"
    if "$@"; then note "ok"; else FAILED=1; fail "$name"; return 1; fi
    return 0
}

# --- 1. Build (cheapest real check; nothing else matters if it doesn't compile)
run "build" go build -mod=vendor ./... || exit 1

# --- 2. golangci-lint on new/changed code (strict)
step "golangci-lint (new code vs ${BASE:0:12})"
if command -v golangci-lint >/dev/null 2>&1; then
    if ! golangci-lint run --new-from-rev="$BASE" ./... ; then
        FAILED=1; fail "golangci-lint new-code"
    else note "ok"; fi
else
    note "golangci-lint not installed; skipping (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"
fi

# --- 3. go vet (whole tree; cheap)
run "go vet" go vet -mod=vendor ./... || exit 1

# --- 4. Semgrep slop rules on ADDED lines only (new-code gating)
if command -v semgrep >/dev/null 2>&1; then
    step "semgrep slop (added lines)"
    if ! python3 scripts/quality/semgrep-diff.py --base "$BASE"; then
        FAILED=1; fail "semgrep"
    else note "ok"; fi
fi

# --- 5. Diff-level checks (only meaningful on a real diff)
if [ "$BASE" != "HEAD" ]; then
    step "diff-check vs ${BASE:0:12}"
    if ! python3 scripts/quality/diff-check.py --base "$BASE"; then
        FAILED=1; fail "diff-check"
    else note "ok"; fi
fi

# --- 6. Debt ratchet (per-rule totals may only shrink)
step "lint-debt ratchet"
LINT_JSON="$(mktemp --suffix=-gate-lint.json)"
golangci-lint run --output.json.path="$LINT_JSON" --output.text.path=/dev/null ./... >/dev/null 2>&1 || true
if ! python3 scripts/quality/ratchet.py "$LINT_JSON"; then
    FAILED=1; fail "ratchet"
else note "ok"; fi
rm -f "$LINT_JSON"

[ "$QUICK" = 1 ] && { [ "$FAILED" = 0 ] && step "gate: PASS (quick)" || exit 1; [ "$FAILED" = 0 ] && exit 0; exit 1; }

# --- 7. Tests with race + shuffle (slowest; run last among the always-on set)
step "go test -race -shuffle=on"
if ! go test -mod=vendor -race -shuffle=on -count=1 ./... ; then
    FAILED=1; fail "go test"
fi

# --- 8. govulncheck (network-dependent; non-fatal if offline)
if command -v govulncheck >/dev/null 2>&1; then
    step "govulncheck"
    if ! govulncheck -mode=query ./... >/dev/null 2>&1; then
        # query mode needs no network if the vuln db is cached; fall back to scan
        if ! govulncheck ./... ; then
            FAILED=1; fail "govulncheck"
        else note "ok"; fi
    else note "ok"; fi
fi

# --- 9. go.mod / go.sum drift
step "go mod tidy drift"
cp go.mod /tmp/gate-go.mod; cp go.sum /tmp/gate-go.sum
go mod tidy >/dev/null 2>&1 || true
if ! git diff --quiet -- go.mod go.sum 2>/dev/null && ! cmp -s go.mod /tmp/gate-go.mod; then
    FAILED=1; fail "go.mod/go.sum drift — run 'go mod tidy' and commit"
else
    cp /tmp/gate-go.mod go.mod; cp /tmp/gate-go.sum go.sum; note "ok"
fi

if [ "$FAILED" = 0 ]; then
    step "gate: PASS"
    exit 0
fi
printf '\n\033[31mGATE FAILED\033[0m — fix the findings above (file:line + rule + hint), do not suppress them.\n' >&2
exit 1
