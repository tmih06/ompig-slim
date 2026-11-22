# Features

## What's inside

| Area | Detail |
|---|---|
| Tools | `read` (files, dirs, URLs, GitHub issue/PR links and `issue://N` / `pr://N` with automatic repository detection), `bash`, `edit`, `write`, `grep`, `find`, `web_search`, `codemode`, `todo` |
| Prompt | Completed slash commands use distinct colors; suggestions match purpose keywords (`/clea` finds `/new (clear, fresh)`) and an unknown command names its closest match (`Did you mean /new?`); `/skill:` badges compose skills in prompts and `/goal` objectives; `/skill:` loads and `@path`/`@url` mentions are read up front and shown as a Read tree under the prompt |
| Loop guard | [Corrective steers](loop-guard.md) after five identical tool-call turns or three recurring deterministic failures on the same target |
| Streaming rules | [Code-pattern checks](streaming-rules.md) with 34 bundled correctness, security, and performance reminders plus custom Markdown rules that can block tool calls before execution |
| Commit author | [Model author](model-author.md) rewrites the agent's `git commit` to author as its own model, chosen by model ID, off by default; enable with `/model-author on` or the `modelAuthor.enabled` setting |
| Models | ~5,900 IDs across 90 catalog providers, OAuth, API keys, custom endpoints; [Gemini Decisions judge inference](gemini-decisions.md) |
| Routing | Roles, fallback chains for chat, judge and compaction, auto thinking, `/model` manager, `Alt+P` picker |
| Accounts | Multiple accounts per provider, single-account `/logout` picker, quota-aware routing, imported credentials, rate-limit resets |
| Subagents | `Agent` with fixed `@task`, `@smol` and `@slow` [roles](model-roles.md#subagent-roles-and-tools), `get_subagent_result`, `steer_subagent`, workflows, Agent Hub, [explicit child outcomes](subagents.md) |
| TUI | omp-style tool cards, task/quota widget, todo reminder, Mermaid diagrams |
| Integrations | Publishes agent state for other tools; Herdr is an installable extension; [OpenCode Free](opencode-free.md) is built in |
| Themes | Built-in `system`/`dark`/`light`, colorful Markdown; [100 optional palettes](../themes/README.md) |
| State | Pi layout: `~/.pi/agent` global, `.pi` per project, so existing sessions carry over |

Account assignments stay sticky per provider, session, and model across restarts.
Broker clients share assignment state. See [account routing](account-routing.md).

Codex routing prefers eligible, unblocked Free accounts before paid accounts, even when Plus is selected.
Fresh usage reports identify plans when tokens omit them or contain an older plan.
New assignments prefer the account with the most quota left in its shortest model-relevant rolling window.
Offline or failed usage probes retain token-based plan checks and round-robin assignment.

Codex account labels and usage views prefer the access-token email, including for existing JSON imports,
so they match `/usage reset`. Stored email metadata is the fallback when the token has no email.

Codemode and other tools run with your user permissions; codemode is not an OS sandbox. Credentials live
in local SQLite, or in the configured broker’s SQLite vault, unencrypted at rest.
