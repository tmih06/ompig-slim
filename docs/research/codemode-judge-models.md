# Judge models in Pi codemode and oh-my-pi

## Scope

Read-only investigation of the official [Pi codemode documentation](https://docs.pi.dev/latest/codemode)
and the checkout at `/home/ubuntu/oh-my-pi`. Source references below are relative
to that checkout. No provider requests or credential inspection were performed.

## Main distinction

Pi exposes classifiers as `models.classify(model, { state, questions })` inside
its QuickJS codemode runtime. Scripts discover credentialed classifiers with
`models.getAvailableOfType("classifier")`; chat models cannot be invoked through
that API. Provider errors are returned as `stopReason`/`errorMessage`. At most
four classifier/image calls run concurrently per script; usage is added to the
codemode result and session cost.
Sources: [Codemode: Models and Classify](https://docs.pi.dev/latest/codemode#models),
[Use classifier models](https://docs.pi.dev/latest/models#use-classifier-models).

The inspected oh-my-pi checkout instead exposes judgment through its JS/Python
`eval` runtimes: `judge(state, questions, options)` and
`judgeBatch(states, questions, options)` (`judge_batch` in Python). The script
does not select a classifier directly: the host resolves its configured `judge`
model role. This is an explicitly callable inference service, not a mandatory
review step automatically run after each script or tool call.
Sources: `packages/coding-agent/src/prompts/tools/eval-judge.md:1`,
`packages/coding-agent/src/eval/judgment-bridge.ts:1`.

## Execution path in oh-my-pi

```text
JS judge(state, questions, { images })
  -> __omp_call_tool__("__judge__", args)
  -> runEvalJudgment: validate state/questions; load images; acquire request slot
  -> sessionJudge: bind session registry/settings/cache/cost gate/usage ledger
  -> resolveJudge / ChainJudge: resolve the judge-role candidate chain
  -> native decision API OR TextJudge backed by chat/local inference
  -> normalize answers; return answer map to the script
```

The JS prelude unwraps the host's `{ answers, model }` result to `answers`.
The boundary accepts a nonempty string, JSON object, or JSON array as state;
questions are keyed by ID and support `choice`, `bool`, and `score`. The bridge
maps public `bool` to the library's internal `noul`, and back to
`{ type: "bool", bool: P(yes) }`. Pi's documented bool result instead uses
`{ type: "bool", probability }`.
Sources: `packages/coding-agent/src/eval/js/shared/prelude.txt:469`,
`packages/coding-agent/src/eval/judgment-bridge.ts:131`, `:160`, `:199`, `:225`,
`:239`, `:266`; [Pi classifier types](https://docs.pi.dev/latest/codemode#classify).

## Model selection and transports

The `judge` role accepts native judge, tiny, and chat models. Backend selection
uses the model API rather than its provider name: native judgment APIs use
`NativeJudge`, `local-inference` uses an on-device keyword backend, and other
candidates use a prompted chat backend. Resolution supports role chains, and
candidate lists are cached briefly (one second), so role/catalog changes can
take effect without rebuilding feature consumers.
Sources: `packages/coding-agent/src/config/model-roles.ts:51`, `:90`;
`packages/coding-agent/src/judgment/index.ts:171`, `:234`, `:366`, `:635`.

Native wire adapters cover TypeSafe System One, OpenRouter Decisions,
Cloudflare Workers AI System One, and OpenAI Decisions. They share the typed
judgment interface but encode/decode provider-specific requests. Native auth
uses the existing credential/retry machinery, including key rotation on
401/403, bounded transient-error retries, and provider question-count limits.
Source: `packages/ai/src/judgment/native.ts:1`, `:52`.

Chat/local fallback is not equivalent to a native probability distribution:
`TextJudge` asks for keyword answers and synthesizes one-hot probabilities and
confidence 1. Critically, after the first native candidate in a chain, only
native candidates remain: failed native judgments cannot silently fall back to
uncalibrated chat answers. Image requests filter incompatible candidates and
fit images to native transport limits rather than silently dropping them.
Sources: `packages/ai/src/judgment/text.ts:1`;
`packages/coding-agent/src/judgment/index.ts:267`, `:280`, `:307`, `:390`.

## Batches, cost, and failures

`judgeBatch` hands work to a host-owned job, rather than leaving a JS loop
running in one cell. It supports bounded concurrency (up to 32), per-item
retries (default one), incremental `drain`, results/status/cancellation, and
reattachment after a kernel reset. Per-item failures are data; run-wide failure
is surfaced by `drain`. Ownership survives cells/kernel resets, not a promised
process-crash recovery mechanism.
Sources: `packages/coding-agent/src/prompts/tools/eval-judge.md:13`;
`packages/coding-agent/src/eval/judgment-batch-bridge.ts:1`, `:43`;
`packages/coding-agent/src/eval/js/shared/prelude.txt:568`.

Each billed attempt, including failures/retries, is journaled to the session's
model-usage ledger and emits judgment telemetry; wholly cached native answers
are not billed again. The session cost gate admits attempts and records cost.
Native answers are cached by model and request, allowing already-answered
questions to be reused. Exhausted candidate chains throw errors; this differs
from Pi's documented nonthrowing provider-error result.
Sources: `packages/coding-agent/src/judgment/index.ts:126`, `:450`, `:539`, `:681`;
`packages/coding-agent/src/eval/judgment-bridge.ts:239`.

## Design takeaway

Both designs let a script send state plus a rubric to cheap typed inference,
then use ordinary code to filter, rank, branch, or aggregate before returning
output to the main model. Pi makes model discovery/selection explicit in the
script. oh-my-pi centralizes selection, fallback policy, caching, billing, and
batch ownership in a reusable host-side judge service, exposed through a thin
runtime bridge. Neither API alone implies an automatic approval/security gate.
