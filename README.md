# ompig-slim

<p align="center">
  <img src="assets/ompig-banner.svg" alt="OMPi in GO-slim startup logo" width="800">
</p>

[![CI](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml/badge.svg)](https://github.com/tmih06/ompig-slim/actions/workflows/ci.yml)

A native Go coding assistant built on [PiG](https://github.com/MichaelKinsy/PiG), with personal `omp-slim` features integrated into the library runtime. It provides PiG's interactive terminal and CLI with shared Pi/OMP configuration and added account routing, provider support, built-in web tools, codemode, and subagents.

This is an independent personal project, not an official PiG, Pi, or oh-my-pi release. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for license and attribution details.

## Get started

Requires Go 1.27 or newer. The repository pins Go 1.27.3 and vendors its dependencies.

```sh
git clone https://github.com/tmih06/ompig-slim.git
cd ompig-slim
make build
./bin/ompig-slim
```

To install in `~/.local/bin`:

```sh
make install
```

Run `ompig-slim --help` for CLI options. Sign in with `/login` in the interactive app or `ompig-slim login` in the terminal. Provider API-key environment variables are also supported.

## What it includes

- PiG's native Go runtime and full CLI, including interactive, print, JSON, and RPC modes.
- Shared Pi configuration under `~/.pi/agent` and project settings under `.pi`.
- Multiple provider accounts with account selection, quota-aware routing, and credential migration.
- Built-in `web_search`, `web_fetch`, and persistent codemode tools.
- Optional native subagents, model metadata, usage views, and terminal model selectors.
- Native Linux and macOS releases for x64 and arm64.

Codemode and other agent tools run with your user permissions; codemode is not an OS security sandbox. Account credentials are stored locally in SQLite and are not encrypted at rest.

## Provider login and Codex JSON

For browser or device-code sign-in, choose **OpenAI Codex** in `/login` (or run
`/login openai-codex`) and select the login method. Both flows use the Codex
public client, with the native `codex_cli_rs` originator shared by browser
sign-in and API requests. Free-tier accounts without a workspace ID can sign
in using their token's email identity; model eligibility and quotas still depend
on the account plan and OpenAI's backend. Existing accounts should sign in again
to obtain the updated login scopes.

`/login` opens one fuzzy-searchable list of account and API-key login options.
Choose **Codex JSON** (or run `/login codex-json`) to paste an exported JSON
credential object. Select a model under `codex-json` with `/model` afterward.

The importer keeps only `accessToken`, optional `refreshToken`, account ID from
the token or `providerSpecificData.chatgptAccountId`, and expiry. It accepts
`expiresAt` (RFC3339), `expiresIn` (seconds), or the token's JWT expiry; JWT expiry
caps the imported lifetime. Other fields, including `idToken`, are discarded.
Expired imports are rejected. Terminal CLI imports are also available through
`ompig-slim login codex-json`; use a single-line JSON string there.

Multiple imports participate in the existing quota-aware account fallback.
Refreshable tokens rotate normally. When a request or refresh discovers that an
imported account can no longer refresh, its secrets and routing blocks are
removed from live database state, retaining only an OMP-compatible disabled row.
Timeouts, rate limits, server errors, and unclassified failures do not delete
credentials. Cleanup occurs when credentials are used, not on a background timer;
ordinary Codex browser-login accounts are unaffected.

## Saved rate-limit resets

Use `/usage reset` to spend a banked Codex or Claude rate-limit reset. The
picker shows only accounts with usable, unexpired resets and requires a second
confirmation. Empty, ineligible, expired, and failed accounts are hidden.
Use `/usage reset codex/active`, `/usage reset claude/active`, or
`/usage reset <provider>/<account-key>` to choose an account directly.

Codex selects the soonest-expiring available credit; Claude rechecks and spends
the grant its endpoint selects. Successful redemption invalidates the account's
quota cache and clears matching quota blocks, preserving unrelated auth and
policy blocks. A Claude 5h-only reset leaves weekly blocks intact.

Redemption consumes a provider credit irreversibly. If the outcome is uncertain,
check `/usage` before trying again; mutation requests are not automatically
retried. SDK `Session.DispatchSlash("/usage reset")` returns an inventory listing
without a picker; print-mode prompts do not dispatch built-in slash commands.

## Development

```sh
make check    # tests and vet, including the patched PiG library
make race     # race-enabled checks
make smoke    # compiled codemode, subagent, and web-tool scenarios
make release  # build supported platform archives
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before changing the code or vendored runtime. In particular, the vendored PiG library contains project-specific patches; a routine `go mod vendor` would replace them.

For technical architecture, project conventions, and maintenance guidance, see [CONTEXT.md](CONTEXT.md).
