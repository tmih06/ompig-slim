# Usage reference

Full command, key, and slash-command reference for `ompig-slim`. The README has the short version.


```sh
cd ~/code/thing && ompig-slim    # then /login, or export OPENAI_API_KEY
```

| Command | What it does |
|---|---|
| `ompig-slim "list the failing tests"` | start interactive with an opening prompt |
| `ompig-slim -p "review this diff"` | print one answer and exit |
| `ompig-slim --mode json` / `--mode rpc` | JSONL event stream, or an RPC loop over stdio |
| `ompig-slim -c` / `-r` / `--session <id>` | continue the last session, pick one, open by id |
| `ompig-slim --model openai/gpt-4o:high` | pick `provider/id` with a thinking level |
| `ompig-slim --model @review` | run on a saved [model role](model-roles.md) |
| `ompig-slim --list-models` | refresh and list available models, including [custom models](custom-models.md) |
| `ompig-slim @src/a.ts "explain"` | attach files or images to the prompt |
| `ompig-slim --tools read,grep,find` | restrict the tool set for one run ([selection rules](tool-selection.md)) |

Model lists refresh automatically. Codex and Codex JSON use the signed-in accounts'
provider roster. Other supported providers learn new models from the public catalog.
Interactive startup uses cached lists while discovery runs in the background.
Use `--offline` to skip discovery or `update --models` to force a refresh.
See [model discovery and offline behavior](model-discovery.md).
ClinePass does not list `cline-free/` promotions reserved for Cline's IDE extension and CLI.
The agent receives its selected provider and model in context; model changes are
appended to the conversation rather than rewriting its cached prefix.

| Key | Action |
|---|---|
| `Ctrl+O` | expand or collapse tool output |
| `Ctrl+P` | cycle to the next model in the role cycle |
| `Alt+P` | model picker for this session |
| `Ctrl+Shift+Left` | cycle to the previous model |
| `Ctrl+-` | undo an editor change |
| `Ctrl+V` | paste from the clipboard |
| `Shift+Tab` | cycle thinking level |
| `Ctrl+Q` | queue a follow-up behind the current task |
| `Alt+Q` | restore queued messages to the editor |
| `Alt+W` | fold the task/quota widget to its border control |
| `Alt+A` | Agent Hub (live subagents) |
| `↓` on an empty prompt | open the subagent tree below the status line; `↑`/`↓` select, `Enter` inspect, `Esc` close |
| `Esc` | interrupt |

Default shortcuts are the same on every OS. Your `~/.pi/agent/keybindings.json`
overrides still apply. macOS calls the Alt key Option. Terminal key mappings can
intercept shortcuts before the application receives them. Ctrl+Z requests process
suspension; native Windows reports that suspension is unsupported.

Mistral browser sign-in supports multiple accounts and displays the account email when available.
Repeat `/login` for each account; list them with `ompig-slim auth accounts --provider mistral` and
switch with `ompig-slim auth use mistral '<account-key>'`. See [Mistral account identity](mistral-account-identity.md).

Slash commands: `/login` `/model` `/settings` `/usage` `/goal` `/subagents` `/compact` `/tree` `/fork` `/resume`
`/export` `/hotkeys` `/quit`. On interactive exit, `/quit` leaves the logo, session message and tool-call
counts, token usage, and recorded cost in your terminal. Saved sessions include a resume command.
Use `/goal <objective>` (or `/goal set <objective>`) to start working autonomously;
`/goal pause`, `/goal resume`, `/goal complete`, and `/goal drop` control it. An active goal continues on
its own until you stop it, and a goal that reports the same blocker three times pauses itself and asks for
the missing input ([goal contract](goal.md)). Interrupting with `Esc` or `/goal pause` takes effect
immediately and starts no further turn, and an ordinary follow-up afterwards does not resume the objective:
only `/goal resume` does, and it restarts the same paused objective. `ompig-slim --help` lists every flag.

## Session history and compaction

Resuming a session initially loads its retained context. Scroll upward in fullscreen
mode to load the older transcript across compaction boundaries. Display history
does not expand the model context.

Idle checkpoints run before the selected model's estimated cache expiry when
there is enough history to compact. This is a paid summary request, not a guarantee
of a cache hit or lower cost. `compaction.idleEnabled: false` disables the idle
policy. See [transcript history and idle compaction](compaction.md) for settings,
provider metadata requirements, and regular-mode behavior.

## Durable headless work

Daily interactive usage is unchanged. For unattended work with crash recovery:

```sh
ompig-slim durable run --state /data/workdesk --cwd /work/repo \
  --model provider/model --request-id ticket-123 'Implement and verify the change'
ompig-slim durable resume --state /data/workdesk
ompig-slim durable status --state /data/workdesk
# Optional committed transcript events and live steering:
ompig-slim durable run --state /data/workdesk --cwd /work/repo \
  --model provider/model --request-id ticket-123 \
  --events-file /data/events.jsonl --control-file /data/control.jsonl \
  'Implement and verify the change'
printf '%s\n' '{"content":"Also inspect the failing test"}' >> /data/control.jsonl
```

Persist both directories across sandbox restarts. This opt-in mode uses PiG
Durable and its native coding tools, not the daily agent's extensions or session
format. See [durability guarantees and limits](durable.md).

## For other tools

Every session publishes what the agent is doing, so a multiplexer, status bar, or notifier can watch
a run without knowing anything about this program:

```sh
# one document per running agent, rewritten on each change
ls  <PIG_HOME|~/.pi>/state/agent/*/status.json
cat <PIG_HOME|~/.pi>/state/agent/$$/status.json
# {"state": "working", "agent": "ompig-slim", "mode": "tui", "session_id": "...",
#  "session_file": "...", "cwd": "...", "pid": 1234, "updated_at": "..."}
```

`state` is `idle`, `working`, `blocked` (with the dialog in `detail`), or `exited`. Check
`mode == "tui"` if you only care about interactive agents. The files are written atomically; treat a
missing one, or one whose `pid` is no longer running, as "no information" rather than an error.

The [Herdr](https://herdr.dev) adapter is an installable extension built on that file, not part of the
binary:

```sh
ompig-slim install ./extensions/herdr
```

It gives Herdr a sidebar row, tab and workspace rollups, `herdr agent wait`, and session restore after
a Herdr server restart.
