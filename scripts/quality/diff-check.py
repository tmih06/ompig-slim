#!/usr/bin/env python3
"""Diff-level quality checks for a pull request.

Runs on the diff between a base ref and HEAD and fails (exit 1) when the diff:

  * adds ``//nolint``, ``t.Skip(``, or ``//go:build ignore``
  * removes test assertions, or deletes a test file without a replacement
  * touches gate config, ``.semgrep/``, ``scripts/quality/``, CI workflows or
    coverage config without a CODEOWNER approval signal
  * changes ``go.mod`` without a justification line in the PR description
  * exceeds the size cap calibrated to this repo's typical PR

Emit one ``file:line`` finding per violation so the agent can fix it.

Environment:
  PR_BODY                 pull-request description text (for go.mod justification)
  OWNER_APPROVED          "true" when a gate-config change has CODEOWNER approval
  DIFF_SIZE_CAP           max added+removed lines (default 1200)

Usage:
  bash:  scripts/quality/diff-check.py --base origin/main
  stdin: git diff origin/main...HEAD | scripts/quality/diff-check.py --stdin
"""
import os
import re
import subprocess
import sys

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Gate-owned surfaces an agent must not weaken. Any change here needs
# OWNER_APPROVED (CI maps this to a CODEOWNERS approval).
GATE_PATHS = re.compile(
    r"^("
    r"\.golangci\.(yml|yaml)$"
    r"|\.semgrep/"
    r"|scripts/quality/"
    r"|\.github/workflows/"
    r"|docs/quality-baseline\.(json|md)$"
    r"|\.github/CODEOWNERS$"
    r")"
)

ADDED_PATTERNS = [
    (re.compile(r"//nolint\b"), "//nolint requires a human-approved reason"),
    (re.compile(r"\bt\.Skip(?:f|Now)?\s*\("), "new t.Skip — cover the case or fix the test instead"),
    (re.compile(r"//go:build\s+ignore"), "//go:build ignore hides code from the gate"),
]

# Lines that, when removed from a test, usually mean an assertion was deleted.
TEST_ASSERT_RE = re.compile(
    r"\b("
    r"t\.Error|t\.Fatal|t\.Fail|t\.Errorf|t\.Fatalf|"
    r"assert\.|require\.|"
    r"expect"
    r")"
)
TEST_FILE_RE = re.compile(r"_test\.go$")


def git_diff(base):
    out = subprocess.run(
        ["git", "-C", REPO, "diff", "--unified=0", f"{base}...HEAD"],
        capture_output=True, text=True, check=True,
    )
    return out.stdout


def parse_diff(diff_text):
    """Yield (file, kind, lineno, text) for each added/removed content line."""
    current = None
    new_line = 0
    for raw in diff_text.splitlines():
        if raw.startswith("+++ b/"):
            current = raw[6:]
        elif raw.startswith("@@"):
            m = re.search(r"\+(\d+)", raw)
            new_line = int(m.group(1)) if m else 0
        elif current is not None:
            if raw.startswith("+") and not raw.startswith("+++"):
                yield current, "add", new_line, raw[1:]
                new_line += 1
            elif raw.startswith("-") and not raw.startswith("---"):
                yield current, "del", None, raw[1:]
            elif not raw.startswith("\\"):
                new_line += 1


def main():
    args = sys.argv[1:]
    if "--stdin" in args:
        diff_text = sys.stdin.read()
    else:
        base = "HEAD"
        if "--base" in args:
            base = args[args.index("--base") + 1]
        elif "PR_BASE" in os.environ:
            base = os.environ["PR_BASE"]
        diff_text = git_diff(base)

    body = os.environ.get("PR_BODY", "")
    owner_ok = os.environ.get("OWNER_APPROVED", "").lower() == "true"
    cap = int(os.environ.get("DIFF_SIZE_CAP", "1200"))

    findings = []
    touched = set()
    deleted_tests = set()
    added_tests = set()
    removed_asserts = []
    added_lines = removed_lines = 0
    touched_gate = set()
    touched_gomod = False

    for path, kind, lineno, text in parse_diff(diff_text):
        touched.add(path)
        if kind == "add":
            added_lines += 1
        else:
            removed_lines += 1

        if GATE_PATHS.match(path):
            touched_gate.add(path)
        if path == "go.mod":
            touched_gomod = True
        if TEST_FILE_RE.search(path):
            if kind == "add":
                added_tests.add(path)
            else:
                removed_asserts.append((path, text)) if TEST_ASSERT_RE.search(text) else None

        if kind == "add" and path.endswith(".go"):
            for rx, hint in ADDED_PATTERNS:
                if rx.search(text):
                    findings.append(f"{path}:{lineno}: {rx.pattern} -> {hint}")

    # Deleted test files are only OK if a same-package test file is added.
    try:
        deleted = subprocess.run(
            ["git", "-C", REPO, "diff", "--diff-filter=D", "--name-only", "HEAD"],
            capture_output=True, text=True, check=True,
        ).stdout.split()
        for path in deleted:
            if TEST_FILE_RE.search(path):
                deleted_tests.add(path)
    except subprocess.CalledProcessError:
        deleted = []
    for path in deleted_tests:
        if not any(t.startswith(path.rsplit("_test.go", 1)[0]) for t in added_tests):
            findings.append(
                f"{path}: test file deleted without a replacement in the same package"
            )

    if removed_asserts and not added_tests:
        findings.append(
            f"tests: {len(removed_asserts)} assertion line(s) removed and no "
            "new test assertions added — this can silently weaken coverage"
        )

    if touched_gate and not owner_ok:
        for p in sorted(touched_gate):
            findings.append(
                f"{p}: changes quality-gate config or CI; needs CODEOWNER approval "
                "(set OWNER_APPROVED=true in CI when an owner approves)"
            )

    if touched_gomod:
        if not re.search(r"(?im)^\s*(deps?|dependency|justif)[:\-]", body):
            findings.append(
                "go.mod: changed without a justification line in the PR "
                "description (add `Deps: <reason>` or `Justification: <reason>`)"
            )

    total = added_lines + removed_lines
    if total > cap:
        findings.append(
            f"diff: {total} changed lines exceeds the {cap}-line cap; "
            "split the change into smaller, reviewable commits"
        )

    if findings:
        print("diff-check FAIL:", file=sys.stderr)
        for f in findings:
            print("  " + f, file=sys.stderr)
        return 1
    print(f"diff-check OK: {added_lines}+/{removed_lines}- across {len(touched)} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
