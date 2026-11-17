# Account routing

Every provider with stored accounts distributes new session assignments across
eligible, unblocked accounts, including OAuth logins and API-key accounts.
Assignments are sticky per provider, session ID, and model ID. Different models
in one session can use different accounts.

## Initial assignment

The router applies model-plan eligibility before selecting an account. Eligible
free Codex accounts have priority over other eligible accounts. Accounts with an
unknown plan follow known-eligible accounts. Within each priority group, new
assignments use round-robin order. The first assignment uses the selected account
when that account belongs to the highest available priority group.

A shared cursor per provider and priority group coordinates assignments across
models and processes. Reusing an assignment does not advance the cursor. Account
previews, usage displays, and credential exports do not reserve an assignment or
advance the cursor. A request without a session ID uses the existing selection
policy. Explicit API-key overrides bypass the account pool.

Round-robin distributes assignments, not tokens or requests. One busy session
can consume more quota than several quiet sessions. A single session does not
proactively use every account or start every account's provider-side quota window.

## Codex plan claims and usage

Plan checks and priority use the current access-token plan first, falling back
to the stored plan (`orgName`) when the token omits it. Browser logins and JSON
imports retain the latest access/ID-token plan across refreshes without keeping
ID tokens or the source import JSON. This preserves Free priority for models
such as GPT-6 Luna even when Plus is selected or has a durable assignment.
Unknown plans are not assumed to be Free, and paid-only models still exclude
known Free accounts. If an older login/import discarded its only plan claim,
sign in or import it again. Browser Codex and Codex JSON keep separate pools.

Usage views report the account accepted for the session's model. Before any
route is confirmed, the usage hub can still show the provider's selected
account; it is not a prediction of the next request's account.

## Stickiness and fallback

Assignments and cursors persist in namespaced records in the existing SQLite
`cache` table. Session and model identities are hashed in cache keys. No new
credential schema or profile path is required. Assignments survive process
restarts and session resumption. The records use the same ten-year expiry as the
store's selection records.

The credential owner reserves an assignment before transport starts. Concurrent
requests for the same session and model reuse that reservation, even before the
first request completes. A canceled or failed request can therefore leave an
assignment without a successful response.

An account that becomes blocked, disabled, or ineligible loses priority to an
available account. Recognized quota, policy, and authentication rejections can
trigger fallback before any response content escapes. The router persists the
fallback assignment before starting its transport. It never replays a partial
response on another account. An available free account still outranks a paid pin.

An explicit account selection or login resets the provider's assignments and
cursors. Automatic routing never changes the provider's selected account.

## Broker ownership

Broker clients reserve assignments on the host. SQLite transactions coordinate
all clients, and broker restarts preserve assignments. Clients send provider,
session ID, and model ID for routing. They do not upload prompts, responses, or
session contents. Routing creates no local credential mirror and never falls back
to local credentials when the broker is unavailable.

Upgrade clients and the broker together. Older brokers do not implement the
session-assignment operations. Custom credential stores without
`AccountSessionStore` retain the previous in-memory affinity and selection policy.

## Verification

Run `make test` for account routing and broker regressions. Run `make smoke` for
the built executable. Its account-routing scenario uses temporary profiles and
loopback providers to verify distribution, resumption, and durable quota fallback
for Codex JSON, Mistral OAuth, and a custom API-key provider.

### Assignment lookup allocations

`BenchmarkSessionRouting` measures a complete sticky request through a real HTTP
broker and SQLite vault. It excludes setup and warmup, uses no artificial network
delay, and asserts that every request reaches the attempt with the expected
account. It reports allocations and actual HTTP exchanges as well as elapsed time.

```sh
go test -mod=vendor ./internal/broker -run '^$' \
	-bench '^BenchmarkSessionRouting$' -benchtime=100x -count=5
```

On Linux amd64 with an Intel i5-12500H, five alternating before/after runs compared
full account enumeration with an exact-identity lookup during assignment
persistence. Each run completed 100 requests per account-count scenario.

| Stored accounts | Allocations/request before, median and range | After, median and range |
| --- | --- | --- |
| 2 | 1049, 1047 to 1050 | 1040, 1039 to 1041 |
| 64 | 4828, 4824 to 4832 | 4135, 4132 to 4136 |

Both versions made four HTTP exchanges per request and passed all assertions.
The exact lookup avoids materializing unrelated credential rows. It preserves
live block checks and transactional assignment persistence. This is an allocation
improvement, not a demonstrated latency improvement. Timing ranges overlapped on
the shared host. A separate CPU profile showed remaining syscall, SQLite, and
runtime costs. No provider inference or real WAN latency was measured.
