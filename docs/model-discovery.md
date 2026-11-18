# Model discovery

Model availability follows live sources rather than a release-time model ID list.
A new model does not require a binary update when it uses a supported provider
protocol and the source publishes its capabilities.

## Sources and ownership

The public catalog at `https://catalog.stencil.so/models.json.zstd` supplies
new model IDs and metadata for supported public providers. Rows must declare
usable chat capabilities before they become selectable. The catalog cannot
change endpoints, credentials, headers, or transport implementations.

Codex and Codex JSON use the authenticated Codex model endpoint. That roster
decides which models the signed-in accounts can select. The public catalog can
supply missing metadata, but it cannot authorize subscription models.
The two providers retain separate credential and catalog namespaces.

The ClinePass catalog omits `cline-free/` promotions. [Cline reserves those
models for its IDE extension and CLI](https://docs.cline.bot/getting-started/free-models#api-access).

A successful native roster replaces the previous roster. Models removed by the
provider stop appearing. An empty successful response is an empty roster, not
a request to restore bundled models. Explicit custom model definitions still
apply after discovery. Their settings and overrides take precedence.

The existing model runtime owns refresh, cancellation, cache publication, and
availability updates. The CLI, interactive picker, roles, and subagents read
that same composed model data. Getters and terminal rendering do not fetch.

## Refresh and offline behavior

Online `ompig-slim --list-models` waits for a bounded, best-effort catalog refresh
before printing. It uses the last good cache if discovery fails.

Interactive startup loads cached models first and refreshes in the background.
Open the model picker to select a newly discovered model. Long-running sessions
also refresh periodically. The model manager's refresh action and
`ompig-slim update --models` request another refresh.

`--offline`, `PI_OFFLINE`, and `PIG_OFFLINE=1` prevent catalog network requests.
An offline restart restores the last successful matching account roster.
A profile without a matching successful snapshot uses the bundled fallback.
Discovery failures, malformed responses, and cancelled work do not erase a
valid cached roster.

Account-scoped cache identity includes the provider and account scope. Switching
accounts must not restore another account's model list. Catalog snapshots contain
model metadata and opaque scope identifiers, not credentials or account emails.
Credential refresh stays with the existing credential owner, including the
broker when configured.

## Limits

"Current" means the latest successful provider or public-catalog observation.
A network failure, provider rollout, missing account access, or a lagging public
source can delay a model's appearance. Discovery cannot add an unsupported wire
protocol.

Codex's model response does not publish every capability. Unknown output-token
limits do not come from a different model's settings. Matching model metadata or
an explicit [custom model definition](custom-models.md) can supply missing values.
Discovery success proves that a model is listed, not that every request will
succeed or that the account has remaining quota.

## Verification

`python3 scripts/smoke-test-model-discovery.py ./bin/ompig-slim` drives a loopback
provider with an isolated profile. It introduces unknown model IDs, changes the
roster, fails refreshes, restarts offline, changes accounts, selects a discovered
model through the terminal picker, and sends a real request to the fixture.
`make smoke` includes this check. No real credentials or paid inference are needed.
