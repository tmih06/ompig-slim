#!/usr/bin/env python3
"""Compare a golangci-lint JSON report against the recorded baseline.

Per-rule finding totals may only go DOWN relative to docs/quality-baseline.json.
A rule whose count rises is new debt on existing files and fails the gate;
`new-from-rev` already gates newly introduced findings, so this ratchet exists
to stop debt creeping in through edits that don't touch new lines.

Usage:
  python3 scripts/quality/ratchet.py lint.json            # check, exit 1 if a rule grew
  python3 scripts/quality/ratchet.py lint.json --write    # rewrite baseline to current
"""
import collections
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
BASELINE = ROOT / "docs" / "quality-baseline.json"
SEMGREP_CONFIG = ROOT / ".semgrep" / "slop.yml"


def load(path):
    return json.loads(pathlib.Path(path).read_text())


def counts(report):
    c = collections.Counter()
    for issue in report.get("Issues", []):
        c[issue.get("FromLinter", "?")] += 1
    return c


def semgrep_counts():
    """Current per-rule semgrep findings, or None if semgrep is unavailable."""
    try:
        out = subprocess.run(
            ["semgrep", "--config", str(SEMGREP_CONFIG), "--json", "-o", "/dev/stdout", str(ROOT)],
            capture_output=True, text=True, timeout=180, cwd=ROOT,
        )
        data = json.loads(out.stdout)
    except (FileNotFoundError, json.JSONDecodeError, subprocess.TimeoutExpired):
        return None
    if data.get("errors"):
        raise RuntimeError(f"semgrep reported {len(data['errors'])} rule error(s)")
    c = collections.Counter()
    for r in data.get("results", []):
        c[r.get("check_id", "?").split(".")[-1]] += 1
    return c


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    report_path = argv[1]
    write = "--write" in argv[2:]
    try:
        report = load(report_path)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"ratchet: cannot read golangci JSON {report_path}: {exc}", file=sys.stderr)
        return 2
    current = counts(report)
    baseline_doc = load(BASELINE)
    baseline = {k: int(v) for k, v in baseline_doc.get("rules", {}).items()}
    baseline_sg = {k: int(v) for k, v in baseline_doc.get("semgrep_rules", {}).items()}
    try:
        current_sg = semgrep_counts()
    except RuntimeError as exc:
        print(f"ratchet FAIL: {exc}", file=sys.stderr)
        return 2

    if write:
        total = sum(current.values())
        doc = {
            "_comment": baseline_doc.get("_comment", ""),
            "total": total,
            "rules": {k: current[k] for k in sorted(current)},
        }
        if current_sg is not None:
            doc["semgrep_rules"] = {k: current_sg[k] for k in sorted(current_sg)}
        BASELINE.write_text(json.dumps(doc, indent=2) + "\n")
        print(f"ratchet: baseline updated to {total} lint findings across {len(current)} rules")
        return 0

    regressions = []
    for rule, base in baseline.items():
        now = current.get(rule, 0)
        if now > base:
            regressions.append(("lint/" + rule, base, now))
    for rule, now in current.items():
        if rule not in baseline and now > 0:
            regressions.append(("lint/" + rule, 0, now))
    if current_sg is not None:
        for rule, base in baseline_sg.items():
            now = current_sg.get(rule, 0)
            if now > base:
                regressions.append(("semgrep/" + rule, base, now))
        for rule, now in current_sg.items():
            if rule not in baseline_sg and now > 0:
                regressions.append(("semgrep/" + rule, 0, now))

    if regressions:
        print("ratchet FAIL: lint debt increased on existing files:", file=sys.stderr)
        for rule, base, now in sorted(regressions):
            delta = now - base
            print(f"  {rule}: {base} -> {now} (+{delta})", file=sys.stderr)
        print(
            "Fix the new findings, or reduce the count elsewhere in the same "
            "rule. Do not raise docs/quality-baseline.json to hide debt; "
            "lowering it is always welcome.",
            file=sys.stderr,
        )
        return 1
    note = "" if current_sg is not None else " (semgrep unavailable; skipped its ratchet)"
    print(f"ratchet OK: no lint rule grew (current lint total {sum(current.values())}){note}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
