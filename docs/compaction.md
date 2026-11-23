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

The checkpoint request extends the original conversation prefix on the same
model, with the same tool declarations, cache retention, and session affinity.
It includes the final assistant response and asks for a continuation checkpoint.
Tools remain declared for prefix compatibility, but the compactor does not execute
tool calls. Manual and context-overflow compaction retain their existing request
format and fallback behavior.

Idle compaction requires removable conversation history. It skips small sessions,
already-compacted context, active or queued work, and a separately assigned
compaction model that would lose the cached prefix. A new real request supersedes
the old deadline. One idle period cannot create repeated checkpoints.

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
cache metadata. The separate
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
```

The transcript scenario scrolls through two compaction boundaries, excludes a
sibling branch, and checks the next provider request contains only retained
context. The idle scenario checks the summary request's shared prefix, persisted
checkpoint, final assistant response, and subsequent compacted request.
