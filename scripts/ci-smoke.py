#!/usr/bin/env python3
"""Run one of two disjoint CI shards of the canonical smoke-built recipe.

The recipe contains independent Python scenarios, each owning its temporary
profile and loopback servers. Read it through make rather than duplicating its
list here: new scenarios must automatically run in CI as well as locally.
"""
import argparse
import shlex
import subprocess


def commands(recipe):
    """Parse make's dry-run output; reject non-Python commands before executing.

    Shell operators and prerequisite builds are intentionally unsupported. If
    smoke-built gains dependencies between commands, revisit sharding instead
    of silently running an incomplete or reordered suite.
    """
    result = [shlex.split(line) for line in recipe.splitlines() if line.strip()]
    if not result or any(command[0] != "python3" for command in result):
        raise ValueError("smoke-built must contain only independent Python commands")
    return result


def main():
    """Execute shard 0 or 1 in recipe order, propagating any failed scenario."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shard", type=int, choices=(0, 1))
    args = parser.parse_args()
    recipe = subprocess.check_output(
        ["make", "--no-print-directory", "--dry-run", "smoke-built"], text=True,
    )
    for command in commands(recipe)[args.shard::2]:
        print(shlex.join(command), flush=True)
        subprocess.run(command, check=True)


if __name__ == "__main__":
    main()
