---
name: release
description: "Prepare and publish a versioned release. Use when changing a release version, creating or pushing a stable tag, publishing release artifacts, or verifying post-tag CI and release state."
---

# Release

Treat a release as one immutable commit, not as a tag added after the fact. The release
commit owns the user-facing version, source-build version, tests, and documentation; the
tag names that exact commit; CI publishes from that tag.

## 1. Inspect

- Read `AGENTS.md`, `CONTEXT.md` release guidance, the release workflow, and the build or
  installer scripts that consume versions.
- Confirm the current branch, clean working tree, upstream tracking branch, latest stable
  tag, latest beta tag, and the requested version.
- Use the repository's tag convention. In this repository stable tags are annotated
  `vX.Y.Z`; the user's `X.Y.Z` means tag `vX.Y.Z`.
- Check that the requested tag does not already exist locally or remotely. If it exists,
  stop and ask before replacing or force-moving it.

Done: the version source, release consumers, tag convention, required checks, and target
commit are identified, and no existing tag would be overwritten.

## 2. Inventory version references

Search tracked files for the previous and requested versions. Classify every match:

- **Release-facing:** current README badge and install examples, source-build or
  `--version` fallback, package metadata, manifests, and release notes. Update these in
  the release commit.
- **Historical:** benchmark evidence, comparison tables, migration examples, and fixtures
  intentionally describing an older build. Keep these unchanged and record why.
- **Test fixture:** update only when the test is asserting the current release sequence;
  preserve fixtures whose purpose is testing version arithmetic or an old input.

Do not infer completeness from a single version constant. Re-run the search after editing
and review every remaining old-version match.

Done: every old-version occurrence is classified, every release-facing occurrence is
updated, and the remaining old-version matches are intentional and documented by context.

## 3. Prepare and verify

- Make the smallest release-only edits. Do not mix unrelated fixes into the release
  commit.
- Run the repository's required release checks, at minimum the full check/test command
  documented by the repository and the release/build verification that produces the
  artifacts. Run the focused version tests too.
- Build the release binary or archives with the requested version and verify `--version`
  and any embedded startup/self-update version report it.
- Review `git diff`, `git diff --check`, and the staged diff. Stage explicit paths.

Done: required checks pass, artifacts report the requested version, and the staged diff
contains only the release metadata and intended release documentation.

## 4. Commit before tagging

Create the required Conventional Commit with a body that explains the release metadata
and why historical references were left unchanged. Then verify:

```bash
git status --short
git show --stat --oneline HEAD
git show HEAD:<version-source>
```

The worktree must be clean. A tag must never point at a commit whose release-facing
version references have not been reviewed.

Done: the release commit exists, its staged diff was reviewed, and the worktree is clean.

## 5. Publish in order

Push the release commit to the intended branch first. Wait for the required branch CI to
finish successfully when the repository's workflow publishes or validates releases from
that branch. Then create and push the annotated stable tag at the verified commit:

```bash
git push origin <branch>
git tag -a "vX.Y.Z" -m "vX.Y.Z" HEAD
git push origin "vX.Y.Z"
```

Never force-push a release tag as a routine correction. If the tag is wrong, stop, report
the existing tag and the intended commit, and ask whether to create a corrective release
or explicitly authorize tag replacement.

Done: the remote branch and tag both reference the intended release commit, and the tag is
annotated and newly created rather than overwritten.

## 6. Verify delivery

- Query the remote tag and confirm its peeled commit equals the release commit:
  `git ls-remote --tags origin "refs/tags/vX.Y.Z"`.
- Inspect the release workflow runs for the tag and branch commit with `gh run list` or
  `gh run view`. Read failed logs instead of treating an in-progress run as success.
- Verify the GitHub release and artifact names/checksums when the workflow publishes them.
- If release CI fails, identify the exact job and reproduce it locally before changing
  code. A failed tag run is not a successful release.

Done: the remote tag, release commit, CI conclusion, and published artifacts are all
verified, or the remaining blocker is named with its failing job and log evidence.

## Report

Return:

- release version and commit;
- files or references updated, plus intentional historical references left alone;
- checks and artifact verification;
- remote tag and release URL;
- any CI or publication blocker.
