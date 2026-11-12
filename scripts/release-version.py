#!/usr/bin/env python3
"""Print the next patch's beta tag, using CI's monotonically increasing run ID.

The run number belongs to the existing CI workflow, so reruns reuse the same
number and concurrent merges cannot allocate the same beta number. Stable
releases remain manual tags; a beta never advances GitHub's latest pointer.
"""
import re
import sys


def beta_tag(stable, number):
    """Return v<next patch>-beta.<run number>; reject non-stable inputs."""
    match = re.fullmatch(r"v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", stable)
    if not match or not re.fullmatch(r"[1-9][0-9]*", number):
        raise ValueError("expected a stable semver tag and a positive CI run number")
    major, minor, patch = map(int, match.groups())
    return f"v{major}.{minor}.{patch + 1}-beta.{number}"


if __name__ == "__main__":
    print(beta_tag(*sys.argv[1:]))
