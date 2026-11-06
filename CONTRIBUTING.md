# Contributing to ompig-slim

This is a personal coding harness built on PiG, not an official PiG, Pi, or oh-my-pi release. Keep the native core small: prefer an existing extension or skill unless a capability requires a runtime change. Do not reformat or modernize inherited code without a behavioral reason.

## Review

Understand the code you submit and its interactions with the rest of the system. Agent-written code is acceptable; unread output is not. Label model-generated issue and review text. Discuss changes with the maintainer before submitting a pull request.

This local repository does not configure a hosted contributor-approval gate. The original fork's remote approval list and workflows are not implicitly installed by this migration.

## Reports

Keep reports concise. Include the commit or version, operating system, command, minimal reproduction, and relevant output. Reproduce with `--no-extensions` before attributing a failure to core behavior. Follow SECURITY.md for security-sensitive reports; do not post credentials.

## Development

```sh
make check
make build
```

The checks include the patched PiG library, not only the executable entry point. Go's build, vet, and test caches use package inputs and transitive dependencies, so unchanged packages reuse successful results without skipping unknown shared changes.

Dependencies are pinned in `go.mod`, verified by `go.sum`, and checked in under `vendor/`. The PiG sources under `vendor/github.com/MichaelKinsy/PiG` contain the personal feature ports. A blind `go mod vendor` overwrites those ports; preserve and reapply them deliberately when upgrading the library. Keep generated upstream files unchanged and use dedicated native port modules where possible.

## Change rules

- One topic per change; keep the diff to its stated behavior.
- Bug fixes need a meaningful regression test covering the failing boundary.
- Use isolated temporary directories, fixture credentials, and local HTTP servers; tests must not need real credentials or paid model calls.
- Keep dependency and lock changes separate from unrelated behavior changes. Pin direct dependencies exactly and preserve license notices.
- Never remove another session's changes. Stage explicit paths and do not force-push.
- Use commit subjects such as `feat(auth): ...`, `fix(tui): ...`, or `docs: ...`.

The README describes the current feature set and supported native builds. Source behavior for the personal ports comes from tmih06's omp-slim fork; PiG remains the reference for inherited features.
