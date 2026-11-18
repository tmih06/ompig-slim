# Account routing

Providers with stored accounts assign sessions to eligible, unblocked
accounts, including OAuth logins and API-key accounts.
Assignments are sticky per provider, session ID, and model ID. Different models
in one session can use different accounts.

## Initial assignment

The router applies model-plan eligibility before selecting an account. Eligible
free Codex accounts have priority over other eligible accounts. Accounts with an
unknown plan follow known-eligible accounts when the model requires a plan.
Within the highest-priority group, a new Codex session picks the account with
the most quota left in its shortest model-relevant rolling window. An account
with 100% left in its 5-hour window and 30% left weekly scores 100%. Exhaustion
in either window still disqualifies the account. The window length, not the
time until reset, determines which window to compare. If lengths cannot be
compared, the primary window takes precedence. A healthy session keeps its
assignment even when another account has more left.

Ties and assignments without fresh quota measurements use round-robin. The
first assignment in that rotation uses the selected account if it is among
the tied candidates.

A shared cursor per provider and priority group coordinates assignments across
models and processes. Reusing an assignment does not advance the cursor. Account
previews, usage displays, and credential exports do not reserve an assignment or
advance the cursor. A request without a session ID uses the existing selection
policy. Explicit API-key overrides bypass the account pool.

Round-robin distributes assignments, not tokens or requests. One busy session
can consume more quota than several quiet sessions. A single session does not
proactively use every account or start every account's provider-side quota window.

## Codex plan claims and usage

Online Codex routing probes account usage before reserving an assignment.
Fresh, recognized usage plans take priority over access-token claims, including
when the token omits its plan or still names an older plan. Without fresh usage,
plan checks use the access-token plan, then the stored plan (`orgName`). Browser
logins and JSON imports retain the latest access/ID-token plan across refreshes without keeping
ID tokens or the source import JSON. This preserves Free priority for models
such as GPT-6 Luna even when Plus is selected or has a durable assignment.
Unknown plans are not assumed to be Free, and paid-only models still exclude
known Free accounts. Offline mode does not fetch usage. Failed or stale probes
fall back to credential metadata rather than guessing a plan. Probes use the
existing ten-second usage request budget and a five-minute freshness limit.
Routing probes do not spend saved quota resets. A confirmed saved reset
invalidates the routing observation so the next request can use the new quota.
An older login/import with no retained plan can recover Free priority from a successful
usage probe. Browser Codex and Codex JSON keep separate pools.

The detailed `/usage` view shows the session account. Before the first prompt,
it previews the account for the current model without reserving it. If the
preview fails, the view does not label the provider's selected account as the
session account.

## Stickiness and fallback

Assignments and cursors persist in namespaced records in the existing SQLite
`cache` table. Session and model identities are hashed in cache keys. Usage
observations do not change those identities. No new credential schema or profile
path is required. Assignments survive process restarts and session resumption. The records use the same ten-year expiry as the
store's selection records.

The credential owner reserves an assignment before transport starts. Concurrent
requests for the same session and model reuse that reservation, even before the
first request completes. A canceled or failed request can therefore leave an
assignment without a successful response.

An account that becomes blocked, disabled, ineligible, or exhausted according
to fresh model-relevant usage loses priority to an available account. Expired
window observations do not prove that a new window is exhausted. Recognized
quota, policy, and authentication rejections can trigger fallback before any
response content escapes. The router persists the
fallback assignment before starting its transport. It never replays a partial
response on another account. An available free account still outranks a paid pin.

An explicit account selection or login resets the provider's assignments and
cursors. Automatic routing never changes the provider's selected account.

## Broker ownership

Broker clients reserve assignments on the host. SQLite transactions coordinate
all clients, and broker restarts preserve assignments. Clients send provider,
session ID, model ID, and narrow account quota observations for routing. The
observations contain plan and window measurements, not raw usage responses,
credentials, prompts, or session contents. Routing creates no local credential
mirror and never falls back to local credentials when the broker is unavailable.

Upgrade clients and the broker together for usage-aware routing. The v0.2.1
broker supports session assignments but rejects requests that include usage
observations. Custom credential stores without
`AccountSessionStore` retain the previous in-memory affinity and selection policy.

## Verification

Run `make test` for account routing and broker regressions. Run `make smoke` for
the built executable. Its account-routing scenario uses temporary profiles and
loopback providers to verify distribution, resumption, and durable quota fallback
for Codex JSON, Mistral OAuth, and a custom API-key provider. The Codex policy
scenario drives the TUI through a local TLS usage fixture. It checks plan-less
Free accounts, remaining-quota priority, resumption, paid-only model
eligibility, and usage plans that differ from JWT claims. Run
`make test-broker-codex-usage` to check the account label for both Codex
providers in a real PTY against a local broker with synthetic credentials.

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
