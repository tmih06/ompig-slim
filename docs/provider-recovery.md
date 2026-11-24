# Provider error recovery

Native and custom providers share recovery categories for transient failures,
rate limits, quota limits, authentication, permissions, billing, and invalid
requests. Recovery facts use the existing assistant-message diagnostics.
They contain status, error code, category, and a known retry deadline.
Response headers and bodies are not copied into those diagnostics.

Session, summarization, and durable retries honor provider deadlines instead
of shortening them to an exponential-backoff ceiling. Authentication, billing,
invalid requests, and quota failures without a known reset remain terminal
unless an existing account or model fallback resolves them. Automatic retries
remain bounded by the configured retry budget. Escape cancels a session wait.

The existing `retry.enabled`, `retry.maxRetries`, `retry.baseDelayMs`,
`retry.maxAgentDelayMs`, and `retry.maxDelayMs` settings remain available.
`retry.waitForUsageReset` now defaults to true. It permits a known quota reset
wait beyond the ordinary maximum wait, without inventing a missing reset time.
Explicit false values and disabled retry budgets remain honored.

Transport retries do not repeatedly send known quota failures. Bedrock timed
rejections reach the session wait rather than a hidden SDK retry. The opt-in
[durable host](durable.md) retains its persisted retry budget and terminal
failure contract.
