# Model author

An agent's commits can credit the model that wrote them, so the model appears as
a contributor. When the feature is on, the harness rewrites `git commit` in the
agent's bash calls. The model is not asked to remember anything and its prompt
does not change.

The account is chosen by **model ID**, never by provider, so Claude reached
through OpenRouter, Bedrock, or a custom endpoint is treated the same as Claude
through Anthropic.

For `claude-opus-5-5` the command

```sh
git commit -m "fix parser"
```

runs as

```sh
git commit --author='Claude Opus 5.5 <noreply@anthropic.com>' --trailer='Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>' -m "fix parser"
```

The author is the model, so the commit counts toward its contribution graph. The
committer stays the configured git user. The name is the model's display name
without catalog annotations such as `(latest)`, falling back to the model ID.

## Accounts

Each identity below was looked up on GitHub and is the vendor's own account.

| Model ID matches | GitHub account | Commit email |
|---|---|---|
| `claude` | [`claude`](https://github.com/claude) | `noreply@anthropic.com` |
| `gemini` | [`gemini-code-assist[bot]`](https://github.com/apps/gemini-code-assist) | `176961590+gemini-code-assist[bot]@users.noreply.github.com` |
| `gpt-*`, `codex`, `o1`, `o3`, `o4` | [`codex`](https://github.com/codex) (OpenAI) | `267193182+codex@users.noreply.github.com` |
| `swe-*`, `devin` | [`devin-ai-integration[bot]`](https://github.com/apps/devin-ai-integration) | `158243242+devin-ai-integration[bot]@users.noreply.github.com` |

DeepSeek, GLM (Z.ai), Qwen, Kimi (Moonshot), MiniMax, Mistral, Grok, and Llama
have no listed account. Their vendors publish only organization accounts, and an
organization cannot author commits. A model with no entry is left alone: its
commits use the repository's configured identity. Add an entry in
`internal/modelauthor/author.go` only after confirming that the account exists
and belongs to the vendor.

## Configuration

The feature is off by default. Turn it on for every session in `settings.json`:

```json
{ "modelAuthor": { "enabled": true } }
```

`/model-author on|off|status` overrides the setting for the current process.
The command does not write `settings.json`. The model is read at each command,
so switching models mid-session changes the result immediately.

## What is rewritten

The hidden `model-author` extension edits the `command` of each `bash` tool
call in its `tool_call` handler. It finds `git commit` at a command position,
including after git's own options such as `-C dir` and `-c key=value`, and
leaves quoted strings, comments, heredoc bodies, and commit messages that
mention "git commit" alone.

A flag the agent already supplied is kept: an explicit `--author` is never
replaced, and a `Co-Authored-By` trailer for the same identity is not repeated.
`--trailer` needs git 2.32 or newer; on older git the commit fails with git's
usage error.

Limits: only the `bash` tool and tool calls made through it are rewritten. A
commit made by a script, a git alias, `sh -c '...'`, or any other tool is not.
The durable worker does not install this extension.
