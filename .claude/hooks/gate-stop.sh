#!/usr/bin/env bash
# Stop hook: the agent may not declare work done while the quality gate fails.
#
# Reads the Stop event JSON on stdin, runs the cheap gate checks (build, lint on
# new code, vet, slop rules on added lines, diff checks, debt ratchet), and
# exits 2 with the findings on stderr so the agent is sent back to fix them.
#
# Loop guard: repeated failures are counted per session in a state file. After
# GATE_MAX_BLOCKS consecutive failures the hook stops blocking and escalates to
# a human on stdout, so a broken gate cannot loop the agent forever. Claude
# Code's own 8-continuation cap is the outer backstop.
set -uo pipefail
cd "${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"

STATE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/ompig-gate"
mkdir -p "$STATE_DIR"

# Parse stdin JSON without requiring jq.
INPUT="$(cat)"
SESSION="$(printf '%s' "$INPUT" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
[ -z "$SESSION" ] && SESSION="default"
COUNT_FILE="$STATE_DIR/$SESSION.count"
COUNT="$(cat "$COUNT_FILE" 2>/dev/null || echo 0)"

MAX_BLOCKS="${GATE_MAX_BLOCKS:-3}"
export PATH="$PATH:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin"

# Run the cheap gate. Capture output to feed back on failure.
OUT="$(bash scripts/quality/gate.sh --quick 2>&1)"
STATUS=$?

if [ "$STATUS" -eq 0 ]; then
    rm -f "$COUNT_FILE"
    exit 0
fi

COUNT=$((COUNT + 1))
printf '%s' "$COUNT" > "$COUNT_FILE"

if [ "$COUNT" -ge "$MAX_BLOCKS" ]; then
    # Escalate instead of looping. Allow the stop but make the failure loud.
    {
        printf '\n*** QUALITY GATE STILL FAILING after %d blocked attempts ***\n' "$COUNT"
        printf 'The gate cannot be satisfied automatically; a human must review.\n'
        printf 'Fix the findings below, or if a rule is genuinely wrong, ask the human\n'
        printf 'before touching gate config. Do NOT weaken .golangci.yml, .semgrep/, or\n'
        printf 'scripts/quality/ to make it pass.\n\n'
        printf '%s\n' "$OUT"
    } >&2
    # Exit 0: allow the stop so the human sees the final state, not a loop.
    exit 0
fi

# Block the stop and hand the findings back to the agent.
{
    printf 'QUALITY GATE FAILED (attempt %d/%d). Fix these before finishing:\n\n' "$COUNT" "$MAX_BLOCKS"
    printf '%s\n' "$OUT" | grep -vE '^\s*$' | tail -60
    printf '\nFix the code; do not add //nolint, t.Skip, or weaken gate config.\n'
} >&2
exit 2
