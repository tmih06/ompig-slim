# Provider error recovery

Provider recovery applies to every model provider. Native adapters preserve HTTP
status, provider error codes, and known retry deadlines so the session can decide
whether to retry without parsing the displayed error text. Custom
OpenAI-compatible endpoints use the same policy.

## Failure categories

| Failure | Recovery |
| --- | --- |
| Network interruption, timeout, HTTP 408, HTTP 409, or server failure | Retry with bounded exponential backoff. |
| Rate limit or concurrency limit | Honor a valid provider retry deadline. Without one, use backoff. |
| Quota or subscription usage limit with a known reset | Try eligible stored accounts or configured model fallbacks. If none is available, wait for the earliest usable account reset and retry. |
| Quota with no known reset | Stop instead of polling for quota. |
| Invalid credentials, permission denial, or account policy rejection | Use an eligible alternate account or configured fallback when available. Do not repeatedly send the same rejected request. |
| Billing failure, invalid request, or unsupported configuration | Stop unless an existing account or model fallback can resolve the failure. |
| Context overflow | Use the existing context compaction recovery. |
| User cancellation | Cancel the request or wait. Do not resume automatically. |

A provider can omit reset information. A local cooldown is not evidence of a
provider quota reset. The runtime does not promise an exact reset time when the
provider supplies none. Persisted account blocks without reset evidence are
still honored, but do not create an automatic quota-reset wait after a process
restart.

## Waiting status

The interactive editor shows the wait reason and remaining time in the same
status area as the working indicator, above the prompt. Escape cancels a wait.
When the deadline arrives, the session continues the existing task without a
new user prompt. Completed tool results remain in the conversation. Recovery
is not an exactly-once guarantee for external effects.

The session does not send inference requests while it waits. A reset deadline
permits another attempt. It does not guarantee that the provider has restored
quota or accepted the account.

## Retry settings

The existing `retry` settings control recovery.

| Setting | Purpose |
| --- | --- |
| `enabled` | Enable session retries. |
| `maxRetries` | Bound automatic attempts for a failing response. Zero disables retries. |
| `baseDelayMs` | Initial exponential backoff. |
| `maxAgentDelayMs` | Ceiling for exponential backoff, not a license to shorten provider retry deadlines. |
| `maxDelayMs` | Maximum ordinary session wait. Zero disables the ceiling. |
| `waitForUsageReset` | Allow known quota reset waits beyond `maxDelayMs`. Enabled by default. |
| `provider.maxRetries` | Independent transport retry budget. The default is zero. |

Explicit retry deadlines are minimum waits. A provider error without a reset
does not gain a reset from `waitForUsageReset`. Repeated failures still consume
the retry budget.

Account selection and eligibility follow [account routing](account-routing.md).
Configured fallback chains follow [model roles](model-roles.md).
The opt-in [durable host](durable.md) retains its own persisted retry budget and
terminal failure contract. Generation and compaction retries honor the same
provider deadlines.

## Verification

`python3 scripts/smoke-test-provider-recovery.py ./bin/ompig-slim` runs an
isolated loopback provider against the real binary. It checks quota deadlines,
permanent errors, retry limits, the TUI waiting status, automatic continuation,
and Escape cancellation. It uses temporary profiles and fixture credentials.
The scenario also runs in `make smoke`.
