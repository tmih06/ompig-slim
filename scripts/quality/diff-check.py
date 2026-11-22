#!/usr/bin/env python3
"""Diff-level quality checks for a pull request or local working tree.

Runs on the diff between the merge-base with a base ref and the working tree
(covering committed PR changes plus staged, unstaged, and untracked files) and
fails (exit 1) when the diff:

  * adds ``//nolint``, ``//nosemgrep``, ``//lint:ignore``, ``t.Skip(``, or
    ``//go:build ignore``
  * removes test assertions without adding new test assertions, or deletes a
    test file without a replacement in the same package
  * modifies disposable ``vendor/`` files outside ``vendor/modules.txt`` and
    maintained ``vendor/github.com/MichaelKinsy/PiG/``
  * touches gate config, ``.semgrep/``, ``scripts/quality/``, ``Makefile``, CI
    workflows, baseline, ``.claude/``, or ``AGENTS.md``/``CLAUDE.md`` without a
    CODEOWNER approval signal
  * changes ``go.mod`` without a justification line in the PR description
  * exceeds the size cap calibrated to this repo's typical PR (unless owner-approved)

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
# OWNER_APPROVED (CI maps this to owner authorship or CODEOWNERS approval).
GATE_PATHS = re.compile(
    r"^("
    r"\.golangci\.(yml|yaml)$"
    r"|\.semgrep/"
    r"|\.semgrepignore$"
    r"|scripts/quality/"
    r"|\.github/workflows/"
    r"|docs/quality-baseline\.(json|md)$"
    r"|\.github/CODEOWNERS$"
    r"|\.claude/"
    r"|Makefile$"
    r"|AGENTS\.md$"
    r"|CLAUDE\.md$"
    r")"
)

ADDED_PATTERNS = [
    (re.compile(r"//\s*nolint\b"), "//nolint requires a human-approved reason"),
    (re.compile(r"//\s*nosemgrep\b"), "//nosemgrep suppresses slop rules; fix the code instead"),
    (re.compile(r"//\s*lint:ignore\b"), "//lint:ignore suppresses staticcheck; fix the code instead"),
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


def resolve_merge_base(base):
    if base == "HEAD":
        return "HEAD"
    mb = subprocess.run(
        ["git", "-C", REPO, "merge-base", base, "HEAD"],
        capture_output=True, text=True,
    )
    if mb.returncode == 0 and mb.stdout.strip():
        return mb.stdout.strip()
    return base


def git_diff(base):
    """Diff from merge-base to working tree (committed + staged + unstaged + untracked)."""
    ref = resolve_merge_base(base)
    out = subprocess.run(
        ["git", "-C", REPO, "diff", "--unified=0", ref],
        capture_output=True, text=True, check=True,
    ).stdout

    untracked = subprocess.run(
        ["git", "-C", REPO, "ls-files", "--others", "--exclude-standard"],
        capture_output=True, text=True, check=True,
    ).stdout.splitlines()
    chunks = [out]
    for rel in untracked:
        full = os.path.join(REPO, rel)
        if not os.path.isfile(full):
            continue
        try:
            lines = open(full, "r", encoding="utf-8", errors="replace").splitlines()
        except OSError:
            continue
        chunks.append(f"diff --git a/{rel} b/{rel}")
        chunks.append("--- /dev/null")
        chunks.append(f"+++ b/{rel}")
        chunks.append(f"@@ -0,0 +1,{len(lines)} @@")
        for line in lines:
            chunks.append("+" + line)
    return "\n".join(chunks)


def parse_diff(diff_text):
    """Yield (file, kind, lineno, text) for each added/removed line or deleted file."""
    current = None
    old_file = None
    new_line = 0
    for raw in diff_text.splitlines():
        if raw.startswith("--- a/"):
            old_file = raw[6:]
        elif raw.startswith("--- /dev/null"):
            old_file = None
        elif raw.startswith("+++ /dev/null"):
            if old_file is not None:
                yield old_file, "file_del", None, ""
            current = old_file
        elif raw.startswith("+++ b/"):
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


def is_disposable_vendor(path):
    if not path.startswith("vendor/"):
        return False
    if path == "vendor/modules.txt":
        return False
    if path.startswith("vendor/github.com/MichaelKinsy/PiG/"):
        return False
    return True


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
    added_test_files = set()
    added_asserts = []
    removed_asserts = []
    added_lines = removed_lines = 0
    touched_gate = set()
    touched_disposable_vendor = set()
    touched_gomod = False

    for path, kind, lineno, text in parse_diff(diff_text):
        touched.add(path)
        if kind == "file_del":
            if TEST_FILE_RE.search(path):
                deleted_tests.add(path)
            continue

        if kind == "add":
            added_lines += 1
        else:
            removed_lines += 1

        if GATE_PATHS.match(path):
            touched_gate.add(path)
        if is_disposable_vendor(path):
            touched_disposable_vendor.add(path)
        if path == "go.mod":
            touched_gomod = True
        if TEST_FILE_RE.search(path):
            if kind == "add":
                added_test_files.add(path)
                if TEST_ASSERT_RE.search(text):
                    added_asserts.append((path, text))
            elif TEST_ASSERT_RE.search(text):
                removed_asserts.append((path, text))

        if kind == "add" and path.endswith(".go"):
            for rx, hint in ADDED_PATTERNS:
                if rx.search(text):
                    findings.append(f"{path}:{lineno}: {rx.pattern} -> {hint}")

    # Deleted test files are only OK if a same-package test file has additions.
    for path in sorted(deleted_tests):
        pkg_dir = os.path.dirname(path)
        if not any(os.path.dirname(t) == pkg_dir and t not in deleted_tests for t in added_test_files):
            findings.append(
                f"{path}: test file deleted without a replacement in the same package"
            )

    if removed_asserts and not added_asserts:
        findings.append(
            f"tests: {len(removed_asserts)} assertion line(s) removed and no "
            "new test assertions added — this can silently weaken coverage"
        )

    for p in sorted(touched_disposable_vendor):
        findings.append(
            f"{p}: disposable vendor file modified; only vendor/modules.txt and "
            "vendor/github.com/MichaelKinsy/PiG/ are maintained in git"
        )

    if touched_gate and not owner_ok:
        for p in sorted(touched_gate):
            findings.append(
                f"{p}: changes quality-gate config or CI; needs CODEOWNER approval "
                "(set OWNER_APPROVED=true in CI when an owner approves)"
            )

    if touched_gomod:
        if not re.search(r"(?im)^\s*(deps?|dependenc(?:y|ies)|justif(?:ication)?)[:\-]", body):
            findings.append(
                "go.mod: changed without a justification line in the PR "
                "description (add `Deps: <reason>` or `Justification: <reason>`)"
            )

    total = added_lines + removed_lines
    if total > cap and not owner_ok:
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
