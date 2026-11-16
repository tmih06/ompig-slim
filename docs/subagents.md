# Child result contract

A parent reads a child's result through a short completion notification and
through `get_subagent_result`, which returns the result text plus a machine
readable outcome in the tool details. The outcome is what tells an empty result
that means success from an empty result that means the child never answered.

## Outcomes

`Record.Status` is the persisted field. `Record.Outcome()` derives the outcome
from it, so no caller matches status strings of its own.

| Outcome | Source status | Meaning |
|---|---|---|
| `running` | `queued`, `running`, `scheduled` | Not finished; poll or wait |
| `completed` | `completed`, `steered` | Finished a normal turn |
| `failed` | `error` | Stopped on an error |
| `canceled` | `stopped`, `aborted` | A person or the parent stopped it |
| `exhausted` | `exhausted` | The hard turn budget stopped it |

An unknown status reports `running`, so a record from a newer or hand-edited
file stays pollable instead of reading as a success nobody observed.

`max_turns` is a soft limit. Reaching it steers the child to wrap up and adds
`graceTurns` turns. A child that finishes inside that window reports
`completed`. Only the hard budget, after the grace turns, reports `exhausted`,
and it is distinct from a cancellation so a caller can tell a truncated answer
from a stopped one.

Every reader decides success from the outcome, never from the raw status. A
`steered` child completed, so its text is real: a workflow journals its value,
and a caller that matched the status string alone would discard a finished
answer as an empty success.

Cancelling the parent's own wait is the parent's cancellation. It does not
change the child, does not report an outcome, and does not resubmit work. A
foreground `Agent` call that is cancelled hands off to the background instead.

## Retrieval

`get_subagent_result` returns the exact result text for a `completed` child,
including an intentionally empty one. It does not prepend status prose, inflate
the text with the transcript, or truncate it.

For `failed`, `canceled`, and `exhausted`, the text states the outcome first and
then the partial output, so a bounded read cannot mistake it for a finished
answer:

```text
subagent_outcome: exhausted
partial_output: incomplete, the agent did not finish
Two of the five files are audited.
```

A child with no text reports `partial_output: none`. Partial text is never
invented.

The tool details carry the same facts for a caller that reads structure:

```json
{ "agentId": "agent_ab12", "outcome": "exhausted", "partial": true }
```

`error` holds the failure message when it differs from the result text, which
keeps the existing duplicate-error suppression.

An incomplete child returns an error result, on both the `Agent` launch path and
retrieval, so a `failed`, `canceled`, or `exhausted` child reads as a failure in
the tool result itself and not only in its text.

## Notifications

A completion notification keeps its bounded preview. It adds `<outcome>` and,
for incomplete text, `<partial>true</partial>`. The truncation hint still appears
only when the preview is shorter than the result, so a short result never asks
for another retrieval.

```xml
<task-notification>
<task-id>agent_ab12</task-id>
<status>exhausted</status>
<outcome>exhausted</outcome>
<summary>Audit the parser</summary>
<result>Two of the five files are audited.</result>
<partial>true</partial>
</task-notification>
```

## Live inspection

The main view shows each active child as `@role name task usage`. The name
uses the child's alias or handle instead of the generic `Agent` label. Role
labels use the same configured color as the model hub.

Provider-emitted thinking text streams beneath the child's row and in the
Agent Hub inspector. Opening the child transcript shows its thinking blocks
alongside its responses and tool calls. These views do not reveal reasoning
that the provider does not emit.

## Persistence

`Record` persists status, result, and error with the record, so a reload derives
the same outcome. Restoring a record that was still active at save time marks it
`canceled` with `Session ended before the agent completed`, which keeps a crashed
run from reading as success. A terminal failure stays a failure.
