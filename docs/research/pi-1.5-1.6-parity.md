# Pi 1.5 and 1.6: ompig-slim parity investigation

This is a risk-focused source comparison, not a complete verification of every
release-note bullet. Findings labeled BUG are code-level findings unless a
reproduction is explicitly stated; they are not all end-to-end reproductions.
No real credentials or user state were used.

## Pi 1.5 (1.5.0 / 1.5.1 / 1.5.2)

### Sources, baseline, and reading of "relevant"

Authoritative primary sources used:

- Pi's own release pages: <https://pi.dev/changelog/releases/1.5.0>,
  <https://pi.dev/changelog/releases/1.5.1>,
  <https://pi.dev/changelog/releases/1.5.2>. The supplied
  `https://pi.dev/changelog/releases/` index URL 404s; the versioned pages and
  <https://pi.dev/changelog> are the canonical published route.
- Upstream GitHub releases, which mirror the same notes and give exact tag
  commits: `v1.5.0` `f9a2636`, `v1.5.1` `dd30101`, `v1.5.2` `b4afddd`
  (<https://github.com/earendil-works/pi/releases/tag/v1.5.0> etc.).
- The GitHub compare API for `v1.5.1...v1.5.2`, used to resolve one otherwise
  unnumbered item: `claudeCodeVersion` moved from `2.1.280` to `2.1.322` in
  `packages/ai/src/api/anthropic-messages.ts`.

Local baseline: `ompig-slim` HEAD `b75622d`; `go.mod:10` requires
`github.com/MichaelKinsy/PiG v0.7.0`. The maintained vendored tree declares Pi
1.5.0 / `f9a2636` as its behavior pin
(`vendor/github.com/MichaelKinsy/PiG/README.md:20,108`). `main.go:61,71,127`
delegates everything not owned by this repository to that vendored PiG CLI, so
PiG code is the effective implementation for TUI, tools, sessions, models, and
RPC in this binary. `docs/research/pi-durable.md` records the same pin.

Confidence legend used below:

- **High** - code path read directly (and, where marked "reproduced", a local
  check reproduced the upstream failure mode).
- **Medium** - vendored PiG changelog explicitly claims the fix and no
  contradictory code was found, but no targeted runtime reproduction was run.
- **Low** - plausibly relevant, but the upstream description does not map
  cleanly to this Go port.

Status legend: **BUG** = the same failure mode is supported by source inspection (not necessarily reproduced end to end); **MISSING** =
feature/config/default absent (not itself a bug); **PORTED** = fix present in
the vendored code; **OWNED** = this repository deliberately owns the behavior;
**N/A** = TypeScript/npm/Windows/terminal upstream surface with no equivalent
in this Go binary.

**Bottom line.** The answer to "do we have these problems?" is mixed. Many 1.5.0
fixes are already in the vendored PiG tree. Several 1.5.1 fixes are not: the most
concrete are the grep glob-relative-to-`path` bug, `find`/`ls` `limit` handling,
whole-file reads, prompt-template quote parsing, edit-tool line-ending rewrites,
the empty `tools: []` OpenAI-compatible payload, and `tool_search` not following
local `$ref`s. 1.5.2's fork-with-missing-cwd feature and import-copy cleanup are
also absent. These are distinguished from genuinely *missing* 1.5.1 features
(Anthropic message threads, the richer Mermaid renderer, and three new `compat`
knobs). Package-update summaries and cumulative cache-warming notices have since
been ported with regression coverage below. The idle cache-warming default
remains a deliberate configuration divergence, not a confirmed defect.

### A. Confirmed still-present bugs (highest-confidence findings)

| Pi item | Status and observable effect | Evidence | Confidence |
|---|---|---|---|
| 1.5.1 Fixed: grep `glob` matching from launch folder instead of `path` (#11807, #10998) | **BUG.** PiG passes the caller's `--glob` verbatim while searching an absolute `searchPath`; the upstream example (`path: "repo"`, `glob: "migrations/main/*.js"`) returns no matches. Reproduced locally with `rg --json --glob 'migrations/main/*.js' -- needle /tmp/.../repo` (no match) versus the `repo/...` prefixed glob (match). | `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/tools/grep.go:142` (path resolved), `:172` (`--glob`, unadjusted), `:174` (absolute path passed to rg). | High (reproduced) |
| 1.5.1 Breaking/Fixed: grep context from ripgrep; custom `ReadFile` no longer called for context (#9276, #11423) | **BUG / divergence.** PiG still opens matched files itself and calls `GrepOperations.ReadFile` for context; Pi 1.5.1 deliberately stopped doing this. Its windowed reader bounds memory (so the OOM is fixed), and lone CRs are counted as Pi counts, but the extension contract and remote-operation semantics still differ. | `.../tools/grep.go:39-68` (`ReadFile`, `openFile`), `:203` (formatter takes `openFile`); `.../tools/grep_line_window.go:10-20` (D151 divergence); lone-CR fix at vendored `CHANGELOG.md:458`. | High |
| 1.5.1 Fixed: find/ls `limit: 0` and exact-limit notice (#7121) | **BUG.** No `Math.max(1, limit)` clamp. `find` forwards `--max-results 0` to fd (empty output; locally confirmed with fd), and `ls` exits its loop immediately for `limit: 0`. `find` also reports a limit notice when the result count equals, not exceeds, the limit; `ls` checks for an additional entry before setting that notice. | `.../tools/find.go:93-96` (no clamp), `:117` (`--max-results`), `:214,221` (`>=` and notice); `.../tools/ls.go:130-133` (no clamp), `:154,183` (`>=` and notice). | High |
| 1.5.1 Fixed: read tool loading whole files (#9654) | **BUG.** PiG still reads the whole file into `[]byte`; `offset`/`limit` only select which decoded lines are shown. Large files can still exhaust memory, and `ReadOperations` has no `createReadStream`. | `.../tools/read.go:39-45` (`ReadOperations` has only `ReadFile`), `:252-264` (whole `data` passed to `readTextResult`), `:293-343` (`readLocalFile` allocates/grows to full size), `:351-425`. | High |
| 1.5.1 Fixed: prompt template arguments dropping apostrophes and merging words (#11861) | **BUG.** Any `'` opens a quoted run, so `doesn't` keeps the quote open and merges following words; `\"`/`\'` escapes are not handled. Pi 1.5.1 keeps apostrophes inside words and honors escapes. | `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/prompt_templates.go:34-65` (`ParsePromptArgs`). | High |
| 1.5.1 Fixed: edit tool rewriting untouched line endings / OOM after write (#8544) | **BUG.** One detected ending is restored over the whole file, so mixed CRLF/LF/lone-CR files have untouched lines rewritten. The line diff is computed *after* `WriteFile`, retaining the old OOM ordering. | `.../tools/edit.go:300-313` (read, normalize, write), `:326-327` (diff after write); `.../tools/edit_diff.go:21-50` (`detectLineEnding`, `normalizeToLF`, `restoreLineEndings`). | High |
| 1.5.1 Breaking: no `"tools": []` for OpenAI-compatible models with tool history (#11843) | **BUG.** PiG still sends an explicit empty array when a session has tool history and no active tools. Pi 1.5.1 omits the field by default (fixing vLLM 0.20+ 400s) and adds an opt-in compat flag; neither the default nor `requiresToolsWithToolHistory` exists here. | `vendor/github.com/MichaelKinsy/PiG/ai/openai.go:1255-1292` (`hasToolHistory`, `resolveRequestTools` returning `&[]oaiTool{}`); repo-wide search for `requiresToolsWithToolHistory` is empty. | High |
| 1.5.1 Fixed: `tool_search`/`searchTools()` not finding tools behind local `$ref` (#11803) | **BUG.** Search text descends `properties`, `items`, `anyOf`, `oneOf`, `allOf` but never resolves `$ref`/`$defs`, so MCP property names and descriptions behind local references are not searchable. Codemode's declaration renderer expands local refs; other reference-related cases were not verified. | `vendor/github.com/MichaelKinsy/PiG/coding/extension/builtin/toolsearch/toolsearch.go:54-78` (`schemaText`), `:88-93` (`CreateDocument`); contrast `vendor/github.com/MichaelKinsy/PiG/codemode/declarations.go:20-22,265,373`. | High |
| 1.5.2 Fixed: `/import` of a missing-cwd session leaves a duplicate copy (`s.jsonl` + `s-1.jsonl`) (#11975 / #11060) | **BUG.** `copyFileExclusive` writes the destination, then `AssertSessionCwdExists` can fail and return without removing it; a later import picks the next `-N` name. | `vendor/github.com/MichaelKinsy/PiG/coding/session_import.go:47-53` (copy), `:56-65` (open/assert/failure return), `:74-82` (`-N` selection). | High |
| 1.5.2 Fixed: Anthropic OAuth reporting an outdated Claude Code version | **VERSION MISMATCH.** PiG pins `2.1.280`; its effect on authentication was not tested; upstream 1.5.2 changed the stealth user-agent to `2.1.322` (verified in the GitHub compare patch). | `vendor/github.com/MichaelKinsy/PiG/ai/anthropic_client.go:21-23`; `v1.5.1...v1.5.2` diff for `packages/ai/src/api/anthropic-messages.ts`. | High |
| 1.5.1 Changed: `cacheWarming` default becomes `"idle"` | **DELIBERATE DIVERGENCE.** PiG implements idle warming (30-minute horizon) but this project retains `"streaming"` to avoid unsolicited idle refresh costs; users can opt into `"idle"`. The default and profitability policy remain covered by `TestCostPolicyPreservesProfitableCacheWarming`. | `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/settings.go:431-432,2142-2150`; `.../cache_warmer.go:29,376,455`; `docs/cost-audit.md:20`; `TestCostPolicyPreservesProfitableCacheWarming`. | High |

### B. Missing features / knobs / defaults (not confirmed bugs)

| Pi item | Status | Evidence | Confidence |
|---|---|---|---|
| 1.5.1 New Feature + Added: Anthropic message threads (`compat.supportsMessageThreads`, only new messages sent, full-request fallback) | **MISSING.** No thread identifier or thread state exists in the vendored Anthropic provider; requests build the full transcript. This changes request shape/cost, not correctness of an existing local path. | Repo-wide search for `supportsMessageThreads`, `messageThreads`, `threadId` in `*.go` returned no matches. Request construction: `vendor/github.com/MichaelKinsy/PiG/ai/anthropic.go`, `anthropic_client.go`. | High |
| 1.5.1 Added: `compat.promptCacheKeyMode` (`auto`/`enabled`/`disabled`) | **MISSING.** PiG has concrete prompt-cache-key sending (e.g. `ai/openai.go:1711-1712`, `ai/openai_prompt_cache.go`), but no per-model mode to disable it for proxies that reject the field. | No `PromptCacheKeyMode`/`promptCacheKeyMode` in Go source; compare `vendor/github.com/MichaelKinsy/PiG/ai/types.go` compat fields. | High |
| 1.5.1 Added: `compat.requiresNonNullAssistantContent` | **MISSING.** No configuration to send `""` instead of `null` for tool-only assistant messages on strict OpenAI-compatible endpoints. | Repo-wide search for `requiresNonNullAssistantContent` is empty. | High |
| 1.5.1 Fixed (breaking): omit empty `tools` — compat escape hatch | **MISSING** in addition to the BUG above: `requiresToolsWithToolHistory` is absent, so proxies that require the empty array after tool history cannot be accommodated. | Same as BUG row; `ai/openai.go:1255-1292`. | High |
| 1.5.1 New Feature + Added: `pi update --extensions` per-package version/commit summary and unchanged/skipped/warned lines (`result` progress events) | **PORTED.** Changed npm/Git packages emit `result` events; CLI output lists changed versions/commits and separately reports already-current, offline-skipped, or warning-only updates. | `vendor/github.com/MichaelKinsy/PiG/internal/packagemanager/package_update_operations.go`; `coding/cli/package_commands.go`; `TestUpdateReportsChangedGitPackage`, `TestRunPackageUpdatePrintsChangedPackageSummary`, `TestPrintPackageUpdateSummary`. | High |
| 1.5.1 Changed: consecutive cache warms shown as one cumulative `Cache warmed: $N` line | **PORTED.** Adjacent persisted/live cache-warm usage entries now update one cumulative themed notice; any intervening non-warm entry starts a new group. | `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_cache_warming.go`; `TestResumeGroupsAdjacentCacheWarmingUsage`. | High |
| 1.5.1 New Feature: shell `pi auth login/logout` and stored-account lifecycle | **PORTED.** Login supports explicit API keys and browser preference; account commands own opening/closing their credential store, while shared runtime stores retain their caller's ownership. SQLite tombstones, token rotation, logout races, and legacy migration are covered without real credentials. | `vendor/github.com/MichaelKinsy/PiG/coding/cli/auth_command.go`, `auth_login.go`, `auth_accounts.go`; `ai/sqlite_auth_storage.go`; `TestAuthLoginBrowserPreferenceSelectsAlternativeWithoutPrompt`, `TestAuthLoginTypePrefersAccountAndAllowsExplicitAPIKey`, `TestSQLiteStoreDeleteAccountScrubsCredential`, `TestSQLiteStoreModifyDoesNotResurrectLoggedOutAccount`, `TestCodexJSONRefreshRotation`, `TestMigrateAuthJSONImportsCredentialAndRenamesFile`. | High |
| 1.5.0 Added: `pi update` self-update through arbitrary wrapper | **OWNED divergence.** PiG supports `PI_SELF_UPDATE_COMMAND`/`selfUpdateCommand` for non-archive installs, but this repository claims self-update forms of `update`/`upgrade` first for its archive installer (release API, checksum, channel), so the Pi 1.5.0/1.5.1 wrapper path does not govern archive installs. | `vendor/github.com/MichaelKinsy/PiG/coding/cli/self_update.go:37-53`; `vendor/.../internal/codingagent/selfupdate_configured.go:15-20`; `internal/selfupdate/selfupdate.go:512-630`; `main.go:61`. | High |
| 1.5.1 New Feature: idle prompt cache warming generally | **PORTED mechanism, different default** (see divergence row). `cacheWarming: "idle"` is available and economically gated for up to 30 minutes. | `vendor/.../internal/codingagent/cache_warmer.go:29,376,455`; `coding/session_cache_warming.go:149,181-217`. | High |
| 1.5.1 New Feature: richer Mermaid (pie charts, styles, clickable labels; summaries) | **PARTIAL / LOW confidence.** PiG has Mermaid rendering and markdown transformers, but no explicit evidence that the 1.5.1 renderer feature set or `messageTypes` transformer option was ported. This is a TUI presentation gap, not a runtime failure. | `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/mermaid_html_parser.go:69`; `coding/extension/markdowntransform/markdowntransform.go:22-56`; upstream note mirrored only generally in vendored `CHANGELOG.md:1267,1310`. | Low |
| 1.5.1 New Feature: GitHub Copilot Microsoft-Decision-1 classifier via `models.classify()` | **MISSING / OWNED.** PiG has a classifier framework and Gemini decision support, but no Microsoft-Decision-1 catalog entry in the checked tree. This repository's judge roles are configured separately. | `vendor/.../ai/classifier_models_generated.go`; `docs/model-roles.md`; upstream 1.5.1 release notes. | Medium |
| 1.5.2 New Feature + Changed: fork a session whose folder is gone (`Continue`/`Fork`/`Cancel`) | **MISSING.** Missing-cwd paths offer only `Continue`/`Cancel`; interactive resume confirms the current directory but never creates a forked session with a new ID. | `vendor/github.com/MichaelKinsy/PiG/coding/cli/main.go:1157-1167`; `vendor/.../internal/codingagent/extension_session_replacement.go:59-79`. | High |
| 1.5.1 Added: syntax highlighting for Dart, Nix, Groovy, Gradle | **MISSING / LOW.** Not found as named grammars in the vendored tree; affects display only. | `vendor/github.com/MichaelKinsy/PiG/tui/internal/hljs` grammar set; upstream 1.5.1 Added section. | Low |

### C. Ported or already present in vendored PiG (grouped)

These are 1.5.x bullets with matching implementation or an explicit vendored
changelog entry. Line references are representative, not exhaustive.

| Pi 1.5 group | Local state and evidence | Confidence |
|---|---|---|
| 1.5.0 extension changelogs (`pi.changelogPath`, `collapseChangelog`, #5958/#8790); `keyText`/`keyHint` (#4748); blocked inline `ctx.ui.custom()` (#11428) | **PORTED.** Vendored `CHANGELOG.md:49-50,287` and `coding/extension/custom.go:31`; SDK surface in `extensions/sdk`. | Medium |
| 1.5.0 `pi.exec()` `maxBuffer`, `truncated`, SIGKILL after SIGTERM, UTF-8 chunk boundaries, spawn-error stderr (#11360/#11312/#11257) | **PORTED.** `coding/extension/exec.go:28,37-63,77,105-119,136-195,238,298`; `exec_unix.go:20-21`. | High |
| 1.5.0 device-code login opens page/copies code on Enter (#9282/#9301) | **PORTED.** `tui/login_dialog.go:215-234,309`; `ai/auth_interaction.go:67-70`; `coding/cli/auth_login.go:241`. | High |
| 1.5.0 persisted `messageId`/ISO `timestamp` in markdown transform context (#7952); `Usage.cost.totalFromProvider` (#11178); concrete `responseModel` (#11265) | **PORTED.** `coding/extension/markdowntransform/markdowntransform.go:22-49`; `ai/types.go:585-587` and `durable/entry_fast.go:876-890`; `ai/pi_messages_events.go:223-225`, `agent/message_json.go:178-228`. | High |
| 1.5.0 faster streaming / incremental tool-call parsing (#9265) | **PORTED.** `ai/streaming_json_parser.go:129-175`, `ai/tool_call_arguments_stream.go`; faux-provider O(n^2) fix in vendored `CHANGELOG.md:249`. | High |
| 1.5.0 smaller encoded images (#11551); highlighted-output reuse (#10971); code-heavy streaming reuse | **PORTED.** `internal/imageprocessing/images.go:318-341,651-684`; `tui/markdown*` caching noted at vendored `CHANGELOG.md:493,1456`. | Medium |
| 1.5.0 trust decision precedence (#10947); Windows case-insensitive paths / broken `~` prefix (#10488); deleted cwd startup (#11509) | **PORTED.** `coding/cli/project_trust_report.go:32-66`; `CHANGELOG.md:273,307,286`; `coding/cli/main.go:819`. | Medium |
| 1.5.0 OSC 7501 status in regular TUI / no OSC 133 (#11519); Shift+Enter Apple Terminal (#11559); PageUp/PageDown selectors (#7629); Kitty F-keys (#11419/#11345); Orca images (#9329); CJK Markdown URLs (#10934); wrapped-style fixes (#10925); double-click word chars (#10980) | **PORTED** in vendored `tui/` and its SDKs; explicit changelog entries at `CHANGELOG.md:239-247,272,275,282,285,303,493-495`. | Medium |
| 1.5.0 OAuth compaction passing stored token as API key (#11520); tool-order system prompt update (#11495); `/tree` summaries honoring thinking/context edits (#11444/#11602) | **PORTED.** `CHANGELOG.md:253-254,268`; `coding/session_cache_warming.go:156` for resource cleanup. | Medium |
| 1.5.0 bash process-group abort on macOS (#11252/#11289) | **PORTED.** `internal/codingagent/tools/bash_group_unix.go:12-24`; `bash_operations.go:150,195,233`. | High |
| 1.5.0 failed session writes leaving orphaned in-memory entries (#11460) | **PORTED.** `internal/codingagent/session.go:365-398`: record is written before the entry joins memory; on `writeRecord` error nothing is added. | High |
| 1.5.0 `tool_search` ranking (#11331) and any-script tokenization (#10816/#11037) | **PORTED.** `coding/extension/builtin/toolsearch/toolsearch.go:50-78,173-176`; `internal/searchtext/searchtext.go:51`; `CHANGELOG.md:284,297,300`. Local `$ref` is the remaining gap (BUG row). | High |
| 1.5.0 security: MCP OAuth non-http(s) URLs; login-dialog non-web URLs/control chars; glob DoS amplification | **PORTED.** `mcp/oauth/flow.go:327-330` rejects non-HTTP authorization endpoints; `internal/minimatch` memory-bounded brace expansion noted at vendored `CHANGELOG.md:186`. Login URL control-character handling not individually re-verified. | Medium |
| 1.5.0 `pi -e npm/git` refresh (#11375/#11590), skill warning label (#11521), codemode output-type descriptions (#11501), model-scope notice (#11508), stale-npm-version warning (#11536), `mcp login -l` (#be15fe3c), WSL/external-editor status (#11592) | **PORTED** per explicit vendored `CHANGELOG.md:270,283,292-305,498`. These are extension/package/TUI surface paths delegated to PiG. | Medium |
| 1.5.0 provider fixes: OpenRouter reported cost (#9980), OpenCode endpoints (#11434), Zhipu 1308 no-retry (#11537), LiteLLM overflow (#11492), Gemini `const`→`enum` (#11308), faux cache-write accounting, OpenRouter `~anthropic` thinking (#9534), Anthropic system ordering (#11091), non-strict `$defs` (#9557), Bedrock strict-tool retry (#11416), Claude effort cap (#10063), OpenCode thinking binding (#10954) | **PORTED** for the paths that exist in Go: `ai/openrouter_cost.go:20-49`; `CHANGELOG.md:448` (OpenCode endpoints); `ai/overflow.go:24`; `ai/google.go:686`; `CHANGELOG.md:256,260-262,429`. Confidence varies per item; no contradictory code found. | Medium |
| 1.5.0 Claude/Kimi model behavior, DeepSeek cache retention, GPT function namespaces, and terminal-title sanitization | **NOT VERIFIED.** Customized model catalog and role routing (`ai/omp_catalog_data`, `internal/*`, `docs/model-roles.md`) do not establish whether individual upstream provider and terminal bugs are present. | Low |
| 1.5.1 shell `auth login/logout`, RPC idle `ctx.shutdown()` and shutdown cleanup (#11599) | **PORTED.** `coding/cli/auth_login.go:33`; `coding/cli/rpc_shutdown_gate.go:8`; `coding/cli/rpc_shutdown.go:15-25,106-127` (shutdown once; second signal or EOF exits immediately). | High |
| 1.5.1 MCP token-refresh on shutdown (#11692), grep lone-CR context numbering (#11398), strict `prefer` tool reset (#11626), Anthropic `$defs` (#9557), Azure 1.05M context (#11742), Responses `session-id` header (#11639), MCP stdio kill-on-exit | **PORTED.** `coding/mcpext/runtime.go:824-828`; `CHANGELOG.md:458,260`; `ai/session_resources.go:35`; `ai/openai_responses.go:1724`; `mcp/stdio_unix.go:42`; `ai/models_generated.go` has 1,050,000-context Azure entries. | Medium |
| 1.5.1 dropped-stream retry (#11879), image-only steering queue (#8612), paste markers (#11923/#10146), thinking level on `/tree` branch (#11451), Markdown HTML comments (#11710), codemode `NaN`/`Infinity` (#11901) | **NOT VERIFIED.** These distinct behaviors were not traced end to end; Go's JSON handling alone does not establish codemode console formatting. | Low |
| 1.5.1 find on Windows (#9262), untracked `fd` on cancel (#11974), WSL helper paths (#11592), Windows installer (#11944), PowerShell 5.1 (#8842), Alacritty detection (#11767), Codex SSE listeners (#11708), symlinked extension import (#8112) | **NOT VERIFIED / platform-specific.** Platform support alone does not establish parity. WSL/external-editor status is claimed as ported in `CHANGELOG.md:498`. | Low |
| 1.5.1 default-model moves, partner ordering, login cosmetics, GLM allowlist, qwen-token-plan-individual-cn (#7989), Qwen defaults | **OWNED / N-A.** Model discovery, account routing, and provider identity are customized in this repository (`internal/agentstate`, `ai/omp_catalog_data`, `docs/model-roles.md`); upstream provider defaults are not a parity target. | Medium |
| 1.5.0 tool argument validation via pi-schema / coercion / error messages; 1.5.1 BigInt/Undefined conversions | **NOT VERIFIED.** The vendored `schema` package implements coercion (`vendor/.../schema/coerce.go`), but its presence does not establish that the tool-validation path uses it or that TypeBox semantics match. The vendored `CHANGELOG.md` states that the new package was not yet in use. | Low |
| 1.5.0 system prompt tightened; sandbox example removed; undici security bump; npm installer/update flags (pnpm ownership, `git clean`, `--min-release-age=0`, audit output) | **N/A / OWNED.** Prompt prose is `prompts/system.md`; native tools run with OS permissions (AGENTS.md boundary); self-update is the archive installer in `internal/selfupdate`. | High |

### D. Coverage notes for the remaining 1.5.x bullets

- 1.5.0's HTML-export fixes (#11225/#11293, #11261/#11290) are **N/A**: HTML
  export was removed from PiG; `internal/codingagent/slash_session_handlers.go:980`
  and `removed_features_test.go:89` enforce that.
- 1.5.0's npm-only package-lock/symlink, `-e` package refresh, extension-editor
  dialog, todo/Gondolin example, and installer/package-manager messages are
  delegated to PiG; where ported they are listed in C, otherwise they are
  outside this Go binary's owned surface.
- 1.5.1's `GrepOperations.readFile` breaking change and the `tools: []`
  breaking change are explicitly called out above (BUG rows), because they are
  behavioral contracts rather than cosmetic feature gaps.
- Many remaining extension, model-selector, paste, overlay, and MCP transport
  fixes have not been individually verified; absence of a finding is not proof
  that the associated defect is absent.
- 1.5.2's only three bullets are all covered: missing-cwd fork is **MISSING**
  (B), import duplicate cleanup is **BUG** (A), and the stale Claude Code
  version is a version mismatch whose practical effect is untested (A).

### Assigned-scope verification

- `go test -mod=vendor github.com/MichaelKinsy/PiG/internal/packagemanager -run TestUpdateReportsChangedGitPackage -count=1` verifies an update against a local bare Git fixture, the resulting checkout, changed-commit event, and omission of an unchanged result.
- `go test -mod=vendor github.com/MichaelKinsy/PiG/coding/cli -run 'TestAuthLogin|TestStoredAccountKeys|TestPrintPackageUpdateSummary|TestRunPackageUpdatePrintsChangedPackageSummary' -count=1` verifies auth CLI choices and package-update summaries, including the real update-output path.
- `go test -mod=vendor github.com/MichaelKinsy/PiG/ai -run 'TestSQLiteStore|TestMigrateAuthJSON|TestCodexJSONRefresh(Lifecycle|Rotation)' -count=1` verifies credential-store tombstones, migration, and refresh rotation with isolated test data.
- `go test -mod=vendor github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestResumeGroupsAdjacentCacheWarmingUsage|TestCompactedResumeLeavesToolHistoryCold|TestCostPolicyPreservesProfitableCacheWarming' -count=1` verifies cumulative transcript notices, compacted-resume behavior, and the deliberate streaming default.
- The change does not alter this repository's self-update routing or installer ownership. Tests use temporary directories and local Git repositories; no real credentials or provider requests were used.

## Pi 1.6.0

Source: [Pi 1.6.0 release notes](https://pi.dev/changelog/releases/1.6.0),
[GitHub release](https://github.com/earendil-works/pi/releases/tag/v1.6.0).
The following is a prioritized comparison, not a verdict on every 1.6 fix.

### Code-level matches to problems fixed in 1.6

| Pi fix | Finding and local evidence | Confidence |
|---|---|---|
| Find paths relative to search root and leading `./` (#12066, #12020) | **BUG.** `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/tools/find.go:118-131` prepends `**/` and matches absolute paths. `src/*.ts` can match at a deeper `nested/src/`, and `./src/*.ts` becomes `**/./src/*.ts`. Using the same fd arguments in a temporary fixture reproduced the unwanted nested match and missing `./` result; this was not an end-to-end tool invocation. | High |
| Stale image warning after model change (#12131) | **BUG.** `.../tools/read.go:456-466` appends the current model's no-image warning to the tool-result text rather than keeping it TUI-only; the text can be stored and replayed after switching models. | High |
| `pi.exec` signal exit codes (#12088) | **BUG.** `vendor/github.com/MichaelKinsy/PiG/coding/extension/exec.go:296-309` maps a signaled child's negative exit code to 0 rather than 128 plus signal. This is the extension API, not necessarily the built-in bash tool. | High |
| `/resume` search selection (#12245) | **BUG.** `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/session_selector.go:517-546` clamps the prior selected index after filtering rather than resetting to the best match. | High, source-level |
| `/copy` after a failed assistant response (#12119) | **BUG.** `vendor/github.com/MichaelKinsy/PiG/coding/session.go:1762-1780` returns nil for a textless error response rather than continuing to an earlier answer; this helper also serves RPC. Empty aborted messages are skipped. | High, source-level |
| Codex output limit | **DIVERGENCE.** `vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:1499-1502` skips `max_output_tokens` when `p.cfg.Codex`. Whether a given Codex endpoint accepts the new field was not tested. | High for payload decision |
| Non-OpenAI system message roles (#10741) | **LIKELY BUG.** `vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:777-795,1189` selects `developer` for reasoning models unless compatibility overrides disable it; the provider-specific Groq/Qwen failure was not reproduced. | Medium |
| Strict schemas with typeless nodes (#12049) | **LIKELY BUG.** `vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:1346-1354` and `ai/constrained_sampling.go:223-277` do not show Pi 1.6's downgrade for a typeless node such as `items: {}`. A real OpenAI rejection was not tested. | Medium |
| Non-strict Anthropic schemas and extra properties (#12067) | **LIKELY GAP.** The non-strict schema construction in `vendor/github.com/MichaelKinsy/PiG/ai/anthropic.go:899-934` does not add Pi's new `additionalProperties: false` constraint. | Medium |
| WebSocket expiry mid-stream (#12124) | **LIKELY GAP.** `vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go:722-965` handles pre-stream errors, but the inspected read-failure path after streaming starts did not show Pi's fresh-connection retry. Not tested with a long-running stream. | Medium |
| Additional retry messages (#12064, #12128) | **LIKELY GAP.** Patterns in `vendor/github.com/MichaelKinsy/PiG/ai/assistant_retry.go:39-105,259-270` do not list `Connection closed.`, `read ECONNRESET`, `write EPIPE`, or `connect ETIMEDOUT`; Go transport normalization may cover some actual failures, so exposure needs a provider-level test. | Medium |
| External SIGTSTP restores terminal (#12283) | **LIKELY BUG.** `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_signals_unix.go:32,112,138` registers WINCH and CONT and has a Ctrl-Z path, but no handler for externally delivered TSTP. Not checked in a PTY. | Medium |

### Already present, missing features, and non-applicable changes

- **Already mitigated:** the bash truncator keeps the tail of a too-long last
  line (`vendor/github.com/MichaelKinsy/PiG/internal/truncate/truncate.go:114-154`);
  unstarted calls in an aborted tool batch receive abort results
  (`vendor/github.com/MichaelKinsy/PiG/agent/tool_execution.go:225-240`).
- **Features/defaults missing or different:** Anthropic message threads and the
  OpenAI dropped-reasoning notice were not found. The reasoning-drop notice is
  separate from the cache-warming transcript aggregation ported above and
  remains absent. GPT-6.2 defaults differ from Pi's
  (`vendor/github.com/MichaelKinsy/PiG/internal/codingagent/default_models.go`);
  dynamic model discovery can add models, so a missing bundled catalog row does
  not prove a model is unavailable. See `docs/model-discovery.md` and
  `docs/research/gpt-6.2-availability.md`. Qwen per-plan environment variables,
  names and gateways also differ (`vendor/github.com/MichaelKinsy/PiG/ai/auth_env_keys.go:33-35`,
  `ai/provider_names.go:39-41,97-99`).
- **No equivalent in this product:** Pi's Node `undici` advisory and
  managed-Node updater do not imply the same vulnerability in the Go binary;
  removed `/share` and HTML export surfaces are listed in
  `vendor/github.com/MichaelKinsy/PiG/internal/codingagent/removed_features.go`.
  This says nothing about independent dependency vulnerabilities.
- **Not verified:** individual image overlay/redraw fixes; Copilot reasoning
  and optional arguments; Codex cache behavior; stdout drain; Tab completion;
  MCP `tool_search` restoration; session-start/JSON/RPC event delivery; and
  other 1.6 release bullets not listed above. Their absence from this note is
  not evidence they are fixed or irrelevant.

## Verification and suggested order

The review inspected the versioned release notes and the checked-out Go source.
It exercised ripgrep/fd with temporary fixtures using the arguments constructed
by the tools: grep returned zero match events for the path-relative example;
find matched a nested `src` and did not match a leading `./` pattern. These
checks do not replace tests through the executable. No live provider requests,
destructive large-file/OOM experiments, or platform-specific PTY checks were
made. Assigned-scope implementation and regression evidence is recorded above.

Suggested first regression tests and fixes: (1) edit preserving untouched line
endings and session import cleanup, for data integrity; (2) large-file read
memory bounds and grep/find search semantics; (3) provider empty-tools payload
and extension signal status; (4) session selector, image warning and retry
behavior. Validate each via the relevant public interface and preserve PiG
extension and session compatibility rather than applying the TypeScript patches
mechanically.
