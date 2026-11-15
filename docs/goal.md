# Goal contract

`/goal <objective>` starts one autonomous objective for the session. PiG owns the
agent loop; this binary owns the goal state machine in `internal/goal`, and
`internal/goalext` connects it to the tool, the slash command, and the settlement
boundary.

An active goal is poked: at every settlement boundary the extension requests
another turn until the goal leaves `active`. That is the behavior the blocker
policy exists to bound, because a model that keeps reaching the same dead end
would otherwise be poked forever.

## States and transitions

| From | Operation | To |
|---|---|---|
| none | `create` | active |
| active | `pause`, or three identical blocker reports | paused |
| active | `complete` | complete |
| paused | `resume` | active |
| active, paused | `complete` | complete |
| any | `drop` | none |

A settled goal accepts no blocker or progress report, so a late report or a
pending automatic continuation cannot restart work a human stopped. An aborted
run (`Esc`) pauses the goal rather than completing it.

## Blocked and progress outcomes

`blocked` and `progress` are reported, never inferred. The harness does not read
final-answer wording and does not look for a Git diff, because both are proxies:
an agent that answered the objective read-only made progress without editing a
file, and an agent that repeated itself in prose may not be blocked at all.

`blocked` takes a `reason` naming the missing input, access, or decision. It is
required and never derived.

- Three consecutive reports of the same blocker pause the goal
  (`BlockerThreshold`), and that pause is the last automatic turn.
- `progress` clears the streak. So does `resume`, which is a human deciding to
  try again rather than a fourth report of the same blocker.
- A different blocker starts a new count at one.

### Blocker identity

Two reports are the same blocker when their normalized text matches:
whitespace-collapsed and case-folded, compared after truncation to 240 runes.
Nothing else is folded, because merging differently worded blockers would pause a
goal that has moved on to a new problem. `internal/goal/goal_test.go` pins the
normalization and the counts.

## What a pause shows

The report that crosses the threshold returns one notice naming the blocker and
the resume action. Status keeps the blocker and its count, but not the
instruction, so reading status again does not append the same notice. After
`resume` the blocker text stays as the explanation of the pause while the count
returns to zero.

## Status

`/goal show`, and the tool's `get`, report the objective, status, tokens, poke
count, and, when one is recorded, the blocker with its consecutive count. `drop`
discards the goal, so the next objective starts with no inherited blocker.

## Verification

`scripts/smoke-test-goal-blockers.py` drives the built binary against a loopback
provider. It reports a blocker until the goal pauses, then proves no further
provider request happens until an explicit resume, and covers a read-only progress
report and a changed blocker. `internal/goal` and `internal/goalext` tests cover
the transition table, the identity rules, and the notice-once behavior.
