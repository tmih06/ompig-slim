#!/usr/bin/env bash
# Prepare a stable release commit, and optionally publish its branch, tag and CI.
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/release.sh VERSION [--publish]

Prepare a stable release commit. Add --publish to push main, create the
annotated tag, wait for tag CI, and verify the GitHub release. Without
--publish the script stops after the local commit.
EOF
}

if (($# == 1)) && [[ "$1" == "--help" || "$1" == "-h" ]]; then usage; exit 0; fi
if (($# < 1 || $# > 2)); then usage >&2; exit 2; fi
version="${1#v}"
publish=false
if (($# == 2)); then
    [[ "$2" == "--publish" ]] || { usage >&2; exit 2; }
    publish=true
fi
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf 'stable VERSION must be MAJOR.MINOR.PATCH: %s\n' "$version" >&2; exit 2; }
tag="v$version"
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

command -v git >/dev/null || { echo 'git is required' >&2; exit 1; }
command -v go >/dev/null || { echo 'go is required' >&2; exit 1; }
if $publish; then
    command -v gh >/dev/null || { echo 'gh is required for --publish' >&2; exit 1; }
    repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
fi

[[ "$(git branch --show-current)" == main ]] || { echo 'release must run on main' >&2; exit 1; }
[[ -z "$(git status --short)" ]] || { echo 'release requires a clean worktree' >&2; exit 1; }
git show-ref --verify --quiet "refs/tags/$tag" && { echo "tag already exists locally: $tag" >&2; exit 1; }
if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
    echo "tag already exists on origin: $tag" >&2
    exit 1
fi

version_file=vendor/github.com/MichaelKinsy/PiG/internal/coding/pigversion/harness.go
python3 - "$version" "$version_file" <<'PY'
import pathlib
import re
import sys

version, version_file = sys.argv[1:]
path = pathlib.Path(version_file)
text = path.read_text()
updated, count = re.subn(r'(var HarnessVersion = ")([^"]+)(")', rf'\g<1>{version}\g<3>', text, count=1)
if count != 1:
    raise SystemExit(f"could not find HarnessVersion in {path}")
path.write_text(updated)
readme = pathlib.Path("README.md")
text = readme.read_text()
text, badge = re.subn(r'version-[0-9]+\.[0-9]+\.[0-9]+-blue', f'version-{version}-blue', text, count=1)
text, pin = re.subn(r'Pass `--version v[0-9]+\.[0-9]+\.[0-9]+` to pin a release', f'Pass `--version v{version}` to pin a release', text, count=1)
if badge != 1 or pin != 1:
    raise SystemExit("could not update both release references in README.md")
readme.write_text(text)
PY

make check
VERSION="$version" make release
local_binary=$(mktemp)
trap 'rm -f "$local_binary"' EXIT
go build -mod=vendor -trimpath -ldflags="-X github.com/MichaelKinsy/PiG/internal/coding/pigversion.HarnessVersion=$version" -o "$local_binary" ./cmd/ompig-slim
[[ "$($local_binary --version)" == "$version" ]] || { echo 'local binary reported the wrong version' >&2; exit 1; }
python3 scripts/verify-releases.py bin/releases

git add README.md "$version_file"
git diff --cached --check
git commit -m "chore(release): prepare version $version" -m "Update the user-facing and source-build release identity for $version. Historical benchmark and upstream version references remain unchanged."
commit=$(git rev-parse HEAD)
printf 'Prepared release %s at %s\n' "$tag" "$commit"

if ! $publish; then
    echo 'Not published; push main, create the annotated tag, and verify tag CI manually.'
    exit 0
fi

wait_for_run() {
    local run_id=""
    for _ in {1..60}; do
        run_id=$(gh run list --repo "$repo" --workflow ci.yml --commit "$commit" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
        [[ -n "$run_id" ]] && { printf '%s' "$run_id"; return 0; }
        sleep 5
    done
    echo 'timed out waiting for the release workflow to appear' >&2
    return 1
}
wait_for_tag_run() {
    local run_id=""
    for _ in {1..60}; do
        run_id=$(gh run list --repo "$repo" --workflow ci.yml --commit "$commit" --limit 20 --json databaseId,headBranch --jq '.[] | select(.headBranch == "'"$tag"'") | .databaseId' | head -n 1)
        [[ -n "$run_id" ]] && { printf '%s' "$run_id"; return 0; }
        sleep 5
    done
    echo 'timed out waiting for the tagged release workflow to appear' >&2
    return 1
}

git push origin main
branch_run=$(wait_for_run)
gh run watch "$branch_run" --repo "$repo" --exit-status
git tag -a "$tag" -m "$tag" "$commit"
git push origin "$tag"
tag_run=$(wait_for_tag_run)
gh run watch "$tag_run" --repo "$repo" --exit-status
gh release view "$tag" --repo "$repo" --json tagName,isDraft,isPrerelease,assets,url
git ls-remote --tags origin "refs/tags/$tag" "refs/tags/$tag^{}"
