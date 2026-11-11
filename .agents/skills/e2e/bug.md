# Bug lane

Behavior the code misbehaves against. One rule outranks the rest: the fix goes at the **root cause**, and the reproduction that proved the bug stays in the repo as a **regression test**.

Read `../diagnosing-bugs/SKILL.md` for the reasoning loop; this file is the delivery order around it.

## 1. Reproduce red

Make the bug go **red**: one repeatable command, built from the repo's own tooling, that fails on this bug and whose failure is this bug rather than a neighbor.

- Prefer the existing test runner at the seam closest to the symptom. A bespoke script is a last resort and usually means the seam was chosen badly.
- A manual reproduction is legitimate — a carried-out click, request, or CLI invocation with its expected output written down. It proves the bug; the regression test comes at step 6.
- Narrow the signal until it isolates this bug. A signal that also fails for unrelated reasons cannot tell you whether the fix worked.
- When nothing reproduces it, the ticket is a hypothesis. Say what would distinguish a real bug from a misunderstanding, and stop.

Done: one command, run in this session, fails with output you can quote — and its failure is this bug.

## 2. Root cause

Find the code that produces the misbehavior, and state the cause in one sentence naming the file and the assumption it breaks.

- Walk back along the data from the symptom: which value is wrong, who wrote it, which condition made that the wrong value.
- Confirm against the red signal. Change one hypothesis at a time and watch the signal flip; temporary logging and instrumentation are fine while the tree is uncommitted.
- Keep going past the first suspicious line. The line that looks wrong is often faithfully doing what a caller told it to do.
- Record the cause while it is fresh; the step 5 commit body carries it.

Done: the cause is one sentence naming the file:line and the broken assumption, and the red signal confirms it.

## 3. Fix

Make the smallest change at the cause.

- Repair the invariant or the caller that supplied the bad input; a guard around the symptom leaves the next caller exposed.
- Keep the change inside the module that owns the behavior. A fix scattered across callers usually means the cause was misdiagnosed.
- Fix what the ticket needs. An unrelated defect found on the way is a report, not a ride-along.
- Keep the fix free of fallbacks for the state the fix makes impossible.

Done: the edit sits at the cause and nowhere else.

## 4. Confirm

Compare before and after with the step 1 command: same invocation, same environment, both outputs recorded.

- Re-run it and watch the signal flip from red to green.
- Re-run the checks covering the touched code, so the fix is not a second bug traded for the first.
- Hold the tree as it is until the comparison is written down. A fix you cannot show flipping is a fix you do not have, and one that turns something else red has moved the bug rather than removed it.

Done: the step 1 command, re-run in this session, is green where it was red, both outputs are recorded, and the covering checks pass.

## 5. Commit the fix

One commit: the fix and nothing else. The body states the root cause, the comparison that confirmed it, and the behavior a caller can now rely on. Format and wrapping follow `AGENTS.md`.

Done: the commit holds no test file and no unrelated edit, and its body names the root cause.

## 6. Regression test

Promote the reproduction into the suite, at the seam the bug crossed, so the bug cannot return unnoticed.

- Assert the behavior the ticket expected, not the internal shape of the fix. The test has to survive a rewrite of the fix.
- Prove that it guards: revert the fix in the working tree, run the test, watch it fail for this bug's reason, restore the fix, run it green. Both runs happen in this session.
- Derive expected values from an independent source — the ticket, a spec, a known-good output — rather than recomputing them the way the fixed code computes them.
- Name the test after the behavior, so a future failure reads as the promise that broke.

Done: the test fails without the fix and passes with it, and its failure message connects to the ticket without reading the diff.

## 7. Commit the test

A separate commit: the regression test alone. The body names the bug it guards and the command that runs it.

Done: the test commit applies cleanly on top of the fix commit, and the checks covering it pass.
