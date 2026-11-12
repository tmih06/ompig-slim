# Remote ompig-slim broker

The broker owns a **canonical SQLite credential vault and OAuth refresher**, like
oh-my-pi's auth broker. Clients receive access credentials, never real refresh
tokens, and still call providers directly. This is not an inference gateway:
prompts, responses, tools, sessions and workspaces do not pass through it.
Portable preferences synchronize separately with revision checks.

## Deploy

```sh
make build-broker
export OMPIG_BROKER_TOKEN="$(openssl rand -hex 32)" # save securely
./bin/ompig-slim-broker --data-dir=./broker-data
```

Default bind: `127.0.0.1:8765`. Use an HTTPS reverse proxy, private encrypted
network or SSH tunnel for remote access. TLS is operator-owned. One server process
owns the data directory; a filesystem lock refuses a second owner.

The directory is mode 0700 and contains `agent.db` (0600), SQLite WAL/SHM files,
portable preference files, and `server.lock`. Keep the volume across upgrades.
Use SQLite-aware backups or stop the service before copying the database.

### Back up or switch to standalone mode

Both modes use the **same `agent.db` file and schema**, not a broker-specific
credential database. Copying it preserves real access/refresh tokens, account
identities, selection, logout tombstones and quota blocks. No JSON export/import
or credential migration command is needed for a whole-database transfer.

1. Stop the broker gracefully and close every other process using that database.
   Do not leave another machine rotating the same accounts during the transfer.
2. Back up the destination profile, then copy `<broker-data-dir>/agent.db` to
   `<standalone-agent-dir>/agent.db` with mode 0600. Never overwrite an open
   destination database or leave its old `agent.db-wal`/`agent.db-shm` alongside
   the replacement; move the entire old database set into the backup first.
3. Clear the destination's saved broker connection with `ompig-slim broker clear`
   and unset `OMPIG_BROKER_URL`, `OMPIG_BROKER_TOKEN`, and
   `OMPIG_BROKER_TOKEN_FILE`. Start standalone mode on that agent directory.

To switch back, stop standalone mode and copy its closed `agent.db` into the
broker data directory instead. Starting the broker opens that exact database;
configure client connections again as needed. Transfer ownership rather than
running both copies: each copy contains the same potentially single-use OAuth
refresh tokens.

A clean last SQLite close checkpoints its WAL, making the main file transferable
on its own. For a live backup, use SQLite's backup API (for example
`sqlite3 /path/to/agent.db ".backup '/safe/backup/agent.db'"`). **Do not copy only
`agent.db` while it is open or after an unclean shutdown**: recent writes may
still be in `agent.db-wal`. Recover/close it first, use a SQLite-aware backup, or
preserve the stopped database and WAL together. Live backups are suitable for
recovery, not concurrent use of cloned OAuth credentials.

The database does not include preference documents or the broker's bearer token.
Copy the allowlisted settings/model/keybinding files separately for a full
portable profile, and configure the new broker's bearer separately if applicable.
Keep all backups private: unlike HTTP snapshots, the database contains real
refresh tokens. See `TestAgentDBTransfersBetweenBrokerAndStandalone` for the
round-trip portability regression test.

### Container

Releases publish `linux/amd64` and `linux/arm64` images:

```sh
docker volume create ompig-profile
export OMPIG_BROKER_TOKEN="$(openssl rand -hex 32)"
docker run -d --name ompig-broker \
  -p 127.0.0.1:8765:8765 -e OMPIG_BROKER_TOKEN \
  -v ompig-profile:/data \
  ghcr.io/tmih06/ompig-slim-broker:<version>
```

Build locally with `make build-broker-image`. The image runs as UID/GID 10001;
bind mounts must be writable by that UID. `/data` permissions are tightened at
startup. `/healthz` is public and backs the Docker health check. `--version`
reports the release tag. A missing bearer stops startup before creating files.

The broker needs outbound access to the OAuth providers it refreshes, including
any provider-specific host/environment configuration those flows require. It does
not need model catalogs, an inference runtime or an OAuth callback listener.
Custom OAuth refresh implementations must be registered on the broker too;
client-only extension code cannot execute on the server.

## Connect clients

Open `/login`, choose **Remote ompig-slim broker**, and enter the URL and shared
secret. Verification must succeed before saving. Restart to activate the new
connection; changing discovery while a runtime is running is not supported.

The device-local `<agent-dir>/remote-broker.json` is mode 0600 and never uploaded
or stored as a provider credential. Failed verification leaves an existing
connection untouched. Existing provider endpoints and model selections remain
unchanged.

For unattended provisioning:

```sh
ompig-slim broker setup --url https://broker.example.com \
  --token-file /run/secrets/broker --verify
ompig-slim broker show   # URL and secret presence, never the secret
ompig-slim broker clear  # disconnect on next startup; preferences remain
```

The secret resolves from `--token-file`, `--token`, `OMPIG_BROKER_TOKEN_FILE`,
`OMPIG_BROKER_TOKEN`, then stdin (`--token-stdin`). Prefer a mounted file or pipe
instead of exposing it in argv. `--verify` is optional for provisioning before
the server exists. A different saved destination requires `--force`; identical
setup is a no-op.

Alternatively set `OMPIG_BROKER_URL` and `OMPIG_BROKER_TOKEN` (or
`OMPIG_BROKER_TOKEN_FILE`). An explicit environment URL requires an environment
secret and never borrows one from the saved connection. A secret-only override
rotates the saved URL's secret. An unreadable secret file is an error. URLs must
be HTTP(S) origins without credentials, paths, query strings or fragments.

## Credentials and migration

A configured broker **replaces** the profile's local credential store. Client
reads fetch live redacted snapshots; they create no local credential database or
disk snapshot cache. Account selection, logout and quota blocks persist on the
broker and are visible to every client. Login still runs the browser/device flow
on the client, then uploads the result to the broker before returning. Subsequent
refreshes run only on the broker.

Local credentials are not uploaded implicitly. Transfer them explicitly:

```sh
ompig-slim broker migrate --from-local
```

This imports enabled local stored accounts, including their real refresh tokens.
Existing broker identities, including logout tombstones, are skipped: repeating
migration cannot roll back a rotation or revive logout. Local credentials remain
untouched for recovery, but broker-configured runtimes never read them. Stop old
local-only clients before transferring ownership; they could otherwise spend an
old refresh token. Remove obsolete local secrets/backups when recovery is no
longer needed. Environment-only keys are not migrated.

The broker has one current credential protocol: SQLite-backed vault APIs and
redacted snapshots. Local accounts are transferred explicitly with
`broker migrate --from-local`; there is no hosted credential-document format or
client-side credential sync protocol. Upgrade the broker and clients together.

## OAuth and failure semantics

The broker sweeps immediately and every minute, targeting OAuth credentials
expiring within five minutes. Sweeps do not overlap. Manual refresh and background
refresh share the existing SQLite per-account refresh leases and fenced writes.
Concurrent requests observing the same access token reuse the first rotation.
A refresh never resurrects an account logged out during exchange.

Once an exchange starts, the broker finishes and persists it despite client
cancellation or a disconnected response. Retrying with the old observed access
token returns the committed rotation instead of spending the refresh token again.
Shutdown drains HTTP handlers and waits for the background sweep before closing
SQLite. A process crash between provider rotation and SQLite commit can still
require signing in again; no transaction spans the external OAuth provider.

Transient failures retain credentials for retry. Existing provider-specific
permanent-failure handling remains in force (notably Codex JSON retirement);
unclassified errors are not guessed to mean logout. One failing account does not
prevent other accounts refreshing. HTTP errors never echo provider error bodies
or credential material.

A down/refusing broker leaves the CLI repair/version commands usable and reports
`profile sync pending (will retry): …`. Preference reconciliation keeps retrying
without a restart. Invalid bootstrap configuration reports `profile sync inactive: …`
and requires repair and restart. Neither case **falls back to local credentials**.
Credential access fails until connectivity/configuration is repaired. Runtime or
environment API-key overrides retain their existing explicit semantics; these are
not a fallback to stored local secrets.

## Preference synchronization

| State | Behavior |
| --- | --- |
| `settings.json`, `keybindings.json` | Bidirectional document sync |
| `models.json`, `models.yml`, `models.yaml` | Copied unchanged, including embedded keys and custom endpoints |
| Stored accounts, selection, quota blocks | Owned by broker SQLite, accessed via vault APIs |
| Refresh leases | Internal broker SQLite coordination, never client-owned |
| Sessions, project `.pi`, tools, workspaces, logs | Local, never uploaded |
| Skills, themes, extensions, MCP state, other files | Not synchronized |

Preferences reconcile at startup, every five seconds and on normal exit. First
connection adopts hosted files and privately backs up differing local files as
`.broker-sync-<host-hash>-<filename>.backup`; absent hosted files are seeded from
local copies. Later edits and deletion tombstones use per-document CAS.
Conflicting edits preserve both copies and report an error; reconcile to the
same desired content before retrying. A failed document does not stop unrelated
documents from synchronizing. Do not delete ancestry files to suppress conflicts.
Startup failures keep the five-second retry loop and normal-exit flush active;
a failed active-session preference flush can fail CLI exit. Until initial sync
succeeds, an unavailable broker does not fail otherwise usable CLI repair commands.

Synchronization updates files, not every live runtime setting. Reload the session
or restart the client to apply received settings, models and keybindings.
UI/settings/catalog reload behavior stays unchanged. Client paths/environment
references are not rewritten, so referenced resources must exist on each client.
Shared settings may run commands or load packages: trust the broker as fully as
your profile. Disconnecting does not export hosted credentials back to local
SQLite; only pre-existing local credentials remain after `broker clear`.

## API and security boundary

Every data route requires `Authorization: Bearer …`; only `GET /healthz` is public.
The server hashes the configured bearer in memory and compares in constant time.
Bearer authentication is not stored in SQLite. Rotate `OMPIG_BROKER_TOKEN` and
restart, then update clients.

| Route | Contract |
| --- | --- |
| `GET /v1/snapshot` | Live enabled accounts with access credentials; OAuth refresh replaced by `__OMPIG_REMOTE_REFRESH__` |
| `POST /v1/credential` | `operation`: `save`, `modify`, `import`, `select`, `delete-account`, `delete`, `blocked`, `block`, `clear-blocks` |
| `POST /v1/credential/refresh` | Exact provider/account and observed credential; host refreshes and returns redacted result |
| `GET/PUT /v1/items/{name}` | Allowlisted preferences; writes require `If-Match` |

Vault requests use `provider`, nullable `accountKey`, and operation-specific
`credential`, `expected`, `scope`, `scopes`, or `untilMs`. Generic `modify` uses
CAS against `expected`; conflicts return 412 without replaying callbacks.
`explicit` distinguishes an exact nil identity from the provider's selection.
Quota blocks use longest-deadline-wins. `import` only inserts missing identities.
The broker exposes no credential-document or client lease routes.

Trusted direct clients can read access tokens and API keys, not refresh tokens.
This is not a secret-isolating inference gateway or a multi-tenant authorization
system. The bearer still grants broad account mutation authority. Both the vault
and synchronized embedded keys are plaintext at rest: use private permissions,
encrypted volumes and transport encryption. Application-level/end-to-end
encryption and per-device grants are not implemented. Client requests are bounded
to 20 seconds, refuse redirects and cap response reads; vault uploads are capped
at 8 MiB. SIGINT/SIGTERM allows 30 seconds to drain HTTP work.
