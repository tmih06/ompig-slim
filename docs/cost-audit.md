# Model-cost audit

Goal: lower total expected spend while preserving task quality, caching, and useful automation. Fewer tokens or fewer requests alone are not sufficient measures of cost.

## Changes

| Finding | Change | Evidence |
| --- | --- | --- |
| `@agent` used a cloned parent session, including its conversation, to ask the model to route an already-addressed task. | Removed the orchestration-only model path. Mentions dispatch directly; existing `agentMentions: "model"` settings resolve to `direct`. The requested child still runs normally. | `TestLegacyModelMentionUsesDirectDispatch` |
| Human-directed mentions and focused child follow-ups could purchase an unrelated parent synthesis turn on completion. | Detached/human child runs remain visible in the Hub without waking the parent. Model-delegated background tasks still notify the parent, preserving automatic task completion. | `TestHumanAgentRunsDoNotWakeParentModel` (delegated and human cases) |
| Default `smart` join behaved like immediate notification, potentially waking an idle parent once per sibling. | Batch sibling completions while the parent is idle, with the existing 30-second liveness bound. A running parent can still receive results at its next boundary. Explicit `async` delivery remains available. | `TestSmartNotificationsBatchSiblingCompletions` |
| Every completion requested a full-result fetch even when its short result was already included. | Request retrieval only when the preview was actually truncated. Tool guidance discourages polling and recommends `wait:true` when completion is needed. | `TestCompletionNotificationRequestsFetchOnlyForTruncation` |
| Identical result/error strings could be injected twice. | Omit duplicate error text in tool results and notifications; distinct errors remain visible. | `TestAgentResultDoesNotDuplicateIdenticalErrors` |
| Agent/workflow manuals were included in model-facing tool descriptions on every request. | Compact descriptions by default, retaining API contracts, restrictions, local verification, and resume guidance. Full/custom descriptions remain explicit options. | `TestCostDefaultsAndCompactToolDescriptions` |

For the built-in types, rendered Agent description size is **6,338 → 1,602 bytes**, and SubagentWorkflow is **20,846 → 3,243 bytes**. These are description bytes, not tokenizer counts, total request sizes, or measured dollar savings. Custom agent definitions change these sizes. Existing `toolDescriptionMode: "full"` settings remain respected.

## Preserved because it can save money

- **Economically justified cache warming:** streaming remains the default. Existing economics compare estimated refresh cost with continuation probability × avoided cache-miss premium, using a $0.05 minimum expected saving. Replay safety, age bounds, cancellation, and explicit modes remain intact. `TestCostPolicyPreservesProfitableCacheWarming` covers profitable streaming/idle, unprofitable idle, no discount, and unknown prompt size. Estimates are not guarantees of a lower bill; provider pricing and continuation probability matter.
- **Stable cached prefixes:** child prompt inheritance retains the parent prefix unchanged. No per-request timestamp or volatile status was added to system prompts or tool descriptions.
- **Context inheritance:** parent conversations are not copied into children by default. Explicit `inherit_context`, custom agent instructions, and requested skill preloads retain their semantics; deleting necessary context can increase rework and cost.
- **Compaction:** automatic context management can avoid repeatedly sending a much larger history and prevent context-window failures. It was not disabled to eliminate one summarization call.
- **Useful parent continuation:** a model-delegated background result may require parent reasoning to finish the user's task. These notifications still work; human-only child interaction is distinguished from delegated work.
- **Quality controls:** no blanket downgrade of model, thinking effort, tool-output limits, or agent turn limits. These can cause retries, missed evidence, and more expensive rework.

## Other paths inspected

- Hub rendering, recent-activity reconstruction, usage dashboards, model metadata, and selectors use local/session data or non-inference HTTP services, not chat completions. Restoring tool activity reads saved child logs without creating a child model session.
- Codemode runs JavaScript locally; ordinary nested tool calls do not invoke a model. Its explicit `models` APIs can make paid requests. Tool discovery is local, and deferred MCP tools do not churn the codemode description.
- Standard provider tool-result conversion consumes the tool content, not the UI's entire agent record. Large UI details are not an excuse to trim useful model-visible results blindly.
- Workflow orchestration and gates run locally. Each `agent()` is a real model task. Workflow resume reuses unchanged successful leading calls without requesting those models again. Compact guidance favors local parsing, deduplication, tests, and exit-code checks over agents for mechanical work.
- Branch summaries and bug-report summaries remain explicit choices. Structured-output correction and provider retries are conditional recovery paths, not routine routing calls; removing them without an alternative can waste the work already purchased.

## Limits

This is a targeted source audit and local-fixture verification, not proof that every extension, provider, or user workflow is cost-optimal. Custom extensions can request models through their own APIs. No paid provider requests were needed for these regression tests. Actual savings depend on models, cache-hit rates, result sizes, configured descriptions, and workload.
