# Remote ompig-slim broker: hosted profile storage

The broker is a **hosted place to save and synchronize your profile**, not an
inference gateway. All model calls, tools, OAuth login and OAuth refresh happen
on each client. Clients connect directly to their configured providers using
local copies of credentials and configuration. No prompts, responses, tool
results or inference traffic pass through this service.

## Deploy the storage service

```sh
make build-broker
export OMPIG_BROKER_TOKEN="$(openssl rand -hex 32)" # save securely
./bin/ompig-slim-broker --data-dir=./broker-data
```

The default bind is `127.0.0.1:8765`. For remote access, put it behind an HTTPS
reverse proxy or use a private encrypted network/SSH tunnel. There is no automatic
TLS or public ingress configuration. The service needs no provider credentials,
provider environment variables, CLI login, model catalog or OAuth callback ports.
It starts empty; the first connecting client seeds the stored profile.

### Container

```sh
docker build -f Dockerfile.broker -t ompig-slim-broker .
docker volume create ompig-profile
export OMPIG_BROKER_TOKEN="$(openssl rand -hex 32)"
docker run -d --name ompig-broker \
  -p 127.0.0.1:8765:8765 \
  -e OMPIG_BROKER_TOKEN \
  -v ompig-profile:/data \
  ompig-slim-broker
```

The image contains only the storage executable, runs as UID/GID 10001, and saves
profile documents under `/data`. Bind mounts must be writable by that UID.
Back up this volume and keep it across upgrades. Run only one server process per
data directory; a filesystem lock rejects accidental duplicate owners.

## Connect clients

On your already-configured machine, open `/login` and choose **Remote
ompig-slim broker** (or enter `/login remote-ompig-slim-broker` directly).
The dialog asks for two values:

1. **Broker URL**, for example `https://sync.example.com`.
2. **Broker secret**, the same token configured on the server, with masked input.

The connection is verified before it is saved. Restart ompig-slim to activate
synchronization; changing a connection does not replace a running session's sync
worker. Repeat this login on other machines to download the shared profile.
No environment variables are required after login.

The URL and secret are saved in `<agent-dir>/remote-broker.json`, mode 0600.
This file is device-local: it is never uploaded, added to the provider credential
store or included in shared settings. Failed verification or cancellation leaves
a previous saved connection untouched. The login entry is a storage setup flow,
not a synthetic inference provider; existing provider/model selections and direct
API endpoints remain unchanged.

Environment variables remain available for unattended clients:

```sh
export OMPIG_BROKER_URL=https://sync.example.com
export OMPIG_BROKER_TOKEN='<same secret as the server>'
ompig-slim
```

An explicit environment URL requires an environment token and overrides the saved
connection without borrowing its secret. A token-only override rotates the secret
for the saved URL. Continue using `/login` for provider accounts, configuring keys
and editing roles as usual: everything executes **on the client**.

`OMPIG_BROKER_URL` must be an origin (scheme and host, optionally a port), with no
path/query/user credentials. HTTP is supported for loopback, tunnels and private
networks; use HTTPS for ordinary remote connections. Store the bearer in a secret
manager, the local login file or a protected environment file rather than
committing it to shared settings.

## Synchronized state

| State | Behavior |
| --- | --- |
| `settings.json` | Bidirectional sync, including model roles, fallback chains and cycle order |
| `keybindings.json` | Bidirectional sync |
| `models.json`, `models.yml`, `models.yaml` | Copied unchanged: custom providers, endpoints, model definitions and embedded keys remain client-side |
| Stored credentials | Logical multi-account snapshots, including OAuth access/refresh tokens, selected accounts and logout tombstones |
| Project `.pi` configuration | Local, not uploaded |
| Sessions, prompts, logs, tools, workspaces | Local, never uploaded |
| Skills, themes, extensions, MCP state, other files | Not synchronized in this version |
| Environment-only API keys | Not exported; store a key through login or configure the environment on each machine |
| Quota blocks, refresh leases, general SQLite cache | Local, not copied between databases |

The existing local agent directory is used; there is no replacement profile and
no raw `agent.db` transfer. Preferences reconcile at startup, every five seconds,
and on normal CLI exit. New files are written atomically with mode 0600.
Running UI/settings/catalog reload follows the existing runtime behavior; restart
a session if a changed preference or provider definition has not been reloaded.

At first connection, an existing hosted file wins and differing local files are
backed up as `.broker-sync-<host-hash>-<filename>.backup` in your agent directory.
Existing local credentials are likewise backed up before first remote adoption.
An absent hosted file is seeded from the local copy. After that, edits and
deletions synchronize in both directions using per-document revision checks.
Changes to different files can merge independently; changes to the same file are
**not silently merged or overwritten**. A conflict reports an error and retains
both copies. Reconcile the local and hosted documents to the desired same content
before retrying. Do not delete ancestry files just to suppress a conflict.

Client paths and environment references are not rewritten. Resources referenced
by settings must exist on each machine. Shared settings may execute commands or
load packages, so trust this storage account as fully as your local profile.

## OAuth rotation and failure semantics

Login and refresh run on the client, using the normal provider implementations.
Credential mutations take a short hosted storage lease, pull the latest account
snapshot, execute the local mutation once, then publish it before returning.
Other machines can continue inference; only concurrent credential mutations
wait. The server holds no provider refresh logic and sees no model requests.

A lease lasts 60 seconds and is renewed every ten seconds. Acquisition waits at
most 30 seconds. Commits require both the lease owner and the expected document
revision. Restarting the service revokes leases. Client ancestry records preserve
locally committed rotations across a failed upload or crash: on reconnect they
are uploaded if the hosted revision has not advanced, rather than overwritten
or refreshed again. If both copies advanced, synchronization stops with a
credential conflict and preserves the local copy for explicit reconciliation.

This is not a distributed transaction with an OAuth provider. A process crash
between the provider returning a rotated token and its local SQLite commit, or
a network partition/process pause that outlives a lease, can still require
signing in again. The client never automatically replays a refresh callback to
resolve a storage conflict. Back up/reconcile pending credentials before
repeating a login or deleting synchronization ancestry.

A configured but unavailable/refusing service never blocks the CLI: startup
prints `profile sync inactive: …` and continues local-only, so `--version`,
`/login` and the repair path stay usable. While the service is unreachable,
credential writes stay local and unsynchronized; the first reconnect reports a
divergence as a conflict rather than overwriting either side. This version has no
offline queue. A failed preference upload during an active session is reported
and fails an otherwise successful CLI exit. Local files remain intact.

To disconnect, remove the local `remote-broker.json` connection file, unset
`OMPIG_BROKER_URL`, then restart; the downloaded profile and credentials remain
local.

## Security and operations

- Every data route requires `Authorization: Bearer …`; only `GET /healthz` is
  public. The bearer grants full read/write access to this single user's profile,
  **including refresh tokens and embedded provider API keys**.
- This is trusted-machine credential synchronization, not a secret-isolating
  gateway. Both hosted and local copies are plaintext at rest, protected by OS
  permissions. Use encrypted disks/volumes and TLS; application-level/end-to-end
  encryption, tenants and per-device authorization are not implemented.
- Rotate the service environment token and restart to revoke it; update clients.
  There is no web UI, login flow, remote execution API or inference route.
- Documents are allowlisted and capped at 8 MiB; uploads/downloads carry base64
  document bytes. Client requests time out after 20 seconds and refuse redirects.
- SIGINT/SIGTERM allows 30 seconds for HTTP shutdown. Credential uploads and
  configuration replacements are atomic; keep server and clients on matching
  builds because snapshot schema compatibility is not version-negotiated yet.

Storage routes are `GET/PUT /v1/items/{name}` with `If-Match` revision checks,
and `POST/PUT/DELETE /v1/auth/lease` for credential mutation coordination.
`credentials.json` holds logical account records, never a live SQLite file.
