# Quality baseline

Recorded when the gate was introduced (`feat/better-lintin`). The gate runs
strictly on **new and changed code**; this file is the debt that already exists
and may only shrink. Machine-readable counts are in `quality-baseline.json`.

Total: **82 findings**. The codebase is unusually clean — most of this is a
small number of real-but-low-risk patterns plus a few analyzer false positives.

| Linter | Count | What it is | Fix risk | Payoff |
|---|---|---|---|---|
| wrapcheck | 17 | bare `net/http` / `url.Parse` / `flock` errors in `broker/client.go`, `selfupdate.go`, `workdesk.go` | low | wrap I/O errors with context: `fmt.Errorf("…: %w", err)` |
| errcheck | 17 | deliberate discards now flagged by `check-blank`: `tracker.Pause/Drop`, `extCtx.HasUI`, `json.Marshal`, `conversation.Submit`, `fs.Glob`, `path.Match` | low | these are intentional best-effort/no-UI paths; keep or add a one-line `_ =` with a comment |
| contextcheck | 8 | `context.Background()` in shutdown/cleanup paths where the run ctx is already cancelled — `workdesk.go` (×6), `client.go:472`, `setup_command.go` | none | **mostly false positives**; `Background()` is correct for cleanup after cancellation |
| gosec | 9 | `G703` path-taint on user-config paths (intended), `G117` JSON secret fields (intended contract), `G101` placeholder-credential patterns | none | audit-only; the flagged shapes are the feature, not a bug |
| gochecknoglobals | 5 | `SharedFiles`, `goalControlWords`, `progressDoc`, `nextID`, `system` (embedded prompt) | low | 4 are immutable lookup tables; `goal.nextID` is a real mutable counter — consider scoping it |
| gocognit | 5 | `Client.syncFile` (80), `Run` (63), `extensionWithTracker` (48), `failureStreak.record` (46), `Store.credential` (44) | medium | split the syncFile/Run dispatchers into helpers |
| goconst | 12 | repeated literals: `"import"`, `"save"`, `"session-account"`, `"modify"`, `"windows"`, `"beta"` | low | hoist to named consts (cheap) |
| gocritic | 4 | `evalOrder`, `sprintfQuotedString`, `ifElseChain` after disabling noisy `sloppyReassign` | low | style nits |
| cyclop | 1 | `parseRule` cyclomatic 33 | low | split the rule parser switch |
| nestif | 1 | `Client.syncFile` nested-if complexity 40 | medium | same function as the gocognit hit — fix once |
| funlen | 1 | `streamingrules.Extension` 83 lines | none | thin factory; could split registration block |
| exhaustive | 1 | `goalControlWords` switch missing `goalOpPrompt` | low | confirm `goalOpPrompt` is handled or add the case |
| unparam | 1 | `newProgress` `label` always `"download"` | none | API is intended reusable; a second caller would need it |

## Recommended payoff order (risk first)

1. **`errcheck` + `wrapcheck` on the credential/sync boundary** (`broker/client.go`,
   `broker/login.go`, `broker/vault.go`, `selfupdate.go`) — a swallowed error on
   the credential path can corrupt account state; this is the highest-risk debt.
2. **`gocognit`/`nestif`/`cyclop` on `Client.syncFile` and `workdesk.Run`** —
   the two most complex functions sit on the sync and durable paths that the git
   history shows regressing most.
3. **`exhaustive` `goalOpPrompt`** — one enum case; confirm it is intended or
   handle it. Smallest real correctness check.
4. **`goconst` literals** — mechanical, prevents drift of protocol strings
   (`"session-account"`, `"modify"`, `"import"` appear in both client and server).
5. **`gosec` audit** — confirm each `G703`/`G117` is intended; if any is not,
   it is the only finding in the set that is a live bug.

## Deliberately off / not baselined

- **`mnd` (magic numbers)** — 52 mostly-justified hits (file modes `0o755`,
  timeouts, HTTP statuses). The repo names constants only where the value is
  load-bearing; the rule cost > signal. Revisit if numeric drift shows up.
- **`rowserrcheck` / `sqlclosecheck`** — no `database/sql` in the root module
  (SQLite lives inside vendored PiG). Enabled only if the root adds direct SQL.
- **`testifylint`** — repo uses stdlib `testing` only; testify is not a dep.
- **`dupl`** — enabled but with threshold 150; no findings at that level, so
  nothing to baseline.
