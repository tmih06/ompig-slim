# Goal continuation: runtime evidence for #76

All runs use the real `coding.Session`, the production command runner, and a
loopback OpenAI-compatible model that records each request. `internal/goalext/session_fixture_test.go`
holds the fixture.

## Premise under test

A goal continuation is a queued message. Whoever owns the goal decides whether it
runs, and that decision is re-read when the queue is consumed.

## Defect 1: a queued continuation ignores human control

`/goal pause` sets the tracker to `paused`, yet the continuation queued a moment
earlier still runs.

```
pending=0 status=active          # settle has queued poke 1
after pause: pending=0 status=paused
requests=2 pokes=1 status=paused # poke 1 ran as request 2 anyway
```

`Continue` is returned from `agent_before_settle` with a follow-up already
queued. Nothing consults the tracker again when the queue is drained, so the
pause cannot reach the decision.

## Defect 2: Escape pauses the goal too late

`OnAgentSettled` is what pauses an aborted goal, and it runs after the run has
settled. `/goal resume` issued in between sees the goal still `active`:

```
settled status=paused   # once agent_settled has run
after resume: status=active streaming=false requests=1
later:        status=active streaming=false requests=1 pending=0
```

If the resume lands first, it fails with "cannot transition goal from active to
active", and the pause arrives afterwards. Either way the goal does not restart.

## Defect 3: resume starts no turn

`Resume` only flips the status. The next continuation is queued at the *next*
`agent_before_settle`, and an idle session never reaches one. The built binary
shows the same shape: after Escape, `/goal resume` produces no model request at
all.

```
--- after escape: 1 requests
--- after resume: 1 requests
```

## What is already correct after #90

Autonomous continuation of an untouched goal, and ordinary follow-ups after a
pause, already behave. Requests confirm pokes continue every turn while the goal
is active, and a later question carries no poke.
