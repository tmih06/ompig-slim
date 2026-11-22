#!/usr/bin/env python3
"""Run the Semgrep slop rules and report only findings on lines added by a diff.

`new-from-rev` for Semgrep does not exist the way golangci-lint's does, so this
computes the set of added lines per file from `git diff <merge-base>` (covering
committed PR changes plus staged, unstaged, and untracked working-tree files),
runs the rules on the changed Go files, and keeps a finding only when its range
touches an added line. Pre-existing debt in a touched file stays out of the way;
new slop does not.

Usage:
  python3 scripts/quality/semgrep-diff.py [--base REF] [--config config/semgrep/slop.yml]
Exit 1 when any finding survives, 2 on Semgrep config/parse error, 0 otherwise.
"""
import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def resolve_merge_base(base):
    if base == "HEAD":
        return "HEAD"
    mb = subprocess.run(
        ["git", "-C", ROOT, "merge-base", base, "HEAD"],
        capture_output=True, text=True,
    )
    if mb.returncode == 0 and mb.stdout.strip():
        return mb.stdout.strip()
    return base


def added_lines(base):
    """Map filepath -> set of added line numbers from merge-base to working tree + untracked."""
    ref = resolve_merge_base(base)
    out = subprocess.run(
        ["git", "-C", ROOT, "diff", "--unified=0", "--diff-filter=ACMR", ref],
        capture_output=True, text=True, check=True,
    ).stdout
    files = {}
    current = None
    new_line = 0
    for raw in out.splitlines():
        if raw.startswith("+++ b/"):
            current = raw[6:]
            files.setdefault(current, set())
        elif raw.startswith("@@"):
            m = re.search(r"\+(\d+)(?:,(\d+))?", raw)
            new_line = int(m.group(1)) if m else 0
        elif current is not None:
            if raw.startswith("+") and not raw.startswith("+++"):
                files[current].add(new_line)
                new_line += 1
            elif not raw.startswith("-") and not raw.startswith("\\") and not raw.startswith("diff "):
                new_line += 1

    # Include untracked files in the working tree so local hooks cover them too.
    untracked = subprocess.run(
        ["git", "-C", ROOT, "ls-files", "--others", "--exclude-standard"],
        capture_output=True, text=True, check=True,
    ).stdout.splitlines()
    for rel in untracked:
        full = os.path.join(ROOT, rel)
        if os.path.isfile(full):
            try:
                with open(full, "r", encoding="utf-8", errors="replace") as fh:
                    line_count = len(fh.read().splitlines())
            except OSError:
                continue
            files[rel] = set(range(1, line_count + 1))
    return files


def main():
    args = sys.argv[1:]
    base = os.environ.get("PR_BASE", "HEAD")
    config = os.path.join(ROOT, "config", "semgrep", "slop.yml")
    if "--base" in args:
        base = args[args.index("--base") + 1]
    if "--config" in args:
        config = args[args.index("--config") + 1]

    added = added_lines(base)
    targets = [
        f for f in added
        if f.endswith(".go")
        and not f.startswith("vendor/")
        and not f.startswith("config/semgrep/")   # our own fixtures must never scan
        and os.path.isfile(os.path.join(ROOT, f))
    ]
    if not targets:
        print("semgrep-diff: no changed Go files")
        return 0

    proc = subprocess.run(
        ["semgrep", "--config", config, "--json", "-o", "/dev/stdout"] + targets,
        capture_output=True, text=True, cwd=ROOT, timeout=180,
    )
    try:
        data = json.loads(proc.stdout)
    except json.JSONDecodeError:
        print("semgrep-diff: could not parse semgrep output", file=sys.stderr)
        print(proc.stderr[-2000:], file=sys.stderr)
        return 2

    errors = data.get("errors", [])
    if errors:
        print(f"semgrep-diff ERROR: {len(errors)} rule configuration/parse error(s):", file=sys.stderr)
        for err in errors:
            msg = err.get("message") or err.get("long_msg") or str(err)
            print(f"  {msg}", file=sys.stderr)
        return 2

    findings = []
    for r in data.get("results", []):
        path = os.path.relpath(r["path"], ROOT)
        start_line = r["start"]["line"]
        end_line = r.get("end", {}).get("line", start_line)
        file_added = added.get(path, set())
        if any(line in file_added for line in range(start_line, end_line + 1)):
            findings.append((path, start_line, r["check_id"].split(".")[-1], r["extra"]["message"].strip()))

    if findings:
        print("semgrep-diff FAIL: new code hit a slop rule:", file=sys.stderr)
        for path, line, rid, msg in findings:
            first = msg.splitlines()[0] if msg else ""
            print(f"  {path}:{line}  [{rid}] {first}", file=sys.stderr)
        return 1
    print(f"semgrep-diff OK: no slop findings on {sum(len(v) for v in added.values())} added lines")
    return 0


if __name__ == "__main__":
    sys.exit(main())
