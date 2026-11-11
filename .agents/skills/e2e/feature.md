# Feature lane

Behavior the code does not have yet. Two rules shape the work: extend the code that already owns the behavior, and write each test before the code that makes it pass.

Read `../tdd/SKILL.md` for the loop and for what makes a test worth keeping; this file is the delivery order around it.

## 1. Survey

Find what the repo already has before designing anything.

- Search by the ticket's domain words rather than imagined names: the glossary term first, then the types, helpers, extension points, config, and tests around the module that owns it.
- Name the owner the feature belongs to, and extend it. Behavior that exists already has an implementation; a second one is a defect, not a shortcut.
- Read the owner's call sites and its existing tests, so the new behavior enters through the same seam the rest of the module uses.
- When the survey finds the ticket already implemented, return to triage and report it with evidence instead of reshipping it.

Done: every part of the ticket either names the existing code it extends or is justified as genuinely new, and the searches behind that claim are named.

## 2. Seams

Write down the public boundary each test will observe, with one line on what it catches and what it misses.

- A seam the ticket and the existing interface already determine: proceed. A seam the ticket leaves open in a way that changes the public interface: ask before the first test.
- Prefer the existing seam. A new one earns itself by the behavior it isolates.
- Settle the shape of an interface before testing it. A test written against a guessed shape commits you to the guess.

Done: the seam list is written down with what each catches, and no question about the interface is left open.

## 3. Slices

Red → green, one seam at a time: a failing test, then the least code that passes it, then the next test.

- Work in vertical slices. A full suite written up front specifies imagined behavior and molds the implementation to a guess.
- Write only enough code for the current test. Parameters, options, and extension points arrive with the caller that needs them.
- Run the covering checks after each slice, not once at the end. A green you never watched is a green you do not have.
- When a slice shows the seam was wrong, change the seam and its test then, before the next slice.

Done: every agreed seam has a test that went red before its implementation, and the checks covering the touched code pass.

## 4. Commit

One commit: the behavior and the tests that specify it, together. The body states the behavior added, the seams covered, and the existing code extended. Documentation that `AGENTS.md` requires for this change goes in the same commit.

When triage split the ticket into landable slices, commit each slice separately, in dependency order.

Done: each commit is one revertible behavior change whose tests fail without it, with no unrelated edit riding along.
