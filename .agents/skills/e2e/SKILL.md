---
name: e2e
description: "Ship one ticket end to end: triage it, then the bug lane (reproduce red, root-cause, fix, confirm, regression test) or the feature lane (survey the code that already owns it, then TDD), then debloat with simplify, optimize with measurements, commit, and open the PR. Use when asked to take an issue, requirement, spec, or bug report all the way to a pull request, or when the user says \"e2e\" or \"end to end\"."
---

# End to End

Carry one ticket from hand-off to an open pull request. Triage splits it into a **bug lane** or a **feature lane**; both rejoin at **landing**. Read only the lane triage picks.

A **ticket** is the input in whatever shape it arrived: a GitHub issue, a requirement, a spec, a stack trace, or one sentence in the prompt. Normalize it once, then every step below treats it the same way.

Invoking this skill authorizes the commits, the push, and the PR its steps describe — and nothing beyond them.

## 1. Pickup

Turn the hand-off into a ticket you can work against.

- Read the input in full: every comment on an issue, and anything it links to. A pasted paragraph or a spoken sentence is the ticket, so its text is all you need to begin.
- Restate the ticket in three lines: the observable problem, the behavior the ticket expects, and the **acceptance check** — the command, request, or interaction whose result proves the expectation is met.
- Read the rules before touching code: `AGENTS.md`, plus `GLOSSARY.md`, `CONTEXT.md`, and ADRs where the repo has them.
- Work on a dedicated branch off the default branch, named for the ticket. Stage explicit paths and leave other sessions' edits alone.
- Run the repo's checks once before changing anything. A failure already there is not yours to fix or to hide; record it now so later red is attributable.

Done: the three-line restatement exists, the repo rules and check commands are in context, the branch is checked out, and the failures that are not yours are written down.

## 2. Triage

Route the ticket, and stop the ones that should not become code.

- **Lane:** a bug is behavior the code misbehaves against; a feature is behavior that does not exist yet. A ticket that needs both runs the bug lane for the broken part, then the feature lane for the rest.
- **Already done:** search before building. When the code already implements the ticket, report where with evidence and stop — a second implementation is the defect, not the fix.
- **Not actionable:** no acceptance check, contradictory requirements, or a design question the ticket leaves open. Ask before writing code, naming the one question that would unblock you.
- **Size:** a ticket too large to land in one pass is several tickets. Say which slice ships and which waits.

Done: the lane is chosen with a one-line reason, and every question that would change the design is answered from the ticket or from the code, or asked.

## 3. Todo list

Read the chosen lane file (step 4), then `todo init` with two phases: the lane's steps in their order, then landing's steps. Copy the step names verbatim, so a later checkpoint compares like for like. Keep the list honest while you work: mark an item done when it is done, and block it with its reason when it stalls.

Done: the todo has one phase per stage, names every step of this ticket, and carries nothing else.

## 4. Run the lane

- Bug: [bug.md](bug.md) — reproduce red, root-cause, fix, confirm, regression test.
- Feature: [feature.md](feature.md) — survey the code that already owns the behavior, then TDD.

Each lane names what its commits contain; the format and body rules live in `AGENTS.md`.

Stop at the step that cannot complete: no reproduction, a check failing outside your diff, or a seam the ticket leaves undecided. Report it, leave the tree consistent, and end the run — landing does not start.

Done: every lane step is done or blocked with its reason, the lane's commits exist, and the checks covering the ticket's surfaces pass on the branch.

## 5. Landing

Each landing step loads a skill by reading its file, resolved against this skill's directory (use absolute paths in tool calls). `simplify` is user-invoked, so it never appears in the available-skills list; read its file directly.

1. **Debloat** — read `../simplify/SKILL.md` and run it over this branch's diff. Apply its routine findings and ask about anything it classes as a significant removal. Commit the debloat on its own.
2. **Optimize** — read `../optimize/SKILL.md` and run it over the code the ticket changed. Measure before claiming a win and commit only a change you measured; with no measurement, report that and leave the debloated tree committed.
3. **Pull request** — read `../pr/SKILL.md` and open the PR with the base branch, body, and evidence taken from commands run in this session. One commit per purpose, matching `AGENTS.md`: lane work, then debloat, then a measured optimize when one exists.

Done: the branch carries those commits, and the PR exists with a diff that matches them and evidence that quotes this session's real output.

## 6. Summary report

After landing is complete, return a concise summary report. Use these headings, and write **None** when a category produced no work:

- **Made** — the ticket behavior implemented or fixed, with the key files or commits.
- **Simplified** — the debloat changes made, including notable removals or clarifications.
- **Optimized** — each measured performance or efficiency change, with before/after evidence; state when review found no measured optimization.
- **Verification** — the checks run and their results.
- **Delivery** — the branch, commits, and pull-request link.

The report must describe the complete run, not only the final diff, and must distinguish measured optimization from an optimization opportunity that was left unchanged.

Done: the report is returned after the PR is open and every heading accounts for the work performed or explicitly records that none was done.
