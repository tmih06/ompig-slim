# Codex conversation

## User

can you find the code that manage the input prompt box, that render the "Working" on the top edge of that prompt box

## Assistant

I’ll look for the prompt box component and the code that renders “Working” along its top edge.

## Activity

    $ pwd; rg --files -g 'AGENTS.md' -g '!node_modules' -g '!vendor' -g '!target' -g '!dist'; rg -n 'Working' --glob '!node_modules/**' --glob '!vendor/**' --glob '!target/**' --glob '!dist/**' .
    /home/ubuntu/.herdr/worktrees/ompig-slim/feat-usage-on-status-bar
    ✗ (1) • 0ms

## Activity

    $ ls -la; rg --files --hidden -g '!.git' | head -80; for p in /home/ubuntu/AGENTS.md /home/ubuntu/.herdr/AGENTS.md /home/ubuntu/.herdr/worktrees/AGENTS.md /home/ubuntu/.herdr/worktrees/ompig-slim/AGENTS.md; do if [ -f "$p" ]; then cat "$p"; fi; done
    total 100
    drwxrwxr-x 6 ubuntu ubuntu  4096 Nov  7 11:37 .
    drwxrwxr-x 4 ubuntu ubuntu  4096 Nov  7 11:37 ..
    drwxrwxr-x 3 ubuntu ubuntu  4096 Nov  7 11:37 .agents
    -rw-rw-r-- 1 ubuntu ubuntu    72 Nov  7 11:37 .git
    drwxrwxr-x 3 ubuntu ubuntu  4096 Nov  7 11:37 .github
    -rw-rw-r-- 1 ubuntu ubuntu    34 Nov  7 11:37 .gitignore
    -rw-rw-r-- 1 ubuntu ubuntu  1155 Nov  7 11:37 .gitleaks.toml
    -rw-rw-r-- 1 ubuntu ubuntu  3433 Nov  7 11:37 CONTRIBUTING.md
    -rw-rw-r-- 1 ubuntu ubuntu  1198 Nov  7 11:37 LICENSE
    -rw-rw-r-- 1 ubuntu ubuntu   583 Nov  7 11:37 Makefile
    -rw-rw-r-- 1 ubuntu ubuntu  1324 Nov  7 11:37 NOTICE
    -rw-rw-r-- 1 ubuntu ubuntu 15882 Nov  7 11:37 README.md
    -rw-rw-r-- 1 ubuntu ubuntu  5869 Nov  7 11:37 SECURITY.md
    -rw-rw-r-- 1 ubuntu ubuntu  2003 Nov  7 11:37 go.mod
    -rw-rw-r-- 1 ubuntu ubuntu  8224 Nov  7 11:37 go.sum
    -rw-rw-r-- 1 ubuntu ubuntu   573 Nov  7 11:37 main.go
    drwxrwxr-x 2 ubuntu ubuntu  4096 Nov  7 11:37 scripts
    -rw-rw-r-- 1 ubuntu ubuntu   519 Nov  7 11:37 skills-lock.json
    drwxrwxr-x 7 ubuntu ubuntu  4096 Nov  7 11:37 vendor
    README.md
    NOTICE
    vendor/modules.txt
    vendor/cloud.google.com/go/compute/metadata/syscheck.go
    vendor/cloud.google.com/go/compute/metadata/README.md
    vendor/cloud.google.com/go/compute/metadata/log.go
    vendor/cloud.google.com/go/compute/metadata/retry_linux.go
    vendor/cloud.google.com/go/compute/metadata/syscheck_windows.go
    vendor/cloud.google.com/go/compute/metadata/CHANGES.md
    vendor/cloud.google.com/go/compute/metadata/retry.go
    vendor/cloud.google.com/go/compute/metadata/metadata.go
    vendor/cloud.google.com/go/compute/metadata/syscheck_linux.go
    vendor/cloud.google.com/go/compute/metadata/LICENSE
    vendor/go.yaml.in/yaml/v3/README.md
    vendor/go.yaml.in/yaml/v3/readerc.go
    vendor/go.yaml.in/yaml/v3/apic.go
    vendor/go.yaml.in/yaml/v3/parserc.go
    vendor/go.yaml.in/yaml/v3/NOTICE
    vendor/go.yaml.in/yaml/v3/yamlprivateh.go
    vendor/go.yaml.in/yaml/v3/decode.go
    vendor/go.yaml.in/yaml/v3/scannerc.go
    vendor/go.yaml.in/yaml/v3/emitterc.go
    vendor/go.yaml.in/yaml/v3/sorter.go
    vendor/go.yaml.in/yaml/v3/resolve.go
    vendor/go.yaml.in/yaml/v3/yaml.go
    vendor/go.yaml.in/yaml/v3/writerc.go
    vendor/go.yaml.in/yaml/v3/yamlh.go
    vendor/go.yaml.in/yaml/v3/encode.go
    vendor/go.yaml.in/yaml/v3/LICENSE
    go.sum
    vendor/golang.org/x/net/idna/tables17.0.0.go
    vendor/golang.org/x/net/idna/tables15.0.0.go
    vendor/golang.org/x/net/idna/idna.go
    vendor/golang.org/x/net/idna/punycode.go
    vendor/golang.org/x/net/idna/trie.go
    vendor/golang.org/x/net/idna/trieval.go
    vendor/golang.org/x/net/PATENTS
    vendor/golang.org/x/net/LICENSE
    go.mod
    .gitleaks.toml
    vendor/golang.org/x/oauth2/README.md
    vendor/golang.org/x/oauth2/transport.go
    vendor/golang.org/x/oauth2/oauth2.go
    vendor/golang.org/x/oauth2/.travis.yml
    vendor/golang.org/x/oauth2/internal/transport.go
    vendor/golang.org/x/oauth2/internal/oauth2.go
    vendor/golang.org/x/oauth2/internal/token.go
    vendor/golang.org/x/oauth2/internal/doc.go
    vendor/golang.org/x/oauth2/jws/jws.go
    vendor/golang.org/x/oauth2/token.go
    vendor/golang.org/x/oauth2/deviceauth.go
    vendor/golang.org/x/oauth2/jwt/jwt.go
    .github/workflows/ci.yml
    vendor/golang.org/x/oauth2/authhandler/authhandler.go
    SECURITY.md
    skills-lock.json
    CONTRIBUTING.md
    main.go
    LICENSE
    .gitignore
    vendor/golang.org/x/oauth2/pkce.go
    vendor/github.com/mattn/go-isatty/README.md
    scripts/check-go.sh
    vendor/github.com/mattn/go-isatty/isatty_others.go
    vendor/golang.org/x/oauth2/google/internal/impersonate/impersonate.go
    vendor/github.com/mattn/go-isatty/isatty_solaris.go
    vendor/github.com/mattn/go-isatty/isatty_plan9.go
    vendor/github.com/mattn/go-isatty/go.test.sh
    vendor/github.com/mattn/go-isatty/doc.go
    vendor/github.com/mattn/go-isatty/isatty_windows.go
    vendor/github.com/mattn/go-isatty/isatty_tcgets.go
    vendor/github.com/mattn/go-isatty/isatty_bsd.go
    vendor/github.com/mattn/go-isatty/LICENSE
    vendor/golang.org/x/oauth2/google/internal/externalaccountauthorizeduser/externalaccountauthorizeduser.go
    vendor/golang.org/x/oauth2/google/internal/stsexchange/sts_exchange.go
    vendor/golang.org/x/oauth2/google/internal/stsexchange/clientauth.go
    vendor/github.com/mattn/go-runewidth/new.txt
    vendor/github.com/mattn/go-runewidth/README.md
    vendor/github.com/mattn/go-runewidth/runewidth_windows.go
    vendor/github.com/mattn/go-runewidth/runewidth_appengine.go
    ✓ • 0ms

## Activity

    $ rg -n 'Working' vendor; cat main.go; cat go.mod; rg --files --hidden -g AGENTS.md -g '!.git'
    vendor/golang.org/x/sys/windows/types_windows_arm64.go:28:    MinimumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/types_windows_arm64.go:29:    MaximumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/types_windows_386.go:28:    MinimumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/types_windows_386.go:29:    MaximumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/types_windows.go:3120:    ProcessWorkingSetWatch
    vendor/golang.org/x/sys/windows/types_windows.go:3147:    ProcessWorkingSetWatchEx
    vendor/golang.org/x/sys/windows/types_windows.go:3162:    ProcessWorkingSetControl
    vendor/golang.org/x/sys/windows/types_windows.go:3222:    WorkingSetPrivateSize        int64
    vendor/golang.org/x/sys/windows/types_windows.go:3239:    PeakWorkingSetSize           uintptr
    vendor/golang.org/x/sys/windows/types_windows.go:3240:    WorkingSetSize               uintptr
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:304:    procGetProcessWorkingSetSizeEx                           = modkernel32.NewProc("GetProcessWorkingSetSizeEx")
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:393:    procSetProcessWorkingSetSizeEx                           = modkernel32.NewProc("SetProcessWorkingSetSizeEx")
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:463:    procQueryWorkingSetEx                                    = modpsapi.NewProc("QueryWorkingSetEx")
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:2651:func GetProcessWorkingSetSizeEx(hProcess Handle, lpMinimumWorkingSetSize *uintptr, lpMaximumWorkingSetSize *uintptr, flags *uint32) {
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:2652:    syscall.SyscallN(procGetProcessWorkingSetSizeEx.Addr(), uintptr(hProcess), uintptr(unsafe.Pointer(lpMinimumWorkingSetSize)), uintptr(unsafe.Pointer(lpMaximumWorkingSetSize)), uintptr(unsafe.Pointer(flags)))
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:3433:func SetProcessWorkingSetSizeEx(hProcess Handle, dwMinimumWorkingSetSize uintptr, dwMaximumWorkingSetSize uintptr, flags uint32) (err error) {
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:3434:    r1, _, e1 := syscall.SyscallN(procSetProcessWorkingSetSizeEx.Addr(), uintptr(hProcess), uintptr(dwMinimumWorkingSetSize), uintptr(dwMaximumWorkingSetSize), uintptr(flags))
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:3985:func QueryWorkingSetEx(process Handle, pv uintptr, cb uint32) (err error) {
    vendor/golang.org/x/sys/windows/zsyscall_windows.go:3986:    r1, _, e1 := syscall.SyscallN(procQueryWorkingSetEx.Addr(), uintptr(process), uintptr(pv), uintptr(cb))
    vendor/golang.org/x/sys/windows/types_windows_amd64.go:28:    MinimumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/types_windows_amd64.go:29:    MaximumWorkingSetSize   uintptr
    vendor/golang.org/x/sys/windows/syscall_windows.go:364://sys    GetProcessWorkingSetSizeEx(hProcess Handle, lpMinimumWorkingSetSize *uintptr, lpMaximumWorkingSetSize *uintptr, flags *uint32)
    vendor/golang.org/x/sys/windows/syscall_windows.go:365://sys    SetProcessWorkingSetSizeEx(hProcess Handle, dwMinimumWorkingSetSize uintptr, dwMaximumWorkingSetSize uintptr, flags uint32) (err error)
    vendor/golang.org/x/sys/windows/syscall_windows.go:444://sys   QueryWorkingSetEx(process Handle, pv uintptr, cb uint32) (err error) = psapi.QueryWorkingSetEx
    vendor/github.com/charmbracelet/x/ansi/cwd.go:8:// NotifyWorkingDirectory returns a sequence that notifies the terminal
    vendor/github.com/charmbracelet/x/ansi/cwd.go:18:func NotifyWorkingDirectory(host string, paths ...string) string {
    vendor/github.com/MichaelKinsy/PiG/third_party/semver/README.md:146:### Working With Prerelease Versions
    vendor/modernc.org/sqlite/lib/sqlite_g_0000000000060000.go:87997:const _PowerSystemWorking = 1
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:577:- Go API `durable/storage/sqlite`: `Options` is renamed `SqliteStorageOptions`, the name of Pi's type. Before: `sqlite.Open(db, sqlite.Options{WorkingSetBytes: n})`. After: `sqlite.Open(db, sqlite.SqliteStorageOptions{WorkingSetBytes: n})`.
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:578:- Go API `durable/storage/sqlite/node`: the connection settings move to `NodeSqliteDatabaseOptions`, and `NodeSqliteStorageOptions` embeds them next to `Prefix` and `WorkingSetBytes`. `OpenNodeSqliteDatabase` takes `NodeSqliteDatabaseOptions`. Before: `OpenNodeSqliteDatabase(path, node.NodeSqliteStorageOptions{BusyTimeoutMs: &ms})`. After: `OpenNodeSqliteDatabase(path, node.NodeSqliteDatabaseOptions{BusyTimeoutMs: &ms})`. A literal of `NodeSqliteStorageOptions` that sets a connection field names the embedded struct (`NodeSqliteStorageOptions{NodeSqliteDatabaseOptions: node.NodeSqliteDatabaseOptions{BusyTimeoutMs: &ms}}`).
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:639:- Extension SDKs (Go, Python, Rust): a delegated editor component takes the handlers Pi's `CustomEditor` keeps as public fields on its base: `onEscape`, `onCtrlD`, `onPasteImage`, `onExtensionShortcut`, `actionHandlers` and `onAction`. The host answers the key of an app action the component handles with a hook instead of running the action, and the component runs its handler inside `handleInput`, in the order Pi's `CustomEditor.handleInput` gives them (Escape only while no autocomplete shows, Ctrl+D only on an empty editor), as pinned Pi does for each handler set or not. The base also reaches the members `CustomEditor` inherits from `Editor` (`borderColor`, `disableSubmit`, `focused`, `getPaddingX`, `getAutocompleteMaxVisible`, `setAutocompleteProvider`, and the host's `onSubmit` and `onChange`); a component hears the working status the host shows in its border (`setWorkingStatusIndicator`) and a theme change (`invalidate`, which the Node runtime passes to its `CustomEditor` too).
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:1157:- Durable turns no longer re-read and re-decode the whole stored history for every model call. SQLite storage keeps a bounded working set of decoded entries (64 MiB by default; `NodeSqliteStorageOptions.WorkingSetBytes` changes it, 0 turns it off) that commits write through and a change made through another connection resets, and the Harness extends each conversation's derived context by the entries committed since. A warm turn on a 3,500-turn history reads about 540 rows (pi-durable 1.0.4 reads about 212,000), and the store format is unchanged (D104). On the durable-bench workload at 3,500 turns, a warm PiG turn went from about 9.6 s to about 0.37 s on one host with the default GOMAXPROCS, and to about 0.2 s with GOMAXPROCS=1.
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:1171:- `extension.ExtensionUIDialogOptions` and `extension.WorkingIndicatorOptions` are typed structs instead of `any`. `ModelRuntime.GetAllAvailable` is added and `ModelRuntime.GetModels` takes an optional provider. `codingagent.PiSlashCommand` is `SlashCommandInfo`; the `/help` summary type is `HelpCommandInfo`.
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:2203:- `setWorkingMessage` with no message restores the default working label and an empty string shows an empty label, as in Pi (`message ?? defaultWorkingMessage`). Both used to show the default. The Go, Rust and Python extension SDKs gain `ClearWorkingMessage()`, `clear_working_message()` and `set_working_message(None)` for the no-argument form; `SetWorkingMessage(message)` still takes a string. A `UIContext` can implement the new `extension.WorkingMessageClearer` interface to tell the two apart; one that does not keeps the old behavior, so existing implementations still compile. `setHiddenThinkingLabel` follows the same rule (`ClearHiddenThinkingLabel()`, `clear_hidden_thinking_label()`, `set_hidden_thinking_label(None)`; `extension.HiddenThinkingLabelClearer`). A Session replacement now also resets the working message, visibility and indicator an extension set, as Pi does.
    vendor/github.com/MichaelKinsy/PiG/CHANGELOG.md:2303:- A subprocess custom editor that sets `embedWorkingStatus` shows the working, compaction, retry and branch-summary status inside the editor, as Pi does (D94) (#135).
    vendor/github.com/MichaelKinsy/PiG/internal/subagents/prompt.go:30:    env := "# Environment\nWorking directory: " + cwd + "\n"
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/program_status_reporter.go:178:        return tui.ProgramStatus{State: tui.ProgramStateWorking, Message: "Compacting context"}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/program_status_reporter.go:182:        status = tui.ProgramStatus{State: tui.ProgramStateWorking}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/program_status_reporter.go:184:    if status.State == tui.ProgramStateWorking || status.State == tui.ProgramStateDone {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go:839:    options := CustomEditorOptions{PaddingX: &paddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/tools/bash_operations.go:114:        return BashOperationsResult{}, fmt.Errorf("Working directory does not exist: %s\nCannot execute %s commands.", cwd, o.ShellName)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:83:func (m *InteractiveMode) startWorkingLoader() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:87:    message := "Working"
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:92:    m.applyWorkingIndicatorOptions(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:97:func (m *InteractiveMode) stopWorkingLoader() { m.clearStatusIndicator("working") }
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:271:            if m.remoteEditor != nil && m.activeWorkingIndicatorEmbedded {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:101:// resetWorkingStatus returns the working message, visibility and indicator to their defaults, as Pi does when a Session replacement resets extension UI (interactive-mode.ts:2474-2482).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:103:func (m *InteractiveMode) resetWorkingStatus() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:106:    m.setWorkingIndicator(nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:112:        indicator.SetMessage("Working (" + key + " to interrupt)")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:128:        m.resetWorkingStatus()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_chat.go:533:func (m *InteractiveMode) setWorkingVisible(visible bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_chat.go:538:        m.startWorkingLoader()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_chat.go:543:    m.statusLine.SetWorking(visible && !m.isIdle)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:12:// upstream: packages/coding-agent/src/modes/interactive/interactive-mode.ts:setEditorWorkingStatusIndicator
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:13:func (m *InteractiveMode) setEditorWorkingStatusIndicator(indicator *tui.StatusIndicator) bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:18:        embeds := m.remoteEditor.editor.EmbedWorkingStatus()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:21:            m.editor.SetWorkingStatusIndicator(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:23:            m.editor.SetWorkingStatusIndicator(nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:27:    if !m.editor.EmbedWorkingStatus {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:30:    m.editor.SetWorkingStatusIndicator(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:31:    return m.editor.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:37:    m.activeWorkingIndicatorEmbedded = m.setEditorWorkingStatusIndicator(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:40:    if !m.activeWorkingIndicatorEmbedded {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:52:    embedded := m.activeWorkingIndicatorEmbedded
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:58:    m.activeWorkingIndicatorEmbedded = false
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:59:    m.setEditorWorkingStatusIndicator(nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:157:func (m *InteractiveMode) applyWorkingIndicatorOptions(indicator *tui.StatusIndicator) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:171:func (m *InteractiveMode) setWorkingIndicator(options *extension.WorkingIndicatorOptions) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:174:        m.applyWorkingIndicatorOptions(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:184:// setWorkingMessage is Pi's setWorkingMessage (interactive-mode.ts:2641-2646): nil restores the default label, an empty string shows an empty one.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:185:func (m *InteractiveMode) setWorkingMessage(message *string) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:192:        m.statusLine.SetWorkingMessage(custom)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:195:        label := "Working"
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/keys.go:169://    Working Esc              → outcomeAbort
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_editor_component.go:119:func (*componentEditor) EmbedWorkingStatus() bool { return false }
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:155:        Working:        m.frontendWorking,
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:227:// frontendWorking reports the active status indicator, which the terminal
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:229:// an extension's only when it set its own with setWorkingIndicator.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:230:func (m *InteractiveMode) frontendWorking() (frontend.Working, bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:233:        return frontend.Working{}, false
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:235:    working := frontend.Working{
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:236:        Kind:     frontend.WorkingKind(indicator.Kind),
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:74:    // customWorkingMessage: extension-set message shown during streaming.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:76:    customWorkingMessage string
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:126:// SetWorking flips the spinner column on/off.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:127:func (s *FooterComponent) SetWorking(b bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:273:// SetWorkingMessage sets a custom message shown during streaming.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:276:func (s *FooterComponent) SetWorkingMessage(message string) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:278:    s.customWorkingMessage = message
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:283:// GetWorkingMessage returns the current custom working message or "".
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:284:func (s *FooterComponent) GetWorkingMessage() string {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:287:    return s.customWorkingMessage
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:480:func (u *ExtUIContext) SetWorkingMessage(message string) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:481:    u.setWorkingMessage(&message)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:484:// ClearWorkingMessage is Pi's setWorkingMessage() with no argument: it restores the default working label ([extension.WorkingMessageClearer]).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:485:func (u *ExtUIContext) ClearWorkingMessage() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:486:    u.setWorkingMessage(nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:489:func (u *ExtUIContext) setWorkingMessage(message *string) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:491:        u.m.updateStatusOnOwner(func() { u.m.setWorkingMessage(message) })
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:495:func (u *ExtUIContext) SetWorkingVisible(visible bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:497:        u.m.updateStatusOnOwner(func() { u.m.setWorkingVisible(visible) })
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:501:func (u *ExtUIContext) SetWorkingIndicator(options extension.WorkingIndicatorOptions) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:506:    owned := extension.WorkingIndicatorOptions{}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_context.go:513:    u.m.updateStatusOnOwner(func() { u.m.setWorkingIndicator(&owned) })
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/parity_harness.go:78:    loader := tui.NewCancellableLoader(nil, nil, nil, "Working...", nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/parity_harness.go:82:    renderOK := strings.Contains(rendered, "Working...")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_events.go:110:        m.startWorkingLoader()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_events.go:117:        m.stopWorkingLoader()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_events.go:141:                m.startWorkingLoader()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_turn.go:303:    m.statusLine.SetWorking(m.workingVisible)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_turn.go:324:                    m.statusLine.SetWorking(false)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_turn.go:325:                    m.stopWorkingLoader() // belt-and-suspenders: ensure loader is removed on abort
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:359:    activeWorkingIndicatorEmbedded bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:360:    workingIndicatorOptions        *extension.WorkingIndicatorOptions
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1677:    m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:90:    m.activeWorkingIndicatorEmbedded = m.setEditorWorkingStatusIndicator(m.activeStatusIndicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:91:    if !m.activeWorkingIndicatorEmbedded {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:96:// EditorEmbedWorkingStatusChanged applies a component's embedWorkingStatus opt-in, which Pi reads from the editor each time it shows a status; a delegated component declares it after its factory returned.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:97:func (h *remoteEditor) EditorEmbedWorkingStatusChanged() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:121:    if indicator := m.activeStatusIndicator; indicator != nil && m.remoteEditor.editor.EmbedWorkingStatus() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:122:        config.WorkingStatus = &extension.RemoteEditorStatus{
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:50:// options.EmbedWorkingStatus is the editor's readonly embedWorkingStatus (custom-editor.ts:25 `options?.embedWorkingStatus ?? false`).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:58:    editor.EmbedWorkingStatus = options.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:151:// CustomEditorOptions is custom-editor.ts:5-8 CustomEditorOptions = EditorOptions & { embedWorkingStatus?: boolean }. A nil field is an omitted option.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:157:    // EmbedWorkingStatus renders working, compaction, summarization and retry status in the editor's top border; omitted is false.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:158:    EmbedWorkingStatus bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:177:    editor.EmbedWorkingStatus = options.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/extension-api.md:88:- `ui.setWorkingMessage(message)`, `ui.setWorkingVisible(bool)`, `ui.setWorkingIndicator(options)` - control the bordered loader.
    vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/extension-api.md:237:  - The host calls the component's `setWorkingStatusIndicator(status)` when an editor that embeds the working status gets a new status, a new frame of it, or none, and its `invalidate()` when the theme changes. Go: override `Editor.SetWorkingStatusIndicator` and `Editor.Invalidate`. Rust: `EditorComponent::set_working_status_indicator` and `invalidate`. Python: the methods of `EditorComponent`.
    vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/rpc-extension-ui.md:20:- `setWorkingMessage()`, `setWorkingVisible()`, `setWorkingIndicator()`, `setHiddenThinkingLabel()`, `setFooter()`, `setHeader()`, `addAutocompleteProvider()`, `setEditorComponent()` and `setToolsExpanded()` do nothing.
    vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/mcp.md:110:| `--cwd <dir>` | Working directory for a stdio server |
    vendor/github.com/MichaelKinsy/PiG/internal/pigdocs/content/sessions.md:18:## Working with sessions
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/README.md:9:`Context.SetWorkingMessage("")` shows an empty working label, as Pi does; `Context.ClearWorkingMessage()` restores the default label (Pi's `setWorkingMessage()` with no argument).  `Context.ClearHiddenThinkingLabel()` does the same for `SetHiddenThinkingLabel`.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:293:        WorkingStatus *EditorStatus    `json:"workingStatus"`
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:320:        status := payload.WorkingStatus
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:350:// retry status in its top border, as Pi's CustomEditor does with embedWorkingStatus: true. The host then renders the
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:353:    EmbedWorkingStatus() bool
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:387:    if embedder, ok := component.(EditorStatusEmbedder); ok && embedder.EmbedWorkingStatus() {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_component.go:388:        return e.conn.notify("ui.editor.options", map[string]any{"key": key, "embedWorkingStatus": true})
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:216:    SetWorkingMessage      func(message *string)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:217:    SetWorkingVisible      func(visible bool)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:295:    if f := ui.SetWorkingMessage; f != nil {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:296:        named("setWorkingMessage", func(args []json.RawMessage) (any, error) {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:302:    if f := ui.SetWorkingVisible; f != nil {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:303:        named("setWorkingVisible", func(args []json.RawMessage) (any, error) {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:410:// SetWorkingMessage is `ui.setWorkingMessage(message?)`.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:411:func (u *RunnerUIContext) SetWorkingMessage(message *string) error {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:412:    return u.call("setWorkingMessage", message)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:415:// SetWorkingVisible is `ui.setWorkingVisible(visible)`.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:416:func (u *RunnerUIContext) SetWorkingVisible(visible bool) error {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/extension_runner_members.go:417:    return u.call("setWorkingVisible", visible)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:245:// SetWorkingStatusIndicator is CustomEditor's setWorkingStatusIndicator, which the host calls when an editor that embeds the status
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:248:func (e *Editor) SetWorkingStatusIndicator(*EditorStatus) error { return nil }
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:256:    SetWorkingStatusIndicator(status *EditorStatus) error
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:269:func (s *editorSession) setWorkingStatus(status *EditorStatus) error {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:271:        return setter.SetWorkingStatusIndicator(status)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:283:// editorConfigured applies the host's configuration to the component: a changed working status goes to its setWorkingStatusIndicator.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/editor_hooks.go:292:    s.fail("setWorkingStatusIndicator", s.setWorkingStatus(status))
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:28:// WorkingIndicatorOptions mirrors upstream's loose shape. The host treats this
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:30:type WorkingIndicatorOptions = map[string]any
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:377:    "ui.setWorkingMessage": true, "ui.setWorkingVisible": true, "ui.setWorkingIndicator": true, "ui.setHiddenThinkingLabel": true,
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:431:// SetWorkingMessage sets the message shown during LLM streaming. An empty string is a message, shown as an empty label, as in Pi
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:432:// (`message ?? defaultWorkingMessage`); [Context.ClearWorkingMessage] restores the default label.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:433:func (c Context) SetWorkingMessage(message string) {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:434:    _, _ = c.callHost("ui.setWorkingMessage", map[string]string{
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:439:// ClearWorkingMessage restores the default working label: Pi's setWorkingMessage() with no argument.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:440:func (c Context) ClearWorkingMessage() {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:441:    _, _ = c.callHost("ui.setWorkingMessage", map[string]string{})
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:444:// SetWorkingVisible toggles whether the working indicator is shown.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:445:func (c Context) SetWorkingVisible(visible bool) {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:446:    _, _ = c.callHost("ui.setWorkingVisible", map[string]bool{
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:451:// SetWorkingIndicator configures the interactive working indicator.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:452:func (c Context) SetWorkingIndicator(options WorkingIndicatorOptions) error {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/context.go:453:    result, err := c.callHost("ui.setWorkingIndicator", options)
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:17:// [Working] node, the footer as a [Footer] node and an overlay as an
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:416:// [Thinking], [Editor], [Selector], [Settings], [Working], [Footer] and
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:445:// [Working] node "working" while a working indicator shows, the editor (or
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:688:// WorkingKind is what a working indicator reports.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:689:type WorkingKind string
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:692:    // WorkingAgent is the agent working on a turn.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:693:    WorkingAgent WorkingKind = "working"
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:694:    // WorkingCompaction is a manual or automatic compaction.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:695:    WorkingCompaction WorkingKind = "compaction"
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:696:    // WorkingBranchSummary is the summary of a branch the user leaves.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:697:    WorkingBranchSummary WorkingKind = "branchSummary"
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:698:    // WorkingRetry is the countdown to retrying a failed request.
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:699:    WorkingRetry WorkingKind = "retry"
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:702:// Working is the indicator PiG shows while something runs, which its
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:705:type Working struct {
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:706:    Kind WorkingKind
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:707:    // Message is the text after the spinner, as PiG draws it: "Working" or
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk/frontend/frontend.go:799:func (Working) frontendNode()      {}
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:1799:    FMinimumWorkingSetSize   TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:1800:    FMaximumWorkingSetSize   TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:3108:    FMinimumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:3109:    FMaximumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:3119:    FMinimumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:3120:    FMaximumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows_386.go:3124:    FWorkingSetLimit       TSIZE_T
    vendor/github.com/MichaelKinsy/PiG/extensions/sdk-rs/README.md:9:`Context::set_working_message("")` shows an empty working label, as Pi does, and `Context::clear_working_message()` restores the default label (Pi's `setWorkingMessage()` with no argument); `clear_hidden_thinking_label()` does the same for `set_hidden_thinking_label`.
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:223:func (u *graphUIContext) SetWorkingMessage(message string) {
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:224:    if u.ids["setWorkingMessage"] == "" {
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:225:        u.UIContext.SetWorkingMessage(message)
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:228:    u.void("setWorkingMessage", message)
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:231:func (u *graphUIContext) SetWorkingVisible(visible bool) {
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:232:    if u.ids["setWorkingVisible"] == "" {
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:233:        u.UIContext.SetWorkingVisible(visible)
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:236:    u.void("setWorkingVisible", visible)
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:409:    case "setWorkingMessage":
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:411:        ui.SetWorkingMessage(message)
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:412:    case "setWorkingVisible":
    vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go:414:        ui.SetWorkingVisible(visible)
    vendor/github.com/MichaelKinsy/PiG/coding/mcpext/cli.go:117:  --cwd <dir>             Working directory for a stdio server
    vendor/github.com/MichaelKinsy/PiG/coding/extension/opaque_types.go:103:// WorkingIndicatorOptions mirrors upstream WorkingIndicatorOptions.
    vendor/github.com/MichaelKinsy/PiG/coding/extension/opaque_types.go:104:type WorkingIndicatorOptions struct {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/remote_editor.go:35:    // EmbedWorkingStatus reports whether this editor wants status in its border.
    vendor/github.com/MichaelKinsy/PiG/coding/extension/remote_editor.go:36:    EmbedWorkingStatus() bool
    vendor/github.com/MichaelKinsy/PiG/coding/extension/remote_editor.go:62:    // WorkingStatus is the current border status; nil clears it.
    vendor/github.com/MichaelKinsy/PiG/coding/extension/remote_editor.go:63:    WorkingStatus *RemoteEditorStatus `json:"workingStatus"`
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:13:// WorkingMessageClearer is implemented by a [UIContext] that tells an absent working message from an empty one. Pi's setWorkingMessage(message?: string) shows
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:14:// `message ?? defaultWorkingMessage`: no argument restores the default label and an empty string is a message. ClearWorkingMessage is the no-argument form; the
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:15:// extension host calls it when an extension sends no message, and falls back to SetWorkingMessage("") for a UIContext that does not implement it. It is a separate
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:17:type WorkingMessageClearer interface {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:18:    ClearWorkingMessage()
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:21:// HiddenThinkingLabelClearer is implemented by a [UIContext] that tells an absent hidden-thinking label from an empty one, as [WorkingMessageClearer] does for the working message: Pi's
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:125:    // SetWorkingMessage sets the working/loading message shown during streaming. A UIContext that implements [WorkingMessageClearer] shows an empty
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:126:    // message as an empty label, as Pi's `message ?? defaultWorkingMessage` does (interactive-mode.ts:2641-2646); one that does not treats
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:129:    SetWorkingMessage(message string)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:131:    // SetWorkingVisible toggles whether the working/loading indicator is shown.
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:133:    SetWorkingVisible(visible bool)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:135:    // SetWorkingIndicator configures the interactive working
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:138:    SetWorkingIndicator(opts WorkingIndicatorOptions)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:332:func (*noopUIContext) SetWorkingMessage(string)                                        {}
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:333:func (*noopUIContext) SetWorkingVisible(bool)                                          {}
    vendor/github.com/MichaelKinsy/PiG/coding/extension/ui.go:334:func (*noopUIContext) SetWorkingIndicator(WorkingIndicatorOptions)                     {}
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/host_calls.go:61:    case "ui.notify", "ui.setStatus", "ui.setWorkingMessage", "ui.setWorkingIndicator", "ui.setWorkingVisible", "ui.setHiddenThinkingLabel", CallOwnThinkingRuns,
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/types.go:288:    Folder   string `yaml:"folder,omitempty"`   // container path; default image WorkingDir then /workspace
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:1731:    FMinimumWorkingSetSize   TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:1732:    FMaximumWorkingSetSize   TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:2918:    FMinimumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:2919:    FMaximumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:2927:    FMinimumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:2928:    FMaximumWorkingSetSize TSIZE_T
    vendor/modernc.org/sqlite/lib/sqlite_windows.go:2931:    FWorkingSetLimit       TSIZE_T
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/protocol.go:482:    Cwd    string            `json:"cwd"`              // Working directory
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1300://     ui.setWidget, ui.setWorkingIndicator, ui.setWorkingMessage,
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1365:        case "ui.notify", "ui.setStatus", "ui.setWorkingIndicator", "ui.setWorkingMessage",
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1366:            "ui.setWorkingVisible", "ui.setHiddenThinkingLabel", "ui.setWidget", "ui.setFooter",
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1390:    case "ui.setWorkingIndicator":
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1391:        return b.handleSetWorkingIndicator(ui, call.Args)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1392:    case "ui.setWorkingMessage":
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1393:        return b.handleSetWorkingMessage(ui, call.Args)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1394:    case "ui.setWorkingVisible":
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:1395:        return b.handleSetWorkingVisible(ui, call.Args)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2654:func (b *UIBridge) handleSetWorkingIndicator(ui extension.UIContext, args json.RawMessage) (*CallResultPayload, error) {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2655:    var opts extension.WorkingIndicatorOptions
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2657:        return nil, fmt.Errorf("parse setWorkingIndicator args: %w", err)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2659:    ui.SetWorkingIndicator(opts)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2663:func (b *UIBridge) handleSetWorkingMessage(ui extension.UIContext, args json.RawMessage) (*CallResultPayload, error) {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2669:        return nil, fmt.Errorf("parse setWorkingMessage args: %w", err)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2672:        if clearer, ok := ui.(extension.WorkingMessageClearer); ok {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2673:            clearer.ClearWorkingMessage()
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2676:        ui.SetWorkingMessage("")
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2679:    ui.SetWorkingMessage(*p.Message)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2683:func (b *UIBridge) handleSetWorkingVisible(ui extension.UIContext, args json.RawMessage) (*CallResultPayload, error) {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2688:        return nil, fmt.Errorf("parse setWorkingVisible args: %w", err)
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/ui_bridge.go:2690:    ui.SetWorkingVisible(p.Visible)
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:107:    WorkingDir string
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:192:    workspaceFolder := resolveWorkspaceFolder(effective, image.WorkingDir)
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:326:    output, err := commands.Output(ctx, engine, []string{"image", "inspect", "--format", "{{.Id}}\t{{.Os}}\t{{.Architecture}}\t{{.Config.WorkingDir}}", imageReference})
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:339:    return inspectedRuntimeImage{ID: strings.TrimSpace(parts[0]), Platform: strings.TrimSpace(parts[1]) + "/" + strings.TrimSpace(parts[2]), WorkingDir: workingDir}, nil
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:343:// to: an explicit Piglet folder, else the image WorkingDir, else /workspace.
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:344:func resolveWorkspaceFolder(effective *AgentEnvironment, imageWorkingDir string) string {
    vendor/github.com/MichaelKinsy/PiG/coding/piglet/runtime.go:348:    if dir := strings.TrimSpace(imageWorkingDir); dir != "" && dir != "/" && strings.HasPrefix(dir, "/") {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:296:// ClearWorkingMessage restores the default working label (Pi's setWorkingMessage() with no argument) through the bound UI's own
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:297:// [extension.WorkingMessageClearer]; for a bound UI without one it sends the empty message, as the subprocess bridge does. Embedding the bound UI
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:299:func (c *uiPromptContext) ClearWorkingMessage() {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:300:    if clearer, ok := c.UIContext.(extension.WorkingMessageClearer); ok {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:301:        clearer.ClearWorkingMessage()
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/inproc/ui_prompt.go:304:    c.SetWorkingMessage("")
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:57:    EmbedWorkingStatus    bool `json:"embedWorkingStatus,omitempty"`
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:73:    embedWorkingStatus bool
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:93:func (p *editorProxy) EmbedWorkingStatus() bool {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:96:    return p.embedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:269:        p.embedWorkingStatus = payload.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:272:            if options, ok := host.(interface{ EditorEmbedWorkingStatusChanged() }); ok {
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:273:                options.EditorEmbedWorkingStatusChanged()
    vendor/github.com/MichaelKinsy/PiG/coding/extension/host/subprocess/editor_proxy.go:316:        proxy := &editorProxy{ext: extName, conn: owner, key: payload.Key, embedWorkingStatus: payload.EmbedWorkingStatus, delegated: payload.Delegated, bound: make(chan struct{})}
    vendor/github.com/MichaelKinsy/PiG/coding/cli/main.go:19://    --cwd <dir>                 Working directory (default: current dir)
    vendor/github.com/MichaelKinsy/PiG/coding/cli/main.go:1016:    // Working directory is process.cwd() (no CLI override; matches upstream).
    vendor/github.com/MichaelKinsy/PiG/coding/cli/rpc_ui.go:397:func (*rpcUIContext) SetWorkingMessage(string)                              {}
    vendor/github.com/MichaelKinsy/PiG/coding/cli/rpc_ui.go:398:func (*rpcUIContext) SetWorkingVisible(bool)                                {}
    vendor/github.com/MichaelKinsy/PiG/coding/cli/rpc_ui.go:399:func (*rpcUIContext) SetWorkingIndicator(extension.WorkingIndicatorOptions) {}
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:140:    // Working returns the working indicator the dock reports above the
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:142:    Working func() (frontend.Working, bool)
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:596:    var working frontend.Working
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:597:    hasWorking := false
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:598:    if t.hooks.Working != nil {
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:599:        working, hasWorking = t.hooks.Working()
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:636:        if _, ok := c.(*StatusIndicator); ok && hasWorking {
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:658:    if hasWorking {
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:1138:// the most frequent node, the message and editor nodes, and the Working,
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:1157:    case frontend.Working:
    vendor/github.com/MichaelKinsy/PiG/tui/tui_surface.go:1158:        b, ok := b.(frontend.Working)
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:10:// SetWorkingStatusIndicator selects the status displayed by an opted-in editor.
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:11:func (e *Editor) SetWorkingStatusIndicator(indicator *StatusIndicator) {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:26:    return e.EmbedWorkingStatus && indicator != nil && indicator.IsDirty()
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:32:    if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:41:    return e.EmbedWorkingStatus && e.workingStatusIndicator != nil
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:47:    if !e.EmbedWorkingStatus || e.workingStatusIndicator == nil || width <= 0 {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:51:    // Embedded working and compaction indicators take the editor's border color (interactive-mode.ts showWorkingStatusIndicator
    vendor/github.com/tetratelabs/wazero/fsconfig.go:33://   - Working directories are typically tracked in wasm, though possible some
    vendor/github.com/MichaelKinsy/PiG/tui/program_status.go:24:    ProgramStateWorking ProgramState = "working"
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:32:    // EmbedWorkingStatus opts into the coding-agent status border.
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:33:    EmbedWorkingStatus     bool
    vendor/github.com/tetratelabs/wazero/api/features.go:52:    // proposal due to the WebAssembly Working Group merging them
    vendor/github.com/tetratelabs/wazero/api/features.go:117:    // proposal due to the WebAssembly Working Group merging them
    vendor/github.com/tetratelabs/wazero/RATIONALE.md:1036:Working around these issues causes expense to all users of wazero, so we'd
    package main
    
    import (
        "os"
    
        _ "github.com/MichaelKinsy/PiG/ai/all" // Register bundled providers, including Amazon Bedrock.
        "github.com/MichaelKinsy/PiG/coding/cli"
        _ "github.com/MichaelKinsy/PiG/coding/extension/host/lispcell" // Register the stock Lisp extension builder.
    
        _ "modernc.org/sqlite" // Register the native driver used by shared Pi/OMP credentials.
    )
    
    func main() {
        // Delegate arguments, terminal ownership, signals, and exit status to PiG.
        // The vendored personal runtime supplies the defaults; keep the complete CLI.
        cli.Main(os.Args[1:], nil)
    }
    module ompig-slim
    
    go 1.27.0
    
    toolchain go1.27.3
    
    // Match PiG's Windows junction behavior; dependency godebug directives do not propagate.
    godebug winsymlink=0
    
    require github.com/MichaelKinsy/PiG v0.7.0
    
    require (
        cloud.google.com/go/compute/metadata v0.9.0 // indirect
        github.com/BurntSushi/toml v1.6.0 // indirect
        github.com/MichaelKinsy/PiG/extensions/sdk v0.7.0 // indirect
        github.com/Microsoft/go-winio v0.6.4 // indirect
        github.com/aws/aws-sdk-go-v2 v1.47.1 // indirect
        github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.10 // indirect
        github.com/aws/aws-sdk-go-v2/credentials v1.19.16 // indirect
        github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
        github.com/aws/smithy-go v1.28.1 // indirect
        github.com/charmbracelet/x/ansi v0.11.7 // indirect
        github.com/clipperhouse/displaywidth v0.11.0 // indirect
        github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
        github.com/dlclark/regexp2 v1.12.0 // indirect
        github.com/dustin/go-humanize v1.0.1 // indirect
        github.com/fsnotify/fsnotify v1.5.4 // indirect
        github.com/gofrs/flock v0.13.0 // indirect
        github.com/google/uuid v1.6.0 // indirect
        github.com/gorilla/websocket v1.5.3 // indirect
        github.com/klauspost/compress v1.19.2 // indirect
        github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
        github.com/mattn/go-isatty v0.0.20 // indirect
        github.com/mattn/go-runewidth v0.0.23 // indirect
        github.com/ncruces/go-strftime v1.0.0 // indirect
        github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
        github.com/tetratelabs/wazero v1.12.0 // indirect
        go.yaml.in/yaml/v3 v3.0.5 // indirect
        golang.org/x/mod v0.41.0 // indirect
        golang.org/x/net v0.62.0 // indirect
        golang.org/x/oauth2 v0.37.0 // indirect
        golang.org/x/sys v0.49.0 // indirect
        golang.org/x/term v0.47.0 // indirect
        golang.org/x/text v0.43.0 // indirect
        modernc.org/libc v1.73.4 // indirect
        modernc.org/mathutil v1.7.1 // indirect
        modernc.org/memory v1.11.0 // indirect
        modernc.org/sqlite v1.53.0 // indirect
    )
    vendor/github.com/MichaelKinsy/PiG/AGENTS.md
    vendor/github.com/clipperhouse/displaywidth/AGENTS.md
    vendor/github.com/MichaelKinsy/PiG/ai/AGENTS.md
    vendor/github.com/aws/smithy-go/AGENTS.md
    vendor/github.com/MichaelKinsy/PiG/coding/extension/AGENTS.md
    vendor/github.com/MichaelKinsy/PiG/tui/AGENTS.md
    vendor/github.com/MichaelKinsy/PiG/coding/AGENTS.md
    ✓ • 66ms

## Activity

    $ cat vendor/github.com/MichaelKinsy/PiG/AGENTS.md vendor/github.com/MichaelKinsy/PiG/tui/AGENTS.md; nl -ba vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go | head -140; rg -n 'workingStatus|render.*Border|topBorder|statusBorder' vendor/github.com/MichaelKinsy/PiG/tui/editor*; nl -ba vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go | sed -n '65,100p'; rg -n 'EmbedWorkingStatus|SetWorkingStatusIndicator' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/{interactive*,custom_editor*}
    # AGENTS.md
    
    Operating rules for AI agents maintaining `pig`, the Go port of upstream `pi`. This file is context, not human marketing. Keep it small, current, and enforceable.
    
    ## Mission
    
    `pig` must match upstream `pi` observable behavior unless `docs/parity/DIVERGENCES.md` records a numbered, scrutinized exception. Treat the port as a compiler from upstream TypeScript behavior to Go behavior: `docs/parity/PORT_MAP.md` defines the file map, parity scenarios define behavior, `test/parity/coverage.md` reports proof, and gates keep the claims honest.
    
    Source of truth:
    - `.upstream/current/` is the upstream mirror.
    - `.upstream/pin` is the one upstream pin: a Pi release tag, or a pinned pi-mono commit (`ref`, `commit`, `package_version`, display `version` such as `1.1.0+42a3497d`). `internal/coding/pigversion/pigversion.go` is generated from it (re-exported by `coding/upstream.go` as `coding.UpstreamVersion`, `coding.UpstreamPackageVersion`, `coding.UpstreamCommit`). `PI_UPSTREAM_REF=<tag|commit>` overrides the pin for one run, and `make pin-bump PI_UPSTREAM_REF=<tag|commit>` moves it and regenerates what derives from it (`docs/parity/upstream-pin.md`).
    - `docs/parity/PORT_MAP.md` maps upstream files to Go files, deferred entries, or designed-out entries.
    - `test/parity/scenarios/<family>/*.toml` define observed behavior.
    - `test/parity/interfaces/upstream-v<version>.json` is the compiler-derived public
      package interface denominator; observable adapter inventories such as
      `cli-v<version>.json` extend it for non-package user contracts;
      `mapping-v<version>.json` records each Pig disposition and closure evidence. Generated inventory drift and strict mapping
      are Foundation gates. PORT_MAP remains the file navigation roll-up until
      Foundation I finishes generating its statuses from these ledgers.
    - `test/parity/upstream-sync/v<version>.toml` accounts for every changed tracked source file in a version leap.
    - `test/parity/async-contracts.toml` accounts for Promise/async semantics across the complete pinned upstream source tree.
    - `test/parity/coverage.md` and the generated block below report verification.
    - `docs/parity/DIVERGENCES.md` records intentional differences.
    
    No prose status claim overrides those files.
    
    If you change exported Go API, CLI flags, settings, parity scenarios, or docs mirrors, run `make generate` and commit the result. CI's drift gates compare these files. Never hand-edit generated files; regenerate.
    
    When landing a new port, record its full upstream source path in a production
    `// Ports packages/.../file.ts` comment and update PORT_MAP in the same change.
    The drift gate rejects explicit port claims left not-started, deferred, or n/a.
    Use partial until behavioral closure is reviewed; source existence is not proof.
    
    ## Product-extension boundary
    
    Stock PiG contains upstream-parity behavior and the inert, product-neutral mechanisms needed to discover, validate, select, build, verify, load, isolate, and run Packages, Resources, extensions, Piglets, and Piglet Binaries. A Piglet selects and activates product behavior. PiG Standard owns branded presentation, opinionated defaults, selected extensions, onboarding, games, and other optional product workflows.
    
    Classify every proposed additive capability before implementation:
    
    1. `required substrate` remains in Stock PiG.
    2. `inert capability` remains in Stock PiG only when an extension or Piglet must select it.
    3. Composition, presentation, policy, and branded behavior belong in PiG Standard or another explicit Piglet or Package.
    4. Caller-free or unneeded behavior is deleted.
    5. Behavior that overlaps Pi but differs observably is fixed or recorded in `docs/parity/DIVERGENCES.md` after explicit approval.
    
    Use `product-neutral diagnostics` for generic reports that describe Stock mechanisms. Use `explicit unsafe opt-in` only for a capability that stays off by default, is visible at configuration boundaries, and must not be selected by PiG Standard.
    
    A Piglet cannot supply the parser, resolver, verifier, SDK bridge, isolation boundary, or build machinery needed to load itself. Do not move those bootstrap mechanisms into PiG Standard. Do not activate Standard behavior in Stock PiG. Do not put product UI, Piglet-specific recommendations, workflow-specific logic, or linked extension products under `internal/` or `cmd/` except for the minimum generic host capability.
    
    Additive records use `docs/additive-features.md`. Observable Stock PiG differences use `docs/parity/DIVERGENCES.md`. Place one short typed source marker at each production decision point. Tests do not satisfy the source-marker requirement.
    
    Change the version only with `make set-version VERSION=x.y.z`.
    
    ## Pig-owned format and update policy
    
    Pig-owned source, manifest, record, graph/plan, builder, Marketplace,
    publication, and subprocess formats have one strict current unversioned shape
    while Pig and its clients upgrade atomically. Release/content identity, exact
    upstream-owned schema versions, and external-standard versions remain. Do not
    add a Pig-owned format discriminator, negotiation, compatibility reader,
    migration, redirect, or dual write unless exact upstream regularly versions the
    equivalent schema or the user explicitly approves independently deployed
    compatibility.
    
    Self-update preserves Pi's command routing but uses one proven native ownership
    tier per operation. Standalone download, package-manager, immutable
    Piglet Binary, OCI/Image, and unsupported/read-only paths must
    not substitute for one another after work starts. Downloads require complete
    checksum and overflow-safe size verification; startup checks remain bounded and
    best-effort, while explicit update failures remain actionable.
    
    ## Extension API porting rules
    
    The pig extension system is a **port** of the upstream pi extension API,
    not a replacement for it. Two downstream-only constructs exist so we can
    stay subprocess-only without giving up the pi extension API:
    
    - **Multi-language SDK bridges** (`extensions/sdk`, `extensions/sdk-rs`,
      `extensions/sdk-py`): see `docs/additive-features.md` D19 and
      `docs/extension-api-parity.md` “Parity boundary”. All three SDKs target the
      **same current wire contract** in
      `coding/extension/host/subprocess/protocol.go` and the same
      registration/host-call/tool-result shapes. The wire has no independent
      version or compatibility negotiation. The conformance suite in
      `test/extension-conformance/` is the
      authority: a new SDK feature is only “done” when it matches the
      in-process Go reference there.
    
      Every SDK exposes the same extension API. Separate the three axes:
    
      - **Interface**: the set of capabilities and their parameters and return
        meaning. Identical across SDKs, modulo the naming convention.
      - **Behavior**: what a call returns for a given host state, including
        error and edge cases. Identical across SDKs.
      - **Implementation**: how an SDK produces that answer: host call versus
        replicated state, sync versus async internals, threading, allocation,
        borrowing, error representation. Free to differ per language.
    
      Implementation variance is expected and needs no record. Interface or
      behavior variance is drift. Returning a constant where another SDK
      returns host data is a behavior divergence even though the interface
      matches, and it is worse than a missing method: a missing method fails
      loudly at the call site, while a plausible constant is silently believed.
      A stub is therefore not an acceptable resting state, and "the host call
      is async but this API is sync" is a reason to replicate the value into
      state, not a reason to fabricate one.
    
      An extension is portable between SDKs by rewriting it, not by
      rearchitecting it or dropping a feature. A capability landing in one SDK
      is unfinished until it lands in all of them with a conformance row that
      fails when any one is broken. Bind that row to a value distinguishable
      from each SDK's fallback, or an SDK that never makes the call passes by
      defaulting.
    - **Packed runtime cells** (`coding/extension/host/subprocess/cell_plan.go`,
      `coding/extension/host/runtimecell/`): see `docs/additive-features.md` D20. These
      are a host-side optimization. Each contained extension keeps its own
      socket and its own `register` handshake. Packed mode must
      be observationally identical to isolated mode from the extension’s
      point of view; if a difference appears, fix the host or add a numbered
      divergence: do not change the SDK API to paper over it.
    
    Do not, without an explicit approved spec:
    - add a WASM or embedded-JS extension runtime,
    - add a dynamic Go plugin / `plugin.Open` extension loader,
    - add a linked/in-process production extension runtime, except the compiled-in extension factory form of `docs/specs/extension-factory-trust.md`: an `extension.ExtensionFactory` that the source of the binary's own `main` package passes to the loader by value (Stock PiG's built-in rows, or an embedding program's own factories). A Piglet member, including a D31 fused extension, never uses it. Settings and `-e builtin:<name>` may select or disable a registered built-in by name, as Pi does; no user, project, Package, file, settings entry, Piglet, or wire peer can supply or add factory code,
    - introduce multi-register (`RegisterPayload.Extensions`,
      `RequestPayload.TargetExtension`): packed cells deliberately stay on
      one socket and one registration per extension,
    - diverge SDK semantics from the in-process Go reference without first
      numbering the divergence and citing a failing conformance test.
    
    When adding a host-side feature, first read `docs/extension-authoring.md`.
    The implementation order is: upstream pi behavior → protocol shape in
    `protocol.go` → host wiring in `coding/extension/host/subprocess/` → SDK
    ergonomics in each `extensions/sdk*/` → conformance test row → parity
    matrix row in `docs/extension-api-parity.md` → numbered divergence if
    needed.
    
    `extensions/sdk-ts` is a declaration-only adapter. It re-exports the exact
    pinned Pi extension declarations and augments them with PiG-only types. The
    Node subprocess runtime remains the implementation, so do not duplicate Pi's
    TypeScript runtime in this package. Keep its Pi dependency equal to
    `coding.UpstreamVersion` and verify it with `make test-sdk-ts`.
    
    Extension authoring has two source forms. A conventional factory uses the one
    standard symbol for its language; an exact standalone is always isolated. Pig
    derives wire, SDK, runner, placement, and registration details. Runtime
    registration owns identity and capabilities. Do not add an authored extension
    manifest or another global extension configuration plane. Isolated, packed, and
    fused realizations of one factory must pass the same conformance behavior.
    
    ## TypeScript-to-Go semantic translation
    
    Read `docs/typescript-to-go-porting.md` before translating a new behavior
    family or changing a parity-facing Go representation. Port observed semantics,
    not TypeScript syntax: account explicitly for omitted/undefined/null state,
    closed unions, insertion order, UTF-16/code-point/terminal-cell units, numeric
    conversion, exceptions, callback ownership, Node path/URL/regex/JSON behavior,
    and every state or wire effect that callers observe. A TypeScript `interface`
    does not imply a Go interface; use a struct for data and a small consumer-owned
    Go interface only for a real behavioral contract. Constructors normally return
    concrete types. "Port semantics, not syntax" governs behavior, not naming: it is
    not license to rename. Mirror upstream identifiers and structure 1:1 (camelCase →
    exported PascalCase, same literal union values, same field names), and diverge
    only where a Go language mechanic forces it (no `Symbol.for`, no field-bearing
    interfaces, discriminated union → sealed interface), recording each forced case.
    A needless rename is drift that inflates the next upstream sync and obscures
    correspondence. Do not change a public extension or TUI contract for presumed
    speed: profile the representative path and prove allocations/inlining and
    byte-faithful behavior before and after.
    
    ## Faithful, general implementations
    
    Port Pi's design, not one case's output. Use Pi's shared data (model catalog, `defaultModelPerProvider`, auth metadata) and one shared function where Pi has one. Never hard-code a provider or model in shared code; cite the Pi line when Pi itself special-cases one. Test shared paths with several provider shapes, never only Copilot or `test-faux`, and fix a special case's siblings together.
    
    ## TypeScript async/Promise parity
    
    Treat every upstream `async`, `Promise`, `.then`, `Promise.all`, `Promise.race`, and intentionally unawaited call as control-flow behavior, not syntax to erase. Before porting it, record whether the caller waits, what ordering is guaranteed, how cancellation and errors propagate, whether work is concurrent, and which executor/event loop owns callbacks. Then preserve that contract in Go:
    
    - an awaited Promise normally maps to a blocking Go call that returns its value/error; do not add `go func()` merely because TypeScript says `async`;
    - `Promise.all`/`allSettled` maps to owned, joined concurrency with deterministic result/error handling, not detached goroutines;
    - an intentionally unawaited Promise maps only to an owned background task with cancellation, error reporting, and shutdown draining; naked fire-and-forget goroutines are not equivalent;
    - Promise rejection maps to a surfaced Go error on the same operation unless upstream explicitly handles it;
    - `AbortSignal` and request/session lifetime map to `context.Context` or an equivalent cancellation channel;
    - async TUI/event handlers perform slow work off the UI loop and marshal UI mutation back through the loop's approved run-on-main path.
    
    Probe upstream call sites, not only the declared return type: an `async` function with no `await` may still create a microtask boundary, while a Promise-returning callback may be awaited by its host. Regression evidence must prove completion ordering, cancellation, and error propagation; output-only happy-path tests do not prove async parity. When adding or changing an extension API, record the async contract in `docs/extension-api-parity.md` and exercise isolated and packed subprocess modes where applicable.
    
    ## Reward function
    
    Highest-value work, in order:
    1. Find a pig-vs-pi behavior difference and fix it at the source, or number it in `docs/parity/DIVERGENCES.md`.
    2. Tighten an existing scenario comparator: `wait_contains` → `output_normalized_equal` → `output_equal` → `escaped_output_equal`, or increase `runs` where flake risk matters.
    3. Promote silent drift to a numbered divergence with call-site markers, remove condition, and parity coverage or explicit allowance.
    4. Add honest file coverage where the asserted output actually exercises the claimed `covers` paths.
    
    Low-value or misleading work: green check counts, scenario counts, `covers` entries not exercised by the assertion, normalized output hiding a real rendering difference, `runs = 1` for non-trivial behavior, renderer band-aids when the bug is upstream-source logic, and stopping because `make check` is green.
    
    ## Loop smells
    
    Stop and report instead of claiming success when:
    - Everything passed on the first try.
    - A scenario passed before and after the supposed fix.
    - The loop produced zero bugs, zero comparator tightenings, and zero numbered divergences.
    - A scenario claims coverage but asserts only banner, prompt, or ready text.
    - ANSI or whitespace was normalized to hide a real difference.
    - `family-gaps` is green only because one broad scenario claims many files.
    - A comment explains a rendering difference but no divergence number exists.
    
    Investigation is the work. Green is necessary, not sufficient.
    
    ## Loop self-report
    
    Before `signal_loop_success`, include this block in the final assistant message and fill it honestly:
    
    ```md
    ### Loop self-report
    
    Family: <name>
    
    Bugs found and fixed at the source (count: N):
      - <cause> → <fix> @ <file:line>
    
    Comparators tightened (count: N):
      - <scenario>: <before> → <after>
    
    Divergences numbered in docs/parity/DIVERGENCES.md (count: N):
      - D<N> <one-line>
    
    Lint suppressions added or changed (count: N):
      - <rule> @ <file/config>: <why source fix would be less faithful/correct>
    
    New file coverage (count: N PORT_MAP entries newly ✅):
      - <upstream path>
    
    Band-aids consciously chosen (count: N, should be 0):
      - <site>: <symptom>: <why root fix deferred>: <follow-up>
    
    Scope expansion (count: N files outside nominal family):
      - <file>: <reason>: <call sites audited>
    
    Cross-family scenarios re-probed/tightened (count: N):
      - <family>/<scenario>: <what changed and why>
    
    Smells investigated (count: N):
      - <smell> → <finding>
    
    Smells deferred to user (count: N, should be 0 unless blocked):
      - <smell>: <blocker>
    ```
    
    A report with all zero counts is a failed loop unless the user explicitly accepts that outcome.
    
    ## Coverage
    
    The block below is generated by `make coverage
    make foundation-check
    make interface-inventory
    make interface-inventory-test
    make interface-inventory-drift
    make interface-go-drift
    make interface-recommendations-drift
    make interface-recommendations
    make interface-mapping-quality
    make interface-mapping-strict
    make upstream-delta
    make async-contracts` or `make verify` from `docs/parity/PORT_MAP.md` and `test/parity/scenarios/**/*.toml`. Do not hand-edit between the markers. Fix `test/parity/cmd/coverage/main.go` if the math is wrong.
    
    <!-- BEGIN COVERAGE -->
    <!--
      Machine-generated by `make coverage`. Do not hand-edit.
      This block is the ONLY status claim AGENTS.md makes about the
      port. Everything else is invariant rule, not progress narrative.
    -->
    
    **Porting:** 910 / 929 intended-portable entries ✅ (98.0%); **Verification:** 837 behavioral (92.0%), 73 untested.
    Raw PORT_MAP rows: 1032. Breakdown: 103 n/a (designed out) · 19 🟡 partial. See docs/parity/DIVERGENCES.md for the documented exceptions.
    
    | package | src files | n/a | intended | ✅ | 🟡 | ⬜ | ported | behavioral |
    |---|---:|---:|---:|---:|---:|---:|---:|---:|
    | `agent` | 6 | 1 | 5 | 5 | 0 | 0 | 100.0% | 100.0% |
    | `ai` | 265 | 22 | 243 | 243 | 0 | 0 | 100.0% | 98.8% |
    | `chord` | 247 | 28 | 219 | 213 | 6 | 0 | 97.3% | 86.9% |
    | `codemode` | 17 | 4 | 13 | 13 | 0 | 0 | 100.0% | 100.0% |
    | `coding-agent` | 256 | 27 | 229 | 228 | 1 | 0 | 99.6% | 99.6% |
    | `durable` | 97 | 10 | 87 | 76 | 11 | 0 | 87.4% | 98.7% |
    | `env` | 23 | 3 | 20 | 19 | 1 | 0 | 95.0% | 100.0% |
    | `evals` | 6 | 0 | 6 | 6 | 0 | 0 | 100.0% | 100.0% |
    | `mcp` | 18 | 3 | 15 | 15 | 0 | 0 | 100.0% | 100.0% |
    | `schema` | 43 | 3 | 40 | 40 | 0 | 0 | 100.0% | 0.0% |
    | `telemetry` | 8 | 1 | 7 | 7 | 0 | 0 | 100.0% | 100.0% |
    | `tui` | 46 | 1 | 45 | 45 | 0 | 0 | 100.0% | 100.0% |
    
    - **Four core packages (agent, ai, coding-agent, tui):** 521 / 522 intended-portable files ✅ (99.8%) from 573 rows; 517 behavioral (99.2%), 0 weak-only, 4 untested.
    - **Shipped surface (agent, ai, codemode, coding-agent, mcp, schema, telemetry, tui; coding-agent without ):** 596 / 597 intended-portable files ✅ (99.8%) from 659 rows; 552 behavioral (92.6%), 0 weak-only, 44 untested.
    - **Whole monorepo (12 packages):** 910 / 929 intended-portable files ✅ (98.0%) from 1032 rows; 837 behavioral (92.0%), 0 weak-only, 73 untested.
    - **Interface closure:** 14466 of 14469 semantic interface IDs closed (395 designed-out, 3 pending, 14071 ported). A file-level ✅ does not close an interface ID.
    
    Behavioral evidence includes paired scenarios and reviewed mutation-proven unit tests; the family table below counts paired scenarios only.
    Weak scenarios not counted as behavioral verification: 4 boot-only, 4 registration-only, 1 smoke-only.
    
    | family | scenarios | behavioral | boot-only | weak | deferred | upstream behavioral covered | last run |
    |---|---:|---:|---:|---:|---:|---:|---|
    | `_top` | 1 | 1 | 0 | 0 | 0 | 1 | not run |
    | `ai-sdk` | 3 | 3 | 0 | 0 | 0 | 90 | not run |
    | `autocomplete` | 14 | 14 | 0 | 0 | 0 | 5 | not run |
    | `cli-utils` | 38 | 38 | 0 | 0 | 0 | 17 | not run |
    | `clipboard-images` | 7 | 7 | 0 | 0 | 2 | 11 | not run |
    | `compaction` | 15 | 15 | 0 | 0 | 0 | 10 | not run |
    | `export-html` | 7 | 7 | 0 | 0 | 0 | 2 | not run |
    | `extension-host` | 1 | 1 | 0 | 0 | 0 | 2 | not run |
    | `extensions-runtime` | 143 | 142 | 0 | 1 | 0 | 63 | not run |
    | `footer` | 12 | 12 | 0 | 0 | 0 | 9 | not run |
    | `fullscreen` | 14 | 14 | 0 | 0 | 0 | 11 | not run |
    | `interactive-rendering` | 73 | 72 | 1 | 0 | 0 | 37 | not run |
    | `json` | 14 | 14 | 0 | 0 | 0 | 10 | not run |
    | `model-resolver-selector` | 28 | 28 | 0 | 0 | 0 | 16 | not run |
    | `model-runtime-store-catalog` | 25 | 25 | 0 | 0 | 0 | 24 | not run |
    | `oauth` | 32 | 32 | 0 | 0 | 0 | 19 | not run |
    | `print` | 13 | 13 | 0 | 0 | 0 | 9 | not run |
    | `project-trust` | 26 | 26 | 0 | 0 | 0 | 19 | not run |
    | `providers-faux-streaming` | 66 | 65 | 0 | 1 | 0 | 49 | not run |
    | `providers-registry` | 9 | 6 | 0 | 3 | 0 | 27 | not run |
    | `rpc` | 93 | 93 | 0 | 0 | 0 | 32 | not run |
    | `selectors` | 14 | 14 | 0 | 0 | 1 | 15 | not run |
    | `session` | 43 | 43 | 0 | 0 | 0 | 19 | not run |
    | `settings` | 16 | 16 | 0 | 0 | 0 | 21 | not run |
    | `slash-commands` | 29 | 28 | 1 | 0 | 0 | 21 | not run |
    | `startup` | 19 | 18 | 1 | 0 | 0 | 10 | not run |
    | `tools` | 32 | 32 | 0 | 0 | 0 | 35 | not run |
    | `tree` | 9 | 8 | 1 | 0 | 0 | 9 | not run |
    | `tui-components` | 39 | 39 | 0 | 0 | 0 | 36 | not run |
    
    Full per-file detail: `test/parity/coverage.md`.
    
    <!-- END COVERAGE -->
    
    ## Reliability contexts
    
    Define the upstream contract, complete user path, and material failure modes before implementation. Follow the verification sequence below. Use the complete PiG path as the primary proof for behavior that crosses a Provider, Session, mode, Extension Host, process, or terminal boundary. Retain repeatable evidence.
    
    Test empty, ordinary, boundary, and representative stress inputs. Verify ordering, cancellation, replacement, cleanup, and retained state. Measure latency, memory, allocation, and backpressure on critical paths. A small-input pass does not prove bounded behavior.
    
    Use a term from `docs/project/CONTEXT.md` when that file defines it. Do not redefine ratified terms in a module instruction. Read the nearest module `AGENTS.md` before work in a known reliability context. The root owns shared invariants. The module owns its contract, failure model, and evidence. Store each rule once.
    
    | Context | Instructions |
    |---|---|
    | Transcript, Provider, Event Stream | `ai/AGENTS.md` |
    | Main Screen, terminal rendering, input editing | `tui/AGENTS.md` |
    | Session, Model Runtime, execution mode | `coding/AGENTS.md` |
    | Extension Host, SDK, topology, lifecycle | `coding/extension/AGENTS.md` |
    | upstream oracle, scenario, comparator, evidence | `test/parity/AGENTS.md` |
    | Stock PiG artifact, PiG Standard Piglet release, provenance, publication | `.github/AGENTS.md` |
    
    Apply these rules to new work. Do not delete or weaken accepted tests to conform. Coordinate with the lane owner before changing evidence. Brief or restart active agents after an instruction change.
    
    ## Upstream test parity
    
    A production file is `ported` only when every upstream test file covering it is ported with the same inputs and expected results, or designed out with a numbered, reviewed reason. Record test locations and current upstream hashes in `test/parity/interfaces/test-mapping-v<version>.json`. A `partial` disposition enumerates the missing cases; file-name similarity, existing code, and a passing smoke test do not close them. Port the covering upstream tests in the same change whenever you port or change a production file. Preserve ordering, cancellation, errors, retained state, and caller effects, not only happy-path output.
    
    No release may treat a pending or partial upstream test tagged `hot-path` as closed. `make ci-contracts` enforces the reviewed path tags in `test/parity/interfaces/test-porting-policy-v<version>.json` and rejects a decrease in the total ported-file count against the committed baseline. The owner-approved 0.3.x exception records explicit `deferred-0.3.x` rows without changing their partial/pending mappings, hot-path tags, assertions or baseline; [the release-policy procedure](test/parity/README.md#approved-03x-test-porting-gaps) defines the required approval, missing-case reference and follow-up. All unlisted pending/partial hot paths remain release blockers. Keep tag review and the ported baseline current in the same change; never remove a hot-path tag or lower the baseline to pass the gate. A credential or toolchain requirement is not a scope exclusion. Dispatch test-file ownership through `docs/parity/pending-tests-batches.md`; coordinate production files and shared evidence separately.
    
    ## Verification-driven development
    
    For core parity surfaces (agent loop, message normalization, provider payload conversion, session persistence, auth, tool execution, interactive rendering), use a red → green → refactor loop. Apply [Faithful, general implementations](#faithful-general-implementations).
    
    Bug fixes require a regression guard in the same PR. Add a new unit test, integration test, or parity scenario that would fail on the observed bug, or name the existing failing test that already proves it. If no automated guard is practical, say why in the PR and file a follow-up before merge. Do not rely on manual repro alone for a fixed bug.
    
    1. **Reproduce first.** Add or identify a test/scenario that fails on the current code before changing behavior. If you already patched before writing the test, prove the test is meaningful by describing the exact pre-fix failure it would have caught.
    2. **Compare to upstream.** Read the upstream TypeScript implementation and encode the observable rule in the test name/comment. For provider/message conversion, prefer table-driven tests that assert the serialized request shape, not just successful return. For interactive rendering, assert the runtime event path, not only the leaf component.
    3. **Fix at the source.** Patch the lowest shared layer that matches upstream semantics. Avoid UI/provider band-aids when message normalization or session conversion is the source.
    4. **Harden both boundaries.** For cross-layer bugs, add at least one unit test at the pure conversion layer and one regression at the nearest caller boundary when practical (e.g. NormalizeMessages + OpenAI converter; session loop + print/interactive error surfacing).
    5. **Verify poisoned history.** When a bug involves bad persisted sessions, test with historical/poisoned messages: errored assistant turns, aborted assistant turns, empty text blocks, nil content, orphaned tool calls, and model/provider switches.
    6. **No accepted flake.** Do not document “rerun standalone” as success. Fix root causes instead of hiding failures with longer time budgets, retries, sleeps, env overrides, skips, or normalization. Use timeouts and retries only for faults outside our control, and surface those faults. An unavailable external dependency is a blocker, not passing evidence.
    7. **Report the red/green evidence.** Final summaries for fixes to core parity must state: failing symptom, new regression test names, upstream rule mirrored, and commands run. If a test was added after the fix, say it was not red-proven.
    8. **Production path required.** A new production API/method used to fix behavior must have at least one production call site or a test that drives the production path. Test-only call sites do not prove the product behavior changed.
    9. **Coverage quality over coverage quantity.** A PORT_MAP entry is not accepted as verified by a weak scenario. Boot-only, smoke-only, and deferred scenarios may stay in the report, but they do not count as behavioral verification. Registration-only scenarios count only for registry/catalog/auth-wiring paths, never for provider stream/payload conversion files.
    
    Use this acceptance matrix for shared provider, model, auth, and tool regressions. Explain inapplicable shapes from the upstream contract, not available credentials. Do not use `--list-models` as stream/payload proof.
    
    Cover both OpenAI-compatible API paths when the change reaches both:
    
    ```text
    ai/openai.go             # openai-completions
    ai/openai_responses.go   # openai-responses
    agent/transform.go       # upstream transform-messages parity before provider conversion
    ```
    
    Shared-path acceptance matrix for regressions:
    
    | required axis | minimum cases |
    |---|---|
    | provider shape | OAuth, API key, OpenAI-compatible with a custom base URL, and no default model, as the path allows |
    | API path | each API kind reached by the shared behavior, including `openai-completions` and `openai-responses` when both can apply |
    | mode | pure shared-function/converter unit test plus nearest caller (`print`, interactive, or RPC, as applicable) |
    | history | fresh plus poisoned persisted history when relevant |
    | assistant prior turn | error, aborted, empty text, tool-call/orphaned tool-call when relevant |
    | auth/network | hermetic success and hermetic failure when touching auth refresh/error display |
    
    ## Done criteria
    
    A surface is done only when:
    1. a canonical scenario exists under `test/parity/scenarios/<family>/`;
    2. `covers = [...]` names exact upstream paths from `docs/parity/PORT_MAP.md` **and** those paths are exercised by the asserted behavior;
    3. the scenario has behavioral verification quality (not boot-only, registration-only, smoke-only, or deferred) unless the surface is explicitly accepted as weak-only;
    4. the scenario was derived from upstream first;
    5. drift exposed by the scenario is fixed or recorded as a numbered divergence;
    6. legacy duplicate or weak coverage claims are removed;
    7. pure unit tests and caller-path tests exist for cross-layer bugs;
    8. `go test ./...` and the relevant parity gate pass.
    
    9. for an upstream leap, `make upstream-delta` proves every changed tracked source file has a non-pending disposition and durable evidence.
    
    10. `make interface-inventory-drift` proves the public package export graph and
        exact published declaration inventory are current, and
        `make interface-mapping-strict` proves every stable interface ID has complete
        mapping/reachability/conformance/evidence closure.
    
    11. `make async-contracts` proves every current upstream Promise/async source has an explicit ordering/cancellation/error contract and durable evidence.
    
    12. interaction and state-machine behavior found during probing is closed in
        `test/parity/behavior-contracts.toml` when declaration shape cannot express it.
        Contracts bind a hashed current-upstream source range and its complete
        keybinding/event inventory to Pig targets, a failing unit regression, and a
        behavioral parity scenario. Boundary transitions such as first/last
        wraparound, empty/singleton states, cancellation, ordering, and persistence
        are separate obligations; interface presence does not prove them.
    
    `test/parity/interfaces/behavior-inputs-v<version>.json` is the compiler-derived
    denominator for upstream input/action state machines and render surfaces. It
    accounts for keybinding declarations, platform defaults, consumers, raw input,
    branch/state effects, theme calls, glyph/string tokens, width/layout calls,
    numeric positioning literals, and referenced file constants. Its reviewed
    mapping assigns each surface to one owner family and records pending, partial,
    ported, deferred, divergence, or designed-out closure. `make
    behavior-contracts` rejects source/facet drift, declaration/consumer mismatch,
    missing mappings, invalid family ownership, and stale evidence. `make
    behavior-contracts-strict` rejects pending or partial surfaces. Generated
    facets identify review obligations; they never prove branch semantics, visual
    output, or promote a mapping.
    
    Parallel porter campaigns may run only read-only `inventory` and `verify` jobs
    against disposable snapshots via `make porter-campaign`. Reports
    must be tied to one commit, upstream version, and owner family, and are proposals
    for the coordinator to re-probe. Production/evidence edits (`family` and
    `tighten`) remain serialized in the canonical workspace.
    
    Choose the option that makes the next upstream sync smaller. Do not embed pig-specific behavior in mirrored code unless it is a documented divergence.
    
    ## Parity scenario rules
    
    Probe upstream first. Record enough evidence in comments for the next maintainer to see what was measured. Prefer strong, byte-faithful comparators. Use substring checks only when no stronger stable assertion exists.
    
    Scenario quality rules:
    - `covers` must name behav
    ... command output truncated for persistence ...
    ite a numbered divergence. Comments alone are not enough.
    - Do not weaken a comparator to keep a pig bug green.
    - Scenario comments must explain any `normalize_replace`, `lint-known-gap`, `skip`, `serial`, group override, or weak-quality tag.
    - Keep `covers` narrow. Broad claims inflate the dashboard and hide untested behavior.
    - Re-probe affected cross-family scenarios when a fix changes shared behavior.
    
    Isolation rules for parallel parity:
    - Agent/home dirs are ephemeral copies. Never write into checked-in fixture roots.
    - Use `{{TEMP}}` for writable paths; do not hardcode `/tmp` paths.
    - tmux session names must be unique and cleanup may kill only `parity-*` sessions.
    - `PIG_CODING_AGENT_DIR` and `PI_CODING_AGENT_DIR` win over `PIG_HOME`, and every child process inherits them, including a tmux session started from your shell. A manual probe or test that runs pig must set or clear all of `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME`, `PI_HOME` and `HOME`, or `/logout` and login rewrite the real credentials (the 2026-10-06 lane `auth.json` wipe). A package whose `TestMain` calls `testenv.ScopeTempDir` or `testenv.RunScoped` replaces the agent-directory variables. Other packages inherit them, so a plain `go test` run from an agent shell sets `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME` and `PI_HOME` to scratch directories first. `make test` runs each group under seeded directories and fails a group that adds, removes or changes any file or directory in them (`automation/ci/agent-dir-guard.sh`).
    - `pre_clear_paths` is deprecated; prefer fresh temp dirs.
    - Use the default scheduler groups unless a justified `group = "..."` or `serial` tag is required.
    - Deferred scenarios stay visible in coverage; do not use skip/defer to hide known drift.
    
    ## Divergences
    
    A prior divergence record is investigation evidence, not permanent approval.
    Re-probe it against the exact current upstream and current production path at
    every upstream leap and foundation/release review. Delete or reclassify entries
    that are language mechanics, additive behavior, test limitations,
    unreachable/equal behavior, stale scaffolds, or caller-free code. IDs are global
    across `docs/parity/DIVERGENCES.md` and additive ledgers; duplicate IDs fail even when they
    live in different files.
    
    A divergence is allowed only when it is user-visible or interop-relevant and cannot or should not be made faithful now. Every active divergence must have:
    - `D<N>` id in `docs/parity/DIVERGENCES.md`;
    - `SCRUTINIZED:approved`;
    - remove-when condition;
    - `// pig divergence (D<N>): ...` at each call site;
    - parity coverage or an explicit parity allowance.
    
    Each record is one file, `docs/parity/divergences/D<N>.md`: an active record is a `## D<N> <title>` section, a retired record is one `- D<N>: ...` line. Edit that file only. `make divergences` generates `docs/parity/DIVERGENCES.md` from the files, and a lane does not commit the generated ledger; the merger regenerates it (`TestDivergenceLedgerEqualsItsRecordFiles` fails when they differ).
    
    `make divergence-guard` (part of `make check`) catches divergences nobody recorded. It flags, in the agent, ai, coding, internal/codingagent, tui and cmd/pig packages: duration and cap literals with no upstream counterpart (`magic-literal`), event sends that race a context-done case (`cancel-drop-send`), non-blocking sends that drop their value (`default-drop-send`), decode errors skipped inside loops (`stream-parse-continue`), unknown or missing stop reasons mapped to success (`stop-reason-success`), SSE fields parsed outside `ai/sse.go` (`hand-rolled-sse`), `recover()` that discards the panic (`bare-recover`), discarded persistence errors (`discarded-io-error`), hooks that proceed when their handler fails (`hook-fail-open`), errors classified by status-code substrings (`error-status-substring`), and agent `Send`/`Continue` outside `coding/session*.go` (`agent-run-outside-session`). `bare-recover` scans every non-test Go file in the repository, not only those packages. A hit passes only with a `// pig divergence (D<N>): ...` marker on its line or the line above, or, when Pi really does the same, `// upstream: <file>:<symbol>` there; the file must exist in `.upstream/current` or be a SHA-256-verified dependency snapshot whose exact version matches a dependency pin in that mirror (`test/parity/dependency-sources.json`; see `docs/parity/dependency-source-proof.md`), the symbol must appear in it, and for a literal the value must appear too. A literal whose value appears in an upstream file that `docs/parity/PORT_MAP.md` maps to the Go file also passes. Every current hit is listed in `automation/ci/divguard/baseline.toml` with its finding and owning slice. The gate compares the scan with that file in both directions: a new hit fails, and so does an entry whose hit is gone, so a fix deletes its entries in the same change. It does not compare the file with its previous version, so keeping it shrink-only is a review rule: never add an entry for new code. A green guard shows these patterns are absent or accounted for, not that the code matches Pi; allow markers prove only that the cited file and symbol exist (and, for literals, that the value appears there).
    
    Do not record ordinary TS-to-Go mechanics, test harness differences, or an
    SDK's implementation of a capability every SDK has (naming convention, `Result`
    vs `(T, error)` vs raised exception, ownership and borrowing, host call versus
    replicated state).
    
    That exemption covers implementation only. An SDK that lacks a capability
    another SDK has, or that answers the same call differently, is drift regardless
    of how faithfully its interface is spelled. Record it and close it.
    
    ## Native Windows development
    
    Windows is a first-class target. Windows-only failures are fixed on a Windows machine, not by repeated CI round trips.
    
    Set up the toolchain at the pinned versions:
    - Go from `.go-version`, Node from `.node-version`, Python 3.12, and the Rust toolchain that `ci.yml` names in `RUST_VERSION`.
    - Git for Windows, and run the `make` targets and `automation/` scripts from Git Bash.
    - `fd` and `rg` from `automation/ci/install-windows-tools.ps1`, which verifies each archive against `automation/ci/windows-tools.txt`.
    - The Pi oracle with `automation/ci/install-npm-toolchain.sh`, then `python automation/pin/pin.py install-oracle`, then `python automation/ci/npm-locked.py internal/typebox/testdata/npm` and `python automation/ci/npm-locked.py ai/testdata/sdk`, as the Windows CI job does.
    - `./automation/gen/mirror-upstream.sh` from Git Bash with `MSYS=winsymlinks:nativestrict`.
    
    Reproduce a Windows CI job locally before you change code. Run the same command the job runs (see the `windows` job in `.github/workflows/ci.yml`). Use `go test -v` and a `-timeout` below the job limit, so a stall prints a stack.
    
    Set `PIG_HOME` and `PIG_CODING_AGENT_DIR` to a directory with a space in its path, as CI does. Many Windows bugs are path bugs: separators, drive letters, 8.3 short names, and `D:\work\project` against `/work/project`. Compare with what Pi does on Windows before you change a path rule.
    
    Work on a branch named `team/<name>/<topic>`. Push it to the staging remote, not to the public repository. The integrator stacks verified branches into the release branch. Each commit follows Commit hygiene below: `git commit --signoff -S`.
    
    ## Code and lint policy
    
    Apply [Faithful, general implementations](#faithful-general-implementations) to code review and provider/model literal guard exceptions; each exception requires a Pi source citation.
    
    Fix valid findings at the source. Preserve upstream behavior over stylistic lint suggestions. Use the narrowest suppression for real false positives and explain why a source fix would be less faithful or less correct. `nolintlint` is enabled; stale or unexplained pragmas fail. Security suppressions must name the CLI threat-model reason.
    
    Code and interface style:
    - Production comments state only current behavior and non-obvious protocol, concurrency, security, compatibility, or behavioral invariants. Delete comments that restate code, narrate an implementation pass, preserve project history, or point to `docs/parity/PORT_MAP.md`, a phase, a future row, or another status ledger instead of stating the current contract. Keep migration and parity rationale in the owning ledger or specification.
    - Do not hard-wrap Markdown prose or source comments to fit a terminal pane. Editors can display-wrap long lines. Preserve structural line breaks in code blocks, tables, quoted material, poetry, generated files, and formats with an enforced line-length contract.
    - Roadmap phases, workstream labels, future-work promises, internal issue ownership, and placeholder implementation plans fail the source-hygiene gate; keep them in tracked specifications instead.
    - Historical comments, phase labels, placeholders, compatibility paths, manual
      parity lists, test-only public implementations, and prior approvals have no
      grandfather status. Revalidate against current upstream and production use;
      update the current rationale, bind the full production path, reclassify, or
      delete.
    - Production-grade Go means concrete validated data, minimal consumer-owned
      interfaces, deterministic order, bounded resources, explicit lifetime/error
      ownership, and one mechanism per concept. Do not mirror TypeScript internals
      or add abstraction merely to look structurally similar; preserve observable
      Pi semantics and portable wire/session/source behavior. Quality archaeology is
      not a style sweep: change debt only with current contract/caller evidence and
      leave accurate provenance and healthy out-of-family code alone.
    - `iface` rejects unused/duplicate interface declarations, opaque interface
      returns with one concrete implementation, and unexported contracts leaked
      through exported APIs. Do not suppress it
      by default; delete an unneeded interface or keep the one consumer-owned
      contract.
    - Do not enable or apply `ireturn` or `interfacebloat` mechanically. Extension
      protocol interfaces, tagged unions, providers, and factories have legitimate
      interface returns, and an upstream public protocol may be broad. Review those
      surfaces against `docs/typescript-to-go-porting.md` and measure hot paths.
    - Performance claims require a representative benchmark with allocations and a
      CPU/allocation profile. Interface dispatch is not a defect without evidence;
      prioritize the measured cost center and preserve upstream behavior.
    
    - Exact-count assertions must derive their expectation from an independent denominator or identify an explicitly reviewed generated snapshot and use its owning regeneration command. A newly observed number proves that output changed, not that the new number is correct.
    - A runtime mutation must compile before its behavioral failure counts as evidence. A compiler failure counts only when compilation is the contract under test.
    - For extension, TUI, model/provider, Session/RPC, Package/Resource, and startup/reload critical paths, capture a representative benchmark/profile and resource-lifetime disposition with the owning behavior family. No blocking extension IPC or unbounded work may run on the TUI input/render loop.
    - Ordinary editor and transient popup changes must not clear or replay transcript history. No TUI input handler may synchronously copy, decode, or scan unbounded Session history.
    - Every known bug found in scope is fixed with red/mutation evidence or remains
      an explicit user-owned blocker. Passing unrelated gates does not close it.
    
    Go style:
    - Build, CI, release-candidate, security-analysis, and documented setup commands use the Go version in `.go-version`, the single source. Do not write the version number in this file.
    - `.go-version` is the one Go pin. Workflows, scripts, images, the devcontainer, and the `toolchain` lines of `go.mod` and `go.work` name that release and no other. A script reads `.go-version` instead of repeating the number. Moving the pin changes `.go-version` and its recorded copies in one change; `make compliance` and `TestGoVersionPolicy` fail on any other release. Dated measurement records keep the release they ran on.
    - Maintained modules declare Go 1.26 or 1.26.0 as their language floor unless a concrete language requirement is approved. Do not raise a `go` directive merely to match the build toolchain.
    - Use language features available at the module floor. Prefer applicable modern standard-library APIs such as `slices.Sort`, `slices.SortFunc`, `cmp.Compare`, `maps.Copy`, `slices.Sorted(maps.Keys(m))`, `strings.Cut`, `CutPrefix`, `SplitSeq`, `for i := range n`, `wg.Go`, `min`, and `max`.
    - `go fix -diff ./...` should be empty.
    - When observable behavior changes, update the doc comment in the same commit.
    
    Lint commands when touching lint/config/code:
    ```bash
    go tool golangci-lint config verify
    make lint-changed
    make lint
    ```
    
    `make port-lint` runs the porting anti-pattern analyzers (`automation/ci/portlint/README.md`) against a ratcheting `baseline.toml`. Fix a finding at the source, or mark one reviewed false positive with `//portlint:allow <check> <reason>`. Never add a baseline entry for new code.
    
    Use `make lint-changed` in the development loop. It runs every configured linter over whole changed Go files in changed packages relative to the merge base with `main`. Use `make lint` for the full-repository gate. `make check` always runs the full-repository gate.
    
    ## Commands
    
    Run `make generate` after changing exported Go API, CLI flags, settings, parity scenarios, or docs mirrors. Commit the result. Never hand-edit generated files; regenerate.
    
    Primary gates:
    ```bash
    make generate
    make build
    make vet
    make lint
    make test
    make lint-scenarios
    make parity-fast
    make parity
    make check
    make verify
    make coverage
    ```
    
    Other gates as needed:
    ```bash
    make port-lint
    make schedule-report
    make test-stress
    make parity-stress
    make parity-perf
    make test-timing   # blocking wall-clock/ratio gates (internal/testenv.TimingGate); PR CI only warns (PIG_PERF_TIMING_REPORT_ONLY=1)
    make parity-live
    make release-check
    go tool govulncheck ./...
    go tool deadcode ./...
    go fix -diff ./...
    ```
    
    Choose the test tier by the moment, as `docs/project/test-economy.md` defines: `go test ./<pkg> -run '<Tests>'` while iterating, `make test-changed`, `make lint-changed` and `go vet` on touched packages before a handoff commit, `make ci-drift ci-contracts` and the affected `make parity-family` runs before reporting ready, and the full suites (`make test`, `make check`, `make verify`, `make parity`, `go test ./...`) in CI, integration and release preview. Do not pass `-count=1` unless the test reads inputs through a child process. Use `-race` on edited packages, not the whole module. Bound `go test -fuzz` with `-parallel` and `-fuzztime`. On a shared host, build with `-trimpath`.
    
    `make test` requires and exercises the Go, Node, Python, and Rust toolchains; fixture build failures are fatal. `make check` combines deterministic gates with one strict Pig/Pi pair per hermetic scenario. `make verify` is the end-of-loop gate: it runs each scenario's declared durability and regenerates coverage. `make parity` runs declared scenario pairs in parallel with default concurrency groups and suppresses `runtime_ratio_max` because parallel CPU contention makes that metric unreliable. `make parity-perf` runs serially and enforces runtime ratios; use it for release gates or cron, not normal loops.
    
    ## Documentation language
    
    Write instructions, references, procedures, and operational documentation as simple, direct technical English. Write one instruction per sentence, in the present tense and the active voice. Keep one term for each thing. Do not apply controlled technical English mechanically to personal essays, origin stories, interviews, or release narratives. Preserve the author's vocabulary, humor, uncertainty, and cadence in those forms.
    
    Do not hard-wrap Markdown prose to a terminal width. Use structural newlines only where Markdown or the content requires them.
    
    Where Pig differs from Pi in a way a user can observe, state the difference at
    the point of use and cite its `D<N>`. An agent carrying Pi pretraining does the
    Pi thing wherever Pig's difference is undocumented.
    
    ## Public project writing
    
    Write about PiG as a faithful Go implementation of Pi. Name Pi as the reference
    implementation and link to its repository and documentation. Credit Michael
    Kinsy as PiG's creator and state that PiG was originally developed at Hewlett
    Packard Enterprise. Keep those facts separate from legal notices.
    
    Do not call PiG an official Pi project or imply endorsement by Pi's maintainers.
    Do not claim HPE sponsorship, Open Program Office approval, public release,
    community governance, certification, or trademark approval until the relevant
    decision is recorded and public. Do not use HPE logos, wordmarks, or exact
    corporate styling without recorded Brand approval. Keep Stock PiG branding
    product-neutral.
    
    Use direct technical prose. Avoid slogans, superlatives, invented motives,
    competitive framing, and promises of future collaboration. Prefer verifiable
    facts: the pinned Pi version, observed parity, named divergences, supported
    artifacts, and published evidence.
    
    ## Commit hygiene
    
    User-facing changes go in `changelog.d/<lane>-<sha>.md`, one fragment per commit range, as `### Added`, `### Changed`, `### Fixed` (and `Upgrading`, `Deprecated`, `Removed`, `Security`, `Known issues`) sections of bullets in `CHANGELOG.md`'s style. `<sha>` is a commit of the range reachable from `HEAD`; a rebase, squash or amend replaces SHAs, so rename the fragment after the commit that replaced it. Every `### Fixed` bullet ends with `<!-- fix: <sha> -->` naming the commit (or commits, comma-separated) that fixes it, also reachable from `HEAD` (or named by a `Fix-Commit: <full sha>` trailer of a squash commit `HEAD` reaches, see `.github/CONTRIBUTING.md`): the bug ledger reads `### Fixed`, and a bullet without a marker has its commits guessed from history, which once credited the 0.5.0 squash and its ~11,000 tests to five bullets. `### Upgrading` is what a user of the previous release must change; the fold appends it to the release's "Upgrading from" section. Do not edit `CHANGELOG.md`. `make changelog-check` validates the names, sections, markers and reachability against the full history of `HEAD` (a shallow clone fails) and `make changelog-unreleased` prints the assembly; fragments whose commits a squash replaced before these rules are listed in `automation/ci/changelogfrag/legacy.txt`, are exempt from the name and marker rules, and the list only shrinks. The integrator runs `make changelog-fold VERSION=x.y.z` to append the fragments to that release in `CHANGELOG.md` and delete them; it runs again at release for fragments written since. `make bug-ledger` refuses to attribute the tests of a commit that touches more than 500 files (a squash or merge) and names the entry; name the real fix commit in its marker.
    
    Every commit requires both a DCO `Signed-off-by` line and a signature GitHub marks **Verified**. Use `git commit --signoff -S`. Sign-off is not a cryptographic signature. See `.github/CONTRIBUTING.md` for SSH signing setup and unsigned-commit repair. If signing is unavailable, a maintainer lands the change in a maintainer-signed commit with a `Co-authored-by:` trailer.
    
    Before committing, run `git diff --stat HEAD` and `git status --short`, then stage only files intentionally changed. Never `git add .`, `git add -A`, or add a whole directory without inspecting contents. Testdata dirs accumulate temp files.
    
    Forbidden commands:
    - `git reset --hard`
    - `git checkout .`
    - `git clean -fd`
    - `git stash`
    - `git commit --no-verify`
    - `git push -f`
    - `tmux kill-server`
    - `go install ./cmd/pig`
    
    Only `tmux kill-session -t <name>` is allowed. Commit only files changed in this session.
    
    Do not dispatch subagents to read local files available in this worktree. Read them yourself.
    
    ## When unsure
    
    Read upstream source, read upstream tests, probe upstream behavior, then make pig match it. Ask the user instead of guessing.
    # TUI reliability
    
    ## Read first
    
    - `../docs/project/CONTEXT.md`
    - `../.upstream/current/packages/tui/src/tui-main-screen.ts`
    - `../.upstream/current/packages/tui/src/components/editor.ts`
    - `main_screen_upstream_parity_test.go` after confirming its currently documented Pi 0.84 model against the pinned source
    
    ## Contract
    
    The Main Screen preserves scrollback. The Editor changes only the required visible rows. The Viewport follows Pi's state machine. Frame work does not scale with unchanged Session history.
    
    ## Failure modes
    
    | Failure | Detection | Required result |
    |---|---|---|
    | Ordinary editor shrink repaints the viewport | Delete one newline or wrap row | Use a local differential update |
    | The viewport snaps backward | Compare viewport origin before and after shrink | Match Pi |
    | A frame replays transcript history | Compare short and large Sessions | Keep frame work bounded |
    | A stale or duplicate row remains | Apply bytes to the terminal simulator | Match Pi's final screen |
    | Scrollback is cleared | Detect `CSI 2J` and `CSI 3J` | Emit neither for an ordinary edit |
    | The cursor homes | Detect `CSI H` | Use Pi-compatible relative movement |
    | Wrapped input enters history early | Move Up within wrapped input | Keep Pi's cursor and history state |
    | Resize corrupts layout | Change width and height with ANSI, wide text, and images | Match Pi's final screen |
    | A hidden-row change cannot be repaired locally | Change content above the viewport | Use Pi's recovery render |
    
    ## Evidence
    
    Define the terminal scenario before renderer code. Compare escaped Pi and PiG output and terminal-simulator state. Retain inputs, dimensions, bytes, screen state, and mutation failure. Use isolated tests for width, row, cursor, and viewport calculations. Profile editor shrink, streaming, overlays, and large transcripts. Run `go test -race ./tui` and the owning parity scenarios.
         1    package tui
         2
         3    import (
         4        "fmt"
         5        "strings"
         6
         7        "github.com/MichaelKinsy/PiG/tui/widthx"
         8    )
         9
        10    // SetWorkingStatusIndicator selects the status displayed by an opted-in editor.
        11    func (e *Editor) SetWorkingStatusIndicator(indicator *StatusIndicator) {
        12        e.workingStatusIndicator = indicator
        13        e.Invalidate()
        14    }
        15
        16    // IsDirty reports whether the editor's rendered lines changed since the last
        17    // consume. The embedded status indicator is animated and relabelled by its
        18    // owner (spinner ticks, working message, retry countdown) without touching the
        19    // editor, so its own dirty flag counts too; otherwise the per-child render
        20    // cache would keep the old border row until a keystroke dirtied the editor.
        21    func (e *Editor) IsDirty() bool {
        22        if e.invalidatable.IsDirty() {
        23            return true
        24        }
        25        indicator := e.workingStatusIndicator
        26        return e.EmbedWorkingStatus && indicator != nil && indicator.IsDirty()
        27    }
        28
        29    // NeedsRedraw consumes the editor's and the embedded indicator's dirty flags.
        30    func (e *Editor) NeedsRedraw() bool {
        31        dirty := e.invalidatable.NeedsRedraw()
        32        if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
        33            dirty = indicator.NeedsRedraw() || dirty
        34        }
        35        return dirty
        36    }
        37
        38    // pig additive (D91): SurfaceLive reports that the embedded indicator, which changes without
        39    // invalidating the editor, draws in the editor.
        40    func (e *Editor) SurfaceLive() bool {
        41        return e.EmbedWorkingStatus && e.workingStatusIndicator != nil
        42    }
        43
        44    // renderStatusBorder mirrors CustomEditor.renderTopBorder, retaining a centered
        45    // overflow label when the complete status or spinner can fit to its left.
        46    func (e *Editor) renderStatusBorder(width, hiddenLineCount int, fallback string) string {
        47        if !e.EmbedWorkingStatus || e.workingStatusIndicator == nil || width <= 0 {
        48            return fallback
        49        }
        50        indicator := e.workingStatusIndicator
        51        // Embedded working and compaction indicators take the editor's border color (interactive-mode.ts showWorkingStatusIndicator
        52        // and the compaction_start colorFn, Pi 1.2.0); branch summary and retry keep their own colors.
        53        if indicator.Kind == "working" || indicator.Kind == "compaction" {
        54            loader := &Loader{Message: indicator.Message, Frame: indicator.Frame, Frames: indicator.Frames, IndicatorVerbatim: indicator.IndicatorVerbatim, spinnerColorFn: e.colorEditorBorder, messageColorFn: e.colorEditorBorder}
        55            indicator = &StatusIndicator{Loader: loader, Kind: indicator.Kind}
        56        }
        57        status := indicator.RenderInBorder(max(1, width-5))
        58        statusWidth := widthx.VisibleWidth(status)
        59        if statusWidth == 0 {
        60            return fallback
        61        }
        62        overflowLabel := ""
        63        if hiddenLineCount > 0 {
        64            overflowLabel = fmt.Sprintf(" ↑ %d more ", hiddenLineCount)
        65        }
        66        overflowWidth := widthx.VisibleWidth(overflowLabel)
        67        overflowStart := (width - overflowWidth) / 2
        68        canFitOverflow := func() bool {
        69            return overflowLabel != "" && overflowWidth+2 <= width && overflowStart-(3+statusWidth+1) >= 1
        70        }
        71        if overflowLabel != "" && !canFitOverflow() {
        72            status = indicator.RenderSpinnerInBorder(width)
        73            statusWidth = widthx.VisibleWidth(status)
        74        }
        75        color := e.colorEditorBorder
        76        if canFitOverflow() {
        77            leftWidth := 3 + statusWidth + 1
        78            return color("── ") + status + color(" "+strings.Repeat("─", overflowStart-leftWidth)+overflowLabel+strings.Repeat("─", width-overflowStart-overflowWidth))
        79        }
        80        if width >= statusWidth+5 {
        81            return color("── ") + status + color(" "+strings.Repeat("─", width-statusWidth-4))
        82        }
        83        status = indicator.RenderSpinnerInBorder(width)
        84        statusWidth = widthx.VisibleWidth(status)
        85        prefixWidth := min(3, max(0, width-statusWidth))
        86        return color(strings.Repeat("─", prefixWidth)) + status + color(strings.Repeat("─", max(0, width-prefixWidth-statusWidth)))
        87    }
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:12:    e.workingStatusIndicator = indicator
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:25:    indicator := e.workingStatusIndicator
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:32:    if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:41:    return e.EmbedWorkingStatus && e.workingStatusIndicator != nil
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:44:// renderStatusBorder mirrors CustomEditor.renderTopBorder, retaining a centered
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:46:func (e *Editor) renderStatusBorder(width, hiddenLineCount int, fallback string) string {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:47:    if !e.EmbedWorkingStatus || e.workingStatusIndicator == nil || width <= 0 {
    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:50:    indicator := e.workingStatusIndicator
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:34:    workingStatusIndicator *StatusIndicator
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:148:    // rowCache holds the padded content rows of the last frame by visible row, and topBorder and bottomBorder its borders.
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:150:    topBorder    editorRenderedBorder
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:272:// WithEditorTheme renders the editor with theme instead of the active theme (the editor.ts constructor's theme argument): the border uses theme.BorderColor and the autocomplete popup uses theme.SelectList.
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:910:// renderBorder colors the plain or scroll border. The default-colored border is reused from the last frame while its inputs are unchanged.
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:911:func (e *Editor) renderBorder(cache *editorRenderedBorder, arrow string, width, hidden int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:960:func (e *Editor) renderTopBorder(width, hidden int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:961:    return e.renderBorder(&e.topBorder, "↑", width, hidden)
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:964:func (e *Editor) renderBottomBorder(width, hidden int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:965:    return e.renderBorder(&e.bottomBorder, "↓", width, hidden)
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:1023:    top := e.renderTopBorder(width, e.scrollOffset)
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:1024:    top = e.renderStatusBorder(width, e.scrollOffset, top)
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:1031:    out = append(out, e.renderBottomBorder(width, linesBelow))
        65    // extensionDialogLines measures a dialog component at the current width so the
        66    // chat cap reflects the dialog actually on screen rather than a fixed guess.
        67    func (m *InteractiveMode) extensionDialogLines(component tui.Component) int {
        68        width := m.tuiInst.Width()
        69        if width <= 0 || component == nil {
        70            return 0
        71        }
        72        return len(component.Render(width))
        73    }
        74
        75    func (m *InteractiveMode) setStatusContainerLabel(label string) {
        76        if m.activeStatusIndicator != nil && m.activeStatusIndicator.Kind == "retry" {
        77            m.activeStatusIndicator.SetMessage(label)
        78            return
        79        }
        80        m.showStatusIndicator(&tui.StatusIndicator{Kind: "retry", Loader: tui.NewLoader(m.tuiInst, tui.ThemeFg("warning"), tui.ThemeFg("muted"), label, nil)})
        81    }
        82
        83    func (m *InteractiveMode) startWorkingLoader() {
        84        if !m.workingVisible {
        85            return
        86        }
        87        message := "Working"
        88        if m.workingMessage != nil {
        89            message = *m.workingMessage
        90        }
        91        indicator := &tui.StatusIndicator{Kind: "working", Loader: tui.NewLoader(m.tuiInst, tui.ThemeFg("accent"), tui.ThemeFg("muted"), message, nil)}
        92        m.applyWorkingIndicatorOptions(indicator)
        93        m.showStatusIndicator(indicator)
        94        m.tuiInst.RequestRender()
        95    }
        96
        97    func (m *InteractiveMode) stopWorkingLoader() { m.clearStatusIndicator("working") }
        98
        99    // tickSpinner drives status-indicator frames. Like Pi's Loader, whose
       100    // setInterval exists only while a loader does, the ticker parks once a tick
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:50:// options.EmbedWorkingStatus is the editor's readonly embedWorkingStatus (custom-editor.ts:25 `options?.embedWorkingStatus ?? false`).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:58:    editor.EmbedWorkingStatus = options.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:157:    // EmbedWorkingStatus renders working, compaction, summarization and retry status in the editor's top border; omitted is false.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:158:    EmbedWorkingStatus bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/custom_editor.go:177:    editor.EmbedWorkingStatus = options.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1677:    m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:18:        embeds := m.remoteEditor.editor.EmbedWorkingStatus()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:21:            m.editor.SetWorkingStatusIndicator(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:23:            m.editor.SetWorkingStatusIndicator(nil)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:27:    if !m.editor.EmbedWorkingStatus {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:30:    m.editor.SetWorkingStatusIndicator(indicator)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:31:    return m.editor.EmbedWorkingStatus
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go:839:    options := CustomEditorOptions{PaddingX: &paddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true}
    ✓ • 0ms

## Assistant

Found it—the prompt box lives in the vendored PiG code.

- **“Working” on the top edge:** [`tui/editor_status.go:46`](vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go:46) — `renderStatusBorder()` draws the spinner and label into the border, like `── ⠋ Working ─────`.
- **Prompt box rendering:** [`tui/editor.go:1023`](vendor/github.com/MichaelKinsy/PiG/tui/editor.go:1023) — builds the top border and calls `renderStatusBorder()`.
- **“Working” label and loader:** [`interactive_layout.go:83`](vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go:83) — `startWorkingLoader()` sets the default text to `"Working"`.

Start with `editor_status.go` if you want to change how that top edge looks.

## User

ok good, i want on that same row, on the rightside, is there anything rendered yet ? if not, i want it can dynamically detect what model am i choosing and the provider of that model to render the live update quota bar, for example, if im using luna 6 of codex subscription, it will render the quota bar of it, use the total instead of the specific quota for each accounts

## Assistant

I’ll check what the right side of that border currently renders, then trace the selected model, provider, and quota data to see how to add a live aggregate usage bar.

## Activity

    $ git status --short; cat vendor/github.com/MichaelKinsy/PiG/tui/AGENTS.md; rg -n -i 'quota|usage.*bar|rate.?limit|subscription' vendor/github.com/MichaelKinsy/PiG/internal/codingagent --glob '*.go' | head -100; rg --files -g '!vendor/**' -g '!go.sum' -g '!go.mod' --hidden -g '!.git'
    # TUI reliability
    
    ## Read first
    
    - `../docs/project/CONTEXT.md`
    - `../.upstream/current/packages/tui/src/tui-main-screen.ts`
    - `../.upstream/current/packages/tui/src/components/editor.ts`
    - `main_screen_upstream_parity_test.go` after confirming its currently documented Pi 0.84 model against the pinned source
    
    ## Contract
    
    The Main Screen preserves scrollback. The Editor changes only the required visible rows. The Viewport follows Pi's state machine. Frame work does not scale with unchanged Session history.
    
    ## Failure modes
    
    | Failure | Detection | Required result |
    |---|---|---|
    | Ordinary editor shrink repaints the viewport | Delete one newline or wrap row | Use a local differential update |
    | The viewport snaps backward | Compare viewport origin before and after shrink | Match Pi |
    | A frame replays transcript history | Compare short and large Sessions | Keep frame work bounded |
    | A stale or duplicate row remains | Apply bytes to the terminal simulator | Match Pi's final screen |
    | Scrollback is cleared | Detect `CSI 2J` and `CSI 3J` | Emit neither for an ordinary edit |
    | The cursor homes | Detect `CSI H` | Use Pi-compatible relative movement |
    | Wrapped input enters history early | Move Up within wrapped input | Keep Pi's cursor and history state |
    | Resize corrupts layout | Change width and height with ANSI, wide text, and images | Match Pi's final screen |
    | A hidden-row change cannot be repaired locally | Change content above the viewport | Use Pi's recovery render |
    
    ## Evidence
    
    Define the terminal scenario before renderer code. Compare escaped Pi and PiG output and terminal-simulator state. Retain inputs, dimensions, bytes, screen state, and mutation failure. Use isolated tests for width, row, cursor, and viewport calculations. Profile editor shrink, streaming, overlays, and large transcripts. Run `go test -race ./tui` and the owning parity scenarios.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_models.go:310:    m.maybeWarnAboutAnthropicSubscriptionAuthAsync()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model_availability.go:96:// ProviderIsSubscription reads the currently composed provider's OAuth capability without reading credentials.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model_availability.go:97:func (r *ModelRegistry) ProviderIsSubscription(providerID string) bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model_availability.go:99:        return provider.Auth.OAuth != nil && provider.Auth.OAuth.IsSubscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model_availability.go:102:    return auth.OAuth != nil && auth.OAuth.IsSubscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:424:    // usageService fetches provider quotas for the board. nil leaves the board
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:425:    // without a quota source (no credentials wired), which it reports as empty.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:426:    usageService UsageQuotaService
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:523:    // anthropicSubWarningShown gates the one-time Anthropic subscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:524:    // auth warning. Mirrors upstream anthropicSubscriptionWarningShown.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1127:    // UsageService is the quota service the /usage board reads. The host builds
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1129:    // board without a quota source.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1130:    UsageService UsageQuotaService
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1133:    // which asks the quota service for the store's active account; a present
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1709:    // Provider count + subscription indicator for footer.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1713:    // Warn if Anthropic subscription auth is active. Auth resolution can run
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1716:    m.maybeWarnAboutAnthropicSubscriptionAuthAsync()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go:78:        t.Fatal("a fast quota report was hidden by a pending account")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go:128:        t.Fatal("cached quota waited for the slow account")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go:184:func TestUsageCachedOAuthQuotaDoesNotWaitForTokenRefresh(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go:200:    now = now.Add(2 * time.Minute) // Token is near expiry; the quota is still fresh.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go:203:        t.Fatalf("fresh quota waited for OAuth: reports=%d refreshes=%d err=%v", len(snapshot.Reports), refreshes.Load(), err)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service_test.go:300:        t.Fatalf("a rejected credential must not serve stale quota, got %d reports", len(second.Reports))
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:4:// fullscreen overlay mount/close lifecycle, the quota service, and the local
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:18:// UsageQuotaService is the quota source the /usage board reads.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:21:type UsageQuotaService interface {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:22:    // FetchSnapshot refreshes every configured account's quota. force bypasses
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:39:// showUsageDashboard opens the fullscreen quota board, or focuses it when it is
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:206:// decision, which asks the quota service for the store's active account; a
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go:218:func (m *InteractiveMode) resolveUsageActiveAccount(ctx context.Context, service UsageQuotaService, provider string) (ai.UsageActiveAccount, bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:3:// Ports packages/coding-agent/src/core/provider-usage.ts: provider quota
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:65:// UsageService is the per-runtime quota service behind a usage dashboard.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:128:// NewUsageService builds a quota service. A nil store probes ambient auth only.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:463:    // Quota freshness does not depend on token freshness. A cached report
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:553:// still render the last known quota.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go:679:            // A rejected credential must not keep rendering stale quota as
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:30:// (providers/<id>.ts) rather than the OAuth flow's own name ("Meta (Muse subscription)"). A provider outside the
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:52:// loginProviderSubscription reports whether a provider's OAuth sign-in is backed by a subscription: upstream's provider.auth.oauth?.isSubscription === true (interactive-mode.ts:5722, 5756).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:53:func (m *InteractiveMode) loginProviderSubscription(id string) *bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:58:    subscription := auth.OAuth != nil && auth.OAuth.IsSubscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:59:    return &subscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:810:    m.statusLine.SetUsingSubscription(m.footerUsingSubscription(m.opts.Model))
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:813:// footerUsingSubscription mirrors footer.ts: Kimi Coding is
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:814:// subscription-backed despite API-key authentication; any other provider
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:815:// needs a stored OAuth login whose provider is a subscription login.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:816:func (m *InteractiveMode) footerUsingSubscription(model *ai.Model) bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:827:    if !ai.IsOAuthSubscriptionProvider(providerID) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:851:// to the active model's subscription marker.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:854:    footer.SetSubscriptionResolver(m.footerUsingSubscription)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go:279:        UsingSubscription:  d.usingSubscription,
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:59:    // subscriptionFor decides the "(sub)" marker for a newly bound model.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:60:    subscriptionFor func(*ai.Model) bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:78:    // usingSubscription: OAuth subscription pricing (e.g. GitHub Copilot).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:80:    usingSubscription bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:142:// SetModel rebinds the model. Upstream footer.ts derives the subscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:146:    subscriptionFor := s.subscriptionFor
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:148:    usingSubscription := false
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:149:    if subscriptionFor != nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:150:        usingSubscription = subscriptionFor(m)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:154:    if subscriptionFor != nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:155:        s.usingSubscription = usingSubscription
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:161:// SetSubscriptionResolver installs the "(sub)" decision used by SetModel.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:162:func (s *FooterComponent) SetSubscriptionResolver(resolve func(*ai.Model) bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:164:    s.subscriptionFor = resolve
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:231:// SetUsingSubscription updates the OAuth subscription indicator.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:233:func (s *FooterComponent) SetUsingSubscription(v bool) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:235:    s.usingSubscription = v
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:356:        usingSubscription:      s.usingSubscription,
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:382:    // usingSubscription is true when the model's provider uses OAuth
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:383:    // subscription pricing (e.g. GitHub Copilot). Shows "(sub)" in cost.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:385:    usingSubscription bool
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:401:// Kimi Coding and OAuth subscription logins show the cost even at zero, with
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:403:func footerUsageParts(u footerUsageTotals, usingSubscription bool) []string {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:420:    if (u.cost != 0 && !math.IsNaN(u.cost)) || usingSubscription {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:422:        if usingSubscription {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/status_line.go:479:    leftParts := footerUsageParts(d.usage, d.usingSubscription)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/keybindings.go:74:    {id: "app.usage.refresh", description: "Refresh provider usage quotas", keys: []KeyID{"r"}},
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/settings_selector.go:161:        Description:  "Warn when Anthropic subscription auth may use paid extra usage",
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/provider_registration_namespace.go:69:        input.OAuth = &ExtensionOAuthConfig{Name: callbacks.Name, IsSubscription: callbacks.IsSubscription}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:28:    rateLimitPattern    = lazyregexp.New(`(?:^|;)t=(\d+)`)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:65:func parseRateLimitDelay(value string) float64 {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:66:    match := rateLimitPattern.FindStringSubmatch(value)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:155:            delay = parseRateLimitDelay(response.header.Get("Ratelimit"))
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:158:            return nil, errors.New("Hugging Face rate limit reached; retry in " + jsnumber.String(delay) + "s")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/llama/huggingface.go:160:        return nil, errors.New("Hugging Face rate limit reached")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_extensions.go:1701:    // The typed models are recomposed on the same committed registry changes that republish the catalog. The subscription is an observer: the registry's single listener slot belongs to ModelRuntime.SetChangeListener callers.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:82:    // UsageService is the replacement Session's quota service, built from its own
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:84:    UsageService    UsageQuotaService
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/footer_data_provider.go:115:// notifyBranchChange runs the OnBranchChange subscribers in subscription order, like upstream's branchChangeCallbacks.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:3:// Ports packages/coding-agent/src/modes/interactive/interactive-mode.ts (maybeWarnAboutAnthropicSubscriptionAuth).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:12:const anthropicSubscriptionAuthWarning = "Anthropic subscription auth is active. " +
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:24:// isAnthropicSubscriptionAuthKey mirrors upstream (interactive-mode.ts:158).
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:25:func isAnthropicSubscriptionAuthKey(apiKey string) bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:29:// maybeWarnAboutAnthropicSubscriptionAuth mirrors upstream
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:33:func (m *InteractiveMode) maybeWarnAboutAnthropicSubscriptionAuth(ctx context.Context) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:34:    if !m.canWarnAboutAnthropicSubscriptionAuth() || !m.hasAnthropicSubscriptionAuth(ctx) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:38:    m.showWarning(anthropicSubscriptionAuthWarning)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:41:func (m *InteractiveMode) canWarnAboutAnthropicSubscriptionAuth() bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:49:func (m *InteractiveMode) hasAnthropicSubscriptionAuth(ctx context.Context) bool {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/auth_warnings.go:58:    return err == nil && resolved != nil && isAnthropicSubscriptionAuthKey(resolved.Auth.APIKey)
    README.md
    NOTICE
    .agents/skills/simplify/agents/openai.yaml
    .agents/skills/simplify/SKILL.md
    .agents/skills/writing-for-agents/agents/openai.yaml
    .agents/skills/writing-for-agents/SKILL.md
    .agents/skills/writing-for-agents/SKILL-MECHANICS.md
    Makefile
    .gitleaks.toml
    .github/workflows/ci.yml
    SECURITY.md
    skills-lock.json
    CONTRIBUTING.md
    main.go
    LICENSE
    .gitignore
    scripts/check-go.sh
    scripts/measure-startup.py
    scripts/test-check-go.py
    scripts/verify-releases.py
    scripts/build-binaries.sh
    scripts/gen-added-provider-catalogs.py
    scripts/smoke-test-subagents-binary.py
    scripts/smoke-test-codemode-binary.py
    ✓ • 497ms

## Activity

    $ cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go; cat vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go; rg -n 'type Usage|Aggregate|aggregate|Total' vendor/github.com/MichaelKinsy/PiG/ai/usage* vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_dashboard*; rg -n 'Post|Dispatch|ticker|Tick|usageService|ctx.Done' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go | head -75; cat scripts/check-go.sh; cat CONTRIBUTING.md
    package codingagent
    
    // Wires the /usage board into the interactive mode: the slash command, the
    // fullscreen overlay mount/close lifecycle, the quota service, and the local
    // activity scanner. Ports packages/coding-agent/src/modes/interactive/
    // interactive-mode.ts showUsageDashboard/closeUsageDashboard and the /usage
    // branch of the submit handler.
    
    import (
        "context"
        "path/filepath"
        "sync"
    
        "github.com/MichaelKinsy/PiG/ai"
        "github.com/MichaelKinsy/PiG/tui"
    )
    
    // UsageQuotaService is the quota source the /usage board reads.
    // *UsageService satisfies it; the interface keeps the board testable without a
    // credential store and lets the host substitute a stub.
    type UsageQuotaService interface {
        // FetchSnapshot refreshes every configured account's quota. force bypasses
        // the cache TTL.
        FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error)
        // ActiveAccount reports the read-only identity of the account a provider
        // currently routes to. No accountKey asks for the store's active account;
        // one argument names the exact account, where a nil key is an explicit
        // identity-less account, not the default.
        ActiveAccount(ctx context.Context, provider string, accountKey ...*string) (ai.UsageActiveAccount, bool)
    }
    
    // usageDashboardState is the mounted /usage overlay.
    type usageDashboardState struct {
        component    *tui.UsageDashboardComponent
        handle       *tui.OverlayHandle
        previousMode string
    }
    
    // showUsageDashboard opens the fullscreen quota board, or focuses it when it is
    // already open. Primary reports live on the alternate screen, including when
    // chat uses normal scrollback, so a regular-mode chat switches to fullscreen
    // first, as upstream does.
    func (m *InteractiveMode) showUsageDashboard() {
        if m.tuiInst == nil {
            return
        }
        if m.usageDashboard != nil {
            m.usageDashboard.handle.Focus()
            return
        }
        previousMode := "regular"
        if m.altScreen != nil {
            previousMode = "fullscreen"
        }
        if previousMode == "regular" && !m.switchTuiMode("fullscreen", true, true) {
            m.showWarning("Close active overlays before opening usage.")
            return
        }
    
        if m.usageActivity == nil {
            m.usageActivity = tui.NewUsageActivity(m.usageSessionRoots())
        } else {
            m.usageActivity.SetRoots(m.usageSessionRoots())
        }
        activity := m.usageActivity
    
        provider := ""
        if m.opts.Model != nil {
            provider = m.opts.Model.ProviderID()
        }
        service := m.opts.UsageService
        if service == nil {
            service = m.usageService
        }
        // activeAccountMu guards the identity the refresh goroutine resolves
        // against the owner loop's read for the detail view.
        var activeAccountMu sync.Mutex
        var activeAccount *ai.UsageActiveAccount
    
        component := tui.NewUsageDashboard(tui.UsageDashboardOptions{
            InitialRefresh: true,
            ViewportRows:   m.usageViewportRows,
            LoadActivity: func(ctx context.Context, push func([]tui.DailyActivityPoint)) error {
                return activity.Load(ctx, push)
            },
            RefreshProgress: func(ctx context.Context, force bool, push func(tui.UsageDashboardSnapshot)) (tui.UsageDashboardSnapshot, error) {
                if service == nil {
                    return tui.UsageDashboardSnapshot{}, nil
                }
                if provider != "" {
                    account, ok := m.resolveUsageActiveAccount(ctx, service, provider)
                    activeAccountMu.Lock()
                    if ok {
                        resolved := account
                        activeAccount = &resolved
                    } else {
                        activeAccount = nil
                    }
                    activeAccountMu.Unlock()
                }
                var snapshot ai.UsageSnapshot
                var err error
                if progressive, ok := service.(interface {
                    FetchSnapshotProgress(context.Context, bool, func(ai.UsageSnapshot)) (ai.UsageSnapshot, error)
                }); ok {
                    snapshot, err = progressive.FetchSnapshotProgress(ctx, force, func(snapshot ai.UsageSnapshot) { push(tui.NewUsageDashboardSnapshot(snapshot)) })
                } else {
                    snapshot, err = service.FetchSnapshot(ctx, force)
                }
                return tui.NewUsageDashboardSnapshot(snapshot), err
            },
            ResolveActiveAccount: func(providerID string) *ai.UsageActiveAccount {
                if providerID != provider {
                    return nil
                }
                activeAccountMu.Lock()
                defer activeAccountMu.Unlock()
                return activeAccount
            },
            RequestRender: m.requestRender,
            OnClose:       m.closeUsageDashboard,
        })
        // The board owns the whole screen: 100% width and height with no margin,
        // anchored bottom-center as upstream's showOverlay requests.
        options := tui.OverlaySpec{
            Width:     &tui.OverlayValue{Value: 100, Percent: true},
            MaxHeight: &tui.OverlayValue{Value: 100, Percent: true},
            Anchor:    "bottom-center",
            Margin:    tui.OverlayMarginAll(0),
        }.Options()
        options.Fullscreen = true
        handle := m.tuiInst.ShowOverlay(component, options)
        if handle == nil {
            return
        }
        m.usageDashboard = &usageDashboardState{component: component, handle: handle, previousMode: previousMode}
        // The overlay's first refresh and activity scan run on the owner loop so
        // their pushes land through the mode's render request.
        component.Start(m.backgroundContext())
        m.requestRender()
    }
    
    // closeUsageDashboard disposes the board, hides its overlay, and restores the
    // chat's previous screen mode.
    func (m *InteractiveMode) closeUsageDashboard() {
        dashboard := m.usageDashboard
        if dashboard == nil {
            return
        }
        m.usageDashboard = nil
        dashboard.component.Dispose()
        // Hide removes the entry synchronously (overlayRemoveTarget runs under the
        // overlay lock before apply returns), so HasOverlay is false for the
        // renderer swap and switchTuiMode's own guard passes.
        dashboard.handle.Hide()
        if !m.isShuttingDownMode() && dashboard.previousMode != m.currentTuiMode() {
            m.switchTuiMode(dashboard.previousMode, true, true)
        }
        m.requestRender()
    }
    
    // disposeUsageDashboard releases the board when the mode ends: it stops the
    // activity scan and refresh goroutines and drops the mounted state without
    // touching the renderer, which teardown is already stopping.
    func (m *InteractiveMode) disposeUsageDashboard() {
        dashboard := m.usageDashboard
        if dashboard == nil {
            return
        }
        m.usageDashboard = nil
        dashboard.component.Dispose()
        dashboard.handle.Hide()
    }
    
    // usageViewportRows reports the terminal's visible rows for the board frame.
    func (m *InteractiveMode) usageViewportRows() int {
        if m.tuiInst == nil {
            return 24
        }
        if terminal := m.tuiInst.Terminal(); terminal != nil {
            if rows := terminal.Rows(); rows > 0 {
                return rows
            }
        }
        return 24
    }
    
    // usageSessionRoots lists the session directories the activity heatmap scans:
    // the agent directory's sessions plus the current session's directory.
    func (m *InteractiveMode) usageSessionRoots() []string {
        roots := make([]string, 0, 2)
        if m.opts.AgentDir != "" {
            roots = append(roots, filepath.Join(m.opts.AgentDir, "sessions"))
        }
        session := m.currentSession()
        if session != nil {
            if dir := session.GetSessionDir(); dir != "" {
                roots = append(roots, dir)
            }
        }
        return roots
    }
    
    // usageAccountKey reports the account key this session is routed to for
    // provider. present is false when the host wires no per-session routing
    // decision, which asks the quota service for the store's active account; a
    // present result passes the key through, nil included, because a nil key names
    // an identity-less account rather than the default.
    func (m *InteractiveMode) usageAccountKey(provider string) (*string, bool) {
        if m.opts.UsageAccountKey == nil {
            return nil, false
        }
        return m.opts.UsageAccountKey(provider)
    }
    
    // resolveUsageActiveAccount reads the routed account identity for provider,
    // passing the session's key only when the host reported one.
    func (m *InteractiveMode) resolveUsageActiveAccount(ctx context.Context, service UsageQuotaService, provider string) (ai.UsageActiveAccount, bool) {
        key, present := m.usageAccountKey(provider)
        if !present {
            return service.ActiveAccount(ctx, provider)
        }
        return service.ActiveAccount(ctx, provider, key)
    }
    
    // backgroundContext returns the run's background context, or the process
    // context when the mode has not started one (component tests).
    func (m *InteractiveMode) backgroundContext() context.Context {
        if m.backgroundCtx != nil {
            return m.backgroundCtx
        }
        return context.Background()
    }
    
    // isShuttingDownMode reports whether the mode is tearing down or already torn
    // down, which keeps the board from switching the renderer back mid-stop.
    func (m *InteractiveMode) isShuttingDownMode() bool {
        return m.runEnded.Load() || m.requestExit.Load() || m.inputLoopErr != nil
    }
    
    // currentTuiMode reports the mounted renderer's mode.
    func (m *InteractiveMode) currentTuiMode() string {
        if m.altScreen != nil {
            return "fullscreen"
        }
        return "regular"
    }
    
    // usageHandler runs /usage: it opens the board and rejects unknown arguments as
    // upstream's submit handler does.
    func usageHandler(sc *SlashContext) error {
        if sc.Args != "" && sc.Args != "show" {
            if sc.ShowError != nil {
                sc.ShowError("Usage: /usage")
            }
            return nil
        }
        if sc.ShowUsage != nil {
            sc.ShowUsage()
        }
        return nil
    }
    package tui
    
    import (
        "fmt"
        "strings"
    
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    // SetWorkingStatusIndicator selects the status displayed by an opted-in editor.
    func (e *Editor) SetWorkingStatusIndicator(indicator *StatusIndicator) {
        e.workingStatusIndicator = indicator
        e.Invalidate()
    }
    
    // IsDirty reports whether the editor's rendered lines changed since the last
    // consume. The embedded status indicator is animated and relabelled by its
    // owner (spinner ticks, working message, retry countdown) without touching the
    // editor, so its own dirty flag counts too; otherwise the per-child render
    // cache would keep the old border row until a keystroke dirtied the editor.
    func (e *Editor) IsDirty() bool {
        if e.invalidatable.IsDirty() {
            return true
        }
        indicator := e.workingStatusIndicator
        return e.EmbedWorkingStatus && indicator != nil && indicator.IsDirty()
    }
    
    // NeedsRedraw consumes the editor's and the embedded indicator's dirty flags.
    func (e *Editor) NeedsRedraw() bool {
        dirty := e.invalidatable.NeedsRedraw()
        if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
            dirty = indicator.NeedsRedraw() || dirty
        }
        return dirty
    }
    
    // pig additive (D91): SurfaceLive reports that the embedded indicator, which changes without
    // invalidating the editor, draws in the editor.
    func (e *Editor) SurfaceLive() bool {
        return e.EmbedWorkingStatus && e.workingStatusIndicator != nil
    }
    
    // renderStatusBorder mirrors CustomEditor.renderTopBorder, retaining a centered
    // overflow label when the complete status or spinner can fit to its left.
    func (e *Editor) renderStatusBorder(width, hiddenLineCount int, fallback string) string {
        if !e.EmbedWorkingStatus || e.workingStatusIndicator == nil || width <= 0 {
            return fallback
        }
        indicator := e.workingStatusIndicator
        // Embedded working and compaction indicators take the editor's border color (interactive-mode.ts showWorkingStatusIndicator
        // and the compaction_start colorFn, Pi 1.2.0); branch summary and retry keep their own colors.
        if indicator.Kind == "working" || indicator.Kind == "compaction" {
            loader := &Loader{Message: indicator.Message, Frame: indicator.Frame, Frames: indicator.Frames, IndicatorVerbatim: indicator.IndicatorVerbatim, spinnerColorFn: e.colorEditorBorder, messageColorFn: e.colorEditorBorder}
            indicator = &StatusIndicator{Loader: loader, Kind: indicator.Kind}
        }
        status := indicator.RenderInBorder(max(1, width-5))
        statusWidth := widthx.VisibleWidth(status)
        if statusWidth == 0 {
            return fallback
        }
        overflowLabel := ""
        if hiddenLineCount > 0 {
            overflowLabel = fmt.Sprintf(" ↑ %d more ", hiddenLineCount)
        }
        overflowWidth := widthx.VisibleWidth(overflowLabel)
        overflowStart := (width - overflowWidth) / 2
        canFitOverflow := func() bool {
            return overflowLabel != "" && overflowWidth+2 <= width && overflowStart-(3+statusWidth+1) >= 1
        }
        if overflowLabel != "" && !canFitOverflow() {
            status = indicator.RenderSpinnerInBorder(width)
            statusWidth = widthx.VisibleWidth(status)
        }
        color := e.colorEditorBorder
        if canFitOverflow() {
            leftWidth := 3 + statusWidth + 1
            return color("── ") + status + color(" "+strings.Repeat("─", overflowStart-leftWidth)+overflowLabel+strings.Repeat("─", width-overflowStart-overflowWidth))
        }
        if width >= statusWidth+5 {
            return color("── ") + status + color(" "+strings.Repeat("─", width-statusWidth-4))
        }
        status = indicator.RenderSpinnerInBorder(width)
        statusWidth = widthx.VisibleWidth(status)
        prefixWidth := min(3, max(0, width-statusWidth))
        return color(strings.Repeat("─", prefixWidth)) + status + color(strings.Repeat("─", max(0, width-prefixWidth-statusWidth)))
    }
    rg: vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_dashboard*: No such file or directory (os error 2)
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:20:type UsageUnit string
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:34:type UsageStatus string
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:44:type UsageWindow struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:59:type UsageAmount struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:69:type UsageScope struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:82:type UsageLimit struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:93:type UsageResetCreditDetail struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:109:type UsageResetCredits struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:120:type UsageReport struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:131:type UsageLogger interface {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:138:type UsageCredential struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:159:type UsageFetchParams struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:168:type UsageFetchContext struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:178:type UsageHeaderParser func(headers map[string]string, nowMs int64, responseStatus *int) *UsageReport
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:184:type UsageProvider struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:222:type UsageHTTPError struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:240:type UsageUnavailableAccount struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:250:type UsageSnapshot struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:260:type UsageActiveAccount struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:608:type UsageAccountSettings struct {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:185:// kimiBuildAggregateRow reads one `usages[key]` aggregate.
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:186:func kimiBuildAggregateRow(key string, record map[string]any, nowMs int64) *kimiUsageRow {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:275:            // Kimi Code's aggregate quota resets weekly, but the payload carries
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:283:        if total := kimiBuildUsageRow(totalQuota, "Total quota", nowMs); total != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:327:        for key, aggregate := range usages {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:331:            aggregateRecord, ok := usageRecord(aggregate)
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:335:            if row := kimiBuildAggregateRow(key, aggregateRecord, nowMs); row != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:519:    intervalTotalCount   *float64
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:525:    weeklyTotalCount     *float64
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:565:    bucket.intervalTotalCount = readNumber("current_interval_total_count")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:571:    bucket.weeklyTotalCount = readNumber("current_weekly_total_count")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:582:    return zero(bucket.intervalTotalCount) && zero(bucket.weeklyTotalCount) &&
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:661:        bucket.intervalUsageCount, bucket.intervalTotalCount, accountID); limit != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go:666:        bucket.weeklyUsageCount, bucket.weeklyTotalCount, accountID); limit != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:396:        intervalTotalCount: &zero,
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:397:        weeklyTotalCount:   &zero,
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:27:    for _, f := range [...]float64{cost.Input, cost.Output, cost.CacheRead, cost.CacheWrite, cost.Total} {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:47:    TotalTokens  int       `json:"totalTokens"`
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:57:    TotalTokens  int       `json:"totalTokens"`
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:64:    return usageWire{usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite, usage.CacheWrite1h, usage.Reasoning, usage.TotalTokens, usage.Cost}
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:68:    return usageWireSplitsAfterCost{usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite, usage.TotalTokens, usage.Cost, usage.CacheWrite1h, usage.Reasoning}
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:80:        TotalTokens  usageInt  `json:"totalTokens"`
    vendor/github.com/MichaelKinsy/PiG/ai/usage_json.go:88:        Reasoning: value.Reasoning.ptr(), TotalTokens: int(value.TotalTokens), Cost: value.Cost}
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_claude_reset.go:172:    if resetsTotal, present := record["resets_total"]; present {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_claude_reset.go:173:        number, ok := usageNumber(resetsTotal)
    247:    ownerOverflow       []ownerPost
    363:    // spinnerTickQueued is set while a spinner tick waits in uiTaskCh.
    364:    spinnerTickQueued atomic.Bool
    365:    // spinnerWakeCh resumes a parked spinner ticker; spinnerWakeGen counts
    369:    // spinnerParkCh carries an idle tick's request to park the ticker.
    424:    // usageService fetches provider quotas for the board. nil leaves the board
    426:    usageService UsageQuotaService
    649:    // transcript view disposed exactly once across the explicit-quit, ctx.Done,
    663:    // loop (installRenderDispatcher); renderWakeCh is its channel, which every
    679:    currentInputTicket      *inputTicket
    728:// Every queued task wakes a parked spinner ticker, whose next tick re-checks
    752:// installRenderDispatcher runs throttled scheduled renders on the main input
    758:func (m *InteractiveMode) installRenderDispatcher() {
    765:    m.tuiInst.SetOwnerDispatcher(m.postToMain)
    779:// main loop. A frame wakes a parked spinner ticker, whose next tick re-checks
    830:    case <-ctx.Done():
    939:func (m *InteractiveMode) routeInputChunk(ctx context.Context, chunk []byte, readCh chan<- inputChunk) *inputTicket {
    945:func (m *InteractiveMode) routeInputSequence(ctx context.Context, chunk []byte, more bool, settled chan struct{}, readCh chan<- inputChunk) *inputTicket {
    946:    var ticket *inputTicket
    963:            case <-ctx.Done():
    969:                ticket = newInputTicket()
    971:                ticket = &inputTicket{done: settled}
    982:        case <-ctx.Done():
    998:    case <-ctx.Done():
    1580:            case <-ctx.Done():
    1622:    m.installRenderDispatcher()
    1625:    m.tuiInst.SetOverlayCommandDispatcher(func(command func()) {
    1849:    // Dispatch session_start through the extension runner. The bridge performs
    1918:            // both are owned by the main input loop. Post the work there
    1954:    // Start spinner ticker
    #!/usr/bin/env bash
    # Use canonical GOPATH imports for cacheable vendored tests. This also preserves
    # assembly/linkname identities; testing vendor/... with a prefixed import path
    # breaks wazero's native backend. Production builds and vet use module mode.
    set -euo pipefail
    cd "$(dirname "$0")/.."
    root="$PWD"
    compiler="$("${GO:-go}" env GOROOT)/bin/go"
    cache_root="${OMPIG_CHECK_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/ompig-check}"
    mkdir -p "$cache_root/gopath/src" "$cache_root/profile/agent" "$cache_root/profile/library"
    cache_root="$(cd "$cache_root" && pwd)"
    for source in "$root"/vendor/*; do
        [[ -d "$source" ]] || continue
        link="$cache_root/gopath/src/${source##*/}"
        if [[ -L "$link" ]]; then
            if [[ "$(readlink "$link")" != "$source" ]]; then rm "$link"; fi
        elif [[ -e "$link" ]]; then
            printf 'Refusing to replace non-symlink %s\n' "$link" >&2
            exit 1
        fi
        if [[ ! -L "$link" ]]; then ln -s "$source" "$link"; fi
    done
    export PIG_CODING_AGENT_DIR="$cache_root/profile/agent"
    export PI_CODING_AGENT_DIR="$PIG_CODING_AGENT_DIR"
    export OMPIG_SLIM_CODING_AGENT_DIR="$PIG_CODING_AGENT_DIR"
    export PIG_HOME="$cache_root/profile/library" PI_HOME="$cache_root/profile/library"
    export PI_SKIP_VERSION_CHECK=1 PI_TELEMETRY=0
    case "${1:-check}" in
        test|race|check)
            # Bash 3.2 (macOS) treats an empty array as unset under nounset.
            # Keep the command nonempty, including when no flags are requested.
            test_command=(test)
            if [[ "${1:-check}" == race ]]; then test_command+=(-race); fi
            "$compiler" "${test_command[@]}" -mod=vendor ./...
            # Resolve patterns in module mode: GOPATH wildcard discovery ignores
            # symlinked trees, while explicit canonical imports resolve them.
            packages=()
            package_list="$("$compiler" list -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/...)"
            if [[ -z "$package_list" ]]; then printf 'No PiG packages found\n' >&2; exit 1; fi
            while IFS= read -r package; do
                if [[ -n "$package" ]]; then packages+=("$package"); fi
            done <<< "$package_list"
            (cd "$cache_root/gopath/src" && GO111MODULE=off GOPATH="$cache_root/gopath" GOTOOLCHAIN=local \
                "$compiler" "${test_command[@]}" "${packages[@]}")
            ;;
        vet) ;;
        *) printf 'Usage: %s [check|test|race|vet]\n' "$0" >&2; exit 2 ;;
    esac
    if [[ "${1:-check}" == check || "${1:-check}" == vet ]]; then
        "$compiler" vet -mod=vendor ./... ./vendor/github.com/MichaelKinsy/PiG/...
    fi
    # Contributing to ompig-slim
    
    This is a personal coding harness built on PiG, not an official PiG, Pi, or oh-my-pi release. Keep the native core small: prefer an existing extension or skill unless a capability requires a runtime change. Do not reformat or modernize inherited code without a behavioral reason.
    
    ## Review
    
    Understand the code you submit and its interactions with the rest of the system. Agent-written code is acceptable; unread output is not. Label model-generated issue and review text. Discuss changes with the maintainer before submitting a pull request.
    
    Public development and CI run at [tmih06/ompig-slim](https://github.com/tmih06/ompig-slim). The original fork's contributor approval list is not inherited; this repository does not configure a separate hosted contributor-approval gate.
    
    ## Reports
    
    Keep reports concise. Include the commit or version, operating system, command, minimal reproduction, and relevant output. Reproduce with `--no-extensions` before attributing a failure to core behavior. Follow SECURITY.md for security-sensitive reports; do not post credentials.
    
    ## Development
    
    ```sh
    make check
    make build
    ```
    
    The checks include the patched PiG library and the executable entry point. `make test` uses a stable GOPATH symlink to enable Go’s native test-result cache for vendored packages; Go does not cache those results when testing `./vendor/...` in module mode. `make vet`, production builds, and releases use `-mod=vendor`. `make race` checks the same packages with the race detector. Go validates source, dependencies, flags, and observed test inputs before reusing results; a cache hit never skips a check command. Set `OMPIG_CHECK_CACHE` to choose the isolated test workspace.
    
    Dependencies are pinned in `go.mod`, verified by `go.sum`, and checked in under `vendor/`. The PiG sources under `vendor/github.com/MichaelKinsy/PiG` contain the personal feature ports. A blind `go mod vendor` overwrites those ports; preserve and reapply them deliberately when upgrading the library. Keep generated upstream files unchanged and use dedicated native port modules where possible.
    
    ## Change rules
    
    - One topic per change; keep the diff to its stated behavior.
    - Bug fixes need a meaningful regression test covering the failing boundary.
    - Use isolated temporary directories, fixture credentials, and local HTTP servers; tests must not need real credentials or paid model calls.
    - Keep dependency and lock changes separate from unrelated behavior changes. Pin direct dependencies exactly and preserve license notices.
    - Never remove another session's changes. Stage explicit paths and do not force-push.
    - Use commit subjects such as `feat(auth): ...`, `fix(tui): ...`, or `docs: ...`.
    
    CI runs tests, vet, race checks, compiled codemode and PTY startup smoke tests on Linux and macOS. It separately validates workflows and scripts, scans all Git history including nested archives for secrets, and verifies checksums, executable headers, and bundled notices for all four release archives. Actions and CI tools are pinned. Go caches are separated by OS, architecture, toolchain, and job, with source-based save keys and compatible restore prefixes. Tool caches include their exact versions.
    
    The README describes the current feature set and supported native builds. Source behavior for the personal ports comes from tmih06's omp-slim fork; PiG remains the reference for inherited features.
    ✓ • 16ms

## Activity

    $ wc -l vendor/github.com/MichaelKinsy/PiG/AGENTS.md; sed -n '150,360p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md; cat vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go | head -130; rg -n 'Aggregate|aggregate|Summary|summary' vendor/github.com/MichaelKinsy/PiG/tui/usage* | head -90; sed -n '1945,1990p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; sed -n '720,755p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go | head -110; cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_models.go | sed -n '270,320p'; rg -n 'UsageService|backgroundCancel|backgroundCtx' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
    673 vendor/github.com/MichaelKinsy/PiG/AGENTS.md
    fused realizations of one factory must pass the same conformance behavior.
    
    ## TypeScript-to-Go semantic translation
    
    Read `docs/typescript-to-go-porting.md` before translating a new behavior
    family or changing a parity-facing Go representation. Port observed semantics,
    not TypeScript syntax: account explicitly for omitted/undefined/null state,
    closed unions, insertion order, UTF-16/code-point/terminal-cell units, numeric
    conversion, exceptions, callback ownership, Node path/URL/regex/JSON behavior,
    and every state or wire effect that callers observe. A TypeScript `interface`
    does not imply a Go interface; use a struct for data and a small consumer-owned
    Go interface only for a real behavioral contract. Constructors normally return
    concrete types. "Port semantics, not syntax" governs behavior, not naming: it is
    not license to rename. Mirror upstream identifiers and structure 1:1 (camelCase →
    exported PascalCase, same literal union values, same field names), and diverge
    only where a Go language mechanic forces it (no `Symbol.for`, no field-bearing
    interfaces, discriminated union → sealed interface), recording each forced case.
    A needless rename is drift that inflates the next upstream sync and obscures
    correspondence. Do not change a public extension or TUI contract for presumed
    speed: profile the representative path and prove allocations/inlining and
    byte-faithful behavior before and after.
    
    ## Faithful, general implementations
    
    Port Pi's design, not one case's output. Use Pi's shared data (model catalog, `defaultModelPerProvider`, auth metadata) and one shared function where Pi has one. Never hard-code a provider or model in shared code; cite the Pi line when Pi itself special-cases one. Test shared paths with several provider shapes, never only Copilot or `test-faux`, and fix a special case's siblings together.
    
    ## TypeScript async/Promise parity
    
    Treat every upstream `async`, `Promise`, `.then`, `Promise.all`, `Promise.race`, and intentionally unawaited call as control-flow behavior, not syntax to erase. Before porting it, record whether the caller waits, what ordering is guaranteed, how cancellation and errors propagate, whether work is concurrent, and which executor/event loop owns callbacks. Then preserve that contract in Go:
    
    - an awaited Promise normally maps to a blocking Go call that returns its value/error; do not add `go func()` merely because TypeScript says `async`;
    - `Promise.all`/`allSettled` maps to owned, joined concurrency with deterministic result/error handling, not detached goroutines;
    - an intentionally unawaited Promise maps only to an owned background task with cancellation, error reporting, and shutdown draining; naked fire-and-forget goroutines are not equivalent;
    - Promise rejection maps to a surfaced Go error on the same operation unless upstream explicitly handles it;
    - `AbortSignal` and request/session lifetime map to `context.Context` or an equivalent cancellation channel;
    - async TUI/event handlers perform slow work off the UI loop and marshal UI mutation back through the loop's approved run-on-main path.
    
    Probe upstream call sites, not only the declared return type: an `async` function with no `await` may still create a microtask boundary, while a Promise-returning callback may be awaited by its host. Regression evidence must prove completion ordering, cancellation, and error propagation; output-only happy-path tests do not prove async parity. When adding or changing an extension API, record the async contract in `docs/extension-api-parity.md` and exercise isolated and packed subprocess modes where applicable.
    
    ## Reward function
    
    Highest-value work, in order:
    1. Find a pig-vs-pi behavior difference and fix it at the source, or number it in `docs/parity/DIVERGENCES.md`.
    2. Tighten an existing scenario comparator: `wait_contains` → `output_normalized_equal` → `output_equal` → `escaped_output_equal`, or increase `runs` where flake risk matters.
    3. Promote silent drift to a numbered divergence with call-site markers, remove condition, and parity coverage or explicit allowance.
    4. Add honest file coverage where the asserted output actually exercises the claimed `covers` paths.
    
    Low-value or misleading work: green check counts, scenario counts, `covers` entries not exercised by the assertion, normalized output hiding a real rendering difference, `runs = 1` for non-trivial behavior, renderer band-aids when the bug is upstream-source logic, and stopping because `make check` is green.
    
    ## Loop smells
    
    Stop and report instead of claiming success when:
    - Everything passed on the first try.
    - A scenario passed before and after the supposed fix.
    - The loop produced zero bugs, zero comparator tightenings, and zero numbered divergences.
    - A scenario claims coverage but asserts only banner, prompt, or ready text.
    - ANSI or whitespace was normalized to hide a real difference.
    - `family-gaps` is green only because one broad scenario claims many files.
    - A comment explains a rendering difference but no divergence number exists.
    
    Investigation is the work. Green is necessary, not sufficient.
    
    ## Loop self-report
    
    Before `signal_loop_success`, include this block in the final assistant message and fill it honestly:
    
    ```md
    ### Loop self-report
    
    Family: <name>
    
    Bugs found and fixed at the source (count: N):
      - <cause> → <fix> @ <file:line>
    
    Comparators tightened (count: N):
      - <scenario>: <before> → <after>
    
    Divergences numbered in docs/parity/DIVERGENCES.md (count: N):
      - D<N> <one-line>
    
    Lint suppressions added or changed (count: N):
      - <rule> @ <file/config>: <why source fix would be less faithful/correct>
    
    New file coverage (count: N PORT_MAP entries newly ✅):
      - <upstream path>
    
    Band-aids consciously chosen (count: N, should be 0):
      - <site>: <symptom>: <why root fix deferred>: <follow-up>
    
    Scope expansion (count: N files outside nominal family):
      - <file>: <reason>: <call sites audited>
    
    Cross-family scenarios re-probed/tightened (count: N):
      - <family>/<scenario>: <what changed and why>
    
    Smells investigated (count: N):
      - <smell> → <finding>
    
    Smells deferred to user (count: N, should be 0 unless blocked):
      - <smell>: <blocker>
    ```
    
    A report with all zero counts is a failed loop unless the user explicitly accepts that outcome.
    
    ## Coverage
    
    The block below is generated by `make coverage
    make foundation-check
    make interface-inventory
    make interface-inventory-test
    make interface-inventory-drift
    make interface-go-drift
    make interface-recommendations-drift
    make interface-recommendations
    make interface-mapping-quality
    make interface-mapping-strict
    make upstream-delta
    make async-contracts` or `make verify` from `docs/parity/PORT_MAP.md` and `test/parity/scenarios/**/*.toml`. Do not hand-edit between the markers. Fix `test/parity/cmd/coverage/main.go` if the math is wrong.
    
    <!-- BEGIN COVERAGE -->
    <!--
      Machine-generated by `make coverage`. Do not hand-edit.
      This block is the ONLY status claim AGENTS.md makes about the
      port. Everything else is invariant rule, not progress narrative.
    -->
    
    **Porting:** 910 / 929 intended-portable entries ✅ (98.0%); **Verification:** 837 behavioral (92.0%), 73 untested.
    Raw PORT_MAP rows: 1032. Breakdown: 103 n/a (designed out) · 19 🟡 partial. See docs/parity/DIVERGENCES.md for the documented exceptions.
    
    | package | src files | n/a | intended | ✅ | 🟡 | ⬜ | ported | behavioral |
    |---|---:|---:|---:|---:|---:|---:|---:|---:|
    | `agent` | 6 | 1 | 5 | 5 | 0 | 0 | 100.0% | 100.0% |
    | `ai` | 265 | 22 | 243 | 243 | 0 | 0 | 100.0% | 98.8% |
    | `chord` | 247 | 28 | 219 | 213 | 6 | 0 | 97.3% | 86.9% |
    | `codemode` | 17 | 4 | 13 | 13 | 0 | 0 | 100.0% | 100.0% |
    | `coding-agent` | 256 | 27 | 229 | 228 | 1 | 0 | 99.6% | 99.6% |
    | `durable` | 97 | 10 | 87 | 76 | 11 | 0 | 87.4% | 98.7% |
    | `env` | 23 | 3 | 20 | 19 | 1 | 0 | 95.0% | 100.0% |
    | `evals` | 6 | 0 | 6 | 6 | 0 | 0 | 100.0% | 100.0% |
    | `mcp` | 18 | 3 | 15 | 15 | 0 | 0 | 100.0% | 100.0% |
    | `schema` | 43 | 3 | 40 | 40 | 0 | 0 | 100.0% | 0.0% |
    | `telemetry` | 8 | 1 | 7 | 7 | 0 | 0 | 100.0% | 100.0% |
    | `tui` | 46 | 1 | 45 | 45 | 0 | 0 | 100.0% | 100.0% |
    
    - **Four core packages (agent, ai, coding-agent, tui):** 521 / 522 intended-portable files ✅ (99.8%) from 573 rows; 517 behavioral (99.2%), 0 weak-only, 4 untested.
    - **Shipped surface (agent, ai, codemode, coding-agent, mcp, schema, telemetry, tui; coding-agent without ):** 596 / 597 intended-portable files ✅ (99.8%) from 659 rows; 552 behavioral (92.6%), 0 weak-only, 44 untested.
    - **Whole monorepo (12 packages):** 910 / 929 intended-portable files ✅ (98.0%) from 1032 rows; 837 behavioral (92.0%), 0 weak-only, 73 untested.
    - **Interface closure:** 14466 of 14469 semantic interface IDs closed (395 designed-out, 3 pending, 14071 ported). A file-level ✅ does not close an interface ID.
    
    Behavioral evidence includes paired scenarios and reviewed mutation-proven unit tests; the family table below counts paired scenarios only.
    Weak scenarios not counted as behavioral verification: 4 boot-only, 4 registration-only, 1 smoke-only.
    
    | family | scenarios | behavioral | boot-only | weak | deferred | upstream behavioral covered | last run |
    |---|---:|---:|---:|---:|---:|---:|---|
    | `_top` | 1 | 1 | 0 | 0 | 0 | 1 | not run |
    | `ai-sdk` | 3 | 3 | 0 | 0 | 0 | 90 | not run |
    | `autocomplete` | 14 | 14 | 0 | 0 | 0 | 5 | not run |
    | `cli-utils` | 38 | 38 | 0 | 0 | 0 | 17 | not run |
    | `clipboard-images` | 7 | 7 | 0 | 0 | 2 | 11 | not run |
    | `compaction` | 15 | 15 | 0 | 0 | 0 | 10 | not run |
    | `export-html` | 7 | 7 | 0 | 0 | 0 | 2 | not run |
    | `extension-host` | 1 | 1 | 0 | 0 | 0 | 2 | not run |
    | `extensions-runtime` | 143 | 142 | 0 | 1 | 0 | 63 | not run |
    | `footer` | 12 | 12 | 0 | 0 | 0 | 9 | not run |
    | `fullscreen` | 14 | 14 | 0 | 0 | 0 | 11 | not run |
    | `interactive-rendering` | 73 | 72 | 1 | 0 | 0 | 37 | not run |
    | `json` | 14 | 14 | 0 | 0 | 0 | 10 | not run |
    | `model-resolver-selector` | 28 | 28 | 0 | 0 | 0 | 16 | not run |
    | `model-runtime-store-catalog` | 25 | 25 | 0 | 0 | 0 | 24 | not run |
    | `oauth` | 32 | 32 | 0 | 0 | 0 | 19 | not run |
    | `print` | 13 | 13 | 0 | 0 | 0 | 9 | not run |
    | `project-trust` | 26 | 26 | 0 | 0 | 0 | 19 | not run |
    | `providers-faux-streaming` | 66 | 65 | 0 | 1 | 0 | 49 | not run |
    | `providers-registry` | 9 | 6 | 0 | 3 | 0 | 27 | not run |
    | `rpc` | 93 | 93 | 0 | 0 | 0 | 32 | not run |
    | `selectors` | 14 | 14 | 0 | 0 | 1 | 15 | not run |
    | `session` | 43 | 43 | 0 | 0 | 0 | 19 | not run |
    | `settings` | 16 | 16 | 0 | 0 | 0 | 21 | not run |
    | `slash-commands` | 29 | 28 | 1 | 0 | 0 | 21 | not run |
    | `startup` | 19 | 18 | 1 | 0 | 0 | 10 | not run |
    | `tools` | 32 | 32 | 0 | 0 | 0 | 35 | not run |
    | `tree` | 9 | 8 | 1 | 0 | 0 | 9 | not run |
    | `tui-components` | 39 | 39 | 0 | 0 | 0 | 36 | not run |
    
    Full per-file detail: `test/parity/coverage.md`.
    
    <!-- END COVERAGE -->
    
    ## Reliability contexts
    
    Define the upstream contract, complete user path, and material failure modes before implementation. Follow the verification sequence below. Use the complete PiG path as the primary proof for behavior that crosses a Provider, Session, mode, Extension Host, process, or terminal boundary. Retain repeatable evidence.
    
    Test empty, ordinary, boundary, and representative stress inputs. Verify ordering, cancellation, replacement, cleanup, and retained state. Measure latency, memory, allocation, and backpressure on critical paths. A small-input pass does not prove bounded behavior.
    
    Use a term from `docs/project/CONTEXT.md` when that file defines it. Do not redefine ratified terms in a module instruction. Read the nearest module `AGENTS.md` before work in a known reliability context. The root owns shared invariants. The module owns its contract, failure model, and evidence. Store each rule once.
    
    | Context | Instructions |
    |---|---|
    | Transcript, Provider, Event Stream | `ai/AGENTS.md` |
    | Main Screen, terminal rendering, input editing | `tui/AGENTS.md` |
    | Session, Model Runtime, execution mode | `coding/AGENTS.md` |
    | Extension Host, SDK, topology, lifecycle | `coding/extension/AGENTS.md` |
    | upstream oracle, scenario, comparator, evidence | `test/parity/AGENTS.md` |
    | Stock PiG artifact, PiG Standard Piglet release, provenance, publication | `.github/AGENTS.md` |
    
    Apply these rules to new work. Do not delete or weaken accepted tests to conform. Coordinate with the lane owner before changing evidence. Brief or restart active agents after an instruction change.
    
    ## Upstream test parity
    
    A production file is `ported` only when every upstream test file covering it is ported with the same inputs and expected results, or designed out with a numbered, reviewed reason. Record test locations and current upstream hashes in `test/parity/interfaces/test-mapping-v<version>.json`. A `partial` disposition enumerates the missing cases; file-name similarity, existing code, and a passing smoke test do not close them. Port the covering upstream tests in the same change whenever you port or change a production file. Preserve ordering, cancellation, errors, retained state, and caller effects, not only happy-path output.
    
    package ai
    
    // Ports packages/ai/src/utils/usage.ts and the provider contract of
    // packages/ai/src/usage.ts: the normalized quota-report schema shared by the
    // usage dashboard and the multi-account quota service, plus the pure helpers
    // the display layer resolves limits with.
    
    import (
        "context"
        "encoding/json"
        "math"
        "net/http"
        "sort"
        "strconv"
        "strings"
        "unicode/utf16"
    )
    
    // UsageUnit is the unit of one quantitative amount.
    type UsageUnit string
    
    const (
        UsageUnitPercent  UsageUnit = "percent"
        UsageUnitTokens   UsageUnit = "tokens"
        UsageUnitRequests UsageUnit = "requests"
        UsageUnitCredits  UsageUnit = "credits"
        UsageUnitUSD      UsageUnit = "usd"
        UsageUnitMinutes  UsageUnit = "minutes"
        UsageUnitBytes    UsageUnit = "bytes"
        UsageUnitUnknown  UsageUnit = "unknown"
    )
    
    // UsageStatus is the normalized health of one limit.
    type UsageStatus string
    
    const (
        UsageStatusOK        UsageStatus = "ok"
        UsageStatusWarning   UsageStatus = "warning"
        UsageStatusExhausted UsageStatus = "exhausted"
        UsageStatusUnknown   UsageStatus = "unknown"
    )
    
    // UsageWindow is the time window a limit applies to.
    type UsageWindow struct {
        // ID is a stable identifier such as "5h", "7d" or "monthly".
        ID string `json:"id"`
        // Label is the human label ("5 Hour", "7 Day").
        Label string `json:"label"`
        // DurationMs is the window length in milliseconds when known.
        DurationMs *int64 `json:"durationMs,omitempty"`
        // ResetsAt is the absolute reset time in epoch milliseconds.
        ResetsAt *int64 `json:"resetsAt,omitempty"`
        // ResetLabel is the verb shown before the countdown ("tick", "regen");
        // empty means "resets".
        ResetLabel string `json:"resetLabel,omitempty"`
    }
    
    // UsageAmount is the quantitative data of one limit.
    type UsageAmount struct {
        Used              *float64  `json:"used,omitempty"`
        Limit             *float64  `json:"limit,omitempty"`
        Remaining         *float64  `json:"remaining,omitempty"`
        UsedFraction      *float64  `json:"usedFraction,omitempty"`
        RemainingFraction *float64  `json:"remainingFraction,omitempty"`
        Unit              UsageUnit `json:"unit"`
    }
    
    // UsageScope describes what a limit applies to.
    type UsageScope struct {
        Provider    string `json:"provider"`
        AccountID   string `json:"accountId,omitempty"`
        ProjectID   string `json:"projectId,omitempty"`
        OrgID       string `json:"orgId,omitempty"`
        ModelID     string `json:"modelId,omitempty"`
        Tier        string `json:"tier,omitempty"`
        WindowID    string `json:"windowId,omitempty"`
        Shared      bool   `json:"shared,omitempty"`
        SharedGroup string `json:"sharedGroup,omitempty"`
    }
    
    // UsageLimit is one normalized quota window or bucket.
    type UsageLimit struct {
        ID     string       `json:"id"`
        Label  string       `json:"label"`
        Scope  UsageScope   `json:"scope"`
        Window *UsageWindow `json:"window,omitempty"`
        Amount UsageAmount  `json:"amount"`
        Status UsageStatus  `json:"status,omitempty"`
        Notes  []string     `json:"notes,omitempty"`
    }
    
    // UsageResetCreditDetail is one saved banked reset a provider issued.
    type UsageResetCreditDetail struct {
        ID             string             `json:"id,omitempty"`
        Title          string             `json:"title,omitempty"`
        Program        string             `json:"program,omitempty"`
        RemainingCount int                `json:"remainingCount,omitempty"`
        Usable         *bool              `json:"usable,omitempty"`
        RequiresLimit  *bool              `json:"requiresLimit,omitempty"`
        Clears         []string           `json:"clears,omitempty"`
        Blocking       []string           `json:"blocking,omitempty"`
        UsedFractions  map[string]float64 `json:"usedFractions,omitempty"`
        GrantedAt      string             `json:"grantedAt,omitempty"`
        ExpiresAt      string             `json:"expiresAt,omitempty"`
        Status         string             `json:"status,omitempty"`
    }
    
    // UsageResetCredits is an account's banked rate-limit resets.
    type UsageResetCredits struct {
        AvailableCount  int                      `json:"availableCount"`
        RedeemableCount *int                     `json:"redeemableCount,omitempty"`
        NextCreditID    string                   `json:"nextCreditId,omitempty"`
        Eligible        *bool                    `json:"eligible,omitempty"`
        Reason          string                   `json:"reason,omitempty"`
        CooldownUntil   string                   `json:"cooldownUntil,omitempty"`
        Credits         []UsageResetCreditDetail `json:"credits,omitempty"`
    }
    
    // UsageReport is one provider account's quota snapshot.
    type UsageReport struct {
        Provider     string             `json:"provider"`
        FetchedAt    int64              `json:"fetchedAt"`
        Limits       []UsageLimit       `json:"limits"`
        ResetCredits *UsageResetCredits `json:"resetCredits,omitempty"`
        Notes        []string           `json:"notes,omitempty"`
        Metadata     map[string]any     `json:"metadata,omitempty"`
        Raw          any                `json:"raw,omitempty"`
    }
    
    // UsageLogger is the optional logger handed to usage fetchers.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:606:        cardStatus = AggregateStatus(windowStatuses)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:949:    summary := make([]string, 0, 12)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:955:    summary = append(summary, clipRow(
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:959:    summary = append(summary, "")
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:970:    summary = append(summary, theme.Fg(string(ThemeColorDim), clipRow(monthLine, innerWidth)))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:987:        summary = append(summary, clipRow(strings.TrimRight(line.String(), " "), innerWidth))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:989:    return summary
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:260:// resolveUsageAggregateStatus maps an aggregate limit status to the report's
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:262:func resolveUsageAggregateStatus(limits []ai.UsageLimit) usageDisplayStatus {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:267:    status := AggregateStatus(statuses)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:288:// formatUsageAggregateAmount is the provider-level column summary: `% free`
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:290:func formatUsageAggregateAmount(limits []ai.UsageLimit) string {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:454:    // Reserve the aggregate's columns in every block; it prints once at the end.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:695:            amountText := formatUsageAggregateAmount(sortedLimits)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:696:            status := resolveUsageAggregateStatus(sortedLimits)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:743:                    // Wrapped buckets lose the single trailing aggregate, so each
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:7:// size changed, and aggregates billed provider requests per local calendar
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:72:// the fresh aggregate. Pushes stop as soon as ctx is cancelled.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:87:    a.points = aggregateActivityDays(a.files, cutoff)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:222:// aggregateActivityDays groups cached requests by local calendar day, counting
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:224:func aggregateActivityDays(files map[string]*cachedActivityFile, cutoff int64) []DailyActivityPoint {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_activity.go:289:    case "usage", "model_usage", "compaction", "branch_summary":
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:179:// UsageResetSummary is the saved inventory and current eligibility shared by
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:181:type UsageResetSummary struct {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:204:func SummarizeUsageResetCredits(reset *ai.UsageResetCredits, nowMs int64) *UsageResetSummary {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:279:    return &UsageResetSummary{
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:668:// AggregateStatus aggregates status across a bucket's limits, mirroring the
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:671:func AggregateStatus(statuses []ai.UsageStatus) ai.UsageStatus {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:703:    if AggregateStatus(statuses) == ai.UsageStatusExhausted {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:711:// CardWindowRow is one quota bucket on a provider card: the account-aggregate
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:843:// ProviderCardResetCredits is the saved-reset summary on a provider card.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:851:// ProviderCard is the compact per-provider summary backing one card in the
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:984:                Status:    AggregateStatus(statuses),
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1035:        var resetRows []*UsageResetSummary
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1037:            summary := SummarizeUsageResetCredits(report.ResetCredits, nowMs)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1038:            if summary != nil && summary.BankedCount > 0 {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1039:                resetRows = append(resetRows, summary)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1047:        for _, summary := range resetRows {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1048:            bankedCount += summary.BankedCount
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1049:            redeemableCount += summary.RedeemableCount
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1050:            if summary.SoonestExpiry != "" {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1051:                if parsed, err := time.Parse(time.RFC3339Nano, summary.SoonestExpiry); err == nil {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1055:            if summary.UnavailableReason != "" && !seenReasons[summary.UnavailableReason] {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1056:                seenReasons[summary.UnavailableReason] = true
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:1057:                reasons = append(reasons, summary.UnavailableReason)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:189:func TestUsageResetCreditSummary(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:193:    summary := SummarizeUsageResetCredits(&ai.UsageResetCredits{
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:200:    if summary == nil {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:201:        t.Fatal("nil summary")
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:203:    if summary.BankedCount != 3 {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:204:        t.Errorf("banked = %d, want 3", summary.BankedCount)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:207:    if summary.RedeemableCount != 3 {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:208:        t.Errorf("redeemable = %d, want 3", summary.RedeemableCount)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:210:    if summary.SoonestExpiry != soon {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:211:        t.Errorf("soonest = %q, want %q", summary.SoonestExpiry, soon)
                    _ = m.postToMain(m.backgroundCtx, m.updateProviderInfo)
                }
            })
        }
    
        // Agent events are drained by the inputLoop select (case <-m.eventCh →
        // handleAgentEvent), so they run on the main goroutine single-threaded with
        // keystrokes and posted UI tasks. No separate event-processor goroutine.
    
        // Start spinner ticker
        go m.tickSpinner(ctx)
    
        m.startGitBranchWatcher(ctx)
    
        // Terminal resize re-render. The TUI re-queries term.GetSize on every
        // Render() but renders only fire on input/stream/flash events, so a resize
        // while pig is idle leaves stale-width rules until the next keystroke. Unix
        // drives this from SIGWINCH; Windows (no SIGWINCH) polls the console size.
        // Mirrors upstream `process.stdout.on("resize", ...)`.
        stopResize := m.installResizeHandler(ctx)
        m.onStop(func() error { stopResize(); return nil })
    
        // Auto-submit initial message (positional arg / piped
        // stdin) once the TUI is up. Mirrors upstream's initial-message
        // dispatch in coding-agent/cli/initial-message.ts: the message
        // is fed through the same handleSubmit path the user's keystrokes
        // hit, so transcript rendering / session writes / extension
        // hooks all see it identically.
        if strings.TrimSpace(m.opts.InitialMessage) != "" || len(m.opts.InitialImages) > 0 {
            m.handleSubmitWithImages(ctx, m.opts.InitialMessage, m.opts.InitialImages)
        }
        if len(m.opts.InitialMessages) > 0 {
            done := make(chan struct{})
            m.initialMessagesDone = done
            m.backgroundTasks.Go(func() {
                defer close(done)
                m.submitInitialMessages(ctx, m.opts.InitialMessages)
            })
        }
        mark("interactive-ready")
        m.frontendInputReady()
        return m.inputLoop(ctx, os.Stdin)
    }
    
    // postUITask hands a closure to the main input loop to run
    // single-threaded with keystroke handling. Used by background workers
    // (async autocomplete) so they never mutate the lock-free TUI component
    // tree from their own goroutine. The send is non-blocking: if the queue
    // is saturated or the loop has exited, the (cosmetic) task is dropped
    // and the next keystroke refreshes, avoiding worker-goroutine leaks on
    // shutdown. It reports whether fn was queued.
    // Every queued task wakes a parked spinner ticker, whose next tick re-checks
    // whether the task started work that animates.
    func (m *InteractiveMode) postUITask(fn func()) bool {
        if !m.postUITaskQuiet(fn) {
            return false
        }
        m.wakeSpinner()
        return true
    }
    
    // postUITaskQuiet is postUITask without the spinner wake, for the spinner's
    // own ticks.
    func (m *InteractiveMode) postUITaskQuiet(fn func()) bool {
        if fn == nil {
            return false
        }
        select {
        case m.uiTaskCh <- fn:
            return true
        default:
            return false
        }
    }
    
    // installRenderDispatcher runs throttled scheduled renders on the main input
    // loop instead of a throttle-timer goroutine, so doRender never reads the
    // lock-free component tree concurrently with the main loop's mutations (e.g.
    // the working-spinner frame advanced by tickSpinner). The loop keeps the next
    package codingagent
    
    import (
        "fmt"
        "time"
    
        "github.com/MichaelKinsy/PiG/coding/extension"
        "github.com/MichaelKinsy/PiG/tui"
    )
    
    // Only the active editor can opt into border status.
    // upstream: packages/coding-agent/src/modes/interactive/interactive-mode.ts:setEditorWorkingStatusIndicator
    func (m *InteractiveMode) setEditorWorkingStatusIndicator(indicator *tui.StatusIndicator) bool {
        if m.editor == nil {
            return false
        }
        if m.remoteEditor != nil {
            embeds := m.remoteEditor.editor.EmbedWorkingStatus()
            // A delegated component draws its border through the host editor's own render (its `super.render`), so the indicator goes to that editor when the component opts in.
            if embeds && m.editor.IsDelegated() {
                m.editor.SetWorkingStatusIndicator(indicator)
            } else {
                m.editor.SetWorkingStatusIndicator(nil)
            }
            return embeds
        }
        if !m.editor.EmbedWorkingStatus {
            indicator = nil
        }
        m.editor.SetWorkingStatusIndicator(indicator)
        return m.editor.EmbedWorkingStatus
    }
    
    func (m *InteractiveMode) showStatusIndicator(indicator *tui.StatusIndicator) {
        m.clearStatusIndicator("")
        m.activeStatusIndicator = indicator
        m.activeWorkingIndicatorEmbedded = m.setEditorWorkingStatusIndicator(indicator)
        m.statusLastFrame = time.Now()
        m.statusContainer.Clear()
        if !m.activeWorkingIndicatorEmbedded {
            m.statusContainer.Add(indicator)
        }
        m.reconfigureRemoteEditor()
        m.resetSpinnerInterval()
    }
    
    func (m *InteractiveMode) clearStatusIndicator(kind string) {
        previous := m.activeStatusIndicator
        if kind != "" && (previous == nil || previous.Kind != kind) {
            return
        }
        embedded := m.activeWorkingIndicatorEmbedded
        if previous != nil && previous.Kind == "retry" && m.retryCountdownStop != nil {
            m.retryCountdownStop()
            m.retryCountdownStop = nil
        }
        m.activeStatusIndicator = nil
        m.activeWorkingIndicatorEmbedded = false
        m.setEditorWorkingStatusIndicator(nil)
        if m.statusContainer != nil {
            m.statusContainer.Clear()
            if previous != nil && !embedded && m.opts.TuiMode != "fullscreen" && m.tuiInst != nil && m.statusClearOnShrink() {
                m.statusContainer.Add(&tui.IdleStatus{})
            }
        }
        m.reconfigureRemoteEditor()
        m.resetSpinnerInterval()
    }
    
    func (m *InteractiveMode) statusCancelHint() string {
        key := "escape"
        if m.keybindings != nil {
            key = m.keybindings.KeyText("app.interrupt")
        }
        return "(" + key + " to cancel)"
    }
    
    func (m *InteractiveMode) showCompactionStatusIndicator(reason string) {
        label := "Auto-compacting... "
        switch reason {
        case "manual":
            label = "Compacting context... "
        case "overflow":
            label = "Context overflow detected, Auto-compacting... "
        }
        m.showStatusIndicator(&tui.StatusIndicator{Kind: "compaction", Loader: tui.NewLoader(m.tuiInst, tui.ThemeFg("accent"), tui.ThemeFg("muted"), label+m.statusCancelHint(), nil)})
    }
    
    func (m *InteractiveMode) showBranchSummaryStatusIndicator() {
        m.showStatusIndicator(&tui.StatusIndicator{Kind: "branchSummary", Loader: tui.NewLoader(m.tuiInst, tui.ThemeFg("accent"), tui.ThemeFg("muted"), "Summarizing branch... "+m.statusCancelHint(), nil)})
    }
    
    // showRetryStatusIndicator owns one countdown until replacement, disposal, or shutdown.
    // The owner loop applies its queued labels; disposal joins the worker even if it is waiting for queue space.
    func (m *InteractiveMode) showRetryStatusIndicator(attempt, maxAttempts, delayMs int) {
        m.clearStatusIndicator("")
        remaining := int((delayMs + 999) / 1000)
        cancelHint := m.statusCancelHint()
        message := func(seconds int) string {
            return fmt.Sprintf("Retrying (%d/%d) in %ds... %s", attempt, maxAttempts, seconds, cancelHint)
        }
        m.setStatusContainerLabel(message(remaining))
        stop := make(chan struct{})
        stopped := make(chan struct{})
        m.retryCountdownStop = func() {
            close(stop)
            <-stopped
        }
        var done <-chan struct{}
        if m.runCtx != nil {
        }
        if result == nil {
            message := "Only one model available"
            if len(handle.ScopedModels()) > 0 {
                message = "Only one model in scope"
            }
            m.showStatus(message)
            return
        }
        m.opts.Model = result.Model
        m.statusLine.SetModel(result.Model)
        m.refreshThinkingLevel()
        m.awaitModelChange(ctx, func() error {
            if complete != nil {
                complete()
            }
            return nil
        }, func(err error) {
            if err != nil {
                m.showError(err.Error())
                return
            }
            m.finishCycleModel(result)
        })
    }
    
    // finishCycleModel is the rest of Pi's cycleModel once model_select settled.
    func (m *InteractiveMode) finishCycleModel(result *ModelCycleResult) {
        // A handler may have changed the thinking level.
        m.refreshThinkingLevel()
        displayName := result.Model.DisplayName
        if displayName == "" {
            displayName = result.Model.ID
        }
        thinking := ""
        if (result.Model.ProviderMeta.Reasoning || result.Model.Capabilities.MaxThinking != "") && result.ThinkingLevel != ai.ThinkingOff {
            thinking = " (thinking: " + string(result.ThinkingLevel) + ")"
        }
        m.showStatus("Switched to " + displayName + thinking)
        // Pi warns after a successful cycle too (interactive-mode.ts:4510).
        m.maybeWarnAboutAnthropicSubscriptionAuthAsync()
    }
    
    func modelSpec(model *ai.Model) string {
        if model == nil {
            return ""
        }
        providerID := model.ProviderMeta.ProviderID
        if providerID == "" && model.Provider != nil {
            providerID = model.Provider.ID()
        }
    82:    // UsageService is the replacement Session's quota service, built from its own
    84:    UsageService    UsageQuotaService
    202:    m.opts.UsageService = r.UsageService
    ✓ • 158ms

## Activity

    $ sed -n '360,440p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md; sed -n '560,673p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md; sed -n '710,755p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '850,1015p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '35,92p' vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go; rg -n 'func .*Card|func .*Snapshot' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; rg -n 'func .*Render|func .*render|SetModel' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_frontend.go | head; rg -n 'm.opts.Model =|m.editor =' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*.go | head -45; ls vendor/github.com/MichaelKinsy/PiG/docs/project/CONTEXT.md vendor/github.com/MichaelKinsy/PiG/.upstream/current/packages/tui/src/components/editor.ts
    
    No release may treat a pending or partial upstream test tagged `hot-path` as closed. `make ci-contracts` enforces the reviewed path tags in `test/parity/interfaces/test-porting-policy-v<version>.json` and rejects a decrease in the total ported-file count against the committed baseline. The owner-approved 0.3.x exception records explicit `deferred-0.3.x` rows without changing their partial/pending mappings, hot-path tags, assertions or baseline; [the release-policy procedure](test/parity/README.md#approved-03x-test-porting-gaps) defines the required approval, missing-case reference and follow-up. All unlisted pending/partial hot paths remain release blockers. Keep tag review and the ported baseline current in the same change; never remove a hot-path tag or lower the baseline to pass the gate. A credential or toolchain requirement is not a scope exclusion. Dispatch test-file ownership through `docs/parity/pending-tests-batches.md`; coordinate production files and shared evidence separately.
    
    ## Verification-driven development
    
    For core parity surfaces (agent loop, message normalization, provider payload conversion, session persistence, auth, tool execution, interactive rendering), use a red → green → refactor loop. Apply [Faithful, general implementations](#faithful-general-implementations).
    
    Bug fixes require a regression guard in the same PR. Add a new unit test, integration test, or parity scenario that would fail on the observed bug, or name the existing failing test that already proves it. If no automated guard is practical, say why in the PR and file a follow-up before merge. Do not rely on manual repro alone for a fixed bug.
    
    1. **Reproduce first.** Add or identify a test/scenario that fails on the current code before changing behavior. If you already patched before writing the test, prove the test is meaningful by describing the exact pre-fix failure it would have caught.
    2. **Compare to upstream.** Read the upstream TypeScript implementation and encode the observable rule in the test name/comment. For provider/message conversion, prefer table-driven tests that assert the serialized request shape, not just successful return. For interactive rendering, assert the runtime event path, not only the leaf component.
    3. **Fix at the source.** Patch the lowest shared layer that matches upstream semantics. Avoid UI/provider band-aids when message normalization or session conversion is the source.
    4. **Harden both boundaries.** For cross-layer bugs, add at least one unit test at the pure conversion layer and one regression at the nearest caller boundary when practical (e.g. NormalizeMessages + OpenAI converter; session loop + print/interactive error surfacing).
    5. **Verify poisoned history.** When a bug involves bad persisted sessions, test with historical/poisoned messages: errored assistant turns, aborted assistant turns, empty text blocks, nil content, orphaned tool calls, and model/provider switches.
    6. **No accepted flake.** Do not document “rerun standalone” as success. Fix root causes instead of hiding failures with longer time budgets, retries, sleeps, env overrides, skips, or normalization. Use timeouts and retries only for faults outside our control, and surface those faults. An unavailable external dependency is a blocker, not passing evidence.
    7. **Report the red/green evidence.** Final summaries for fixes to core parity must state: failing symptom, new regression test names, upstream rule mirrored, and commands run. If a test was added after the fix, say it was not red-proven.
    8. **Production path required.** A new production API/method used to fix behavior must have at least one production call site or a test that drives the production path. Test-only call sites do not prove the product behavior changed.
    9. **Coverage quality over coverage quantity.** A PORT_MAP entry is not accepted as verified by a weak scenario. Boot-only, smoke-only, and deferred scenarios may stay in the report, but they do not count as behavioral verification. Registration-only scenarios count only for registry/catalog/auth-wiring paths, never for provider stream/payload conversion files.
    
    Use this acceptance matrix for shared provider, model, auth, and tool regressions. Explain inapplicable shapes from the upstream contract, not available credentials. Do not use `--list-models` as stream/payload proof.
    
    Cover both OpenAI-compatible API paths when the change reaches both:
    
    ```text
    ai/openai.go             # openai-completions
    ai/openai_responses.go   # openai-responses
    agent/transform.go       # upstream transform-messages parity before provider conversion
    ```
    
    Shared-path acceptance matrix for regressions:
    
    | required axis | minimum cases |
    |---|---|
    | provider shape | OAuth, API key, OpenAI-compatible with a custom base URL, and no default model, as the path allows |
    | API path | each API kind reached by the shared behavior, including `openai-completions` and `openai-responses` when both can apply |
    | mode | pure shared-function/converter unit test plus nearest caller (`print`, interactive, or RPC, as applicable) |
    | history | fresh plus poisoned persisted history when relevant |
    | assistant prior turn | error, aborted, empty text, tool-call/orphaned tool-call when relevant |
    | auth/network | hermetic success and hermetic failure when touching auth refresh/error display |
    
    ## Done criteria
    
    A surface is done only when:
    1. a canonical scenario exists under `test/parity/scenarios/<family>/`;
    2. `covers = [...]` names exact upstream paths from `docs/parity/PORT_MAP.md` **and** those paths are exercised by the asserted behavior;
    3. the scenario has behavioral verification quality (not boot-only, registration-only, smoke-only, or deferred) unless the surface is explicitly accepted as weak-only;
    4. the scenario was derived from upstream first;
    5. drift exposed by the scenario is fixed or recorded as a numbered divergence;
    6. legacy duplicate or weak coverage claims are removed;
    7. pure unit tests and caller-path tests exist for cross-layer bugs;
    8. `go test ./...` and the relevant parity gate pass.
    
    9. for an upstream leap, `make upstream-delta` proves every changed tracked source file has a non-pending disposition and durable evidence.
    
    10. `make interface-inventory-drift` proves the public package export graph and
        exact published declaration inventory are current, and
        `make interface-mapping-strict` proves every stable interface ID has complete
        mapping/reachability/conformance/evidence closure.
    
    11. `make async-contracts` proves every current upstream Promise/async source has an explicit ordering/cancellation/error contract and durable evidence.
    
    12. interaction and state-machine behavior found during probing is closed in
        `test/parity/behavior-contracts.toml` when declaration shape cannot express it.
        Contracts bind a hashed current-upstream source range and its complete
        keybinding/event inventory to Pig targets, a failing unit regression, and a
        behavioral parity scenario. Boundary transitions such as first/last
        wraparound, empty/singleton states, cancellation, ordering, and persistence
        are separate obligations; interface presence does not prove them.
    
    `test/parity/interfaces/behavior-inputs-v<version>.json` is the compiler-derived
    denominator for upstream input/action state machines and render surfaces. It
    accounts for keybinding declarations, platform defaults, consumers, raw input,
    branch/state effects, theme calls, glyph/string tokens, width/layout calls,
    numeric positioning literals, and referenced file constants. Its reviewed
    mapping assigns each surface to one owner family and records pending, partial,
    ported, deferred, divergence, or designed-out closure. `make
    behavior-contracts` rejects source/facet drift, declaration/consumer mismatch,
    missing mappings, invalid family ownership, and stale evidence. `make
    behavior-contracts-strict` rejects pending or partial surfaces. Generated
    facets identify review obligations; they never prove branch semantics, visual
    output, or promote a mapping.
    - Every known bug found in scope is fixed with red/mutation evidence or remains
      an explicit user-owned blocker. Passing unrelated gates does not close it.
    
    Go style:
    - Build, CI, release-candidate, security-analysis, and documented setup commands use the Go version in `.go-version`, the single source. Do not write the version number in this file.
    - `.go-version` is the one Go pin. Workflows, scripts, images, the devcontainer, and the `toolchain` lines of `go.mod` and `go.work` name that release and no other. A script reads `.go-version` instead of repeating the number. Moving the pin changes `.go-version` and its recorded copies in one change; `make compliance` and `TestGoVersionPolicy` fail on any other release. Dated measurement records keep the release they ran on.
    - Maintained modules declare Go 1.26 or 1.26.0 as their language floor unless a concrete language requirement is approved. Do not raise a `go` directive merely to match the build toolchain.
    - Use language features available at the module floor. Prefer applicable modern standard-library APIs such as `slices.Sort`, `slices.SortFunc`, `cmp.Compare`, `maps.Copy`, `slices.Sorted(maps.Keys(m))`, `strings.Cut`, `CutPrefix`, `SplitSeq`, `for i := range n`, `wg.Go`, `min`, and `max`.
    - `go fix -diff ./...` should be empty.
    - When observable behavior changes, update the doc comment in the same commit.
    
    Lint commands when touching lint/config/code:
    ```bash
    go tool golangci-lint config verify
    make lint-changed
    make lint
    ```
    
    `make port-lint` runs the porting anti-pattern analyzers (`automation/ci/portlint/README.md`) against a ratcheting `baseline.toml`. Fix a finding at the source, or mark one reviewed false positive with `//portlint:allow <check> <reason>`. Never add a baseline entry for new code.
    
    Use `make lint-changed` in the development loop. It runs every configured linter over whole changed Go files in changed packages relative to the merge base with `main`. Use `make lint` for the full-repository gate. `make check` always runs the full-repository gate.
    
    ## Commands
    
    Run `make generate` after changing exported Go API, CLI flags, settings, parity scenarios, or docs mirrors. Commit the result. Never hand-edit generated files; regenerate.
    
    Primary gates:
    ```bash
    make generate
    make build
    make vet
    make lint
    make test
    make lint-scenarios
    make parity-fast
    make parity
    make check
    make verify
    make coverage
    ```
    
    Other gates as needed:
    ```bash
    make port-lint
    make schedule-report
    make test-stress
    make parity-stress
    make parity-perf
    make test-timing   # blocking wall-clock/ratio gates (internal/testenv.TimingGate); PR CI only warns (PIG_PERF_TIMING_REPORT_ONLY=1)
    make parity-live
    make release-check
    go tool govulncheck ./...
    go tool deadcode ./...
    go fix -diff ./...
    ```
    
    Choose the test tier by the moment, as `docs/project/test-economy.md` defines: `go test ./<pkg> -run '<Tests>'` while iterating, `make test-changed`, `make lint-changed` and `go vet` on touched packages before a handoff commit, `make ci-drift ci-contracts` and the affected `make parity-family` runs before reporting ready, and the full suites (`make test`, `make check`, `make verify`, `make parity`, `go test ./...`) in CI, integration and release preview. Do not pass `-count=1` unless the test reads inputs through a child process. Use `-race` on edited packages, not the whole module. Bound `go test -fuzz` with `-parallel` and `-fuzztime`. On a shared host, build with `-trimpath`.
    
    `make test` requires and exercises the Go, Node, Python, and Rust toolchains; fixture build failures are fatal. `make check` combines deterministic gates with one strict Pig/Pi pair per hermetic scenario. `make verify` is the end-of-loop gate: it runs each scenario's declared durability and regenerates coverage. `make parity` runs declared scenario pairs in parallel with default concurrency groups and suppresses `runtime_ratio_max` because parallel CPU contention makes that metric unreliable. `make parity-perf` runs serially and enforces runtime ratios; use it for release gates or cron, not normal loops.
    
    ## Documentation language
    
    Write instructions, references, procedures, and operational documentation as simple, direct technical English. Write one instruction per sentence, in the present tense and the active voice. Keep one term for each thing. Do not apply controlled technical English mechanically to personal essays, origin stories, interviews, or release narratives. Preserve the author's vocabulary, humor, uncertainty, and cadence in those forms.
    
    Do not hard-wrap Markdown prose to a terminal width. Use structural newlines only where Markdown or the content requires them.
    
    Where Pig differs from Pi in a way a user can observe, state the difference at
    the point of use and cite its `D<N>`. An agent carrying Pi pretraining does the
    Pi thing wherever Pig's difference is undocumented.
    
    ## Public project writing
    
    Write about PiG as a faithful Go implementation of Pi. Name Pi as the reference
    implementation and link to its repository and documentation. Credit Michael
    Kinsy as PiG's creator and state that PiG was originally developed at Hewlett
    Packard Enterprise. Keep those facts separate from legal notices.
    
    Do not call PiG an official Pi project or imply endorsement by Pi's maintainers.
    Do not claim HPE sponsorship, Open Program Office approval, public release,
    community governance, certification, or trademark approval until the relevant
    decision is recorded and public. Do not use HPE logos, wordmarks, or exact
    corporate styling without recorded Brand approval. Keep Stock PiG branding
    product-neutral.
    
    Use direct technical prose. Avoid slogans, superlatives, invented motives,
    competitive framing, and promises of future collaboration. Prefer verifiable
    facts: the pinned Pi version, observed parity, named divergences, supported
    artifacts, and published evidence.
    
    ## Commit hygiene
    
    User-facing changes go in `changelog.d/<lane>-<sha>.md`, one fragment per commit range, as `### Added`, `### Changed`, `### Fixed` (and `Upgrading`, `Deprecated`, `Removed`, `Security`, `Known issues`) sections of bullets in `CHANGELOG.md`'s style. `<sha>` is a commit of the range reachable from `HEAD`; a rebase, squash or amend replaces SHAs, so rename the fragment after the commit that replaced it. Every `### Fixed` bullet ends with `<!-- fix: <sha> -->` naming the commit (or commits, comma-separated) that fixes it, also reachable from `HEAD` (or named by a `Fix-Commit: <full sha>` trailer of a squash commit `HEAD` reaches, see `.github/CONTRIBUTING.md`): the bug ledger reads `### Fixed`, and a bullet without a marker has its commits guessed from history, which once credited the 0.5.0 squash and its ~11,000 tests to five bullets. `### Upgrading` is what a user of the previous release must change; the fold appends it to the release's "Upgrading from" section. Do not edit `CHANGELOG.md`. `make changelog-check` validates the names, sections, markers and reachability against the full history of `HEAD` (a shallow clone fails) and `make changelog-unreleased` prints the assembly; fragments whose commits a squash replaced before these rules are listed in `automation/ci/changelogfrag/legacy.txt`, are exempt from the name and marker rules, and the list only shrinks. The integrator runs `make changelog-fold VERSION=x.y.z` to append the fragments to that release in `CHANGELOG.md` and delete them; it runs again at release for fragments written since. `make bug-ledger` refuses to attribute the tests of a commit that touches more than 500 files (a squash or merge) and names the entry; name the real fix commit in its marker.
    
    Every commit requires both a DCO `Signed-off-by` line and a signature GitHub marks **Verified**. Use `git commit --signoff -S`. Sign-off is not a cryptographic signature. See `.github/CONTRIBUTING.md` for SSH signing setup and unsigned-commit repair. If signing is unavailable, a maintainer lands the change in a maintainer-signed commit with a `Co-authored-by:` trailer.
    
    Before committing, run `git diff --stat HEAD` and `git status --short`, then stage only files intentionally changed. Never `git add .`, `git add -A`, or add a whole directory without inspecting contents. Testdata dirs accumulate temp files.
    
    Forbidden commands:
    - `git reset --hard`
    - `git checkout .`
    - `git clean -fd`
    - `git stash`
    - `git commit --no-verify`
    - `git push -f`
    - `tmux kill-server`
    - `go install ./cmd/pig`
    
    Only `tmux kill-session -t <name>` is allowed. Commit only files changed in this session.
    
    Do not dispatch subagents to read local files available in this worktree. Read them yourself.
    
    ## When unsure
    
    Read upstream source, read upstream tests, probe upstream behavior, then make pig match it. Ask the user instead of guessing.
    
    // CardWindowRow is one quota bucket on a provider card: the account-aggregate
    // of one window group.
    type CardWindowRow struct {
        // Label is the display label (limit label with tier folded in).
        Label string
        // WindowTag is the window label/id shown dim after the label when sibling
        // rows share a label.
        WindowTag string
        // Fraction is the mean used fraction across accounts (0..1, >1 = overage).
        Fraction *float64
        Status   ai.UsageStatus
        // ResetMs is the reset countdown of the worst account, ms from now.
        ResetMs *int64
        // UsedText is an absolute one-sided amount for limits without a fraction.
        UsedText string
    }
    
    // UsageUnavailableAccountView is a connected account whose usage lookup
    // produced no attributable report. The backend snapshot entry carries extra
    // fields (reason, accountKey, ambient) this view ignores.
    type UsageUnavailableAccountView struct {
        Provider string
        Label    string
    }
    
    // UsagePlanGroup is one provider subscription plan's reports.
    type UsagePlanGroup struct {
        Provider            string
        Name                string
        Reports             []ai.UsageReport
        UnavailableAccounts []UsageUnavailableAccountView
    }
    
    // GroupUsageReportsByPlan keeps quota totals and account columns separate for
    // each provider subscription plan.
    func GroupUsageReportsByPlan(reports []ai.UsageReport, unavailable []UsageUnavailableAccountView) []UsagePlanGroup {
        type planKey struct{ provider, plan string }
        order := make([]planKey, 0, len(reports)+len(unavailable))
        groups := map[planKey]*UsagePlanGroup{}
        getPlan := func(provider, plan string) *UsagePlanGroup {
            key := planKey{provider: provider, plan: plan}
            if group, ok := groups[key]; ok {
                return group
            }
            group := &UsagePlanGroup{Provider: provider}
    
    // ProviderCard is the compact per-provider summary backing one card in the
    // subscriptions grid.
    type ProviderCard struct {
        Provider string
        Name     string
        // Accounts counts represented accounts, including unavailable usage lookups.
        Accounts            int
        UnavailableAccounts []string
        Windows             []CardWindowRow
        Unlimited           bool
        Notes               []string
        Idle                bool
        ResetCredits        *ProviderCardResetCredits
        DaybreakAccounts    []string
    }
    
    // CardMaxWindows is the number of rows a provider card may show before
    // "+N more".
    const CardMaxWindows = 4
    
    // idleFraction is the fraction below which a window counts as untouched.
    const idleFraction = 0.005
    
    // compactWindowTag renders a compact duration tag for a window (`7d`, `1d`,
    // `5h`, `mo`).
    func compactWindowTag(window ai.UsageWindow) string {
        if window.DurationMs != nil && *window.DurationMs > 0 {
            hours := float64(*window.DurationMs) / 3600000
            switch {
            case hours >= 28*24:
                return "mo"
            case hours >= 24:
                return strconv.Itoa(int(math.Round(hours/24))) + "d"
            }
            return strconv.Itoa(int(math.Round(hours))) + "h"
        }
        id := strings.ToLower(window.ID)
        if len([]rune(id)) <= 3 {
            return id
        }
        runes := []rune(id)
        return string(runes[:1])
    }
    
    // limitFractionValue is the bucket-ordering fraction, -1 when unreported.
    func limitFractionValue(limit ai.UsageLimit) float64 {
        fraction, ok := ai.UsageUsedFraction(limit)
        if !ok {
            return -1
        }
        return fraction
    }
    
    // BuildProviderCards collapses usage reports into one compact card per provider
    // and plan: limits grouped by quota bucket, each bucket showing the mean used
    // fraction across accounts with the most-used account's reset countdown. Rows
    // keep the provider's declared window order; cards sort by their fullest window
    // so what is burning is on top-left; fully idle providers collapse into a tick.
    func BuildProviderCards(reports []ai.UsageReport, nowMs int64, unavailable []UsageUnavailableAccountView) []ProviderCard {
        displayReports := CollapseSharedUsageReports(reports)
        grouped := GroupUsageReportsByPlan(displayReports, unavailable)
        cards := make([]ProviderCard, 0, len(grouped))
        for _, group := range grouped {
            missingLabels := make([]string, 0, len(group.UnavailableAccounts))
            for _, account := range group.UnavailableAccounts {
                missingLabels = append(missingLabels, account.Label)
            }
    
            type bucket struct {
                label  string
                limits []ai.UsageLimit
            }
            buckets := make([]*bucket, 0, len(group.Reports))
            bucketByKey := map[string]*bucket{}
            for _, report := range group.Reports {
                for _, limit := range report.Limits {
                    label := FormatLimitTitle(limit)
                    key := limit.Scope.SharedGroup
                    if key == "" {
                        windowID := limit.Scope.WindowID
                        if limit.Window != nil && limit.Window.ID != "" {
                            windowID = limit.Window.ID
                        }
                        if windowID == "" {
                            windowID = "default"
                        }
                        key = label + "|" + windowID + "|" + limit.ID
                    }
                    entry, ok := bucketByKey[key]
                    if !ok {
                        entry = &bucket{label: label}
                        bucketByKey[key] = entry
                        buckets = append(buckets, entry)
                    }
                    entry.limits = append(entry.limits, limit)
                }
            }
    
            declared := make([]CardWindowRow, 0, len(buckets))
            for _, entry := range buckets {
                var sum float64
                count := 0
                statuses := make([]ai.UsageStatus, 0, len(entry.limits))
                worst := entry.limits[0]
                worstFraction := limitFractionValue(worst)
                for _, limit := range entry.limits {
                    if fraction, ok := ai.UsageUsedFraction(limit); ok {
                        sum += fraction
                        count++
                    }
                    if value := limitFractionValue(limit); value > worstFraction {
                        worst, worstFraction = limit, value
                    }
                    statuses = append(statuses, limit.Status)
                }
                var fraction *float64
                if count > 0 {
                    mean := sum / float64(count)
                    fraction = &mean
                }
                var resetMs *int64
                tag := ""
                if worst.Window != nil {
                    tag = compactWindowTag(*worst.Window)
                    if worst.Window.ResetsAt != nil && *worst.Window.ResetsAt > nowMs {
                        delta := *worst.Window.ResetsAt - nowMs
                        resetMs = &delta
                    }
                }
                row := CardWindowRow{
                    Label:     entry.label,
                    WindowTag: tag,
                    Fraction:  fraction,
                    Status:    AggregateStatus(statuses),
                    ResetMs:   resetMs,
                }
                if fraction == nil {
                    row.UsedText = FormatAbsoluteOnlyAmount(entry.limits)
                }
                declared = append(declared, row)
            }
    
            // Cards render at most CardMaxWindows rows; keep the fullest visible
            // (still in declared order) so an exhausted bucket never hides.
            visible := map[int]bool{}
            indices := make([]int, len(declared))
            for i := range declared {
                indices[i] = i
            }
            sort.SliceStable(indices, func(a, b int) bool {
                return windowSortFraction(declared[indices[a]]) > windowSortFraction(declared[indices[b]])
            })
            for _, index := range indices {
                if len(visible) >= CardMaxWindows {
                    break
                }
                visible[index] = true
            }
            windows := make([]CardWindowRow, 0, len(declared))
            for index, row := range declared {
                if visible[index] {
                    windows = append(windows, row)
                }
            }
            for index, row := range declared {
    
    const (
        UsageStatusOK        UsageStatus = "ok"
        UsageStatusWarning   UsageStatus = "warning"
        UsageStatusExhausted UsageStatus = "exhausted"
        UsageStatusUnknown   UsageStatus = "unknown"
    )
    
    // UsageWindow is the time window a limit applies to.
    type UsageWindow struct {
        // ID is a stable identifier such as "5h", "7d" or "monthly".
        ID string `json:"id"`
        // Label is the human label ("5 Hour", "7 Day").
        Label string `json:"label"`
        // DurationMs is the window length in milliseconds when known.
        DurationMs *int64 `json:"durationMs,omitempty"`
        // ResetsAt is the absolute reset time in epoch milliseconds.
        ResetsAt *int64 `json:"resetsAt,omitempty"`
        // ResetLabel is the verb shown before the countdown ("tick", "regen");
        // empty means "resets".
        ResetLabel string `json:"resetLabel,omitempty"`
    }
    
    // UsageAmount is the quantitative data of one limit.
    type UsageAmount struct {
        Used              *float64  `json:"used,omitempty"`
        Limit             *float64  `json:"limit,omitempty"`
        Remaining         *float64  `json:"remaining,omitempty"`
        UsedFraction      *float64  `json:"usedFraction,omitempty"`
        RemainingFraction *float64  `json:"remainingFraction,omitempty"`
        Unit              UsageUnit `json:"unit"`
    }
    
    // UsageScope describes what a limit applies to.
    type UsageScope struct {
        Provider    string `json:"provider"`
        AccountID   string `json:"accountId,omitempty"`
        ProjectID   string `json:"projectId,omitempty"`
        OrgID       string `json:"orgId,omitempty"`
        ModelID     string `json:"modelId,omitempty"`
        Tier        string `json:"tier,omitempty"`
        WindowID    string `json:"windowId,omitempty"`
        Shared      bool   `json:"shared,omitempty"`
        SharedGroup string `json:"sharedGroup,omitempty"`
    }
    
    // UsageLimit is one normalized quota window or bucket.
    type UsageLimit struct {
        ID     string       `json:"id"`
        Label  string       `json:"label"`
        Scope  UsageScope   `json:"scope"`
        Window *UsageWindow `json:"window,omitempty"`
        Amount UsageAmount  `json:"amount"`
        Status UsageStatus  `json:"status,omitempty"`
        Notes  []string     `json:"notes,omitempty"`
    }
    
    // UsageResetCreditDetail is one saved banked reset a provider issued.
    909:func BuildProviderCards(reports []ai.UsageReport, nowMs int64, unavailable []UsageUnavailableAccountView) []ProviderCard {
    1166:func windowSortFraction(row CardWindowRow) float64 {
    539:func (m *InteractiveMode) applyBaseRendererWiring() {
    551:func (m *InteractiveMode) rewireRenderer() {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go:1677:    m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_models.go:126:        m.opts.Model = newModel
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_models.go:279:    m.opts.Model = result.Model
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_status.go:14:    if m.editor == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/autocomplete_wrappers.go:364:    if u.m == nil || u.m.editor == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_custom.go:76:    if u.m.tuiInst == nil || u.m.editorContainer == nil || u.m.editor == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_bug.go:46:    if m.opts.Model == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_commands.go:307:            if m.opts.Model == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go:843:    if m.editor == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go:847:        m.editor = m.customEditor.Editor
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_queue_hint.go:66:    if !termProgramIs(appleTerminalProgram) || m.editor == nil || !m.builtInEditor() || m.editor.Text() != "" {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/native_subagents.go:311:    if m.editor == nil || m.editor.Text() != "" {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_auth.go:784:    if m.editor == nil || m.tuiInst == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_extension_session_actions.go:87:    m.opts.Model = model
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_post_login.go:94:                if deferSelection && m.modelSelectionGeneration == generation && m.currentSession() == session && m.opts.SessionHandle == handle && m.opts.Model == previousModel {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_post_login.go:176:        return m.modelSelectionGeneration == generation && m.currentSession() == session && m.opts.SessionHandle == handle && m.opts.Model == expected
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_post_login.go:206:                    m.opts.Model = model
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_rebind.go:145:    m.opts.Model = m.agent.Model()
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_editor.go:51:    if m.editor == nil {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_editor_snapshot.go:5:    if m.editor == nil || m.editorSnapshotOwner == m.editor {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/ext_ui_editor_snapshot.go:14:        if m.editor == editor {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_share.go:22:    if m.chatContainer == nil || m.editorContainer == nil || m.editor == nil || m.tuiInst == nil {
    ls: cannot access 'vendor/github.com/MichaelKinsy/PiG/docs/project/CONTEXT.md': No such file or directory
    ls: cannot access 'vendor/github.com/MichaelKinsy/PiG/.upstream/current/packages/tui/src/components/editor.ts': No such file or directory
    ✗ (2) • 90ms

## Assistant

The right side is currently empty; the row only adds “Working” on the left and a centered hidden-lines indicator when needed. There’s already a quota service behind `/usage` that reads all configured accounts. I’ll reuse it, show totals for the selected model’s provider, and keep separate quota windows such as 5-hour and weekly limits.

## Activity

    $ sed -n '775,810p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; sed -n '235,265p' vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go; sed -n '118,132p' vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go; sed -n '710,730p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '850,875p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '1,85p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go; sed -n '1000,1040p' vendor/github.com/MichaelKinsy/PiG/tui/editor.go; sed -n '1,90p' vendor/github.com/MichaelKinsy/PiG/tui/editor_status_test.go; rg -n 'BeforeRender|beforeRender|Set.*Render|renderHook' vendor/github.com/MichaelKinsy/PiG/tui/*.go | head -40; sed -n '440,560p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md
        m.frameTimer.Reset(max(time.Until(due), 0))
    }
    
    // runScheduledRender runs the renderer's due throttled frame, if any, on the
    // main loop. A frame wakes a parked spinner ticker, whose next tick re-checks
    // whether the session still animates (D172).
    func (m *InteractiveMode) runScheduledRender() {
        m.tuiInst.RunDueRender()
        m.wakeSpinner()
    }
    
    // runOnMain hands fn to the main input loop and blocks until the loop accepts
    // it (backpressure) or ctx is cancelled (shutdown/abort); the owner loop itself never blocks (see postFromOwner). Unlike postUITask it
    // never drops fn, so it is the tool for state the loop must not miss: streamed
    // bash output, login results, turn-end state. It does not wait for fn to run;
    // the loop executes queued tasks FIFO, so a caller that posts in order sees its
    // mutations applied in order. Callers must not hold a lock the loop also takes.
    // Mirrors upstream pi's single-event-loop model, where a worker's result is
    // delivered to the loop rather than mutating UI state from the worker
    // (Bubble Tea's p.Send has the same blocking-with-cancel semantics).
    func (m *InteractiveMode) runOnMain(ctx context.Context, fn func()) {
        _ = m.postToMain(ctx, fn)
    }
    
    // errOwnerLoopUnavailable reports a task the owner loop could not accept.
    var errOwnerLoopUnavailable = errors.New("interactive mode is not accepting work")
    
    // postToMain is runOnMain for callers that must report a task the loop never
    // accepted, such as an extension's sendUserMessage: it never drops fn, and
    // returns an error only when ctx ends first. The owner loop posting to itself
    // never blocks and never runs fn inline: see postFromOwner.
    func (m *InteractiveMode) postToMain(ctx context.Context, fn func()) error {
        if fn == nil {
            return nil
        }
        // The owner check reads the goroutine id, which costs a stack walk, so a post that the queue accepts at once skips it. While owner overflow is pending the owner's own posts must queue behind it.
    func NewUsageHTTPError(message string, status int) *UsageHTTPError {
        return &UsageHTTPError{Status: status, Message: message}
    }
    
    // UsageUnavailableAccount is one account whose quota could not be read.
    type UsageUnavailableAccount struct {
        Provider   string  `json:"provider"`
        Label      string  `json:"label"`
        AccountKey *string `json:"accountKey,omitempty"`
        // Ambient marks an environment/runtime-key probe rather than a stored account.
        Ambient bool   `json:"ambient,omitempty"`
        Reason  string `json:"reason"`
    }
    
    // UsageSnapshot is one quota pass over every configured provider account.
    type UsageSnapshot struct {
        // Reports holds the successful reports, one per account that answered.
        Reports []UsageReport `json:"reports"`
        // UnavailableAccounts holds accounts that answered with nothing, instead of
        // being silently omitted.
        UnavailableAccounts []UsageUnavailableAccount `json:"unavailableAccounts"`
    }
    
    // UsageActiveAccount is the session-routing identity of the account a provider
    // currently uses.
    type UsageActiveAccount struct {
        AccountKey *string
        Email      string
        AccountID  string
        OrgID      string
        OrgName    string
    
    // UsageReport is one provider account's quota snapshot.
    type UsageReport struct {
        Provider     string             `json:"provider"`
        FetchedAt    int64              `json:"fetchedAt"`
        Limits       []UsageLimit       `json:"limits"`
        ResetCredits *UsageResetCredits `json:"resetCredits,omitempty"`
        Notes        []string           `json:"notes,omitempty"`
        Metadata     map[string]any     `json:"metadata,omitempty"`
        Raw          any                `json:"raw,omitempty"`
    }
    
    // UsageLogger is the optional logger handed to usage fetchers.
    type UsageLogger interface {
        Debug(message string, meta map[string]any)
    
    // CardWindowRow is one quota bucket on a provider card: the account-aggregate
    // of one window group.
    type CardWindowRow struct {
        // Label is the display label (limit label with tier folded in).
        Label string
        // WindowTag is the window label/id shown dim after the label when sibling
        // rows share a label.
        WindowTag string
        // Fraction is the mean used fraction across accounts (0..1, >1 = overage).
        Fraction *float64
        Status   ai.UsageStatus
        // ResetMs is the reset countdown of the worst account, ms from now.
        ResetMs *int64
        // UsedText is an absolute one-sided amount for limits without a fraction.
        UsedText string
    }
    
    // UsageUnavailableAccountView is a connected account whose usage lookup
    // produced no attributable report. The backend snapshot entry carries extra
    // fields (reason, accountKey, ambient) this view ignores.
    
    // ProviderCard is the compact per-provider summary backing one card in the
    // subscriptions grid.
    type ProviderCard struct {
        Provider string
        Name     string
        // Accounts counts represented accounts, including unavailable usage lookups.
        Accounts            int
        UnavailableAccounts []string
        Windows             []CardWindowRow
        Unlimited           bool
        Notes               []string
        Idle                bool
        ResetCredits        *ProviderCardResetCredits
        DaybreakAccounts    []string
    }
    
    // CardMaxWindows is the number of rows a provider card may show before
    // "+N more".
    const CardMaxWindows = 4
    
    // idleFraction is the fraction below which a window counts as untouched.
    const idleFraction = 0.005
    
    // compactWindowTag renders a compact duration tag for a window (`7d`, `1d`,
    // `5h`, `mo`).
    package codingagent
    
    // Ports packages/coding-agent/src/core/provider-usage.ts: provider quota
    // fetching across every stored account plus ambient auth, with the upstream
    // per-(provider, endpoint, account) cache, failure cool-down, in-flight
    // coalescing, waiter cancellation and credential-only refresh.
    
    import (
        "context"
        "errors"
        "math/rand/v2"
        "net/http"
        "sort"
        "sync"
        "sync/atomic"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
    )
    
    // Upstream service constants (provider-usage.ts).
    const (
        // UsageRequestTimeout is the upstream default request budget.
        UsageRequestTimeout = 10 * time.Second
        // UsageReportTTL is the upstream report TTL before refresh.
        UsageReportTTL = 5 * time.Minute
        // usageFailureCoolDown is the per-credential cool-down after a failure.
        usageFailureCoolDown = 10 * time.Second
        // UsageLastGoodRetention is how long a failed probe may serve its last good
        // report.
        UsageLastGoodRetention = 24 * time.Hour
        // usageOAuthRefreshSkew refreshes a token this close to expiry.
        usageOAuthRefreshSkew = 5 * time.Minute
        // NoConfiguredCredential is the reason marker for a provider with no stored,
        // runtime or environment credential.
        NoConfiguredCredential = "no configured credential"
    )
    
    // UsageServiceOptions configures a UsageService.
    type UsageServiceOptions struct {
        // Credentials is the account-aware credential store. A store without
        // ListAccounts falls back to the single-credential List/Read surface.
        Credentials ai.CredentialStore
        // Accounts, when set, is the multi-account view of the same store.
        Accounts ai.AccountCredentialStore
        // GetAuth resolves request auth, refreshing one account's OAuth token when
        // it is near expiry. Account scope travels in the overrides so an explicit
        // identity-less account is distinguishable from "the selected account". It
        // must never change credential lifecycle.
        GetAuth func(ctx context.Context, providerID string, overrides ai.AuthResolutionOverrides) (*ai.AuthResult, error)
        // BaseURLFor reports a configured provider base URL.
        BaseURLFor func(providerID string) string
        // HTTPClient is the client every probe uses. Nil means http.DefaultClient.
        HTTPClient *http.Client
        // Logger receives probe warnings.
        Logger ai.UsageLogger
        // Providers overrides the built-in registry (tests, extension providers).
        Providers []*ai.UsageProvider
        // Now reports the current time; nil means time.Now.
        Now func() time.Time
        // Random returns a float in [0,1) for cache jitter; nil means math/rand.
        Random func() float64
    }
    
    // UsageService is the per-runtime quota service behind a usage dashboard.
    type UsageService struct {
        options   UsageServiceOptions
        providers []*ai.UsageProvider
    
        mu       sync.Mutex
        cache    map[string]*usageCacheEntry
        inFlight map[string]*usageSharedProbe
    }
    
    // usageCacheEntry is one cached probe result.
    type usageCacheEntry struct {
        report      *ai.UsageReport
        expiresAt   time.Time
        reason      string
        unsupported bool
        lastGood    *usageLastGood
        // cacheVersion records the provider's cache shape when the entry was built.
        cacheVersion int
    }
    
        if e.scrollOffset > maxOffset {
            e.scrollOffset = maxOffset
        }
        if e.scrollOffset < 0 {
            e.scrollOffset = 0
        }
    
        end := min(e.scrollOffset+maxVis, len(visual))
        visible := visual[e.scrollOffset:end]
        e.renderedVisibleLineCount = len(visible)
        // Mirrors upstream editor.ts render: every content row is padded to the
        // content width, padding or not. The trailing cells matter beyond looks:
        // CompositeTuiLine keeps SGR codes only when a later cell follows them, so
        // an unpadded row ending in the cursor's fake cursor loses its reset
        // under an overlay and paints the gap before the overlay inverse. A cursor
        // appended past the content width sits in the right padding, which then
        // gives up one cell.
        for i, line := range visible {
            visible[i] = e.paddedRow(i, line, paddingX, contentWidth)
        }
    
        // Render top border (with "↑ N more" indicator if scrolled down).
        // Mirrors editor.ts:451-465.
        top := e.renderTopBorder(width, e.scrollOffset)
        top = e.renderStatusBorder(width, e.scrollOffset, top)
        out := make([]string, 0, len(visible)+2)
        out = append(out, top)
        out = append(out, visible...)
    
        // Bottom border (with "↓ N more" indicator if more content below).
        linesBelow := len(visual) - end
        out = append(out, e.renderBottomBorder(width, linesBelow))
    
        // Render slash-autocomplete popup as additional rows
        // below the editor frame. Mirrors upstream editor.ts:521-528 which
        // appends `autocompleteList.render(contentWidth)` lines to its
        // own output.
        e.renderedAutocompleteHeight = 0
        if len(e.autocompleteItems) > 0 {
            autocomplete := e.renderAutocomplete(contentWidth)
            e.renderedAutocompleteHeight = len(autocomplete)
    sed: can't read vendor/github.com/MichaelKinsy/PiG/tui/editor_status_test.go: No such file or directory
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:393:// SetMaxLines caps how many lines Render() returns. When n>0, only the
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:787:    // loop instead of a timer goroutine (SetRenderDeadline): a frame records
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1620:// SetRenderDispatcher installs a hook that runs throttled scheduled renders
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1626:// SetRenderDeadline owner. Thread-safe.
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1627:func (t *tuiBase) SetRenderDispatcher(dispatch func(render func())) {
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1634:// SetRenderDeadline makes the loop that owns component-tree mutation keep the
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1641:// SetRenderDispatcher hook; nil restores the timer goroutine. Thread-safe.
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1642:func (t *tuiBase) SetRenderDeadline(wake func(due time.Time)) {
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1672:// RunDueRender runs the frame whose SetRenderDeadline deadline has passed, on
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1691:// one, to run after d: on the SetRenderDeadline owner when there is one,
    vendor/github.com/MichaelKinsy/PiG/tui/tui.go:1705:// ownerFrame is a frame armed on the SetRenderDeadline owner. Stop runs under mu,
    vendor/github.com/MichaelKinsy/PiG/tui/login_dialog.go:118:func (d *LoginDialogComponent) SetCopyToClipboard(copyText func(text string) error, requestRender func()) {
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:55:    SetRendererFallback(func(width int) []string)
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:67:func (c *ToolExecutionComponent) SetDefinition(definition *ToolDefinitionRenderers, args json.RawMessage) {
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:79:// Deprecated: use SetDefinition(&ToolDefinitionRenderers{}, args).
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:81:    c.SetDefinition(&ToolDefinitionRenderers{}, args)
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:168:        fallback.SetRendererFallback(c.callFallback().Render)
    vendor/github.com/MichaelKinsy/PiG/tui/tool_execution_definition.go:179:                fallback.SetRendererFallback(func(width int) []string {
    vendor/github.com/MichaelKinsy/PiG/tui/renderer.go:94:    // SetRenderDispatcher marshals timer-scheduled renders onto the driver loop.
    vendor/github.com/MichaelKinsy/PiG/tui/renderer.go:95:    SetRenderDispatcher(dispatch func(render func()))
    vendor/github.com/MichaelKinsy/PiG/tui/renderer.go:96:    // SetRenderDeadline makes the driver loop keep the throttled-frame deadline; RunDueRender runs the frame once it passed.
    vendor/github.com/MichaelKinsy/PiG/tui/renderer.go:97:    SetRenderDeadline(wake func(due time.Time))
    vendor/github.com/MichaelKinsy/PiG/tui/settings_list.go:235:func (s *SettingsList) Render(width int) []string {
    output, or promote a mapping.
    
    Parallel porter campaigns may run only read-only `inventory` and `verify` jobs
    against disposable snapshots via `make porter-campaign`. Reports
    must be tied to one commit, upstream version, and owner family, and are proposals
    for the coordinator to re-probe. Production/evidence edits (`family` and
    `tighten`) remain serialized in the canonical workspace.
    
    Choose the option that makes the next upstream sync smaller. Do not embed pig-specific behavior in mirrored code unless it is a documented divergence.
    
    ## Parity scenario rules
    
    Probe upstream first. Record enough evidence in comments for the next maintainer to see what was measured. Prefer strong, byte-faithful comparators. Use substring checks only when no stronger stable assertion exists.
    
    Scenario quality rules:
    - `covers` must name behavior exercised by the asserted crop/output, not merely code touched by startup, registration, or import/linking.
    - Boot-only scenarios may exist but cannot claim deep file coverage and do not count as behavioral verification.
    - Registration-only scenarios (for example `--list-models`) prove catalog visibility only. They must be tagged `registration-only`; they may cover registry/catalog/auth-wiring paths but must not claim provider stream/payload conversion files.
    - Smoke-only scenarios must be tagged `smoke-only`; they are useful tripwires but not acceptance evidence.
    - If output differs, fix pig or cite a numbered divergence. Comments alone are not enough.
    - Do not weaken a comparator to keep a pig bug green.
    - Scenario comments must explain any `normalize_replace`, `lint-known-gap`, `skip`, `serial`, group override, or weak-quality tag.
    - Keep `covers` narrow. Broad claims inflate the dashboard and hide untested behavior.
    - Re-probe affected cross-family scenarios when a fix changes shared behavior.
    
    Isolation rules for parallel parity:
    - Agent/home dirs are ephemeral copies. Never write into checked-in fixture roots.
    - Use `{{TEMP}}` for writable paths; do not hardcode `/tmp` paths.
    - tmux session names must be unique and cleanup may kill only `parity-*` sessions.
    - `PIG_CODING_AGENT_DIR` and `PI_CODING_AGENT_DIR` win over `PIG_HOME`, and every child process inherits them, including a tmux session started from your shell. A manual probe or test that runs pig must set or clear all of `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME`, `PI_HOME` and `HOME`, or `/logout` and login rewrite the real credentials (the 2026-10-06 lane `auth.json` wipe). A package whose `TestMain` calls `testenv.ScopeTempDir` or `testenv.RunScoped` replaces the agent-directory variables. Other packages inherit them, so a plain `go test` run from an agent shell sets `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME` and `PI_HOME` to scratch directories first. `make test` runs each group under seeded directories and fails a group that adds, removes or changes any file or directory in them (`automation/ci/agent-dir-guard.sh`).
    - `pre_clear_paths` is deprecated; prefer fresh temp dirs.
    - Use the default scheduler groups unless a justified `group = "..."` or `serial` tag is required.
    - Deferred scenarios stay visible in coverage; do not use skip/defer to hide known drift.
    
    ## Divergences
    
    A prior divergence record is investigation evidence, not permanent approval.
    Re-probe it against the exact current upstream and current production path at
    every upstream leap and foundation/release review. Delete or reclassify entries
    that are language mechanics, additive behavior, test limitations,
    unreachable/equal behavior, stale scaffolds, or caller-free code. IDs are global
    across `docs/parity/DIVERGENCES.md` and additive ledgers; duplicate IDs fail even when they
    live in different files.
    
    A divergence is allowed only when it is user-visible or interop-relevant and cannot or should not be made faithful now. Every active divergence must have:
    - `D<N>` id in `docs/parity/DIVERGENCES.md`;
    - `SCRUTINIZED:approved`;
    - remove-when condition;
    - `// pig divergence (D<N>): ...` at each call site;
    - parity coverage or an explicit parity allowance.
    
    Each record is one file, `docs/parity/divergences/D<N>.md`: an active record is a `## D<N> <title>` section, a retired record is one `- D<N>: ...` line. Edit that file only. `make divergences` generates `docs/parity/DIVERGENCES.md` from the files, and a lane does not commit the generated ledger; the merger regenerates it (`TestDivergenceLedgerEqualsItsRecordFiles` fails when they differ).
    
    `make divergence-guard` (part of `make check`) catches divergences nobody recorded. It flags, in the agent, ai, coding, internal/codingagent, tui and cmd/pig packages: duration and cap literals with no upstream counterpart (`magic-literal`), event sends that race a context-done case (`cancel-drop-send`), non-blocking sends that drop their value (`default-drop-send`), decode errors skipped inside loops (`stream-parse-continue`), unknown or missing stop reasons mapped to success (`stop-reason-success`), SSE fields parsed outside `ai/sse.go` (`hand-rolled-sse`), `recover()` that discards the panic (`bare-recover`), discarded persistence errors (`discarded-io-error`), hooks that proceed when their handler fails (`hook-fail-open`), errors classified by status-code substrings (`error-status-substring`), and agent `Send`/`Continue` outside `coding/session*.go` (`agent-run-outside-session`). `bare-recover` scans every non-test Go file in the repository, not only those packages. A hit passes only with a `// pig divergence (D<N>): ...` marker on its line or the line above, or, when Pi really does the same, `// upstream: <file>:<symbol>` there; the file must exist in `.upstream/current` or be a SHA-256-verified dependency snapshot whose exact version matches a dependency pin in that mirror (`test/parity/dependency-sources.json`; see `docs/parity/dependency-source-proof.md`), the symbol must appear in it, and for a literal the value must appear too. A literal whose value appears in an upstream file that `docs/parity/PORT_MAP.md` maps to the Go file also passes. Every current hit is listed in `automation/ci/divguard/baseline.toml` with its finding and owning slice. The gate compares the scan with that file in both directions: a new hit fails, and so does an entry whose hit is gone, so a fix deletes its entries in the same change. It does not compare the file with its previous version, so keeping it shrink-only is a review rule: never add an entry for new code. A green guard shows these patterns are absent or accounted for, not that the code matches Pi; allow markers prove only that the cited file and symbol exist (and, for literals, that the value appears there).
    
    Do not record ordinary TS-to-Go mechanics, test harness differences, or an
    SDK's implementation of a capability every SDK has (naming convention, `Result`
    vs `(T, error)` vs raised exception, ownership and borrowing, host call versus
    replicated state).
    
    That exemption covers implementation only. An SDK that lacks a capability
    another SDK has, or that answers the same call differently, is drift regardless
    of how faithfully its interface is spelled. Record it and close it.
    
    ## Native Windows development
    
    Windows is a first-class target. Windows-only failures are fixed on a Windows machine, not by repeated CI round trips.
    
    Set up the toolchain at the pinned versions:
    - Go from `.go-version`, Node from `.node-version`, Python 3.12, and the Rust toolchain that `ci.yml` names in `RUST_VERSION`.
    - Git for Windows, and run the `make` targets and `automation/` scripts from Git Bash.
    - `fd` and `rg` from `automation/ci/install-windows-tools.ps1`, which verifies each archive against `automation/ci/windows-tools.txt`.
    - The Pi oracle with `automation/ci/install-npm-toolchain.sh`, then `python automation/pin/pin.py install-oracle`, then `python automation/ci/npm-locked.py internal/typebox/testdata/npm` and `python automation/ci/npm-locked.py ai/testdata/sdk`, as the Windows CI job does.
    - `./automation/gen/mirror-upstream.sh` from Git Bash with `MSYS=winsymlinks:nativestrict`.
    
    Reproduce a Windows CI job locally before you change code. Run the same command the job runs (see the `windows` job in `.github/workflows/ci.yml`). Use `go test -v` and a `-timeout` below the job limit, so a stall prints a stack.
    
    Set `PIG_HOME` and `PIG_CODING_AGENT_DIR` to a directory with a space in its path, as CI does. Many Windows bugs are path bugs: separators, drive letters, 8.3 short names, and `D:\work\project` against `/work/project`. Compare with what Pi does on Windows before you change a path rule.
    
    Work on a branch named `team/<name>/<topic>`. Push it to the staging remote, not to the public repository. The integrator stacks verified branches into the release branch. Each commit follows Commit hygiene below: `git commit --signoff -S`.
    
    ## Code and lint policy
    
    Apply [Faithful, general implementations](#faithful-general-implementations) to code review and provider/model literal guard exceptions; each exception requires a Pi source citation.
    
    Fix valid findings at the source. Preserve upstream behavior over stylistic lint suggestions. Use the narrowest suppression for real false positives and explain why a source fix would be less faithful or less correct. `nolintlint` is enabled; stale or unexplained pragmas fail. Security suppressions must name the CLI threat-model reason.
    
    Code and interface style:
    - Production comments state only current behavior and non-obvious protocol, concurrency, security, compatibility, or behavioral invariants. Delete comments that restate code, narrate an implementation pass, preserve project history, or point to `docs/parity/PORT_MAP.md`, a phase, a future row, or another status ledger instead of stating the current contract. Keep migration and parity rationale in the owning ledger or specification.
    - Do not hard-wrap Markdown prose or source comments to fit a terminal pane. Editors can display-wrap long lines. Preserve structural line breaks in code blocks, tables, quoted material, poetry, generated files, and formats with an enforced line-length contract.
    - Roadmap phases, workstream labels, future-work promises, internal issue ownership, and placeholder implementation plans fail the source-hygiene gate; keep them in tracked specifications instead.
    - Historical comments, phase labels, placeholders, compatibility paths, manual
      parity lists, test-only public implementations, and prior approvals have no
      grandfather status. Revalidate against current upstream and production use;
      update the current rationale, bind the full production path, reclassify, or
      delete.
    - Production-grade Go means concrete validated data, minimal consumer-owned
      interfaces, deterministic order, bounded resources, explicit lifetime/error
      ownership, and one mechanism per concept. Do not mirror TypeScript internals
      or add abstraction merely to look structurally similar; preserve observable
      Pi semantics and portable wire/session/source behavior. Quality archaeology is
      not a style sweep: change debt only with current contract/caller evidence and
      leave accurate provenance and healthy out-of-family code alone.
    - `iface` rejects unused/duplicate interface declarations, opaque interface
      returns with one concrete implementation, and unexported contracts leaked
      through exported APIs. Do not suppress it
      by default; delete an unneeded interface or keep the one consumer-owned
      contract.
    - Do not enable or apply `ireturn` or `interfacebloat` mechanically. Extension
      protocol interfaces, tagged unions, providers, and factories have legitimate
      interface returns, and an upstream public protocol may be broad. Review those
      surfaces against `docs/typescript-to-go-porting.md` and measure hot paths.
    - Performance claims require a representative benchmark with allocations and a
      CPU/allocation profile. Interface dispatch is not a defect without evidence;
      prioritize the measured cost center and preserve upstream behavior.
    
    - Exact-count assertions must derive their expectation from an independent denominator or identify an explicitly reviewed generated snapshot and use its owning regeneration command. A newly observed number proves that output changed, not that the new number is correct.
    - A runtime mutation must compile before its behavioral failure counts as evidence. A compiler failure counts only when compilation is the contract under test.
    - For extension, TUI, model/provider, Session/RPC, Package/Resource, and startup/reload critical paths, capture a representative benchmark/profile and resource-lifetime disposition with the owning behavior family. No blocking extension IPC or unbounded work may run on the TUI input/render loop.
    - Ordinary editor and transient popup changes must not clear or replay transcript history. No TUI input handler may synchronously copy, decode, or scan unbounded Session history.
    - Every known bug found in scope is fixed with red/mutation evidence or remains
    ✓ • 182ms

## Activity

    $ rg -n 'generation|Generation' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go | head; sed -n '410,435p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; sed -n '830,860p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go; sed -n '192,210p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go; rg -n 'func.*CollapseSharedUsageReports|func.*UsageUsedFraction|Provider.*Name' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go | head; sed -n '1,70p' vendor/github.com/MichaelKinsy/PiG/tui/editor.go; sed -n '1018,1035p' vendor/github.com/MichaelKinsy/PiG/tui/editor.go; rg -n 'Slice|Cut|Truncate' vendor/github.com/MichaelKinsy/PiG/tui/widthx/*.go | head; rg -n 'requestRender\(' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_layout.go
        lastStatusMessage string
    
        // Countdown goroutine cancel function for auto-retry.
        // Called on AutoRetryEndEvent or when a new retry starts.
        retryCountdownStop func()
    
        // Slash-command registry. Owns 14 builtins + the dynamic
        // extension-registered set. Constructed in Run().
        slashRegistry *SlashRegistry
    
        // usageDashboard is the mounted /usage overlay; usageActivity is its local
        // session-history scanner, kept across opens so its per-file cache survives.
        usageDashboard *usageDashboardState
        usageActivity  *tui.UsageActivity
        // usageService fetches provider quotas for the board. nil leaves the board
        // without a quota source (no credentials wired), which it reports as empty.
        usageService UsageQuotaService
    
        // App-level keybinding registry loaded from <agentDir>/keybindings.json.
        // Mirrors upstream core/keybindings.ts for interactive-mode bindings.
        keybindings *KeybindingsManager
    
        // toolByID holds only pending calls so completion updates the streaming card and releases its ID for later calls. toolOrder retains transcript order for Ctrl+O; toolsExpanded is the global expansion choice.
        toolMu        sync.Mutex
        toolByID      map[string]*tui.ToolExecutionComponent
        toolStarts    map[string]time.Time
    
    // defaultCustomEditor returns the CustomEditor over the current default editor, registering the app handlers Pi's setupKeyHandlers binds the
    // first time and again whenever the editor changes.
    func (m *InteractiveMode) defaultCustomEditor() *CustomEditor {
        if m.customEditor != nil && m.customEditor.Editor == m.editor && m.customEditor.keybindings == m.keybindings {
            return m.customEditor
        }
        // interactive-mode.ts:645-651 builds the editor from the settings' editorPaddingX and autocompleteMaxVisible.
        paddingX, autocompleteMaxVisible := m.opts.Settings.GetEditorPaddingX(), m.opts.Settings.GetAutocompleteMaxVisible()
        options := CustomEditorOptions{PaddingX: &paddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true}
        superInput := func(data string) {
            m.runEditorAction(classifyKeyWithBindings(data, m.keybindings), data)
        }
        if m.editor == nil {
            // interactive-mode.ts:645 `new CustomEditor(this.ui, getEditorTheme(), this.keybindings, {...})`: the default editor is the CustomEditor's own.
            m.customEditor = NewCustomEditor(m.tuiInst, tui.EditorTheme{}, m.keybindings, options)
            m.customEditor.superInput = superInput
            m.editor = m.customEditor.Editor
        } else {
            m.customEditor = wrapCustomEditor(m.editor, m.keybindings, superInput, options)
        }
        m.setupKeyHandlers(m.customEditor)
        return m.customEditor
    }
    
    // runEditorAction runs the editor handling for one classified key under the key's context and keeps the first error for handleCustomEditorKey.
    func (m *InteractiveMode) runEditorAction(action keyAction, data string) {
        if err := m.handleEditorAction(m.inputCtx, action, data); err != nil && m.inputErr == nil {
            m.inputErr = err
        }
    }
        m.opts.BuiltinExtensions = r.BuiltinExtensions
        m.opts.ExplicitBuiltinPaths = r.ExplicitBuiltinPaths
        m.opts.ReloadBuiltinExtensions = r.ReloadBuiltinExtensions
        m.opts.SubprocessUIBridge = r.SubprocessUIBridge
        m.opts.SubprocessHost = r.SubprocessHost
        m.opts.ModelLookup = r.ModelLookup
        m.opts.ModelCatalog = r.ModelCatalog
        m.opts.ModelClassify = r.ModelClassify
        m.opts.ModelGenerateImages = r.ModelGenerateImages
        m.opts.RequestAuthRuntime = r.RequestAuthRuntime
        m.opts.UsageService = r.UsageService
        m.opts.UsageAccountKey = r.UsageAccountKey
        if m.extCtx != nil {
            m.extCtx.CWD = r.CWD
        }
        if event.Reason == "new" {
            m.restoreBuiltInHeader()
        }
        if r.SettingsManager != nil {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:53:// FormatProviderName title-cases a provider id for display
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:55:func FormatProviderName(provider string) string {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:425:func CollapseSharedUsageReports(reports []ai.UsageReport) []ai.UsageReport {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:785:        name := SanitizeDisplayLine(FormatProviderName(key.provider))
    vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go:286:func UsageUsedFraction(limit UsageLimit) (float64, bool) {
    package tui
    
    import (
        "context"
        "fmt"
        "maps"
        "math"
        "regexp"
        "slices"
        "strconv"
        "strings"
        "time"
        "unicode/utf8"
    
        "github.com/MichaelKinsy/PiG/internal/lazyregexp"
    
        "github.com/MichaelKinsy/PiG/internal/jsstring"
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    var pasteMarkerRegex = lazyregexp.New(`\[paste #(\d+)( (\+\d+ lines|\d+ chars))?\]`)
    var pasteMarkerSingle = lazyregexp.New(`^\[paste #(\d+)( (\+\d+ lines|\d+ chars))?\]$`)
    var pasteCtrlCSIU = lazyregexp.New(`\x1b\[(\d+);5u`)
    
    const attachmentAutocompleteDebounce = 20 * time.Millisecond
    
    // Editor is a multi-line text input with undo/kill-ring support.
    type Editor struct {
        // selectListTheme, when set by WithEditorTheme, styles the autocomplete popup in place of the active theme.
        selectListTheme *SelectListTheme
        invalidatable
        // EmbedWorkingStatus opts into the coding-agent status border.
        EmbedWorkingStatus     bool
        workingStatusIndicator *StatusIndicator
        // BorderColor colors a complete border after layout and truncation. Nil selects the application-derived default.
        BorderColor func(string) string
        // Focused emits widthx.CursorMarker so the TUI can position the hardware cursor for IME candidate windows.
        Focused bool
        lines   []string
        cursor  [2]int // [logical line, UTF-16 column]
        // typed holds the line that typing at its end last produced. strings.Builder never rewrites bytes it has handed out, so the next key
        // typed at the end of that line extends the same buffer instead of copying the line.
        typed      strings.Builder
        jumpMode   string // "forward" | "backward" | ""
        history    []editorState
        killRing   KillRing
        lastAction string // "kill" | "yank" | "type-word" | ""
    
        preferredVisualCol   *int
        snappedFromCursorCol *int
    
        // Slash-command autocomplete suggestions.
        // Mirrors upstream `editor.ts` autocomplete state
        // (autocompleteProvider / autocompleteList / autocompletePrefix).
        autocomplete                  AutocompleteProvider
        autocompleteItems             []AutocompleteItem
        autocompleteTriggerCharacters []rune
        // Complete provider callbacks run on owned workers and publish only on the editor loop.
        asyncAutocomplete      *AsyncAutocompleteProvider
        autocompleteLifetime   context.Context
        startAutocompleteTask  func(func())
        startAutocompleteInput func(AutocompleteWork)
        autocompleteError      func(error)
        autocompleteTaskDone   <-chan struct{}
        autocompleteChanged    func(AutocompleteProvider)
        asyncCancel            context.CancelFunc
        asyncSeq               uint64
        scheduleAsyncApply     func(func())
        // nativeAutocomplete is the request of the terminal read in progress, started by FlushAutocomplete.
        nativeAutocomplete *nativeAutocompleteRequest
            visible[i] = e.paddedRow(i, line, paddingX, contentWidth)
        }
    
        // Render top border (with "↑ N more" indicator if scrolled down).
        // Mirrors editor.ts:451-465.
        top := e.renderTopBorder(width, e.scrollOffset)
        top = e.renderStatusBorder(width, e.scrollOffset, top)
        out := make([]string, 0, len(visible)+2)
        out = append(out, top)
        out = append(out, visible...)
    
        // Bottom border (with "↓ N more" indicator if more content below).
        linesBelow := len(visual) - end
        out = append(out, e.renderBottomBorder(width, linesBelow))
    
        // Render slash-autocomplete popup as additional rows
        // below the editor frame. Mirrors upstream editor.ts:521-528 which
        // appends `autocompleteList.render(contentWidth)` lines to its
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/tracker.go:49:        if params, after, ok := strings.Cut(body, ";"); ok {
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:14:// SliceResult is the return shape of SliceWithWidth.
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:15:type SliceResult struct {
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:20:// SliceByColumn extracts a range of visible columns [startCol, startCol+length)
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:25:func SliceByColumn(line string, startCol, length int, strict bool) string {
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:26:    return SliceWithWidth(line, startCol, length, strict).Text
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:29:// SliceWithWidth is like SliceByColumn but also returns the actual visible
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:31:func SliceWithWidth(line string, startCol, length int, strict bool) SliceResult {
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:33:        return SliceResult{}
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/slice.go:90:    return SliceResult{Text: result.String(), Width: resultWidth}
    173:                    m.requestRender()
    ✓ • 172ms

## Activity

    $ rg -n 'func .*Strip|func .*Sanitize' vendor/github.com/MichaelKinsy/PiG/tui/widthx/*.go; rg -n 'func \(m .*requestRender' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*.go; sed -n '415,460p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '1,75p' vendor/github.com/MichaelKinsy/PiG/tui/editor_status_test.go; go version
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/grapheme_range.go:44:func StripTerminalSequences(s string) string {
    vendor/github.com/MichaelKinsy/PiG/tui/widthx/widthx.go:27:func StripAnsi(s string) string {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_tui.go:306:func (m *InteractiveMode) requestRender() {
            collapsed = append(collapsed, limit)
        }
        if !changed {
            return limits
        }
        return collapsed
    }
    
    // CollapseSharedUsageReports collapses routing-specific copies of a shared
    // quota for user-facing usage views.
    func CollapseSharedUsageReports(reports []ai.UsageReport) []ai.UsageReport {
        collapsed := make([]ai.UsageReport, 0, len(reports))
        changed := false
        for _, report := range reports {
            limits := collapseSharedLimits(report.Limits)
            if len(limits) != len(report.Limits) {
                changed = true
                copied := report
                copied.Limits = limits
                collapsed = append(collapsed, copied)
                continue
            }
            collapsed = append(collapsed, report)
        }
        if !changed {
            return reports
        }
        return collapsed
    }
    
    // ─── Codex labels ────────────────────────────────────────────────────────────
    
    func metadataString(report ai.UsageReport, key string) string {
        value, ok := report.Metadata[key]
        if !ok {
            return ""
        }
        text, _ := value.(string)
        return text
    }
    
    func metadataBool(report ai.UsageReport, key string) bool {
        value, ok := report.Metadata[key]
        if !ok {
            return false
        }
    sed: can't read vendor/github.com/MichaelKinsy/PiG/tui/editor_status_test.go: No such file or directory
    go version go1.27.3 linux/arm64
    ✓ • 187ms

## Activity

    $ sed -n '300,325p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_tui.go; rg -n 'Before|Hook' vendor/github.com/MichaelKinsy/PiG/tui/tui.go | head; sed -n '1,75p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; sed -n '1,70p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_rebind.go
    // upstream: interactive-mode.ts:4578 (clearEditor)
    func (m *InteractiveMode) ClearEditor() {
        m.editor.Clear()
        m.tuiInst.Render()
    }
    
    func (m *InteractiveMode) requestRender() {
        m.rendererMu.RLock()
        defer m.rendererMu.RUnlock()
        m.tuiInst.RequestRender()
    }
    
    // teardownCurrentTui runs the current renderer's cleanup (owner-loop tick context
    // cancel). Idempotent and nil-safe; reads the field at call time so a renderer
    // swap's replacement cleanup is the one that runs at teardown.
    func (m *InteractiveMode) teardownCurrentTui() {
        if m.currentTuiCleanup != nil {
            m.currentTuiCleanup()
        }
    }
    
    func (m *InteractiveMode) createInteractiveTui(ctx context.Context) interactiveTuiHandle {
        uiCtx, cancelUI := context.WithCancel(ctx)
        reads := &sync.WaitGroup{}
        m.clipboardCtx, m.clipboardReads = uiCtx, reads
        // The first renderer draws on options.terminal (interactive-mode.ts:626); a renderer switch draws on the previous renderer's terminal (:897 `terminal = previousUi.terminal`).
    229:// InsertBefore adds comp immediately before ref and reports whether ref was a child. Pi splices into Container.children for this (interactive-mode.ts:3836-3839).
    230:func (c *Container) InsertBefore(ref, comp Component) bool {
    1676:    if t.frameDue.IsZero() || t.now().Before(t.frameDue) {
    package ai
    
    // Ports packages/ai/src/usage/openai-codex.ts, openai-codex-base-url.ts and
    // openai-codex-reset.ts: the ChatGPT account usage endpoint, its saved
    // rate-limit resets, and the `x-codex-*` response-header snapshot.
    
    import (
        "context"
        "math"
        "strings"
    )
    
    const (
        codexDefaultBaseURL     = "https://chatgpt.com/backend-api"
        codexUsagePath          = "wham/usage"
        codexVerifiedAccessPath = "accounts/verified_access"
        codexResetCreditsPath   = "wham/rate-limit-reset-credits"
        codexJWTAuthClaim       = "https://api.openai.com/auth"
        codexJWTProfileClaim    = "https://api.openai.com/profile"
        codexSpendCapLimitID    = "openai-codex:spend-cap"
    )
    
    // normalizeCodexBaseURL resolves the account-API base URL. These endpoints live
    // on the canonical ChatGPT origin behind the Codex bearer, so a streaming proxy
    // override must not be used here; an accepted canonical override keeps its
    // origin with `/backend-api` restored.
    func normalizeCodexBaseURL(baseURL string) string {
        trimmed := usageTrimTrailingSlashes(trimJSWhitespace(baseURL))
        if trimmed == "" {
            return codexDefaultBaseURL
        }
        parsed, err := parseHTTPURL(trimmed)
        if err != nil {
            return codexDefaultBaseURL
        }
        host := strings.ToLower(parsed.Hostname)
        if host != "chatgpt.com" && host != "chat.openai.com" {
            return codexDefaultBaseURL
        }
        return parsed.origin() + "/backend-api"
    }
    
    // codexUsageWindow is one parsed `*_window` payload.
    type codexUsageWindow struct {
        usedPercent        *float64
        limitWindowSeconds *int64
        resetAfterSeconds  *float64
        resetAt            *float64
    }
    
    // codexAdditionalUsage is one parsed `additional_rate_limits[]` entry.
    type codexAdditionalUsage struct {
        limitName    string
        meteredName  string
        allowed      *bool
        limitReached *bool
        primary      *codexUsageWindow
        secondary    *codexUsageWindow
    }
    
    // codexSpendCap is the per-user credit cap from `spend_control.individual_limit`.
    type codexSpendCap struct {
        used              *float64
        limit             *float64
        remaining         *float64
        usedFraction      *float64
        reached           *bool
        resetAt           *float64
        resetAfterSeconds *float64
    }
    
    // codexParsedUsage is the parsed `/wham/usage` payload.
    type codexParsedUsage struct {
        planType      string
        allowed       *bool
    package codingagent
    
    // Ports packages/coding-agent/src/modes/interactive/interactive-mode.ts.
    
    import (
        "context"
        "errors"
        "sync/atomic"
    
        "github.com/MichaelKinsy/PiG/agent"
        "github.com/MichaelKinsy/PiG/coding/extension"
        "github.com/MichaelKinsy/PiG/coding/extension/host/inproc"
    )
    
    // runOnMainAndWait waits for an accepted owner operation, including its awaited work. A cancelled queued operation does not mutate the UI, and a caller whose ctx ends before the loop starts the operation abandons it.
    func (m *InteractiveMode) runOnMainAndWait(ctx context.Context, work func() error) error {
        if m.onOwnerLoop() {
            // Queueing and waiting from the owner loop would wait for work only this goroutine can run.
            if err := ctx.Err(); err != nil {
                return context.Cause(ctx)
            }
            return work()
        }
        result := make(chan error, 1)
        // claimed decides once whether the loop runs the operation or the cancelled caller abandons it.
        var claimed atomic.Bool
        if err := m.postToMain(ctx, func() {
            if !claimed.CompareAndSwap(false, true) {
                return
            }
            if ctx.Err() != nil {
                result <- context.Cause(ctx)
                return
            }
            result <- work()
        }); err != nil {
            return err
        }
        var stopped <-chan struct{}
        if m.runCtx != nil {
            stopped = m.runCtx.Done()
        }
        select {
        case err := <-result:
            return err
        case <-ctx.Done():
            if claimed.CompareAndSwap(false, true) {
                return context.Cause(ctx)
            }
            return <-result
        case <-stopped:
            return context.Cause(m.runCtx)
        }
    }
    
    // rebindCurrentSession captures the Session before awaiting extension binding. A replaced startup bind cannot subscribe or update the replacement's title on completion.
    func (m *InteractiveMode) rebindCurrentSession(ctx context.Context, renderBeforeBind bool) error {
        var session InteractiveSessionHandle
        var runner *inproc.Runner
        var cwd string
        var event extension.SessionStartEvent
        if err := m.runOnMainAndWait(ctx, func() error {
            m.bindSubagentUI()
            session, runner, cwd = m.opts.SessionHandle, m.newRunner, m.opts.CWD
            if session == nil {
                return errors.New("interactive: SessionHandle is required")
            }
            m.eventCh = nil
            m.programStatusReporter().Reset()
            m.applyRuntimeSettings()
    ✓ • 0ms

## Activity

    $ sed -n '285,325p' vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go; rg -n 'ModelID:|SharedGroup:|ID:' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go | tail -30; sed -n '458,485p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md; sed -n '660,673p' vendor/github.com/MichaelKinsy/PiG/AGENTS.md; ls /home/ubuntu/.cache/ompig-check/gopath/src
    // used, then inverted remaining fraction.
    func UsageUsedFraction(limit UsageLimit) (float64, bool) {
        amount := limit.Amount
        if amount.UsedFraction != nil {
            return *amount.UsedFraction, true
        }
        if amount.Used != nil && amount.Limit != nil && *amount.Limit > 0 {
            return *amount.Used / *amount.Limit, true
        }
        if amount.Unit == UsageUnitPercent && amount.Used != nil {
            return *amount.Used / 100, true
        }
        if amount.RemainingFraction != nil {
            return math.Max(0, 1-*amount.RemainingFraction), true
        }
        return 0, false
    }
    
    // IsUsageLimitExhausted reports whether a limit has spent its quota. An
    // explicit provider status wins; otherwise the amount fields decide.
    func IsUsageLimitExhausted(limit UsageLimit) bool {
        if limit.Status != "" && limit.Status != UsageStatusUnknown {
            return limit.Status == UsageStatusExhausted
        }
        amount := limit.Amount
        if amount.UsedFraction != nil && *amount.UsedFraction >= 1 {
            return true
        }
        if amount.RemainingFraction != nil && *amount.RemainingFraction <= 0 {
            return true
        }
        if amount.Used != nil && amount.Limit != nil && *amount.Used >= *amount.Limit {
            return true
        }
        if amount.Remaining != nil && *amount.Remaining <= 0 {
            return true
        }
        if amount.Unit == UsageUnitPercent && amount.Used != nil && *amount.Used >= 100 {
            return true
        }
        return false
    308:        return UsageWindow{ID: id, Label: label, DurationMs: &duration, ResetsAt: resetsAt}
    314:    return UsageWindow{ID: key, Label: label, ResetsAt: resetsAt}
    348:        ID:     "openai-codex:" + key,
    350:        Scope:  UsageScope{Provider: "openai-codex", WindowID: usageWindow.ID, Shared: true},
    400:        ID:    "openai-codex:" + slug + ":" + key,
    404:            AccountID: accountID,
    406:            ModelID:   limitName,
    407:            WindowID:  usageWindow.ID,
    432:        ID:     codexSpendCapLimitID,
    434:        Scope:  UsageScope{Provider: "openai-codex", AccountID: accountID, Tier: "credits", WindowID: "spend-cap", Shared: true},
    435:        Window: &UsageWindow{ID: "spend-cap", Label: "Spend cap", ResetsAt: resetsAt},
    495:        credit := codexResetCredit{ID: id}
    565:        ID: "openai-codex",
    - Smoke-only scenarios must be tagged `smoke-only`; they are useful tripwires but not acceptance evidence.
    - If output differs, fix pig or cite a numbered divergence. Comments alone are not enough.
    - Do not weaken a comparator to keep a pig bug green.
    - Scenario comments must explain any `normalize_replace`, `lint-known-gap`, `skip`, `serial`, group override, or weak-quality tag.
    - Keep `covers` narrow. Broad claims inflate the dashboard and hide untested behavior.
    - Re-probe affected cross-family scenarios when a fix changes shared behavior.
    
    Isolation rules for parallel parity:
    - Agent/home dirs are ephemeral copies. Never write into checked-in fixture roots.
    - Use `{{TEMP}}` for writable paths; do not hardcode `/tmp` paths.
    - tmux session names must be unique and cleanup may kill only `parity-*` sessions.
    - `PIG_CODING_AGENT_DIR` and `PI_CODING_AGENT_DIR` win over `PIG_HOME`, and every child process inherits them, including a tmux session started from your shell. A manual probe or test that runs pig must set or clear all of `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME`, `PI_HOME` and `HOME`, or `/logout` and login rewrite the real credentials (the 2026-10-06 lane `auth.json` wipe). A package whose `TestMain` calls `testenv.ScopeTempDir` or `testenv.RunScoped` replaces the agent-directory variables. Other packages inherit them, so a plain `go test` run from an agent shell sets `PIG_CODING_AGENT_DIR`, `PI_CODING_AGENT_DIR`, `PIG_HOME` and `PI_HOME` to scratch directories first. `make test` runs each group under seeded directories and fails a group that adds, removes or changes any file or directory in them (`automation/ci/agent-dir-guard.sh`).
    - `pre_clear_paths` is deprecated; prefer fresh temp dirs.
    - Use the default scheduler groups unless a justified `group = "..."` or `serial` tag is required.
    - Deferred scenarios stay visible in coverage; do not use skip/defer to hide known drift.
    
    ## Divergences
    
    A prior divergence record is investigation evidence, not permanent approval.
    Re-probe it against the exact current upstream and current production path at
    every upstream leap and foundation/release review. Delete or reclassify entries
    that are language mechanics, additive behavior, test limitations,
    unreachable/equal behavior, stale scaffolds, or caller-free code. IDs are global
    across `docs/parity/DIVERGENCES.md` and additive ledgers; duplicate IDs fail even when they
    live in different files.
    
    A divergence is allowed only when it is user-visible or interop-relevant and cannot or should not be made faithful now. Every active divergence must have:
    - `D<N>` id in `docs/parity/DIVERGENCES.md`;
    - `git clean -fd`
    - `git stash`
    - `git commit --no-verify`
    - `git push -f`
    - `tmux kill-server`
    - `go install ./cmd/pig`
    
    Only `tmux kill-session -t <name>` is allowed. Commit only files changed in this session.
    
    Do not dispatch subagents to read local files available in this worktree. Read them yourself.
    
    ## When unsure
    
    Read upstream source, read upstream tests, probe upstream behavior, then make pig match it. Ask the user instead of guessing.
    cloud.google.com
    github.com
    go.yaml.in
    golang.org
    modernc.org
    ompig-slim
    ✓ • 15ms

## Activity

    $ cat > /tmp/add_prompt_quota.py <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'tui/editor.go'
        s=p.read_text().replace('workingStatusIndicator *StatusIndicator','workingStatusIndicator *StatusIndicator\n\t// TopRightStatus supplies an optional single-line border label within the available cells. It runs on the editor owner loop.\n\tTopRightStatus func(int) string\n\ttopRightBudget int\n\ttopRightRendered string')
        s=s.replace('top = e.renderStatusBorder(width, e.scrollOffset, top)','top = e.renderStatusBorder(width, e.scrollOffset, top)\n\ttop = e.renderTopRightStatus(width, top)')
        p.write_text(s)
        p=root/'tui/editor_status.go';s=p.read_text().replace('if e.invalidatable.IsDirty() {','if e.invalidatable.IsDirty() || e.topRightStatusChanged() {',1).replace('dirty := e.invalidatable.NeedsRedraw()','dirty := e.topRightStatusChanged()\n\tdirty = e.invalidatable.NeedsRedraw() || dirty',1).replace('return e.EmbedWorkingStatus && e.workingStatusIndicator != nil\n}', 'return e.TopRightStatus != nil || e.EmbedWorkingStatus && e.workingStatusIndicator != nil\n}',1)
        s+='''
        func (e *Editor) topRightStatusChanged() bool {
        	if e.TopRightStatus == nil {
        		return e.topRightRendered != ""
        	}
        	return e.TopRightStatus(e.topRightBudget) != e.topRightRendered
        }
        
        // Only replace trailing border cells, preserving the working label and centered scroll indicator.
        func (e *Editor) renderTopRightStatus(width int, border string) string {
        	plain := widthx.StripAnsi(border)
        	trailing := len([]rune(plain)) - len([]rune(strings.TrimRight(plain, "─")))
        	e.topRightBudget = max(0, min(width-4, trailing-4))
        	e.topRightRendered = ""
        	if e.TopRightStatus == nil || e.topRightBudget <= 0 {
        		return border
        	}
        	label := e.TopRightStatus(e.topRightBudget)
        	labelWidth := widthx.VisibleWidth(label)
        	if labelWidth == 0 || labelWidth > e.topRightBudget {
        		return border
        	}
        	e.topRightRendered = label
        	left := width - labelWidth - 3
        	return widthx.SliceByColumn(border, 0, left, false) + e.colorEditorBorder(" ") + label + e.colorEditorBorder(" ─")
        }
        ''';p.write_text(s)
        p=root/'internal/codingagent/interactive.go';s=p.read_text().replace('usageService UsageQuotaService','usageService UsageQuotaService\n\tpromptQuota *promptQuotaState',1).replace('go m.tickSpinner(ctx)','go m.tickSpinner(ctx)\n\tm.startPromptQuota(ctx)',1).replace('m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})','m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})\n\tm.editor.TopRightStatus = m.promptQuotaLabel',1);p.write_text(s)
        p=root/'internal/codingagent/interactive_input.go';s=p.read_text().replace('m.setupKeyHandlers(m.customEditor)\n\treturn m.customEditor','m.editor.TopRightStatus = m.promptQuotaLabel\n\tm.setupKeyHandlers(m.customEditor)\n\treturn m.customEditor',1);p.write_text(s)
        p=root/'internal/codingagent/interactive_runtime.go';s=p.read_text().replace('m.opts.UsageService = r.UsageService','m.opts.UsageService = r.UsageService\n\tm.promptQuota = nil\n\tm.refreshPromptQuota(m.backgroundContext())',1);p.write_text(s)
        PY
        python /tmp/add_prompt_quota.py
    ✓ • 71ms

## Activity

    $ rg -n 'backgroundTasks|backgroundCtx|cancelBackground' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go | head -30; sed -n '1,90p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage_test.go; rg -n 'StatusBorder|EmbedWorkingStatus' vendor/github.com/MichaelKinsy/PiG/tui/*test.go | head -20; sed -n '1,65p' vendor/github.com/MichaelKinsy/PiG/tui/status_indicator.go; git diff --stat
    278:    backgroundCtx    context.Context
    280:    backgroundTasks  backgroundGroup
    692:    ctx := m.backgroundCtx
    693:    m.backgroundTasks.Go(func() {
    1591:    m.backgroundCtx, m.backgroundCancel = context.WithCancel(ctx)
    1598:        m.backgroundTasks.Wait()
    1600:        m.backgroundCtx = nil
    1699:    m.editor.SetAutocompleteTaskOwner(m.backgroundCtx, m.backgroundTasks.Go, m.reportAutocompleteError)
    1942:        m.backgroundTasks.Go(func() {
    1944:            refreshCtx, cancel := context.WithTimeout(m.backgroundCtx, 15*time.Second)
    1947:                _ = m.postToMain(m.backgroundCtx, m.updateProviderInfo)
    1982:        m.backgroundTasks.Go(func() {
    sed: can't read vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage_test.go: No such file or directory
    package tui
    
    import (
        "strings"
        "unicode"
    
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    // StatusIndicator renders a loader either standalone or inside an editor border.
    // Its owner advances animation and disposes operation timers on replacement.
    type StatusIndicator struct {
        *Loader
        Kind string
    }
    
    func (s *StatusIndicator) RenderInBorder(width int) string {
        lines := s.Render(width + 2)
        if len(lines) < 2 {
            return ""
        }
        line := strings.TrimPrefix(lines[1], " ")
        return widthx.TruncateToWidth(strings.TrimRightFunc(line, func(r rune) bool { return r == '\ufeff' || (r != '\u0085' && unicode.IsSpace(r)) }), width, "", false)
    }
    
    func (s *StatusIndicator) RenderSpinnerInBorder(width int) string {
        return widthx.TruncateToWidth(s.renderedIndicator(), width, "", false)
    }
    
    // IdleStatus reserves the standalone loader's two rows after clearing it.
    type IdleStatus struct{}
    
    func (*IdleStatus) Invalidate() {}
    func (*IdleStatus) Render(width int) []string {
        line := strings.Repeat(" ", max(0, width))
        return []string{line, line}
    }
     .../PiG/internal/codingagent/interactive.go        |  3 ++
     .../PiG/internal/codingagent/interactive_input.go  |  1 +
     .../internal/codingagent/interactive_runtime.go    |  2 ++
     vendor/github.com/MichaelKinsy/PiG/tui/editor.go   |  5 ++++
     .../MichaelKinsy/PiG/tui/editor_status.go          | 33 ++++++++++++++++++++--
     5 files changed, 41 insertions(+), 3 deletions(-)
    ✓ • 200ms

## Assistant

The bar is wired to the selected provider and will refresh in the background once a minute. It shows remaining quota across accounts, using capacity-weighted totals when available. I’m checking model switches, partial or unavailable reports, and narrow terminals so the bar doesn’t overwrite “Working” or the hidden-lines indicator.

## Activity

    $ cat .gitignore; git status --short --untracked-files=all; git check-ignore -v vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; sed -n '1585,1604p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; rg -n 'func NewEditor|func NewLoader' vendor/github.com/MichaelKinsy/PiG/tui/{editor,loader}.go; rg -n 'type backgroundGroup|func .*backgroundGroup' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*.go; sed -n '1,50p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_latency_test.go
    /bin/
    /ompig-slim
    /ompig-slim.exe
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
                }
            }
        }()
    
        removeWriteErrorHandler := m.exitOnDeadTerminalWrite()
        m.onStop(func() error { removeWriteErrorHandler(); return nil })
        m.backgroundCtx, m.backgroundCancel = context.WithCancel(ctx)
        m.onStop(func() error {
            m.backgroundCancel()
            m.disposeUsageDashboard()
            m.disposeArminComponents()
            m.disposeLogoAnimation()
            m.disposeMarkdownBlocks()
            m.backgroundTasks.Wait()
            m.userBashTasks = nil
            m.backgroundCtx = nil
            return nil
        })
        mark := m.startupMark()
        m.initStartupTheme()
    vendor/github.com/MichaelKinsy/PiG/tui/loader.go:55:func NewLoader(ui TUI, spinnerColorFn, messageColorFn func(string) string, message string, indicator *LoaderIndicatorOptions) *Loader {
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:212:func NewEditor(options ...EditorOption) *Editor {
    vendor/github.com/MichaelKinsy/PiG/tui/editor.go:231:func NewEditorWithTUI(ui TUI, theme EditorTheme, options EditorOptions) *Editor {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/uncaught_goroutine.go:51:type backgroundGroup struct{ sync.WaitGroup }
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/uncaught_goroutine.go:54:func (g *backgroundGroup) Go(task func()) {
    package codingagent
    
    import (
        "context"
        "sync/atomic"
        "testing"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
    )
    
    func TestUsageStartsFastAccountBeforeSlowAccountFinishes(t *testing.T) {
        store := newUsageTestStore()
        store.add("test", new("slow"), ai.Credential{Type: ai.CredentialAPIKey, Key: "slow"})
        store.add("test", new("fast"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fast"})
        release := make(chan struct{})
        defer close(release)
        slowStarted, fastStarted := make(chan struct{}), make(chan struct{})
        provider := &ai.UsageProvider{ID: "test", FetchUsage: func(ctx context.Context, params ai.UsageFetchParams, _ ai.UsageFetchContext) (*ai.UsageReport, error) {
            if params.Credential.APIKey == "slow" {
                close(slowStarted)
                select {
                case <-release:
                case <-ctx.Done():
                    return nil, context.Cause(ctx)
                }
            } else {
                close(fastStarted)
            }
            return usageReportFor("test"), nil
        }}
        service := usageTestService(store, provider)
        ctx, cancel := context.WithCancel(t.Context())
        defer cancel()
        done := make(chan struct{})
        go func() { defer close(done); _, _ = service.FetchSnapshot(ctx, false) }()
        <-slowStarted
        select {
        case <-fastStarted:
        case <-time.After(2 * time.Second):
            t.Fatal("the slow account blocked an independent account's request")
        }
        cancel()
        <-done
    }
    
    func TestUsageProgressPublishesFastAccountWhileSlowAccountIsPending(t *testing.T) {
        store := newUsageTestStore()
        store.add("test", new("slow"), ai.Credential{Type: ai.CredentialAPIKey, Key: "slow"})
        store.add("test", new("fast"), ai.Credential{Type: ai.CredentialAPIKey, Key: "fast"})
    ✓ • 195ms

## User

only show 1 bar and choose the essential one, with the reset time too

## Assistant

I’ll show one bar for the most-used quota window across the provider’s accounts, plus its next reset time. For example, it will switch between the 5-hour and weekly quota depending on which has less capacity remaining.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go');s=p.read_text().replace('m.startPromptQuota(ctx)','m.startPromptQuota(m.backgroundCtx)');p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go');s=p.read_text();start=s.index('\t\tif len(windows) > 0 {');end=s.index('\n\t\t\tpartial :=',start)
        s=s[:start]+'''\t\tif len(windows) > 0 {
        			window := windows[0]
        			for _, candidate := range windows[1:] {
        				if candidate.used > window.used {
        					window = candidate
        				}
        			}
        			remaining := 1 - window.used
        			filled := int(math.Round(remaining * 5))
        			reset := "reset unknown"
        			if window.resetsAt > 0 {
        				delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
        				minutes := int(math.Ceil(delta.Minutes()))
        				if minutes < 60 {
        					reset = fmt.Sprintf("reset %dm", minutes)
        				} else if minutes < 24*60 {
        					reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
        				} else {
        					reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
        				}
        			}
        			status = fmt.Sprintf("%s [%s%s] %.0f%% left · %s", window.label, strings.Repeat("■", filled), strings.Repeat("□", 5-filled), remaining*100, reset)'''+s[end:]
        s=s.replace('// Omit the model name first on narrow terminals; never truncate a quota into a misleading percentage or hide a limiting window.', '// Omit the model name first on narrow terminals; keep the limiting window and its reset intact.')
        s=s.replace('used  float64\n}', 'used  float64\n\tresetsAt int64\n}')
        # gofmt alignment differs inspect through generic insertion
        s=s.replace('type promptQuotaWindow struct {\n\tlabel string\n\tused  float64\n}', 'type promptQuotaWindow struct {\n\tlabel string\n\tused float64\n\tresetsAt int64\n}')
        s=s.replace('unit                ai.UsageUnit','unit                ai.UsageUnit\n\t\tresetsAt int64')
        s=s.replace('b.sum += fraction', '''if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
        				b.resetsAt = *limit.Window.ResetsAt
        			}
        			b.sum += fraction''')
        s=s.replace('used: max(0, min(1, used))}', 'used: max(0, min(1, used)), resetsAt: b.resetsAt}')
        p.write_text(s)
        PY
        rg -n 'resetsAt|type bucket|unit ' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive{,_prompt_quota}.go
    103:            if window.resetsAt > 0 {
    104:                delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
    136:    resetsAt int64
    141:    type bucket struct {
    146:        unit                ai.UsageUnit
    147:        resetsAt int64
    181:            if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
    182:                b.resetsAt = *limit.Window.ResetsAt
    187:            if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
    201:        result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used)), resetsAt: b.resetsAt})
    ✓ • 242ms

## Activity

    $ sed -n '130,152p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; ls vendor/github.com/MichaelKinsy/PiG/tui/*test.go | head; rg -n 'func NewInteractive|func .*InteractiveMode' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go | tail -12; sed -n '212,228p' vendor/github.com/MichaelKinsy/PiG/tui/editor.go; sed -n '53,85p' vendor/github.com/MichaelKinsy/PiG/tui/loader.go
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
            weighted            bool
            unit                ai.UsageUnit
            resetsAt            int64
        }
        var buckets []*bucket
        byKey := map[string]*bucket{}
        for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
            if report.Provider != provider {
    vendor/github.com/MichaelKinsy/PiG/tui/model_metrics_test.go
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go
    vendor/github.com/MichaelKinsy/PiG/tui/usage_progress_test.go
    1349:func (m *InteractiveMode) structuredSystemPrompt() bool {
    1354:func (m *InteractiveMode) currentSystemPromptOptions() *extension.BuildSystemPromptOptions {
    1368:func (m *InteractiveMode) setTurnSystemPrompt(prompt string) {
    1375:func (m *InteractiveMode) clearTurnSystemPrompt() {
    1388:func NewInteractiveMode(runtimeHost InteractiveRuntime, options InteractiveModeOptions) *InteractiveMode {
    1430:func (m *InteractiveMode) newSessionManager() *SessionManager {
    1445:func (m *InteractiveMode) currentSession() *Session {
    1501:func (m *InteractiveMode) printResumeHint() {
    1515:func (m *InteractiveMode) Run(ctx context.Context) (err error) {
    1543:func (m *InteractiveMode) startupMark() func(string) {
    1552:func (m *InteractiveMode) Init(ctx context.Context) (err error) {
    1913:func (m *InteractiveMode) run(ctx context.Context) error {
    func NewEditor(options ...EditorOption) *Editor {
        return newEditor(nil, options)
    }
    
    func newEditor(ui TUI, options []EditorOption) *Editor {
        e := &Editor{ui: ui, lines: []string{""}, inputHistIdx: -1, maxVisibleLines: 5, renderWidth: 80, wrapCacheWidth: -1}
        for _, option := range options {
            option(e)
        }
        return e
    }
    
    // EditorOptions is editor.ts EditorOptions: PaddingX is the horizontal content padding (default 0, floored at 0) and AutocompleteMaxVisible the autocomplete row limit (default 5, clamped to 3..20).
    type EditorOptions struct {
        PaddingX               *int
        AutocompleteMaxVisible *int
    }
    
    // NewLoader is `new Loader(ui, spinnerColorFn, messageColorFn, message, indicator)` (components/loader.ts:28-41). ui may be nil; it receives a RequestRender after every change to the shown text. A nil color function leaves its text unstyled. A nil indicator is upstream's undefined (the default spinner); the constructor applies a non-nil one through SetIndicator, as upstream's constructor calls setIndicator(indicator).
    func NewLoader(ui TUI, spinnerColorFn, messageColorFn func(string) string, message string, indicator *LoaderIndicatorOptions) *Loader {
        l := &Loader{ui: ui, spinnerColorFn: spinnerColorFn, messageColorFn: messageColorFn, Message: message}
        l.SetIndicator(indicator)
        return l
    }
    
    // requestRender asks the Loader's TUI for a render (loader.ts updateDisplay: `if (this.ui) this.ui.requestRender()`). The caller must not hold l.mu.
    func (l *Loader) requestRender() {
        if l.ui != nil {
            l.ui.RequestRender()
        }
    }
    
    // Render produces ["", paddedLine] matching upstream Loader.render(width)
    // which returns ["", ...super.render(width)] where super is Text(paddingX=1).
    func (l *Loader) Render(width int) []string {
        l.mu.Lock()
        defer l.mu.Unlock()
        l.updateDisplay()
        // Upstream Loader extends Text(paddingX=1, paddingY=0) and prepends one
        // empty line in its render override.
        return append([]string{""}, l.Text.Render(width)...)
    }
    
    // updateDisplay rewrites the Text content from the current indicator frame and message (upstream updateDisplay). The caller holds mu.
    func (l *Loader) updateDisplay() {
        content := l.styledMessage()
        if indicator := l.renderedIndicator(); indicator != "" {
            content = indicator + " " + content
        }
        l.PaddingX, l.PaddingY = loaderPaddingX, 0
    ✓ • 27ms

## Activity

    $ sed -n '808,850p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; sed -n '1388,1428p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; rg -n 'uiTaskCh|ownerPostCh|owner.*Ch' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go | head -18; rg -n 'type Model struct' vendor/github.com/MichaelKinsy/PiG/ai/*.go
        if fn == nil {
            return nil
        }
        // The owner check reads the goroutine id, which costs a stack walk, so a post that the queue accepts at once skips it. While owner overflow is pending the owner's own posts must queue behind it.
        if !m.ownerOverflowActive.Load() && m.postUITask(fn) {
            return nil
        }
        if m.onOwnerLoop() {
            m.postFromOwner(ctx, fn)
            return nil
        }
        if ctx == nil {
            // No cancellation source (only happens in tests with no running loop).
            // Degrade to the non-blocking post rather than risk a permanent block.
            if !m.postUITask(fn) {
                return errOwnerLoopUnavailable
            }
            return nil
        }
        select {
        case m.uiTaskCh <- fn:
            m.wakeSpinner()
            return nil
        case <-ctx.Done():
            return fmt.Errorf("%w: %w", errOwnerLoopUnavailable, context.Cause(ctx))
        }
    }
    
    func (m *InteractiveMode) showCacheMissNotices() bool {
        if m.opts.SettingsManager != nil {
            return m.opts.SettingsManager.GetShowCacheMissNotices()
        }
        return m.opts.Settings.GetShowCacheMissNotices()
    }
    
    func (m *InteractiveMode) setModalInputChannel(ch chan []byte) {
        m.modalInputMu.Lock()
        defer m.modalInputMu.Unlock()
        m.setModalInputChannelLocked(ch)
        m.modalInputDepth = 0
    }
    
    func (m *InteractiveMode) setModalInputChannelLocked(ch chan []byte) {
    func NewInteractiveMode(runtimeHost InteractiveRuntime, options InteractiveModeOptions) *InteractiveMode {
        opts := options
        if opts.CWD == "" {
            opts.CWD, _ = os.Getwd()
        }
        if opts.AgentDir == "" {
            opts.AgentDir = DefaultAgentDir()
        }
        // Mirrors the upstream InteractiveMode constructor's
        // setCapabilityOverrides(settingsManager.getTerminalCapabilityOverrides()).
        tui.SetCapabilityOverrides(opts.Settings.GetTerminalCapabilityOverrides())
        ensurePngTranscoder(nil)
        // Pi's constructor uses options.tuiMode ?? settingsManager.getTuiMode().
        if opts.TuiMode == "" {
            opts.TuiMode = string((&SettingsManager{merged: opts.Settings}).GetTuiMode())
        }
    
        m := &InteractiveMode{
            autoTrustOnReloadCwd: opts.AutoTrustOnReloadCwd,
            runtimeHost:          runtimeHost,
            opts:                 opts,
            isIdle:               true,
            resourceSourceInfo:   map[string]ResourceSourceInfo{},
            toolByID:             make(map[string]*tui.ToolExecutionComponent),
            toolStarts:           make(map[string]time.Time),
            uiTaskCh:             make(chan func(), 64),
            extensionErrorWakeCh: make(chan struct{}, 1),
            pendingArgs:          make(map[int]*pendingToolArg),
            workingVisible:       true,
            spinnerIntervalCh:    make(chan time.Duration, 1),
            spinnerWakeCh:        make(chan struct{}, 1),
            spinnerParkCh:        make(chan spinnerPark, 1),
            outputPad:            int(opts.Settings.GetOutputPad()),
            installChanges:       installchange.Default(),
        }
        if opts.InitialThemeSetting != nil {
            setting := *opts.InitialThemeSetting
            m.themeState.currentThemeSetting.Store(&setting)
        }
        return m
    }
    245:    // ownerOverflow holds, in order, the tasks the owner loop posted to its own full uiTaskCh; one background forwarder hands them to the loop. ownerOverflowActive is set while it is non-empty. See postFromOwner.
    363:    // spinnerTickQueued is set while a spinner tick waits in uiTaskCh.
    481:    // uiTaskCh carries closures posted by background workers (e.g. the async
    486:    uiTaskCh chan func()
    746:    case m.uiTaskCh <- fn:
    828:    case m.uiTaskCh <- fn:
    1413:        uiTaskCh:             make(chan func(), 64),
    1693:    // result back here via uiTaskCh so the editor is mutated and rendered
    vendor/github.com/MichaelKinsy/PiG/ai/types.go:1132:type Model struct {
    ✓ • 101ms

## Activity

    $ rg -n '^func New' vendor/github.com/MichaelKinsy/PiG/tui/tui.go | head; sed -n '1132,1155p' vendor/github.com/MichaelKinsy/PiG/ai/types.go; rg -n 'func .*ProviderID' vendor/github.com/MichaelKinsy/PiG/ai/*.go
    176:func NewContainer(children ...Component) *Container {
    1425:func New() *TuiMainScreen {
    1455:func NewTuiMainScreen(terminal Terminal, showHardwareCursor *bool, logDirectory string) *TuiMainScreen {
    1480:func NewWithOutput(out io.Writer, cols, rows int) *TuiMainScreen {
    type Model struct {
        // Type is optional: chat is the default model type, so models without it are chat models.
        Type             ModelType
        ID               string
        DisplayName      string
        Provider         Provider
        Capabilities     ModelCapabilities
        ProviderMeta     ProviderMetadata
        Input            []ModelInputModality
        ThinkingLevelMap ThinkingLevelMap
        SamplingParams   map[string]any
        // SamplingParamsByThinkingLevel overrides SamplingParams for the effective thinking level on OpenAI-compatible APIs.
        SamplingParamsByThinkingLevel SamplingParamsByThinkingLevel
        PromptCache                   ModelPromptCache
        // InputLimits mirrors upstream Model.inputLimits.
        InputLimits *ModelInputLimits
        // Int is the intelligence index delivered by the model catalog (upstream `int`). It is nil when
        // no source publishes a score; it is never synthesized or defaulted to 0.
        Int *float64
        // TPS is the catalog-estimated output speed in tokens per second; nil when unmeasured.
        TPS *float64
        // Description is the provider- or catalog-published one-line blurb; empty when the source has none.
        Description string
        // Catalog records which metadata source filled this model's catalog fields, for display and debugging.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist.go:583:func (p *googleCloudCodeAssistProvider) ID() string { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_streams.go:244:func modelProviderID(model *Model) string {
    vendor/github.com/MichaelKinsy/PiG/ai/pi_messages.go:51:func (p *piMessagesProvider) ID() string   { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/anthropic.go:86:func (p *anthropicProvider) ID() string { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_kit.go:85:func ModelProviderID(model *Model) string {
    vendor/github.com/MichaelKinsy/PiG/ai/faux.go:411:func (p *FauxProviderHandle) ID() string { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/google.go:106:func (p *googleProvider) ID() string { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/mistral.go:70:func (p *mistralProvider) ID() string   { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/model_types.go:100:func (m *Model) ProviderID() string           { return modelProviderID(m) }
    vendor/github.com/MichaelKinsy/PiG/ai/model_types.go:101:func (m *ImageModel) ProviderID() string      { return m.Provider }
    vendor/github.com/MichaelKinsy/PiG/ai/model_types.go:102:func (m *ClassifierModel) ProviderID() string { return m.Provider }
    vendor/github.com/MichaelKinsy/PiG/ai/openai.go:195:func (p *openAIProvider) ID() string { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/google_vertex.go:60:func (p *googleVertexProvider) ID() string   { return p.cfg.ProviderID }
    vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:173:func (p *openAIResponsesProvider) ID() string   { return p.cfg.ProviderID }
    ✓ • 0ms

## Activity

    $ cat > vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go <<'EOF'
        package codingagent
        
        import (
         "context"
         "errors"
         "io"
         "math"
         "strings"
         "testing"
         "time"
        
         "github.com/MichaelKinsy/PiG/ai"
         "github.com/MichaelKinsy/PiG/tui"
        )
        
        func promptTestLimit(id string, fraction float64, reset int64) ai.UsageLimit {
         return ai.UsageLimit{ID:id, Scope: ai.UsageScope{WindowID:id}, Window:&ai.UsageWindow{ID:id, ResetsAt:&reset}, Amount:ai.UsageAmount{UsedFraction:&fraction}}
        }
        
        func TestPromptQuotaTotalsAndEssentialWindow(t *testing.T) {
         later := time.Now().Add(4*time.Hour).UnixMilli()
         sooner := time.Now().Add(2*time.Hour).UnixMilli()
         snapshot := ai.UsageSnapshot{Reports:[]ai.UsageReport{
          {Provider:"openai-codex", Limits:[]ai.UsageLimit{promptTestLimit("5h", .8, sooner), promptTestLimit("7d", .2, later)}},
          {Provider:"openai-codex", Limits:[]ai.UsageLimit{promptTestLimit("5h", .2, later), promptTestLimit("7d", .6, later)}},
          {Provider:"other", Limits:[]ai.UsageLimit{promptTestLimit("monthly", 1, sooner)}},
         }}
         windows:=promptQuotaWindows(snapshot,"openai-codex","luna-6")
         if len(windows)!=2 || windows[0].used != .5 || windows[0].resetsAt != sooner {t.Fatalf("wrong account total or next reset: %+v",windows)}
         m:= &InteractiveMode{opts:InteractiveModeOptions{Model:&ai.Model{ID:"luna-6", ProviderMeta:ai.ProviderMetadata{ProviderID:"openai-codex"}}}, promptQuota:&promptQuotaState{loaded:true,snapshot:snapshot}}
         label:=m.promptQuotaLabel(200)
         if strings.Count(label,"[")!=1 || !strings.Contains(label,"5h") || !strings.Contains(label,"50% left") || !strings.Contains(label,"reset 2h") || strings.Contains(label,"7d") {t.Fatalf("wrong essential quota: %q",label)}
         // Select a different model and provider without waiting for another network fetch.
         m.opts.Model=&ai.Model{ID:"different",ProviderMeta:ai.ProviderMetadata{ProviderID:"other"}}
         if label=m.promptQuotaLabel(200); !strings.Contains(label,"different") || !strings.Contains(label,"0% left") {t.Fatalf("model switch retained the old quota: %q",label)}
         m.opts.Model.ProviderMeta.ProviderID="unsupported"
         if label=m.promptQuotaLabel(200); !strings.Contains(label,"unavailable") || strings.Contains(label,"[") {t.Fatalf("missing quota was rendered as capacity: %q",label)}
        }
        
        func TestPromptQuotaWeightsCapacityAndFiltersModels(t *testing.T) {
         a,b:=promptTestLimit("monthly",1,0),promptTestLimit("monthly",0,0)
         a.Amount.Limit=new(100.0); b.Amount.Limit=new(900.0)
         modelOnly:=promptTestLimit("special",1,0);modelOnly.Scope.ModelID="another-model"
         invalid:=promptTestLimit("invalid",math.NaN(),0)
         snapshot:=ai.UsageSnapshot{Reports:[]ai.UsageReport{{Provider:"test",Limits:[]ai.UsageLimit{a,modelOnly,invalid}}, {Provider:"test",Limits:[]ai.UsageLimit{b}}}}
         windows:=promptQuotaWindows(snapshot,"test","selected-model")
         if len(windows)!=1 || math.Abs(windows[0].used-.1)>1e-9 {t.Fatalf("wrong weighted total or model filtering: %+v",windows)}
        }
        
        type promptQuotaTestService struct {fetch func(context.Context)(ai.UsageSnapshot,error)}
        func(s promptQuotaTestService) FetchSnapshot(ctx context.Context, force bool)(ai.UsageSnapshot,error){return s.fetch(ctx)}
        func(promptQuotaTestService) ActiveAccount(context.Context,string,...*string)(ai.UsageActiveAccount,bool){return ai.UsageActiveAccount{},false}
        
        func TestPromptQuotaRefreshPublishesOnOwnerAndRejectsReplacedSession(t *testing.T){
         ctx,cancel:=context.WithCancel(t.Context());defer cancel()
         m:=&InteractiveMode{uiTaskCh:make(chan func(),8),tuiInst:tui.NewWithOutput(io.Discard,100,30),editor:tui.NewEditor()}
         m.opts.UsageService=promptQuotaTestService{fetch:func(context.Context)(ai.UsageSnapshot,error){return ai.UsageSnapshot{Reports:[]ai.UsageReport{{Provider:"test"}}},nil}}
         m.refreshPromptQuota(ctx)
         var apply func()
         select {case apply= <-m.uiTaskCh:case <-time.After(2*time.Second):t.Fatal("refresh was not delivered")}
         if m.promptQuota.loaded {t.Fatal("worker mutated owner state")}
         apply()
         if !m.promptQuota.loaded || len(m.promptQuota.snapshot.Reports)!=1 {t.Fatal("owner did not publish quota")}
         m.refreshPromptQuota(ctx)
         select {case apply= <-m.uiTaskCh:case <-time.After(2*time.Second):t.Fatal("refresh was not delivered")}
         replacement:= &promptQuotaState{}
         m.promptQuota=replacement
         apply()
         if replacement.loaded {t.Fatal("old session overwrote replacement quota")}
         cancel();m.backgroundTasks.Wait()
        }
        
        func TestPromptQuotaCancellationJoinsFetch(t *testing.T){
         ctx,cancel:=context.WithCancel(t.Context())
         started:=make(chan struct{})
         m:=&InteractiveMode{uiTaskCh:make(chan func(),8)}
         m.opts.UsageService=promptQuotaTestService{fetch:func(ctx context.Context)(ai.UsageSnapshot,error){close(started);<-ctx.Done();return ai.UsageSnapshot{},errors.New("cancelled")}}
         m.startPromptQuota(ctx)
         <-started;cancel()
         done:=make(chan struct{})
         go func(){m.backgroundTasks.Wait();close(done)}()
         select{case <-done:case <-time.After(2*time.Second):t.Fatal("quota tasks outlived shutdown")}
        }
        EOF
        cat > vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go <<'EOF'
        package tui
        
        import (
         "strings"
         "testing"
        
         "github.com/MichaelKinsy/PiG/tui/widthx"
        )
        
        func TestPromptQuotaBorderPreservesStatusAndWidth(t *testing.T){
         e:=NewEditor()
         e.EmbedWorkingStatus=true
         e.SetWorkingStatusIndicator(&StatusIndicator{Kind:"working",Loader:NewLoader(nil,nil,nil,"Working",nil)})
         label:="5h [■■□□□] 40% left · reset 2h"
         e.TopRightStatus=func(budget int)string{if widthx.VisibleWidth(label)<=budget{return label};return ""}
         for _,width:=range []int{1,10,35,80,120}{
          fallback:=e.renderTopBorder(width,3)
          left:=e.renderStatusBorder(width,3,fallback)
          row:=e.renderTopRightStatus(width,left)
          if widthx.VisibleWidth(row)!=width {t.Fatalf("width %d produced %d cells: %q",width,widthx.VisibleWidth(row),row)}
          plain:=widthx.StripAnsi(row)
          if strings.Contains(widthx.StripAnsi(left),"Working") && !strings.Contains(plain,"Working"){t.Fatalf("quota overwrote status: %q",plain)}
          if strings.Contains(widthx.StripAnsi(left),"↑ 3 more") && !strings.Contains(plain,"↑ 3 more"){t.Fatalf("quota overwrote scroll label: %q",plain)}
          if width==120 && !strings.HasSuffix(plain,label+" ─"){t.Fatalf("quota not right aligned: %q",plain)}
         }
        }
        
        func TestPromptQuotaBorderUpdatesWhenIdle(t *testing.T){
         e:=NewEditor()
         label:="50% left"
         e.TopRightStatus=func(int)string{return label}
         e.Render(80);e.NeedsRedraw()
         if e.IsDirty(){t.Fatal("unchanged quota invalidated editor")}
         label="25% left"
         if !e.IsDirty(){t.Fatal("quota update was hidden by render cache")}
         e.Render(80);e.NeedsRedraw()
         e.TopRightStatus=nil
         if !e.IsDirty(){t.Fatal("removing quota did not invalidate border")}
         row:=widthx.StripAnsi(e.Render(80)[0])
         if strings.Contains(row,"left"){t.Fatal("removed quota remained visible")}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    ✓ • 0ms

## Activity

    $ ps -eo pid,etime,pcpu,pmem,args --sort=-pcpu | head -12; git diff -- vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go; sed -n '65,145p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; cat Makefile
        PID     ELAPSED %CPU %MEM COMMAND
    3181228       00:00  100  0.0 ps -eo pid,etime,pcpu,pmem,args --sort=-pcpu
    3180560       00:21 64.2  2.6 /home/ubuntu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-arm64/pkg/tool/linux_arm64/compile @/tmp/args3863289594
    3180905       00:10 57.5  1.3 /home/ubuntu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-arm64/pkg/tool/linux_arm64/compile -o /home/ubuntu/.cache/ompig-check/tmp/go-build2214620260/b419/_pkg_.a -trimpath /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding=>github.com/MichaelKinsy/PiG@v0.7.0/coding;/home/ubuntu/.cache/ompig-check/tmp/go-build2214620260/b419=> -p github.com/MichaelKinsy/PiG/coding -lang=go1.26 -complete -buildid PqpwdYj1wQuZXeVZSlgf/PqpwdYj1wQuZXeVZSlgf -goversion go1.27.3 -c=4 -nolocalimports -importcfg /home/ubuntu/.cache/ompig-check/tmp/go-build2214620260/b419/importcfg -pack /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/auth_storage.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/block_images.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/bug_report.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/client_identity.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/compaction_api.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/create_agent_session.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/define_tool.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/discover_extensions.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/doc.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/early_model_stream.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/extension_bridge.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/extension_flag_values.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/extension_host.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/extension_runner.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_api_request.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_availability.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_catalog_backend.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_registry_facade.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_resolver.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_classify.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_create.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_members.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_native.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_selector.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_typed.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/model_stream_forwarding.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/native_provider.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/nested_tool_calls.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/registered_stream_provider.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/resource_loader.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/resource_loader_extend.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/resource_loader_extension_files.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/runtime.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/runtime_replacement.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/services.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/services_extension_runtime.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_bind.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_boundaries.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_cache_warming.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_compaction_extensions.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_custom_entries.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_custom_messages.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_event_barrier.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_event_hold.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extend_resources.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_core.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_event.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_hooks.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_host.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_loader_host.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_extension_replacement.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_factories.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_bind.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_emit.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runner_ui.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_extension_runtime.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_mcp.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_model_runtime.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_package_manager.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_resource_loader.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_graph_services.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_host_actions.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_images.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_import.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_loadout.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_manager.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_model_cycle.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_ops.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_preflight.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_prompt.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_prompt_resources.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_queue_events.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_recovery.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_replaced.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_replicated_state.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_retry_scheduler.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_run_state.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_scoped_models.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_state.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_subagents.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_summarization_auth.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_tool_registry.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_tools.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_transcript.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/session_tree.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/settings.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/test_faux_enable.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/tool_definitions.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/upstream.go /home/ubuntu/ompig-slim/vendor/github.com/MichaelKinsy/PiG/coding/virtual_models.go
    3181221       00:00 46.4  0.1 /home/ubuntu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-arm64/pkg/tool/linux_arm64/compile -o /home/ubuntu/.cache/ompig-check/tmp/go-build3410468844/b079/_pkg_.a -trimpath /home/ubuntu/.cache/ompig-check/tmp/go-build3410468844/b079=> -p github.com/MichaelKinsy/PiG/ai -lang=go1.27 -complete -installsuffix race -buildid b6ZeP0Fr54Q2WA14_VlF/b6ZeP0Fr54Q2WA14_VlF -goversion go1.27.3 -c=4 -race -nolocalimports -importcfg /home/ubuntu/.cache/ompig-check/tmp/go-build3410468844/b079/importcfg -pack /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/abort_race.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/abort_utils.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/account_policy.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/account_request.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/account_route_ops.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/account_routing.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/accumulated_string.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_client.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_delta_fast.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_event_jsread.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_federation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_message_key.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_request_jsonv2.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/anthropic_tool_input.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/api_key_providers.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/api_streams.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/api_strip.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/assistant_message_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/assistant_observation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/assistant_publication.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/assistant_retry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/assistant_snapshot.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_env_keys.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_interaction.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_json_migration.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_lazy.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_login.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_mutex.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_native_login.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_providers.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_reload.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_reload_snapshot.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_resolve.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_revision_unix.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/auth_store.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/azure.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/azure_config.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/azure_openai_responses.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/block_deltas.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/body_readiness.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/builtin_providers.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/builtin_providers_google_openai_together.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/classifier.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/classifier_http.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/classifier_join.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/classifier_models_generated.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/classifier_wire.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/cloudflare.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/cloudflare_ai_binding.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/cloudflare_streams.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/compat.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/constrained_sampling.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/content_block.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/continuation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/continuation_ticks.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/copilot_verification_uri.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/credential_expiry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/credential_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/decoded_form.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/deferred_option.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/diagnostics.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/direct_simple.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/doc.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/errors.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/estimate.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/event_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/event_stream_continuation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/event_stream_generic.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/event_stream_partial.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/events.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/faux.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/faux_core.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/faux_deferred.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/form_fields.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/gemini_chunk_jsread.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/generated_maps.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/github_copilot_filter.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/githubcopilot.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_heal.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_options.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_request.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_cloud_code_registration.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_sdk_pipeline.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_vertex.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_vertex_config.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/google_wire.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/goroutine_id.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/http_body_observation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/http_proxy.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/http_transport.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/identity.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/identity_models.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/image_models_generated.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/images.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/images_registry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/in_memory_credential_store.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/js_async_generator.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/js_date_parse.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/js_promise.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/json_plain.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/json_repair.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/known_provider.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/llama_cpp_classify.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/messages.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/mistral.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/mistral_chunk_jsread.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/mistral_config.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/mistral_reader.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/mistral_response_body.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_catalog.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_catalog_metadata.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_catalog_metadata_snapshot.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_compat_clone.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_compat_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_operations_export.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_types.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/model_utils.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_catalog_codec.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_catalog_shape.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_generated.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_options.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_provider.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_auth.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_close.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_refresh.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_signal.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_runtime_typed.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_store.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_store_read.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/models_store_snapshot.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/native_body_iterator.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/node_http_proxy.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/node_readable.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/node_readable_iterator.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/node_web_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_anthropic.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_authorization_input.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_callback_server.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_device_code.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_kimi.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_meta.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_openai_chatgpt.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_openrouter.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_page.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_radius.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_registry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_response_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_trusted_url.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_types.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/oauth_xai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_apikey.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_apikey_alibaba.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_apikey_cloudflare.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_apikey_registry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_browser.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_cursor.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_mistral.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_perplexity.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_registry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_snowflake.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_snowflake_catalog.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/omp_oauth_zai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_cache_control.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_chunk_fast.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_chunk_jsread.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_codex_frames.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_codex_http.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_codex_responses.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_codex_sse_stream.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_completions_compat.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_decisions.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_error.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_message_jsonv2.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_message_key.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_prompt_cache.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_responses.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_responses_delta_fast.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_responses_jsread.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_responses_request_jsonv2.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openai_stream_decoder.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/opencode_headers.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_api.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_cost.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_images.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_images_api.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_images_chat.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_user_models.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/openrouter_variants.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/operation_options.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/optional_api.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/ordered_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/ordered_json_object.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/overflow.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/pi_messages.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/pi_messages_events.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/pi_messages_reader.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/pkce.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_constructors.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_env.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_headers.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_kit.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_login.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_names.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_request_options.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_response.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_retry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_stream_event.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_streams.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_strip.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/provider_wrap.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/providers_added.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/providers_added_catalog.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/providers_generated.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/pull_race.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/radius.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/radius_config.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/register_builtins.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/registry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_body_jsonv2.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_body_raw.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_body_splice.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_body_stringify.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_message_cache.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_message_splice_jsonv2.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_string_splice.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/request_validity.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/runtime_credentials.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sampling_params_assign.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sanitize_unicode.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/schema_object_order.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sdk_request_url.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/session_resources.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/simple_options.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/socket_read_linux.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/socket_read_race_linux.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sqlite_auth_storage.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sse.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/sse_event_idle.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_builder.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_continuation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_forward.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_observation.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_observation_capture.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/stream_response.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/streamed_blocks.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/streaming_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/streaming_json_parser.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/string_enum.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/system_one.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/telemetry.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/test_faux.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/text.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/tool_call_arguments.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/tool_flow.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/tool_flow_repair.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/transcript.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/transform_messages.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/transport_diagnostic.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/transport_error.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/tslib_async.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/tslib_promise.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/types.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/types_unions.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_contract.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_json.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_claude.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_claude_reset.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_cursor.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_google.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_kimi.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_openai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_shared.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_xai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/usage_provider_zai.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/user_agent.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/user_agent_unix.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/uuid.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/validate.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/validate_arguments.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/validation_memo.go /home/ubuntu/.cache/ompig-check/gopath/src/github.com/MichaelKinsy/PiG/ai/zstd_codex.go
    3180510       00:25 38.8  1.0 /home/ubuntu/.wakatime/wakatime-cli-linux-arm64 --sync-ai-activity --plugin oh-my-pi/17.4.2 pi-wakatime/0.1.0 --project-folder /home/ubuntu/.herdr/worktrees/ompig-slim/feat-web-search-fetch-tools
    3181046       00:04 34.8  0.8 /home/ubuntu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-arm64/pkg/tool/linux_arm64/vet -atomic -bools -buildtag -directive -errorsas -ifaceassert -nilfunc -printf -slog -stdversion -stringintconv -tests /home/ubuntu/.cache/ompig-check/tmp/go-build3410468844/b316/vet.cfg
    3181134       00:02 29.8  0.5 /home/ubuntu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-arm64/pkg/tool/linux_arm64/compile @/tmp/args230909422
     188473    23:03:05 26.8  2.2 /home/ubuntu/.cache/ms-playwright/chromium_headless_shell-1248/chrome-headless-shell-linux-arm64/chrome-headless-shell --type=renderer --headless=old --no-sandbox --disable-dev-shm-usage --remote-debugging-port=37825 --allow-pre-commit-input --ozone-platform=headless --disable-gpu-compositing --lang=en-US --num-raster-threads=2 --enable-main-frame-before-activation --renderer-client-id=4 --launch-time-ticks=6783991329 --shared-files=v8_context_snapshot_data:100 --field-trial-handle=3,i,1065610364759064543,2813521501046795269,262144 --disable-features=PaintHolding --variations-seed-version --pseudonymization-salt-handle=7,i,14375264915265215641,3168946831109747464,4 --trace-process-track-uuid=3190708990060038890
    3181056       00:04 19.6  0.2 go test -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/tui ./vendor/github.com/MichaelKinsy/PiG/internal/codingagent -run ^TestPromptQuota -count=1
     188463    23:03:05 18.3  0.2 /home/ubuntu/.cache/ms-playwright/chromium_headless_shell-1248/chrome-headless-shell-linux-arm64/chrome-headless-shell --type=gpu-process --gpu-recent-crash-count=0 --no-sandbox --disable-dev-shm-usage --headless=old --ozone-platform=headless --use-angle=swiftshader-webgl --gpu-preferences=YAAAAAAAAAAgAAAEAAAAAAAAAAAAAGAASAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAACAAAAAAAAAAAAAAAAAAAAMAAAAAAAAAAwAAAAAAAAAAAAAAAAAAAACAAAAAAAAAAMAAAAAQAAAAAAAAAAAAAACAAAAAAAAAAIAAAAAAAAAA== --shared-files --field-trial-handle=3,i,1065610364759064543,2813521501046795269,262144 --disable-features=PaintHolding --variations-seed-version --pseudonymization-salt-handle=7,i,14375264915265215641,3168946831109747464,4 --trace-process-track-uuid=3190708988185955192
    3181217       00:00 15.1  0.0 /home/ubuntu/.local/bin/codebase-memory-mcp --cbm-daemon-internal
    diff --git a/vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go b/vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    index 53278ef..1ea8c96 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    @@ -19,7 +19,7 @@ func (e *Editor) SetWorkingStatusIndicator(indicator *StatusIndicator) {
     // editor, so its own dirty flag counts too; otherwise the per-child render
     // cache would keep the old border row until a keystroke dirtied the editor.
     func (e *Editor) IsDirty() bool {
    -    if e.invalidatable.IsDirty() {
    +    if e.invalidatable.IsDirty() || e.topRightStatusChanged() {
             return true
         }
         indicator := e.workingStatusIndicator
    @@ -28,7 +28,8 @@ func (e *Editor) IsDirty() bool {
    
     // NeedsRedraw consumes the editor's and the embedded indicator's dirty flags.
     func (e *Editor) NeedsRedraw() bool {
    -    dirty := e.invalidatable.NeedsRedraw()
    +    dirty := e.topRightStatusChanged()
    +    dirty = e.invalidatable.NeedsRedraw() || dirty
         if indicator := e.workingStatusIndicator; e.EmbedWorkingStatus && indicator != nil {
             dirty = indicator.NeedsRedraw() || dirty
         }
    @@ -38,7 +39,7 @@ func (e *Editor) NeedsRedraw() bool {
     // pig additive (D91): SurfaceLive reports that the embedded indicator, which changes without
     // invalidating the editor, draws in the editor.
     func (e *Editor) SurfaceLive() bool {
    -    return e.EmbedWorkingStatus && e.workingStatusIndicator != nil
    +    return e.TopRightStatus != nil || e.EmbedWorkingStatus && e.workingStatusIndicator != nil
     }
    
     // renderStatusBorder mirrors CustomEditor.renderTopBorder, retaining a centered
    @@ -85,3 +86,29 @@ func (e *Editor) renderStatusBorder(width, hiddenLineCount int, fallback string)
         prefixWidth := min(3, max(0, width-statusWidth))
         return color(strings.Repeat("─", prefixWidth)) + status + color(strings.Repeat("─", max(0, width-prefixWidth-statusWidth)))
     }
    +
    +func (e *Editor) topRightStatusChanged() bool {
    +    if e.TopRightStatus == nil {
    +        return e.topRightRendered != ""
    +    }
    +    return e.TopRightStatus(e.topRightBudget) != e.topRightRendered
    +}
    +
    +// Only replace trailing border cells, preserving the working label and centered scroll indicator.
    +func (e *Editor) renderTopRightStatus(width int, border string) string {
    +    plain := widthx.StripAnsi(border)
    +    trailing := len([]rune(plain)) - len([]rune(strings.TrimRight(plain, "─")))
    +    e.topRightBudget = max(0, min(width-4, trailing-4))
    +    e.topRightRendered = ""
    +    if e.TopRightStatus == nil || e.topRightBudget <= 0 {
    +        return border
    +    }
    +    label := e.TopRightStatus(e.topRightBudget)
    +    labelWidth := widthx.VisibleWidth(label)
    +    if labelWidth == 0 || labelWidth > e.topRightBudget {
    +        return border
    +    }
    +    e.topRightRendered = label
    +    left := width - labelWidth - 3
    +    return widthx.SliceByColumn(border, 0, left, false) + e.colorEditorBorder(" ") + label + e.colorEditorBorder(" ─")
    +}
                // A replacement session owns a new quota service and state.
                if m.promptQuota != state || ctx.Err() != nil {
                    return
                }
                state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
                if m.editor != nil {
                    m.editor.Invalidate()
                }
                m.requestRender()
            })
        })
    }
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        prefix := tui.SanitizeDisplayLine(tui.FormatProviderName(provider))
        model := tui.SanitizeDisplayLine(m.opts.Model.ID)
        state := m.promptQuota
        if state == nil {
            return ""
        }
        status := "loading…"
        if state.loaded {
            status = "quota unavailable"
            windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
            if len(windows) > 0 {
                window := windows[0]
                for _, candidate := range windows[1:] {
                    if candidate.used > window.used {
                        window = candidate
                    }
                }
                remaining := 1 - window.used
                filled := int(math.Round(remaining * 5))
                reset := "reset unknown"
                if window.resetsAt > 0 {
                    delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                    minutes := int(math.Ceil(delta.Minutes()))
                    if minutes < 60 {
                        reset = fmt.Sprintf("reset %dm", minutes)
                    } else if minutes < 24*60 {
                        reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                    } else {
                        reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                    }
                }
                status = fmt.Sprintf("%s [%s%s] %.0f%% left · %s", window.label, strings.Repeat("■", filled), strings.Repeat("□", 5-filled), remaining*100, reset)
                partial := state.failed
                for _, account := range state.snapshot.UnavailableAccounts {
                    partial = partial || account.Provider == provider
                }
                if partial {
                    status += " (partial)"
                }
            }
        }
        // Omit the model name first on narrow terminals; keep the limiting window and its reset intact.
        for _, label := range []string{prefix + " " + model + " · " + status, prefix + " · " + status, status} {
            if widthx.VisibleWidth(label) <= maxWidth {
                return label
            }
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
            weighted            bool
    GO ?= go
    PREFIX ?= $(HOME)/.local
    BINDIR ?= $(PREFIX)/bin
    
    .PHONY: build run install check test race vet smoke release
    
    build:
        $(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    
    run:
        $(GO) run -mod=vendor . $(ARGS)
    
    install:
        $(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o "$(BINDIR)/ompig-slim" .
    
    check test race vet:
        GO="$(GO)" bash scripts/check-go.sh $@
    
    smoke: build
        python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
        python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim
    
    release:
        bash scripts/build-binaries.sh $(ARGS)
    ✓ • 0ms

## Activity

    $ rg -n 'usage|quota|prompt|Working' README.md | head -25; ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -8
    77:`auth login` requires an interactive terminal and masks secret input. Account sign-in is preferred when available; `--api-key` explicitly chooses API-key setup. `--no-browser` selects an alternative sign-in method when offered and never launches a browser. It cannot be combined with `--api-key`. Successful Radius sign-in also configures its global MCP server without a second prompt.
    89:| Account routing | Per-session/provider/model affinity, plan/model eligibility, quota/auth/policy failure scopes, same-account refresh before rotation, and no replay after streamed output |
    92:| Google | Gemini CLI and Antigravity sign-in, project/model discovery, and native Cloud Code Assist streaming, thinking, tools, usage, and error handling |
    94:| Usage | `/usage` plan-grouped quota dashboard, detail view, refresh/caching, reset/daybreak/credit displays, and local 371-day activity heatmap with fork de-duplication |
    122:context usage, and compactions. At an empty prompt, press **Down** or **Left**
    156:`/usage` displays completed account reports as they arrive. Independent accounts fetch concurrently, so a slow provider does not delay healthy providers. Opening the dashboard uses the five-minute quota cache; press `r` to force a refresh. Closing the board cancels its requests, and reopening starts a fresh request if the previous one was canceled.
    170:The native executable was also exercised with isolated profiles: streamed file/shell/nested-code tools, session continuation, JSON/RPC, HTML export, SQLite migration and account logout, quota rotation, all 802 added catalog entries, and local requests through all 16 added provider IDs. A real HTTP fixture additionally verified forced OAuth refresh after a 401 and persisted credential rotation through the library's request path. Real terminal sessions covered masked API-key entry/cancellation, startup, throughput, `/usage` navigation, and both model selectors. Race checks passed for AI/account routing, the coding runtime, metadata, TUI, and CLI packages. Other platform archives are cross-built; only the Linux arm64 executable was run on this host.
    182:Source provenance: `tmih06/omp-slim`, HEAD `ba23ed9c3` over baseline `5a31a6d5f`, including the existing working-tree auth, account-selection, usage, and metadata fixes. No source profile credentials belong in this repository.
        PID     ELAPSED %CPU COMMAND
    3181498       00:00 50.0 ps
    3181404       00:03 43.6 compile
    3181420       00:02 43.3 compile
    3181425       00:02 42.5 vet
    3181221       00:10 41.4 compile
    3181134       00:12 36.6 compile
    3181427       00:02 36.3 compile
    ✓ • 48ms

## Assistant

The display now shows a single bar with remaining percentage and the next reset countdown. It updates when you switch models or providers, and marks totals as partial if an account’s quota can’t be read. The focused tests are still compiling.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('README.md');s=p.read_text();needle='`/usage` displays completed account reports as they arrive.';s=s.replace(needle, 'The prompt’s top-right border shows one live quota bar for the selected model’s provider, aggregated across accounts. It chooses the quota window with the least remaining capacity and shows the next account reset countdown. Quotas refresh every minute in the background; unavailable accounts mark the total as partial. The label shortens or hides when the border has insufficient space.\n\n'+needle);p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go');s=p.read_text().replace('key = limit.Scope.SharedGroup','key = limit.Scope.SharedGroup + "|" + windowID');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ✓ • 162ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go');s=p.read_text();needle='\t// Select a different model and provider without waiting for another network fetch.';insert='''
        	// A freshly fetched weekly bottleneck replaces the short-window bar.
        	m.promptQuota.snapshot.Reports[0].Limits[1].Amount.UsedFraction = new(1.0)
        	m.promptQuota.snapshot.Reports[1].Limits[1].Amount.UsedFraction = new(.8)
        	label = m.promptQuotaLabel(200)
        	if strings.Count(label, "[") != 1 || !strings.Contains(label, "7d") || !strings.Contains(label, "10% left") || strings.Contains(label, "5h") {
        		t.Fatalf("did not select the new limiting window: %q", label)
        	}
        	m.promptQuota.snapshot.UnavailableAccounts = []ai.UsageUnavailableAccount{{Provider: "openai-codex"}}
        	if label = m.promptQuotaLabel(200); !strings.Contains(label, "partial") {
        		t.Fatalf("incomplete totals were presented as complete: %q", label)
        	}
        	m.editor = tui.NewEditor()
        	m.editor.TopRightStatus = m.promptQuotaLabel
        	if row := m.editor.Render(200)[0]; !strings.Contains(row, "10% left") {
        		t.Fatalf("quota did not reach the prompt border: %q", row)
        	}
        ''';s=s.replace(needle,insert+'\n'+needle);p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -8
        PID     ELAPSED %CPU COMMAND
    3182035       00:28 50.4 compile
    3182382       00:19 50.2 compile
    3182881       00:00 50.0 ps
    3182332       00:21 41.7 compile
    3181221       00:58 35.0 compile
     188473    23:04:03 26.8 chrome-headless
    3182524       00:13 21.4 wakatime-cli-li
    ✓ • 313ms

## Activity

    $ git diff --check; git diff --stat; sed -n '1420,1454p' vendor/github.com/MichaelKinsy/PiG/tui/tui.go; sed -n '45,72p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/uncaught_goroutine.go
     README.md                                          |  2 ++
     .../PiG/internal/codingagent/interactive.go        |  3 ++
     .../PiG/internal/codingagent/interactive_input.go  |  1 +
     .../internal/codingagent/interactive_runtime.go    |  2 ++
     vendor/github.com/MichaelKinsy/PiG/tui/editor.go   |  5 ++++
     .../MichaelKinsy/PiG/tui/editor_status.go          | 33 ++++++++++++++++++++--
     6 files changed, 43 insertions(+), 3 deletions(-)
    }
    
    const minRenderInterval = 16 * time.Millisecond
    
    // New creates the main-screen renderer for the process's terminal.
    func New() *TuiMainScreen {
        return NewTuiMainScreen(processTerminal, nil, "")
    }
    
    // terminalWriter writes through a Terminal: its Write for frames and its HideCursor and ShowCursor for the cursor visibility sequences, which
    // upstream's terminal methods send straight to stdout, outside the write log.
    type terminalWriter struct{ terminal Terminal }
    
    func (w terminalWriter) Write(data []byte) (int, error) {
        w.terminal.Write(string(data))
        return len(data), nil
    }
    
    type terminalCursorWriter struct{ terminal Terminal }
    
    func (w terminalCursorWriter) Write(data []byte) (int, error) {
        switch string(data) {
        case "\x1b[?25l":
            w.terminal.HideCursor()
        case "\x1b[?25h":
            w.terminal.ShowCursor()
        default:
            w.terminal.Write(string(data))
        }
        return len(data), nil
    }
    
    // NewTuiMainScreen is `new TuiMainScreen(terminal, showHardwareCursor, logDirectory)` (tui-main-screen.ts, TuiBase's constructor): the inline-flow
    // renderer that draws on terminal and reads its size from it. A nil showHardwareCursor keeps TuiBase's default, false (tui.ts:508,536-538; only the settings getter reads PI_HARDWARE_CURSOR); logDirectory is where
    // the differential-render overflow crash log goes, the OS temp directory when empty.
            panic(value)
        }
        (*handler)(value, debug.Stack())
    }
    
    // backgroundGroup joins goroutines the interactive session starts. A panic in one reaches the uncaught handler.
    type backgroundGroup struct{ sync.WaitGroup }
    
    // Go runs task in a new goroutine that the group's Wait joins.
    func (g *backgroundGroup) Go(task func()) {
        g.WaitGroup.Go(func() {
            defer RecoverUncaught()
            task()
        })
    }
    ✓ • 62ms

## Activity

    $ go test -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/tui ./vendor/github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/tui    0.176s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.113s
    ✓ • 1m 37s

## Activity

    $ cat > vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go <<'EOF'
        package codingagent
        
        import (
        	"context"
        	"fmt"
        	"math"
        	"strings"
        	"time"
        
        	"github.com/MichaelKinsy/PiG/ai"
        	"github.com/MichaelKinsy/PiG/tui"
        	"github.com/MichaelKinsy/PiG/tui/widthx"
        )
        
        // Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
        const promptQuotaInterval = time.Minute
        
        type promptQuotaState struct {
        	snapshot ai.UsageSnapshot
        	fetching bool
        	loaded bool
        	failed bool
        }
        
        // Polling and network work belong to the run's joined background task group. Only the owner loop reads or publishes presentation state.
        func (m *InteractiveMode) startPromptQuota(ctx context.Context) {
        	m.refreshPromptQuota(ctx)
        	m.backgroundTasks.Go(func() {
        		ticker := time.NewTicker(promptQuotaInterval)
        		defer ticker.Stop()
        		for {
        			select {
        			case <-ctx.Done():
        				return
        			case <-ticker.C:
        				if err := m.postToMain(ctx, func() { m.refreshPromptQuota(ctx) }); err != nil {
        					return
        				}
        			}
        		}
        	})
        }
        
        func (m *InteractiveMode) refreshPromptQuota(ctx context.Context) {
        	service := m.opts.UsageService
        	if service == nil {
        		service = m.usageService
        	}
        	if service == nil || ctx.Err() != nil {
        		return
        	}
        	if m.promptQuota == nil {
        		m.promptQuota = &promptQuotaState{}
        	}
        	state := m.promptQuota
        	if state.fetching {
        		return
        	}
        	state.fetching = true
        	m.backgroundTasks.Go(func() {
        		fetchCtx, cancel := context.WithTimeout(ctx, 2*UsageRequestTimeout)
        		defer cancel()
        		snapshot, err := service.FetchSnapshot(fetchCtx, true)
        		_ = m.postToMain(ctx, func() {
        			// A replacement session owns a new quota service and state.
        			if m.promptQuota != state || ctx.Err() != nil {
        				return
        			}
        			state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
        			if m.editor != nil {
        				m.editor.Invalidate()
        			}
        			m.requestRender()
        		})
        	})
        }
        
        func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        	if m.opts.Model == nil || maxWidth <= 0 {
        		return ""
        	}
        	provider := m.opts.Model.ProviderID()
        	prefix := tui.SanitizeDisplayLine(tui.FormatProviderName(provider))
        	model := tui.SanitizeDisplayLine(m.opts.Model.ID)
        	state := m.promptQuota
        	if state == nil {
        		return ""
        	}
        	status := "loading…"
        	if state.loaded {
        		status = "quota unavailable"
        		windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
        		if len(windows) > 0 {
        			parts := make([]string, 0, len(windows))
        			for _, window := range windows {
        				remaining := 1 - window.used
        				filled := int(math.Round(remaining * 5))
        				parts = append(parts, fmt.Sprintf("%s [%s%s] %.0f%%", window.label, strings.Repeat("■", filled), strings.Repeat("□", 5-filled), remaining*100))
        			}
        			status = strings.Join(parts, " ") + " left"
        			partial := state.failed
        			for _, account := range state.snapshot.UnavailableAccounts {
        				partial = partial || account.Provider == provider
        			}
        			if partial {
        				status += " (partial)"
        			}
        		}
        	}
        	// Omit the model name first on narrow terminals; never truncate a quota into a misleading percentage or hide a limiting window.
        	for _, label := range []string{prefix + " " + model + " · " + status, prefix + " · " + status, status} {
        		if widthx.VisibleWidth(label) <= maxWidth {
        			return label
        		}
        	}
        	return ""
        }
        
        type promptQuotaWindow struct {
        	label string
        	used float64
        }
        
        // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
        func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        	type bucket struct {
        		label string
        		sum, used, capacity float64
        		count int
        		weighted bool
        		unit ai.UsageUnit
        	}
        	var buckets []*bucket
        	byKey := map[string]*bucket{}
        	for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
        		if report.Provider != provider {
        			continue
        		}
        		for _, limit := range report.Limits {
        			if limit.Scope.ModelID != "" && limit.Scope.ModelID != model {
        				continue
        			}
        			fraction, ok := ai.UsageUsedFraction(limit)
        			if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
        				continue
        			}
        			windowID, label := limit.Scope.WindowID, limit.Label
        			if limit.Window != nil {
        				windowID = limit.Window.ID
        				label = windowID
        			}
        			if label == "" {
        				label = limit.ID
        			}
        			key := limit.ID + "|" + windowID
        			if limit.Scope.SharedGroup != "" {
        				key = limit.Scope.SharedGroup
        			}
        			b := byKey[key]
        			if b == nil {
        				b = &bucket{label: tui.SanitizeDisplayLine(label), weighted: true, unit: limit.Amount.Unit}
        				byKey[key] = b
        				buckets = append(buckets, b)
        			}
        			b.sum += fraction
        			b.count++
        			capacity := limit.Amount.Limit
        			if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
        				b.weighted = false
        			} else {
        				b.used += fraction * *capacity
        				b.capacity += *capacity
        			}
        		}
        	}
        	result := make([]promptQuotaWindow, 0, len(buckets))
        	for _, b := range buckets {
        		used := b.sum / float64(b.count)
        		if b.weighted && b.capacity > 0 {
        			used = b.used / b.capacity
        		}
        		result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used))})
        	}
        	return result
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go vendor/github.com/MichaelKinsy/PiG/tui/editor.go vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
        go test -mod=vendor ./vendor/github.com/MichaelKinsy/PiG/tui ./vendor/github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/tui    0.192s [no tests to run]
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.086s [no tests to run]
    ✓ • 3m 30s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go');s=p.read_text().replace('// consume. The embedded status indicator is animated and relabelled by its','// consume. The right border label can change with its data source. The embedded status indicator is animated and relabelled by its').replace('// NeedsRedraw consumes the editor\'s and the embedded indicator\'s dirty flags.','// NeedsRedraw consumes the editor\'s and embedded indicator\'s dirty flags and checks the right border label.').replace('// invalidating the editor, draws in the editor.','// invalidating the editor, or a dynamic right border label draws in the editor.')
        a=s.index('func (e *Editor) topRightStatusChanged() bool {');b=s.index('// Only replace trailing',a)
        s=s[:a]+'''func (e *Editor) topRightStatusChanged() bool {
        	return e.topRightStatusLabel() != e.topRightRendered
        }
        
        func (e *Editor) topRightStatusLabel() string {
        	if e.TopRightStatus == nil || e.topRightBudget <= 0 {
        		return ""
        	}
        	label := e.TopRightStatus(e.topRightBudget)
        	if widthx.VisibleWidth(label) > e.topRightBudget {
        		return ""
        	}
        	return label
        }
        
        '''+s[b:]
        s=s.replace('func (e *Editor) renderTopRightStatus(width int, border string) string {\n\tplain', 'func (e *Editor) renderTopRightStatus(width int, border string) string {\n\te.topRightRendered = ""\n\tif e.TopRightStatus == nil {\n\t\te.topRightBudget = 0\n\t\treturn border\n\t}\n\tplain').replace('\n\te.topRightRendered = ""\n\tif e.TopRightStatus == nil || e.topRightBudget <= 0 {\n\t\treturn border\n\t}\n\tlabel := e.TopRightStatus(e.topRightBudget)', '\n\tlabel := e.topRightStatusLabel()').replace('if labelWidth == 0 || labelWidth > e.topRightBudget {','if labelWidth == 0 {');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    ✓ • 0ms

## Activity

    $ rg -n 'usageService =' vendor/github.com/MichaelKinsy/PiG/internal/codingagent; ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -8; git status --short
        PID     ELAPSED %CPU COMMAND
    3184493       00:00  200 ps
    3184189       00:08 98.0 compile
    3184200       00:07 89.4 subagents.test
     188473    23:04:50 26.8 chrome-headless
     188463    23:04:51 18.3 chrome-headless
    3184487       00:00 14.2 bash
    3136094       18:28 11.5 omp
     M README.md
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
    ?? vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        import subprocess
        p=Path('vendor/github.com/MichaelKinsy/PiG/tui/editor.go')
        original=p.read_text()
        try:
         p.write_text(original.replace('\ttop = e.renderTopRightStatus(width, top)\n',''))
         result=subprocess.run(['go','test','-mod=vendor','./vendor/github.com/MichaelKinsy/PiG/tui','-run','^TestPromptQuotaBorderUpdatesWhenIdle$','-count=1'],capture_output=True,text=True)
         print(result.stdout+result.stderr)
         if result.returncode==0:
          raise SystemExit('Mutation unexpectedly passed')
        finally:
         p.write_text(original)
        PY
    --- FAIL: TestPromptQuotaBorderUpdatesWhenIdle (0.01s)
        editor_quota_test.go:52: quota update was hidden by render cache
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/tui    0.121s
    FAIL
    
    ✓ • 12.99s

## Assistant

The focused tests passed, including account aggregation, choosing the limiting window, model/provider switches, and reset selection. I’m finishing the border redraw and background-task cancellation checks, then I’ll build the executable.

## Activity

    $ rg -n 'func \(s \*UsageService\) FetchSnapshot|return snapshot|errors.Join|UnavailableAccounts' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go | head -45; sed -n '160,270p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go; git diff --check; git diff --numstat; wc -l vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota*.go vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    278:        snapshot.UnavailableAccounts = append(snapshot.UnavailableAccounts, unavailable)
    280:    return snapshot
    func (service *UsageService) FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error) {
        return service.FetchSnapshotProgress(ctx, force, nil)
    }
    
    type usageSnapshotLabel struct {
        probe   *usageProviderProbe
        account *ai.CredentialAccountInfo
    }
    
    // FetchSnapshotProgress publishes completed accounts as they arrive, so an
    // unavailable endpoint cannot hide healthy providers behind its timeout. Each
    // push is a new snapshot in stable provider/account order. Cached accounts are
    // published immediately without network when force is false. The caller owns
    // push and receives no calls after this method returns.
    func (service *UsageService) FetchSnapshotProgress(ctx context.Context, force bool, push func(ai.UsageSnapshot)) (ai.UsageSnapshot, error) {
        if ctx == nil {
            ctx = context.Background()
        }
        if err := context.Cause(ctx); err != nil {
            return ai.UsageSnapshot{}, err
        }
        probes, err := service.collectProbes(ctx)
        if err != nil {
            return ai.UsageSnapshot{}, err
        }
        var labels []usageSnapshotLabel
        for _, probe := range probes {
            if len(probe.accounts) == 0 {
                labels = append(labels, usageSnapshotLabel{probe: probe})
                continue
            }
            for i := range probe.accounts {
                labels = append(labels, usageSnapshotLabel{probe: probe, account: &probe.accounts[i]})
            }
        }
        results := make([]usageProbeResult, len(labels))
        completed := make([]bool, len(labels))
        type response struct {
            index  int
            result usageProbeResult
            err    error
        }
        responses := make(chan response, len(labels))
        workCtx, cancel := context.WithCancel(ctx)
        var workers sync.WaitGroup
        // provider-usage.ts starts all account requests before Promise.all. Keep
        // the same independence, including accounts belonging to one provider.
        for index := range labels {
            workers.Go(func() {
                if workCtx.Err() != nil {
                    return
                }
                label := labels[index]
                var result usageProbeResult
                var err error
                if label.account == nil {
                    result, err = service.fetchAmbient(workCtx, label.probe, force)
                } else {
                    result, err = service.fetchAccount(workCtx, label.probe, *label.account, force)
                }
                responses <- response{index, result, err}
            })
        }
        defer func() { cancel(); workers.Wait() }()
        for range labels {
            select {
            case <-ctx.Done():
                return ai.UsageSnapshot{}, context.Cause(ctx)
            case response := <-responses:
                if response.err != nil {
                    return ai.UsageSnapshot{}, response.err
                }
                results[response.index], completed[response.index] = response.result, true
                if push != nil && response.result.reason != NoConfiguredCredential && !(labels[response.index].account == nil && response.result.unsupported) {
                    push(service.usageSnapshot(labels, results, completed))
                }
            }
        }
        return service.usageSnapshot(labels, results, completed), nil
    }
    
    func (service *UsageService) usageSnapshot(labels []usageSnapshotLabel, results []usageProbeResult, completed []bool) ai.UsageSnapshot {
        snapshot := ai.UsageSnapshot{Reports: make([]ai.UsageReport, 0, len(results))}
        for index := range results {
            if !completed[index] {
                continue
            }
            result := results[index]
            if result.report != nil {
                snapshot.Reports = append(snapshot.Reports, *result.report)
                continue
            }
            // A provider nothing is configured for was never asked; reporting it would
            // invent a failure.
            if result.reason == NoConfiguredCredential {
                continue
            }
            entry := labels[index]
            // An ambient key the endpoint does not accept is not a failed account.
            if entry.account == nil && result.unsupported {
                continue
            }
            reason := result.reason
            if reason == "" {
                reason = "no usage reported"
            }
            unavailable := ai.UsageUnavailableAccount{Provider: entry.probe.provider, Reason: reason}
            if entry.account == nil {
                unavailable.Label = entry.probe.provider + " (environment)"
                unavailable.Ambient = true
            } else {
    2    0    README.md
    3    0    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
    1    0    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
    2    0    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
    5    0    vendor/github.com/MichaelKinsy/PiG/tui/editor.go
    42    6    vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
      204 vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
      147 vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
       64 vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
      415 total
    ✓ • 28ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go');s=p.read_text().replace('m.promptQuota = nil\n\tm.refreshPromptQuota(m.backgroundContext())','m.promptQuota = nil\n\tif m.backgroundCtx != nil {\n\t\tm.refreshPromptQuota(m.backgroundCtx)\n\t}');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
        sed -n '1,95p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; sed -n '86,145p' vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    package codingagent
    
    import (
        "context"
        "fmt"
        "math"
        "strings"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
        "github.com/MichaelKinsy/PiG/tui"
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    // Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
    const promptQuotaInterval = time.Minute
    
    type promptQuotaState struct {
        snapshot ai.UsageSnapshot
        fetching bool
        loaded   bool
        failed   bool
    }
    
    // Polling and network work belong to the run's joined background task group. Only the owner loop reads or publishes presentation state.
    func (m *InteractiveMode) startPromptQuota(ctx context.Context) {
        m.refreshPromptQuota(ctx)
        m.backgroundTasks.Go(func() {
            ticker := time.NewTicker(promptQuotaInterval)
            defer ticker.Stop()
            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if err := m.postToMain(ctx, func() { m.refreshPromptQuota(ctx) }); err != nil {
                        return
                    }
                }
            }
        })
    }
    
    func (m *InteractiveMode) refreshPromptQuota(ctx context.Context) {
        service := m.opts.UsageService
        if service == nil {
            service = m.usageService
        }
        if service == nil || ctx.Err() != nil {
            return
        }
        if m.promptQuota == nil {
            m.promptQuota = &promptQuotaState{}
        }
        state := m.promptQuota
        if state.fetching {
            return
        }
        state.fetching = true
        m.backgroundTasks.Go(func() {
            fetchCtx, cancel := context.WithTimeout(ctx, 2*UsageRequestTimeout)
            defer cancel()
            snapshot, err := service.FetchSnapshot(fetchCtx, true)
            _ = m.postToMain(ctx, func() {
                // A replacement session owns a new quota service and state.
                if m.promptQuota != state || ctx.Err() != nil {
                    return
                }
                state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
                if m.editor != nil {
                    m.editor.Invalidate()
                }
                m.requestRender()
            })
        })
    }
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        prefix := tui.SanitizeDisplayLine(tui.FormatProviderName(provider))
        model := tui.SanitizeDisplayLine(m.opts.Model.ID)
        state := m.promptQuota
        if state == nil {
            return ""
        }
        status := "loading…"
        if state.loaded {
            status = "quota unavailable"
            windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
            if len(windows) > 0 {
                window := windows[0]
                for _, candidate := range windows[1:] {
        prefixWidth := min(3, max(0, width-statusWidth))
        return color(strings.Repeat("─", prefixWidth)) + status + color(strings.Repeat("─", max(0, width-prefixWidth-statusWidth)))
    }
    
    func (e *Editor) topRightStatusChanged() bool {
        return e.topRightStatusLabel() != e.topRightRendered
    }
    
    func (e *Editor) topRightStatusLabel() string {
        if e.TopRightStatus == nil || e.topRightBudget <= 0 {
            return ""
        }
        label := e.TopRightStatus(e.topRightBudget)
        if widthx.VisibleWidth(label) > e.topRightBudget {
            return ""
        }
        return label
    }
    
    // Only replace trailing border cells, preserving the working label and centered scroll indicator.
    func (e *Editor) renderTopRightStatus(width int, border string) string {
        e.topRightRendered = ""
        if e.TopRightStatus == nil {
            e.topRightBudget = 0
            return border
        }
        plain := widthx.StripAnsi(border)
        trailing := len([]rune(plain)) - len([]rune(strings.TrimRight(plain, "─")))
        e.topRightBudget = max(0, min(width-4, trailing-4))
        label := e.topRightStatusLabel()
        labelWidth := widthx.VisibleWidth(label)
        if labelWidth == 0 {
            return border
        }
        e.topRightRendered = label
        left := width - labelWidth - 3
        return widthx.SliceByColumn(border, 0, left, false) + e.colorEditorBorder(" ") + label + e.colorEditorBorder(" ─")
    }
    ✓ • 156ms

## Activity

    $ ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -8; git diff --check; git diff -- vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
        PID     ELAPSED %CPU COMMAND
    3188124       00:21 81.1 compile
    3188821       00:00 50.0 ps
    3188267       00:16 48.0 compile
    3188803       00:00 42.0 compile
    3188372       00:12 36.4 wakatime-cli-li
    3188783       00:01 32.0 codebase-memory
    3188814       00:00 27.7 compile
    diff --git a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
    index 0f77cf8..c1e47eb 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
    @@ -424,6 +424,7 @@ type InteractiveMode struct {
         // usageService fetches provider quotas for the board. nil leaves the board
         // without a quota source (no credentials wired), which it reports as empty.
         usageService UsageQuotaService
    +    promptQuota  *promptQuotaState
    
         // App-level keybinding registry loaded from <agentDir>/keybindings.json.
         // Mirrors upstream core/keybindings.ts for interactive-mode bindings.
    @@ -1675,6 +1676,7 @@ func (m *InteractiveMode) Init(ctx context.Context) (err error) {
         mark("tui-layout-built")
         editorPaddingX, autocompleteMaxVisible := m.opts.Settings.GetEditorPaddingX(), m.opts.Settings.GetAutocompleteMaxVisible()
         m.editor = NewEditorWithOptions(CustomEditorOptions{UI: m.tuiInst, PaddingX: &editorPaddingX, AutocompleteMaxVisible: &autocompleteMaxVisible, EmbedWorkingStatus: true})
    +    m.editor.TopRightStatus = m.promptQuotaLabel
         m.bindEditorSnapshot()
         m.defaultCustomEditor()
         m.bindEditorSnapshot()
    @@ -1953,6 +1955,7 @@ func (m *InteractiveMode) run(ctx context.Context) error {
    
         // Start spinner ticker
         go m.tickSpinner(ctx)
    +    m.startPromptQuota(m.backgroundCtx)
    
         m.startGitBranchWatcher(ctx)
    
    diff --git a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
    index 9c3024d..195be08 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
    @@ -848,6 +848,7 @@ func (m *InteractiveMode) defaultCustomEditor() *CustomEditor {
         } else {
             m.customEditor = wrapCustomEditor(m.editor, m.keybindings, superInput, options)
         }
    +    m.editor.TopRightStatus = m.promptQuotaLabel
         m.setupKeyHandlers(m.customEditor)
         return m.customEditor
     }
    diff --git a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
    index ca66b81..65f63c5 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
    @@ -200,6 +200,10 @@ func (m *InteractiveMode) applyReplacement(session InteractiveSessionHandle, r I
         m.opts.ModelGenerateImages = r.ModelGenerateImages
         m.opts.RequestAuthRuntime = r.RequestAuthRuntime
         m.opts.UsageService = r.UsageService
    +    m.promptQuota = nil
    +    if m.backgroundCtx != nil {
    +        m.refreshPromptQuota(m.backgroundCtx)
    +    }
         m.opts.UsageAccountKey = r.UsageAccountKey
         if m.extCtx != nil {
             m.extCtx.CWD = r.CWD
    ✓ • 85ms

## Assistant

The redraw regression test correctly failed when I temporarily removed the border rendering call, so it catches a quota update getting stuck on screen. The implementation is restored. The race checks and executable build are still running; I haven’t tested against a live subscription account.

## Activity

    $ rg -n '^func \(t \*TuiMainScreen\) RequestRender|^func \(t \*TuiMainScreen\) SetRenderScheduler' vendor/github.com/MichaelKinsy/PiG/tui/tui.go; ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -8; git status --short
        PID     ELAPSED %CPU COMMAND
    3190295       00:00  100 ps
    3190169       00:06 94.5 compile
    3190123       00:07 65.1 compile
    3190226       00:05 56.3 codebase-memory
    3188267       00:45 49.1 compile
     188473    23:06:59 26.8 chrome-headless
     188463    23:06:59 18.3 chrome-headless
     M README.md
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
    ?? vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    ✓ • 54ms

## Activity

    $ make build
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 1m 35s

## Assistant

The executable builds successfully. The race checks are still running for the prompt renderer and quota integration. The bar shows the account-total remaining percentage and the next reset, with separate account details kept in `/usage`.

## Activity

    $ make build
        git diff --check
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 43.22s

## Activity

    $ python - <<'PY'
        import os, subprocess, tempfile
        with tempfile.TemporaryDirectory(prefix='ompig-quota-check-') as profile:
         env=os.environ.copy()
         for key in ('PIG_CODING_AGENT_DIR','PI_CODING_AGENT_DIR','OMPIG_SLIM_CODING_AGENT_DIR'):
          env[key]=profile+'/agent'
         for key in ('PIG_HOME','PI_HOME'):
          env[key]=profile+'/library'
         env.update(PI_SKIP_VERSION_CHECK='1',PI_TELEMETRY='0')
         raise SystemExit(subprocess.call(['go','test','-race','-mod=vendor','github.com/MichaelKinsy/PiG/tui','github.com/MichaelKinsy/PiG/internal/codingagent'],env=env))
        PY
    ok      github.com/MichaelKinsy/PiG/tui    1.371s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    1.232s
    ✓ • 3m 04s

## Assistant

Implemented one right-aligned quota bar that:

- Follows the selected model/provider.
- Aggregates quota across accounts.
- Chooses the window with the least capacity remaining.
- Shows remaining percentage and next reset countdown.
- Refreshes every minute.

“Working” stays on the left. Renderer and quota tests passed with race detection; the executable builds successfully.

## User

remove the provider name and model, i only want the window, bar, percentage, and reset time, make the bar more detailed, same as the one inside the usage board

## Assistant

I’ll remove the provider and model labels, then match the bar’s detail and styling to the usage board.

## Activity

    $ git status --short; rg -n 'bar|Bar|▓|█|▏|░|■' vendor/github.com/MichaelKinsy/PiG/tui/usage* | head -85; cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
     M README.md
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
    ?? vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:278:    if !strings.ContainsAny(lines[labelAt+1], "█░") {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:279:        t.Errorf("stacked bar row = %q", lines[labelAt+1])
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:527:        t.Errorf("overview missing bar value:\n%s", overview)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard_test.go:809:            t.Errorf("width %d lost bar values:\n%s", width, text)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:18:    usageReportBarWidthMax = 24
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:21:    // bar instead of abbreviating every account to the same `a…st`.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:157:// on one row above the bars, padded to the bar column so each account stays in
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:401:// renderUsageBar draws one account's quota bar.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:402:func renderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:410:        return theme.Fg(string(ThemeColorDim), widthx.TruncateToWidth(used+" used", barWidth, "…", false))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:414:        return theme.Fg(string(ThemeColorDim), strings.Repeat("·", barWidth))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:417:    exact := clamped * float64(barWidth)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:423:        partial = "▓"
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:427:    leading := strings.Repeat("█", fullCells) + partial
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:432:    empty := strings.Repeat("░", max(0, barWidth-filled))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:441:    barWidth    int
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:442:    // stacked prints label and bar on separate rows when even one column
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:450:// bars, and per-account values, and below one column's minimum width each
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:451:// account stacks label over bar. No account is ever dropped horizontally.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:461:        barWidth := max(1, budget)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:466:                columnWidth: barWidth,
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:467:                barWidth:    barWidth,
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:480:            barWidth:    min(columnWidth, usageReportBarWidthMax),
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:716:                    // prints its own label, bar, and remaining value.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:726:                            "  "+renderUsageBar(theme, account.limit, block.barWidth)+suffix, " "))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:733:                bars := make([]string, 0, len(blockAccounts))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:735:                    bars = append(bars, padUsageColumn(renderUsageBar(theme, account.limit, block.barWidth), block.columnWidth))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:737:                barLine := "  " + strings.Join(bars, " ")
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:739:                    barLine += " " + amountText
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:741:                lines = append(lines, strings.TrimRight(barLine, " "))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:376:// FormatAbsoluteOnlyAmount renders text for a bucket no quota bar can draw:
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:282:    usageCardMinBarWidth = 12
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:289:    barWidth     int
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:576:func miniBar(fraction *float64, status ai.UsageStatus, width int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:583:    return theme.Fg(StatusColorToken(status), strings.Repeat("█", filled)) +
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:584:        theme.Fg(string(ThemeColorDim), strings.Repeat("░", width-filled))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:707:            prefix+miniBar(window.Fraction, window.Status, layout.barWidth)+pctText+resetText, contentWidth) {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:776:        // One geometry per grid row: labels, bars, and resets share columns,
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:777:        // and every card stacks together when inline bars would be too short.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:801:        inlineBarWidth := contentWidth - labelWidth - 1 - suffixWidth
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:802:        stacked := inlineBarWidth < usageCardMinBarWidth
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:841:            barWidth:     max(1, inlineBarWidth),
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:846:            layout.barWidth = max(1, contentWidth-suffixWidth)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:873:    // Untouched providers collapse into a single tick line: per-window bars at
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:984:                line.WriteString(ramp[cell-1] + "■" + reset + " ")
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:1071:    bar := ActiveTheme().Fg(string(ThemeColorBorder), "│")
    vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go:1072:    return bar + " " + padToWidth(clipRow(content, innerWidth), innerWidth) + " " + bar
    package codingagent
    
    import (
        "context"
        "fmt"
        "math"
        "strings"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
        "github.com/MichaelKinsy/PiG/tui"
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    // Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
    const promptQuotaInterval = time.Minute
    
    type promptQuotaState struct {
        snapshot ai.UsageSnapshot
        fetching bool
        loaded   bool
        failed   bool
    }
    
    // Polling and network work belong to the run's joined background task group. Only the owner loop reads or publishes presentation state.
    func (m *InteractiveMode) startPromptQuota(ctx context.Context) {
        m.refreshPromptQuota(ctx)
        m.backgroundTasks.Go(func() {
            ticker := time.NewTicker(promptQuotaInterval)
            defer ticker.Stop()
            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if err := m.postToMain(ctx, func() { m.refreshPromptQuota(ctx) }); err != nil {
                        return
                    }
                }
            }
        })
    }
    
    func (m *InteractiveMode) refreshPromptQuota(ctx context.Context) {
        service := m.opts.UsageService
        if service == nil {
            service = m.usageService
        }
        if service == nil || ctx.Err() != nil {
            return
        }
        if m.promptQuota == nil {
            m.promptQuota = &promptQuotaState{}
        }
        state := m.promptQuota
        if state.fetching {
            return
        }
        state.fetching = true
        m.backgroundTasks.Go(func() {
            fetchCtx, cancel := context.WithTimeout(ctx, 2*UsageRequestTimeout)
            defer cancel()
            snapshot, err := service.FetchSnapshot(fetchCtx, true)
            _ = m.postToMain(ctx, func() {
                // A replacement session owns a new quota service and state.
                if m.promptQuota != state || ctx.Err() != nil {
                    return
                }
                state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
                if m.editor != nil {
                    m.editor.Invalidate()
                }
                m.requestRender()
            })
        })
    }
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        prefix := tui.SanitizeDisplayLine(tui.FormatProviderName(provider))
        model := tui.SanitizeDisplayLine(m.opts.Model.ID)
        state := m.promptQuota
        if state == nil {
            return ""
        }
        status := "loading…"
        if state.loaded {
            status = "quota unavailable"
            windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
            if len(windows) > 0 {
                window := windows[0]
                for _, candidate := range windows[1:] {
                    if candidate.used > window.used {
                        window = candidate
                    }
                }
                remaining := 1 - window.used
                filled := int(math.Round(remaining * 5))
                reset := "reset unknown"
                if window.resetsAt > 0 {
                    delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                    minutes := int(math.Ceil(delta.Minutes()))
                    if minutes < 60 {
                        reset = fmt.Sprintf("reset %dm", minutes)
                    } else if minutes < 24*60 {
                        reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                    } else {
                        reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                    }
                }
                status = fmt.Sprintf("%s [%s%s] %.0f%% left · %s", window.label, strings.Repeat("■", filled), strings.Repeat("□", 5-filled), remaining*100, reset)
                partial := state.failed
                for _, account := range state.snapshot.UnavailableAccounts {
                    partial = partial || account.Provider == provider
                }
                if partial {
                    status += " (partial)"
                }
            }
        }
        // Omit the model name first on narrow terminals; keep the limiting window and its reset intact.
        for _, label := range []string{prefix + " " + model + " · " + status, prefix + " · " + status, status} {
            if widthx.VisibleWidth(label) <= maxWidth {
                return label
            }
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
            weighted            bool
            unit                ai.UsageUnit
            resetsAt            int64
        }
        var buckets []*bucket
        byKey := map[string]*bucket{}
        for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
            if report.Provider != provider {
                continue
            }
            for _, limit := range report.Limits {
                if limit.Scope.ModelID != "" && limit.Scope.ModelID != model {
                    continue
                }
                fraction, ok := ai.UsageUsedFraction(limit)
                if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
                    continue
                }
                windowID, label := limit.Scope.WindowID, limit.Label
                if limit.Window != nil {
                    windowID = limit.Window.ID
                    label = windowID
                }
                if label == "" {
                    label = limit.ID
                }
                key := limit.ID + "|" + windowID
                if limit.Scope.SharedGroup != "" {
                    key = limit.Scope.SharedGroup + "|" + windowID
                }
                b := byKey[key]
                if b == nil {
                    b = &bucket{label: tui.SanitizeDisplayLine(label), weighted: true, unit: limit.Amount.Unit}
                    byKey[key] = b
                    buckets = append(buckets, b)
                }
                if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
                    b.resetsAt = *limit.Window.ResetsAt
                }
                b.sum += fraction
                b.count++
                capacity := limit.Amount.Limit
                if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
                    b.weighted = false
                } else {
                    b.used += fraction * *capacity
                    b.capacity += *capacity
                }
            }
        }
        result := make([]promptQuotaWindow, 0, len(buckets))
        for _, b := range buckets {
            used := b.sum / float64(b.count)
            if b.weighted && b.capacity > 0 {
                used = b.used / b.capacity
            }
            result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used)), resetsAt: b.resetsAt})
        }
        return result
    }
    package codingagent
    
    import (
        "context"
        "errors"
        "io"
        "math"
        "strings"
        "testing"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
        "github.com/MichaelKinsy/PiG/tui"
    )
    
    func promptTestLimit(id string, fraction float64, reset int64) ai.UsageLimit {
        return ai.UsageLimit{ID: id, Scope: ai.UsageScope{WindowID: id}, Window: &ai.UsageWindow{ID: id, ResetsAt: &reset}, Amount: ai.UsageAmount{UsedFraction: &fraction}}
    }
    
    func TestPromptQuotaTotalsAndEssentialWindow(t *testing.T) {
        later := time.Now().Add(4 * time.Hour).UnixMilli()
        sooner := time.Now().Add(2 * time.Hour).UnixMilli()
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
            {Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .8, sooner), promptTestLimit("7d", .2, later)}},
            {Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .2, later), promptTestLimit("7d", .6, later)}},
            {Provider: "other", Limits: []ai.UsageLimit{promptTestLimit("monthly", 1, sooner)}},
        }}
        windows := promptQuotaWindows(snapshot, "openai-codex", "luna-6")
        if len(windows) != 2 || windows[0].used != .5 || windows[0].resetsAt != sooner {
            t.Fatalf("wrong account total or next reset: %+v", windows)
        }
        m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "luna-6", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
        label := m.promptQuotaLabel(200)
        if strings.Count(label, "[") != 1 || !strings.Contains(label, "5h") || !strings.Contains(label, "50% left") || !strings.Contains(label, "reset 2h") || strings.Contains(label, "7d") {
            t.Fatalf("wrong essential quota: %q", label)
        }
    
        // A freshly fetched weekly bottleneck replaces the short-window bar.
        m.promptQuota.snapshot.Reports[0].Limits[1].Amount.UsedFraction = new(1.0)
        m.promptQuota.snapshot.Reports[1].Limits[1].Amount.UsedFraction = new(.8)
        label = m.promptQuotaLabel(200)
        if strings.Count(label, "[") != 1 || !strings.Contains(label, "7d") || !strings.Contains(label, "10% left") || strings.Contains(label, "5h") {
            t.Fatalf("did not select the new limiting window: %q", label)
        }
        m.promptQuota.snapshot.UnavailableAccounts = []ai.UsageUnavailableAccount{{Provider: "openai-codex"}}
        if label = m.promptQuotaLabel(200); !strings.Contains(label, "partial") {
            t.Fatalf("incomplete totals were presented as complete: %q", label)
        }
        m.editor = tui.NewEditor()
        m.editor.TopRightStatus = m.promptQuotaLabel
        if row := m.editor.Render(200)[0]; !strings.Contains(row, "10% left") {
            t.Fatalf("quota did not reach the prompt border: %q", row)
        }
    
        // Select a different model and provider without waiting for another network fetch.
        m.opts.Model = &ai.Model{ID: "different", ProviderMeta: ai.ProviderMetadata{ProviderID: "other"}}
        if label = m.promptQuotaLabel(200); !strings.Contains(label, "different") || !strings.Contains(label, "0% left") {
            t.Fatalf("model switch retained the old quota: %q", label)
        }
        m.opts.Model.ProviderMeta.ProviderID = "unsupported"
        if label = m.promptQuotaLabel(200); !strings.Contains(label, "unavailable") || strings.Contains(label, "[") {
            t.Fatalf("missing quota was rendered as capacity: %q", label)
        }
    }
    
    func TestPromptQuotaWeightsCapacityAndFiltersModels(t *testing.T) {
        a, b := promptTestLimit("monthly", 1, 0), promptTestLimit("monthly", 0, 0)
        a.Amount.Limit = new(100.0)
        b.Amount.Limit = new(900.0)
        modelOnly := promptTestLimit("special", 1, 0)
        modelOnly.Scope.ModelID = "another-model"
        invalid := promptTestLimit("invalid", math.NaN(), 0)
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "test", Limits: []ai.UsageLimit{a, modelOnly, invalid}}, {Provider: "test", Limits: []ai.UsageLimit{b}}}}
        windows := promptQuotaWindows(snapshot, "test", "selected-model")
        if len(windows) != 1 || math.Abs(windows[0].used-.1) > 1e-9 {
            t.Fatalf("wrong weighted total or model filtering: %+v", windows)
        }
    }
    
    type promptQuotaTestService struct {
        fetch func(context.Context) (ai.UsageSnapshot, error)
    }
    
    func (s promptQuotaTestService) FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error) {
        return s.fetch(ctx)
    }
    func (promptQuotaTestService) ActiveAccount(context.Context, string, ...*string) (ai.UsageActiveAccount, bool) {
        return ai.UsageActiveAccount{}, false
    }
    
    func TestPromptQuotaRefreshPublishesOnOwnerAndRejectsReplacedSession(t *testing.T) {
        ctx, cancel := context.WithCancel(t.Context())
        defer cancel()
        m := &InteractiveMode{uiTaskCh: make(chan func(), 8), tuiInst: tui.NewWithOutput(io.Discard, 100, 30), editor: tui.NewEditor()}
        m.opts.UsageService = promptQuotaTestService{fetch: func(context.Context) (ai.UsageSnapshot, error) {
            return ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "test"}}}, nil
        }}
        m.refreshPromptQuota(ctx)
        var apply func()
        select {
        case apply = <-m.uiTaskCh:
        case <-time.After(2 * time.Second):
            t.Fatal("refresh was not delivered")
        }
        if m.promptQuota.loaded {
            t.Fatal("worker mutated owner state")
        }
        apply()
        if !m.promptQuota.loaded || len(m.promptQuota.snapshot.Reports) != 1 {
            t.Fatal("owner did not publish quota")
        }
        m.refreshPromptQuota(ctx)
        select {
        case apply = <-m.uiTaskCh:
        case <-time.After(2 * time.Second):
            t.Fatal("refresh was not delivered")
        }
        replacement := &promptQuotaState{}
        m.promptQuota = replacement
        apply()
        if replacement.loaded {
            t.Fatal("old session overwrote replacement quota")
        }
        cancel()
        m.backgroundTasks.Wait()
    }
    
    func TestPromptQuotaCancellationJoinsFetch(t *testing.T) {
        ctx, cancel := context.WithCancel(t.Context())
        started := make(chan struct{})
        m := &InteractiveMode{uiTaskCh: make(chan func(), 8)}
        m.opts.UsageService = promptQuotaTestService{fetch: func(ctx context.Context) (ai.UsageSnapshot, error) {
            close(started)
            <-ctx.Done()
            return ai.UsageSnapshot{}, errors.New("cancelled")
        }}
        m.startPromptQuota(ctx)
        <-started
        cancel()
        done := make(chan struct{})
        go func() { m.backgroundTasks.Wait(); close(done) }()
        select {
        case <-done:
        case <-time.After(2 * time.Second):
            t.Fatal("quota tasks outlived shutdown")
        }
    }
    ✓ • 55ms

## Activity

    $ sed -n '568,588p' vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go; sed -n '655,711p' vendor/github.com/MichaelKinsy/PiG/tui/usage_dashboard.go; sed -n '395,438p' vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go; rg -n 'func StatusColorToken' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; cat vendor/github.com/MichaelKinsy/PiG/tui/AGENTS.md
        case ai.UsageStatusWarning:
            return theme.Fg(string(ThemeColorWarning), UsageGlyphWarning)
        case ai.UsageStatusOK:
            return theme.Fg(string(ThemeColorSuccess), UsageGlyphSuccess)
        }
        return theme.Fg(string(ThemeColorDim), UsageGlyphInfo)
    }
    
    func miniBar(fraction *float64, status ai.UsageStatus, width int) string {
        theme := ActiveTheme()
        if fraction == nil {
            return theme.Fg(string(ThemeColorDim), strings.Repeat("·", width))
        }
        clamped := math.Min(math.Max(*fraction, 0), 1)
        filled := int(math.Round(clamped * float64(width)))
        return theme.Fg(StatusColorToken(status), strings.Repeat("█", filled)) +
            theme.Fg(string(ThemeColorDim), strings.Repeat("░", width-filled))
    }
    
    // renderCardLines draws one provider card at the given width.
    func (d *UsageDashboardComponent) renderCardLines(card ProviderCard, width int, labels [][]string, layout usageCardRowLayout) []string {
            for _, line := range widthx.WrapTextWithAnsi(text, max(1, width-2)) {
                lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), line))
            }
        }
    
        if card.Unlimited {
            lines = append(lines, "  "+theme.Fg(string(ThemeColorDim), "no limits"))
            return lines
        }
    
        hidden := len(card.Windows) - CardMaxWindows
        contentWidth := max(1, width-2)
        for index := 0; index < min(len(card.Windows), CardMaxWindows); index++ {
            window := card.Windows[index]
            labelLines := labels[index]
            label := ""
            if len(labelLines) > 0 {
                label = labelLines[0]
            }
            prefix := ""
            if !layout.stacked {
                prefix = label + strings.Repeat(" ", max(0, layout.labelWidth-widthx.VisibleWidth(label))) + " "
            }
            if layout.stacked {
                for line := 0; line < layout.labelHeights[index]; line++ {
                    if line < len(labelLines) {
                        lines = append(lines, "  "+labelLines[line])
                    }
                }
            }
            if window.Fraction == nil {
                text := window.UsedText
                if text == "" {
                    text = "no data"
                }
                text = theme.Fg(string(ThemeColorDim), text)
                for _, line := range widthx.WrapTextWithAnsi(prefix+text, contentWidth) {
                    lines = append(lines, "  "+line)
                }
                continue
            }
            freePct := max(0, int(math.Round((1-*window.Fraction)*100)))
            pctText := theme.Fg(StatusColorToken(window.Status), padLeft(strconv.Itoa(freePct)+"%", 5))
            resetPlain := ""
            if window.ResetMs != nil {
                resetPlain = FormatDuration(*window.ResetMs)
            }
            resetText := ""
            if layout.resetWidth > 0 {
                resetText = " " + theme.Fg(string(ThemeColorDim), padLeft(resetPlain, layout.resetWidth))
            }
            for _, line := range widthx.WrapTextWithAnsi(
                prefix+miniBar(window.Fraction, window.Status, layout.barWidth)+pctText+resetText, contentWidth) {
                lines = append(lines, "  "+line)
            }
        }
        if hidden > 0 {
        if maxReset-minReset > 60000 {
            return verb + " in " + FormatDuration(minReset) + "–" + FormatDuration(maxReset)
        }
        return verb + " in " + FormatDuration(minReset)
    }
    
    // renderUsageBar draws one account's quota bar.
    func renderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
        if limit.Amount.Used != nil && IsUsedOnlyAbsoluteAmount(limit) {
            used := ""
            if limit.Amount.Unit == ai.UsageUnitUSD {
                used = "$" + strconv.FormatFloat(*limit.Amount.Used, 'f', 2, 64)
            } else {
                used = strconv.FormatFloat(math.Round(*limit.Amount.Used*100)/100, 'f', -1, 64) + " " + string(limit.Amount.Unit)
            }
            return theme.Fg(string(ThemeColorDim), widthx.TruncateToWidth(used+" used", barWidth, "…", false))
        }
        fraction, ok := ai.UsageUsedFraction(limit)
        if !ok {
            return theme.Fg(string(ThemeColorDim), strings.Repeat("·", barWidth))
        }
        clamped := math.Min(math.Max(fraction, 0), 1)
        exact := clamped * float64(barWidth)
        fullCells := int(math.Floor(exact))
        remainder := exact - float64(fullCells)
        partial := ""
        switch {
        case remainder >= 2.0/3.0:
            partial = "▓"
        case remainder >= 1.0/3.0:
            partial = "▒"
        }
        leading := strings.Repeat("█", fullCells) + partial
        filled := fullCells
        if partial != "" {
            filled++
        }
        empty := strings.Repeat("░", max(0, barWidth-filled))
        return theme.Fg(StatusColorToken(limit.Status), leading) + theme.Fg(string(ThemeColorDim), empty)
    }
    
    // usageAccountBlock is one width-fitting slice of a bucket's account columns.
    type usageAccountBlock struct {
        start       int
    656:func StatusColorToken(status ai.UsageStatus) string {
    # TUI reliability
    
    ## Read first
    
    - `../docs/project/CONTEXT.md`
    - `../.upstream/current/packages/tui/src/tui-main-screen.ts`
    - `../.upstream/current/packages/tui/src/components/editor.ts`
    - `main_screen_upstream_parity_test.go` after confirming its currently documented Pi 0.84 model against the pinned source
    
    ## Contract
    
    The Main Screen preserves scrollback. The Editor changes only the required visible rows. The Viewport follows Pi's state machine. Frame work does not scale with unchanged Session history.
    
    ## Failure modes
    
    | Failure | Detection | Required result |
    |---|---|---|
    | Ordinary editor shrink repaints the viewport | Delete one newline or wrap row | Use a local differential update |
    | The viewport snaps backward | Compare viewport origin before and after shrink | Match Pi |
    | A frame replays transcript history | Compare short and large Sessions | Keep frame work bounded |
    | A stale or duplicate row remains | Apply bytes to the terminal simulator | Match Pi's final screen |
    | Scrollback is cleared | Detect `CSI 2J` and `CSI 3J` | Emit neither for an ordinary edit |
    | The cursor homes | Detect `CSI H` | Use Pi-compatible relative movement |
    | Wrapped input enters history early | Move Up within wrapped input | Keep Pi's cursor and history state |
    | Resize corrupts layout | Change width and height with ANSI, wide text, and images | Match Pi's final screen |
    | A hidden-row change cannot be repaired locally | Change content above the viewport | Use Pi's recovery render |
    
    ## Evidence
    
    Define the terminal scenario before renderer code. Compare escaped Pi and PiG output and terminal-simulator state. Retain inputs, dimensions, bytes, screen state, and mutation failure. Use isolated tests for width, row, cursor, and viewport calculations. Profile editor shrink, streaming, overlays, and large transcripts. Run `go test -race ./tui` and the owning parity scenarios.
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go');s=p.read_text().replace('"github.com/MichaelKinsy/PiG/tui"','"github.com/MichaelKinsy/PiG/tui"\n "github.com/MichaelKinsy/PiG/tui/widthx"');needle='\t// A freshly fetched weekly bottleneck replaces the short-window bar.';s=s.replace(needle,'''\tminimumWidth := widthx.VisibleWidth(label) - 24 + 12
        	if compact := m.promptQuotaLabel(minimumWidth); widthx.VisibleWidth(compact) != minimumWidth {
        		t.Fatalf("bar did not shrink to 12 cells: %q", compact)
        	}
        	if compact := m.promptQuotaLabel(minimumWidth - 1); compact != "" {
        		t.Fatalf("bar rendered without enough room for its fields: %q", compact)
        	}
        
        '''+needle);p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go');s=p.read_text().replace('5h [■■□□□] 40% left · reset 2h','5h ███████░░░░░ 40% left · reset 2h');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
        sed -n '76,145p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        state := m.promptQuota
        if state == nil {
            return ""
        }
        status := "loading…"
        if state.loaded {
            status = "quota unavailable"
            windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
            if len(windows) > 0 {
                window := windows[0]
                for _, candidate := range windows[1:] {
                    if candidate.used > window.used {
                        window = candidate
                    }
                }
                remaining := 1 - window.used
                reset := "reset unknown"
                if window.resetsAt > 0 {
                    delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                    minutes := int(math.Ceil(delta.Minutes()))
                    if minutes < 60 {
                        reset = fmt.Sprintf("reset %dm", minutes)
                    } else if minutes < 24*60 {
                        reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                    } else {
                        reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                    }
                }
                suffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)
                partial := state.failed
                for _, account := range state.snapshot.UnavailableAccounts {
                    partial = partial || account.Provider == provider
                }
                if partial {
                    suffix += " (partial)"
                }
                prefix := window.label + " "
                barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
                if barWidth < 12 {
                    return ""
                }
                quotaStatus := ai.UsageStatusOK
                if window.used >= 1 {
                    quotaStatus = ai.UsageStatusExhausted
                } else if window.used >= .8 {
                    quotaStatus = ai.UsageStatusWarning
                }
                limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
                status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
            }
        }
        if widthx.VisibleWidth(status) <= maxWidth {
            return status
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    ✓ • 66ms

## Activity

    $ git diff -- vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go; rg -n 'renderUsageBar|RenderUsageBar' vendor/github.com/MichaelKinsy/PiG/tui vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota*; git status --short
    diff --git a/vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go b/vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go
    index b4c0dea..2c20da0 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go
    @@ -398,8 +398,8 @@ func resolveUsageResetRange(limits []ai.UsageLimit, nowMs int64) string {
         return verb + " in " + FormatDuration(minReset)
     }
    
    -// renderUsageBar draws one account's quota bar.
    -func renderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
    +// RenderUsageBar draws one account's quota bar.
    +func RenderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
         if limit.Amount.Used != nil && IsUsedOnlyAbsoluteAmount(limit) {
             used := ""
             if limit.Amount.Unit == ai.UsageUnitUSD {
    @@ -723,7 +723,7 @@ func RenderUsageReports(options UsageReportOptions) string {
                                     formatUsagePercentFree((1-math.Min(math.Max(fraction, 0), 1))*100))
                             }
                             lines = append(lines, strings.TrimRight(
    -                            "  "+renderUsageBar(theme, account.limit, block.barWidth)+suffix, " "))
    +                            "  "+RenderUsageBar(theme, account.limit, block.barWidth)+suffix, " "))
                         }
                         continue
                     }
    @@ -732,7 +732,7 @@ func RenderUsageReports(options UsageReportOptions) string {
                     lines = append(lines, strings.TrimRight("  "+strings.Join(headers, " "), " "))
                     bars := make([]string, 0, len(blockAccounts))
                     for _, account := range blockAccounts {
    -                    bars = append(bars, padUsageColumn(renderUsageBar(theme, account.limit, block.barWidth), block.columnWidth))
    +                    bars = append(bars, padUsageColumn(RenderUsageBar(theme, account.limit, block.barWidth), block.columnWidth))
                     }
                     barLine := "  " + strings.Join(bars, " ")
                     if blockIndex == 0 && !wrapped && amountText != "" {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go:38:    wantBar := tui.RenderUsageBar(tui.ActiveTheme(), ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: new(.5)}, Status: ai.UsageStatusOK}, 24)
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go:130:            status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:401:// RenderUsageBar draws one account's quota bar.
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:402:func RenderUsageBar(theme *Theme, limit ai.UsageLimit, barWidth int) string {
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:726:                            "  "+RenderUsageBar(theme, account.limit, block.barWidth)+suffix, " "))
    vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go:735:                    bars = append(bars, padUsageColumn(RenderUsageBar(theme, account.limit, block.barWidth), block.columnWidth))
     M README.md
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
     M vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
    ?? vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'tui/usage_report.go';s=p.read_text().replace('renderUsageBar','RenderUsageBar');p.write_text(s)
        p=root/'internal/codingagent/interactive_prompt_quota.go';s=p.read_text().replace('\n\t"strings"','').replace('\n\tprefix := tui.SanitizeDisplayLine(tui.FormatProviderName(provider))\n\tmodel := tui.SanitizeDisplayLine(m.opts.Model.ID)','').replace('\n\t\t\tfilled := int(math.Round(remaining * 5))','').replace('\n\t\t\tstatus = fmt.Sprintf("%s [%s%s] %.0f%% left · %s", window.label, strings.Repeat("■", filled), strings.Repeat("□", 5-filled), remaining*100, reset)','\n\t\t\tsuffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)').replace('\n\t\t\t\tstatus += " (partial)"','\n\t\t\t\tsuffix += " (partial)"')
        s=s.replace('\n\t\t\tif partial {\n\t\t\t\tsuffix += " (partial)"\n\t\t\t}', '''
        			if partial {
        				suffix += " (partial)"
        			}
        			prefix := window.label + " "
        			barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
        			if barWidth < 12 {
        				return ""
        			}
        			quotaStatus := ai.UsageStatusOK
        			if window.used >= 1 {
        				quotaStatus = ai.UsageStatusExhausted
        			} else if window.used >= .8 {
        				quotaStatus = ai.UsageStatusWarning
        			}
        			limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
        			status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix''')
        a=s.index('\t// Omit the model name first');b=s.index('\n\treturn ""\n}',a)
        s=s[:a]+'''\tif widthx.VisibleWidth(status) <= maxWidth {
        		return status
        	}'''+s[b:];p.write_text(s)
        p=root/'internal/codingagent/interactive_prompt_quota_test.go';s=p.read_text().replace('strings.Count(label, "[") != 1','strings.Count(label, "█") == 0').replace('!strings.Contains(label, "different") || !strings.Contains(label, "0% left")','strings.Contains(label, "different") || !strings.Contains(label, "monthly") || !strings.Contains(label, "0% left")').replace('strings.Contains(label, "[")','strings.ContainsAny(label, "█░")')
        s=s.replace('\tlabel := m.promptQuotaLabel(200)','''\tlabel := m.promptQuotaLabel(200)
        	if strings.Contains(label, "luna-6") || strings.Contains(strings.ToLower(label), "codex") {
        		t.Fatalf("quota label included model/provider: %q", label)
        	}
        	wantBar := tui.RenderUsageBar(tui.ActiveTheme(), ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: new(.5)}, Status: ai.UsageStatusOK}, 24)
        	if !strings.Contains(label, wantBar) {
        		t.Fatalf("quota does not use the usage board bar: %q", label)
        	}''',1)
        p.write_text(s)
        p=Path('README.md');s=p.read_text().replace('The label shortens or hides when the border has insufficient space.', 'The label shows only the window, the usage board’s detailed bar, remaining percentage, and next reset. The bar fits 12–24 cells and hides when the border has insufficient space.');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/tui github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestPromptQuota|TestUsage' -count=1
    ok      github.com/MichaelKinsy/PiG/tui    0.143s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.134s
    ✓ • 47.01s

## Assistant

The label now shows only the window, bar, remaining percentage, and reset countdown. It uses the usage board’s colored `█▓▒░` renderer, with 12–24 cells depending on available space. Tests and the build are running.

## Activity

    $ git diff --check; ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -7
        PID     ELAPSED %CPU COMMAND
    3204247       00:02 87.0 compile
    3204308       00:00 78.2 compile
    3203876       00:14 54.2 compile
    3204338       00:00 50.0 ps
     188473    23:13:42 26.8 chrome-headless
     188463    23:13:42 18.3 chrome-headless
    ✓ • 85ms

## Activity

    $ make build
        git diff --check
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 1m 34s

## Activity

    $ python - <<'PY'
        import os, subprocess, tempfile
        with tempfile.TemporaryDirectory(prefix='ompig-quota-style-') as profile:
         env=os.environ.copy()
         for key in ('PIG_CODING_AGENT_DIR','PI_CODING_AGENT_DIR','OMPIG_SLIM_CODING_AGENT_DIR'):
          env[key]=profile+'/agent'
         for key in ('PIG_HOME','PI_HOME'):
          env[key]=profile+'/library'
         env.update(PI_SKIP_VERSION_CHECK='1',PI_TELEMETRY='0')
         raise SystemExit(subprocess.call(['go','test','-race','-mod=vendor','github.com/MichaelKinsy/PiG/tui','github.com/MichaelKinsy/PiG/internal/codingagent'],env=env))
        PY
    ok      github.com/MichaelKinsy/PiG/tui    1.649s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    1.238s
    ✓ • 1m 46s

## Assistant

Done. The row now shows only the window, bar, remaining percentage, and reset countdown.

The bar uses `/usage`’s colored shading with 12–24 cells, adapting to available space.

Race tests and build passed.

## User

if not fetchable then just dont show

## Assistant

I’ll hide the quota display while it’s loading or when quota data can’t be fetched.

## Activity

    $ sed -n '76,146p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; sed -n '20,100p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go; rg -n 'quota unavailable|loading…|prompt.*quota' README.md vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota*
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        state := m.promptQuota
        if state == nil {
            return ""
        }
        status := "loading…"
        if state.loaded {
            status = "quota unavailable"
            windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
            if len(windows) > 0 {
                window := windows[0]
                for _, candidate := range windows[1:] {
                    if candidate.used > window.used {
                        window = candidate
                    }
                }
                remaining := 1 - window.used
                reset := "reset unknown"
                if window.resetsAt > 0 {
                    delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                    minutes := int(math.Ceil(delta.Minutes()))
                    if minutes < 60 {
                        reset = fmt.Sprintf("reset %dm", minutes)
                    } else if minutes < 24*60 {
                        reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                    } else {
                        reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                    }
                }
                suffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)
                partial := state.failed
                for _, account := range state.snapshot.UnavailableAccounts {
                    partial = partial || account.Provider == provider
                }
                if partial {
                    suffix += " (partial)"
                }
                prefix := window.label + " "
                barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
                if barWidth < 12 {
                    return ""
                }
                quotaStatus := ai.UsageStatusOK
                if window.used >= 1 {
                    quotaStatus = ai.UsageStatusExhausted
                } else if window.used >= .8 {
                    quotaStatus = ai.UsageStatusWarning
                }
                limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
                status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
            }
        }
        if widthx.VisibleWidth(status) <= maxWidth {
            return status
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
    
    func TestPromptQuotaTotalsAndEssentialWindow(t *testing.T) {
        later := time.Now().Add(4 * time.Hour).UnixMilli()
        sooner := time.Now().Add(2 * time.Hour).UnixMilli()
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
            {Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .8, sooner), promptTestLimit("7d", .2, later)}},
            {Provider: "openai-codex", Limits: []ai.UsageLimit{promptTestLimit("5h", .2, later), promptTestLimit("7d", .6, later)}},
            {Provider: "other", Limits: []ai.UsageLimit{promptTestLimit("monthly", 1, sooner)}},
        }}
        windows := promptQuotaWindows(snapshot, "openai-codex", "luna-6")
        if len(windows) != 2 || windows[0].used != .5 || windows[0].resetsAt != sooner {
            t.Fatalf("wrong account total or next reset: %+v", windows)
        }
        m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "luna-6", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
        label := m.promptQuotaLabel(200)
        if strings.Contains(label, "luna-6") || strings.Contains(strings.ToLower(label), "codex") {
            t.Fatalf("quota label included model/provider: %q", label)
        }
        wantBar := tui.RenderUsageBar(tui.ActiveTheme(), ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: new(.5)}, Status: ai.UsageStatusOK}, 24)
        if !strings.Contains(label, wantBar) {
            t.Fatalf("quota does not use the usage board bar: %q", label)
        }
        if strings.Count(label, "█") == 0 || !strings.Contains(label, "5h") || !strings.Contains(label, "50% left") || !strings.Contains(label, "reset 2h") || strings.Contains(label, "7d") {
            t.Fatalf("wrong essential quota: %q", label)
        }
    
        minimumWidth := widthx.VisibleWidth(label) - 24 + 12
        if compact := m.promptQuotaLabel(minimumWidth); widthx.VisibleWidth(compact) != minimumWidth {
            t.Fatalf("bar did not shrink to 12 cells: %q", compact)
        }
        if compact := m.promptQuotaLabel(minimumWidth - 1); compact != "" {
            t.Fatalf("bar rendered without enough room for its fields: %q", compact)
        }
    
        // A freshly fetched weekly bottleneck replaces the short-window bar.
        m.promptQuota.snapshot.Reports[0].Limits[1].Amount.UsedFraction = new(1.0)
        m.promptQuota.snapshot.Reports[1].Limits[1].Amount.UsedFraction = new(.8)
        label = m.promptQuotaLabel(200)
        if strings.Count(label, "█") == 0 || !strings.Contains(label, "7d") || !strings.Contains(label, "10% left") || strings.Contains(label, "5h") {
            t.Fatalf("did not select the new limiting window: %q", label)
        }
        m.promptQuota.snapshot.UnavailableAccounts = []ai.UsageUnavailableAccount{{Provider: "openai-codex"}}
        if label = m.promptQuotaLabel(200); !strings.Contains(label, "partial") {
            t.Fatalf("incomplete totals were presented as complete: %q", label)
        }
        m.editor = tui.NewEditor()
        m.editor.TopRightStatus = m.promptQuotaLabel
        if row := m.editor.Render(200)[0]; !strings.Contains(row, "10% left") {
            t.Fatalf("quota did not reach the prompt border: %q", row)
        }
    
        // Select a different model and provider without waiting for another network fetch.
        m.opts.Model = &ai.Model{ID: "different", ProviderMeta: ai.ProviderMetadata{ProviderID: "other"}}
        if label = m.promptQuotaLabel(200); strings.Contains(label, "different") || !strings.Contains(label, "monthly") || !strings.Contains(label, "0% left") {
            t.Fatalf("model switch retained the old quota: %q", label)
        }
        m.opts.Model.ProviderMeta.ProviderID = "unsupported"
        if label = m.promptQuotaLabel(200); !strings.Contains(label, "unavailable") || strings.ContainsAny(label, "█░") {
            t.Fatalf("missing quota was rendered as capacity: %q", label)
        }
    }
    
    func TestPromptQuotaWeightsCapacityAndFiltersModels(t *testing.T) {
        a, b := promptTestLimit("monthly", 1, 0), promptTestLimit("monthly", 0, 0)
        a.Amount.Limit = new(100.0)
        b.Amount.Limit = new(900.0)
        modelOnly := promptTestLimit("special", 1, 0)
        modelOnly.Scope.ModelID = "another-model"
        invalid := promptTestLimit("invalid", math.NaN(), 0)
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{{Provider: "test", Limits: []ai.UsageLimit{a, modelOnly, invalid}}, {Provider: "test", Limits: []ai.UsageLimit{b}}}}
        windows := promptQuotaWindows(snapshot, "test", "selected-model")
        if len(windows) != 1 || math.Abs(windows[0].used-.1) > 1e-9 {
            t.Fatalf("wrong weighted total or model filtering: %+v", windows)
        }
    }
    
    type promptQuotaTestService struct {
        fetch func(context.Context) (ai.UsageSnapshot, error)
    }
    
    func (s promptQuotaTestService) FetchSnapshot(ctx context.Context, force bool) (ai.UsageSnapshot, error) {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go:14:// Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go:86:    status := "loading…"
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go:88:        status = "quota unavailable"
    README.md:156:The prompt’s top-right border shows one live quota bar for the selected model’s provider, aggregated across accounts. It chooses the quota window with the least remaining capacity and shows the next account reset countdown. Quotas refresh every minute in the background; unavailable accounts mark the total as partial. The label shows only the window, the usage board’s detailed bar, remaining percentage, and next reset. The bar fits 12–24 cells and hides when the border has insufficient space.
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go');s=p.read_text().replace('if label = m.promptQuotaLabel(200); !strings.Contains(label, "unavailable") || strings.ContainsAny(label, "█░") {\n\t\tt.Fatalf("missing quota was rendered as capacity: %q", label)\n\t}', '''if label = m.promptQuotaLabel(200); label != "" {
        		t.Fatalf("missing quota should be hidden: %q", label)
        	}
        	if row := m.editor.Render(200)[0]; strings.Contains(row, "%") || strings.Contains(row, "quota") {
        		t.Fatalf("unavailable quota was not cleared from the border: %q", row)
        	}
        	m.opts.Model.ProviderMeta.ProviderID = "openai-codex"
        	m.promptQuota.loaded = false
        	if label = m.promptQuotaLabel(200); label != "" {
        		t.Fatalf("loading quota should be hidden: %q", label)
        	}
        	m.promptQuota.loaded, m.promptQuota.failed = true, true
        	if label = m.promptQuotaLabel(200); label != "" {
        		t.Fatalf("failed quota fetch should be hidden: %q", label)
        	}''');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        go test -mod=vendor github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuotaTotalsAndEssentialWindow$' -count=1
    --- FAIL: TestPromptQuotaTotalsAndEssentialWindow (0.00s)
        interactive_prompt_quota_test.go:78: missing quota should be hidden: "quota unavailable"
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/internal/codingagent    0.051s
    FAIL
    ✗ (1) • 16.76s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go');s=p.read_text().replace('if state == nil {\n\t\treturn ""\n\t}\n\tstatus := "loading…"', 'if state == nil || !state.loaded || state.failed {\n\t\treturn ""\n\t}\n\tstatus := "loading…"');a=s.index('\tstatus := "loading…"');b=s.index('\n\tif widthx.VisibleWidth(status)',a);lines=s[a:b].splitlines();s=s[:a]+'\tstatus := ""\n'+'\n'.join(line[1:] if line.startswith('\t') else line for line in lines[3:-1])+s[b:];p.write_text(s)
        p=Path('README.md');s=p.read_text().replace('The bar fits 12–24 cells and hides when the border has insufficient space.', 'The bar fits 12–24 cells and hides when the border has insufficient space or quota data cannot be fetched. Initial loading also leaves the border empty.');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
        git diff --check
    ✓ • 0ms

## Activity

    $ sed -n '76,144p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; git diff --check
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        state := m.promptQuota
        if state == nil || !state.loaded || state.failed {
            return ""
        }
        status := ""
        windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
        if len(windows) > 0 {
            window := windows[0]
            for _, candidate := range windows[1:] {
                if candidate.used > window.used {
                    window = candidate
                }
            }
            remaining := 1 - window.used
            reset := "reset unknown"
            if window.resetsAt > 0 {
                delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                minutes := int(math.Ceil(delta.Minutes()))
                if minutes < 60 {
                    reset = fmt.Sprintf("reset %dm", minutes)
                } else if minutes < 24*60 {
                    reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                } else {
                    reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                }
            }
            suffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)
            partial := state.failed
            for _, account := range state.snapshot.UnavailableAccounts {
                partial = partial || account.Provider == provider
            }
            if partial {
                suffix += " (partial)"
            }
            prefix := window.label + " "
            barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
            if barWidth < 12 {
                return ""
            }
            quotaStatus := ai.UsageStatusOK
            if window.used >= 1 {
                quotaStatus = ai.UsageStatusExhausted
            } else if window.used >= .8 {
                quotaStatus = ai.UsageStatusWarning
            }
            limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
            status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
        }
        if widthx.VisibleWidth(status) <= maxWidth {
            return status
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
    ✓ • 14ms

## Activity

    $ go test -mod=vendor github.com/MichaelKinsy/PiG/tui github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/tui    0.124s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.175s
    ✓ • 29.16s

## Activity

    $ make build
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 44.00s

## Assistant

Done. The quota display stays hidden during initial loading, when fetching fails, or when no quota is available. Tests and build passed.

## User

make it priotize better, for example, when using luna 6, it should priotize use from the free tier account instead of the plus tier account

## Assistant

I’ll trace account selection and update it so a compatible free-tier account is preferred over a Plus account when using Luna 6.

## Activity

    $ git status --short; rg -n -i 'free.*tier|tier.*free|plan.*priority|priority.*plan|plan.*rank|prefer.*free|luna.?6' vendor/github.com/MichaelKinsy/PiG/ai vendor/github.com/MichaelKinsy/PiG/internal/codingagent vendor/github.com/MichaelKinsy/PiG/coding | head -100; cat vendor/github.com/MichaelKinsy/PiG/ai/AGENTS.md; cat vendor/github.com/MichaelKinsy/PiG/coding/AGENTS.md
     M README.md
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_input.go
     M vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor.go
     M vendor/github.com/MichaelKinsy/PiG/tui/editor_status.go
     M vendor/github.com/MichaelKinsy/PiG/tui/usage_report.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ?? vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
    ?? vendor/github.com/MichaelKinsy/PiG/tui/editor_quota_test.go
    vendor/github.com/MichaelKinsy/PiG/ai/assistant_retry.go:19:    // OpenCode Go/free-tier limits returned as 429 JSON error types by
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go:29:    windows := promptQuotaWindows(snapshot, "openai-codex", "luna-6")
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go:33:    m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "luna-6", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go:35:    if strings.Contains(label, "luna-6") || strings.Contains(strings.ToLower(label), "codex") {
    vendor/github.com/MichaelKinsy/PiG/ai/oauth_anthropic.go:125:    // The preferred port can be forwarded into containers or over SSH. Anthropic accepts any loopback port, so login falls back to a free port when the preferred one cannot be bound (anthropic.ts, #10571); without a callback server the manual prompt is the only way to finish.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:47:    // googleCCAOnboardTimeoutMs bounds the Antigravity free-tier onboarding.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:54:    googleCCATierFree     = "free-tier"
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:366:// and onboards the account's tier; Antigravity provisions its free tier.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:427:    if tierID != googleCCATierFree && envProjectID == "" {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:441:    if tierID != googleCCATierFree && envProjectID != "" {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:507:// googleAntigravityProject ports googleAntigravityProjectHook's provisioning: loadCodeAssist, the free-tier
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:569:    if err := googleAssertFreeTierEligible(initial); err != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:574:            onProgress("Provisioning the Antigravity free tier...")
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:612:// googleAssertFreeTierEligible reports the account's free-tier ineligibility, with its validation URL.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:613:func googleAssertFreeTierEligible(payload googleAntigravityLoadCodeAssist) error {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:615:        if tier.ID == googleCCATierFree {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:620:        if tier.TierID != googleCCATierFree || tier.ReasonMessage == "" {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:632:// googleAntigravityOnboard provisions the Antigravity free tier and waits for its operation.
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:648:        "tierId":   googleCCATierFree,
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_auth.go:747:// googleDefaultTier is getDefaultTier: the marked default, else the legacy tier, else free.
    vendor/github.com/MichaelKinsy/PiG/ai/faux.go:278:    // Cost is the per-million-token price; nil is a free model. Its Tiers are ignored (upstream's cost has none).
    # AI runtime reliability
    
    ## Read first
    
    - `../docs/project/CONTEXT.md`
    - `../docs/typescript-to-go-porting.md`
    - `../.upstream/current/packages/ai/src/types.ts`
    - `../.upstream/current/packages/ai/src/utils/event-stream.ts`
    - the matching provider under `../.upstream/current/packages/ai/src/api/`
    
    ## Contract
    
    A Provider receives a normalized Transcript. Public Message, content, Usage, and event shapes match pinned Pi. An Event Stream preserves event order and has one terminal result. Provider setup can return an immediate error. Runtime, provider, authentication, and cancellation failures after stream creation terminate the stream.
    
    ## Failure modes
    
    | Failure | Detection | Required result |
    |---|---|---|
    | Invalid JSON enters trusted provider code | Supply a non-JSON value or non-finite number | Reject it at the public boundary |
    | Result depends on event consumption | Request Result without iteration after more than the former queue bound | Result completes |
    | Iteration stops while waiting | Cancel the iterator context | No waiter or goroutine remains |
    | Cancellation races with termination | Run concurrent Push, iteration, and Result under the race detector | One terminal result |
    | A goroutine that drives a coroutine turn takes the executor's lock only after the body switched back to it, while another turn's body or reaction runs | `TestContinuationLateDriverDoesNotContinueAnotherTurnsDrain` holds the first body between its switch and its driver, runs a second turn into a reaction that does not return, then releases the driver | The late driver continues only a drain its own body holds; no body runs beside a reaction; every turn finishes and the executor is never left draining with no loop |
    | A provider emits an invalid sequence | Exercise setup, start, block, delta, end, and terminal paths | Match Pi event order |
    | Replay metadata crosses provider or model identity | Replay signed content through another model | Drop or transform it as Pi does |
    | Consumed events remain retained | Drain a long stream | Release consumed queue entries |
    | A transport consumer stalls | Stop reading subprocess frames | Apply bounded transport backpressure and cancellation |
    | A provider receives a Transcript its API rejects (a tool result without its tool call, anything between a call and its results, a duplicate call id, a role order the API refuses) | Run any test, or any binary built with `-tags requestvalidity` | `internal/requestvalidity` panics at the send seam: the wire body of each JSON API, the Converse input, and the transcript every provider and faux provider receives. `PIG_REQUEST_VALIDITY_REPORT=<file>` records violations without panicking. A test that sends an invalid request on purpose uses `invalidRequest(t)` and states why |
    | A request build copies the whole transcript (cost grows with every turn) | Convert a 100-turn and an 800-turn history for every API and compare the allocations per turn | A provider reads the transcript through `TranscriptContext.borrowed` and never modifies the slice, a message or anything a message references; `Messages` stays the copying accessor for callers that keep or change what they get. `TestRequestBuildLeavesTheTranscriptUnchanged` compares the transcript before and after the request build of one catalog model of every request shape, `TestRequestBuildDoesNotCopyWhatItDoesNotSend` fails when a build copies tool-result details it never sends, and `TestRequestBuildAllocationsAreLinearInTheHistory` bounds the cost per turn |
    
    ## Evidence
    
    Write failure cases before provider code. Use isolated tests for normalization, JSON, payload conversion, event order, and cancellation. Use a real Model Runtime path for `complete`, `stream`, and `streamSimple`. Cover every provider family. Run `go test -race ./ai`. Retain payload and event-sequence evidence. Mutation-check one event discriminator and one provider content field. `test/pipeline-diff` sends seeded random histories through Pi's convertToLlm, transformMessages and API converters and through PiG's, and compares the requests on the wire; every contract rule comes from the provider's documentation and Pi's `transform-messages.ts`, cited in `internal/requestvalidity/wire.go`.
    # Session runtime reliability
    
    ## Read first
    
    - `../docs/project/CONTEXT.md`
    - `../.upstream/current/packages/coding-agent/src/core/agent-session.ts`
    - `../.upstream/current/packages/coding-agent/src/core/model-runtime.ts`
    - `../docs/typescript-to-go-porting.md`
    
    ## Contract
    
    One Session owns one Model Runtime. TUI, print, JSON, and RPC use that runtime. Session replacement cancels and drains prior work. Persistence, paging, compaction, and reconciliation preserve order and identity.
    
    ## Failure modes
    
    | Failure | Detection | Required result |
    |---|---|---|
    | A mode uses a different model path | Run one request in every mode | Match request, events, and result |
    | Session replacement leaks prior work | Replace an active Session | Cancel and drain the prior runtime |
    | Paging loses or duplicates an append | Append during page transfer | Deliver each entry once, in order |
    | Resume scans or copies full history on input | Measure editor and command paths on a large Session | Keep work bounded |
    | Compaction races with queued input | Queue input during compaction | Preserve Pi ordering |
    | Poisoned history breaks a later turn | Load aborted, errored, empty, and orphaned entries | Recover or fail at the owning boundary |
    | Shutdown leaves work or files open | Cancel during provider, tool, and extension activity | Drain owned work and close resources |
    
    ## Evidence
    
    Write state-transition and ordering cases first. Use isolated tests for parsers, paging, and reconciliation. Use complete mode paths for runtime ownership, replacement, compaction, and resume. Test ordinary and representative large Sessions. Record latency, allocations, memory, file reads, goroutines, and cancellation. Retain Session fixture identity and run evidence.
    ✓ • 0ms

## Activity

    $ rg -n -i 'affinity|eligib|plan.?type|plan.?tier|account.*rank|sort.*account' vendor/github.com/MichaelKinsy/PiG/ai/account* vendor/github.com/MichaelKinsy/PiG/coding/*account* vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*account*; rg --files vendor/github.com/MichaelKinsy/PiG | rg 'account.*(rout|select)|rout.*account|codex.*(account|plan)'
    rg: vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*account*: No such file or directory (os error 2)
    vendor/github.com/MichaelKinsy/PiG/ai/account_request.go:43:// that named an account keeps it. A merged API key already outranks the stored accounts.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:5:// account handoff event, and the affinity reset an explicit login or account change performs.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:36:// runtime's request affinity, so the choice takes effect on the next request. False when the
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:48:        runtime.clearAccountAffinity(providerID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:57:// this runtime's request affinity for that provider.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:68:        runtime.clearAccountAffinity(providerID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:138:// clearAccountAffinity drops this runtime's successful-request affinity for a provider.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:139:func (runtime *ModelRuntime) clearAccountAffinity(providerID string) {
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:184:// usage board's in-use marker. It reads the runtime's affinity only; present is false when the
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:3:// Behavioural tests of the request-local account router: model-plan eligibility, transient
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:4:// non-rotation, the pre/post-content fallback boundary, and per-model session affinity.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:140:    payload := fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_plan_type":%q}}`, plan)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:157:        t.Fatalf("free plan must be ineligible for gpt-5.6-sol, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:161:        t.Fatalf("pro plan must be eligible for gpt-5.6-sol, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:165:        t.Fatalf("plus plan must be eligible for gpt-5.6-luna, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:168:        t.Fatalf("plus plan must be ineligible for a Spark model, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:174:    // An unreadable plan stays probeable rather than eligible.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:180:        t.Fatalf("another provider is always eligible, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:183:        t.Fatalf("an API key is always eligible, got %v", meets)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:314:func TestRouterAffinityPerModel(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:335:    // Another model has no pin: the affinity is per model, not per session.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:337:        t.Fatal("affinity must not leak across models")
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:354:        t.Fatal("ClearProvider must drop the provider's affinity")
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:378:func TestRouterNoEligibleAccountError(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:389:        t.Fatal("an ineligible account must fail before any request")
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:395:        t.Fatalf("no request may be sent for an ineligible account, tried %v", tried)
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:20:// Codex plan tiers, matched against the normalized chatgpt_plan_type claim exactly as
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:29:// codexPlanSeparators folds the space and hyphen runs of a plan tier into one underscore, as
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:52:// tier): the account stays probeable, ranked after every known-eligible account.
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:55:// eligible.
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:88:// codexPlanClaim reads payload["https://api.openai.com/auth"].chatgpt_plan_type from the
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:90:// guarantee from the backend, so every failure reports "unknown" rather than "eligible".
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:112:    planRaw, ok := claims["chatgpt_plan_type"]
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:86:    // affinity is provider → session → model → the account key that session's last success used.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:87:    affinity map[string]map[string]map[string]*string
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:97:    router := &AccountRouter{now: time.Now, affinity: map[string]map[string]map[string]*string{}, localBlocks: map[string]map[accountKeyValue]map[string]int64{}}
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:132:// ClearProvider drops this runtime's successful-request affinity for a provider, so an explicit
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:140:    delete(r.affinity, providerID)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:152:    sessions := r.affinity[providerID]
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:184:    sessions := r.affinity[providerID]
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:187:        r.affinity[providerID] = sessions
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:246:// known-eligible account. It returns nil when the router does not route this request.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:274:    // Explicit selection is a preference, not permission to send an ineligible account; the sort
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:276:    slices.SortStableFunc(accounts, func(left, right *CredentialAccountInfo) int {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:277:        return accountRank(right, pinned, pinnedSet) - accountRank(left, pinned, pinnedSet)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:279:    var eligible, unknown []*CredentialAccountInfo
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:285:            eligible = append(eligible, account)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:300:            eligible = append(eligible, account)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:303:    eligible = append(eligible, unknown...)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:304:    if len(eligible) == 0 {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:305:        return nil, fmt.Errorf("No stored account meets the plan requirement for %s/%s. Sign in with an eligible account.", providerID, modelID)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:307:    return eligible, nil
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:310:// accountRank scores a candidate: the session's pinned account, else the provider's selected
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:312:func accountRank(account *CredentialAccountInfo, pinned *string, pinnedSet bool) int {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:462:// accountUnavailableMessage reports that every eligible account is blocked, naming the soonest
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:470:    return fmt.Sprintf("All eligible accounts for %s/%s are temporarily unavailable.%s", providerID, modelID, reset)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:518:// SetStore replaces the credential store the router routes over and drops the affinity and
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:531:    r.affinity = map[string]map[string]map[string]*string{}
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    ✓ • 0ms

## Activity

    $ cat vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go; sed -n '1,125p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '240,335p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '1,220p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; sed -n '300,380p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; cat vendor/github.com/MichaelKinsy/PiG/ai/account_request.go
    package ai
    
    // Ports packages/ai/src/utils/account-policy.ts: the model entitlements and failure
    // scopes used when routing one request across the stored accounts of a provider.
    
    import (
        "encoding/base64"
        "encoding/json"
        "strconv"
        "strings"
    
        "github.com/MichaelKinsy/PiG/internal/jsstring"
        "github.com/MichaelKinsy/PiG/internal/lazyregexp"
    )
    
    // jsSpaceBody is jsSpaceClass without its brackets, for patterns that add their own
    // characters to the class (JavaScript's \s, which Go's \s does not spell).
    var jsSpaceBody = jsSpaceClass[1 : len(jsSpaceClass)-1]
    
    // Codex plan tiers, matched against the normalized chatgpt_plan_type claim exactly as
    // account-policy.ts does: the tier must sit between the separators, so "pro" does not match
    // "professional" and "go" does not match "gpt".
    var (
        codexProPlan  = lazyregexp.New(`(?:^|_)pro(?:_|$)`)
        codexPaidPlan = lazyregexp.New(`(?:^|_)(?:plus|business|team|enterprise|edu|education|teacher|teachers|health|gov|government)(?:_|$)`)
        codexFreePlan = lazyregexp.New(`(?:^|_)(?:free|go)(?:_|$)`)
    )
    
    // codexPlanSeparators folds the space and hyphen runs of a plan tier into one underscore, as
    // account-policy.ts's `replace(/[\s-]+/g, "_")` does.
    var codexPlanSeparators = lazyregexp.New(`[` + jsSpaceBody + `-]+`)
    
    // codexPaidModels are the catalog models a ChatGPT paid plan is required for. Node-compatible
    // projection of pi-catalog rules/runtime/behavior.kdl (account-policy.ts CODEX_PAID_MODELS).
    var codexPaidModels = map[string]bool{
        "gpt-5.6":          true,
        "gpt-5.6-sol":      true,
        "gpt-5.6-sol-pro":  true,
        "gpt-5.6-luna":     true,
        "gpt-5.6-luna-pro": true,
    }
    
    // AccountPolicyModelID is the canonical bare model identity used by OMP's model-policy
    // blocks: the last path segment, JavaScript-trimmed and lowercased.
    func AccountPolicyModelID(modelID string) string {
        index := strings.LastIndex(modelID, "/")
        return jsstring.ToLower(jsstring.Trim(modelID[index+1:]))
    }
    
    // AccountMeetsModelPlan reports whether a known Codex plan satisfies the catalog requirement
    // for modelID. A nil result means the plan cannot be read (an unparseable token or an unknown
    // tier): the account stays probeable, ranked after every known-eligible account.
    //
    // Only openai-codex OAuth credentials carry a plan; every other provider and every API key is
    // eligible.
    func AccountMeetsModelPlan(provider, modelID string, credential Credential) *bool {
        if provider != "openai-codex" || credential.Type != CredentialOAuth {
            return new(true)
        }
        id := AccountPolicyModelID(modelID)
        requirement := "none"
        switch {
        case strings.Contains(id, "-spark"):
            requirement = "pro"
        case codexPaidModels[id] || strings.HasPrefix(id, "gpt-6.1-terra"):
            requirement = "paid"
        }
        if requirement == "none" {
            return new(true)
        }
        plan, ok := codexPlanClaim(credential.Access)
        if !ok {
            return nil
        }
        switch {
        case plan == "prolite" || plan == "pro_lite":
            return new(requirement == "paid")
        case codexProPlan.MatchString(plan):
            return new(true)
        case codexPaidPlan.MatchString(plan):
            return new(requirement == "paid")
        case codexFreePlan.MatchString(plan):
            return new(false)
        }
        return nil
    }
    
    // codexPlanClaim reads payload["https://api.openai.com/auth"].chatgpt_plan_type from the
    // stored OAuth access token. Stored JWT claims are an external boundary, not an entitlement
    // guarantee from the backend, so every failure reports "unknown" rather than "eligible".
    func codexPlanClaim(access string) (string, bool) {
        parts := strings.Split(access, ".")
        if len(parts) < 2 {
            return "", false
        }
        decoded, err := decodeJWTPayload(parts[1])
        if err != nil {
            return "", false
        }
        var payload map[string]json.RawMessage
        if err := json.Unmarshal(decoded, &payload); err != nil {
            return "", false
        }
        authRaw, ok := payload["https://api.openai.com/auth"]
        if !ok {
            return "", false
        }
        var claims map[string]json.RawMessage
        if err := json.Unmarshal(authRaw, &claims); err != nil {
            return "", false
        }
        planRaw, ok := claims["chatgpt_plan_type"]
        if !ok {
            return "", false
        }
        var planValue any
        if err := json.Unmarshal(planRaw, &planValue); err != nil {
            return "", false
        }
        plan, ok := planValue.(string)
        if !ok {
            return "", false
        }
        normalized := codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")
        return strings.TrimPrefix(normalized, "chatgpt_"), true
    }
    
    // decodeJWTPayload decodes a base64url JWT segment the way atob does after the caller's
    // -/+ and _// mapping: ASCII whitespace is ignored, and missing padding is accepted.
    func decodeJWTPayload(segment string) ([]byte, error) {
        text := strings.Map(func(r rune) rune {
            switch r {
            case ' ', '\t', '\n', '\v', '\f', '\r':
                return -1
            }
            return r
        }, segment)
        text = strings.NewReplacer("-", "+", "_", "/").Replace(text)
        if index := strings.IndexByte(text, '='); index >= 0 {
            text = text[:index]
        }
        return base64.RawStdEncoding.DecodeString(text)
    }
    
    // AccountQuotaScope is OMP's quota scope for one model, keeping Codex chat and Spark meters
    // separate. Providers without a per-model meter use the unscoped ("") block.
    func AccountQuotaScope(provider, modelID string) string {
        if provider != "openai-codex" {
            return ""
        }
        if strings.Contains(AccountPolicyModelID(modelID), "-spark") {
            return "spark"
        }
        return "chat"
    }
    
    // AccountRequestBlockScopes lists the persisted OMP block scopes a request must honor.
    func AccountRequestBlockScopes(provider, modelID string) []string {
        id := AccountPolicyModelID(modelID)
        scopes := []string{"auth", "account-policy", AccountQuotaScope(provider, modelID)}
        if provider == "openai-codex" {
            scopes = append(scopes, "shared")
        }
        if provider == "openai-codex" || provider == "cursor" {
            scopes = append(scopes, "model-policy:"+id)
        }
        return scopes
    }
    
    // MinAccountBlockMs is how long every account-scoped rejection blocks its account at least.
    const MinAccountBlockMs = int64(60_000)
    
    var (
        // accountPolicyDeniedModel matches Codex's "The '<model>' model is not supported when
        // using Codex with a ChatGPT account." rejection.
        accountPolicyDeniedModel = lazyregexp.NewJSIgnoreCase(`\bThe ['"]([^'"\r\n]+)['"] model is not supported when using Codex with a ChatGPT account\.`)
        // accountPolicyCursorDenial and accountPolicyCursorDetail match Cursor's changeable
        // rate-limit wording for a model the account's plan cannot use.
        accountPolicyCursorDenial = lazyregexp.NewJSIgnoreCase(`\bERROR_RATE_LIMITED_CHANGEABLE\b`)
        accountPolicyCursorDetail = lazyregexp.NewJSIgnoreCase(`Named models unavailable|Model unavailable on|Free plans can only use`)
        // accountPolicyDeniedAccount matches provider wording for an account the vendor disabled.
        accountPolicyDeniedAccount = lazyregexp.NewJSIgnoreCase(`cyber_policy|account.*(?:deactivated|suspended)|organization.*(?:restricted|disabled)`)
        // accountPolicyConcurrency matches a transient 429: a sibling account would not help.
        accountPolicyConcurrency = lazyregexp.NewJSIgnoreCase(`too many requests|per.?minute|tokens per min|requests per min|concurren`)
        // accountPolicyAuthFailure matches rejections another account can repair.
        accountPolicyAuthFailure = lazyregexp.NewJSIgnoreCase(`invalidated oauth token|invalid_grant|OAuth refresh (?:failed|lease)|token (?:has been )?(?:expired|revoked)|authentication (?:failed|required)|invalid (?:api key|access token)`)
        // accountPolicyRetryMinutes and accountPolicyRetrySeconds read the provider's own wait out
        // of the message, which account-policy.ts spells case-sensitively.
        accountPolicyRetryMinutes = lazyregexp.New(`try again in[` + jsSpaceBody + `]*~?[` + jsSpaceBody + `]*(\d+(?:\.\d+)?)[` + jsSpaceBody + `]*min`)
        accountPolicyRetrySeconds = lazyregexp.New(`retry (?:after|in)[` + jsSpaceBody + `]*(\d+(?:\.\d+)?)[` + jsSpaceBody + `]*s`)
    )
    
    // accountQuotaMessagePattern classifies error text that means the account is exhausted rather
    // than the request being throttled or the provider failing. It is the full source list
    // (retry.ts NON_RETRYABLE_PROVIDER_LIMIT_ERROR_PATTERN).
    var accountQuotaMessagePattern = buildProviderErrorPattern([]string{
        "GoUsageLimitError",
        "FreeUsageLimitError",
        "Monthly usage limit reached",
        "available balance",
        "insufficient_quota",
        "out of budget",
        `quota (?:exceeded|exhausted)`,
        "billing",
        "subscription_sharing_usage_limit_exceeded",
        "usage_limit_reached",
        "usage_not_included",
        "You have hit your ChatGPT usage limit",
        "out of credits",
        "subscription_sharing_workplace_individual_usage_limit_exceeded",
        `"code"` + jsSpaceClass + `*:` + jsSpaceClass + `*"?1308([^0-9]|$)`,
    })
    
    // IsAccountQuotaErrorMessage reports whether provider error text describes an exhausted account
    // rather than transient throttling. These errors do not clear by waiting, so only another
    // stored account can repair them.
    func IsAccountQuotaErrorMessage(message string) bool {
        return accountQuotaMessagePattern.MatchString(message)
    }
    
    // AccountFailureScope is the block an account rejection earns.
    type AccountFailureScope struct {
        // Scope is the persisted OMP block scope: "auth", "account-policy", a quota scope ("chat",
        // "spark", or "" for every scope), or "model-policy:<model>".
        Scope string
        // Until is the block deadline in epoch milliseconds.
        Until int64
    }
    
    // AccountFailureBlock classifies a failed request as a rejection of the account, or of the
    // model under this account, and reports the block it earns. A nil result means a sibling
    // credential cannot repair the failure (a transient transport failure, a server error, or an
    // invalid request), so the request must not rotate.
    //
    // status is the response status, or 0 when no response arrived. retryAfterMs is the parsed
    // Retry-After delay, or nil.
    func AccountFailureBlock(provider, modelID, message string, status int, retryAfterMs *int64) *AccountFailureScope {
        now := nowMillis()
        id := AccountPolicyModelID(modelID)
        if match := accountPolicyDeniedModel.FindStringSubmatch(message); match != nil {
            if provider != "openai-codex" || AccountPolicyModelID(match[1]) != id {
                return nil
            }
            return &AccountFailureScope{Scope: "model-policy:" + id, Until: now + MinAccountBlockMs}
        }
        if provider == "cursor" && accountPolicyCursorDenial.MatchString(message) && accountPolicyCursorDetail.MatchString(message) {
            return &AccountFailureScope{Scope: "model-policy:" + id, Until: now + MinAccountBlockMs}
        }
        if status == 403 || accountPolicyDeniedAccount.MatchString(message) {
            return &AccountFailureScope{Scope: "account-policy", Until: now + MinAccountBlockMs}
        }
        if IsAccountQuotaErrorMessage(message) || (status == 429 && !accountPolicyConcurrency.MatchString(message)) {
            delay := MinAccountBlockMs
            if retryAfterMs != nil && *retryAfterMs > delay {
                delay = *retryAfterMs
            }
            if minutes := accountPolicyRetryMinutes.FindStringSubmatch(message); minutes != nil {
                if value, err := strconv.ParseFloat(minutes[1], 64); err == nil {
                    if scaled := int64(value * float64(MinAccountBlockMs)); scaled > delay {
                        delay = scaled
                    }
                }
            }
            if seconds := accountPolicyRetrySeconds.FindStringSubmatch(message); seconds != nil {
                if value, err := strconv.ParseFloat(seconds[1], 64); err == nil {
                    if scaled := int64(value * 1000); scaled > delay {
                        delay = scaled
                    }
                }
            }
            return &AccountFailureScope{Scope: AccountQuotaScope(provider, modelID), Until: now + delay}
        }
        if status == 401 || accountPolicyAuthFailure.MatchString(message) {
            return &AccountFailureScope{Scope: "auth", Until: now + MinAccountBlockMs}
        }
        return nil
    }
    package ai
    
    // Request-local account routing across the stored accounts of one provider: model-plan gates,
    // OMP-compatible persisted blocks, a same-account OAuth refresh retry, and replay-safe fallback
    // that never lets a partial response escape twice.
    //
    // Ports packages/coding-agent/src/core/account-pool.ts. The router is transport-agnostic: the
    // caller supplies an attempt that opens one try for one account, so chat streams, image
    // generation and classification share the same selection, blocking and fallback policy.
    
    import (
        "context"
        "errors"
        "fmt"
        "math"
        "net/http"
        "slices"
        "strconv"
        "strings"
        "sync"
        "time"
    )
    
    // AccountRotation is one account handoff, as sessions and the JSON/RPC event protocols report it.
    type AccountRotation struct {
        ProviderID     string  `json:"providerId"`
        FromAccountKey *string `json:"fromAccountKey"`
        ToAccountKey   *string `json:"toAccountKey"`
        // Reason is "quota", "policy" or "auth".
        Reason    string `json:"reason"`
        Message   string `json:"message"`
        SessionID string `json:"sessionId,omitempty"`
    }
    
    // AccountRouteRequest is what one request tells the router about itself.
    type AccountRouteRequest struct {
        // SessionID pins a session's successes per model, so its next turn reuses the account that
        // worked for it.
        SessionID string
        // APIKeySet reports that the request carries an explicit key. An explicit key is the caller's
        // account decision: routing is bypassed so it is never rotated past.
        APIKeySet bool
    }
    
    // AccountSelection is the stored account one attempt resolves. AccountKeySet is always true for
    // a routed attempt: an explicit nil key addresses the identity-less account, which is not the
    // provider's selected one.
    type AccountSelection struct {
        AccountKey    *string
        AccountKeySet bool
    }
    
    // AccountObserve receives the provider response of the current attempt, so a rejection can be
    // classified by status and Retry-After. A nil observer records nothing.
    type AccountObserve func(ProviderResponse)
    
    // AccountAttemptStream opens one chat attempt for one account.
    type AccountAttemptStream func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (*AssistantMessageEventStream, error)
    
    // AccountAttemptImages opens one image-generation attempt for one account.
    type AccountAttemptImages func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (AssistantImages, error)
    
    // AccountAttemptClassify opens one classification attempt for one account.
    type AccountAttemptClassify func(ctx context.Context, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (ClassifierResult, error)
    
    // accountKeyValue is a comparable projection of a nullable account key.
    type accountKeyValue struct {
        key string
        set bool
    }
    
    func accountKeyOf(accountKey *string) accountKeyValue {
        if accountKey == nil {
            return accountKeyValue{}
        }
        return accountKeyValue{key: *accountKey, set: true}
    }
    
    // AccountRouter routes independent requests without changing the provider's persistent selected
    // account. A store without the account methods yields a router that never routes.
    type AccountRouter struct {
        credentials AccountCredentialStore
        now         func() time.Time
    
        mu sync.Mutex
        // affinity is provider → session → model → the account key that session's last success used.
        affinity map[string]map[string]map[string]*string
        // localBlocks holds this run's blocks by provider → account → scope, so an account stays
        // retired even when the store cannot persist the block.
        localBlocks map[string]map[accountKeyValue]map[string]int64
        // listeners observe handoffs.
        listeners []*accountRotationListener
    }
    
    // NewAccountRouter builds a router over store. A nil or single-account store never routes.
    func NewAccountRouter(store CredentialStore) *AccountRouter {
        router := &AccountRouter{now: time.Now, affinity: map[string]map[string]map[string]*string{}, localBlocks: map[string]map[accountKeyValue]map[string]int64{}}
        if store != nil {
            if accounts, ok := store.(AccountCredentialStore); ok {
                router.credentials = accounts
            }
        }
        return router
    }
    
    // Pooled reports whether the store holds several accounts per provider.
    func (r *AccountRouter) Pooled() bool { return r != nil && r.credentials != nil }
    
    // SetClock replaces the router's clock (tests).
    func (r *AccountRouter) SetClock(now func() time.Time) {
        if now != nil {
            r.now = now
        }
    }
    
    // OnRotation subscribes to account handoffs and returns the detach function.
    func (r *AccountRouter) OnRotation(listener func(AccountRotation)) func() {
        if r == nil || listener == nil {
            return func() {}
        }
        detach := &accountRotationListener{listener: listener}
        r.mu.Lock()
        r.listeners = append(r.listeners, detach)
        r.mu.Unlock()
        return func() {
        scope   string
    }
    
    // Candidates returns the accounts one request may use, best first: the session's pinned
    // account, then the provider's selected account, then row order. Accounts whose Codex plan
    // cannot serve the model are dropped up front; accounts whose plan cannot be read follow every
    // known-eligible account. It returns nil when the router does not route this request.
    func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request AccountRouteRequest) ([]*CredentialAccountInfo, error) {
        if !r.Pooled() || request.APIKeySet || model == nil {
            return nil, nil
        }
        if _, routed := AccountRequestFromContext(ctx); routed {
            // This request already carries its account decision.
            return nil, nil
        }
        if err := context.Cause(ctx); err != nil {
            return nil, err
        }
        all, err := r.credentials.ListAccounts(ctx)
        if err != nil {
            return nil, err
        }
        providerID, modelID := model.ProviderID(), model.ModelID()
        accounts := make([]*CredentialAccountInfo, 0, len(all))
        for i := range all {
            if all[i].ProviderID == providerID {
                account := all[i]
                accounts = append(accounts, &account)
            }
        }
        if len(accounts) == 0 {
            return nil, nil
        }
        pinned, pinnedSet := r.PeekAccountKey(providerID, request.SessionID, modelID)
        // Explicit selection is a preference, not permission to send an ineligible account; the sort
        // is stable, so rows keep their order within a rank.
        slices.SortStableFunc(accounts, func(left, right *CredentialAccountInfo) int {
            return accountRank(right, pinned, pinnedSet) - accountRank(left, pinned, pinnedSet)
        })
        var eligible, unknown []*CredentialAccountInfo
        for _, account := range accounts {
            if err := context.Cause(ctx); err != nil {
                return nil, err
            }
            if providerID != "openai-codex" || account.Type != CredentialOAuth {
                eligible = append(eligible, account)
                continue
            }
            credential, err := r.credentials.ReadAccount(ctx, providerID, account.AccountKey)
            if err != nil {
                return nil, err
            }
            if credential == nil {
                continue
            }
            meets := AccountMeetsModelPlan(providerID, modelID, *credential)
            switch {
            case meets == nil:
                unknown = append(unknown, account)
            case *meets:
                eligible = append(eligible, account)
            }
        }
        eligible = append(eligible, unknown...)
        if len(eligible) == 0 {
            return nil, fmt.Errorf("No stored account meets the plan requirement for %s/%s. Sign in with an eligible account.", providerID, modelID)
        }
        return eligible, nil
    }
    
    // accountRank scores a candidate: the session's pinned account, else the provider's selected
    // account, else row order.
    func accountRank(account *CredentialAccountInfo, pinned *string, pinnedSet bool) int {
        switch {
        case pinnedSet && accountKeyOf(account.AccountKey) == accountKeyOf(pinned):
            return 2
        case account.Active:
            return 1
        }
        return 0
    }
    
    // ResolveAccount reports the account a request would use, for model-scoped credential exports.
    // present is false for a request the router does not route.
    func (r *AccountRouter) ResolveAccount(ctx context.Context, model AnyModel, request AccountRouteRequest) (accountKey *string, present bool, err error) {
        accounts, err := r.Candidates(ctx, model, request)
        if err != nil || accounts == nil {
            return nil, false, err
        }
        var earliest *int64
        for _, account := range accounts {
            if err := context.Cause(ctx); err != nil {
                return nil, false, err
            }
            until, err := r.BlockedUntil(ctx, model, account)
            if err != nil {
    package ai_test
    
    // Behavioural tests of the request-local account router: model-plan eligibility, transient
    // non-rotation, the pre/post-content fallback boundary, and per-model session affinity.
    
    import (
        "context"
        "encoding/base64"
        "errors"
        "fmt"
        "strings"
        "sync"
        "testing"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
    )
    
    // fakeAccountStore is a multi-account credential store used only by these tests.
    type fakeAccountStore struct {
        mu       sync.Mutex
        accounts []ai.CredentialAccountInfo
        creds    map[string]ai.Credential
        blocks   map[string]int64
        blockErr error
        now      func() time.Time
    }
    
    func newFakeAccountStore(entries ...ai.CredentialAccountInfo) *fakeAccountStore {
        return &fakeAccountStore{accounts: entries, creds: map[string]ai.Credential{}, blocks: map[string]int64{}, now: time.Now}
    }
    
    func credentialID(providerID string, accountKey *string) string {
        if accountKey == nil {
            return providerID + "\x00<none>"
        }
        return providerID + "\x00" + *accountKey
    }
    
    func (store *fakeAccountStore) put(providerID string, accountKey *string, credential ai.Credential) {
        store.creds[credentialID(providerID, accountKey)] = credential
    }
    
    func (store *fakeAccountStore) Read(ctx context.Context, providerID string) (*ai.Credential, error) {
        for _, account := range store.accounts {
            if account.ProviderID == providerID && account.Active {
                return store.ReadAccount(ctx, providerID, account.AccountKey)
            }
        }
        return nil, nil
    }
    
    func (store *fakeAccountStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
        var out []ai.CredentialInfo
        for _, account := range store.accounts {
            out = append(out, ai.CredentialInfo{ProviderID: account.ProviderID, Type: account.Type})
        }
        return out, nil
    }
    
    func (store *fakeAccountStore) Modify(ctx context.Context, providerID string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
        return nil, errors.New("not used")
    }
    
    func (store *fakeAccountStore) Delete(ctx context.Context, providerID string) error { return nil }
    
    func (store *fakeAccountStore) ReadAccount(ctx context.Context, providerID string, accountKey *string) (*ai.Credential, error) {
        credential, ok := store.creds[credentialID(providerID, accountKey)]
        if !ok {
            return nil, nil
        }
        return &credential, nil
    }
    
    func (store *fakeAccountStore) ListAccounts(ctx context.Context) ([]ai.CredentialAccountInfo, error) {
        store.mu.Lock()
        defer store.mu.Unlock()
        return append([]ai.CredentialAccountInfo(nil), store.accounts...), nil
    }
    
    func (store *fakeAccountStore) SaveAccount(ctx context.Context, providerID string, accountKey *string, credential ai.Credential) ([]ai.CredentialAccountInfo, error) {
        return nil, errors.New("not used")
    }
    
    func (store *fakeAccountStore) SelectAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
        return false, nil
    }
    
    func (store *fakeAccountStore) DeleteAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
        return false, nil
    }
    
    func (store *fakeAccountStore) ModifyAccount(ctx context.Context, providerID string, accountKey *string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
        return nil, errors.New("not used")
    }
    
    func (store *fakeAccountStore) GetAccountBlockedUntil(ctx context.Context, providerID string, accountKey *string, scopes []string) (*int64, error) {
        store.mu.Lock()
        defer store.mu.Unlock()
        until := int64(0)
        for _, scope := range append([]string{""}, scopes...) {
            if deadline := store.blocks[credentialID(providerID, accountKey)+"\x00"+scope]; deadline > until {
                until = deadline
            }
        }
        if until == 0 {
            return nil, nil
        }
        return &until, nil
    }
    
    func (store *fakeAccountStore) BlockAccount(ctx context.Context, providerID string, accountKey *string, scope string, untilMs int64) error {
        store.mu.Lock()
        defer store.mu.Unlock()
        key := credentialID(providerID, accountKey) + "\x00" + scope
        if untilMs > store.blocks[key] {
            store.blocks[key] = untilMs
        }
        return store.blockErr
    }
    
    func (store *fakeAccountStore) AccountCount() (int, error) { return len(store.accounts), nil }
    
    // block records a block directly, as another process would have written it.
    func (store *fakeAccountStore) block(providerID string, accountKey *string, scope string, untilMs int64) {
        store.mu.Lock()
        defer store.mu.Unlock()
        store.blocks[credentialID(providerID, accountKey)+"\x00"+scope] = untilMs
    }
    
    func keyOf(value string) *string { return new(value) }
    
    func accountInfo(providerID string, accountKey *string, active bool) ai.CredentialAccountInfo {
        return ai.CredentialAccountInfo{ProviderID: providerID, Type: ai.CredentialOAuth, AccountKey: accountKey, Active: active}
    }
    
    // codexToken builds an unsigned JWT whose ChatGPT plan claim is plan.
    func codexToken(plan string) string {
        encode := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
        payload := fmt.Sprintf(`{"https://api.openai.com/auth":{"chatgpt_plan_type":%q}}`, plan)
        return encode(`{"alg":"none"}`) + "." + encode(payload) + ".signature"
    }
    
    // okModel builds a chat model identity the router routes on.
    func okModel(provider, id string) *ai.Model {
        return &ai.Model{ID: id, ProviderMeta: ai.ProviderMetadata{ProviderID: provider, API: ai.APIOpenAICompletions}}
    }
    
    func TestAccountMeetsModelPlan(t *testing.T) {
        codex := func(plan string) ai.Credential {
            return ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(plan)}
        }
        free := codex("free")
    
        // A free plan cannot serve a paid catalog model, and the gate rejects it before any request.
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", free); meets == nil || *meets {
            t.Fatalf("free plan must be ineligible for gpt-5.6-sol, got %v", meets)
        }
        // A pro plan can.
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", codex("pro")); meets == nil || !*meets {
            t.Fatalf("pro plan must be eligible for gpt-5.6-sol, got %v", meets)
        }
        // Plus serves paid models but not a Spark model, which needs pro.
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-luna", codex("plus")); meets == nil || !*meets {
            t.Fatalf("plus plan must be eligible for gpt-5.6-luna, got %v", meets)
        }
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-spark", codex("plus")); meets == nil || *meets {
            t.Fatalf("plus plan must be ineligible for a Spark model, got %v", meets)
        }
        // Any plan serves a model with no catalog requirement, free included.
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5", free); meets == nil || !*meets {
            t.Fatalf("a free plan must serve an unrestricted model, got %v", meets)
        }
        // An unreadable plan stays probeable rather than eligible.
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", ai.Credential{Type: ai.CredentialOAuth, Access: "not-a-jwt"}); meets != nil {
            t.Fatalf("an unreadable plan must stay unknown, got %v", meets)
        }
        // A different provider and an API key carry no plan.
        if meets := ai.AccountMeetsModelPlan("openai", "gpt-6.1-sol", free); meets == nil || !*meets {
            t.Fatalf("another provider is always eligible, got %v", meets)
        }
        if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-6.1-sol", ai.Credential{Type: ai.CredentialAPIKey, Key: "sk-test"}); meets == nil || !*meets {
            t.Fatalf("an API key is always eligible, got %v", meets)
        }
    }
    
    func TestAccountFailureBlockClassifies(t *testing.T) {
        now := time.Now().UnixMilli()
        quota := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached", 429, nil)
        if quota == nil || quota.Scope != "chat" || quota.Until < now+59_000 {
            t.Fatalf("quota failure must block chat for at least a minute, got %+v", quota)
        }
        // Transient throttling is not an account failure: a sibling would not help.
        if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "too many requests, please slow down", 429, nil); block != nil {
            t.Fatalf("transient 429 must not rotate, got %+v", block)
        }
        if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "internal server error", 500, nil); block != nil {
            t.Fatalf("server failure must not rotate, got %+v", block)
        }
        // A Spark model keeps its own meter.
        if spark := ai.AccountFailureBlock("openai-codex", "gpt-5.6-spark", "insufficient_quota", 200, nil); spark == nil || spark.Scope != "spark" {
            t.Fatalf("a Spark quota failure must use the spark scope, got %+v", spark)
        }
        // An auth rejection blocks the auth scope for a minute.
        if auth := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "invalidated oauth token", 401, nil); auth == nil || auth.Scope != "auth" {
            t.Fatalf("an auth rejection must use the auth scope, got %+v", auth)
        }
        // The plan denial is scoped to the model, not the account.
        denied := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", `The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.`, 400, nil)
        if denied == nil || denied.Scope != "model-policy:gpt-6.1-sol" {
            t.Fatalf("a plan denial must use the model-policy scope, got %+v", denied)
        }
        // The provider's own wait outranks the one-minute floor.
        if slow := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached; try again in 5 min", 429, nil); slow == nil || slow.Until < now+299_000 {
            t.Fatalf("a stated retry wait must extend the block, got %+v", slow)
        }
    }
    
    func TestRouterTransientFailureDoesNotRotate(t *testing.T) {
        store := newFakeAccountStore(
        // Another process blocked the selected account; a fresh router must honor it immediately.
        store.block("p", keyOf("a"), "auth", time.Now().Add(time.Hour).UnixMilli())
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var tried []string
        _, _ = router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            tried = append(tried, *selection.AccountKey)
            return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
        })
        if len(tried) != 1 || tried[0] != "b" {
            t.Fatalf("a persisted block must keep the account out of the candidate list, tried %v", tried)
        }
    }
    
    func TestRouterAffinityPerModel(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        request := ai.AccountRouteRequest{SessionID: "s1"}
        first := okModel("p", "m1")
        second := okModel("p", "m2")
        var tried []string
        attempt := func(accountKey string) func(context.Context, ai.AccountSelection, ai.AccountObserve, bool) (ai.AssistantImages, error) {
            return func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
                tried = append(tried, *selection.AccountKey)
                return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
            }
        }
        // First model: no pin, so the provider's selected account serves it and becomes the pin.
        _, _ = router.ImagesRoutes(context.Background(), first, request, attempt("a"))
        if key, present := router.PeekAccountKey("p", "s1", "m1"); !present || key == nil || *key != "a" {
            t.Fatalf("a success must pin the model's account, got %v %v", key, present)
        }
        // Another model has no pin: the affinity is per model, not per session.
        if _, present := router.PeekAccountKey("p", "s1", "m2"); present {
            t.Fatal("affinity must not leak across models")
        }
        // A block on the pinned account moves that model to a sibling without touching the pin of
        // the model that has one.
        tried = nil
        store.block("p", keyOf("a"), "", time.Now().Add(time.Minute).UnixMilli())
        store.accounts = []ai.CredentialAccountInfo{
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        }
        _, _ = router.ImagesRoutes(context.Background(), second, request, attempt("b"))
        if key, present := router.PeekAccountKey("p", "s1", "m2"); !present || key == nil || *key != "b" {
            t.Fatalf("the second model must pin its own account, got %v %v", key, present)
        }
        // Clearing a provider drops every pin for it.
        router.ClearProvider("p")
        if _, present := router.PeekAccountKey("p", "s1", "m1"); present {
            t.Fatal("ClearProvider must drop the provider's affinity")
        }
    }
    
    func TestRouterExplicitAPIKeyBypasses(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var selections []ai.AccountSelection
        _, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{APIKeySet: true}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            selections = append(selections, selection)
            return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
        })
        if err != nil {
            t.Fatal(err)
        }
        if len(selections) != 1 || selections[0].AccountKeySet {
            t.Fatalf("an explicit key must bypass the pool, got %+v", selections)
        }
    }
    
    func TestRouterNoEligibleAccountError(t *testing.T) {
        store := newFakeAccountStore(accountInfo("openai-codex", keyOf("free-account"), true))
        store.put("openai-codex", keyOf("free-account"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("free")})
    package ai
    
    // Request-scoped stored-account selection. A request attempt that a credential pool routes to
    // one stored account marks its context, so every auth-resolution path that request takes — the
    // model collection's chat, image and classifier operations, and the coding layer's request
    // preparation — resolves that account instead of the provider's selected one.
    
    import (
        "context"
        "net/http"
    )
    
    // AccountRequest is the stored-account scope of one request attempt.
    type AccountRequest struct {
        // AccountKey names the account to resolve. It is meaningful only with AccountKeySet: an
        // explicit nil key names the identity-less account, while an unset key means the provider's
        // selection.
        AccountKey *string
        // AccountKeySet reports that the request named an account, including the identity-less one.
        AccountKeySet bool
        // ForceOAuthRefresh refreshes a stored OAuth credential even while it still looks valid,
        // because the provider rejected that token.
        ForceOAuthRefresh bool
    }
    
    type accountRequestContextKey struct{}
    
    // WithAccountRequest marks ctx with the account scope of one request attempt.
    func WithAccountRequest(ctx context.Context, request AccountRequest) context.Context {
        return context.WithValue(ctx, accountRequestContextKey{}, request)
    }
    
    // AccountRequestFromContext reports the account scope of the request attempt running under ctx.
    func AccountRequestFromContext(ctx context.Context) (AccountRequest, bool) {
        if ctx == nil {
            return AccountRequest{}, false
        }
        request, ok := ctx.Value(accountRequestContextKey{}).(AccountRequest)
        return request, ok
    }
    
    // ApplyAccountRequest merges ctx's account scope into overrides. Explicit overrides win: a caller
    // that named an account keeps it. A merged API key already outranks the stored accounts.
    func ApplyAccountRequest(ctx context.Context, overrides AuthResolutionOverrides) AuthResolutionOverrides {
        request, ok := AccountRequestFromContext(ctx)
        if !ok {
            return overrides
        }
        if !overrides.AccountKeySet {
            overrides.AccountKey = request.AccountKey
            overrides.AccountKeySet = request.AccountKeySet
        }
        overrides.ForceOAuthRefresh = overrides.ForceOAuthRefresh || request.ForceOAuthRefresh
        return overrides
    }
    
    // WithAccountSelection marks ctx with one attempt's account selection. An attempt the router did
    // not route leaves ctx unchanged, so the provider's selected account still applies.
    func WithAccountSelection(ctx context.Context, selection AccountSelection, forceOAuthRefresh bool) context.Context {
        if !selection.AccountKeySet && !forceOAuthRefresh {
            return ctx
        }
        return WithAccountRequest(ctx, AccountRequest{AccountKey: selection.AccountKey, AccountKeySet: selection.AccountKeySet, ForceOAuthRefresh: forceOAuthRefresh})
    }
    
    type accountResponseObserverKey struct{}
    
    func withAccountResponseObserver(ctx context.Context, observe AccountObserve) context.Context {
        return context.WithValue(ctx, accountResponseObserverKey{}, observe)
    }
    
    // Native APIs invoke the public OnResponse hook for accepted responses. Account
    // policy also needs rejected HTTP headers, without changing that public hook's
    // semantics or reading/consuming the response body.
    func observeRejectedAccountResponse(ctx context.Context, response *http.Response) {
        if response == nil || response.StatusCode < 400 {
            return
        }
        if observe, ok := ctx.Value(accountResponseObserverKey{}).(AccountObserve); ok && observe != nil {
            observe(ProviderResponse{Status: response.StatusCode, Headers: headersToRecord(response.Header)})
        }
    }
    ✓ • 0ms

## Activity

    $ sed -n '240,329p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '1,65p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; sed -n '1,180p' vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go; rg -n 'candidates\(|candidateAccounts|listCandidates|accountCandidates' vendor/github.com/MichaelKinsy/PiG/ai/account*; rg -n 'func .*Routes|func .*route' vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; rg -n 'gpt-6.*luna' vendor/github.com/MichaelKinsy/PiG/ai/models_generated.go | head
        scope   string
    }
    
    // Candidates returns the accounts one request may use, best first: the session's pinned
    // account, then the provider's selected account, then row order. Accounts whose Codex plan
    // cannot serve the model are dropped up front; accounts whose plan cannot be read follow every
    // known-eligible account. It returns nil when the router does not route this request.
    func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request AccountRouteRequest) ([]*CredentialAccountInfo, error) {
        if !r.Pooled() || request.APIKeySet || model == nil {
            return nil, nil
        }
        if _, routed := AccountRequestFromContext(ctx); routed {
            // This request already carries its account decision.
            return nil, nil
        }
        if err := context.Cause(ctx); err != nil {
            return nil, err
        }
        all, err := r.credentials.ListAccounts(ctx)
        if err != nil {
            return nil, err
        }
        providerID, modelID := model.ProviderID(), model.ModelID()
        accounts := make([]*CredentialAccountInfo, 0, len(all))
        for i := range all {
            if all[i].ProviderID == providerID {
                account := all[i]
                accounts = append(accounts, &account)
            }
        }
        if len(accounts) == 0 {
            return nil, nil
        }
        pinned, pinnedSet := r.PeekAccountKey(providerID, request.SessionID, modelID)
        // Explicit selection is a preference, not permission to send an ineligible account; the sort
        // is stable, so rows keep their order within a rank.
        slices.SortStableFunc(accounts, func(left, right *CredentialAccountInfo) int {
            return accountRank(right, pinned, pinnedSet) - accountRank(left, pinned, pinnedSet)
        })
        var eligible, unknown []*CredentialAccountInfo
        for _, account := range accounts {
            if err := context.Cause(ctx); err != nil {
                return nil, err
            }
            if providerID != "openai-codex" || account.Type != CredentialOAuth {
                eligible = append(eligible, account)
                continue
            }
            credential, err := r.credentials.ReadAccount(ctx, providerID, account.AccountKey)
            if err != nil {
                return nil, err
            }
            if credential == nil {
                continue
            }
            meets := AccountMeetsModelPlan(providerID, modelID, *credential)
            switch {
            case meets == nil:
                unknown = append(unknown, account)
            case *meets:
                eligible = append(eligible, account)
            }
        }
        eligible = append(eligible, unknown...)
        if len(eligible) == 0 {
            return nil, fmt.Errorf("No stored account meets the plan requirement for %s/%s. Sign in with an eligible account.", providerID, modelID)
        }
        return eligible, nil
    }
    
    // accountRank scores a candidate: the session's pinned account, else the provider's selected
    // account, else row order.
    func accountRank(account *CredentialAccountInfo, pinned *string, pinnedSet bool) int {
        switch {
        case pinnedSet && accountKeyOf(account.AccountKey) == accountKeyOf(pinned):
            return 2
        case account.Active:
            return 1
        }
        return 0
    }
    
    // ResolveAccount reports the account a request would use, for model-scoped credential exports.
    // present is false for a request the router does not route.
    func (r *AccountRouter) ResolveAccount(ctx context.Context, model AnyModel, request AccountRouteRequest) (accountKey *string, present bool, err error) {
        accounts, err := r.Candidates(ctx, model, request)
        if err != nil || accounts == nil {
            return nil, false, err
        }
        var earliest *int64
    package ai_test
    
    // Behavioural tests of the request-local account router: model-plan eligibility, transient
    // non-rotation, the pre/post-content fallback boundary, and per-model session affinity.
    
    import (
        "context"
        "encoding/base64"
        "errors"
        "fmt"
        "strings"
        "sync"
        "testing"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
    )
    
    // fakeAccountStore is a multi-account credential store used only by these tests.
    type fakeAccountStore struct {
        mu       sync.Mutex
        accounts []ai.CredentialAccountInfo
        creds    map[string]ai.Credential
        blocks   map[string]int64
        blockErr error
        now      func() time.Time
    }
    
    func newFakeAccountStore(entries ...ai.CredentialAccountInfo) *fakeAccountStore {
        return &fakeAccountStore{accounts: entries, creds: map[string]ai.Credential{}, blocks: map[string]int64{}, now: time.Now}
    }
    
    func credentialID(providerID string, accountKey *string) string {
        if accountKey == nil {
            return providerID + "\x00<none>"
        }
        return providerID + "\x00" + *accountKey
    }
    
    func (store *fakeAccountStore) put(providerID string, accountKey *string, credential ai.Credential) {
        store.creds[credentialID(providerID, accountKey)] = credential
    }
    
    func (store *fakeAccountStore) Read(ctx context.Context, providerID string) (*ai.Credential, error) {
        for _, account := range store.accounts {
            if account.ProviderID == providerID && account.Active {
                return store.ReadAccount(ctx, providerID, account.AccountKey)
            }
        }
        return nil, nil
    }
    
    func (store *fakeAccountStore) List(ctx context.Context) ([]ai.CredentialInfo, error) {
        var out []ai.CredentialInfo
        for _, account := range store.accounts {
            out = append(out, ai.CredentialInfo{ProviderID: account.ProviderID, Type: account.Type})
        }
        return out, nil
    }
    
    func (store *fakeAccountStore) Modify(ctx context.Context, providerID string, fn func(*ai.Credential) (*ai.Credential, error)) (*ai.Credential, error) {
        return nil, errors.New("not used")
    }
    
    func (store *fakeAccountStore) Delete(ctx context.Context, providerID string) error { return nil }
    package ai
    
    // The routed chat, image and classifier operations of an AccountRouter. Each drives the same
    // candidate list, block check, same-account OAuth refresh retry and rotation policy, and each
    // keeps one hard boundary: nothing a failed attempt already delivered is ever replayed.
    
    import (
        "context"
        "errors"
        "sync/atomic"
        "time"
    )
    
    // StreamRoutes runs one chat request across the provider's stored accounts and returns at once
    // with the stream of the attempt that produced a response.
    //
    // Only a rejection that arrives before any content escapes is retried on a sibling account: the
    // attempt's start event is held until a real event follows it, and an error event with no
    // content, no abort and no caller cancellation blocks the account and rotates. Once any content
    // is emitted, a failure is reported as it is — rotating would replay a partial answer.
    //
    // A request the router does not route opens once against the provider's selected account,
    // exactly as an unpooled request.
    func (r *AccountRouter) StreamRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) *AssistantMessageEventStream {
        outer := NewAssistantMessageEventStream()
        ctx = outer.ObservationContext(ctx)
        go r.streamRoutes(ctx, outer, model, request, open)
        return outer
    }
    
    func (r *AccountRouter) streamRoutes(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) {
        accounts, err := r.Candidates(ctx, model, request)
        if err != nil {
            if context.Cause(ctx) != nil {
                err = AbortReason(ctx)
            }
            pushAccountSetupError(outer, model, err)
            return
        }
        if accounts == nil {
            // An unpooled request still consumes its provider through the bounded attempt reader, so a
            // provider that closes without resolving a result cannot hang the consumer here.
            observed := newObservedResponse()
            stream, openErr := open(ctx, AccountSelection{}, observed.record, false)
            if openErr != nil {
                if context.Cause(ctx) != nil {
                    openErr = AbortReason(ctx)
                }
                pushAccountSetupError(outer, model, openErr)
                return
            }
            if stream == nil {
                pushAccountSetupError(outer, model, errors.New("provider returned no stream"))
                return
            }
            r.driveAttempt(ctx, outer, model, nil, request, stream, observed, false)
            return
        }
        var lastFailure *ErrorEvent
        var lastSetupError error
        var earliest *int64
        var previous *accountFailure
        for _, account := range accounts {
            if context.Cause(ctx) != nil {
                pushAccountSetupError(outer, model, AbortReason(ctx))
                return
            }
            until, blockErr := r.BlockedUntil(ctx, model, account)
            if blockErr != nil {
                pushAccountSetupError(outer, model, blockErr)
                return
            }
            if until != nil && *until > r.nowMillis() {
                earliest = earliestDeadline(earliest, *until)
                continue
            }
            if previous != nil {
                r.reportRotation(model.ProviderID(), model.ModelID(), *previous, account, request)
            }
            for refresh := range 2 {
                attemptCtx, selection := r.attemptContext(ctx, account)
                observed := newObservedResponse()
                attemptCtx = withAccountResponseObserver(attemptCtx, observed.record)
                stream, openErr := open(attemptCtx, selection, observed.record, refresh == 1)
                if openErr != nil {
                    if context.Cause(ctx) != nil {
                        pushAccountSetupError(outer, model, AbortReason(ctx))
                        return
                    }
                    block := r.Fail(ctx, model, account, accountErrorText(openErr), observed.get())
                    if block == nil {
                        pushAccountSetupError(outer, model, openErr)
                        return
                    }
                    previous = &accountFailure{account: account, scope: block.Scope}
                    lastFailure, lastSetupError = nil, openErr
                    break
                }
                if stream == nil {
                    pushAccountSetupError(outer, model, errors.New("provider returned no stream"))
                    return
                }
                outcome := r.driveAttempt(ctx, outer, model, account, request, stream, observed, refresh == 1)
                lastFailure, lastSetupError = outcome.event, nil
                switch {
                case outcome.delivered:
                    return
                case outcome.retryRefresh:
                    continue
                case outcome.rotated:
                    previous = outcome.failure
                }
                break
            }
        }
        if lastFailure != nil {
            _ = outer.Push(*lastFailure)
            return
        }
        if lastSetupError != nil {
            pushAccountSetupError(outer, model, lastSetupError)
            return
        }
        pushAccountSetupError(outer, model, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest)))
    }
    
    // accountStreamOutcome is what one attempt's event run decided.
    type accountStreamOutcome struct {
        // emitted reports that content reached the consumer, so nothing may replay.
        emitted bool
        // delivered reports that the attempt was delivered and the request is finished.
        delivered bool
        // retryRefresh reports that the same account must be retried after a forced token refresh.
        retryRefresh bool
        // rotated reports that this account was retired and a sibling should be tried.
        rotated bool
        failure *accountFailure
        event   *ErrorEvent
    }
    
    // driveAttempt consumes one attempt's stream, holding its start event until a real event follows.
    func (r *AccountRouter) driveAttempt(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest, stream *AssistantMessageEventStream, observed *observedResponse, forceOAuthRefresh bool) accountStreamOutcome {
        var start *StartEvent
        // partial is the last message the consumer saw, so a cancellation terminal reports the same
        // content instead of discarding it.
        var partial *AssistantMessage
        emitted := false
        terminal := false
        for event := range stream.Events(ctx) {
            if emittedContent, ok := event.(StartEvent); ok && !emitted {
                copied := emittedContent
                start = &copied
                partial = copied.Partial
                continue
            }
            if current := eventPartial(event); current != nil {
                partial = current
            }
            if failure, ok := event.(ErrorEvent); ok && !emitted && errorEventIsEmpty(failure) && failure.Reason != StopReasonAborted && context.Cause(ctx) == nil && account != nil {
                if shouldRetryWithRefresh(account, forceOAuthRefresh, observed.get()) {
                    return accountStreamOutcome{retryRefresh: true}
                }
                block := r.Fail(ctx, model, account, errorMessageOf(failure), observed.get())
                if block != nil {
                    return accountStreamOutcome{rotated: true, failure: &accountFailure{account: account, scope: block.Scope}, event: &failure}
                }
            }
            if start != nil {
                _ = outer.Push(*start)
                start = nil
                emitted = true
            }
            emitted = true
            switch event.(type) {
            case DoneEvent:
                terminal = true
                // A successful turn pins the account for this session and model (account-pool.ts
                // stream `if (event.type === "done") this.pin(...)`).
                if account != nil {
                    r.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:24:func (r *AccountRouter) StreamRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) *AssistantMessageEventStream {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:31:func (r *AccountRouter) streamRoutes(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:282:func (r *AccountRouter) ImagesRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptImages) (AssistantImages, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:290:func (r *AccountRouter) ClassifyRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptClassify) (ClassifierResult, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:314:func routeAccountResult[T any](
    1399:        ID:                   "global.openai.gpt-6-luna",
    1415:        Record:               "{\"id\":\"global.openai.gpt-6-luna\",\"name\":\"GPT-6 Luna (Global)\",\"api\":\"bedrock-converse-stream\",\"provider\":\"amazon-bedrock\",\"baseUrl\":\"https://bedrock-runtime.us-east-1.amazonaws.com\",\"reasoning\":true,\"input\":[\"text\",\"image\"],\"cost\":{\"input\":0.1,\"output\":0.5,\"cacheRead\":0.01,\"cacheWrite\":0.125,\"tiers\":[{\"inputTokensAbove\":272000,\"input\":0.2,\"output\":0.75,\"cacheRead\":0.02,\"cacheWrite\":0.25}]},\"contextWindow\":1050000,\"maxTokens\":128000,\"compat\":{\"supportsStrictMode\":true},\"thinkingLevelMap\":{\"xhigh\":\"xhigh\"},\"inputLimits\":{\"images\":{\"maxPerMessage\":20,\"resize\":{\"maxWidth\":2000,\"maxHeight\":2000,\"maxBytes\":4718592,\"jpegQuality\":80}}},\"type\":\"chat\"}",
    2357:        ID:                   "openai.gpt-6-luna",
    2373:        Record:               "{\"id\":\"openai.gpt-6-luna\",\"name\":\"GPT-6 Luna\",\"api\":\"bedrock-converse-stream\",\"provider\":\"amazon-bedrock\",\"baseUrl\":\"https://bedrock-runtime.us-east-1.amazonaws.com\",\"reasoning\":true,\"input\":[\"text\",\"image\"],\"cost\":{\"input\":0.11,\"output\":0.55,\"cacheRead\":0.011,\"cacheWrite\":0.1375,\"tiers\":[{\"inputTokensAbove\":272000,\"input\":0.22,\"output\":0.825,\"cacheRead\":0.022,\"cacheWrite\":0.275}]},\"contextWindow\":1050000,\"maxTokens\":128000,\"compat\":{\"supportsStrictMode\":true},\"thinkingLevelMap\":{\"xhigh\":\"xhigh\"},\"inputLimits\":{\"images\":{\"maxPerMessage\":20,\"resize\":{\"maxWidth\":2000,\"maxHeight\":2000,\"maxBytes\":4718592,\"jpegQuality\":80}}},\"type\":\"chat\"}",
    3253:        ID:                   "us.openai.gpt-6-luna",
    3269:        Record:               "{\"id\":\"us.openai.gpt-6-luna\",\"name\":\"GPT-6 Luna (US)\",\"api\":\"bedrock-converse-stream\",\"provider\":\"amazon-bedrock\",\"baseUrl\":\"https://bedrock-runtime.us-east-1.amazonaws.com\",\"reasoning\":true,\"input\":[\"text\",\"image\"],\"cost\":{\"input\":0.11,\"output\":0.55,\"cacheRead\":0.011,\"cacheWrite\":0.1375,\"tiers\":[{\"inputTokensAbove\":272000,\"input\":0.22,\"output\":0.825,\"cacheRead\":0.022,\"cacheWrite\":0.275}]},\"contextWindow\":1050000,\"maxTokens\":128000,\"compat\":{\"supportsStrictMode\":true},\"thinkingLevelMap\":{\"xhigh\":\"xhigh\"},\"inputLimits\":{\"images\":{\"maxPerMessage\":20,\"resize\":{\"maxWidth\":2000,\"maxHeight\":2000,\"maxBytes\":4718592,\"jpegQuality\":80}}},\"type\":\"chat\"}",
    4454:        ID:                   "gpt-6-luna",
    4469:        Record:               "{\"id\":\"gpt-6-luna\",\"name\":\"GPT-6 Luna\",\"api\":\"azure-openai-responses\",\"provider\":\"azure\",\"baseUrl\":\"\",\"reasoning\":true,\"input\":[\"text\",\"image\"],\"cost\":{\"input\":0.1,\"output\":0.5,\"cacheRead\":0.01,\"cacheWrite\":0.125},\"contextWindow\":272000,\"maxTokens\":128000,\"thinkingLevelMap\":{\"off\":\"none\",\"minimal\":null,\"low\":\"low\",\"medium\":\"medium\",\"high\":\"high\",\"xhigh\":\"xhigh\",\"max\":\"max\"},\"compat\":{\"supportsOpenAIGrammarTools\":true},\"inputLimits\":{\"images\":{\"resize\":{\"maxWidth\":2000,\"maxHeight\":2000,\"maxBytes\":4718592,\"jpegQuality\":80}}},\"type\":\"chat\"}",
    5889:        ID:                   "gpt-6-luna",
    5905:        Record:               "{\"id\":\"gpt-6-luna\",\"name\":\"GPT-6 Luna\",\"api\":\"openai-responses\",\"provider\":\"cloudflare-ai-gateway\",\"baseUrl\":\"https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/openai\",\"reasoning\":true,\"input\":[\"text\",\"image\"],\"cost\":{\"input\":0.1,\"output\":0.5,\"cacheRead\":0.01,\"cacheWrite\":0.125,\"tiers\":[{\"inputTokensAbove\":272000,\"input\":0.2,\"output\":0.75,\"cacheRead\":0.02,\"cacheWrite\":0.25}]},\"contextWindow\":1050000,\"maxTokens\":128000,\"thinkingLevelMap\":{\"off\":\"none\",\"minimal\":null,\"low\":\"low\",\"medium\":\"medium\",\"high\":\"high\",\"xhigh\":\"xhigh\",\"max\":\"max\"},\"compat\":{\"supportsStrictMode\":true,\"supportsOpenAIGrammarTools\":true},\"inputLimits\":{\"images\":{\"resize\":{\"maxWidth\":2000,\"maxHeight\":2000,\"maxBytes\":4718592,\"jpegQuality\":80}}},\"type\":\"chat\"}",
    ✓ • 0ms

## Activity

    $ sed -n '88,133p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '225,300p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; sed -n '380,440p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; rg -n 'StreamRoutes|DoneEvent|ErrorEvent' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go | tail -25; sed -n '444,535p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go
        // localBlocks holds this run's blocks by provider → account → scope, so an account stays
        // retired even when the store cannot persist the block.
        localBlocks map[string]map[accountKeyValue]map[string]int64
        // listeners observe handoffs.
        listeners []*accountRotationListener
    }
    
    // NewAccountRouter builds a router over store. A nil or single-account store never routes.
    func NewAccountRouter(store CredentialStore) *AccountRouter {
        router := &AccountRouter{now: time.Now, affinity: map[string]map[string]map[string]*string{}, localBlocks: map[string]map[accountKeyValue]map[string]int64{}}
        if store != nil {
            if accounts, ok := store.(AccountCredentialStore); ok {
                router.credentials = accounts
            }
        }
        return router
    }
    
    // Pooled reports whether the store holds several accounts per provider.
    func (r *AccountRouter) Pooled() bool { return r != nil && r.credentials != nil }
    
    // SetClock replaces the router's clock (tests).
    func (r *AccountRouter) SetClock(now func() time.Time) {
        if now != nil {
            r.now = now
        }
    }
    
    // OnRotation subscribes to account handoffs and returns the detach function.
    func (r *AccountRouter) OnRotation(listener func(AccountRotation)) func() {
        if r == nil || listener == nil {
            return func() {}
        }
        detach := &accountRotationListener{listener: listener}
        r.mu.Lock()
        r.listeners = append(r.listeners, detach)
        r.mu.Unlock()
        return func() {
            r.mu.Lock()
            r.listeners = slices.DeleteFunc(r.listeners, func(existing *accountRotationListener) bool { return existing == detach })
            r.mu.Unlock()
        }
    }
    
    // ClearProvider drops this runtime's successful-request affinity for a provider, so an explicit
    // login or account change takes effect on the next request.
        model := okModel("p", "m")
        var tried []string
        result, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            tried = append(tried, *selection.AccountKey)
            return ai.AssistantImages{}, errors.New("connection reset by peer")
        })
        if err == nil {
            t.Fatal("a transport failure must surface as an error")
        }
        if len(tried) != 1 || tried[0] != "a" {
            t.Fatalf("a transient failure must not rotate accounts, tried %v", tried)
        }
        if result.StopReason != "" {
            t.Fatalf("expected no result, got %+v", result)
        }
    }
    
    func TestRouterRotatesOnQuotaBeforeOutput(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var tried []string
        result, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            tried = append(tried, *selection.AccountKey)
            if *selection.AccountKey == "a" {
                observe(ai.ProviderResponse{Status: 429, Headers: map[string]string{}})
                return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota"}, nil
            }
            return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop, Output: []ai.ImagesOutputContent{ai.TextContent{Text: "ok"}}}, nil
        })
        if err != nil {
            t.Fatalf("rotation must hide the first account's rejection: %v", err)
        }
        if len(tried) != 2 || tried[0] != "a" || tried[1] != "b" {
            t.Fatalf("expected a then b, got %v", tried)
        }
        if result.StopReason != ai.ImagesStopReasonStop {
            t.Fatalf("expected the second account's result, got %+v", result)
        }
        until, err := store.GetAccountBlockedUntil(context.Background(), "p", keyOf("a"), ai.AccountRequestBlockScopes("p", "m"))
        if err != nil || until == nil || *until <= time.Now().UnixMilli() {
            t.Fatalf("the rejected account must stay blocked: %v %v", until, err)
        }
    }
    
    func TestRouterKeepsPartialOutput(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var tried []string
        result, _ := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            tried = append(tried, *selection.AccountKey)
            // The account reports quota exhaustion, but one image was already produced: replaying
            // would bill the work twice and discard output the caller can use.
            return ai.AssistantImages{StopReason: ai.ImagesStopReasonError, ErrorMessage: "insufficient_quota", Output: []ai.ImagesOutputContent{ai.TextContent{Text: "partial"}}}, nil
        })
        if len(tried) != 1 {
            t.Fatalf("a partial result must not rotate, tried %v", tried)
        }
        if len(result.Output) != 1 {
            t.Fatalf("the partial result must be delivered, got %+v", result)
        }
    }
    
    func TestRouterHonorsPersistedBlocksAcrossProcesses(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        // Another process blocked the selected account; a fresh router must honor it immediately.
        store.put("openai-codex", keyOf("free-account"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("free")})
        router := ai.NewAccountRouter(store)
        model := okModel("openai-codex", "gpt-5.6-sol")
        var tried []string
        _, err := router.ImagesRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
            tried = append(tried, *selection.AccountKey)
            return ai.AssistantImages{StopReason: ai.ImagesStopReasonStop}, nil
        })
        if err == nil {
            t.Fatal("an ineligible account must fail before any request")
        }
        if !strings.Contains(err.Error(), "No stored account meets the plan requirement") {
            t.Fatalf("unexpected error: %v", err)
        }
        if len(tried) != 0 {
            t.Fatalf("no request may be sent for an ineligible account, tried %v", tried)
        }
    }
    
    func TestWithAccountKeyPresence(t *testing.T) {
        overrides := ai.WithAccountKey(ai.AuthResolutionOverrides{}, nil)
        if !overrides.AccountKeySet || overrides.AccountKey != nil {
            t.Fatalf("an explicit nil key must be preserved as present, got %+v", overrides)
        }
        // Applying a request scope never overrides a caller's explicit choice.
        named := ai.WithAccountKey(ai.AuthResolutionOverrides{}, keyOf("chosen"))
        ctx := ai.WithAccountRequest(context.Background(), ai.AccountRequest{AccountKey: nil, AccountKeySet: true})
        if merged := ai.ApplyAccountRequest(ctx, named); merged.AccountKey == nil || *merged.AccountKey != "chosen" {
            t.Fatalf("a caller's account must win, got %+v", merged)
        }
        if merged := ai.ApplyAccountRequest(ctx, ai.AuthResolutionOverrides{}); !merged.AccountKeySet || merged.AccountKey != nil {
            t.Fatalf("the request scope must supply a present nil key, got %+v", merged)
        }
    }
    
    // okStream builds a stream that starts and finishes with one text block.
    func okStream() *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}}
        _ = stream.Push(ai.StartEvent{Partial: partial})
        _ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "hello", Partial: partial})
        final := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}, StopReason: ai.StopReasonStop}
        _ = stream.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: final})
        return stream
    }
    
    // emptyErrorStream builds a stream whose only outcome is an error with no content.
    func emptyErrorStream(message string) *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{}, StopReason: ai.StopReasonError, ErrorMessage: message}})
        return stream
    }
    
    // partialErrorStream builds a stream that emits content and then fails.
    func partialErrorStream(message string) *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}}
        _ = stream.Push(ai.StartEvent{Partial: partial})
        _ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "half", Partial: partial})
        _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}, StopReason: ai.StopReasonError, ErrorMessage: message}})
        return stream
    422:    _ = stream.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: final})
    429:    _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{}, StopReason: ai.StopReasonError, ErrorMessage: message}})
    439:    _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}, StopReason: ai.StopReasonError, ErrorMessage: message}})
    457:func TestStreamRoutesRotatesBeforeContent(t *testing.T) {
    465:    stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    485:func TestStreamRoutesNeverReplaysAfterContent(t *testing.T) {
    493:    stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    509:func TestStreamRoutesRetriesSameAccountAfterForcedRefresh(t *testing.T) {
    515:    stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    532:func TestStreamRoutesTransientFailureSurfaces(t *testing.T) {
    540:    stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    628:func TestStreamRoutesCancellationWithoutContentSettles(t *testing.T) {
    638:    stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    659:func TestStreamRoutesCancellationAfterContentSettles(t *testing.T) {
    669:    stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    694:func TestStreamRoutesUnpooledWithoutResultSettles(t *testing.T) {
    701:    stream := router.StreamRoutes(ctx, model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    714:func TestStreamRoutesLiveProviderWithoutResultReportsError(t *testing.T) {
    719:    stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
    func collectStream(t *testing.T, stream *ai.AssistantMessageEventStream) (string, *ai.AssistantMessage) {
        t.Helper()
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        var text strings.Builder
        for event := range stream.Events(ctx) {
            if delta, ok := event.(ai.TextDeltaEvent); ok {
                text.WriteString(delta.Delta)
            }
        }
        return text.String(), stream.Result()
    }
    
    func TestStreamRoutesRotatesBeforeContent(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var tried []string
        stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
            tried = append(tried, *selection.AccountKey)
            if *selection.AccountKey == "a" {
                observe(ai.ProviderResponse{Status: 429, Headers: map[string]string{}})
                return emptyErrorStream("insufficient_quota"), nil
            }
            return okStream(), nil
        })
        text, message := collectStream(t, stream)
        if text != "hello" {
            t.Fatalf("expected the sibling's answer, got %q", text)
        }
        if message == nil || message.StopReason != ai.StopReasonStop {
            t.Fatalf("expected a successful result, got %+v", message)
        }
        if len(tried) != 2 {
            t.Fatalf("expected both accounts to be tried, got %v", tried)
        }
    }
    
    func TestStreamRoutesNeverReplaysAfterContent(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
        )
        router := ai.NewAccountRouter(store)
        model := okModel("p", "m")
        var tried []string
        stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
            tried = append(tried, *selection.AccountKey)
            return partialErrorStream("usage_limit_reached"), nil
        })
        text, message := collectStream(t, stream)
        if len(tried) != 1 {
            t.Fatalf("a failure after content must not rotate, tried %v", tried)
        }
        if text != "half" {
            t.Fatalf("the partial answer must be delivered exactly once, got %q", text)
        }
        if message == nil || message.StopReason != ai.StopReasonError {
            t.Fatalf("the failure must be reported, got %+v", message)
        }
    }
    
    func TestStreamRoutesRetriesSameAccountAfterForcedRefresh(t *testing.T) {
        store := newFakeAccountStore(accountInfo("openai-codex", keyOf("a"), true))
        store.put("openai-codex", keyOf("a"), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken("pro")})
        router := ai.NewAccountRouter(store)
        model := okModel("openai-codex", "gpt-5")
        var forces []bool
        stream := router.StreamRoutes(context.Background(), model, ai.AccountRouteRequest{}, func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
            forces = append(forces, force)
            if !force {
                observe(ai.ProviderResponse{Status: 401, Headers: map[string]string{}})
                return emptyErrorStream("invalidated oauth token"), nil
            }
            return okStream(), nil
        })
        text, _ := collectStream(t, stream)
        if text != "hello" {
            t.Fatalf("the refreshed attempt must answer, got %q", text)
        }
        if len(forces) != 2 || forces[0] || !forces[1] {
            t.Fatalf("expected one plain then one forced attempt, got %v", forces)
        }
    }
    
    func TestStreamRoutesTransientFailureSurfaces(t *testing.T) {
        store := newFakeAccountStore(
            accountInfo("p", keyOf("a"), true),
            accountInfo("p", keyOf("b"), false),
    ✓ • 0ms

## Assistant

The router currently prefers the account that last worked for the session, then the selected account. That can keep Luna 6 on Plus even after a free account becomes available again. I’ll put eligible free accounts first, with Plus as the fallback when they’re blocked, exhausted, or rejected.

## Activity

    $ cat >> vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go <<'EOF'
        
        func TestRouterPrefersFreeCodexAccountsForEligibleModels(t *testing.T) {
         for _, tc := range []struct {
          name, provider, model string
          want []string
         }{
          {"luna", "openai-codex", "gpt-6-luna", []string{"free", "plus", "pro"}},
          {"unrestricted", "openai-codex", "gpt-5", []string{"free", "plus", "pro"}},
          {"paid model", "openai-codex", "gpt-5.6-sol", []string{"plus", "pro"}},
          {"terra", "openai-codex", "gpt-6.1-terra", []string{"plus", "pro"}},
          {"spark", "openai-codex", "gpt-5.6-spark", []string{"pro"}},
          {"other provider", "other", "gpt-6-luna", []string{"plus", "pro", "free"}},
         } {
          t.Run(tc.name, func(t *testing.T) {
           store := newFakeAccountStore(accountInfo(tc.provider,keyOf("plus"),true), accountInfo(tc.provider,keyOf("pro"),false), accountInfo(tc.provider,keyOf("free"),false))
           for _,plan:=range []string{"plus","pro","free"} {store.put(tc.provider,keyOf(plan),ai.Credential{Type:ai.CredentialOAuth,Access:codexToken(plan)})}
           accounts,err:=ai.NewAccountRouter(store).Candidates(t.Context(),okModel(tc.provider,tc.model),ai.AccountRouteRequest{})
           if err!=nil {t.Fatal(err)}
           if len(accounts)!=len(tc.want) {t.Fatalf("got %d candidates, want %v",len(accounts),tc.want)}
           for i, want:=range tc.want {if accounts[i].AccountKey==nil || *accounts[i].AccountKey!=want {t.Fatalf("candidate %d = %v, want %s",i,accounts[i].AccountKey,want)}}
          })
         }
        }
        
        func TestStreamRoutesPrefersRecoveredFreeCodexAccountOverPaidAffinity(t *testing.T) {
         store:=newFakeAccountStore(accountInfo("openai-codex",keyOf("plus"),true),accountInfo("openai-codex",keyOf("free"),false))
         for _,plan:=range []string{"plus","free"} {store.put("openai-codex",keyOf(plan),ai.Credential{Type:ai.CredentialOAuth,Access:codexToken(plan)})}
         now:=time.Now()
         store.block("openai-codex",keyOf("free"),"chat",now.Add(time.Minute).UnixMilli())
         router:=ai.NewAccountRouter(store)
         router.SetClock(func()time.Time{return now})
         model:=okModel("openai-codex","gpt-6-luna")
         request:=ai.AccountRouteRequest{SessionID:"free-first-session"}
         var tried []string
         attempt:=func(ctx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(*ai.AssistantMessageEventStream,error){
          tried=append(tried,*selection.AccountKey)
          return okStream(),nil
         }
         text,_:=collectStream(t,router.StreamRoutes(t.Context(),model,request,attempt))
         if text!="hello" || len(tried)!=1 || tried[0]!="plus" {t.Fatalf("blocked free account should use Plus: %q %v",text,tried)}
         key,present:=router.PeekAccountKey("openai-codex",request.SessionID,model.ID)
         if !present || key==nil || *key!="plus" {t.Fatal("successful fallback did not establish Plus affinity")}
         now=now.Add(2*time.Minute)
         tried=nil
         text,_=collectStream(t,router.StreamRoutes(t.Context(),model,request,attempt))
         if text!="hello" || len(tried)!=1 || tried[0]!="free" {t.Fatalf("recovered free account must outrank Plus affinity: %q %v",text,tried)}
        }
        
        func TestStreamRoutesFallsBackFromFreeCodexQuotaToPlus(t *testing.T) {
         store:=newFakeAccountStore(accountInfo("openai-codex",keyOf("plus"),true),accountInfo("openai-codex",keyOf("free"),false))
         for _,plan:=range []string{"plus","free"} {store.put("openai-codex",keyOf(plan),ai.Credential{Type:ai.CredentialOAuth,Access:codexToken(plan)})}
         router:=ai.NewAccountRouter(store)
         model:=okModel("openai-codex","gpt-6-luna")
         var tried []string
         attempt:=func(ctx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(*ai.AssistantMessageEventStream,error){
          tried=append(tried,*selection.AccountKey)
          if *selection.AccountKey=="free" {observe(ai.ProviderResponse{Status:429});return emptyErrorStream("usage_limit_reached"),nil}
          return okStream(),nil
         }
         text,_:=collectStream(t,router.StreamRoutes(t.Context(),model,ai.AccountRouteRequest{},attempt))
         if text!="hello" || len(tried)!=2 || tried[0]!="free" || tried[1]!="plus" {t.Fatalf("expected free then Plus on quota exhaustion: %q %v",text,tried)}
         tried=nil
         text,_=collectStream(t,router.StreamRoutes(t.Context(),model,ai.AccountRouteRequest{},attempt))
         if text!="hello" || len(tried)!=1 || tried[0]!="plus" {t.Fatalf("blocked free account was retried: %q %v",text,tried)}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai -run 'TestRouterPrefersFreeCodex|TestStreamRoutesPrefersRecoveredFreeCodex|TestStreamRoutesFallsBackFromFreeCodex' -count=1
    --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels (0.00s)
        --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels/luna (0.00s)
            account_routing_test.go:762: candidate 0 = 0x7f370799b340, want free
        --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels/unrestricted (0.00s)
            account_routing_test.go:762: candidate 0 = 0x7f370799b420, want free
    --- FAIL: TestStreamRoutesPrefersRecoveredFreeCodexAccountOverPaidAffinity (0.00s)
        account_routing_test.go:797: recovered free account must outrank Plus affinity: "hello" [plus]
    --- FAIL: TestStreamRoutesFallsBackFromFreeCodexQuotaToPlus (0.00s)
        account_routing_test.go:819: expected free then Plus on quota exhaustion: "hello" [plus]
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/ai    0.068s
    FAIL
    ✗ (1) • 16.32s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go');s=p.read_text().replace('// Candidates returns the accounts one request may use, best first: the session\'s pinned\n// account, then the provider\'s selected account, then row order. Accounts whose Codex plan', '// Candidates returns the accounts one request may use, best first: eligible Codex free-tier\n// accounts, then other eligible accounts. Within each group, the session\'s pinned account\n// precedes the provider\'s selected account, then row order. Accounts whose Codex plan').replace('var eligible, unknown []*CredentialAccountInfo','var freeTier, eligible, unknown []*CredentialAccountInfo',1).replace('case *meets:\n\t\t\teligible = append(eligible, account)', '''case *meets:
        			// Personal runtime policy: preserve paid quota when a free account can serve this model, even after a paid fallback established affinity.
        			plan, _ := codexPlanClaim(credential.Access)
        			if slices.Contains(strings.Split(plan, "_"), "free") {
        				freeTier = append(freeTier, account)
        			} else {
        				eligible = append(eligible, account)
        			}''',1).replace('\teligible = append(eligible, unknown...)','\teligible = append(freeTier, eligible...)\n\teligible = append(eligible, unknown...)',1);p.write_text(s)
        p=Path('README.md');s=p.read_text();needle='| Account routing | Per-session/provider/model affinity,';s=s.replace(needle,'| Account routing | Eligible Codex free accounts before paid accounts, per-session/provider/model affinity,');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai -run 'TestRouter|TestStreamRoutes|TestAccount' -count=1
    ok      github.com/MichaelKinsy/PiG/ai    0.201s
    ✓ • 17.71s

## Activity

    $ sed -n '30,75p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go; sed -n '28,43p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '130,143p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; git diff -- vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
            return nil
        }
        return runtime.services.Registry().RegistryCredentialAccounts()
    }
    
    // SelectCredentialAccount makes one stored account the provider's active account and drops this
    // runtime's request affinity, so the choice takes effect on the next request. False when the
    // store cannot or did not select it.
    func (runtime *ModelRuntime) SelectCredentialAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
        store := runtime.credentialAccounts()
        if store == nil {
            return false, nil
        }
        selected, err := store.SelectAccount(ctx, providerID, accountKey)
        if err != nil {
            return false, err
        }
        if selected {
            runtime.clearAccountAffinity(providerID)
            if err := runtime.refreshProviderAvailabilityAfterCredentialChange(ctx, providerID); err != nil {
                return true, err
            }
        }
        return selected, nil
    }
    
    // DeleteCredentialAccount removes one stored account, keeping the provider's others, and drops
    // this runtime's request affinity for that provider.
    func (runtime *ModelRuntime) DeleteCredentialAccount(ctx context.Context, providerID, accountKey string) (bool, error) {
        store := runtime.credentialAccounts()
        if store == nil {
            return false, nil
        }
        removed, err := store.DeleteAccount(ctx, providerID, accountKey)
        if err != nil {
            return false, err
        }
        if removed {
            runtime.clearAccountAffinity(providerID)
            if err := runtime.refreshProviderAvailabilityAfterCredentialChange(ctx, providerID); err != nil {
                return true, err
            }
        }
        return removed, nil
    }
    
        ToAccountKey   *string `json:"toAccountKey"`
        // Reason is "quota", "policy" or "auth".
        Reason    string `json:"reason"`
        Message   string `json:"message"`
        SessionID string `json:"sessionId,omitempty"`
    }
    
    // AccountRouteRequest is what one request tells the router about itself.
    type AccountRouteRequest struct {
        // SessionID pins a session's successes per model, so its next turn reuses the account that
        // worked for it.
        SessionID string
        // APIKeySet reports that the request carries an explicit key. An explicit key is the caller's
        // account decision: routing is bypassed so it is never rotated past.
        APIKeySet bool
    }
    }
    
    // ClearProvider drops this runtime's successful-request affinity for a provider, so an explicit
    // login or account change takes effect on the next request.
    func (r *AccountRouter) ClearProvider(providerID string) {
        if r == nil {
            return
        }
        r.mu.Lock()
        defer r.mu.Unlock()
        delete(r.affinity, providerID)
    }
    
    // PeekAccountKey reports the account this provider's recent requests were pinned to, for
    diff --git a/vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go b/vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    index 1477ee5..fa64250 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    @@ -240,8 +240,9 @@ type accountFailure struct {
         scope   string
     }
    
    -// Candidates returns the accounts one request may use, best first: the session's pinned
    -// account, then the provider's selected account, then row order. Accounts whose Codex plan
    +// Candidates returns the accounts one request may use, best first: eligible Codex free-tier
    +// accounts, then other eligible accounts. Within each group, the session's pinned account
    +// precedes the provider's selected account, then row order. Accounts whose Codex plan
     // cannot serve the model are dropped up front; accounts whose plan cannot be read follow every
     // known-eligible account. It returns nil when the router does not route this request.
     func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request AccountRouteRequest) ([]*CredentialAccountInfo, error) {
    @@ -276,7 +277,7 @@ func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request
         slices.SortStableFunc(accounts, func(left, right *CredentialAccountInfo) int {
             return accountRank(right, pinned, pinnedSet) - accountRank(left, pinned, pinnedSet)
         })
    -    var eligible, unknown []*CredentialAccountInfo
    +    var freeTier, eligible, unknown []*CredentialAccountInfo
         for _, account := range accounts {
             if err := context.Cause(ctx); err != nil {
                 return nil, err
    @@ -297,9 +298,16 @@ func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request
             case meets == nil:
                 unknown = append(unknown, account)
             case *meets:
    -            eligible = append(eligible, account)
    +            // Personal runtime policy: preserve paid quota when a free account can serve this model, even after a paid fallback established affinity.
    +            plan, _ := codexPlanClaim(credential.Access)
    +            if slices.Contains(strings.Split(plan, "_"), "free") {
    +                freeTier = append(freeTier, account)
    +            } else {
    +                eligible = append(eligible, account)
    +            }
             }
         }
    +    eligible = append(freeTier, eligible...)
         eligible = append(eligible, unknown...)
         if len(eligible) == 0 {
             return nil, fmt.Errorf("No stored account meets the plan requirement for %s/%s. Sign in with an eligible account.", providerID, modelID)
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go');s=p.read_text().replace('// SessionID pins a session\'s successes per model, so its next turn reuses the account that\n\t// worked for it.', '// SessionID pins a session\'s successes per model within the router\'s plan priority.').replace('// ClearProvider drops this runtime\'s successful-request affinity for a provider, so an explicit\n// login or account change takes effect on the next request.', '// ClearProvider drops this runtime\'s successful-request affinity for a provider, so the next\n// request uses the selected account within the router\'s plan priority.');p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go');s=p.read_text().replace('// runtime\'s request affinity, so the choice takes effect on the next request. False when the\n// store cannot or did not select it.', '// runtime\'s request affinity. The next request prefers that account within the router\'s plan\n// priority. False when the store cannot or did not select it.');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go
        git diff --check
    ✓ • 0ms

## Assistant

The routing tests now pass: Luna 6 chooses free first, falls back to Plus when free quota is exhausted, and returns to free after its block expires. Paid-only models still use eligible paid accounts. The full AI race checks and executable build are running.

## Activity

    $ python - <<'PY'
        import os, subprocess, tempfile
        with tempfile.TemporaryDirectory(prefix='ompig-free-routing-') as profile:
         env=os.environ.copy()
         for key in ('PIG_CODING_AGENT_DIR','PI_CODING_AGENT_DIR','OMPIG_SLIM_CODING_AGENT_DIR'):
          env[key]=profile+'/agent'
         for key in ('PIG_HOME','PI_HOME'):
          env[key]=profile+'/library'
         env.update(PI_SKIP_VERSION_CHECK='1',PI_TELEMETRY='0')
         raise SystemExit(subprocess.call(['go','test','-race','-mod=vendor','github.com/MichaelKinsy/PiG/ai'],env=env))
        PY
    ok      github.com/MichaelKinsy/PiG/ai    16.982s
    ✓ • 56.45s

## Activity

    $ make build
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 1m 09s

## Activity

    $ git diff --check; git diff --stat; ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -6
     README.md                                          |  4 +-
     .../MichaelKinsy/PiG/ai/account_routing.go         | 23 ++++--
     .../MichaelKinsy/PiG/ai/account_routing_test.go    | 92 ++++++++++++++++++++++
     .../PiG/coding/model_runtime_accounts.go           |  4 +-
     .../PiG/internal/codingagent/interactive.go        |  3 +
     .../PiG/internal/codingagent/interactive_input.go  |  1 +
     .../internal/codingagent/interactive_runtime.go    |  4 +
     vendor/github.com/MichaelKinsy/PiG/tui/editor.go   |  5 ++
     .../MichaelKinsy/PiG/tui/editor_status.go          | 48 +++++++++--
     .../MichaelKinsy/PiG/tui/usage_report.go           |  8 +-
     10 files changed, 171 insertions(+), 21 deletions(-)
        PID     ELAPSED %CPU COMMAND
    3220477       00:11  113 wakatime-cli-li
    3220882       00:00  100 codebase-memory
    3220883       00:00  100 ps
     188473    23:21:34 26.8 chrome-headless
     188463    23:21:34 18.3 chrome-headless
    ✓ • 0ms

## Assistant

Implemented. Eligible free-tier Codex accounts now take priority over Plus, including for Luna 6.

Plus remains the fallback when free accounts are unavailable. Once a free account’s block expires, routing prefers it again. Paid-only model restrictions remain enforced.

AI race tests and build passed.

## User

free tier account reset on 30days window, not 7 days

## Assistant

I’ll check how free-tier reset windows are labeled and make sure the display uses the 30-day window and the provider’s reset time.

## Activity

    $ sed -n '270,365p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; rg -n '7d|7.day|weekly|secondary|2592000|30d|604800' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go | head -95; sed -n '75,150p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; rg -n 'planType|plan_type|buildCodex|codexBuild|codexWindow' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go
        }
        return parsed
    }
    
    // codexBuildWindowLabel returns the id/label pair for a window span.
    func codexBuildWindow(seconds int64) (string, string) {
        return usageWindowFromSeconds(seconds)
    }
    
    // codexResolveResetTime resolves a window's absolute reset time.
    func codexResolveResetTime(window *codexUsageWindow, nowMs int64) *int64 {
        if window == nil {
            return nil
        }
        if window.resetAt != nil {
            value := *window.resetAt
            millis := value
            if value <= 1_000_000_000_000 {
                millis = value * 1000
            }
            if !math.IsNaN(millis) && !math.IsInf(millis, 0) {
                converted := int64(millis)
                return &converted
            }
        }
        if window.resetAfterSeconds != nil {
            value := nowMs + int64(*window.resetAfterSeconds*1000)
            return &value
        }
        return nil
    }
    
    // codexBuildUsageWindow renders a parsed window as a UsageWindow.
    func codexBuildUsageWindow(window *codexUsageWindow, key string, nowMs int64) UsageWindow {
        resetsAt := codexResolveResetTime(window, nowMs)
        if window != nil && window.limitWindowSeconds != nil {
            id, label := codexBuildWindow(*window.limitWindowSeconds)
            duration := *window.limitWindowSeconds * 1000
            return UsageWindow{ID: id, Label: label, DurationMs: &duration, ResetsAt: resetsAt}
        }
        label := "Secondary window"
        if key == "primary" {
            label = "Primary window"
        }
        return UsageWindow{ID: key, Label: label, ResetsAt: resetsAt}
    }
    
    // codexBuildUsageAmount renders a window's percent amount.
    func codexBuildUsageAmount(window *codexUsageWindow) UsageAmount {
        if window == nil || window.usedPercent == nil {
            return UsageAmount{Unit: UsageUnitPercent}
        }
        return usagePercentAmount(usageClampPercent(*window.usedPercent))
    }
    
    // codexBuildUsageStatus keeps a 100% advisory window a warning while the
    // backend still reports the account as allowed.
    func codexBuildUsageStatus(usedFraction *float64, explicitlyAllowed bool) UsageStatus {
        status := usageStatusFor(usedFraction)
        if status == UsageStatusExhausted && explicitlyAllowed {
            return UsageStatusWarning
        }
        return status
    }
    
    // codexRequestAllowed reports whether Codex will still serve this meter.
    func codexRequestAllowed(allowed, limitReached *bool, creditOverage bool) bool {
        if creditOverage && limitReached != nil && *limitReached {
            return true
        }
        return allowed != nil && *allowed && limitReached != nil && !*limitReached
    }
    
    // buildCodexUsageLimit renders a plan window limit.
    func buildCodexUsageLimit(key string, window *codexUsageWindow, allowed, limitReached *bool, creditOverage bool, nowMs int64) UsageLimit {
        usageWindow := codexBuildUsageWindow(window, key, nowMs)
        amount := codexBuildUsageAmount(window)
        return UsageLimit{
            ID:     "openai-codex:" + key,
            Label:  usageWindow.Label,
            Scope:  UsageScope{Provider: "openai-codex", WindowID: usageWindow.ID, Shared: true},
            Window: &usageWindow,
            Amount: amount,
            Status: codexBuildUsageStatus(amount.UsedFraction, codexRequestAllowed(allowed, limitReached, creditOverage)),
        }
    }
    
    // codexAdditionalLimitSlug derives the meter slug used in limit ids.
    func codexAdditionalLimitSlug(limitName, meteredName string) string {
        probe := strings.ToLower(limitName + " " + meteredName)
        if strings.Contains(probe, "spark") || strings.Contains(probe, "bengalfox") {
            return "spark"
        }
        source := meteredName
        if source == "" {
            source = limitName
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:167:    case "anthropic:7d":
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:168:        return "Claude weekly"
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:169:    case "anthropic:7d:opus":
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:170:        return "Claude Opus weekly"
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:171:    case "anthropic:7d:sonnet":
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:172:        return "Claude Sonnet weekly"
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:874:// compactWindowTag renders a compact duration tag for a window (`7d`, `1d`,
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:86:        "x-codex-secondary-used-percent":   "95",
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:87:        "x-codex-secondary-window-minutes": "10080",
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:96:    if report.Limits[0].Window.ID != "5h" || report.Limits[1].Window.ID != "7d" {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:102:        t.Fatalf("secondary status = %q, want warning", report.Limits[1].Status)
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:180:                "kind":      "weekly_scoped",
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:198:        t.Fatal("scoped weekly row missing")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:201:        t.Fatal("a model-scoped weekly row must not be shared")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:256:        t.Fatal("weekly pool missing")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:258:    if pool.Window == nil || pool.Window.ID != "weekly" {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:397:        weeklyTotalCount:   &zero,
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go:399:        weeklyStatus:       &unlimited,
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:58:    secondary    *codexUsageWindow
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:78:    secondary     *codexUsageWindow
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:123:        secondary: parseCodexUsageWindow(rateLimit["secondary_window"]),
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:137:    if entry.primary == nil && entry.secondary == nil && entry.allowed == nil && entry.limitReached == nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:253:        parsed.secondary = parseCodexUsageWindow(rateLimit["secondary_window"])
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:267:    if parsed.primary == nil && parsed.secondary == nil && parsed.allowed == nil && parsed.limitReached == nil &&
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:576:// parseCodexRateLimitHeaders parses `x-codex-{primary,secondary}-*` headers.
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:594:    secondary := parseWindow("secondary")
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:595:    if primary == nil && secondary == nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:607:    if secondary != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:608:        limits = append(limits, buildCodexUsageLimit("secondary", secondary, allowed, limitReached, false, nowMs))
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:717:    if parsed != nil && parsed.secondary != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:718:        limits = append(limits, buildCodexUsageLimit("secondary", parsed.secondary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:735:            if extra.secondary != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go:736:                limits = append(limits, buildCodexAdditionalUsageLimit("secondary", slug, displayName, extra.secondary, accountID, extra.limitName, extra.meteredName, extra.allowed, extra.limitReached, nowMs))
        allowed       *bool
        limitReached  *bool
        primary       *codexUsageWindow
        secondary     *codexUsageWindow
        additional    []codexAdditionalUsage
        spendCap      *codexSpendCap
        creditOverage bool
        raw           any
    }
    
    // parseCodexUsageWindow reads one window object.
    func parseCodexUsageWindow(value any) *codexUsageWindow {
        record, ok := usageRecord(value)
        if !ok {
            return nil
        }
        window := &codexUsageWindow{}
        if number, ok := usageNumber(record["used_percent"]); ok {
            window.usedPercent = &number
        }
        if number, ok := usageNumber(record["limit_window_seconds"]); ok {
            seconds := int64(math.Trunc(number))
            window.limitWindowSeconds = &seconds
        }
        if number, ok := usageNumber(record["reset_after_seconds"]); ok {
            window.resetAfterSeconds = &number
        }
        if number, ok := usageNumber(record["reset_at"]); ok {
            window.resetAt = &number
        }
        if window.usedPercent == nil && window.limitWindowSeconds == nil && window.resetAfterSeconds == nil && window.resetAt == nil {
            return nil
        }
        return window
    }
    
    // parseCodexAdditionalUsage reads one additional meter entry.
    func parseCodexAdditionalUsage(value any) *codexAdditionalUsage {
        record, ok := usageRecord(value)
        if !ok {
            return nil
        }
        rateLimit, ok := usageRecord(record["rate_limit"])
        if !ok {
            return nil
        }
        entry := &codexAdditionalUsage{
            primary:   parseCodexUsageWindow(rateLimit["primary_window"]),
            secondary: parseCodexUsageWindow(rateLimit["secondary_window"]),
        }
        if name, ok := usageString(record, "limit_name"); ok {
            entry.limitName = name
        }
        if name, ok := usageString(record, "metered_feature"); ok {
            entry.meteredName = name
        }
        if flag, ok := usageBool(rateLimit, "allowed"); ok {
            entry.allowed = &flag
        }
        if flag, ok := usageBool(rateLimit, "limit_reached"); ok {
            entry.limitReached = &flag
        }
        if entry.primary == nil && entry.secondary == nil && entry.allowed == nil && entry.limitReached == nil {
            return nil
        }
        return entry
    }
    
    // parseCodexSpendCap reads `spend_control.individual_limit`.
    func parseCodexSpendCap(payload map[string]any) *codexSpendCap {
        spendControl, ok := usageRecord(payload["spend_control"])
        if !ok {
            return nil
        }
        individual, ok := usageRecord(spendControl["individual_limit"])
        if !ok {
    74:    planType      string
    234:    if planType, ok := usageString(record, "plan_type"); ok {
    235:        parsed.planType = planType
    274:// codexBuildWindowLabel returns the id/label pair for a window span.
    275:func codexBuildWindow(seconds int64) (string, string) {
    302:// codexBuildUsageWindow renders a parsed window as a UsageWindow.
    303:func codexBuildUsageWindow(window *codexUsageWindow, key string, nowMs int64) UsageWindow {
    306:        id, label := codexBuildWindow(*window.limitWindowSeconds)
    317:// codexBuildUsageAmount renders a window's percent amount.
    318:func codexBuildUsageAmount(window *codexUsageWindow) UsageAmount {
    325:// codexBuildUsageStatus keeps a 100% advisory window a warning while the
    327:func codexBuildUsageStatus(usedFraction *float64, explicitlyAllowed bool) UsageStatus {
    343:// buildCodexUsageLimit renders a plan window limit.
    344:func buildCodexUsageLimit(key string, window *codexUsageWindow, allowed, limitReached *bool, creditOverage bool, nowMs int64) UsageLimit {
    345:    usageWindow := codexBuildUsageWindow(window, key, nowMs)
    346:    amount := codexBuildUsageAmount(window)
    353:        Status: codexBuildUsageStatus(amount.UsedFraction, codexRequestAllowed(allowed, limitReached, creditOverage)),
    395:// buildCodexAdditionalUsageLimit renders one additional meter window.
    396:func buildCodexAdditionalUsageLimit(key, slug, displayName string, window *codexUsageWindow, accountID, limitName, meteredName string, allowed, limitReached *bool, nowMs int64) UsageLimit {
    397:    usageWindow := codexBuildUsageWindow(window, key, nowMs)
    398:    amount := codexBuildUsageAmount(window)
    412:        Status: codexBuildUsageStatus(amount.UsedFraction, codexRequestAllowed(allowed, limitReached, false)),
    416:// buildCodexSpendCapLimit renders the per-user credit cap.
    417:func buildCodexSpendCapLimit(cap *codexSpendCap, accountID string, nowMs int64) UsageLimit {
    428:            status = codexBuildUsageStatus(cap.usedFraction, true)
    605:        limits = append(limits, buildCodexUsageLimit("primary", primary, allowed, limitReached, false, nowMs))
    608:        limits = append(limits, buildCodexUsageLimit("secondary", secondary, allowed, limitReached, false, nowMs))
    695:    planType := ""
    697:        planType = parsed.planType
    699:    if planType == "" {
    701:            planType, _ = usageString(record, "plan_type")
    715:        limits = append(limits, buildCodexUsageLimit("primary", parsed.primary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
    718:        limits = append(limits, buildCodexUsageLimit("secondary", parsed.secondary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
    733:                limits = append(limits, buildCodexAdditionalUsageLimit("primary", slug, displayName, extra.primary, accountID, extra.limitName, extra.meteredName, extra.allowed, extra.limitReached, nowMs))
    736:                limits = append(limits, buildCodexAdditionalUsageLimit("secondary", slug, displayName, extra.secondary, accountID, extra.limitName, extra.meteredName, extra.allowed, extra.limitReached, nowMs))
    740:            limits = append(limits, buildCodexSpendCapLimit(parsed.spendCap, accountID, nowMs))
    775:        "planType":    planType,
    ✓ • 0ms

## Activity

    $ rg -n 'func usageWindowFromSeconds' vendor/github.com/MichaelKinsy/PiG/ai/*.go; sed -n '680,725p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; sed -n '58,112p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go; rg -n '7d|604800|weekly' vendor/github.com/MichaelKinsy/PiG/ai/account* vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go; sed -n '150,205p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_shared.go:256:func usageWindowFromSeconds(seconds int64) (id string, label string) {
            <-daybreakCh
            if response.Status == 401 || response.Status == 403 {
                return nil, NewUsageHTTPError(strings.TrimSpace("Codex usage endpoint returned "+usageFormatCount(int64(response.Status))+" "+usageStatusText(response)), response.Status)
            }
            usageLogWarn(fctx, "Codex usage request failed", map[string]any{"status": response.Status, "provider": params.Provider})
            return nil, nil
        }
        payload, ok := response.JSON()
        if !ok {
            <-daybreakCh
            usageLogWarn(fctx, "Codex usage request error", map[string]any{"provider": params.Provider, "error": "invalid JSON"})
            return nil, nil
        }
    
        parsed := parseCodexUsagePayload(payload)
        planType := ""
        if parsed != nil {
            planType = parsed.planType
        }
        if planType == "" {
            if record, ok := usageRecord(payload); ok {
                planType, _ = usageString(record, "plan_type")
            }
        }
    
        creditOverage := parsed != nil && parsed.creditOverage
        limits := make([]UsageLimit, 0, 4)
        meterStates := map[string]any{}
        planAllowed, planLimitReached := codexPlanMeterState(parsed, creditOverage)
        if planAllowed != nil {
            meterStates["chat"] = map[string]any{"allowed": *planAllowed, "limitReached": *planLimitReached}
        } else {
            meterStates["chat"] = map[string]any{}
        }
        if parsed != nil && parsed.primary != nil {
            limits = append(limits, buildCodexUsageLimit("primary", parsed.primary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
        }
        if parsed != nil && parsed.secondary != nil {
            limits = append(limits, buildCodexUsageLimit("secondary", parsed.secondary, parsed.allowed, parsed.limitReached, creditOverage, nowMs))
        }
        if parsed != nil {
            for _, extra := range parsed.additional {
                slug := codexAdditionalLimitSlug(extra.limitName, extra.meteredName)
                displayName := codexAdditionalDisplayName(slug, extra.limitName)
                state := map[string]any{}
                if extra.allowed != nil {
        }
        limit.Status = ""
        if got := UsageLimitStatus(limit); got != UsageStatusExhausted {
            t.Fatalf("status = %q, want exhausted", got)
        }
    }
    
    func TestSortUsageLimitsExhaustedFirst(t *testing.T) {
        low := 0.1
        high := 0.8
        full := 1.0
        limits := []UsageLimit{
            {ID: "a", Label: "A", Amount: UsageAmount{UsedFraction: &low, Unit: UsageUnitPercent}},
            {ID: "b", Label: "B", Amount: UsageAmount{UsedFraction: &full, Unit: UsageUnitPercent}},
            {ID: "c", Label: "C", Amount: UsageAmount{UsedFraction: &high, Unit: UsageUnitPercent}},
        }
        SortUsageLimits(limits)
        got := []string{limits[0].ID, limits[1].ID, limits[2].ID}
        want := []string{"b", "c", "a"}
        if strings.Join(got, ",") != strings.Join(want, ",") {
            t.Fatalf("order = %v, want %v", got, want)
        }
    }
    
    func TestCodexRateLimitHeadersProduceWindows(t *testing.T) {
        headers := map[string]string{
            "x-codex-primary-used-percent":     "40",
            "x-codex-primary-window-minutes":   "300",
            "x-codex-secondary-used-percent":   "95",
            "x-codex-secondary-window-minutes": "10080",
        }
        report := parseCodexRateLimitHeaders(headers, 1_700_000_000_000, new(int(200)))
        if report == nil {
            t.Fatal("report is nil")
        }
        if len(report.Limits) != 2 {
            t.Fatalf("limits = %d, want 2", len(report.Limits))
        }
        if report.Limits[0].Window.ID != "5h" || report.Limits[1].Window.ID != "7d" {
            t.Fatalf("windows = %q/%q", report.Limits[0].Window.ID, report.Limits[1].Window.ID)
        }
        // A successful response marks both windows explicitly allowed, so even a
        // rounded 100% window stays a warning rather than a block.
        if report.Limits[1].Status != UsageStatusWarning {
            t.Fatalf("secondary status = %q, want warning", report.Limits[1].Status)
        }
        if fraction, ok := UsageUsedFraction(report.Limits[0]); !ok || fraction != 0.4 {
            t.Fatalf("primary fraction = %v/%v", fraction, ok)
        }
    }
    
    func TestCodexUsagePayloadKeepsSparkMeterSeparate(t *testing.T) {
        payload := map[string]any{
            "plan_type": "pro",
            "rate_limit": map[string]any{
            resetsAt            int64
        }
        var buckets []*bucket
        byKey := map[string]*bucket{}
        for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
            if report.Provider != provider {
                continue
            }
            for _, limit := range report.Limits {
                if limit.Scope.ModelID != "" && limit.Scope.ModelID != model {
                    continue
                }
                fraction, ok := ai.UsageUsedFraction(limit)
                if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
                    continue
                }
                windowID, label := limit.Scope.WindowID, limit.Label
                if limit.Window != nil {
                    windowID = limit.Window.ID
                    label = windowID
                }
                if label == "" {
                    label = limit.ID
                }
                key := limit.ID + "|" + windowID
                if limit.Scope.SharedGroup != "" {
                    key = limit.Scope.SharedGroup + "|" + windowID
                }
                b := byKey[key]
                if b == nil {
                    b = &bucket{label: tui.SanitizeDisplayLine(label), weighted: true, unit: limit.Amount.Unit}
                    byKey[key] = b
                    buckets = append(buckets, b)
                }
                if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
                    b.resetsAt = *limit.Window.ResetsAt
                }
                b.sum += fraction
                b.count++
                capacity := limit.Amount.Limit
                if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
                    b.weighted = false
                } else {
                    b.used += fraction * *capacity
                    b.capacity += *capacity
                }
            }
        }
        result := make([]promptQuotaWindow, 0, len(buckets))
        for _, b := range buckets {
            used := b.sum / float64(b.count)
            if b.weighted && b.capacity > 0 {
                used = b.used / b.capacity
            }
            result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used)), resetsAt: b.resetsAt})
        }
    ✓ • 0ms

## Activity

    $ sed -n '256,291p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_shared.go; sed -n '575,612p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; rg -n 'func Test.*Codex' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go; sed -n '1,26p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go; rg -n 'httptest|UsageFetchContext|fetchCodexUsage' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go | head -25
    func usageWindowFromSeconds(seconds int64) (id string, label string) {
        if seconds >= 86_400 {
            days := int64(math.Round(float64(seconds) / 86_400))
            return strconv.FormatInt(days, 10) + "d", usageWindowDurationLabel(days, "day")
        }
        hours := seconds / 3600
        if hours < 1 {
            hours = 1
        }
        return strconv.FormatInt(hours, 10) + "h", usageWindowDurationLabel(hours, "hour")
    }
    
    // usageClampPercent clamps a 0..100 utilization value.
    func usageClampPercent(value float64) float64 { return math.Min(math.Max(value, 0), 100) }
    
    // usageIntFraction clamps a value into 0..1.
    func usageIntFraction(value float64) float64 { return math.Min(math.Max(value, 0), 1) }
    
    // usageAbsoluteURL joins a base URL and a path with exactly one slash.
    func usageAbsoluteURL(base, path string) string {
        if base == "" {
            return path
        }
        if path == "" {
            return base
        }
        return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
    }
    
    // usageHTTPURL is the part of a parsed URL a probe's endpoint derivation uses.
    type usageHTTPURL struct {
        Scheme   string
        Host     string
        Hostname string
    }
    
    
    // parseCodexRateLimitHeaders parses `x-codex-{primary,secondary}-*` headers.
    func parseCodexRateLimitHeaders(headers map[string]string, nowMs int64, responseStatus *int) *UsageReport {
        parseWindow := func(key string) *codexUsageWindow {
            usedPercent, ok := usageNumber(headers["x-codex-"+key+"-used-percent"])
            if !ok {
                return nil
            }
            window := &codexUsageWindow{usedPercent: &usedPercent}
            if minutes, ok := usageNumber(headers["x-codex-"+key+"-window-minutes"]); ok {
                seconds := int64(math.Trunc(minutes * 60))
                window.limitWindowSeconds = &seconds
            }
            if resetAt, ok := usageNumber(headers["x-codex-"+key+"-reset-at"]); ok {
                window.resetAt = &resetAt
            }
            return window
        }
        primary := parseWindow("primary")
        secondary := parseWindow("secondary")
        if primary == nil && secondary == nil {
            return nil
        }
        var allowed, limitReached *bool
        if responseStatus != nil && *responseStatus >= 200 && *responseStatus < 300 {
            allowed = new(true)
            limitReached = new(false)
        }
        limits := make([]UsageLimit, 0, 2)
        if primary != nil {
            limits = append(limits, buildCodexUsageLimit("primary", primary, allowed, limitReached, false, nowMs))
        }
        if secondary != nil {
            limits = append(limits, buildCodexUsageLimit("secondary", secondary, allowed, limitReached, false, nowMs))
        }
        return &UsageReport{
            Provider:  "openai-codex",
            FetchedAt: nowMs,
    82:func TestCodexRateLimitHeadersProduceWindows(t *testing.T) {
    109:func TestCodexUsagePayloadKeepsSparkMeterSeparate(t *testing.T) {
    155:func TestCodexSpendCapDrivesCreditSeats(t *testing.T) {
    package ai
    
    import (
        "context"
        "encoding/json"
        "math"
        "net/http"
        "net/http/httptest"
        "strings"
        "sync/atomic"
        "testing"
    )
    
    func usageTestReport(limits ...UsageLimit) *UsageReport {
        return &UsageReport{Provider: "test", FetchedAt: 1, Limits: limits}
    }
    
    func usageTestProvider(fetch func(context.Context, UsageFetchParams, UsageFetchContext) (*UsageReport, error)) *UsageProvider {
        return &UsageProvider{ID: "test", FetchUsage: fetch}
    }
    
    func TestUsageUsedFractionPrecedence(t *testing.T) {
        used := 25.0
        limit := 200.0
        remainingFraction := 0.9
        cases := []struct {
    8:    "net/http/httptest"
    18:func usageTestProvider(fetch func(context.Context, UsageFetchParams, UsageFetchContext) (*UsageReport, error)) *UsageProvider {
    283:    server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
    295:    }, UsageFetchContext{HTTPClient: server.Client()})
    441:    server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
    451:    }, UsageFetchContext{HTTPClient: server.Client()})
    461:    server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
    473:    }, UsageFetchContext{HTTPClient: server.Client()})
    ✓ • 0ms

## Assistant

The parser already uses the provider-reported duration, so it supports `30d` and keeps it separate from a Plus account’s `7d` window. The bar can still show `7d` if that window has less capacity remaining across your accounts. I’ll add regression coverage for the mixed 30-day and 7-day case.

## Assistant

The new tests check that a free account’s `30d` quota stays separate from Plus `7d` totals and that the reset countdown uses the provider’s timestamp. No parser change was needed; the checks are still compiling.

## Activity

    $ ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -6; git diff --check
        PID     ELAPSED %CPU COMMAND
    3230630       00:00 50.0 ps
    3230474       00:05 49.0 compile
    3229747       00:24 42.2 compile
    3230487       00:04 33.4 compile
    3230620       00:00 33.3 vet
    ✓ • 231ms

## Assistant

The parser test passed: both the account endpoint and response headers preserve a `30d` window and the provider’s reset timestamp. The mixed-account prompt test is still running.

## Activity

    $ cat >> vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go <<'EOF'
        
        func TestCodexFreeUsagePreservesThirtyDayWindowAndProviderReset(t *testing.T) {
         const now = int64(1_700_000_000_000)
         const monthSeconds = int64(30*24*60*60)
         // A partially elapsed monthly window must retain the provider's next reset,
         // rather than calculating a new reset 30 days after the fetch.
         reset := now + 10*24*60*60*1000
         parsed := parseCodexUsagePayload(map[string]any{
          "plan_type":"free",
          "rate_limit":map[string]any{
           "allowed":true,"limit_reached":false,
           "primary_window":map[string]any{"used_percent":40.0,"limit_window_seconds":float64(monthSeconds),"reset_at":float64(reset/1000)},
          },
         })
         if parsed==nil || parsed.primary==nil {t.Fatal("free monthly quota was not parsed")}
         limit:=buildCodexUsageLimit("primary",parsed.primary,parsed.allowed,parsed.limitReached,false,now)
         headers:=parseCodexRateLimitHeaders(map[string]string{
          "x-codex-primary-used-percent":"40",
          "x-codex-primary-window-minutes":"43200",
          "x-codex-primary-reset-at":"1700864000",
         },now,new(200))
         if headers==nil || len(headers.Limits)!=1 {t.Fatal("monthly response headers were not parsed")}
         for _,tc:=range []struct{name string;limit UsageLimit}{{"account endpoint",limit},{"response headers",headers.Limits[0]}} {
          t.Run(tc.name,func(t *testing.T){
           window:=tc.limit.Window
           if window==nil || window.ID!="30d" || window.DurationMs==nil || *window.DurationMs!=monthSeconds*1000 {t.Fatalf("monthly quota was mislabeled: %+v",window)}
           if window.ResetsAt==nil || *window.ResetsAt!=reset {t.Fatalf("provider reset was changed: %+v",window)}
          })
         }
        }
        EOF
        cat >> vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go <<'EOF'
        
        func TestPromptQuotaKeepsFreeMonthlyAndPlusWeeklyTotalsSeparate(t *testing.T) {
         monthlyReset:=time.Now().Add(20*24*time.Hour).UnixMilli()
         weeklyReset:=time.Now().Add(4*24*time.Hour).UnixMilli()
         freeA,freeB:=promptTestLimit("30d",.8,monthlyReset),promptTestLimit("30d",.6,monthlyReset)
         plus:=promptTestLimit("7d",.2,weeklyReset)
         // Codex can use the same meter ID for different account window durations.
         freeA.ID,freeB.ID,plus.ID="openai-codex:primary","openai-codex:primary","openai-codex:primary"
         snapshot:=ai.UsageSnapshot{Reports:[]ai.UsageReport{
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"free"},Limits:[]ai.UsageLimit{freeA}},
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"free"},Limits:[]ai.UsageLimit{freeB}},
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"plus"},Limits:[]ai.UsageLimit{plus}},
         }}
         windows:=promptQuotaWindows(snapshot,"openai-codex","gpt-6-luna")
         if len(windows)!=2 || windows[0].label!="30d" || math.Abs(windows[0].used-.7)>1e-9 || windows[0].resetsAt!=monthlyReset || windows[1].label!="7d" || windows[1].used!=.2 {t.Fatalf("free monthly quota was mixed with Plus weekly quota: %+v",windows)}
         m:=&InteractiveMode{opts:InteractiveModeOptions{Model:&ai.Model{ID:"gpt-6-luna",ProviderMeta:ai.ProviderMetadata{ProviderID:"openai-codex"}}},promptQuota:&promptQuotaState{loaded:true,snapshot:snapshot}}
         label:=m.promptQuotaLabel(100)
         if !strings.HasPrefix(label,"30d ") || !strings.Contains(label,"30% left") || !strings.Contains(label,"reset 20d") || strings.Contains(label,"7d") {t.Fatalf("wrong monthly window or reset in prompt: %q",label)}
        }
        EOF
        python - <<'PY'
        from pathlib import Path
        p=Path('README.md');s=p.read_text();needle='Quotas refresh every minute in the background; unavailable accounts mark the total as partial.';s=s.replace(needle,'Window durations and reset times come from the provider: free-tier 30-day quotas stay separate from Plus 7-day quotas when totaling accounts. '+needle);p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_test.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestCodexFreeUsagePreservesThirtyDayWindowAndProviderReset|TestPromptQuotaKeepsFreeMonthlyAndPlusWeeklyTotalsSeparate' -count=1
    ok      github.com/MichaelKinsy/PiG/ai    0.258s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.202s
    ✓ • 2m 51s

## Assistant

Understood: free-tier quota uses `30d`, while Plus uses `7d`. The code already keeps these windows separate and uses the provider’s reset timestamp. Added regression tests; both passed.

## User

it still using the bar from plus account when im on luna 6

## Assistant

The bar still chooses the most-used window across all tiers. I’ll make it follow the tier used to route Luna 6, totaling accounts within that tier, so a Plus window doesn’t override the free-tier bar.

## Activity

    $ cat vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; sed -n '210,235p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_usage.go; sed -n '258,280p' vendor/github.com/MichaelKinsy/PiG/ai/usage_contract.go; sed -n '180,225p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go; rg -n 'UsageAccountKey:' vendor/github.com/MichaelKinsy/PiG -g '*.go'; sed -n '745,790p' vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go; rg -n 'accountKey|planType' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go | tail -25
    package codingagent
    
    import (
        "context"
        "fmt"
        "math"
        "time"
    
        "github.com/MichaelKinsy/PiG/ai"
        "github.com/MichaelKinsy/PiG/tui"
        "github.com/MichaelKinsy/PiG/tui/widthx"
    )
    
    // Personal runtime presentation: the prompt shows aggregate subscription quota across accounts.
    const promptQuotaInterval = time.Minute
    
    type promptQuotaState struct {
        snapshot ai.UsageSnapshot
        fetching bool
        loaded   bool
        failed   bool
    }
    
    // Polling and network work belong to the run's joined background task group. Only the owner loop reads or publishes presentation state.
    func (m *InteractiveMode) startPromptQuota(ctx context.Context) {
        m.refreshPromptQuota(ctx)
        m.backgroundTasks.Go(func() {
            ticker := time.NewTicker(promptQuotaInterval)
            defer ticker.Stop()
            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if err := m.postToMain(ctx, func() { m.refreshPromptQuota(ctx) }); err != nil {
                        return
                    }
                }
            }
        })
    }
    
    func (m *InteractiveMode) refreshPromptQuota(ctx context.Context) {
        service := m.opts.UsageService
        if service == nil {
            service = m.usageService
        }
        if service == nil || ctx.Err() != nil {
            return
        }
        if m.promptQuota == nil {
            m.promptQuota = &promptQuotaState{}
        }
        state := m.promptQuota
        if state.fetching {
            return
        }
        state.fetching = true
        m.backgroundTasks.Go(func() {
            fetchCtx, cancel := context.WithTimeout(ctx, 2*UsageRequestTimeout)
            defer cancel()
            snapshot, err := service.FetchSnapshot(fetchCtx, true)
            _ = m.postToMain(ctx, func() {
                // A replacement session owns a new quota service and state.
                if m.promptQuota != state || ctx.Err() != nil {
                    return
                }
                state.snapshot, state.loaded, state.failed, state.fetching = snapshot, true, err != nil, false
                if m.editor != nil {
                    m.editor.Invalidate()
                }
                m.requestRender()
            })
        })
    }
    
    func (m *InteractiveMode) promptQuotaLabel(maxWidth int) string {
        if m.opts.Model == nil || maxWidth <= 0 {
            return ""
        }
        provider := m.opts.Model.ProviderID()
        state := m.promptQuota
        if state == nil || !state.loaded || state.failed {
            return ""
        }
        status := ""
        windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)
        if len(windows) > 0 {
            window := windows[0]
            for _, candidate := range windows[1:] {
                if candidate.used > window.used {
                    window = candidate
                }
            }
            remaining := 1 - window.used
            reset := "reset unknown"
            if window.resetsAt > 0 {
                delta := max(time.Minute, time.Until(time.UnixMilli(window.resetsAt)))
                minutes := int(math.Ceil(delta.Minutes()))
                if minutes < 60 {
                    reset = fmt.Sprintf("reset %dm", minutes)
                } else if minutes < 24*60 {
                    reset = fmt.Sprintf("reset %dh%02dm", minutes/60, minutes%60)
                } else {
                    reset = fmt.Sprintf("reset %dd%dh", minutes/(24*60), minutes/60%24)
                }
            }
            suffix := fmt.Sprintf(" %.0f%% left · %s", remaining*100, reset)
            partial := state.failed
            for _, account := range state.snapshot.UnavailableAccounts {
                partial = partial || account.Provider == provider
            }
            if partial {
                suffix += " (partial)"
            }
            prefix := window.label + " "
            barWidth := min(24, maxWidth-widthx.VisibleWidth(prefix+suffix))
            if barWidth < 12 {
                return ""
            }
            quotaStatus := ai.UsageStatusOK
            if window.used >= 1 {
                quotaStatus = ai.UsageStatusExhausted
            } else if window.used >= .8 {
                quotaStatus = ai.UsageStatusWarning
            }
            limit := ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: &window.used}, Status: quotaStatus}
            status = prefix + tui.RenderUsageBar(tui.ActiveTheme(), limit, barWidth) + suffix
        }
        if widthx.VisibleWidth(status) <= maxWidth {
            return status
        }
        return ""
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
            weighted            bool
            unit                ai.UsageUnit
            resetsAt            int64
        }
        var buckets []*bucket
        byKey := map[string]*bucket{}
        for _, report := range tui.CollapseSharedUsageReports(snapshot.Reports) {
            if report.Provider != provider {
                continue
            }
            for _, limit := range report.Limits {
                if limit.Scope.ModelID != "" && limit.Scope.ModelID != model {
                    continue
                }
                fraction, ok := ai.UsageUsedFraction(limit)
                if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
                    continue
                }
                windowID, label := limit.Scope.WindowID, limit.Label
                if limit.Window != nil {
                    windowID = limit.Window.ID
                    label = windowID
                }
                if label == "" {
                    label = limit.ID
                }
                key := limit.ID + "|" + windowID
                if limit.Scope.SharedGroup != "" {
                    key = limit.Scope.SharedGroup + "|" + windowID
                }
                b := byKey[key]
                if b == nil {
                    b = &bucket{label: tui.SanitizeDisplayLine(label), weighted: true, unit: limit.Amount.Unit}
                    byKey[key] = b
                    buckets = append(buckets, b)
                }
                if limit.Window != nil && limit.Window.ResetsAt != nil && *limit.Window.ResetsAt > time.Now().UnixMilli() && (b.resetsAt == 0 || *limit.Window.ResetsAt < b.resetsAt) {
                    b.resetsAt = *limit.Window.ResetsAt
                }
                b.sum += fraction
                b.count++
                capacity := limit.Amount.Limit
                if capacity == nil || *capacity <= 0 || math.IsNaN(*capacity) || math.IsInf(*capacity, 0) || b.unit != limit.Amount.Unit {
                    b.weighted = false
                } else {
                    b.used += fraction * *capacity
                    b.capacity += *capacity
                }
            }
        }
        result := make([]promptQuotaWindow, 0, len(buckets))
        for _, b := range buckets {
            used := b.sum / float64(b.count)
            if b.weighted && b.capacity > 0 {
                used = b.used / b.capacity
            }
            result = append(result, promptQuotaWindow{label: b.label, used: max(0, min(1, used)), resetsAt: b.resetsAt})
        }
        return result
    }
        if m.opts.UsageAccountKey == nil {
            return nil, false
        }
        return m.opts.UsageAccountKey(provider)
    }
    
    // resolveUsageActiveAccount reads the routed account identity for provider,
    // passing the session's key only when the host reported one.
    func (m *InteractiveMode) resolveUsageActiveAccount(ctx context.Context, service UsageQuotaService, provider string) (ai.UsageActiveAccount, bool) {
        key, present := m.usageAccountKey(provider)
        if !present {
            return service.ActiveAccount(ctx, provider)
        }
        return service.ActiveAccount(ctx, provider, key)
    }
    
    // backgroundContext returns the run's background context, or the process
    // context when the mode has not started one (component tests).
    func (m *InteractiveMode) backgroundContext() context.Context {
        if m.backgroundCtx != nil {
            return m.backgroundCtx
        }
        return context.Background()
    }
    
    // isShuttingDownMode reports whether the mode is tearing down or already torn
    // UsageActiveAccount is the session-routing identity of the account a provider
    // currently uses.
    type UsageActiveAccount struct {
        AccountKey *string
        Email      string
        AccountID  string
        OrgID      string
        OrgName    string
    }
    
    // usageStatusFor maps a used fraction to the upstream thresholds.
    func usageStatusFor(usedFraction *float64) UsageStatus {
        if usedFraction == nil {
            return UsageStatusUnknown
        }
        if *usedFraction >= 1 {
            return UsageStatusExhausted
        }
        if *usedFraction >= 0.9 {
            return UsageStatusWarning
        }
        return UsageStatusOK
    }
        return runtime.credentialAccounts()
    }
    
    // UsageAccountKey reports the account this session's requests for provider are routed to, for the
    // usage board's in-use marker. It reads the runtime's affinity only; present is false when the
    // session has not routed a request for this provider and model, which asks the quota service for
    // the store's default account.
    func (s *Session) UsageAccountKey(providerID string) (*string, bool) {
        if s == nil {
            return nil, false
        }
        modelID := ""
        if model := s.Model(); model != nil && model.ProviderMeta.ProviderID == providerID {
            modelID = model.ID
        }
        return s.ModelRuntime().PeekAccountKey(providerID, s.ID(), modelID)
    }
    
    // emitAccountRotation publishes one account handoff as the Session event upstream's
    // account_rotation reports (agent-session.ts:237,537). The event carries both nullable account
    // keys unchanged, so an identity-less account stays distinguishable from an unknown one.
    func (s *Session) emitAccountRotation(rotation ai.AccountRotation) {
        s.emitEvent(agent.AccountRotationEvent{
            ProviderID:     rotation.ProviderID,
            FromAccountKey: rotation.FromAccountKey,
            ToAccountKey:   rotation.ToAccountKey,
            Reason:         rotation.Reason,
            Message:        rotation.Message,
        })
    }
    vendor/github.com/MichaelKinsy/PiG/coding/cli/interactive_build.go:153:        UsageAccountKey:         session.UsageAccountKey,
    vendor/github.com/MichaelKinsy/PiG/coding/cli/main.go:1564:        UsageAccountKey:         codingSess.UsageAccountKey,
        if resetCredits != nil && resetCredits.AvailableCount > 0 {
            credits, available, ok := listCodexResetCredits(ctx, fctx, accessToken, accountID, params.BaseURL)
            if ok {
                if len(credits) > 0 {
                    details := make([]UsageResetCreditDetail, 0, len(credits))
                    for _, credit := range credits {
                        status := credit.Status
                        if status == "" {
                            status = "available"
                        }
                        if status != "available" {
                            continue
                        }
                        details = append(details, UsageResetCreditDetail{
                            GrantedAt: credit.GrantedAt,
                            ExpiresAt: credit.ExpiresAt,
                            Status:    credit.Status,
                        })
                    }
                    resetCredits.Credits = details
                }
                // The detail endpoint is authoritative after expiry/redeem.
                resetCredits.AvailableCount = available
            } else {
                usageLogWarn(fctx, "Codex reset credits detail fetch failed", nil)
            }
        }
    
        daybreak := (<-daybreakCh).granted
        metadata := map[string]any{
            "planType":    planType,
            "email":       email,
            "accountId":   accountID,
            "meterStates": meterStates,
        }
        if planAllowed != nil {
            metadata["allowed"] = *planAllowed
        }
        if planLimitReached != nil {
            metadata["limitReached"] = *planLimitReached
        }
        if daybreak {
            metadata["daybreak"] = true
        }
        report := &UsageReport{
            Provider:     "openai-codex",
    271:            accountKey := entry.account.AccountKey
    272:            if accountKey != nil {
    273:                copied := *accountKey
    285:func (service *UsageService) ActiveAccount(ctx context.Context, providerID string, accountKey ...*string) (ai.UsageActiveAccount, bool) {
    290:    if len(accountKey) > 0 {
    293:        key = accountKey[0]
    424:func (service *UsageService) readCredentialSync(providerID string, accountKey *string) *ai.Credential {
    425:    credential, err := service.readCredentialFor(context.Background(), providerID, accountKey)
    433:func (service *UsageService) readCredentialFor(ctx context.Context, providerID string, accountKey *string) (*ai.Credential, error) {
    435:        return service.options.Accounts.ReadAccount(ctx, providerID, accountKey)
    487:func usageAccountResult(result usageProbeResult, accountKey *string) usageProbeResult {
    488:    if result.report != nil && accountKey != nil {
    489:        report := usageWithAccountKey(*result.report, *accountKey)
    519:func accountOverrides(accountKey *string) ai.AuthResolutionOverrides {
    520:    return ai.WithAccountKey(ai.AuthResolutionOverrides{}, accountKey)
    537:func usageWithAccountKey(report ai.UsageReport, accountKey string) ai.UsageReport {
    538:    if current, ok := report.Metadata["accountKey"].(string); ok && current == accountKey {
    545:    metadata["accountKey"] = accountKey
    ✓ • 55ms

## Activity

    $ sed -n '455,535p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '740,805p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '535,548p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/usage_service.go
    
    func metadataBool(report ai.UsageReport, key string) bool {
        value, ok := report.Metadata[key]
        if !ok {
            return false
        }
        flag, _ := value.(bool)
        return flag
    }
    
    // FormatCodexUsageReportLabel qualifies a Codex account label only when a
    // sibling report shares its email: multiple workspaces on one login otherwise
    // render as identical accounts, so the org id (then account id) disambiguates
    // them.
    func FormatCodexUsageReportLabel(report ai.UsageReport, peers []ai.UsageReport, base string) string {
        email := metadataString(report, "email")
        collision := false
        if email != "" {
            for _, peer := range peers {
                if peer.Provider == report.Provider && peer.FetchedAt == report.FetchedAt &&
                    metadataString(peer, "email") == email && !sameReportIdentity(peer, report) {
                    collision = true
                    break
                }
            }
        }
        rawOrg := ""
        if collision {
            rawOrg = metadataString(report, "orgId")
            if rawOrg == "" {
                rawOrg = metadataString(report, "accountId")
            }
            if rawOrg == "" {
                rawOrg = metadataString(report, "accountKey")
            }
            if rawOrg == "" {
                rawOrg = "account " + strconv.Itoa(indexOfReport(peers, report)+1)
            }
        }
        suffix := ""
        if rawOrg != "" && rawOrg != base {
            suffix = " (" + SanitizeDisplayLine(rawOrg) + ")"
        }
        return SanitizeDisplayLine(base) + suffix
    }
    
    // sameReportIdentity reports whether two reports describe one account: the
    // identity fields the usage views disambiguate by.
    func sameReportIdentity(left, right ai.UsageReport) bool {
        return left.Provider == right.Provider &&
            metadataString(left, "accountKey") == metadataString(right, "accountKey") &&
            metadataString(left, "accountId") == metadataString(right, "accountId") &&
            metadataString(left, "orgId") == metadataString(right, "orgId")
    }
    
    // indexOfReport returns the position of report in peers, or -1.
    func indexOfReport(peers []ai.UsageReport, report ai.UsageReport) int {
        for i, peer := range peers {
            if sameReportIdentity(peer, report) && peer.FetchedAt == report.FetchedAt {
                return i
            }
        }
        return -1
    }
    
    // FormatUsageAccountLabel labels one account of a provider, disambiguating
    // Codex workspaces.
    func FormatUsageAccountLabel(report ai.UsageReport, peers []ai.UsageReport, base string) string {
        if report.Provider != "openai-codex" {
            return SanitizeDisplayLine(base)
        }
        return FormatCodexUsageReportLabel(report, peers, base)
    }
    
    // ─── Active account identity ─────────────────────────────────────────────────
    
    func normalizeIdentityValue(value string) string {
        trimmed := strings.TrimSpace(value)
        if trimmed == "" {
            return ""
        }
        Reports             []ai.UsageReport
        UnavailableAccounts []UsageUnavailableAccountView
    }
    
    // GroupUsageReportsByPlan keeps quota totals and account columns separate for
    // each provider subscription plan.
    func GroupUsageReportsByPlan(reports []ai.UsageReport, unavailable []UsageUnavailableAccountView) []UsagePlanGroup {
        type planKey struct{ provider, plan string }
        order := make([]planKey, 0, len(reports)+len(unavailable))
        groups := map[planKey]*UsagePlanGroup{}
        getPlan := func(provider, plan string) *UsagePlanGroup {
            key := planKey{provider: provider, plan: plan}
            if group, ok := groups[key]; ok {
                return group
            }
            group := &UsagePlanGroup{Provider: provider}
            groups[key] = group
            order = append(order, key)
            return group
        }
        for _, report := range reports {
            group := getPlan(report.Provider, normalizeUsagePlan(metadataString(report, "planType")))
            group.Reports = append(group.Reports, report)
        }
        for _, account := range unavailable {
            group := getPlan(account.Provider, "")
            group.UnavailableAccounts = append(group.UnavailableAccounts, account)
        }
        // A provider with any named plan labels its unnamed bucket "Unknown plan".
        named := map[string]bool{}
        for key := range groups {
            if key.plan != "" {
                named[key.provider] = true
            }
        }
        out := make([]UsagePlanGroup, 0, len(order))
        for _, key := range order {
            group := groups[key]
            label := ""
            switch {
            case key.plan != "":
                label = titleCasePlan(key.plan)
            case named[key.provider]:
                label = "Unknown plan"
            }
            name := SanitizeDisplayLine(FormatProviderName(key.provider))
            if label != "" {
                name += " · " + label
            }
            group.Name = name
            out = append(out, *group)
        }
        return out
    }
    
    // normalizeUsagePlan lowercases a provider plan name and strips provider
    // prefixes so sibling subscriptions group together.
    func normalizeUsagePlan(raw string) string {
        plan := strings.ToLower(SanitizeDisplayLine(raw))
        if rest, ok := strings.CutPrefix(plan, "chatgpt"); ok {
            // Upstream requires at least one separator after the provider prefix:
            // `chatgptplus` names no plan, while `chatgpt-plus` is Plus.
            if trimmed := strings.TrimLeft(rest, " \t_-"); trimmed != rest {
                plan = trimmed
            }
        }
    
    // usageWithAccountKey stamps the store account key on a report.
    func usageWithAccountKey(report ai.UsageReport, accountKey string) ai.UsageReport {
        if current, ok := report.Metadata["accountKey"].(string); ok && current == accountKey {
            return report
        }
        metadata := make(map[string]any, len(report.Metadata)+1)
        for key, value := range report.Metadata {
            metadata[key] = value
        }
        metadata["accountKey"] = accountKey
        report.Metadata = metadata
        return report
    }
    ✓ • 0ms

## Activity

    $ cat >> vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go <<'EOF'
        
        func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts(t *testing.T) {
         monthlyReset:=time.Now().Add(20*24*time.Hour).UnixMilli()
         weeklyReset:=time.Now().Add(4*24*time.Hour).UnixMilli()
         snapshot:=ai.UsageSnapshot{Reports:[]ai.UsageReport{
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"plus","accountKey":"plus"},Limits:[]ai.UsageLimit{promptTestLimit("7d",.95,weeklyReset)}},
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"free","accountKey":"free-a"},Limits:[]ai.UsageLimit{promptTestLimit("30d",.2,monthlyReset)}},
          {Provider:"openai-codex",Metadata:map[string]any{"planType":"free","accountKey":"free-b"},Limits:[]ai.UsageLimit{promptTestLimit("30d",.4,monthlyReset)}},
         }}
         m:=&InteractiveMode{opts:InteractiveModeOptions{Model:&ai.Model{ID:"gpt-6-luna",ProviderMeta:ai.ProviderMetadata{ProviderID:"openai-codex"}}},promptQuota:&promptQuotaState{loaded:true,snapshot:snapshot},editor:tui.NewEditor()}
         m.editor.TopRightStatus=m.promptQuotaLabel
         assertFree:=func(){t.Helper();label:=m.promptQuotaLabel(100);if !strings.HasPrefix(label,"30d ") || !strings.Contains(label,"70% left") || strings.Contains(label,"7d"){t.Fatalf("Plus quota overrode the combined free tier: %q",label)}}
         // Before any request has established affinity, follow the free-first policy.
         assertFree()
         key:="free-a"
         m.opts.UsageAccountKey=func(string)(*string,bool){return &key,true}
         assertFree()
         m.editor.Render(100);m.editor.NeedsRedraw()
         // An actual paid fallback changes the bar immediately, using the same snapshot.
         key="plus"
         if !m.editor.IsDirty(){t.Fatal("routed tier change did not invalidate the border")}
         if label:=m.promptQuotaLabel(100);!strings.HasPrefix(label,"7d ") || !strings.Contains(label,"5% left"){t.Fatalf("paid fallback retained the free bar: %q",label)}
         key="free-b"
         assertFree()
         // Do not substitute Plus when the routed free account has no fetched report.
         key="missing-free"
         if label:=m.promptQuotaLabel(100);label!=""{t.Fatalf("missing routed quota fell back to another tier: %q",label)}
         // A paid-only model must choose its eligible tier before a request is routed.
         m.opts.UsageAccountKey=nil
         m.opts.Model.ID="gpt-6.1-terra"
         if label:=m.promptQuotaLabel(100);!strings.HasPrefix(label,"7d "){t.Fatalf("paid-only model selected free quota: %q",label)}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        go test -mod=vendor github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts$' -count=1
    --- FAIL: TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts (0.00s)
        interactive_prompt_quota_test.go:218: Plus quota overrode the combined free tier: "7d \x1b[38;2;205;154;34m██████████████████████▓\x1b[39m\x1b[38;2;126;136;142m░\x1b[39m 5% left · reset 4d0h"
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/internal/codingagent    0.097s
    FAIL
    ✗ (1) • 16.80s

## Activity

    $ sed -n '730,746p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '795,818p' vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; rg -n 'UsagePlanGroup\{' vendor/github.com/MichaelKinsy/PiG | head; sed -n '54,91p' vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go
    // fields (reason, accountKey, ambient) this view ignores.
    type UsageUnavailableAccountView struct {
        Provider string
        Label    string
    }
    
    // UsagePlanGroup is one provider subscription plan's reports.
    type UsagePlanGroup struct {
        Provider            string
        Name                string
        Reports             []ai.UsageReport
        UnavailableAccounts []UsageUnavailableAccountView
    }
    
    // GroupUsageReportsByPlan keeps quota totals and account columns separate for
    // each provider subscription plan.
    func GroupUsageReportsByPlan(reports []ai.UsageReport, unavailable []UsageUnavailableAccountView) []UsagePlanGroup {
    // normalizeUsagePlan lowercases a provider plan name and strips provider
    // prefixes so sibling subscriptions group together.
    func normalizeUsagePlan(raw string) string {
        plan := strings.ToLower(SanitizeDisplayLine(raw))
        if rest, ok := strings.CutPrefix(plan, "chatgpt"); ok {
            // Upstream requires at least one separator after the provider prefix:
            // `chatgptplus` names no plan, while `chatgpt-plus` is Plus.
            if trimmed := strings.TrimLeft(rest, " \t_-"); trimmed != rest {
                plan = trimmed
            }
        }
        var b strings.Builder
        b.Grow(len(plan))
        lastUnderscore := false
        for _, char := range plan {
            if char == '_' || char == '-' {
                if !lastUnderscore {
                    b.WriteByte(' ')
                }
                lastUnderscore = true
                continue
            }
            lastUnderscore = false
            b.WriteRune(char)
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:749:    groups := map[planKey]*UsagePlanGroup{}
    vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go:755:        group := &UsagePlanGroup{Provider: provider}
    // Only openai-codex OAuth credentials carry a plan; every other provider and every API key is
    // eligible.
    func AccountMeetsModelPlan(provider, modelID string, credential Credential) *bool {
        if provider != "openai-codex" || credential.Type != CredentialOAuth {
            return new(true)
        }
        id := AccountPolicyModelID(modelID)
        requirement := "none"
        switch {
        case strings.Contains(id, "-spark"):
            requirement = "pro"
        case codexPaidModels[id] || strings.HasPrefix(id, "gpt-6.1-terra"):
            requirement = "paid"
        }
        if requirement == "none" {
            return new(true)
        }
        plan, ok := codexPlanClaim(credential.Access)
        if !ok {
            return nil
        }
        switch {
        case plan == "prolite" || plan == "pro_lite":
            return new(requirement == "paid")
        case codexProPlan.MatchString(plan):
            return new(true)
        case codexPaidPlan.MatchString(plan):
            return new(requirement == "paid")
        case codexFreePlan.MatchString(plan):
            return new(false)
        }
        return nil
    }
    
    // codexPlanClaim reads payload["https://api.openai.com/auth"].chatgpt_plan_type from the
    // stored OAuth access token. Stored JWT claims are an external boundary, not an entitlement
    // guarantee from the backend, so every failure reports "unknown" rather than "eligible".
    func codexPlanClaim(access string) (string, bool) {
    ✓ • 0ms

## Assistant

The new regression test reproduced it: a nearly exhausted Plus `7d` window overrides the free tier’s `30d` total. The fix filters by the routed tier first, then totals its accounts. It also switches immediately to Plus on a paid fallback and hides the bar if the routed account has no fetched quota report.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'ai/account_policy.go';s=p.read_text();needle='\tid := AccountPolicyModelID(modelID)\n\trequirement := "none"';a=s.index(needle,s.index('func AccountMeetsModelPlan'));s=s[:a]+'''\tplan, _ := codexPlanClaim(credential.Access)
        	return CodexPlanMeetsModel(modelID, plan)
        }
        
        // CodexPlanMeetsModel applies the stored-account entitlement policy to a plan reported by the usage endpoint. Unknown restricted plans remain probeable.
        func CodexPlanMeetsModel(modelID, plan string) *bool {
        '''+s[a:];s=s.replace('\tplan, ok := codexPlanClaim(credential.Access)\n\tif !ok {\n\t\treturn nil\n\t}\n\tswitch {','\tplan = codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")\n\tplan = strings.TrimPrefix(plan, "chatgpt_")\n\tswitch {',1);p.write_text(s)
        p=root/'tui/usage_display.go';s=p.read_text().replace('type UsagePlanGroup struct {\n\tProvider', 'type UsagePlanGroup struct {\n\t// Plan is the normalized subscription tier, empty when the provider reports none.\n\tPlan string\n\tProvider',1).replace('group := &UsagePlanGroup{Provider: provider}','group := &UsagePlanGroup{Provider: provider, Plan: plan}',1);p.write_text(s)
        p=root/'internal/codingagent/interactive_prompt_quota.go';s=p.read_text().replace('windows := promptQuotaWindows(state.snapshot, provider, m.opts.Model.ID)', '''snapshot := state.snapshot
        	snapshot.Reports = m.promptQuotaTierReports(snapshot.Reports, provider, m.opts.Model.ID)
        	windows := promptQuotaWindows(snapshot, provider, m.opts.Model.ID)''',1)
        needle='type promptQuotaWindow struct {'
        helper='''// Total accounts in the routed subscription tier. Before the first routed request, mirror the router's free-first entitlement policy without reading credentials on the render loop.
        func (m *InteractiveMode) promptQuotaTierReports(reports []ai.UsageReport, provider, model string) []ai.UsageReport {
        	if provider != "openai-codex" {
        		return reports
        	}
        	groups := tui.GroupUsageReportsByPlan(reports, nil)
        	key, routed := m.usageAccountKey(provider)
        	if routed {
        		for _, group := range groups {
        			if group.Provider != provider {
        				continue
        			}
        			for _, report := range group.Reports {
        				accountKey, keyed := report.Metadata["accountKey"].(string)
        				if key != nil && keyed && accountKey == *key || key == nil && !keyed {
        					return group.Reports
        				}
        			}
        		}
        		return nil
        	}
        	var fallback []ai.UsageReport
        	for _, group := range groups {
        		if group.Provider != provider {
        			continue
        		}
        		meets := ai.CodexPlanMeetsModel(model, group.Plan)
        		if meets != nil && !*meets {
        			continue
        		}
        		if group.Plan == "free" && meets != nil && *meets {
        			return group.Reports
        		}
        		if fallback == nil {
        			fallback = group.Reports
        		}
        	}
        	return fallback
        }
        
        '''
        s=s.replace(needle,helper+needle,1);p.write_text(s)
        p=Path('README.md');s=p.read_text().replace('The prompt’s top-right border shows one live quota bar for the selected model’s provider, aggregated across accounts. It chooses the quota window with the least remaining capacity and shows the next account reset countdown.', 'The prompt’s top-right border shows one live quota bar for the selected model’s provider, totaling accounts within the routed subscription tier. Before the first Codex request, it prefers an eligible free tier. It follows paid fallbacks and switches back when routing returns to free, choosing the quota window with the least remaining capacity within that tier and showing its next account reset countdown.');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/internal/codingagent -run '^TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.052s
    ✓ • 49.85s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go');s=p.read_text().replace('\tvar fallback []ai.UsageReport\n\tfor _, group', '\tvar fallback []ai.UsageReport\n\tfallbackKnown := false\n\tfor _, group',1).replace('\t\tif fallback == nil {\n\t\t\tfallback = group.Reports\n\t\t}', '\t\tif fallback == nil || !fallbackKnown && meets != nil && *meets {\n\t\t\tfallback = group.Reports\n\t\t\tfallbackKnown = meets != nil && *meets\n\t\t}',1);p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go');s=p.read_text();a=s.index('func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts');head,tail=s[:a],s[a:];tail=tail.replace('Reports: []ai.UsageReport{','Reports: []ai.UsageReport{\n\t\t{Provider: "openai-codex", Metadata: map[string]any{"planType": "unknown", "accountKey": "unknown"}, Limits: []ai.UsageLimit{promptTestLimit("unknown", .99, weeklyReset)}},',1).replace('"planType": "free", "accountKey": "free-b"','"planType": "ChatGPT-Free", "accountKey": "free-b"',1);s=head+tail;p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        git diff --check
    ✓ • 0ms

## Activity

    $ git diff --check; git diff -- vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go; sed -n '136,191p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    diff --git a/vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go b/vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go
    index f7eaa8f..1cb0d23 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go
    @@ -57,6 +57,12 @@ func AccountMeetsModelPlan(provider, modelID string, credential Credential) *boo
         if provider != "openai-codex" || credential.Type != CredentialOAuth {
             return new(true)
         }
    +    plan, _ := codexPlanClaim(credential.Access)
    +    return CodexPlanMeetsModel(modelID, plan)
    +}
    +
    +// CodexPlanMeetsModel applies the stored-account entitlement policy to a plan reported by the usage endpoint. Unknown restricted plans remain probeable.
    +func CodexPlanMeetsModel(modelID, plan string) *bool {
         id := AccountPolicyModelID(modelID)
         requirement := "none"
         switch {
    @@ -68,10 +74,8 @@ func AccountMeetsModelPlan(provider, modelID string, credential Credential) *boo
         if requirement == "none" {
             return new(true)
         }
    -    plan, ok := codexPlanClaim(credential.Access)
    -    if !ok {
    -        return nil
    -    }
    +    plan = codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")
    +    plan = strings.TrimPrefix(plan, "chatgpt_")
         switch {
         case plan == "prolite" || plan == "pro_lite":
             return new(requirement == "paid")
    diff --git a/vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go b/vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go
    index db9500f..7d4ee63 100644
    --- a/vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go
    +++ b/vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go
    @@ -735,6 +735,8 @@ type UsageUnavailableAccountView struct {
    
     // UsagePlanGroup is one provider subscription plan's reports.
     type UsagePlanGroup struct {
    +    // Plan is the normalized subscription tier, empty when the provider reports none.
    +    Plan                string
         Provider            string
         Name                string
         Reports             []ai.UsageReport
    @@ -752,7 +754,7 @@ func GroupUsageReportsByPlan(reports []ai.UsageReport, unavailable []UsageUnavai
             if group, ok := groups[key]; ok {
                 return group
             }
    -        group := &UsagePlanGroup{Provider: provider}
    +        group := &UsagePlanGroup{Provider: provider, Plan: plan}
             groups[key] = group
             order = append(order, key)
             return group
    }
    
    // Total accounts in the routed subscription tier. Before the first routed request, mirror the router's free-first entitlement policy without reading credentials on the render loop.
    func (m *InteractiveMode) promptQuotaTierReports(reports []ai.UsageReport, provider, model string) []ai.UsageReport {
        if provider != "openai-codex" {
            return reports
        }
        groups := tui.GroupUsageReportsByPlan(reports, nil)
        key, routed := m.usageAccountKey(provider)
        if routed {
            for _, group := range groups {
                if group.Provider != provider {
                    continue
                }
                for _, report := range group.Reports {
                    accountKey, keyed := report.Metadata["accountKey"].(string)
                    if key != nil && keyed && accountKey == *key || key == nil && !keyed {
                        return group.Reports
                    }
                }
            }
            return nil
        }
        var fallback []ai.UsageReport
        fallbackKnown := false
        for _, group := range groups {
            if group.Provider != provider {
                continue
            }
            meets := ai.CodexPlanMeetsModel(model, group.Plan)
            if meets != nil && !*meets {
                continue
            }
            if group.Plan == "free" && meets != nil && *meets {
                return group.Reports
            }
            if fallback == nil || !fallbackKnown && meets != nil && *meets {
                fallback = group.Reports
                fallbackKnown = meets != nil && *meets
            }
        }
        return fallback
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
    ✓ • 93ms

## Assistant

The focused prompt tests pass. The full race checks and build are still running. The bar now follows the routed tier while keeping the total across that tier’s accounts.

## Activity

    $ make build
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 2m 50s

## Activity

    $ python - <<'PY'
        import os, subprocess, tempfile
        with tempfile.TemporaryDirectory(prefix='ompig-quota-tier-') as profile:
         env=os.environ.copy()
         for key in ('PIG_CODING_AGENT_DIR','PI_CODING_AGENT_DIR','OMPIG_SLIM_CODING_AGENT_DIR'):
          env[key]=profile+'/agent'
         for key in ('PIG_HOME','PI_HOME'):
          env[key]=profile+'/library'
         env.update(PI_SKIP_VERSION_CHECK='1',PI_TELEMETRY='0')
         raise SystemExit(subprocess.call(['go','test','-race','-mod=vendor','github.com/MichaelKinsy/PiG/ai','github.com/MichaelKinsy/PiG/tui','github.com/MichaelKinsy/PiG/internal/codingagent'],env=env))
        PY
    ok      github.com/MichaelKinsy/PiG/ai    19.895s
    ok      github.com/MichaelKinsy/PiG/tui    1.765s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    1.517s
    ✓ • 3m 47s

## Assistant

Fixed—the bar was still comparing all tiers. It now totals Free accounts for Luna 6’s `30d` bar, showing Plus quota when routing falls back to Plus.

Race checks and build passed. Restart with `./bin/ompig-slim` to load the updated build.

## User

when switch to  6.1 sol, free tier account doesn't have access to that, why dont it update the bar ?

## Assistant

I’ll check the model access rules and the bar’s model-switch handling so GPT-6.1 Sol uses an eligible paid tier instead of the Free bar.

## Activity

    $ sed -n '30,99p' vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go; sed -n '136,191p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go; rg -n 'gpt-6.1-sol|paid model|terra' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go | tail -25; sed -n '182,205p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go
    // account-policy.ts's `replace(/[\s-]+/g, "_")` does.
    var codexPlanSeparators = lazyregexp.New(`[` + jsSpaceBody + `-]+`)
    
    // codexPaidModels are the catalog models a ChatGPT paid plan is required for. Node-compatible
    // projection of pi-catalog rules/runtime/behavior.kdl (account-policy.ts CODEX_PAID_MODELS).
    var codexPaidModels = map[string]bool{
        "gpt-5.6":          true,
        "gpt-5.6-sol":      true,
        "gpt-5.6-sol-pro":  true,
        "gpt-5.6-luna":     true,
        "gpt-5.6-luna-pro": true,
    }
    
    // AccountPolicyModelID is the canonical bare model identity used by OMP's model-policy
    // blocks: the last path segment, JavaScript-trimmed and lowercased.
    func AccountPolicyModelID(modelID string) string {
        index := strings.LastIndex(modelID, "/")
        return jsstring.ToLower(jsstring.Trim(modelID[index+1:]))
    }
    
    // AccountMeetsModelPlan reports whether a known Codex plan satisfies the catalog requirement
    // for modelID. A nil result means the plan cannot be read (an unparseable token or an unknown
    // tier): the account stays probeable, ranked after every known-eligible account.
    //
    // Only openai-codex OAuth credentials carry a plan; every other provider and every API key is
    // eligible.
    func AccountMeetsModelPlan(provider, modelID string, credential Credential) *bool {
        if provider != "openai-codex" || credential.Type != CredentialOAuth {
            return new(true)
        }
        plan, _ := codexPlanClaim(credential.Access)
        return CodexPlanMeetsModel(modelID, plan)
    }
    
    // CodexPlanMeetsModel applies the stored-account entitlement policy to a plan reported by the usage endpoint. Unknown restricted plans remain probeable.
    func CodexPlanMeetsModel(modelID, plan string) *bool {
        id := AccountPolicyModelID(modelID)
        requirement := "none"
        switch {
        case strings.Contains(id, "-spark"):
            requirement = "pro"
        case codexPaidModels[id] || strings.HasPrefix(id, "gpt-6.1-terra"):
            requirement = "paid"
        }
        if requirement == "none" {
            return new(true)
        }
        plan = codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")
        plan = strings.TrimPrefix(plan, "chatgpt_")
        switch {
        case plan == "prolite" || plan == "pro_lite":
            return new(requirement == "paid")
        case codexProPlan.MatchString(plan):
            return new(true)
        case codexPaidPlan.MatchString(plan):
            return new(requirement == "paid")
        case codexFreePlan.MatchString(plan):
            return new(false)
        }
        return nil
    }
    
    // codexPlanClaim reads payload["https://api.openai.com/auth"].chatgpt_plan_type from the
    // stored OAuth access token. Stored JWT claims are an external boundary, not an entitlement
    // guarantee from the backend, so every failure reports "unknown" rather than "eligible".
    func codexPlanClaim(access string) (string, bool) {
        parts := strings.Split(access, ".")
        if len(parts) < 2 {
            return "", false
        }
    }
    
    // Total accounts in the routed subscription tier. Before the first routed request, mirror the router's free-first entitlement policy without reading credentials on the render loop.
    func (m *InteractiveMode) promptQuotaTierReports(reports []ai.UsageReport, provider, model string) []ai.UsageReport {
        if provider != "openai-codex" {
            return reports
        }
        groups := tui.GroupUsageReportsByPlan(reports, nil)
        key, routed := m.usageAccountKey(provider)
        if routed {
            for _, group := range groups {
                if group.Provider != provider {
                    continue
                }
                for _, report := range group.Reports {
                    accountKey, keyed := report.Metadata["accountKey"].(string)
                    if key != nil && keyed && accountKey == *key || key == nil && !keyed {
                        return group.Reports
                    }
                }
            }
            return nil
        }
        var fallback []ai.UsageReport
        fallbackKnown := false
        for _, group := range groups {
            if group.Provider != provider {
                continue
            }
            meets := ai.CodexPlanMeetsModel(model, group.Plan)
            if meets != nil && !*meets {
                continue
            }
            if group.Plan == "free" && meets != nil && *meets {
                return group.Reports
            }
            if fallback == nil || !fallbackKnown && meets != nil && *meets {
                fallback = group.Reports
                fallbackKnown = meets != nil && *meets
            }
        }
        return fallback
    }
    
    type promptQuotaWindow struct {
        label    string
        used     float64
        resetsAt int64
    }
    
    // Sum capacities where the provider supplies absolute values; percentage-only accounts contribute equal capacity. Different windows remain separate.
    func promptQuotaWindows(snapshot ai.UsageSnapshot, provider, model string) []promptQuotaWindow {
        type bucket struct {
            label               string
            sum, used, capacity float64
            count               int
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:163:    // Plus serves paid models but not a Spark model, which needs pro.
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:179:    if meets := ai.AccountMeetsModelPlan("openai", "gpt-6.1-sol", free); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:182:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-6.1-sol", ai.Credential{Type: ai.CredentialAPIKey, Key: "sk-test"}); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:189:    quota := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached", 429, nil)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:194:    if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "too many requests, please slow down", 429, nil); block != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:197:    if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "internal server error", 500, nil); block != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:205:    if auth := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "invalidated oauth token", 401, nil); auth == nil || auth.Scope != "auth" {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:209:    denied := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", `The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.`, 400, nil)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:210:    if denied == nil || denied.Scope != "model-policy:gpt-6.1-sol" {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:214:    if slow := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached; try again in 5 min", 429, nil); slow == nil || slow.Until < now+299_000 {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:743:        {"paid model", "openai-codex", "gpt-5.6-sol", []string{"plus", "pro"}},
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:744:        {"terra", "openai-codex", "gpt-6.1-terra", []string{"plus", "pro"}},
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go:242:    m.opts.Model.ID = "gpt-6.1-terra"
    
    // UsageAccountKey reports the account this session's requests for provider are routed to, for the
    // usage board's in-use marker. It reads the runtime's affinity only; present is false when the
    // session has not routed a request for this provider and model, which asks the quota service for
    // the store's default account.
    func (s *Session) UsageAccountKey(providerID string) (*string, bool) {
        if s == nil {
            return nil, false
        }
        modelID := ""
        if model := s.Model(); model != nil && model.ProviderMeta.ProviderID == providerID {
            modelID = model.ID
        }
        return s.ModelRuntime().PeekAccountKey(providerID, s.ID(), modelID)
    }
    
    // emitAccountRotation publishes one account handoff as the Session event upstream's
    // account_rotation reports (agent-session.ts:237,537). The event carries both nullable account
    // keys unchanged, so an identity-less account stays distinguishable from an unknown one.
    func (s *Session) emitAccountRotation(rotation ai.AccountRotation) {
        s.emitEvent(agent.AccountRotationEvent{
            ProviderID:     rotation.ProviderID,
            FromAccountKey: rotation.FromAccountKey,
            ToAccountKey:   rotation.ToAccountKey,
    ✓ • 0ms

## Assistant

GPT-6.1 Sol was missing from the paid-only model rules. I also found that the bar accepted the last routed tier without checking whether it could serve the newly selected model. I’ll fix both so the switch updates the bar before another request is sent.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go');s=p.read_text().replace('{"terra", "openai-codex", "gpt-6.1-terra", []string{"plus", "pro"}},','{"terra", "openai-codex", "gpt-6.1-terra", []string{"plus", "pro"}},\n\t\t{"sol 6.1", "openai-codex", "gpt-6.1-sol", []string{"plus", "pro"}},\n\t\t{"qualified sol 6.1", "openai-codex", "openai-codex/gpt-6.1-sol", []string{"plus", "pro"}},');p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go');s=p.read_text();needle='\t// Do not substitute Plus when the routed free account has no fetched report.';s=s.replace(needle,'''\t// Model selection must immediately reject a stale free-tier routing identity.
        	m.editor.Render(100)
        	m.editor.NeedsRedraw()
        	m.opts.Model.ID = "gpt-6.1-sol"
        	if !m.editor.IsDirty() {
        		t.Fatal("switching to Sol 6.1 did not invalidate the Free bar")
        	}
        	if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
        		t.Fatalf("Sol 6.1 retained a stale Free bar: %q", label)
        	}
        	m.opts.Model.ID = "gpt-6-luna"
        	assertFree()
        '''+needle,1)
        s=s.replace('m.opts.Model.ID = "gpt-6.1-terra"\n\tif label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {\n\t\tt.Fatalf("paid-only model selected free quota: %q", label)\n\t}', '''for _, model := range []string{"gpt-6.1-terra", "gpt-6.1-sol"} {
        		m.opts.Model.ID = model
        		if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {
        			t.Fatalf("paid-only model %s selected free quota: %q", model, label)
        		}
        	}''',1);p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestRouterPrefersFreeCodexAccountsForEligibleModels|TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts' -count=1
    --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels (0.00s)
        --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels/sol_6.1 (0.00s)
            account_routing_test.go:760: got 3 candidates, want [plus pro]
        --- FAIL: TestRouterPrefersFreeCodexAccountsForEligibleModels/qualified_sol_6.1 (0.00s)
            account_routing_test.go:760: got 3 candidates, want [plus pro]
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/ai    0.145s
    --- FAIL: TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts (0.01s)
        interactive_prompt_quota_test.go:240: switching to Sol 6.1 did not invalidate the Free bar
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/internal/codingagent    0.155s
    FAIL
    ✗ (1) • 39.41s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go');s=p.read_text().replace('// codexPaidModels are the catalog models a ChatGPT paid plan is required for. Node-compatible\n// projection of pi-catalog rules/runtime/behavior.kdl (account-policy.ts CODEX_PAID_MODELS).','// codexPaidModels lists Codex models that require a paid ChatGPT plan.').replace('\t"gpt-5.6-luna-pro": true,','\t"gpt-5.6-luna-pro": true,\n\t"gpt-6.1-sol":      true,');p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go');s=p.read_text();a=s.index('\tif routed {',s.index('func (m *InteractiveMode) promptQuotaTierReports'));b=s.index('\n\tvar fallback',a);s=s[:a]+'''\tif routed {
        		var routedGroup *tui.UsagePlanGroup
        		for i := range groups {
        			group := &groups[i]
        			if group.Provider != provider {
        				continue
        			}
        			for _, report := range group.Reports {
        				accountKey, keyed := report.Metadata["accountKey"].(string)
        				if key != nil && keyed && accountKey == *key || key == nil && !keyed {
        					routedGroup = group
        					break
        				}
        			}
        			if routedGroup != nil {
        				break
        			}
        		}
        		if routedGroup == nil {
        			return nil
        		}
        		// A model switch can leave the previous request's tier in affinity until the new model routes its first request.
        		if meets := ai.CodexPlanMeetsModel(model, routedGroup.Plan); meets == nil || *meets {
        			return routedGroup.Reports
        		}
        	}'''+s[b:];p.write_text(s)
        p=Path('README.md');s=p.read_text().replace('Before the first Codex request, it prefers an eligible free tier.', 'Before the first Codex request, it prefers an eligible free tier. Model switches immediately recheck access: GPT-6.1 Sol uses paid-tier quota even when the previous model used Free.');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
        git diff --check
    ✓ • 86ms

## Assistant

The regression tests reproduced both problems: Sol 6.1 still allowed Free as a candidate, and switching from Luna left the Free bar unchanged. Both fixes are in place. The race checks and build are running.

## Activity

    $ rg -n 'gpt-6\.1-sol' vendor/github.com/MichaelKinsy/PiG/ai --glob '*test.go' | head -35; git diff --check; sed -n '142,181p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:179:    if meets := ai.AccountMeetsModelPlan("openai", "gpt-6.1-sol", free); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:182:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-6.1-sol", ai.Credential{Type: ai.CredentialAPIKey, Key: "sk-test"}); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:189:    quota := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached", 429, nil)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:194:    if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "too many requests, please slow down", 429, nil); block != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:197:    if block := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "internal server error", 500, nil); block != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:205:    if auth := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "invalidated oauth token", 401, nil); auth == nil || auth.Scope != "auth" {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:209:    denied := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", `The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.`, 400, nil)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:210:    if denied == nil || denied.Scope != "model-policy:gpt-6.1-sol" {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:214:    if slow := ai.AccountFailureBlock("openai-codex", "gpt-6.1-sol", "usage_limit_reached; try again in 5 min", 429, nil); slow == nil || slow.Until < now+299_000 {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:745:        {"sol 6.1", "openai-codex", "gpt-6.1-sol", []string{"plus", "pro"}},
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:746:        {"qualified sol 6.1", "openai-codex", "openai-codex/gpt-6.1-sol", []string{"plus", "pro"}},
        }
        groups := tui.GroupUsageReportsByPlan(reports, nil)
        key, routed := m.usageAccountKey(provider)
        if routed {
            var routedGroup *tui.UsagePlanGroup
            for i := range groups {
                group := &groups[i]
                if group.Provider != provider {
                    continue
                }
                for _, report := range group.Reports {
                    accountKey, keyed := report.Metadata["accountKey"].(string)
                    if key != nil && keyed && accountKey == *key || key == nil && !keyed {
                        routedGroup = group
                        break
                    }
                }
                if routedGroup != nil {
                    break
                }
            }
            if routedGroup == nil {
                return nil
            }
            // A model switch can leave the previous request's tier in affinity until the new model routes its first request.
            if meets := ai.CodexPlanMeetsModel(model, routedGroup.Plan); meets == nil || *meets {
                return routedGroup.Reports
            }
        }
        var fallback []ai.UsageReport
        fallbackKnown := false
        for _, group := range groups {
            if group.Provider != provider {
                continue
            }
            meets := ai.CodexPlanMeetsModel(model, group.Plan)
            if meets != nil && !*meets {
                continue
            }
            if group.Plan == "free" && meets != nil && *meets {
    ✓ • 15ms

## Assistant

The bar now rechecks model access on selection and rejects a stale Free-tier assignment for Sol 6.1. That lets it choose the paid-tier bar before you submit a prompt. Compilation is still running for the final checks.

## Activity

    $ ps -eo pid,etime,pcpu,comm --sort=-pcpu | head -6; git diff --check
        PID     ELAPSED %CPU COMMAND
    3261510       00:00  100 git
    3260780       00:17 41.0 compile
    3260219       00:36 40.8 compile
    3260788       00:16 40.6 compile
    3260728       00:20 31.6 compile
    ✓ • 432ms

## Activity

    $ make build
    go build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .
    ✓ • 2m 49s

## Activity

    $ python - <<'PY'
        import os, subprocess, tempfile
        with tempfile.TemporaryDirectory(prefix='ompig-sol-tier-') as profile:
         env=os.environ.copy()
         for key in ('PIG_CODING_AGENT_DIR','PI_CODING_AGENT_DIR','OMPIG_SLIM_CODING_AGENT_DIR'):
          env[key]=profile+'/agent'
         for key in ('PIG_HOME','PI_HOME'):
          env[key]=profile+'/library'
         env.update(PI_SKIP_VERSION_CHECK='1',PI_TELEMETRY='0')
         raise SystemExit(subprocess.call(['go','test','-race','-mod=vendor','github.com/MichaelKinsy/PiG/ai','github.com/MichaelKinsy/PiG/internal/codingagent'],env=env))
        PY
    ok      github.com/MichaelKinsy/PiG/ai    21.536s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    1.846s
    ✓ • 3m 31s

## Assistant

Sol 6.1 was missing from the paid-only rules, and the bar trusted the previous Free-tier assignment. Both are fixed: switching to Sol 6.1 now immediately selects paid-tier quota.

Race tests and build passed. Restart with `./bin/ompig-slim` to apply.

## User

feels like you trying to hardcode it, it will fail with edge cases

## Assistant

You’re right. I added a model-specific rule instead of fixing the source of eligibility data. I’ll trace the provider’s account-specific model discovery and the router’s selection state so the bar can follow that data rather than infer access from model names.

## Activity

    $ rg -n -i 'codex.*model|model.*codex|entitle|available.*account|account.*available|supported.*model|model.*access' vendor/github.com/MichaelKinsy/PiG/ai/openai_codex* vendor/github.com/MichaelKinsy/PiG/coding/model_runtime* vendor/github.com/MichaelKinsy/PiG/ai/*catalog* vendor/github.com/MichaelKinsy/PiG/ai/*discovery* | head -110; rg -n 'UsageAccountKey|PeekAccountKey|ResolveAccount' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime*.go; rg --files vendor/github.com/MichaelKinsy/PiG/ai | rg 'codex|discover|catalog' | head -55; rg -n 'Account.*Model|Models.*Account|Account.*Models' vendor/github.com/MichaelKinsy/PiG/ai vendor/github.com/MichaelKinsy/PiG/coding | head -70
    rg: vendor/github.com/MichaelKinsy/PiG/ai/*discovery*: No such file or directory (os error 2)
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go:321:    // Defaults first so model and caller headers override them (buildBaseCodexHeaders, #10429).
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go:815:    builder := newAssistantStreamBuilder(ctx, APIOpenAICodexResponses, p.cfg.ProviderID, p.cfg.Model)
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go:818:    builder.setProviderEventObserver(opts, providerEventModel(p.cfg.ModelMetadata, APIOpenAICodexResponses, p.cfg.ProviderID, p.cfg.Model))
    vendor/github.com/MichaelKinsy/PiG/ai/models_catalog_codec.go:143:    return nil, nil, fmt.Errorf("encode model: unsupported model %T", model)
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_sse_stream.go:48:// codexBodyReader models the ReadableStreamDefaultReader parseSSE holds over undici's response body.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_typed.go:3:// Ports the typed model accessors and image generation of packages/coding-agent/src/core/model-runtime.ts (getModelsOfType, getModelOfType, getAllModels, getAvailableOfType, getProvider, getAuth(model), generateImages).
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:233:// OpenAICodexModels is OPENAI_CODEX_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:234:func OpenAICodexModels() ChatModelCatalog { return chatModelCatalog("openai-codex") }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:236:// OpenAICodexImageModels is OPENAI_CODEX_IMAGE_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:237:func OpenAICodexImageModels() ImageModelCatalog { return imageModelCatalog("openai-codex") }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:239:// OpenAICodexClassifierModels is OPENAI_CODEX_CLASSIFIER_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:240:func OpenAICodexClassifierModels() ClassifierModelCatalog {
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:241:    return classifierModelCatalog("openai-codex")
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_responses.go:50:    // Defaults first so model and caller headers can override them (openai-codex-responses.ts buildBaseCodexHeaders, #10429).
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_classify.go:36:    if err := ai.AssertClassifierInputSupported(model, request); err != nil {
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_typed.go:131:    accountKey, present, err := router.ResolveAccount(ctx, model, ai.AccountRouteRequest{APIKeySet: apiKeySet})
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:86:        accountKey, present = runtime.PeekAccountKey(providerID, opts.SessionID, opts.ModelID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:129:// PeekAccountKey reports the account a session's requests are pinned to for a provider and model.
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:131:func (runtime *ModelRuntime) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:135:    return runtime.services.Registry().PeekAccountKey(providerID, sessionID, modelID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:174:    return router.ResolveAccount(ctx, model, ai.AccountRouteRequest{SessionID: sessionID, APIKeySet: apiKeySet})
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:183:// UsageAccountKey reports the account this session's requests for provider are routed to, for the
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:187:func (s *Session) UsageAccountKey(providerID string) (*string, bool) {
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:195:    return s.ModelRuntime().PeekAccountKey(providerID, s.ID(), modelID)
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_websocket.go
    vendor/github.com/MichaelKinsy/PiG/ai/oauth_openai_codex.go
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_responses.go
    vendor/github.com/MichaelKinsy/PiG/ai/zstd_codex.go
    vendor/github.com/MichaelKinsy/PiG/ai/providers_added_catalog.go
    vendor/github.com/MichaelKinsy/PiG/ai/models_catalog_codec.go
    vendor/github.com/MichaelKinsy/PiG/ai/model_catalog_metadata_snapshot.go
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_http.go
    vendor/github.com/MichaelKinsy/PiG/ai/catalog_startup_test.go
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_frames.go
    vendor/github.com/MichaelKinsy/PiG/ai/model_catalog.go
    vendor/github.com/MichaelKinsy/PiG/ai/omp_oauth_snowflake_catalog.go
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_sse_stream.go
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_codex.go
    vendor/github.com/MichaelKinsy/PiG/ai/models_catalog_shape.go
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go
    vendor/github.com/MichaelKinsy/PiG/ai/model_catalog_metadata.go
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_test.go:817:    ccaRecordAccountProfile("account-a", "gemini-3.8-flash-low", ccaAntigravityWireProfile{ModelEnum: "MODEL_A", MaxOutputTokens: 1111})
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_test.go:818:    ccaRecordAccountProfile("account-b", "gemini-3.8-flash-low", ccaAntigravityWireProfile{ModelEnum: "MODEL_B", MaxOutputTokens: 2222})
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_test.go:830:    if got := ccaWireProfileForAccount("account-a", "gemini-3.8-flash-low"); got.ModelEnum != "MODEL_A" {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:247:func (r *AccountRouter) Candidates(ctx context.Context, model AnyModel, request AccountRouteRequest) ([]*CredentialAccountInfo, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:295:        meets := AccountMeetsModelPlan(providerID, modelID, *credential)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:331:func (r *AccountRouter) ResolveAccount(ctx context.Context, model AnyModel, request AccountRouteRequest) (accountKey *string, present bool, err error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:357:func (r *AccountRouter) BlockedUntil(ctx context.Context, model AnyModel, account *CredentialAccountInfo) (*int64, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:358:    scopes := AccountRequestBlockScopes(model.ProviderID(), model.ModelID())
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:427:func (r *AccountRouter) Fail(ctx context.Context, model AnyModel, account *CredentialAccountInfo, message string, response *ProviderResponse) *AccountFailureScope {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:438:    block := AccountFailureBlock(model.ProviderID(), model.ModelID(), message, status, retryAfter)
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:508:func (m *Models) Router() *AccountRouter { return m.accounts }
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go:512:func ObserveOnResponse(existing func(context.Context, ProviderResponse, *Model) error, observe AccountObserve) func(context.Context, ProviderResponse, *Model) error {
    vendor/github.com/MichaelKinsy/PiG/ai/classifier.go:228:func ObserveClassifierOnResponse(existing func(ProviderResponse, ClassifierModel) error, observe AccountObserve) func(ProviderResponse, ClassifierModel) error {
    vendor/github.com/MichaelKinsy/PiG/ai/models_runtime_stream.go:98:func (m *Models) applyAuth(ctx context.Context, model *Model, options StreamOptions, selection AccountSelection, forceOAuthRefresh bool) (*Model, StreamOptions, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/models_runtime_stream.go:160:func (m *Models) openStreamAttempt(ctx context.Context, model *Model, transcript TranscriptContext, simple bool, opts StreamOptions, selection AccountSelection, observe AccountObserve, forceOAuthRefresh bool) (*AssistantMessageEventStream, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_registration.go:369:        roster := googleAntigravityModelsFromResponse(payload, endpoint, ccaDiscoveryAccountKey(credential))
    vendor/github.com/MichaelKinsy/PiG/ai/models_runtime_typed.go:72:func ObserveImageOnResponse(existing func(ProviderResponse, ImageModel) error, observe AccountObserve) func(ProviderResponse, ImageModel) error {
    vendor/github.com/MichaelKinsy/PiG/coding/services.go:618:func (runtime *ModelRuntime) startRouted(ctx context.Context, outer *ai.AssistantMessageEventStream, router *ai.AccountRouter, model *ai.Model, transcript ai.TranscriptContext, options ai.StreamOptions, simple bool) {
    vendor/github.com/MichaelKinsy/PiG/coding/services.go:682:func observeStreamResponse(existing func(context.Context, ai.ProviderResponse, *ai.Model) error, observe ai.AccountObserve) func(context.Context, ai.ProviderResponse, *ai.Model) error {
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_typed.go:115:func (runtime *ModelRuntime) resolveAccountOverrides(ctx context.Context, model ai.AnyModel, overrides []ai.AuthResolutionOverrides) []ai.AuthResolutionOverrides {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:149:func TestAccountMeetsModelPlan(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:156:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", free); meets == nil || *meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:160:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", codex("pro")); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:164:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-luna", codex("plus")); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:167:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-spark", codex("plus")); meets == nil || *meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:171:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5", free); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:175:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-5.6-sol", ai.Credential{Type: ai.CredentialOAuth, Access: "not-a-jwt"}); meets != nil {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:179:    if meets := ai.AccountMeetsModelPlan("openai", "gpt-6.1-sol", free); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:182:    if meets := ai.AccountMeetsModelPlan("openai-codex", "gpt-6.1-sol", ai.Credential{Type: ai.CredentialAPIKey, Key: "sk-test"}); meets == nil || !*meets {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:736:func TestRouterPrefersFreeCodexAccountsForEligibleModels(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:755:            accounts, err := ai.NewAccountRouter(store).Candidates(t.Context(), okModel(tc.provider, tc.model), ai.AccountRouteRequest{})
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:24:func (r *AccountRouter) StreamRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) *AssistantMessageEventStream {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:31:func (r *AccountRouter) streamRoutes(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, request AccountRouteRequest, open AccountAttemptStream) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:124:    pushAccountSetupError(outer, model, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest)))
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:142:func (r *AccountRouter) driveAttempt(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest, stream *AssistantMessageEventStream, observed *observedResponse, forceOAuthRefresh bool) accountStreamOutcome {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:249:func pushAccountAbort(stream *AssistantMessageEventStream, model AnyModel, reason error, stopReason StopReason, partial *AssistantMessage) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:282:func (r *AccountRouter) ImagesRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptImages) (AssistantImages, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:290:func (r *AccountRouter) ClassifyRoutes(ctx context.Context, model AnyModel, request AccountRouteRequest, attempt AccountAttemptClassify) (ClassifierResult, error) {
    vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go:427:func pushAccountSetupError(stream *AssistantMessageEventStream, model AnyModel, err error) {
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_google.go:285:            Scope:  UsageScope{Provider: params.Provider, AccountID: credential.AccountID, ProjectID: projectID, ModelID: modelID, Tier: tier, WindowID: window.ID},
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_openai.go:540:            Scope:  UsageScope{Provider: "github-copilot", AccountID: user, ModelID: model, WindowID: window.ID},
    vendor/github.com/MichaelKinsy/PiG/ai/githubcopilot.go:387:// copilotAccountModel is one entry of the account's model catalog after
    vendor/github.com/MichaelKinsy/PiG/ai/githubcopilot.go:389:type copilotAccountModel struct {
    vendor/github.com/MichaelKinsy/PiG/ai/githubcopilot.go:413:    accountModels := make([]copilotAccountModel, 0, len(rawData))
    vendor/github.com/MichaelKinsy/PiG/ai/githubcopilot.go:427:        accountModels = append(accountModels, copilotAccountModel{
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:43:// AccountPolicyModelID is the canonical bare model identity used by OMP's model-policy
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:45:func AccountPolicyModelID(modelID string) string {
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:50:// AccountMeetsModelPlan reports whether a known Codex plan satisfies the catalog requirement
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:56:func AccountMeetsModelPlan(provider, modelID string, credential Credential) *bool {
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:66:    id := AccountPolicyModelID(modelID)
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:155:    if strings.Contains(AccountPolicyModelID(modelID), "-spark") {
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:163:    id := AccountPolicyModelID(modelID)
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:243:    id := AccountPolicyModelID(modelID)
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:245:        if provider != "openai-codex" || AccountPolicyModelID(match[1]) != id {
    vendor/github.com/MichaelKinsy/PiG/ai/google_cloud_code_assist_request.go:203:        profile := ccaWireProfileForAccount(plan.accountKey, plan.wireModelID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:86:        accountKey, present = runtime.PeekAccountKey(providerID, opts.SessionID, opts.ModelID)
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:165:func (runtime *ModelRuntime) ResolveCredentialAccount(ctx context.Context, model ai.AnyModel, sessionID string) (*string, bool, error) {
    ✓ • 128ms

## Activity

    $ rg -n -i 'codex' vendor/github.com/MichaelKinsy/PiG/ai/*registration* | head -65; sed -n '150,181p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go; rg -n 'DiscoverModels|discoverModels|ModelDiscovery|FetchModels' vendor/github.com/MichaelKinsy/PiG/ai/provider* vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model* | head -70; sed -n '1120,1143p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go; rg -n 'type InteractiveRuntime|ResolveCredentialAccount' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*.go vendor/github.com/MichaelKinsy/PiG/coding/cli/*.go | head -45
            return nil
        }
        state := &runtime.availability
        state.mu.Lock()
        state.refreshSeq++
        state.providerSeq[providerID]++
        seq := state.providerSeq[providerID]
        state.errorSeq++
        errorSeq := state.errorSeq
        state.mu.Unlock()
        return runtime.refreshProviderAvailability(ctx, providerID, seq, errorSeq)
    }
    
    // ResolveCredentialAccount reports the account a request for this model would use, for
    // model-scoped credential exports such as `--provider` bearer-token printing.
    func (runtime *ModelRuntime) ResolveCredentialAccount(ctx context.Context, model ai.AnyModel, sessionID string) (*string, bool, error) {
        router := runtime.accountRouter()
        if router == nil {
            return nil, false, nil
        }
        apiKeySet := false
        if runtime.services != nil {
            _, apiKeySet = runtime.services.Registry().RuntimeAPIKey(model.ProviderID())
        }
        return router.ResolveAccount(ctx, model, ai.AccountRouteRequest{SessionID: sessionID, APIKeySet: apiKeySet})
    }
    
    // AccountCredentialStore returns the base credential store's multi-account surface, or nil when
    // the store holds one credential per provider.
    func (runtime *ModelRuntime) AccountCredentialStore() ai.AccountCredentialStore {
        return runtime.credentialAccounts()
    }
        // ModelGenerateImages is the Session-owned runtime generateImages an extension's ctx.modelRegistry.generateImages reaches.
        ModelGenerateImages func(context.Context, *ai.ImageModel, ai.ImagesContext, ...ai.ModelsImagesOptions) ai.AssistantImages
    
        // RequestAuthRuntime is the composed checkAuth/getAuth surface used by
        // warning-only auth checks. Wired by main.go from the same credential and
        // provider configuration used for model requests.
        RequestAuthRuntime *RequestAuthRuntime
    
        // UsageService is the quota service the /usage board reads. The host builds
        // it from the session's credential store and auth runtime; nil leaves the
        // board without a quota source.
        UsageService UsageQuotaService
        // UsageAccountKey reports the account this session is routed to for a
        // provider. present is false when the runtime holds no routing decision,
        // which asks the quota service for the store's active account; a present
        // result passes its key through, nil included, because a nil key names an
        // identity-less account rather than the default.
        UsageAccountKey func(provider string) (accountKey *string, present bool)
    
        // ModelRegistry is the model registry for auth-aware operations
        // (Refresh, GetAvailable, HasConfiguredAuth). Wired by main.go.
        // nil is safe; post-login refresh is skipped.
        // Mirrors upstream session.modelRegistry (interactive-mode.ts:4394).
        ModelRegistry *ModelRegistry
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go:24:type InteractiveRuntime interface {
    ✓ • 0ms

## Activity

    $ rg -n -i 'codex.*discover|discover.*codex|codex.*models|models.*codex' vendor/github.com/MichaelKinsy/PiG/ai --glob '!models_generated.go' --glob '!*.sum' | head -65; rg -n 'Discover|Discovery|FetchModels' vendor/github.com/MichaelKinsy/PiG/ai/provider_definition* vendor/github.com/MichaelKinsy/PiG/ai/types.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model* | head -85; sed -n '18,49p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go; sed -n '70,93p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_runtime.go; sed -n '150,168p' vendor/github.com/MichaelKinsy/PiG/coding/cli/interactive_build.go; sed -n '325,355p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:233:// OpenAICodexModels is OPENAI_CODEX_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:234:func OpenAICodexModels() ChatModelCatalog { return chatModelCatalog("openai-codex") }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:236:// OpenAICodexImageModels is OPENAI_CODEX_IMAGE_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:237:func OpenAICodexImageModels() ImageModelCatalog { return imageModelCatalog("openai-codex") }
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:239:// OpenAICodexClassifierModels is OPENAI_CODEX_CLASSIFIER_MODELS.
    vendor/github.com/MichaelKinsy/PiG/ai/provider_catalogs.go:240:func OpenAICodexClassifierModels() ClassifierModelCatalog {
    vendor/github.com/MichaelKinsy/PiG/ai/provider_constructors.go:72:func OpenaiCodexProvider() *ModelsProvider { return builtinProvider("openai-codex") }
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:33:// codexPaidModels lists Codex models that require a paid ChatGPT plan.
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:34:var codexPaidModels = map[string]bool{
    vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go:71:    case codexPaidModels[id] || strings.HasPrefix(id, "gpt-6.1-terra"):
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_google.go:94:            {label: "spark", models: []string{"gpt-5.3-codex-spark"}},
    vendor/github.com/MichaelKinsy/PiG/ai/usage_provider_google.go:95:            {label: "chat", models: []string{"gpt-5.3-codex"}},
    vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go:736:func TestRouterPrefersFreeCodexAccountsForEligibleModels(t *testing.T) {
    vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:300:    // varies by provider (generic omits it; codex sends null; strict-mode models
    vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:522:// (ToModel) never resolve two different entries. Codex models are cataloged
    vendor/github.com/MichaelKinsy/PiG/ai/openai_responses.go:1853:// getServiceTierCostMultiplier mirrors OpenAI and Codex service-tier pricing. Only OpenAI prices the "fast" tier, which GPT-6 models report for
    vendor/github.com/MichaelKinsy/PiG/ai/openai_codex_sse_stream.go:48:// codexBodyReader models the ReadableStreamDefaultReader parseSSE holds over undici's response body.
    rg: vendor/github.com/MichaelKinsy/PiG/ai/provider_definition*: No such file or directory (os error 2)
    type InteractiveForkResult struct {
        Cancelled    bool
        SelectedText *string
    }
    
    // InteractiveRuntime is the Session replacement contract InteractiveMode needs from the runtime host that created its Session. *coding.Runtime satisfies it through the adapter in cmd/pig; the interface exists because internal/codingagent cannot import coding.
    type InteractiveRuntime interface {
        NewSession(ctx context.Context, options *extension.NewSessionOptions) (extension.CancelledResult, error)
        // SwitchSession opens path and replaces the Session. projectTrustUI supplies the UI of the destination's project trust prompt, as Pi's projectTrustContextFactory does.
        SwitchSession(ctx context.Context, path, cwdOverride string, projectTrustUI func(cwd string) extension.ProjectTrustContext, options *extension.SwitchSessionOptions) (extension.CancelledResult, error)
        Fork(ctx context.Context, entryID string, options *extension.ForkOptions) (InteractiveForkResult, error)
        ImportFromJsonl(ctx context.Context, path, cwdOverride string) (extension.CancelledResult, error)
        // ExtensionCommandActions returns the command actions of session with new, fork and switch routed through the runtime.
        ExtensionCommandActions(session InteractiveSessionHandle) extension.CommandActions
        // EmitQuitShutdown emits session_shutdown for quit once; closing the runtime does not repeat it.
        EmitQuitShutdown()
        // SetBeforeSessionReplacement installs the drain that runs after a replacement is approved and before the outgoing Session aborts.
        SetBeforeSessionReplacement(drain func(context.Context) error)
        // SetBeforeSessionInvalidate installs the synchronous hook that runs after session_shutdown and before the outgoing Session's extension contexts go stale.
        SetBeforeSessionInvalidate(hook func())
        // SetRebindSession installs the awaited callback that runs once the replacement Session is installed.
        SetRebindSession(rebind func(ctx context.Context, session InteractiveSessionHandle) error)
    }
    
    // InteractiveReplacement carries the options of a replacement Session's cwd-bound build, as Pi reads them from the new Session's services. The mode applies it before it rebinds to the Session.
    type InteractiveReplacement struct {
        CWD                     string
        SessionDir              string
        Settings                Settings
        SettingsManager         *SettingsManager
        ModelRegistry           *ModelRegistry
        SystemPrompt            string
        // ExtensionConflicts are the tool and flag conflicts of the build's final extension set, which Pi lists with the extension load errors.
        ExtensionConflicts      []ExtensionConflict
        BuiltinExtensions       []extension.Extension
        ExplicitBuiltinPaths    []string
        ReloadBuiltinExtensions func() []extension.Extension
        SubprocessUIBridge      SubprocessUIBridge
        SubprocessHost          SubprocessHost
        ModelLookup             func(providerID, modelID string) *ai.Model
        ModelCatalog            func(providerID ...string) []*ai.Model
        ModelClassify           func(context.Context, *ai.ClassifierModel, ai.ClassifierContext, ...ai.ModelsClassifierOptions) ai.ClassifierResult
        ModelGenerateImages     func(context.Context, *ai.ImageModel, ai.ImagesContext, ...ai.ModelsImagesOptions) ai.AssistantImages
        RequestAuthRuntime      *RequestAuthRuntime
        // UsageService is the replacement Session's quota service, built from its own
        // credential store so the board never reads the outgoing Session's accounts.
        UsageService    UsageQuotaService
        UsageAccountKey func(provider string) (*string, bool)
    }
    
    // installRuntimeHooks binds the runtime host to this mode as the constructor of Pi's InteractiveMode does: the mode resets extension UI before the outgoing Session's contexts go stale, drains active work before the outgoing Session aborts, and rebinds to the replacement Session.
    func (m *InteractiveMode) installRuntimeHooks() {
        rt := m.runtimeHost
        if rt == nil {
            return
        }
            ModelGenerateImages:     session.ModelRuntime().GenerateImages,
            RequestAuthRuntime:      requestAuth,
            UsageService:            newSessionUsageService(build, requestAuth),
            UsageAccountKey:         session.UsageAccountKey,
        }
        // A typed nil pointer would make these interfaces non-nil.
        if build.Bridge != nil {
            replacement.SubprocessUIBridge = build.Bridge
        }
        if build.Host != nil {
            replacement.SubprocessHost = build.Host
        }
        return replacement
    }
        }
        return 0
    }
    
    // ResolveAccount reports the account a request would use, for model-scoped credential exports.
    // present is false for a request the router does not route.
    func (r *AccountRouter) ResolveAccount(ctx context.Context, model AnyModel, request AccountRouteRequest) (accountKey *string, present bool, err error) {
        accounts, err := r.Candidates(ctx, model, request)
        if err != nil || accounts == nil {
            return nil, false, err
        }
        var earliest *int64
        for _, account := range accounts {
            if err := context.Cause(ctx); err != nil {
                return nil, false, err
            }
            until, err := r.BlockedUntil(ctx, model, account)
            if err != nil {
                return nil, false, err
            }
            if until == nil || *until <= r.nowMillis() {
                return account.AccountKey, true, nil
            }
            earliest = earliestDeadline(earliest, *until)
        }
        return nil, false, errors.New(accountUnavailableMessage(model.ProviderID(), model.ModelID(), earliest))
    }
    
    func (r *AccountRouter) nowMillis() int64 { return r.now().UnixMilli() }
    
    // BlockedUntil reports the latest live block deadline of one account for this request's scopes,
    ✓ • 0ms

## Assistant

This runtime has no account-specific Codex model discovery; it has a bundled catalog and learns rejections while routing requests. I’ll remove the Sol-specific patch and the bar’s tier guesses. The bar will follow the router’s actual account choice, using provider rejection results for fallback. For a model with no established route yet, it will stay hidden rather than guess.

## Activity

    $ sed -n '76,95p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '142,203p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '357,412p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '525,550p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; sed -n '72,107p' vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go; sed -n '345,382p' vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go; sed -n '118,138p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go; rg -n 'PeekAccountKey' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/model*
    }
    
    // AccountRouter routes independent requests without changing the provider's persistent selected
    // account. A store without the account methods yields a router that never routes.
    type AccountRouter struct {
        credentials AccountCredentialStore
        now         func() time.Time
    
        mu sync.Mutex
        // affinity is provider → session → model → the account key that session's last success used.
        affinity map[string]map[string]map[string]*string
        // localBlocks holds this run's blocks by provider → account → scope, so an account stays
        // retired even when the store cannot persist the block.
        localBlocks map[string]map[accountKeyValue]map[string]int64
        // listeners observe handoffs.
        listeners []*accountRotationListener
    }
    
    // NewAccountRouter builds a router over store. A nil or single-account store never routes.
    func NewAccountRouter(store CredentialStore) *AccountRouter {
    // PeekAccountKey reports the account this provider's recent requests were pinned to, for
    // read-only display. present is false when the session has no pin for the provider, session and
    // model; a present nil key is the identity-less account.
    func (r *AccountRouter) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
        if r == nil {
            return nil, false
        }
        r.mu.Lock()
        defer r.mu.Unlock()
        sessions := r.affinity[providerID]
        if sessions == nil {
            return nil, false
        }
        if sessionID == "" {
            for _, models := range sessions {
                for _, accountKey := range models {
                    return accountKey, true
                }
            }
            return nil, false
        }
        models := sessions[sessionID]
        if models == nil {
            return nil, false
        }
        if modelID != "" {
            accountKey, ok := models[modelID]
            return accountKey, ok
        }
        for _, accountKey := range models {
            return accountKey, true
        }
        return nil, false
    }
    
    func (r *AccountRouter) pin(providerID, modelID string, accountKey *string, request AccountRouteRequest) {
        if request.SessionID == "" {
            return
        }
        r.mu.Lock()
        defer r.mu.Unlock()
        sessions := r.affinity[providerID]
        if sessions == nil {
            sessions = map[string]map[string]*string{}
            r.affinity[providerID] = sessions
        }
        models := sessions[request.SessionID]
        if models == nil {
            models = map[string]*string{}
            sessions[request.SessionID] = models
        }
        models[modelID] = accountKey
    }
    
    func (r *AccountRouter) reportRotation(providerID, modelID string, failed accountFailure, next *CredentialAccountInfo, request AccountRouteRequest) {
        reason := accountFailureReason(failed.scope)
        label := "another stored account"
        if next.Label != "" {
            label = next.Label
        } else if next.AccountKey != nil {
            label = *next.AccountKey
        }
    func (r *AccountRouter) BlockedUntil(ctx context.Context, model AnyModel, account *CredentialAccountInfo) (*int64, error) {
        scopes := AccountRequestBlockScopes(model.ProviderID(), model.ModelID())
        stored, err := r.credentials.GetAccountBlockedUntil(ctx, model.ProviderID(), account.AccountKey, scopes)
        if err != nil {
            return nil, err
        }
        now := r.nowMillis()
        until := int64(0)
        if stored != nil && *stored > now {
            until = *stored
        }
        if local := r.localBlockedUntil(model.ProviderID(), account.AccountKey, scopes, now); local > until {
            until = local
        }
        if until == 0 {
            return nil, nil
        }
        return &until, nil
    }
    
    func (r *AccountRouter) localBlockedUntil(providerID string, accountKey *string, scopes []string, now int64) int64 {
        r.mu.Lock()
        defer r.mu.Unlock()
        accounts := r.localBlocks[providerID]
        if accounts == nil {
            return 0
        }
        key := accountKeyOf(accountKey)
        blocks := accounts[key]
        if blocks == nil {
            return 0
        }
        until := int64(0)
        for _, scope := range append([]string{""}, scopes...) {
            deadline := blocks[scope]
            if deadline <= now {
                delete(blocks, scope)
                continue
            }
            if deadline > until {
                until = deadline
            }
        }
        if len(blocks) == 0 {
            delete(accounts, key)
        }
        return until
    }
    
    func (r *AccountRouter) rememberBlock(providerID string, accountKey *string, scope string, until int64) {
        r.mu.Lock()
        defer r.mu.Unlock()
        accounts := r.localBlocks[providerID]
        if accounts == nil {
            accounts = map[accountKeyValue]map[string]int64{}
            r.localBlocks[providerID] = accounts
    // SetStore replaces the credential store the router routes over and drops the affinity and
    // blocks that belonged to the previous one. A collection that already captured this router keeps
    // working: the pointer stays valid, its state follows the new store.
    func (r *AccountRouter) SetStore(store CredentialStore) {
        if r == nil {
            return
        }
        var accounts AccountCredentialStore
        if store != nil {
            accounts, _ = store.(AccountCredentialStore)
        }
        r.mu.Lock()
        r.credentials = accounts
        r.affinity = map[string]map[string]map[string]*string{}
        r.localBlocks = map[string]map[accountKeyValue]map[string]int64{}
        r.mu.Unlock()
    }
            }
            if until != nil && *until > r.nowMillis() {
                earliest = earliestDeadline(earliest, *until)
                continue
            }
            if previous != nil {
                r.reportRotation(model.ProviderID(), model.ModelID(), *previous, account, request)
            }
            for refresh := range 2 {
                attemptCtx, selection := r.attemptContext(ctx, account)
                observed := newObservedResponse()
                attemptCtx = withAccountResponseObserver(attemptCtx, observed.record)
                stream, openErr := open(attemptCtx, selection, observed.record, refresh == 1)
                if openErr != nil {
                    if context.Cause(ctx) != nil {
                        pushAccountSetupError(outer, model, AbortReason(ctx))
                        return
                    }
                    block := r.Fail(ctx, model, account, accountErrorText(openErr), observed.get())
                    if block == nil {
                        pushAccountSetupError(outer, model, openErr)
                        return
                    }
                    previous = &accountFailure{account: account, scope: block.Scope}
                    lastFailure, lastSetupError = nil, openErr
                    break
                }
                if stream == nil {
                    pushAccountSetupError(outer, model, errors.New("provider returned no stream"))
                    return
                }
                outcome := r.driveAttempt(ctx, outer, model, account, request, stream, observed, refresh == 1)
                lastFailure, lastSetupError = outcome.event, nil
                switch {
                case outcome.delivered:
                    return
                continue
            }
            if previous != nil {
                router.reportRotation(model.ProviderID(), model.ModelID(), *previous, account, request)
            }
            for refresh := range 2 {
                attemptCtx, selection := router.attemptContext(ctx, account)
                observed := newObservedResponse()
                attemptCtx = withAccountResponseObserver(attemptCtx, observed.record)
                value, attemptErr := attempt(attemptCtx, selection, observed.record, refresh == 1)
                if attemptErr != nil {
                    if err := context.Cause(ctx); err != nil {
                        return zero, err
                    }
                    block := router.Fail(ctx, model, account, accountErrorText(attemptErr), observed.get())
                    if block == nil {
                        return zero, attemptErr
                    }
                    previous = &accountFailure{account: account, scope: block.Scope}
                    last, lastFailed, lastError = zero, false, attemptErr
                    break
                }
                outcome := adapt(value)
                if !outcome.failed() {
                    if outcome.success() {
                        router.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
                    }
                    return value, nil
                }
                if outcome.output || context.Cause(ctx) != nil {
                    return value, nil
                }
                if shouldRetryWithRefresh(account, refresh == 1, observed.get()) {
                    continue
                }
                block := router.Fail(ctx, model, account, outcome.message, observed.get())
                if block == nil {
                    return value, nil
    }
    
    // OnAccountRotation subscribes to request-local account handoffs and returns the detach
    // function.
    func (runtime *ModelRuntime) OnAccountRotation(listener func(AccountRotation)) func() {
        if runtime == nil || runtime.services == nil || listener == nil {
            return func() {}
        }
        return runtime.services.Registry().OnAccountRotation(listener)
    }
    
    // PeekAccountKey reports the account a session's requests are pinned to for a provider and model.
    // present is false when nothing is pinned; a present nil key is the identity-less account.
    func (runtime *ModelRuntime) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
        if runtime == nil || runtime.services == nil {
            return nil, false
        }
        return runtime.services.Registry().PeekAccountKey(providerID, sessionID, modelID)
    }
    
    // clearAccountAffinity drops this runtime's successful-request affinity for a provider.
    ✗ (1) • 0ms

## Activity

    $ rg -n 'func .*accountRouter|func .*PeekAccountKey' vendor/github.com/MichaelKinsy/PiG/coding/*.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/*.go; sed -n '100,120p' vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go; sed -n '198,265p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go; sed -n '130,143p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go:131:func (runtime *ModelRuntime) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
    vendor/github.com/MichaelKinsy/PiG/coding/services.go:608:func (runtime *ModelRuntime) accountRouter() *ai.AccountRouter {
    vendor/github.com/MichaelKinsy/PiG/internal/codingagent/native_model_runtime.go:211:func (r *ModelRegistry) PeekAccountKey(providerID, sessionID, modelID string) (*string, bool) {
        if settings := ai.AccountCredentialSettings(*credential); settings != nil {
            active.Email = settings.Email
            active.AccountID = settings.AccountID
            active.OrgID = settings.OrgID
            active.OrgName = settings.OrgName
        }
        return active, true
    }
    
    // UsageActiveAccountOptions narrows GetUsageActiveAccount.
    type UsageActiveAccountOptions struct {
        // SessionID and ModelID look up the session's pinned account.
        SessionID string
        ModelID   string
        // AccountKey, with AccountKeyPresent, names the account directly; a present nil key is the
        // identity-less account.
        AccountKey        *string
        AccountKeyPresent bool
    }
    
    // OnAccountRotation subscribes to request-local account handoffs and returns the detach
    }
    
    func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts(t *testing.T) {
        monthlyReset := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
        weeklyReset := time.Now().Add(4 * 24 * time.Hour).UnixMilli()
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "unknown", "accountKey": "unknown"}, Limits: []ai.UsageLimit{promptTestLimit("unknown", .99, weeklyReset)}},
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "plus", "accountKey": "plus"}, Limits: []ai.UsageLimit{promptTestLimit("7d", .95, weeklyReset)}},
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "free", "accountKey": "free-a"}, Limits: []ai.UsageLimit{promptTestLimit("30d", .2, monthlyReset)}},
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "ChatGPT-Free", "accountKey": "free-b"}, Limits: []ai.UsageLimit{promptTestLimit("30d", .4, monthlyReset)}},
        }}
        m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "gpt-6-luna", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}, editor: tui.NewEditor()}
        m.editor.TopRightStatus = m.promptQuotaLabel
        assertFree := func() {
            t.Helper()
            label := m.promptQuotaLabel(100)
            if !strings.HasPrefix(label, "30d ") || !strings.Contains(label, "70% left") || strings.Contains(label, "7d") {
                t.Fatalf("Plus quota overrode the combined free tier: %q", label)
            }
        }
        // Before any request has established affinity, follow the free-first policy.
        assertFree()
        key := "free-a"
        m.opts.UsageAccountKey = func(string) (*string, bool) { return &key, true }
        assertFree()
        m.editor.Render(100)
        m.editor.NeedsRedraw()
        // An actual paid fallback changes the bar immediately, using the same snapshot.
        key = "plus"
        if !m.editor.IsDirty() {
            t.Fatal("routed tier change did not invalidate the border")
        }
        if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
            t.Fatalf("paid fallback retained the free bar: %q", label)
        }
        key = "free-b"
        assertFree()
        // Model selection must immediately reject a stale free-tier routing identity.
        m.editor.Render(100)
        m.editor.NeedsRedraw()
        m.opts.Model.ID = "gpt-6.1-sol"
        if !m.editor.IsDirty() {
            t.Fatal("switching to Sol 6.1 did not invalidate the Free bar")
        }
        if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
            t.Fatalf("Sol 6.1 retained a stale Free bar: %q", label)
        }
        m.opts.Model.ID = "gpt-6-luna"
        assertFree()
        // Do not substitute Plus when the routed free account has no fetched report.
        key = "missing-free"
        if label := m.promptQuotaLabel(100); label != "" {
            t.Fatalf("missing routed quota fell back to another tier: %q", label)
        }
        // A paid-only model must choose its eligible tier before a request is routed.
        m.opts.UsageAccountKey = nil
        for _, model := range []string{"gpt-6.1-terra", "gpt-6.1-sol"} {
            m.opts.Model.ID = model
            if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {
                t.Fatalf("paid-only model %s selected free quota: %q", model, label)
            }
        }
    }
    
    // ClearProvider drops this runtime's successful-request affinity for a provider, so the next
    // request uses the selected account within the router's plan priority.
    func (r *AccountRouter) ClearProvider(providerID string) {
        if r == nil {
            return
        }
        r.mu.Lock()
        defer r.mu.Unlock()
        delete(r.affinity, providerID)
    }
    
    // PeekAccountKey reports the account this provider's recent requests were pinned to, for
    // read-only display. present is false when the session has no pin for the provider, session and
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'ai/account_policy.go';s=p.read_text().replace('// codexPaidModels lists Codex models that require a paid ChatGPT plan.', '// codexPaidModels are the catalog models a ChatGPT paid plan is required for. Node-compatible\n// projection of pi-catalog rules/runtime/behavior.kdl (account-policy.ts CODEX_PAID_MODELS).').replace('\n\t"gpt-6.1-sol":      true,','')
        s=s.replace('\tplan, _ := codexPlanClaim(credential.Access)\n\treturn CodexPlanMeetsModel(modelID, plan)\n}\n\n// CodexPlanMeetsModel applies the stored-account entitlement policy to a plan reported by the usage endpoint. Unknown restricted plans remain probeable.\nfunc CodexPlanMeetsModel(modelID, plan string) *bool {\n','')
        s=s.replace('\tplan = codexPlanSeparators.ReplaceAllString(jsstring.ToLower(jsstring.Trim(plan)), "_")\n\tplan = strings.TrimPrefix(plan, "chatgpt_")\n\tswitch {','\tplan, ok := codexPlanClaim(credential.Access)\n\tif !ok {\n\t\treturn nil\n\t}\n\tswitch {',1);p.write_text(s)
        p=root/'ai/account_routing.go';s=p.read_text().replace('\tmu sync.Mutex','\tmu sync.Mutex\n\tactiveRoutes map[accountRouteIdentity]map[uint64]*string\n\trouteSequence uint64',1).replace('\tr.affinity = map[string]map[string]map[string]*string{}\n\tr.localBlocks =', '\tr.affinity = map[string]map[string]map[string]*string{}\n\tr.activeRoutes = nil\n\tr.localBlocks =',1);p.write_text(s)
        p=root/'ai/account_route_ops.go';s=p.read_text().replace('\t\tfor refresh := range 2 {\n\t\t\tattemptCtx, selection := r.attemptContext(ctx, account)', '\t\tdefer r.trackAccountAttempt(model, account, request)()\n\t\tfor refresh := range 2 {\n\t\t\tattemptCtx, selection := r.attemptContext(ctx, account)',1).replace('\t\tfor refresh := range 2 {\n\t\t\tattemptCtx, selection := router.attemptContext(ctx, account)', '\t\tdefer router.trackAccountAttempt(model, account, request)()\n\t\tfor refresh := range 2 {\n\t\t\tattemptCtx, selection := router.attemptContext(ctx, account)',1);p.write_text(s)
        p=root/'coding/model_runtime_accounts.go';s=p.read_text();a=s.index('// UsageAccountKey reports');b=s.index('\n// emitAccountRotation',a);s=s[:a]+'''// UsageAccountKey reports the account currently attempted for the session's selected model, or its last successful account. A model with no known route has no usage identity.
        func (s *Session) UsageAccountKey(providerID string) (*string, bool) {
        	if s == nil {
        		return nil, false
        	}
        	model := s.Model()
        	if model == nil || model.ProviderID() != providerID {
        		return nil, false
        	}
        	router := s.ModelRuntime().accountRouter()
        	if router == nil {
        		return nil, false
        	}
        	return router.PeekRoutedAccountKey(providerID, s.ID(), model.ID)
        }
        '''+s[b:];p.write_text(s)
        p=root/'internal/codingagent/interactive_prompt_quota.go';s=p.read_text().replace('m.promptQuotaTierReports(snapshot.Reports, provider, m.opts.Model.ID)','m.promptQuotaTierReports(snapshot.Reports, provider)');a=s.index('// Total accounts in the routed subscription tier.');b=s.index('\ntype promptQuotaWindow',a);s=s[:a]+'''// Total the subscription tier named by the router. Unknown routes and missing reports stay hidden; the renderer never predicts model entitlements.
        func (m *InteractiveMode) promptQuotaTierReports(reports []ai.UsageReport, provider string) []ai.UsageReport {
        	key, routed := m.usageAccountKey(provider)
        	if !routed {
        		return nil
        	}
        	for _, group := range tui.GroupUsageReportsByPlan(reports, nil) {
        		if group.Provider != provider {
        			continue
        		}
        		for _, report := range group.Reports {
        			accountKey, keyed := report.Metadata["accountKey"].(string)
        			if key != nil && keyed && accountKey == *key || key == nil && !keyed {
        				return group.Reports
        			}
        		}
        	}
        	return nil
        }
        '''+s[b:];p.write_text(s)
        PY
        cat > vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go <<'EOF'
        package ai
        
        // The usage display follows request attempts and successful routing identities. It does not infer access from a model name or subscription tier.
        type accountRouteIdentity struct {
         provider, session, model string
        }
        
        func (r *AccountRouter) trackAccountAttempt(model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest) func() {
         if request.SessionID == "" {
          return func() {}
         }
         identity := accountRouteIdentity{model.ProviderID(),request.SessionID,model.ModelID()}
         key := account.AccountKey
         if key != nil {key = new(*key)}
         r.mu.Lock()
         if r.activeRoutes == nil {r.activeRoutes=make(map[accountRouteIdentity]map[uint64]*string)}
         attempts:=r.activeRoutes[identity]
         if attempts==nil {attempts=make(map[uint64]*string);r.activeRoutes[identity]=attempts}
         r.routeSequence++
         sequence:=r.routeSequence
         attempts[sequence]=key
         r.mu.Unlock()
         return func(){
          r.mu.Lock()
          defer r.mu.Unlock()
          attempts:=r.activeRoutes[identity]
          delete(attempts,sequence)
          if len(attempts)==0 {delete(r.activeRoutes,identity)}
         }
        }
        
        // PeekRoutedAccountKey reads the latest live attempt, falling back to this model's last successful route. Unknown models and accounts rejected by this router stay hidden. The lookup performs no credential-store or network I/O.
        func (r *AccountRouter) PeekRoutedAccountKey(providerID,sessionID,modelID string)(*string,bool){
         if r==nil || providerID=="" || sessionID=="" || modelID=="" {return nil,false}
         r.mu.Lock()
         var sequence uint64
         var key *string
         present:=false
         for candidate,accountKey:=range r.activeRoutes[accountRouteIdentity{providerID,sessionID,modelID}] {
          if !present || candidate>sequence {sequence,key,present=candidate,accountKey,true}
         }
         if !present {key,present=r.affinity[providerID][sessionID][modelID]}
         if key!=nil {key=new(*key)}
         r.mu.Unlock()
         if !present || r.localBlockedUntil(providerID,key,AccountRequestBlockScopes(providerID,modelID),r.nowMillis())>0 {return nil,false}
         return key,true
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go
    ✓ • 0ms

## Activity

    $ tail -5 vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go; sed -n '30,51p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go; sed -n '170,195p' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go; rg -n 'promptQuotaLabel' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
            if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {
                t.Fatalf("paid-only model %s selected free quota: %q", model, label)
            }
        }
    }
        if len(windows) != 2 || windows[0].used != .5 || windows[0].resetsAt != sooner {
            t.Fatalf("wrong account total or next reset: %+v", windows)
        }
        m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "luna-6", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
        label := m.promptQuotaLabel(200)
        if strings.Contains(label, "luna-6") || strings.Contains(strings.ToLower(label), "codex") {
            t.Fatalf("quota label included model/provider: %q", label)
        }
        wantBar := tui.RenderUsageBar(tui.ActiveTheme(), ai.UsageLimit{Amount: ai.UsageAmount{UsedFraction: new(.5)}, Status: ai.UsageStatusOK}, 24)
        if !strings.Contains(label, wantBar) {
            t.Fatalf("quota does not use the usage board bar: %q", label)
        }
        if strings.Count(label, "█") == 0 || !strings.Contains(label, "5h") || !strings.Contains(label, "50% left") || !strings.Contains(label, "reset 2h") || strings.Contains(label, "7d") {
            t.Fatalf("wrong essential quota: %q", label)
        }
    
        minimumWidth := widthx.VisibleWidth(label) - 24 + 12
        if compact := m.promptQuotaLabel(minimumWidth); widthx.VisibleWidth(compact) != minimumWidth {
            t.Fatalf("bar did not shrink to 12 cells: %q", compact)
        }
        if compact := m.promptQuotaLabel(minimumWidth - 1); compact != "" {
            t.Fatalf("bar rendered without enough room for its fields: %q", compact)
        select {
        case <-done:
        case <-time.After(2 * time.Second):
            t.Fatal("quota tasks outlived shutdown")
        }
    }
    
    func TestPromptQuotaKeepsFreeMonthlyAndPlusWeeklyTotalsSeparate(t *testing.T) {
        monthlyReset := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
        weeklyReset := time.Now().Add(4 * 24 * time.Hour).UnixMilli()
        freeA, freeB := promptTestLimit("30d", .8, monthlyReset), promptTestLimit("30d", .6, monthlyReset)
        plus := promptTestLimit("7d", .2, weeklyReset)
        // Codex can use the same meter ID for different account window durations.
        freeA.ID, freeB.ID, plus.ID = "openai-codex:primary", "openai-codex:primary", "openai-codex:primary"
        snapshot := ai.UsageSnapshot{Reports: []ai.UsageReport{
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "free"}, Limits: []ai.UsageLimit{freeA}},
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "free"}, Limits: []ai.UsageLimit{freeB}},
            {Provider: "openai-codex", Metadata: map[string]any{"planType": "plus"}, Limits: []ai.UsageLimit{plus}},
        }}
        windows := promptQuotaWindows(snapshot, "openai-codex", "gpt-6-luna")
        if len(windows) != 2 || windows[0].label != "30d" || math.Abs(windows[0].used-.7) > 1e-9 || windows[0].resetsAt != monthlyReset || windows[1].label != "7d" || windows[1].used != .2 {
            t.Fatalf("free monthly quota was mixed with Plus weekly quota: %+v", windows)
        }
        m := &InteractiveMode{opts: InteractiveModeOptions{Model: &ai.Model{ID: "gpt-6-luna", ProviderMeta: ai.ProviderMetadata{ProviderID: "openai-codex"}}}, promptQuota: &promptQuotaState{loaded: true, snapshot: snapshot}}
        label := m.promptQuotaLabel(100)
        if !strings.HasPrefix(label, "30d ") || !strings.Contains(label, "30% left") || !strings.Contains(label, "reset 20d") || strings.Contains(label, "7d") {
    34:    label := m.promptQuotaLabel(200)
    47:    if compact := m.promptQuotaLabel(minimumWidth); widthx.VisibleWidth(compact) != minimumWidth {
    50:    if compact := m.promptQuotaLabel(minimumWidth - 1); compact != "" {
    57:    label = m.promptQuotaLabel(200)
    62:    if label = m.promptQuotaLabel(200); !strings.Contains(label, "partial") {
    66:    m.editor.TopRightStatus = m.promptQuotaLabel
    73:    if label = m.promptQuotaLabel(200); strings.Contains(label, "different") || !strings.Contains(label, "monthly") || !strings.Contains(label, "0% left") {
    77:    if label = m.promptQuotaLabel(200); label != "" {
    85:    if label = m.promptQuotaLabel(200); label != "" {
    89:    if label = m.promptQuotaLabel(200); label != "" {
    194:    label := m.promptQuotaLabel(100)
    210:    m.editor.TopRightStatus = m.promptQuotaLabel
    213:        label := m.promptQuotaLabel(100)
    230:    if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
    242:    if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") || !strings.Contains(label, "5% left") {
    249:    if label := m.promptQuotaLabel(100); label != "" {
    256:        if label := m.promptQuotaLabel(100); !strings.HasPrefix(label, "7d ") {
    ✓ • 0ms

## Assistant

The bar now reads the router’s live account or last successful route for the selected model. It no longer checks model names or predicts a tier. I’m testing unlisted models, provider rejection and recovery, overlapping requests, cancellation, and account-store replacement. The router’s pre-existing catalog rules remain in place; I removed the Sol rule I added.

## Activity

    $ sed -n '603,618p' vendor/github.com/MichaelKinsy/PiG/coding/services.go; cat vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go; git diff -- vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go; rg -n 'CodexPlanMeetsModel|gpt-6\.1-sol.*true|forecast|Before the first Codex' vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota.go vendor/github.com/MichaelKinsy/PiG/ai/account_policy.go README.md
            runtime.fail(ctx, outer, preparedModel, err)
        }
    }
    
    // accountRouter is the registry's request-local account router, or nil without services.
    func (runtime *ModelRuntime) accountRouter() *ai.AccountRouter {
        if runtime == nil || runtime.services == nil {
            return nil
        }
        return runtime.services.Registry().AccountRouter()
    }
    
    // startRouted runs one request through the account pool: each candidate account opens its own
    // prepared attempt, so a rejection retried on a sibling account happens before any content
    // escapes to the consumer (model-runtime.ts stream, over prepareRequest).
    func (runtime *ModelRuntime) startRouted(ctx context.Context, outer *ai.AssistantMessageEventStream, router *ai.AccountRouter, model *ai.Model, transcript ai.TranscriptContext, options ai.StreamOptions, simple bool) {
    package ai
    
    // The usage display follows request attempts and successful routing identities. It does not infer access from a model name or subscription tier.
    type accountRouteIdentity struct {
        provider, session, model string
    }
    
    func (r *AccountRouter) trackAccountAttempt(model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest) func() {
        if request.SessionID == "" {
            return func() {}
        }
        identity := accountRouteIdentity{model.ProviderID(), request.SessionID, model.ModelID()}
        key := account.AccountKey
        if key != nil {
            key = new(*key)
        }
        r.mu.Lock()
        if r.activeRoutes == nil {
            r.activeRoutes = make(map[accountRouteIdentity]map[uint64]*string)
        }
        attempts := r.activeRoutes[identity]
        if attempts == nil {
            attempts = make(map[uint64]*string)
            r.activeRoutes[identity] = attempts
        }
        r.routeSequence++
        sequence := r.routeSequence
        attempts[sequence] = key
        r.mu.Unlock()
        return func() {
            r.mu.Lock()
            defer r.mu.Unlock()
            attempts := r.activeRoutes[identity]
            delete(attempts, sequence)
            if len(attempts) == 0 {
                delete(r.activeRoutes, identity)
            }
        }
    }
    
    // PeekRoutedAccountKey reads the latest live attempt, falling back to this model's last successful route. Unknown models and accounts rejected by this router stay hidden. The lookup performs no credential-store or network I/O.
    func (r *AccountRouter) PeekRoutedAccountKey(providerID, sessionID, modelID string) (*string, bool) {
        if r == nil || providerID == "" || sessionID == "" || modelID == "" {
            return nil, false
        }
        r.mu.Lock()
        var sequence uint64
        var key *string
        present := false
        for candidate, accountKey := range r.activeRoutes[accountRouteIdentity{providerID, sessionID, modelID}] {
            if !present || candidate > sequence {
                sequence, key, present = candidate, accountKey, true
            }
        }
        if !present {
            key, present = r.affinity[providerID][sessionID][modelID]
        }
        if key != nil {
            key = new(*key)
        }
        r.mu.Unlock()
        if !present || r.localBlockedUntil(providerID, key, AccountRequestBlockScopes(providerID, modelID), r.nowMillis()) > 0 {
            return nil, false
        }
        return key, true
    }
    ✗ (1) • 7ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go');s=p.read_text().replace('\t// Plan is the normalized subscription tier, empty when the provider reports none.\n\tPlan                string\n','').replace('group := &UsagePlanGroup{Provider: provider, Plan: plan}','group := &UsagePlanGroup{Provider: provider}');p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go');s=p.read_text().replace('// UsageAccountKey reports the account this session is routed to for a\n\t// provider. present is false when the runtime holds no routing decision,','// UsageAccountKey reports the live attempt or latest successful account for this session\'s\n\t// selected model. present is false when the runtime holds no routing decision,');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/tui/usage_display.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
        git diff --check
    ✓ • 400ms

## Activity

    $ sed -n '425,466p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go; rg -n 'rememberBlock\(' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
    }
    
    // Fail classifies a rejected attempt, records the block it earns, and returns the block. A nil
    // block means a sibling account cannot repair the failure.
    func (r *AccountRouter) Fail(ctx context.Context, model AnyModel, account *CredentialAccountInfo, message string, response *ProviderResponse) *AccountFailureScope {
        if account == nil {
            // An unpooled request has no account to retire; its failure is reported, never blocked.
            return nil
        }
        status := 0
        var retryAfter *int64
        if response != nil {
            status = response.Status
            retryAfter = retryAfterDelay(response.Headers, r.now())
        }
        block := AccountFailureBlock(model.ProviderID(), model.ModelID(), message, status, retryAfter)
        if block == nil {
            return nil
        }
        r.rememberBlock(model.ProviderID(), account.AccountKey, block.Scope, block.Until)
        if context.Cause(ctx) == nil && r.credentials != nil {
            // A store failure leaves the in-memory block in place, so this run still retires the
            // account.
            _ = r.credentials.BlockAccount(ctx, model.ProviderID(), account.AccountKey, block.Scope, block.Until)
        }
        return block
    }
    
    // retryAfterDelay parses a Retry-After header as delay-seconds or as an HTTP date.
    func retryAfterDelay(headers map[string]string, now time.Time) *int64 {
        value, ok := headers["retry-after"]
        if !ok || strings.TrimSpace(value) == "" {
            return nil
        }
        if seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
            if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
                return nil
            }
            return new(int64(math.Max(0, seconds*1000)))
        }
        if when, err := http.ParseTime(strings.TrimSpace(value)); err == nil {
            return new(int64(math.Max(0, float64(when.Sub(now)/time.Millisecond))))
    408:func (r *AccountRouter) rememberBlock(providerID string, accountKey *string, scope string, until int64) {
    444:    r.rememberBlock(model.ProviderID(), account.AccountKey, block.Scope, block.Until)
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go');s=p.read_text();a=s.index('func TestUsageRouteHidesPreviouslySuccessfulRejectedAccount');head,tail=s[:a],s[a:];tail=tail.replace('\tmodel := okModel("p", "future-model")','\tnow := time.Now()\n\trouter.SetClock(func() time.Time { return now })\n\tmodel := okModel("p", "future-model")',1);tail=tail.replace('\n}\n','\n\tnow = now.Add(2 * time.Minute)\n\tif _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {\n\t\tt.Fatal("retry expiry revived a rejected account without a new route")\n\t}\n}\n',1);p.write_text(head+tail)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai -run '^TestUsageRouteHidesPreviouslySuccessfulRejectedAccount$' -count=1
    --- FAIL: TestUsageRouteHidesPreviouslySuccessfulRejectedAccount (0.00s)
        account_routing_usage_test.go:179: retry expiry revived a rejected account without a new route
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/ai    0.332s
    FAIL
    ✗ (1) • 9.11s

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'ai/account_routing_test.go';s=p.read_text();s='\n'.join(line for line in s.split('\n') if '{"sol 6.1",' not in line and '{"qualified sol 6.1",' not in line);p.write_text(s)
        p=root/'internal/codingagent/interactive_prompt_quota_test.go';s=p.read_text().replace('\tlabel := m.promptQuotaLabel(200)', '\tm.opts.UsageAccountKey = func(string) (*string, bool) { return nil, true }\n\tlabel := m.promptQuotaLabel(200)',1)
        a=s.index('func TestPromptQuotaKeepsFreeMonthlyAndPlusWeeklyTotalsSeparate');b=s.index('func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts',a);part=s[a:b].replace('"planType": "free"', '"planType": "free", "accountKey": "free"').replace('"planType": "plus"', '"planType": "plus", "accountKey": "plus"').replace('\tlabel := m.promptQuotaLabel(100)', '\tm.opts.UsageAccountKey = func(string) (*string, bool) { return new("free"), true }\n\tlabel := m.promptQuotaLabel(100)',1);s=s[:a]+part+s[b:]
        a=s.index('func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts');s=s[:a]+'''func TestPromptQuotaFollowsRoutedTierAndTotalsAllItsAccounts(t *testing.T) {
         for _,provider:=range []string{"openai-codex","future-quota-provider"} {
          t.Run(provider,func(t *testing.T){
           monthlyReset:=time.Now().Add(20*24*time.Hour).UnixMilli()
           weeklyReset:=time.Now().Add(4*24*time.Hour).UnixMilli()
           snapshot:=ai.UsageSnapshot{Reports:[]ai.UsageReport{
            {Provider:provider,Metadata:map[string]any{"planType":"plus","accountKey":"plus"},Limits:[]ai.UsageLimit{promptTestLimit("7d",.95,weeklyReset)}},
            {Provider:provider,Metadata:map[string]any{"planType":"free","accountKey":"free-a"},Limits:[]ai.UsageLimit{promptTestLimit("30d",.2,monthlyReset)}},
            {Provider:provider,Metadata:map[string]any{"planType":"ChatGPT-Free","accountKey":"free-b"},Limits:[]ai.UsageLimit{promptTestLimit("30d",.4,monthlyReset)}},
           }}
           m:=&InteractiveMode{opts:InteractiveModeOptions{Model:&ai.Model{ID:"unlisted-future-model-a",ProviderMeta:ai.ProviderMetadata{ProviderID:provider}}},promptQuota:&promptQuotaState{loaded:true,snapshot:snapshot},editor:tui.NewEditor()}
           routes:=map[string]string{}
           m.opts.UsageAccountKey=func(string)(*string,bool){key,present:=routes[m.opts.Model.ID];if !present{return nil,false};return &key,true}
           m.editor.TopRightStatus=m.promptQuotaLabel
           if label:=m.promptQuotaLabel(100);label!="" {t.Fatalf("unknown route guessed a subscription tier: %q",label)}
           routes[m.opts.Model.ID]="free-a"
           assertFree:=func(){t.Helper();label:=m.promptQuotaLabel(100);if !strings.HasPrefix(label,"30d ") || !strings.Contains(label,"70% left") || strings.Contains(label,"7d"){t.Fatalf("Plus quota overrode the combined free tier: %q",label)}}
           assertFree()
           m.editor.Render(100);m.editor.NeedsRedraw()
           routes[m.opts.Model.ID]="plus"
           if !m.editor.IsDirty(){t.Fatal("routed account change did not invalidate the border")}
           if label:=m.promptQuotaLabel(100);!strings.HasPrefix(label,"7d ") || !strings.Contains(label,"5% left"){t.Fatalf("paid fallback retained the free bar: %q",label)}
           routes[m.opts.Model.ID]="free-b"
           assertFree()
           m.editor.Render(100);m.editor.NeedsRedraw()
           m.opts.Model.ID="unlisted-future-model-b"
           if !m.editor.IsDirty(){t.Fatal("switching models retained a stale account identity")}
           if label:=m.promptQuotaLabel(100);label!="" {t.Fatalf("new model guessed eligibility from its name: %q",label)}
           routes[m.opts.Model.ID]="plus"
           if label:=m.promptQuotaLabel(100);!strings.HasPrefix(label,"7d "){t.Fatalf("new model ignored its actual paid route: %q",label)}
           m.opts.Model.ID="unlisted-future-model-a"
           assertFree()
           routes[m.opts.Model.ID]="missing-account"
           if label:=m.promptQuotaLabel(100);label!=""{t.Fatalf("missing account quota fell back to another tier: %q",label)}
          })
         }
        }
        ''';p.write_text(s)
        p=Path('README.md');s=p.read_text();a=s.index('The prompt’s top-right border shows');b=s.index('\n\n',a);s=s[:a]+'''The prompt’s top-right border follows the router’s live account attempt or latest successful route for the selected model. It totals accounts within that account’s subscription tier, choosing the quota window with the least remaining capacity and showing its next account reset countdown. An unfamiliar model stays hidden until it has a routing identity; the bar does not infer access from model names or tier rules. Provider rejections drive the router’s fallback and the bar follows the resulting account. Window durations and reset times come from the provider: free-tier 30-day quotas stay separate from Plus 7-day quotas when totaling accounts. Quotas refresh every minute in the background; unavailable accounts mark the total as partial. The label shows only the window, the usage board’s detailed bar, remaining percentage, and next reset. The bar fits 12–24 cells and hides when the border has insufficient space or quota data cannot be fetched. Initial loading also leaves the border empty.'''+s[b:];p.write_text(s)
        PY
        cat > vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go <<'EOF'
        package ai_test
        
        import (
         "context"
         "fmt"
         "testing"
         "time"
        
         "github.com/MichaelKinsy/PiG/ai"
        )
        
        func TestUsageRouteLearnsProviderAccessForUnlistedModel(t *testing.T){
         store:=newFakeAccountStore(accountInfo("openai-codex",keyOf("plus"),true),accountInfo("openai-codex",keyOf("free"),false))
         for _,plan:=range []string{"plus","free"}{store.put("openai-codex",keyOf(plan),ai.Credential{Type:ai.CredentialOAuth,Access:codexToken(plan)})}
         router:=ai.NewAccountRouter(store)
         now:=time.Now();router.SetClock(func()time.Time{return now})
         model:=okModel("openai-codex","future-model-with-no-catalog-rule")
         request:=ai.AccountRouteRequest{SessionID:"live-usage-session"}
         if _,present:=router.PeekRoutedAccountKey("openai-codex",request.SessionID,model.ID);present{t.Fatal("unseen model had a fabricated account")}
         denyFree:=true
         var tried []string
         attempt:=func(ctx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(*ai.AssistantMessageEventStream,error){
          key,present:=router.PeekRoutedAccountKey("openai-codex",request.SessionID,model.ID)
          if !present || key==nil || *key!=*selection.AccountKey{return nil,fmt.Errorf("usage identity did not follow live attempt: %v %v",key,present)}
          tried=append(tried,*selection.AccountKey)
          if denyFree && *selection.AccountKey=="free"{
           observe(ai.ProviderResponse{Status:400})
           return emptyErrorStream(fmt.Sprintf("The '%s' model is not supported when using Codex with a ChatGPT account.",model.ID)),nil
          }
          return okStream(),nil
         }
         text,_:=collectStream(t,router.StreamRoutes(t.Context(),model,request,attempt))
         if text!="hello" || len(tried)!=2 || tried[0]!="free" || tried[1]!="plus"{t.Fatalf("provider denial did not drive fallback: %q %v",text,tried)}
         if key,present:=router.PeekRoutedAccountKey("openai-codex",request.SessionID,model.ID);!present || key==nil || *key!="plus"{t.Fatal("usage did not retain the successful paid account")}
         if _,present:=router.PeekRoutedAccountKey("openai-codex",request.SessionID,"another-unlisted-model");present{t.Fatal("routing identity leaked across models")}
         // A provider denial is scoped to this account/model and respected on the next request.
         tried=nil
         _,_=collectStream(t,router.StreamRoutes(t.Context(),model,request,attempt))
         if len(tried)!=1 || tried[0]!="plus"{t.Fatalf("rejected free account was retried before its block expired: %v",tried)}
         // Access may change: after the provider block expires, a successful free attempt updates usage without any catalog edit.
         now=now.Add(2*time.Minute);denyFree=false;tried=nil
         text,_=collectStream(t,router.StreamRoutes(t.Context(),model,request,attempt))
         if text!="hello" || len(tried)!=1 || tried[0]!="free"{t.Fatalf("provider access change required a model-name rule: %q %v",text,tried)}
        }
        
        func TestUsageRouteOverlappingAttemptCleanup(t *testing.T){
         for _,replace:=range []bool{false,true}{t.Run(fmt.Sprintf("replace_store_%v",replace),func(t *testing.T){
          store:=newFakeAccountStore(accountInfo("p",keyOf("a"),true),accountInfo("p",keyOf("b"),false))
          router:=ai.NewAccountRouter(store)
          model:=okModel("p","future-model")
          request:=ai.AccountRouteRequest{SessionID:"s"}
          ctx,cancel:=context.WithCancel(t.Context());defer cancel()
          start:=func() (chan struct{},chan struct{},chan error){
           entered,release,done:=make(chan struct{}),make(chan struct{}),make(chan error,1)
           go func(){_,err:=router.ImagesRoutes(ctx,model,request,func(attemptCtx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(ai.AssistantImages,error){
            close(entered)
            select{case <-release:return ai.AssistantImages{StopReason:ai.ImagesStopReasonStop},nil;case <-attemptCtx.Done():return ai.AssistantImages{},context.Cause(attemptCtx)}
           });done<-err}()
           return entered,release,done
          }
          wait:=func(entered <-chan struct{}){t.Helper();select{case <-entered:case <-time.After(2*time.Second):t.Fatal("attempt did not start")}}
          first,releaseFirst,doneFirst:=start();wait(first)
          if replace{router.SetStore(newFakeAccountStore(accountInfo("p",keyOf("b"),true)))}else{store.block("p",keyOf("a"),"",time.Now().Add(time.Hour).UnixMilli())}
          second,releaseSecond,doneSecond:=start();wait(second)
          close(releaseFirst)
          if err:=<-doneFirst;err!=nil{t.Fatal(err)}
          if key,present:=router.PeekRoutedAccountKey("p","s",model.ID);!present || key==nil || *key!="b"{t.Fatal("old attempt cleanup erased the newer account")}
          close(releaseSecond)
          if err:=<-doneSecond;err!=nil{t.Fatal(err)}
          if key,present:=router.PeekRoutedAccountKey("p","s",model.ID);!present || key==nil || *key!="b"{t.Fatal("successful route was lost after attempt cleanup")}
         })}
        }
        
        func TestUsageRouteCancellationDoesNotLeaveAnAccount(t *testing.T){
         router:=ai.NewAccountRouter(newFakeAccountStore(accountInfo("p",keyOf("a"),true)))
         model:=okModel("p","future-model")
         ctx,cancel:=context.WithCancel(t.Context());defer cancel()
         started,done:=make(chan struct{}),make(chan error,1)
         go func(){_,err:=router.ImagesRoutes(ctx,model,ai.AccountRouteRequest{SessionID:"s"},func(attemptCtx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(ai.AssistantImages,error){close(started);<-attemptCtx.Done();return ai.AssistantImages{},context.Cause(attemptCtx)});done<-err}()
         select{case <-started:case <-time.After(2*time.Second):t.Fatal("attempt did not start")}
         if _,present:=router.PeekRoutedAccountKey("p","s",model.ID);!present{t.Fatal("active attempt was not visible")}
         cancel()
         select{case <-done:case <-time.After(2*time.Second):t.Fatal("cancelled attempt did not drain")}
         if _,present:=router.PeekRoutedAccountKey("p","s",model.ID);present{t.Fatal("cancelled route remained visible")}
        }
        
        func TestUsageRouteHidesPreviouslySuccessfulRejectedAccount(t *testing.T){
         router:=ai.NewAccountRouter(newFakeAccountStore(accountInfo("p",keyOf("a"),true)))
         model:=okModel("p","future-model")
         request:=ai.AccountRouteRequest{SessionID:"s"}
         _,err:=router.ImagesRoutes(t.Context(),model,request,func(context.Context,ai.AccountSelection,ai.AccountObserve,bool)(ai.AssistantImages,error){return ai.AssistantImages{StopReason:ai.ImagesStopReasonStop},nil})
         if err!=nil{t.Fatal(err)}
         _,_=router.ImagesRoutes(t.Context(),model,request,func(ctx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(ai.AssistantImages,error){observe(ai.ProviderResponse{Status:429});return ai.AssistantImages{StopReason:ai.ImagesStopReasonError,ErrorMessage:"insufficient_quota"},nil})
         if _,present:=router.PeekRoutedAccountKey("p","s",model.ID);present{t.Fatal("a known rejected account remained on the bar")}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive_prompt_quota_test.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestUsageRoute|TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/ai    0.283s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.263s
    ✓ • 3m 08s

## Activity

    $ rg --files vendor/github.com/MichaelKinsy/PiG/coding -g '*test.go' -g '!extension/**' | wc -l; rg -n 'UsageAccountKey' vendor/github.com/MichaelKinsy/PiG/coding --glob '*test.go'
    3
    ✗ (1) • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'ai/account_routing.go';s=p.read_text().replace('\tactiveRoutes  map[accountRouteIdentity]map[uint64]*string','\tactiveRoutes  map[accountRouteIdentity]map[uint64]*string\n\tconfirmedRoutes map[accountRouteIdentity]*string',1)
        # Match gofmt spacing independently if necessary.
        if 'confirmedRoutes map' not in s:
         s=s.replace('\tactiveRoutes map[accountRouteIdentity]map[uint64]*string','\tactiveRoutes map[accountRouteIdentity]map[uint64]*string\n\tconfirmedRoutes map[accountRouteIdentity]*string',1)
        s=s.replace('\tdelete(r.affinity, providerID)\n}', '\tdelete(r.affinity, providerID)\n\tfor identity := range r.confirmedRoutes {\n\t\tif identity.provider == providerID { delete(r.confirmedRoutes, identity) }\n\t}\n}',1)
        s=s.replace('\tmodels[modelID] = accountKey\n}', '\tmodels[modelID] = accountKey\n\tif r.confirmedRoutes == nil { r.confirmedRoutes = make(map[accountRouteIdentity]*string) }\n\tr.confirmedRoutes[accountRouteIdentity{providerID, request.SessionID, modelID}] = accountKey\n}',1)
        s=s.replace('\tr.rememberBlock(model.ProviderID(), account.AccountKey, block.Scope, block.Until)', '\tr.rememberBlock(model.ProviderID(), account.AccountKey, block.Scope, block.Until)\n\tr.invalidateConfirmedRoutes(model.ProviderID(), account.AccountKey, block.Scope)',1)
        s=s.replace('\tr.activeRoutes = nil','\tr.activeRoutes = nil\n\tr.confirmedRoutes = nil',1);p.write_text(s)
        p=root/'ai/account_routing_usage.go';s=p.read_text().replace('package ai\n','package ai\n\nimport "slices"\n',1).replace('key, present = r.affinity[providerID][sessionID][modelID]','key, present = r.confirmedRoutes[accountRouteIdentity{providerID, sessionID, modelID}]',1)
        s+='''
        // A retry deadline makes an account probeable again; it does not confirm that a previously rejected account regained model access.
        func (r *AccountRouter) invalidateConfirmedRoutes(provider string, key *string, scope string) {
        	r.mu.Lock()
        	defer r.mu.Unlock()
        	for identity, confirmedKey := range r.confirmedRoutes {
        		if identity.provider != provider || accountKeyOf(confirmedKey) != accountKeyOf(key) {
        			continue
        		}
        		if scope == "" || slices.Contains(AccountRequestBlockScopes(provider, identity.model), scope) {
        			delete(r.confirmedRoutes, identity)
        		}
        	}
        }
        ''';p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go
        rg -n 'confirmedRoutes|activeRoutes' vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go
        git diff --check
    85:    activeRoutes    map[accountRouteIdentity]map[uint64]*string
    86:    confirmedRoutes map[accountRouteIdentity]*string
    143:    for identity := range r.confirmedRoutes {
    145:            delete(r.confirmedRoutes, identity)
    202:    if r.confirmedRoutes == nil {
    203:        r.confirmedRoutes = make(map[accountRouteIdentity]*string)
    205:    r.confirmedRoutes[accountRouteIdentity{providerID, request.SessionID, modelID}] = accountKey
    552:    r.activeRoutes = nil
    553:    r.confirmedRoutes = nil
    ✓ • 307ms

## Assistant

I found and reproduced another edge case: an expired retry timer could make a rejected account’s old bar reappear without a new successful route. Confirmed routing state is now invalidated by provider rejections. Retry expiry permits another attempt but doesn’t restore the old bar.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        import subprocess
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go')
        original=p.read_text()
        try:
         p.write_text(original.replace('\t\tdefer r.trackAccountAttempt(model, account, request)()\n','',1))
         result=subprocess.run(['go','test','-mod=vendor','github.com/MichaelKinsy/PiG/ai','-run','^TestUsageRouteLearnsProviderAccessForUnlistedModel$','-count=1'],capture_output=True,text=True)
         print(result.stdout+result.stderr)
         if result.returncode==0:
          raise SystemExit('Mutation unexpectedly passed')
         if 'usage identity did not follow live attempt' not in result.stdout+result.stderr and 'provider denial did not drive fallback' not in result.stdout+result.stderr:
          raise SystemExit('Mutation failed outside the intended regression')
        finally:
         p.write_text(original)
        PY
    --- FAIL: TestUsageRouteLearnsProviderAccessForUnlistedModel (0.00s)
        account_routing_usage_test.go:41: provider denial did not drive fallback: "" []
    FAIL
    FAIL    github.com/MichaelKinsy/PiG/ai    0.418s
    FAIL
    
    ✓ • 1m 05s

## Activity

    $ sed -n '414,444p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_test.go; sed -n '142,205p' vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go
    
    // okStream builds a stream that starts and finishes with one text block.
    func okStream() *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}}
        _ = stream.Push(ai.StartEvent{Partial: partial})
        _ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "hello", Partial: partial})
        final := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "hello"}}, StopReason: ai.StopReasonStop}
        _ = stream.Push(ai.DoneEvent{Reason: ai.StopReasonStop, Message: final})
        return stream
    }
    
    // emptyErrorStream builds a stream whose only outcome is an error with no content.
    func emptyErrorStream(message string) *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{}, StopReason: ai.StopReasonError, ErrorMessage: message}})
        return stream
    }
    
    // partialErrorStream builds a stream that emits content and then fails.
    func partialErrorStream(message string) *ai.AssistantMessageEventStream {
        stream := ai.NewAssistantMessageEventStream()
        partial := &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}}
        _ = stream.Push(ai.StartEvent{Partial: partial})
        _ = stream.Push(ai.TextDeltaEvent{ContentIndex: 0, Delta: "half", Partial: partial})
        _ = stream.Push(ai.ErrorEvent{Reason: ai.StopReasonError, Error: &ai.AssistantMessage{Provider: "p", Model: "m", Content: []ai.AssistantContentBlock{ai.TextContent{Text: "half"}}, StopReason: ai.StopReasonError, ErrorMessage: message}})
        return stream
    }
    
    // collectStream reads a stream to its end and returns the visible text and the terminal message.
    func collectStream(t *testing.T, stream *ai.AssistantMessageEventStream) (string, *ai.AssistantMessage) {
    // driveAttempt consumes one attempt's stream, holding its start event until a real event follows.
    func (r *AccountRouter) driveAttempt(ctx context.Context, outer *AssistantMessageEventStream, model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest, stream *AssistantMessageEventStream, observed *observedResponse, forceOAuthRefresh bool) accountStreamOutcome {
        var start *StartEvent
        // partial is the last message the consumer saw, so a cancellation terminal reports the same
        // content instead of discarding it.
        var partial *AssistantMessage
        emitted := false
        terminal := false
        for event := range stream.Events(ctx) {
            if emittedContent, ok := event.(StartEvent); ok && !emitted {
                copied := emittedContent
                start = &copied
                partial = copied.Partial
                continue
            }
            if current := eventPartial(event); current != nil {
                partial = current
            }
            if failure, ok := event.(ErrorEvent); ok && !emitted && errorEventIsEmpty(failure) && failure.Reason != StopReasonAborted && context.Cause(ctx) == nil && account != nil {
                if shouldRetryWithRefresh(account, forceOAuthRefresh, observed.get()) {
                    return accountStreamOutcome{retryRefresh: true}
                }
                block := r.Fail(ctx, model, account, errorMessageOf(failure), observed.get())
                if block != nil {
                    return accountStreamOutcome{rotated: true, failure: &accountFailure{account: account, scope: block.Scope}, event: &failure}
                }
            }
            if start != nil {
                _ = outer.Push(*start)
                start = nil
                emitted = true
            }
            emitted = true
            switch event.(type) {
            case DoneEvent:
                terminal = true
                // A successful turn pins the account for this session and model (account-pool.ts
                // stream `if (event.type === "done") this.pin(...)`).
                if account != nil {
                    r.pin(model.ProviderID(), model.ModelID(), account.AccountKey, request)
                }
            case ErrorEvent:
                terminal = true
            }
            _ = outer.Push(event)
        }
        if terminal {
            // A terminal event resolved the stream's result before it reached the consumer. Ending
            // without a result would leave the consumer's Result pending forever, so an unreachable
            // resolution still ends with a terminal message.
            if result, ok := stream.ResultResolved(); ok && result != nil {
                outer.End(result)
            } else {
                _ = outer.Push(ErrorEvent{Reason: StopReasonError, Error: accountTerminalMessage(model, errors.New("provider stream ended without a result"), StopReasonError)})
            }
            return accountStreamOutcome{emitted: true, delivered: true}
        }
        // The attempt's event iterator closed without a terminal event. A provider that ended its
        // iterator after ending the stream still has a result; one that closed on our cancellation, or
        // ended without any result, does not, and waiting for it would hang this router goroutine and
        // the consumer forever (Pi's EventStream.end() with no argument leaves result() pending).
        message, resolved := stream.ResultResolved()
        if !resolved || message == nil {
            // A cancelled request reports its abort; a live request whose provider closed without a
    ✓ • 0ms

## Assistant

I’m also keeping unconfirmed attempts hidden. A Free-account probe won’t flash a Free bar for a model that rejects it. The account becomes visible when the provider starts returning content or completes successfully; then the bar totals that account’s tier.

## Activity

    $ python - <<'PY'
        from pathlib import Path
        root=Path('vendor/github.com/MichaelKinsy/PiG')
        p=root/'ai/account_routing.go';s=p.read_text().replace('\tif r.confirmedRoutes == nil {\n\t\tr.confirmedRoutes = make(map[accountRouteIdentity]*string)\n\t}\n\tr.confirmedRoutes[accountRouteIdentity{providerID, request.SessionID, modelID}] = accountKey','\tr.confirmRouteLocked(accountRouteIdentity{providerID, request.SessionID, modelID}, accountKey)',1);p.write_text(s)
        p=root/'ai/account_routing_usage.go';s=p.read_text().replace('// PeekRoutedAccountKey reads the latest live attempt, falling back to this model\'s last successful route. Unknown models and accounts rejected by this router stay hidden. The lookup performs no credential-store or network I/O.', '// PeekRoutedAccountKey reads the latest accepted live attempt, falling back to this model\'s last confirmed route. Unverified attempts and rejected accounts stay hidden. The lookup performs no credential-store or network I/O.')
        s=s.replace('\tif !present {\n\t\tkey, present = r.confirmedRoutes[accountRouteIdentity{providerID, sessionID, modelID}]\n\t}', '''\tconfirmedKey, confirmed := r.confirmedRoutes[accountRouteIdentity{providerID, sessionID, modelID}]
        	if present {
        		present = confirmed && accountKeyOf(key) == accountKeyOf(confirmedKey)
        	} else {
        		key, present = confirmedKey, confirmed
        	}''',1)
        s+='''
        func (r *AccountRouter) confirmAccountAttempt(model AnyModel, account *CredentialAccountInfo, request AccountRouteRequest) {
        	if request.SessionID == "" || account == nil {
        		return
        	}
        	r.mu.Lock()
        	defer r.mu.Unlock()
        	r.confirmRouteLocked(accountRouteIdentity{model.ProviderID(), request.SessionID, model.ModelID()}, account.AccountKey)
        }
        
        func (r *AccountRouter) confirmRouteLocked(identity accountRouteIdentity, key *string) {
        	if r.confirmedRoutes == nil {
        		r.confirmedRoutes = make(map[accountRouteIdentity]*string)
        	}
        	if key != nil {
        		key = new(*key)
        	}
        	r.confirmedRoutes[identity] = key
        }
        ''';p.write_text(s)
        p=root/'ai/account_route_ops.go';s=p.read_text();a=s.index('func (r *AccountRouter) driveAttempt');head,tail=s[:a],s[a:];needle='\t\tif start != nil {\n';tail=tail.replace(needle,'''\t\tif !emitted {
        			if _, failure := event.(ErrorEvent); !failure {
        				r.confirmAccountAttempt(model, account, request)
        			}
        		}
        '''+needle,1);p.write_text(head+tail)
        p=root/'coding/model_runtime_accounts.go';s=p.read_text().replace('// UsageAccountKey reports the account currently attempted for the session\'s selected model, or its last successful account. A model with no known route has no usage identity.', '// UsageAccountKey reports the accepted live account for the selected model, or its last confirmed account. Unverified models and attempts have no usage identity.');p.write_text(s)
        p=Path('README.md');s=p.read_text().replace('follows the router’s live account attempt or latest successful route', 'follows the router’s accepted live account or last confirmed route').replace('until it has a routing identity', 'until the provider accepts an account for it');p.write_text(s)
        PY
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go vendor/github.com/MichaelKinsy/PiG/ai/account_route_ops.go vendor/github.com/MichaelKinsy/PiG/coding/model_runtime_accounts.go
    ✓ • 0ms

## Activity

    $ sed -n '15,74p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go; sed -n '106,153p' vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go
            store.put("openai-codex", keyOf(plan), ai.Credential{Type: ai.CredentialOAuth, Access: codexToken(plan)})
        }
        router := ai.NewAccountRouter(store)
        now := time.Now()
        router.SetClock(func() time.Time { return now })
        model := okModel("openai-codex", "future-model-with-no-catalog-rule")
        request := ai.AccountRouteRequest{SessionID: "live-usage-session"}
        if _, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); present {
            t.Fatal("unseen model had a fabricated account")
        }
        denyFree := true
        var tried []string
        attempt := func(ctx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (*ai.AssistantMessageEventStream, error) {
            key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID)
            if !present || key == nil || *key != *selection.AccountKey {
                return nil, fmt.Errorf("usage identity did not follow live attempt: %v %v", key, present)
            }
            tried = append(tried, *selection.AccountKey)
            if denyFree && *selection.AccountKey == "free" {
                observe(ai.ProviderResponse{Status: 400})
                return emptyErrorStream(fmt.Sprintf("The '%s' model is not supported when using Codex with a ChatGPT account.", model.ID)), nil
            }
            return okStream(), nil
        }
        text, _ := collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
        if text != "hello" || len(tried) != 2 || tried[0] != "free" || tried[1] != "plus" {
            t.Fatalf("provider denial did not drive fallback: %q %v", text, tried)
        }
        if key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "plus" {
            t.Fatal("usage did not retain the successful paid account")
        }
        if _, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, "another-unlisted-model"); present {
            t.Fatal("routing identity leaked across models")
        }
        // A provider denial is scoped to this account/model and respected on the next request.
        tried = nil
        _, _ = collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
        if len(tried) != 1 || tried[0] != "plus" {
            t.Fatalf("rejected free account was retried before its block expired: %v", tried)
        }
        // Access may change: after the provider block expires, a successful free attempt updates usage without any catalog edit.
        now = now.Add(2 * time.Minute)
        denyFree = false
        tried = nil
        text, _ = collectStream(t, router.StreamRoutes(t.Context(), model, request, attempt))
        if text != "hello" || len(tried) != 1 || tried[0] != "free" {
            t.Fatalf("provider access change required a model-name rule: %q %v", text, tried)
        }
    }
    
    func TestUsageRouteOverlappingAttemptCleanup(t *testing.T) {
        for _, replace := range []bool{false, true} {
            t.Run(fmt.Sprintf("replace_store_%v", replace), func(t *testing.T) {
                store := newFakeAccountStore(accountInfo("p", keyOf("a"), true), accountInfo("p", keyOf("b"), false))
                router := ai.NewAccountRouter(store)
                model := okModel("p", "future-model")
                request := ai.AccountRouteRequest{SessionID: "s"}
                ctx, cancel := context.WithCancel(t.Context())
                defer cancel()
                start := func() (chan struct{}, chan struct{}, chan error) {
                wait(second)
                close(releaseFirst)
                if err := <-doneFirst; err != nil {
                    t.Fatal(err)
                }
                if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {
                    t.Fatal("old attempt cleanup erased the newer account")
                }
                close(releaseSecond)
                if err := <-doneSecond; err != nil {
                    t.Fatal(err)
                }
                if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {
                    t.Fatal("successful route was lost after attempt cleanup")
                }
            })
        }
    }
    
    func TestUsageRouteCancellationDoesNotLeaveAnAccount(t *testing.T) {
        router := ai.NewAccountRouter(newFakeAccountStore(accountInfo("p", keyOf("a"), true)))
        model := okModel("p", "future-model")
        ctx, cancel := context.WithCancel(t.Context())
        defer cancel()
        started, done := make(chan struct{}), make(chan error, 1)
        go func() {
            _, err := router.ImagesRoutes(ctx, model, ai.AccountRouteRequest{SessionID: "s"}, func(attemptCtx context.Context, selection ai.AccountSelection, observe ai.AccountObserve, force bool) (ai.AssistantImages, error) {
                close(started)
                <-attemptCtx.Done()
                return ai.AssistantImages{}, context.Cause(attemptCtx)
            })
            done <- err
        }()
        select {
        case <-started:
        case <-time.After(2 * time.Second):
            t.Fatal("attempt did not start")
        }
        if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present {
            t.Fatal("active attempt was not visible")
        }
        cancel()
        select {
        case <-done:
        case <-time.After(2 * time.Second):
            t.Fatal("cancelled attempt did not drain")
        }
        if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {
    ✓ • 0ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go');s=p.read_text().replace('func (r *AccountRouter) confirmRouteLocked(identity accountRouteIdentity, key *string) {\n', '''func (r *AccountRouter) confirmRouteLocked(identity accountRouteIdentity, key *string) {
        	// Store replacement removes old attempts. Their late responses cannot confirm an account belonging to the outgoing store.
        	current := false
        	for _, attemptedKey := range r.activeRoutes[identity] {
        		if accountKeyOf(attemptedKey) == accountKeyOf(key) {
        			current = true
        			break
        		}
        	}
        	if !current {
        		return
        	}
        ''',1);p.write_text(s)
        PY
        cat >> vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go <<'EOF'
        
        func TestUsageRouteOldStoreCompletionCannotRestoreBar(t *testing.T){
         router:=ai.NewAccountRouter(newFakeAccountStore(accountInfo("p",keyOf("old"),true)))
         model:=okModel("p","future-model")
         ctx,cancel:=context.WithCancel(t.Context());defer cancel()
         entered,release,done:=make(chan struct{}),make(chan struct{}),make(chan error,1)
         go func(){_,err:=router.ImagesRoutes(ctx,model,ai.AccountRouteRequest{SessionID:"s"},func(attemptCtx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(ai.AssistantImages,error){close(entered);select{case <-release:return ai.AssistantImages{StopReason:ai.ImagesStopReasonStop},nil;case <-attemptCtx.Done():return ai.AssistantImages{},context.Cause(attemptCtx)}});done<-err}()
         select{case <-entered:case <-time.After(2*time.Second):t.Fatal("attempt did not start")}
         router.SetStore(newFakeAccountStore(accountInfo("p",keyOf("new"),true)))
         close(release)
         if err:=<-done;err!=nil{t.Fatal(err)}
         if _,present:=router.PeekRoutedAccountKey("p","s",model.ID);present{t.Fatal("outgoing credentials restored a route in the new store")}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage.go vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go
        git diff --check
    ✓ • 283ms

## Activity

    $ python - <<'PY'
        from pathlib import Path
        p=Path('vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go');s=p.read_text();s=s.replace('\tvar tried []string\n\tattempt :=', '\tvar tried []string\n\tvar visible []bool\n\tattempt :=',1).replace('\t\tif !present || key == nil || *key != *selection.AccountKey {','\t\tif present && (key == nil || *key != *selection.AccountKey) {',1).replace('\t\ttried = append(tried, *selection.AccountKey)','\t\tvisible = append(visible, present)\n\t\ttried = append(tried, *selection.AccountKey)',1)
        s=s.replace('\tif key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "plus" {','\tif visible[0] || visible[1] { t.Fatal("unconfirmed provider attempts became visible") }\n\tif key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "plus" {',1)
        s=s.replace('\ttried = nil\n\t_, _ = collectStream', '\ttried, visible = nil, nil\n\t_, _ = collectStream',1).replace('\tif len(tried) != 1 || tried[0] != "plus" {', '\tif len(tried) != 1 || tried[0] != "plus" || !visible[0] {',1)
        s=s.replace('\ttried = nil\n\ttext, _ = collectStream', '\ttried, visible = nil, nil\n\ttext, _ = collectStream',1).replace('\tif text != "hello" || len(tried) != 1 || tried[0] != "free" {', '\tif text != "hello" || len(tried) != 1 || tried[0] != "free" || visible[0] {',1)
        needle='\t\tt.Fatalf("provider access change required a model-name rule: %q %v", text, tried)\n\t}\n';s=s.replace(needle,needle+'\tif key, present := router.PeekRoutedAccountKey("openai-codex", request.SessionID, model.ID); !present || key == nil || *key != "free" { t.Fatal("accepted free route was not confirmed") }\n',1)
        s=s.replace('if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {\n\t\t\t\tt.Fatal("old attempt cleanup erased the newer account")','if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {\n\t\t\t\tt.Fatal("old cleanup exposed an account while a newer unverified attempt was pending")',1)
        a=s.index('func TestUsageRouteCancellationDoesNotLeaveAnAccount');b=s.index('func TestUsageRouteHidesPreviouslySuccessfulRejectedAccount',a);part=s[a:b].replace('router := ai.NewAccountRouter(newFakeAccountStore(accountInfo("p", keyOf("a"), true)))','store := newFakeAccountStore(accountInfo("p", keyOf("a"), true), accountInfo("p", keyOf("b"), false))\n\trouter := ai.NewAccountRouter(store)',1).replace('if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present {\n\t\tt.Fatal("active attempt was not visible")','if _, present := router.PeekRoutedAccountKey("p", "s", model.ID); present {\n\t\tt.Fatal("unverified attempt became visible")',1)
        part=part.rstrip()[:-1]+'''\tstore.block("p", keyOf("a"), "", time.Now().Add(time.Hour).UnixMilli())
        	_, err := router.ImagesRoutes(t.Context(), model, ai.AccountRouteRequest{SessionID:"s"}, func(context.Context, ai.AccountSelection, ai.AccountObserve, bool)(ai.AssistantImages,error){return ai.AssistantImages{StopReason:ai.ImagesStopReasonStop},nil})
        	if err != nil {t.Fatal(err)}
        	if key, present := router.PeekRoutedAccountKey("p", "s", model.ID); !present || key == nil || *key != "b" {t.Fatal("a cancelled attempt masked a later confirmed account")}
        }
        
        ''';s=s[:a]+part+s[b:];p.write_text(s)
        p=Path('vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go');s=p.read_text().replace('// UsageAccountKey reports the live attempt or latest successful account for this session\'s','// UsageAccountKey reports the accepted live attempt or last confirmed account for this session\'s');p.write_text(s)
        PY
        cat >> vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go <<'EOF'
        
        func TestUsageRouteConfirmsAccountWhileStreaming(t *testing.T){
         router:=ai.NewAccountRouter(newFakeAccountStore(accountInfo("p",keyOf("a"),true)))
         model:=okModel("p","unlisted-streaming-model")
         source:=ai.NewAssistantMessageEventStream()
         partial:=&ai.AssistantMessage{Provider:"p",Model:model.ID,Content:[]ai.AssistantContentBlock{ai.TextContent{Text:"hello"}}}
         _=source.Push(ai.StartEvent{Partial:partial})
         ctx,cancel:=context.WithTimeout(t.Context(),2*time.Second);defer cancel()
         opened:=make(chan struct{})
         stream:=router.StreamRoutes(ctx,model,ai.AccountRouteRequest{SessionID:"s"},func(context.Context,ai.AccountSelection,ai.AccountObserve,bool)(*ai.AssistantMessageEventStream,error){close(opened);return source,nil})
         select{case <-opened:case <-ctx.Done():t.Fatal("stream did not open")}
         if _,present:=router.PeekRoutedAccountKey("p","s",model.ID);present{t.Fatal("start-only response was treated as accepted content")}
         _=source.Push(ai.TextDeltaEvent{ContentIndex:0,Delta:"hello",Partial:partial})
         received:=false
         for event:=range stream.Events(ctx){
          if _,ok:=event.(ai.TextDeltaEvent);ok{
           received=true
           if key,present:=router.PeekRoutedAccountKey("p","s",model.ID);!present || key==nil || *key!="a"{t.Fatal("accepted live content did not update usage before completion")}
           _=source.Push(ai.DoneEvent{Reason:ai.StopReasonStop,Message:&ai.AssistantMessage{Provider:"p",Model:model.ID,Content:partial.Content,StopReason:ai.StopReasonStop}})
          }
         }
         if !received{t.Fatal("stream did not deliver content")}
        }
        
        func TestUsageRouteDenialDoesNotInvalidateAnotherModel(t *testing.T){
         store:=newFakeAccountStore(accountInfo("openai-codex",keyOf("a"),true))
         store.put("openai-codex",keyOf("a"),ai.Credential{Type:ai.CredentialOAuth,Access:codexToken("free")})
         router:=ai.NewAccountRouter(store)
         first,second:=okModel("openai-codex","future-one"),okModel("openai-codex","future-two")
         request:=ai.AccountRouteRequest{SessionID:"s"}
         for _,model:=range []*ai.Model{first,second}{
          _,err:=router.ImagesRoutes(t.Context(),model,request,func(context.Context,ai.AccountSelection,ai.AccountObserve,bool)(ai.AssistantImages,error){return ai.AssistantImages{StopReason:ai.ImagesStopReasonStop},nil})
          if err!=nil{t.Fatal(err)}
         }
         _,_=router.ImagesRoutes(t.Context(),first,request,func(ctx context.Context,selection ai.AccountSelection,observe ai.AccountObserve,force bool)(ai.AssistantImages,error){
          observe(ai.ProviderResponse{Status:400})
          return ai.AssistantImages{StopReason:ai.ImagesStopReasonError,ErrorMessage:"The 'future-one' model is not supported when using Codex with a ChatGPT account."},nil
         })
         if _,present:=router.PeekRoutedAccountKey("openai-codex","s",first.ID);present{t.Fatal("rejected model retained its bar")}
         if _,present:=router.PeekRoutedAccountKey("openai-codex","s",second.ID);!present{t.Fatal("model-specific denial invalidated an unrelated model")}
        }
        EOF
        gofmt -w vendor/github.com/MichaelKinsy/PiG/ai/account_routing_usage_test.go vendor/github.com/MichaelKinsy/PiG/internal/codingagent/interactive.go
        git diff --check
        go test -mod=vendor github.com/MichaelKinsy/PiG/ai github.com/MichaelKinsy/PiG/internal/codingagent -run 'TestUsageRoute|TestPromptQuota' -count=1
    ok      github.com/MichaelKinsy/PiG/ai    0.320s
    ok      github.com/MichaelKinsy/PiG/internal/codingagent    0.054s
    ✓ • 1m 35s

## User

commit everything, push to remote, create a PR
