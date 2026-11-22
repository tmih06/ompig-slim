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
export PATH="$PATH:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin:$HOME/.local/bin"

QUICK=0
[ "${1:-}" = "--quick" ] && QUICK=1

TMP_WORK="$(mktemp -d "${TMPDIR:-/tmp}/ompig-gate.XXXXXX")"
trap 'rm -rf "$TMP_WORK"' EXIT

# Resolve the diff base once: PR base, else merge-base with main, else HEAD.
BASE="${PR_BASE:-}"
if [ -n "$BASE" ] && ! git rev-parse --verify "${BASE}^{commit}" >/dev/null 2>&1; then
    BASE=""
fi
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

# --- 4. Semgrep rule validation, fixture self-test, and diff-scoped scan
if command -v semgrep >/dev/null 2>&1; then
    step "semgrep slop (validate + fixtures + added lines)"
    if ! semgrep --validate --config .semgrep/slop.yml >/dev/null 2>&1; then
        semgrep --validate --config .semgrep/slop.yml >&2
        FAILED=1; fail "semgrep --validate"
    elif ! semgrep --test --config .semgrep/slop.yml .semgrep/slop.go >/dev/null 2>&1; then
        semgrep --test --config .semgrep/slop.yml .semgrep/slop.go >&2
        FAILED=1; fail "semgrep --test fixtures"
    elif ! python3 scripts/quality/semgrep-diff.py --base "$BASE"; then
        FAILED=1; fail "semgrep"
    else
        note "ok"
    fi
fi

# --- 5. Diff-level checks (only meaningful on a real diff)
if [ "$BASE" != "HEAD" ]; then
    step "diff-check vs ${BASE:0:12}"
    if ! python3 scripts/quality/diff-check.py --base "$BASE"; then
        FAILED=1; fail "diff-check"
    else note "ok"; fi
fi

# --- 6. Debt ratchet (per-rule totals may only shrink)
if command -v golangci-lint >/dev/null 2>&1; then
    step "lint-debt ratchet"
    LINT_JSON="$TMP_WORK/gate-lint.json"
    golangci-lint run --output.json.path="$LINT_JSON" --output.text.path=/dev/null ./... >/dev/null 2>&1 || true
    if ! python3 scripts/quality/ratchet.py "$LINT_JSON"; then
        FAILED=1; fail "ratchet"
    else note "ok"; fi
fi

[ "$QUICK" = 1 ] && { [ "$FAILED" = 0 ] && { step "gate: PASS (quick)"; exit 0; }; exit 1; }

# --- 7. Tests with race + shuffle (slowest; run last among the always-on set)
step "go test -race -shuffle=on"
if ! go test -mod=vendor -race -shuffle=on -count=1 ./... ; then
    FAILED=1; fail "go test"
fi

# --- 8. govulncheck (advisory: vuln DB updates asynchronously and vendored PiG
# carries its own transitive pins; report findings without blocking the PR)
if command -v govulncheck >/dev/null 2>&1; then
    step "govulncheck (advisory)"
    VULN_OUT="$TMP_WORK/gate-vuln.txt"
    if govulncheck ./... >"$VULN_OUT" 2>&1; then
        note "ok — no known vulnerabilities"
    else
        if grep -qE 'requires newer Go version|cannot range over|unknown field' "$VULN_OUT"; then
            note "warn — govulncheck binary predates this toolchain; skipping (CI pins a compatible build)"
        else
            note "warn — advisory vulnerabilities reported in transitive dependencies:"
            grep -E '^(Vulnerability #[0-9]+:|    Found in:|    Fixed in:|Your code is affected)' "$VULN_OUT" | sed 's/^/     /' >&2 || true
        fi
    fi
fi

# --- 9. go.mod / go.sum / vendor/modules.txt drift
# `go mod tidy` cannot run directly because vendor/github.com/MichaelKinsy/PiG
# contains maintained local packages absent from upstream PiG@v0.7.0. Instead,
# verify canonical go.mod formatting, module checksums, and exact agreement
# between go.mod, go.sum, and vendor/modules.txt.
step "go.mod / vendor manifest drift"
cp go.mod "$TMP_WORK/go.mod"
go mod edit -fmt
if ! cmp -s go.mod "$TMP_WORK/go.mod"; then
    cp "$TMP_WORK/go.mod" go.mod
    FAILED=1; fail "go.mod is not canonically formatted — run 'go mod edit -fmt'"
elif ! python3 - <<'PY'
import pathlib, re, sys

gomod = pathlib.Path("go.mod").read_text()
gosum = pathlib.Path("go.sum").read_text()
modules_txt = pathlib.Path("vendor/modules.txt").read_text()

req_re = re.compile(r"^\s*([A-Za-z0-9._/\-]+)\s+(v[0-9][^\s/]*)", re.M)
in_req = False
mod_reqs = {}
for line in gomod.splitlines():
    s = line.split("//")[0].strip()
    if s.startswith("require ("):
        in_req = True
        continue
    if in_req and s == ")":
        in_req = False
        continue
    if s.startswith("require "):
        m = req_re.match(s[len("require "):])
        if m:
            mod_reqs[m.group(1)] = m.group(2)
    elif in_req:
        m = req_re.match(s)
        if m:
            mod_reqs[m.group(1)] = m.group(2)

vendor_mods = {}
for line in modules_txt.splitlines():
    if line.startswith("# "):
        parts = line[2:].strip().split()
        if len(parts) >= 2:
            vendor_mods[parts[0]] = parts[1]

sum_mods = set()
for line in gosum.splitlines():
    parts = line.split()
    if len(parts) >= 2:
        sum_mods.add((parts[0], parts[1].split("/")[0]))

errors = []
for mod, ver in sorted(mod_reqs.items()):
    if vendor_mods.get(mod) != ver:
        errors.append(f"go.mod requires {mod}@{ver}, but vendor/modules.txt has {vendor_mods.get(mod)!r}")
    if not mod.startswith("github.com/MichaelKinsy/PiG") and (mod, ver) not in sum_mods:
        errors.append(f"go.mod requires {mod}@{ver}, which is missing from go.sum")

for mod, ver in sorted(vendor_mods.items()):
    if mod_reqs.get(mod) != ver:
        errors.append(f"vendor/modules.txt lists {mod}@{ver}, which does not match go.mod ({mod_reqs.get(mod)!r})")

bootstrap = pathlib.Path("vendor/.bootstrap-modules.txt")
if bootstrap.exists() and bootstrap.read_text() != modules_txt:
    errors.append("vendor/.bootstrap-modules.txt is out of sync with vendor/modules.txt — run 'make deps'")

if errors:
    for e in errors:
        print("  " + e, file=sys.stderr)
    sys.exit(1)
PY
then
    FAILED=1; fail "go.mod / go.sum / vendor/modules.txt drift"
else
    note "ok"
fi

if [ "$FAILED" = 0 ]; then
    step "gate: PASS"
    exit 0
fi
printf '\n\033[31mGATE FAILED\033[0m — fix the findings above (file:line + rule + hint), do not suppress them.\n' >&2
exit 1
