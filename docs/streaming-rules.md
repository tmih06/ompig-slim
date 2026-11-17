# Streaming rules

Streaming rules catch selected code patterns as the agent prepares tool calls.
They send corrective instructions without adding every rule to the system prompt.
This is a tool-focused port of oh-my-pi's Time Traveling Stream Rules, or TTSR.
It is not a linter or a security boundary.

## Bundled checks

Six advisory rules load by default.

| Rule | Pattern |
|---|---|
| `go-ioutil` | Deprecated `io/ioutil` imports |
| `go-rand-v2` | Legacy `math/rand` imports |
| `go-join-hostport` | `fmt.Sprintf("%s:%d", ...)` network addresses |
| `ts-no-any` | TypeScript `: any` annotations and `as any` assertions |
| `ts-bare-catch` | Underscore-prefixed catch bindings |
| `rs-box-leak` | Rust `Box::leak` calls |

A match asks the model to check the code, consider exceptions, and fix a real
violation. It does not block the write. Regex checks can match quoted examples,
comments, and valid compatibility code.

## Rule files

Rules are Markdown files with YAML frontmatter and a corrective instruction body.
For example, `.pi/rules/no-debug-print.md` can contain the following rule.

```markdown
---
description: "Debug print call. Use the project's structured logger."
condition: 'fmt\.Println\('
scope: "tool:edit(*.go), tool:write(*.go)"
interruptMode: always
---

Use the project's structured logger instead of fmt.Println in application code.
```

A blocking rule rejects the matching tool call before it runs. The model receives
the rule body in the blocked tool result and can submit a corrected call.
Set `interruptMode: never` for a reminder after a successful tool call instead.

| Field | Supported values |
|---|---|
| `description` | Optional short pattern summary for the TUI poke. Falls back to the first paragraph of the body. |
| `condition` | Regex string or list of regex strings. Any match triggers the rule. |
| `ttsr_trigger` | Legacy alias used when `condition` is absent. |
| `scope` | String with comma-separated tokens, or list of tokens. Defaults to `tool:edit, tool:write`. |
| Scope tokens | `tool`, `toolcall`, `tool:<name>`, or `tool:<name>(<glob>)`. |
| `interruptMode` | `always` and `tool-only` block. `never` sends a passive reminder. Defaults to `always`. |
| `repeatMode` | `once` only. Delivery is once per agent run, not once per saved session. |
| `enabled` | `false` disables this rule and shadows lower-priority rules with the same filename stem. |

Regex conditions support lookarounds and leading inline flags such as `(?i)` and
`(?m)` through the existing `regexp2` dependency. Each regex has a 50 ms match
timeout, checked on the engine's clock. Timed-out matches count as no match.

Path globs use Go `path.Match` syntax. A glob without a slash matches the basename,
so `*.go` includes nested Go files. A glob with a slash matches the normalized path
supplied to the tool. Recursive `**` and brace expansion are not supported.

Discovery checks these directories in order for each ancestor, starting at the
session working directory and ending at the nearest `.git` marker or filesystem root.

1. `.pi/rules`
2. `.agents/rules`
3. `.omp/rules`
4. `.cursor/rules`

The active agent profile's `rules` directory follows project directories.
Embedded rules have the lowest priority. Files ending in `.md` or `.mdc` load in
filename order. The first accepted rule with a given filename stem wins.
Ordinary instruction files without streaming conditions do not load as streaming rules.
Malformed rules produce a warning on stderr and do not prevent startup.

## Settings

The `ttsr` object in the effective settings controls this port.

```json
{
  "ttsr": {
    "enabled": true,
    "builtinRules": true,
    "disabledRules": ["go-rand-v2"]
  }
}
```

`enabled: false` disables discovery and matching. `builtinRules: false` disables
only embedded rules. `disabledRules` excludes names from every source.
Settings and rule files reload at the start of each agent run.

## Runtime contract

The hidden `streaming-rules` extension owns matching in ordinary interactive,
print, JSONL, and RPC sessions. It remains active with `--no-extensions`, like the
other hidden application extensions. The separate durable workdesk and built-in
child sessions do not install this extension.

The extension checks decoded tool-argument snapshots during streaming and checks
final arguments again at the awaited `tool_call` boundary. A stream match alone
does not interrupt generation or deliver a reminder. Final arguments are authoritative.
This avoids canceling a tool because of an incomplete argument or a discarded
provider attempt. Large streamed snapshots are sampled after 25 percent growth.
Final checks are exact and do not use that throttle.

For `write`, matching sees `content`. For `edit`, it sees each `newText` separately,
not `oldText`, the existing file, or unrelated replacements joined together.
Other explicitly scoped tools match their serialized arguments. Read results and
assistant prose are not monitored. Calls inside codemode pass through the same
`tool_call` and `tool_result` hooks. The codemode script itself is not matched.

Advisory reminders enter the model context as hidden `streaming-rule-reminder`
custom messages at a successful turn boundary. They are persisted with the rule
names. The original tool result is unchanged. Failed tools and aborted turns do
not deliver advisory reminders. Multiple matches of a rule produce one delivery
per run, including concurrent calls. A blocking rule also spends its one delivery,
so a model that ignores the correction can repeat the call. These rules advise
quality rather than enforce access control.

Each delivery adds a distinct `◇ Rule poke` card to the TUI transcript.
The card shows `Reminder sent` or `Tool call blocked`, followed by each rule's
name and a concise pattern summary. For example, `go-ioutil` shows
`Deprecated io/ioutil import. Use io and os.` These are custom cards, not warning
notifications. Summaries use the rule's `description` or its first body paragraph,
with whitespace collapsed and a limit of 160 Unicode characters.

Pokes persist as display-only `streaming-rule-poke` custom entries and render
again when the session resumes. They never enter model context or create an
extra model turn. Print mode stays silent. JSONL and RPC clients receive the
standard `entry_appended` event, not a `notify` event. Pokes follow the existing
once-per-run delivery policy. Incomplete stream matches, failed tools, and
aborted turns do not create advisory pokes. A call that was actually blocked
keeps its poke even if the turn later aborts. Full advisory instructions remain
hidden. A blocked tool's existing error result still contains its correction.

## Differences from oh-my-pi

This port does not include mid-stream abort and retry, prose or thinking scopes,
AST conditions, model-judged questions, agent filters, global path gates, repeat
gaps, `rule://` URLs, or the `/ttsr` command. A streaming rule that uses unsupported
`astCondition`, `question`, `agents`, `globs`, scope, or repeat metadata is rejected
with a warning rather than applied more broadly than intended. Only the three
settings documented above are consumed.

## Verification

`make smoke` runs `scripts/smoke-test-streaming-rules.py` against the built binary
with an isolated profile and a loopback OpenAI-compatible model. It verifies the
next provider request and actual filesystem effects for bundled reminders,
custom blocking rules, nested codemode calls, disabled rules, and removal of an
existing anti-pattern. It also checks persisted reminders and once-per-run delivery.
The `--tui` scenarios check painted poke cards and pattern summaries after a
forced redraw in both fullscreen and regular TUI modes. They cover blocking,
nested codemode calls, persisted display-only entries, and session resume.
