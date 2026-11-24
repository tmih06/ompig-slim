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

`get_subagent_result` always returns immediately at root. A finished child
resolves inline with its exact result text, including an intentionally empty
one; it does not prepend status prose, inflate the text with the transcript,
or truncate it. A running child returns a pending receipt instead, and the
exact full result arrives on its own as a completion notification once the
child finishes. `wait` changes nothing at root and stays accepted for
compatibility. A second read while the request is outstanding reports the
request is already pending and does not schedule another delivery. Reading
the terminal result before that notification fires consumes it, so the parent
is never told twice.

In headless `--print` mode, the process waits for pending result deliveries
and their triggered turns before exiting. The tool still returns a pending
receipt immediately. The exact result arrives in a notification turn before
the session ends.

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

Result tool cards show four wrapped lines by default. Ctrl+O expands the full
result. Folding changes only the transcript display, not the stored result or
the text sent to the model.

A nested agent keeps the synchronous contract: `wait:true` still blocks for its
own child and `wait:false` observes without claiming, because the notification
route delivers to the root parent only.

A resumed child may outlive its own completion notice. A demanded delivery owns
a frozen snapshot of the generation that produced it, so the result a parent
requested still arrives exact even when the record has moved on.

## Notifications

A completion notification keeps its bounded preview. It adds `<outcome>` and,
for incomplete text, `<partial>true</partial>`. The truncation hint still appears
only when the preview is shorter than the result, so a short result never asks
for another retrieval.

A requested full result replaces the preview entirely instead of batching with
siblings. Its envelope adds `<requested>true</requested>`, and the frozen
`<result>` holds exactly the text a terminal read would return.

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

The footer reports `N subagents · M working` while eligible records exist.
Press Down on an empty prompt to open the tree below the status line. Each row
shows `@role name task usage`, with the configured role color. Up and Down move
the highlight. Enter opens the child's transcript session. Escape or Up on the
first row closes the tree. Alt+A opens the full Agent Hub.

Prompt history keeps its normal Up and Down behavior. After Down restores an
empty draft, another Down opens the tree. The Widget setting controls which
children appear. Completed children remain inspectable. Queued children do not
count as working.

The tree and the footer counts clean up finished children 30 seconds after
they complete; a timer re-renders at the deadline, so they leave without
input. The Agent Hub, the transcript, and `get_subagent_result` still reach
the finished child.

Provider-emitted thinking text streams beneath the child's row and in the
Agent Hub inspector. Opening the child transcript shows its thinking blocks
alongside its responses and tool calls. These views do not reveal reasoning
that the provider does not emit.

## Persistence

`Record` persists status, result, and error with the record, so a reload derives
the same outcome. Restoring a record that was still active at save time marks it
`canceled` with `Session ended before the agent completed`, which keeps a crashed
run from reading as success. A terminal failure stays a failure.
