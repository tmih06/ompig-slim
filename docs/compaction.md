# Transcript history and idle compaction

Compaction replaces older model context with a checkpoint summary and a retained
recent tail. It does not delete entries from the session JSONL file.

## Resumed transcript

A resumed interactive session initially renders its retained context. In fullscreen
mode, scrolling upward loads earlier entries from the active branch on demand.
Older compaction markers remain part of that history. Entries from abandoned
branches stay hidden.

Loading display history does not expand the model's context. The next request
still uses the latest checkpoint and retained tail. This keeps the initial resume
from decoding every historical tool result.

The terminal owns native scrollback in regular mode. The application cannot
observe a terminal scrollbar moving. Use the application's backward-scroll
keys — PageUp or Ctrl+Home with an empty editor — to request earlier history
instead. Each keypress loads one bounded page.

## Idle checkpoint policy

Idle compaction uses the last real request's model cache-lifetime metadata.
The scheduler targets 90 percent of that lifetime, with at least ten seconds of
margin. Cache lifetimes are provider estimates, not eviction guarantees.
When a provider has no cache-lifetime metadata, the scheduler uses
`compaction.idleDelaySeconds`, which defaults to 240 seconds. This fallback is an
idle policy, not an estimate or guarantee of the provider's cache lifetime.

Idle compaction uses the assigned `@compact` role, including its thinking level
and fallback chain, just like manual and threshold compaction. Assigning a
different compact model does not disable idle checkpoints. The chat model stays
unchanged.

Without an assigned compact role, the checkpoint request extends the original
conversation prefix on the chat model, with the same tool declarations, cache
retention, and session affinity. It includes the final assistant response and
asks for a continuation checkpoint. Tools remain declared for prefix
compatibility, but the compactor does not execute tool calls. An assigned
compact role uses the normal summarization request instead, without a guarantee
of cache-prefix reuse.

Idle compaction requires removable conversation history. It skips small sessions,
already-compacted context, and active or queued work. A new real request
supersedes the old deadline. One idle period cannot create repeated checkpoints.

While the session is idle, the interactive footer's stats line shows
`Idle compaction check in Ns`. The countdown uses the scheduler's actual
deadline, including when cache warming is off. A new request replaces the
deadline. Active work hides the countdown. The deadline schedules an
eligibility check, so a small session can reach zero without producing a
checkpoint. An overdue offer waiting for availability shows
`Idle compaction pending`.

Completed compaction cards show the trigger, the summary model, and elapsed time,
even when the summary is collapsed. Triggers are `idle`, `manual`, `threshold`,
and `overflow`. Model identities use the successful provider response when
available, or the resolved request model. Failed fallback attempts do not appear.
A summary made with multiple models lists each distinct identity. Extension
summaries have no host-observed model and show `model unknown`.

Elapsed time includes summary hooks, routing, retries, and fallback attempts.
It excludes the idle countdown and persistence. These details are saved with the
checkpoint and appear again on resume. Older checkpoints without these details
keep their existing display.

The summary is a paid model request. A shared prefix can qualify for cached-input
pricing, but the provider controls cache reuse. The compacted conversation also
has a different prefix, so its first continuation can have a cache miss. No cost
or latency improvement is guaranteed.

## Settings

Automatic compaction enables idle checkpoints by default. Disable only the idle
policy in your global `~/.pi/agent/settings.json`:

```json
{
  "compaction": {
    "idleEnabled": false
  }
}
```

`compaction.enabled: false` disables both idle and threshold compaction.
`compaction.keepRecentTokens` controls the retained tail. Set
`compaction.idleDelaySeconds` to change the fallback delay for providers without
cache metadata. Values must be positive, finite, and no more than 1800 seconds;
an out-of-range value falls back to the 240-second default. The separate
`cacheWarming` setting controls cache-refresh requests, not checkpoint summaries.
Durable headless execution has its own [compaction contract](durable.md).

## Model cache metadata

The same policy applies to every provider. The scheduler uses `promptCache.short`
or `promptCache.long`, measured in seconds, from the selected model's metadata.
Explicit request retention takes precedence over `PI_CACHE_RETENTION`. Models
without lifetime hints use the configured fallback delay. A verified metadata
override lets the scheduler use provider timing instead of that fallback.

For a provider with a documented five-minute short-cache lifetime, a
[custom model override](custom-models.md#built-in-providers) can declare:

```json
{
  "providers": {
    "my-provider": {
      "modelOverrides": {
        "my-model": {
          "promptCache": { "short": 300 }
        }
      }
    }
  }
}
```

A hint only informs the scheduler. It does not configure or extend the provider's
cache lifetime. Do not copy the example lifetime to a different provider without
checking its retention policy.

## Verification

The isolated smoke scenarios exercise the built executable with temporary
profiles and loopback providers:

```sh
python3 scripts/smoke-test-resume-scrollback.py bin/ompig-slim
python3 scripts/smoke-test-idle-compaction.py bin/ompig-slim
python3 scripts/smoke-test-compaction-info.py bin/ompig-slim
```

The transcript scenario scrolls through two compaction boundaries, excludes a
sibling branch, and checks the next provider request contains only retained
context. The idle scenario checks the summary request's shared prefix, persisted
checkpoint, final assistant response, and subsequent compacted request. The
compaction-info scenario drives regular and fullscreen PTYs. It checks decreasing
and reset countdowns, idle and manual cards, fallback attribution, elapsed time,
summary expansion, and metadata after resume.
