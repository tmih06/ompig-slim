# Model roles and recovery

The native Go port follows OMP's split between **persistent model management**
and **session-only model selection**:

- `/model` opens the fullscreen manager.
- Alt+P opens the compact session-only picker; Ctrl+L remains a shortcut alias.
- Type `@review` in that picker to select a logical role for this session.
  `--model @review` and subagent `model: "@review"` also resolve roles.
  `pi/review` is the legacy spelling; `*` means `@default`.

## Fullscreen manager

The sidebar contains All, Recent, Roles, and providers. Locked providers remain
visible and offer login. Provider refresh is debounced; F5 explicitly reloads
credentials and catalogs. Search is shared with the embedded assignment browser.
Model details include available context, pricing, reasoning, and performance facts.
Kind tabs filter the catalog and assignments. The bundled roster is imported
from OMP's `@oh-my-pi/pi-catalog@18.14.2`: 5,949 model identities across chat,
image, tiny, judge, search, TTS/STT, embedding, rerank, and video kinds. OMP-owned
providers replace the older Pi catalog instead of merging obsolete rows and
synthetic OpenRouter variants. Explicit user/extension models and authenticated
native-only providers remain available. Assigning a role does not install weights
or implement a provider transport; non-chat descriptors never become chat models.

In Roles:

| Key | Action |
| --- | --- |
| Enter | Assign/replace the primary or highlighted fallback |
| n | Create a custom role |
| f (also a) | Append an ordered fallback |
| x (also d) | Unassign a primary or remove a fallback |
| Shift+D | Delete a custom role after choosing a settings layer and confirming |
| [ / ] | Reorder a fallback or move a role in the cycle track |
| t | Open the inline effort selector |
| @ | In an assignment browser, swap the model list for `@role` rows |
| c | Toggle a chat role in Ctrl+P cycling |
| k | Edit the highlighted model's compaction threshold |
| Escape | Back out of the current strip, assignment, search, or overlay |

Assignment uses inline role, global/project scope, and effort strips. Kind roles
reject incompatible models. Either assignment step can also pick a role instead of
a model: type `@` in the browser to swap the list to `@role` rows, the same
gesture the Alt+P picker uses, and backspace to return to models. Assigning a
primary lets one role follow another (`task` → `@default`); appending a fallback
lets one role reuse another's recovery plan. The saved value keeps the alias, so
repointing the referenced role moves the follower with it instead of pinning the
model it resolved to at assignment time. The Roles list renders that assignment as
`@role` in the referenced role's own color rather than showing the model behind it,
because the alias is what was chosen; the referenced role's own row still shows its
model.

Roles that would close a cycle are not listed, in either step, so every offered
choice saves successfully. Primary assignment follows `modelRoles` aliases and the
built-in inheritance, so `task → @default` and then `default → @task` is refused at
the second step rather than leaving both roles unresolvable; chain destinations
follow chain references instead, and a role that already reaches the chain being
written is hidden for the same reason. Role tags supply labels, colors, and
visibility. Mouse navigation, scrolling, hover, provider rows, and kind tabs are
supported.
Changes persist immediately; closing the manager does not discard them.

To delete a custom role, highlight its primary row and press **Shift+D**. The
confirmation starts on Cancel; select **Delete @role (global)** or **(project)**
and press Enter. Escape cancels. Deletion removes that layer's assignment, tag,
explicit fallback chain, and cycle entries in one settings write; other layers
and the live selected model remain unchanged. A role defined in both layers
must be deleted from each to disappear completely. Built-in roles cannot be
deleted. If another saved assignment or fallback explicitly references the role
(`@role` or `pi/role`, including effort suffixes), reassign that reference first;
deletion reports the dependency instead of leaving a broken alias. Project
writes require trust. Lowercase **x/d** still only clears an assignment.

Assigning Default switches the live session after credential validation. Clearing
it can expose an inherited assignment. Project writes require a trusted project.
Role assignment scope and `modelRoleStorage` do not change the global scope used
by the manager's fallback-chain and cycle-order controls.

Ctrl+P follows the configured role cycle. Removed custom-role names are ignored.
If a nonempty saved cycle contains only removed roles (for example old
`scoped-N` entries), it recovers to explicitly assigned chat roles, preferring
Smol, Default, and Slow; if none are assigned it uses the built-in default cycle.
An explicit empty `cycleOrder: []` still disables cycling, and a valid explicit
cycle never gains other roles just because its models are unavailable. Recovery
is read-only; toggling **c** in Roles saves the displayed effective order.

The footer below the prompt names the active role beside the model, as
`@review claude-sonnet-4`. Two roles may resolve to one physical model, so
cycling or fallback can leave the model name alone unable to say which role is
selected. The role carries the same color it has in the model manager and
catalog, including a `modelTags` override: both read one source, so a role never
shows two hues. That source is the catalog's theme, not the transcript's. On the
stock `dark` and `system` themes those are two palettes — the catalog paints with
OMP's Titanium colors while the transcript keeps PiG's stock ones — so taking the
transcript's would show every role in a different color than the manager. An
explicit or light theme uses one palette for both and is unaffected. A color the
theme does not define falls back to the footer's own hue rather than failing to
render. A session with no role — a direct model selection, or a virtual model —
shows the bare model name as before.

### Session picker

The bottom-sheet picker changes the current session, not a persistent role.
Leading `@` searches quick roles. Tab toggles the Task-only target; selecting that
target changes the task override without switching the main model. Choices that
cannot fit the existing context are marked before selection. Closing/cancelling
the picker restores the editor and does not write an assignment.

### Roles are the only model-selection system

The legacy scoped-model feature is removed, not migrated. `enabledModels` in old
settings is ignored; opening the app or manager never creates `scoped-N` roles.
The old CLI `--models` selection flag and SDK/session scope APIs are removed.
Use `--model @role`, `modelRoles`, and `cycleOrder` instead. Already-created
custom roles remain ordinary roles and can be deleted with Shift+D.

Extensions must remove calls to `getScopedModels`/`ScopedModels` and old
`scopedModels` session options. RPC cycle results no longer include `isScoped`.
Read the runtime catalog for model availability; choose a model or role explicitly. `ompig-slim update --models` is unrelated and
still refreshes catalogs.

## Configuration

Global settings are in `~/.pi/agent/settings.json`; trusted project overrides are
in `.pi/settings.json`, unless an agent directory override is supplied.

```json
{
  "modelRoles": {
    "default": "openai/gpt-4o",
    "review": "openai/gpt-4o:high",
    "fast": "openai/gpt-4o-mini:off",
    "task": "@default"
  },
  "modelTags": {
    "review": { "name": "Reviewer", "color": "accent" }
  },
  "cycleOrder": ["fast", "default", "review"],
  "modelRoleStorage": "global",
  "retry": {
    "modelFallback": true,
    "fallbackOn": "any",
    "fallbackChains": {
      "default": ["openai/gpt-4o-mini:off"],
      "review": ["@slow", "openai/gpt-4o-mini:off"],
      "fast": []
    }
  }
}
```

The built-in role names are `default`, `smol`, `slow`, `vision`, `plan`, `commit`,
`tiny`, `memory`, `task`, `advisor`, `widget`, `image`, `web`, `speech`, `dictation`,
and `judge`. Custom names start with a letter and may include digits, `_`, and `-`.
Unassigned roles resolve through the available catalog and built-in priorities;
missing task/plan assignments inherit Default. A role is distinct from its
physical model, even when several roles select the same model.

Selectors support role aliases, effort suffixes (`off`, `auto`, `minimal`, `low`,
`medium`, `high`, `xhigh`, `max`, `inherit`), and aggregator `@upstream` routing:

```json
{
  "modelRoles": {
    "review": "@slow:high",
    "routed": "openrouter/anthropic/claude-sonnet-4@anthropic:high"
  }
}
```

Alias cycles are errors. A role may alias another role (`task: "@default"`), which
keeps it following that role instead of copying its model; the manager refuses an
assignment that would close a loop. Literal catalog IDs containing `@` win over
routing parsing. Explicit upstreams are carried through requests, refresh, session
audit, and resume; unsupported direct providers reject routing rather than ignore it.

Project role entries overlay global entries independently. Tag properties merge
individually, including explicit `hidden: false`. An empty `cycleOrder` disables
role cycling; absent order uses the built-in default. Model changes record the
logical role, so fallback, cycling, branching, and resume do not infer it from a
physical model shared by multiple roles.

### Custom-model thinking capabilities

Custom definitions inherit declared thinking tiers from a matching bundled model
on the first offline launch, even without `models-metadata.json`. Once fetched,
matching public tiers take precedence over that seed and persist for offline
restarts. Version/variant matching prevents borrowing an unrelated model's ladder;
no manual `thinkingLevelMap` is needed for a recognized model.

Explicit `reasoning: false`, `thinkingLevelMap`, and endpoint compatibility
settings remain authoritative. In particular, `compat.supportsReasoningEffort:
false` prevents inheriting effort tiers and suppresses `reasoning_effort` on the
wire; fetching catalog data never turns it on. Enable it only for an endpoint
that accepts that parameter. For a compatible custom DeepSeek V4.1 Flash endpoint,
the canonical positive tiers are **Low / High / Max**, not generic Minimal/Medium;
Off omits the effort parameter rather than borrowing another host's disable value.

### Auto thinking and compaction

`auto` keeps the configured selector separate from the concrete provider effort.
Before a prompt, a Judge model classifies the task into the local three-tier
rubric, with a four-second total budget. Failure retains the last concrete effort;
single-tier models need no classification. Judge requests are normal billable
requests and their usage is recorded outside conversation context. The default
ceiling is `xhigh`; `providers.autoThinkingMaxEffort: "max"` opts into Max.

The manager's `k` accepts scaled token bases (`100k`), fixed triggers (`f100k`),
and percentages (`80%`). Empty input resets the
model override. Extended-window thresholds require a second unchanged submission
when the registry can resolve an extended window. The preview and auto-compaction
trigger use the persisted threshold, not just a display preference.

## Fallback recovery

`retry.fallbackChains` maps a role or selector to an ordered tail. Precedence is
exact model/effort, longest matching provider/prefix wildcard, explicit role hint,
then matching/default roles. Explicit `[]` prevents default-chain inheritance.
Non-chat kind roles never inherit chat fallbacks. A `provider/*` destination keeps
the failing model ID while changing provider; missing, context-incompatible,
cooling-down, and credential-ineligible candidates are skipped.

A tail entry may name another role (`@slow`, `pi/slow`, or `*`), with an optional
effort suffix. The role destination contributes its model first and then that
role's own chain, depth-first and in the order written, so one role reuses
another's recovery plan instead of copying its models. The saved entry keeps the
role name: repointing the referenced role moves the destination with it. The
failed hop stays attributed to the role that owned the chain, so a role
destination routes recovery and does not switch the logical role.

Role references are acyclic. A write is refused when a role it names already
reaches the chain being written, so `review: ["@slow"]` followed by
`slow: ["@review"]` fails at the second step instead of saving undefined order;
the manager hides every such role, so accepting a listed destination always
succeeds. The check reads the merged authored chains, which is why a project chain
that closes a loop through a global one is refused whichever layer completes it.

A loop that predates the guard — hand-edited config, or a self reference inherited
from a `default` tail a role is itself named in — still terminates: a role is
expanded at most once per chain walk, so it shortens the candidate list rather
than recursing. Inherited tails are not checked at write time because refusing
them would make an unchanged chain unsaveable and block the edit that removes it.

Recovery retains logical role and auto-thinking state, checks destinations before
asking permission to use a quota reserve, and never wraps a chain. Starting at a
chain tail still permits progress after that tail. Signed-thinking failures defer
unsafe cross-model switches until the next prompt. Failed attempts stay in audit
history but are excluded from outgoing context.

Missing credentials can prewalk a chain before a request. Quota-aware prewalking
uses the relevant provider's managed-account snapshot; ambient keys and unknown
health are not treated as exhausted managed accounts. Account rotation and account
blocks are separate from model fallback. Reserve confirmation is interactive when
policy requires it. Enable `retry.waitForUsageReset` to wait for a short reset.

Failed selectors cool down by reason. Before later prompts, eligible primaries
are restored when fit, credentials, and usage health allow. Default cooldowns are
30 minutes for quota, 30 seconds for rate limits, 5 seconds for concurrency,
45 seconds plus jitter for capacity, 20 seconds for server errors, and 5 minutes
for unknown failures. Explicit provider delays take precedence.

Same-model retry defaults are ten retries with a 500ms exponential base and an
8-second exponential cap. The maximum wait is 300000ms. `retry.enabled: false` or
`retry.modelFallback: false` disables chain recovery. `retry.fallbackOn` accepts
`any`, `usage-limit`, and `except-usage-limit`. Cancellation does not trigger chain
walking; context overflow remains compaction's responsibility.

## Catalog freshness and offline recovery

New chat model IDs on supported provider APIs do not require a binary update.
The runtime reads OMP\'s public `https://catalog.stencil.so/models.json.zstd`
source (plain JSON mirrors are also accepted), independently of authenticated
provider discovery. The bundled catalog is a cold-start/offline seed, not the
only list the runtime can select from.

- Startup restores `models-metadata.json` immediately, before selection; network
  work runs in the background. Interactive sessions revalidate every two hours,
  retry after ten minutes on failure, and support forced F5 refresh.
- Conditional ETag requests avoid downloading unchanged data. Valid snapshots
  are written atomically. Network errors, malformed/empty responses, and offline
  mode keep the last usable snapshot and bundled seed.
- Shared-source rows can add IDs with affirmative tool support, text input, and
  positive context/output limits. New IDs retain published limits, pricing,
  modalities and reasoning tiers. Known seed rows keep curated provider metadata
  under OMP\'s additive shared-source precedence; endpoint/native discovery owns
  its own membership and overrides. User model definitions are applied last.
- Public data cannot supply inference endpoints, headers, credentials, arbitrary
  APIs, compatibility code or executable content. Trusted provider transport and
  public-source aliases stay in the binary. Account-scoped providers use their
  native manager rather than an unauthenticated global roster.
- Refreshing the catalog does not switch the current model or rewrite roles.
  Cached additions remain selectable after restart without network access.
  Schema-1 metric caches are retained offline and upgraded with an unconditional
  source fetch when online, so an old ETag cannot block discovery indefinitely.

A genuinely new protocol, authentication flow, provider-specific compatibility
rule or inference backend can still require a binary update. Data refresh is not
remote code loading. `PI_OFFLINE=1` disables native network discovery and periodic
refresh; it does not prevent the cached catalog from being used.

## Validation

`make check` covers the vendored runtime as well as the entry point. Tests include
layered role settings, retired-option rejection, hub/picker state transitions, auto-thinking,
missing-auth and signed-thinking recovery, an actual local SSE quota fallback
chain with primary restoration, and routed request/resume behavior.
`make smoke` includes a real PTY menu flow in
`scripts/smoke-test-model-menu.py`; it uses local fixtures rather than paid API
calls. No live-provider parity result is implied by those offline tests.

Catalog tests additionally traverse every imported provider and assert that the
composed browser retains all 5,949 provider/model/kind identities, including the
61 OpenAI chat models. Display ordering follows OMP's built-in role rank, MRU,
provider priority, version, and revision rules. The reference comparator accepts
`--unfiltered` to keep all native local/web/provider catalogs. The small golden
fixture instead disables unrelated providers in both actual runtimes.
Provider intelligence/performance metadata may refresh independently of the
pinned bundled roster. The comparator can seed both profiles with the same public
`--metadata-cache scripts/testdata/omp-openai-public-metadata.json` snapshot
without filtering the model roster. Run with `--unfiltered --openai` to inspect
the actual OpenAI, local, and web lists. OMP may refresh its public catalog even
with `PI_OFFLINE=1` (that variable controls the native Go runtime only).

Visual regression tests use actual OMP 18.14.2 PTY captures at 120×40 and 80×24
for the manager, provider scope, Roles, assignment browser, session picker,
quick-role picker, and Task picker. `scripts/compare-model-tuis.py` boots both
CLIs against isolated loopback-only fixtures and checks display cells plus
nonblank-cell foreground/background/bold. It requires optional Python `pyte`;
see `tui/testdata/README.md` in the vendored runtime for reproduction.

Stock dark/system model overlays use OMP's Titanium palette; explicit custom
and light themes remain authoritative, and transcript colors are unchanged.
