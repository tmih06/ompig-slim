# Loop guard

The primary agent's loop guard adds a hidden corrective message at the completed
turn boundary. The message reaches the next provider request and the saved
session together, including with extensions disabled. It does not block tools,
retry calls, force writes, terminate the run, or override continuation decisions.

## Identical calls

Five consecutive turns with the same whole-turn tool-call set trigger a steer.
Comparison ignores call IDs, batch order, JSON object key order, and harness
intent fields. Duplicate calls and array order still matter. Changed arguments,
no-call turns, and all-`wait` turns reset this counter.

## Deterministic failures

Three failing turns with the same tool, target, and recognized error class
trigger a recovery steer even when arguments change. The supported classes are:

| Tool | Class | Recovery steer |
|---|---|---|
| `edit` | Unmatched exact text | Re-read the target and use an exact anchor from the fresh content, or change approach |
| `edit` | Ambiguous text with multiple occurrences | Re-read the target and provide a unique exact anchor, or change approach |
| `grep` | Ripgrep regex parse error | Correct the regex, use `literal: true`, or change approach |

The guard classifies existing tool errors, not arbitrary output text. A
successful result that quotes an error does not count as a failure. Expected
search nonmatches, successful polling, failing tests, provider or network errors,
and cancellation are not recognized deterministic classes. The separate
five-identical-turn detector still applies to repeated calls of these kinds.

Targets use lexically cleaned `path` arguments. A missing or empty `grep` path
means `.`. The guard does not resolve symlinks or equate relative and absolute
paths. It retains one fingerprint, with a hashed target, rather than a growing
map of failures. A different failure class or target starts a new streak.

Each completed batch counts at most once. Multiple failures count only when
all have the same recognized fingerprint. A successful operation with the same
target clears the streak, even when that success and a failure occur in the
same batch. Success on an unrelated target preserves the streak. Mixed failure
fingerprints, unrecognized errors, or missing result identity clear the streak
without counting that batch. Result identity comes from tool-call IDs, not
result order or a shared error string.

For `codemode`, only complete structured nested-call records count. Each record
must supply its own tool name, complete arguments, settled status, and enough
error text to recognize the class. Missing, unfinished, oversized, or truncated
records with insufficient failure identity cannot establish a streak. Script text, printed errors, and
display previews are not used to infer which nested tool failed. Same-target
success overrides failure regardless of nested-call completion order. This is
a turn-boundary detector, not a detector for loops inside one script.

## State and diagnostics

Both detectors start fresh on every agent run. Aborted or errored turns,
no-call turns, and all-`wait` turns clear their state. State is not reconstructed
from history or shared with another session or child agent.

Continued failures can earn another steer. The deterministic counter saturates
at three. Its saved diagnostic includes the tool, class, count, and a target
summary capped at 200 Unicode characters plus an ellipsis. It stores no edit
anchors, search patterns, or result bodies. The identical-call diagnostic keeps
its existing argument and result summary bounds.

`internal/loopguard` tests cover batch and lifecycle behavior. Its loopback
provider fixtures drive real Session tools, inspect the next provider request
and saved session, recover through a fresh read, and check polling, expected
nonmatches, and a red-test/fix cycle without deterministic-error steers.
