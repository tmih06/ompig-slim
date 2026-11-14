---
name: code-improve
description: "Review, fix, simplify, and optimize code across four axes (Standards, Spec, Simplification, Optimization). Diagnoses issues in parallel sub-agents and applies fixes and improvements directly in the working tree, verifying with project checks. Use when the user wants to review and improve a branch, PR, or diff (\"code-improve\", \"review since X\"), simplify a change (\"/simplify\"), or optimize performance (\"/optimize\")."
---

Four-axis code review and improvement skill—diagnose issues across all four axes, then fix and improve the code directly:

- **Standards**: does the code conform to this repo's documented coding standards and avoid code smells?
- **Spec**: does the code faithfully implement the originating issue / spec?
- **Simplification**: can the change stay correct with less complexity, duplication, needless abstraction, speculative flexibility, or defensive code?
- **Optimization**: does the change or target introduce avoidable performance, scaling, or algorithmic costs (N+1 queries, serial I/O, unbounded fan-out, missing batching/caching/indexes, wrong complexity, or overload risks)?

## Modes

Choose the mode from the user's request:

1. **Four-axis improve (default)**: review the target diff across all four axes in **parallel read-only sub-agents** (see [Process](#process)), then **apply the fixes and improvements** directly in the working tree, verify with the project's checks, and report what changed. If the user explicitly asks for a **review-only / report-only** pass ("don't edit", "report only"), stop after aggregating the sub-agent findings without editing.
2. **Simplification pass**: when the user asks to simplify a completed workpackage, branch, PR, or set of files (or invokes `/simplify`), follow [SIMPLIFY.md](./SIMPLIFY.md): establish the workpackage, apply routine behavior-preserving simplifications, ask for approval before significant removals, run the project's checks, and report what changed.
3. **Optimization pass**: when the user asks to improve performance, latency, throughput, memory usage, or efficiency (or invokes `/optimize`), follow [OPTIMIZE.md](./OPTIMIZE.md) and [SCALING.md](./SCALING.md): define target metrics, attribute bottlenecks, check static/asymptotic issues, macro-optimize before micro-optimizing, verify with before/after measurements, and report each change with its measured effect. If the user explicitly asks for a **review-only** optimization pass, dispatch only the read-only Optimization sub-agent scoped to the target and report under `## Optimization`.

The originating issue or spec usually lives in **GitHub Issues**. Fetch it with the `gh` CLI, which infers the repository from the clone. When findings or edits name domain concepts, use the vocabulary from `GLOSSARY.md` (or the per-context glossaries that `GLOSSARY-MAP.md` points at).

## Process

### 1. Pin the fixed point

Identify the code under review before spawning sub-agents:

- If the user named a fixed point (commit SHA, branch name, tag, `main`, `HEAD~5`, etc.), use `git diff <fixed-point>...HEAD` and `git log <fixed-point>..HEAD --oneline`.
- Otherwise, fall back to the default branch's merge-base (`git merge-base origin/HEAD HEAD`, or `origin/main` / `origin/master`), or uncommitted work (`git diff`) when there are no branch commits, or the specific files/module the user named.
- If none of these resolve to a non-empty change, ask the user what to review and improve. Confirm any git ref resolves (`git rev-parse <fixed-point>`) before spawning sub-agents.

### 2. Identify the spec source

Look for the originating spec, in this order:

1. Issue references in the commit messages (`#123`, `Closes #45`), fetched with `gh issue view <number> --json number,title,body,comments`. GitHub shares one number space across issues and PRs, so when `gh issue view` doesn't resolve it, try `gh pr view <number> --json number,title,body,comments`.
2. A path the user passed as an argument.
3. A spec file under `docs/`, `specs/`, or `.scratch/` matching the branch name or feature.
4. If nothing is found, ask the user where the spec is. If they say there isn't one, the **Spec** sub-agent will skip and report "no spec available".

### 3. Identify the standards sources

Search the repo for every file that documents how code should be written. When `CODING_STANDARDS.md` or `CONTRIBUTING.md` exists, it must be on the list. Include ADRs under `docs/adr/` or `src/*/docs/adr/` that constrain the area the diff touches.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** below: a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing. Two rules bind it:

- **The repo overrides.** A documented repo standard always wins; where it endorses something the baseline would flag, suppress the smell.
- **Always a judgement call.** Each smell is a labelled heuristic ("possible Feature Envy"), never a hard violation. Like any standard here, skip anything tooling already enforces.

Each smell reads *what it is* → *how to fix*; match it against the diff:

- **Mysterious Name**: a function, variable, or type whose name doesn't reveal what it does or holds. → rename it; if no honest name comes, the design's murky.
- **Duplicated Code**: the same logic shape appears in more than one hunk or file in the change. → extract the shared shape, call it from both.
- **Feature Envy**: a method that reaches into another object's data more than its own. → move the method onto the data it envies.
- **Data Clumps**: the same few fields or params keep travelling together (a type wanting to be born). → bundle them into one type, pass that.
- **Primitive Obsession**: a primitive or string standing in for a domain concept that deserves its own type. → give the concept its own small type.
- **Repeated Switches**: the same `switch`/`if`-cascade on the same type recurs across the change. → replace with polymorphism, or one map both sites share.
- **Shotgun Surgery**: one logical change forces scattered edits across many files in the diff. → gather what changes together into one module.
- **Divergent Change**: one file or module is edited for several unrelated reasons. → split so each module changes for one reason.
- **Speculative Generality**: abstraction, parameters, or hooks added for needs the spec doesn't have. → delete it; inline back until a real need shows.
- **Message Chains**: long `a.b().c().d()` navigation the caller shouldn't depend on. → hide the walk behind one method on the first object.
- **Middle Man**: a class or function that mostly just delegates onward. → cut it, call the real target direct.
- **Refused Bequest**: a subclass or implementer that ignores or overrides most of what it inherits. → drop the inheritance, use composition.

### 4. Spawn the sub-agents in parallel

Dispatch all four sub-agents together, in the foreground, and aggregate the reports they return. Omit Spec when no spec is available, and Optimization when the diff cannot affect runtime behaviour. If the user asked for an optimization pass only, dispatch only the Optimization sub-agent and scope it to the target they name (a diff, file, module, endpoint, or the whole codebase); report under `## Optimization`. All reviewers are read-only: inspect source and return findings without edits or build, lint, test, or formatter runs.

**Standards sub-agent prompt** should include:

- The full diff command and commit list.
- The list of standards-source files you found in step 3, **plus the smell baseline from step 3** pasted in full (the sub-agent has no other access to it).
- The brief: "Report, per file/hunk where relevant, (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Distinguish hard violations from judgement calls: documented-standard breaches can be hard, but baseline smells are always judgement calls, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Under 400 words."

**Spec sub-agent prompt** should include:

- The diff command and commit list.
- The path or fetched contents of the spec.
- The brief: "Report: (a) requirements the spec asked for that are missing or partial; (b) behaviour in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where the implementation looks wrong. Quote the spec line for each finding. Under 400 words."

**Simplification sub-agent prompt** should include:

- The diff command and commit list, standards-source files, and spec path or contents when available.
- The checklist from [SIMPLIFY.md](./SIMPLIFY.md) (section 3, "What to remove or simplify"), pasted or summarized in the prompt.
- The brief: "Look for changes that can satisfy the same requirements with a smaller, more reused implementation. Apply KISS (choose the least complex design that works), YAGNI (build for current requirements), and DRY (give each piece of shared behavior one owner). Before proposing new code or an extraction, inspect each changed implementation and search the repository outside the diff for existing helpers, components, services, types, and shared modules. Read the candidate code and representative callers so you can check semantics, errors, side effects, and module boundaries.
  - **Reuse:** the change recreates an existing capability. Point to the existing symbol and path, and show the call that would replace the new implementation.
  - **De-duplication:** the change repeats domain logic or creates a second implementation of the same behavior. Favor the current canonical owner. If a new shared abstraction is warranted, name the current callers and the ownership rules it must respect.
  - **Simplification:** the change includes needless abstractions, trivial wrappers/pass-throughs, speculative flexibility/configuration/extension points, speculative defensive programming inside trusted code, fallback behavior for impossible states, excessive state/flags/branching, or loose types where invalid states could be unrepresentable. Recommend the smallest deletion or refactor that keeps behavior intact, and flag any removal that would be significant (public APIs, validation/auth/error handling, multi-consumer abstractions, test-covered behavior).
  Scope the review to opportunities introduced or exposed by the diff; unrelated cleanup belongs elsewhere. Match abstraction size to current needs: similar-looking code is not enough reason to share semantics or couple modules. For every finding, give the changed file and lines, the existing code or callers that support it, the replacement or refactor, and the benefit plus compatibility risk. When no compatible reuse candidate exists, say what you searched for any proposed extraction. Mark opportunities as judgement calls unless a documented rule backs them; leave out style-only preferences and tooling-enforced findings. Return under 400 words, or state that you found none."

**Optimization sub-agent prompt** should include:

- The diff command and commit list, plus the spec/issue when available.
- The relevant performance and scaling checklist from [SCALING.md](./SCALING.md) and the static-analysis / macro-optimization principles from [OPTIMIZE.md](./OPTIMIZE.md) pasted into the prompt (the sub-agent has no other access to them).
- The brief: "Review the changed code for concrete performance, scaling, and algorithmic opportunities introduced or exposed by the diff. Follow the changed call path outside the diff when needed to prove a finding. Macro-optimize before micro-optimizing: prefer approach-level changes (batch, join, index, cache, queue, pagination, algorithm/data structure, removing classes of work) over micro-optimizations. Name the operation, its expected scale, and the bottleneck mechanism before proposing a fix. Look for:
  - **Algorithmic & structural cost:** wrong asymptotic complexity, wrong algorithm or data structure, unnecessary repeated work, work in the wrong layer, or inefficient control flow.
  - **N+1 and chatty I/O:** queries or remote calls inside loops, especially a bulk endpoint that issues one query per item; lazy loading; serial awaits for independent work. Prefer set-based or batch queries, joins, eager loading, or a request-scoped loader; chunk large batches.
  - **Unbounded work:** fan-out or concurrency without a bound; loading whole result sets; deep offset pagination; oversized payloads. Bound concurrency, stream or paginate, and project only needed fields.
  - **Database cost:** missing or misused indexes, full scans, over-fetching, per-row writes in a bulk request, write amplification, long transactions, lock contention, connection churn, hot partitions.
  - **Repeated cost:** expensive reads or computations that are reused and could be cached, materialized, or precomputed; call out invalidation, staleness, and stampede risk.
  - **Overload and tail latency:** missing timeouts, retries without backoff/jitter or a retry budget, missing circuit breakers/bulkheads/load shedding, work that belongs on a queue, and fan-out that amplifies p99.
  For each finding, cite changed file/lines, state the mechanism and expected impact, propose the smallest fix, and name the trade-off (consistency, complexity, memory, write cost). Mark whether the finding is proven by the code path or needs measurement, and name the measurement (query count, `EXPLAIN ANALYZE`, profile, load test, p99). Omit micro-optimizations and speculative rewrites that lack a plausible scaling or algorithmic mechanism. Return findings under 400 words, or explicitly report none."

### 5. Apply fixes and improvements

Unless the user explicitly asked for a **review-only / report-only** pass, fix the valid issues raised by the four sub-agents directly in the working tree:

- **Deduplicate and prioritize:** When reviewers flag the same issue, resolve it once under the most relevant axis. Read the affected files in full before editing.
- **Standards & Spec:** Fix documented-standard violations, clear code smells, missing/partial spec requirements, and incorrect implementations. Flag any scope creep that looks intentional before removing it.
- **Simplification:** Apply routine, behavior-preserving simplifications (inlining single-use trivial wrappers, removing newly added duplication, tightening types, removing unreachable branches). Follow the **Approval gate** in [SIMPLIFY.md](./SIMPLIFY.md) before any significant removal (public APIs, validation/auth/error handling, multi-consumer abstractions, test-covered behavior, or uncertain invariants).
- **Optimization:** Apply proven macro-optimizations (fixing N+1s, batching, indexing, algorithmic improvements, bounding work) following [OPTIMIZE.md](./OPTIMIZE.md) and [SCALING.md](./SCALING.md). Do not claim an optimization without before/after evidence; mark unmeasured hypotheses as pending measurement.

### 6. Verify and report

After applying edits, run the project's own checks (type check, lint, and tests covering the affected code). If any approval is pending, leave the working tree in a consistent, passing state.

Present the results under `## Standards`, `## Spec`, `## Simplification`, and `## Optimization` headings. For each axis, list the findings, what was fixed/improved, and anything left untouched or pending user approval. Keep the axes separate; cross-reference shared issues rather than counting them twice.

End with a concise summary: finding and fix counts per axis (noting any skipped axis), any pending approvals, and the validation commands run with their results.

## Why four axes

A change can pass one axis while raising findings on another:

- Code that follows every standard but implements the wrong thing → **Standards pass, Spec fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Spec pass, Standards fail.**
- Code that meets standards and requirements but duplicates an existing capability → **Standards and Spec pass, Simplification opportunity.**
- Code that is clean and correct but issues one query per item in a bulk request → **Standards, Spec, and Simplification pass, Optimization fail.**

Reporting them separately stops one axis from masking another.
