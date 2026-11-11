# Terminal setup

ompig-slim runs in any terminal. Some features need terminal support, and ompig-slim turns each
one on only when it detects that support. This page lists what ompig-slim detects, and
what to change when a key or an image does not work.

## What ompig-slim detects

ompig-slim reads environment variables to decide what your terminal accepts. It does not
ask the terminal, because a terminal that does not answer would delay startup.

| Terminal | Images | True color | Hyperlinks |
|---|---|---|---|
| Kitty | yes | yes | yes |
| Ghostty | yes | yes | yes |
| WezTerm | yes | yes | yes |
| iTerm2 | yes | yes | yes |
| Warp | yes | yes | yes |
| VS Code | no | yes | yes |
| Alacritty | no | yes | yes |
| Zed | no | yes | yes |
| Windows Terminal | no | yes | yes |
| JetBrains | no | yes | no |
| Apple Terminal | no | with `COLORTERM` | no |

Inside `tmux` or `screen`, ompig-slim turns images off. The multiplexer
sits between ompig-slim and the terminal, so what the outer terminal accepts is not
evidence that the pane accepts it. Hyperlinks stay off in `screen`. In `tmux`,
ompig-slim asks tmux whether the attached client forwards hyperlinks and uses them
only when it does.

ompig-slim reads `TERM_PROGRAM`, `TERM` and `COLORTERM` first. It also accepts
`KITTY_WINDOW_ID`, `GHOSTTY_RESOURCES_DIR`, `WEZTERM_PANE`, `ITERM_SESSION_ID`,
`WARP_SESSION_ID` and `WT_SESSION` as evidence, because some terminals set only
these. A terminal it does not recognize gets true color only with
`COLORTERM=truecolor` or `COLORTERM=24bit`, and no images or hyperlinks.

To correct detection, set `PI_HYPERLINKS`, `PI_IMAGE_PROTOCOL` or `PI_TRUE_COLOR`
(see [environment variables](environment-variables.md)), or the matching
`terminal.*` setting (see [settings](settings.md)). The setting wins.

## Keys that do not respond

A terminal sends a key to ompig-slim only if it does not use the key itself. When a
binding does nothing, the terminal usually consumed the key.

Check first whether the key works outside ompig-slim. If the terminal has a command
bound to it, unbind the command or bind the ompig-slim action to a different key. See
[keybindings](keybindings.md).

### tmux

`tmux` must send extended keys, or modified keys such as `shift+enter` and
`ctrl+enter` arrive as their unmodified form. ompig-slim warns at startup when it finds
the wrong setting.

Add both lines to `~/.tmux.conf` and restart tmux:

```tmux
set -g extended-keys on
set -g extended-keys-format csi-u
```

`extended-keys-format xterm` also sends modified keys, but in a format that
carries less information. ompig-slim warns about it and works better with `csi-u`.

ompig-slim does not warn when it cannot query tmux, because a sandbox that blocks the
query is not evidence of a wrong setting.

### Apple Terminal

ompig-slim enables enhanced key reporting when available. If Terminal.app still sends
plain Return for `shift+enter`, ompig-slim asks macOS whether Shift is held and
treats the key as `shift+enter`. This works only when ompig-slim runs on the same Mac
as Terminal.app. Over SSH, ompig-slim cannot see the local modifier state.

### Termux

Termux reports its window size differently from a desktop terminal. ompig-slim detects
Termux with `TERMUX_VERSION` and does not repaint the whole buffer when the
window changes, because the repaint costs more than it corrects there.

## Images

ompig-slim shows images when the terminal accepts the Kitty graphics protocol or the
iTerm2 protocol. In every other terminal ompig-slim writes the file path instead, so no
information is lost.

Images are off inside `tmux` and `screen`. To see images, run ompig-slim outside the
multiplexer, or set `PI_IMAGE_PROTOCOL` when you know the multiplexer forwards
them.

## Display problems

| Symptom | Cause | What to do |
|---|---|---|
| ompig-slim exits with `Rendered line N exceeds terminal width` | a component wrote a row wider than the terminal | fix the component to truncate its rows to the render width; the crash log lists every rendered row with its width |
| colors look wrong | the terminal reports no true color | set `COLORTERM=truecolor` if the terminal supports it |
| a stray `[?62;22c` appears | the terminal answered a capability query late | ompig-slim consumes this answer; report it with the terminal name |
| in fullscreen mode, rows show text that belongs elsewhere after the transcript scrolls | the terminal does not move rows inside a scroll region correctly | set `PIG_TUI_SCROLL_ROWS=0`, and report it with the terminal name |

In fullscreen mode, when the transcript grows or scrolls, ompig-slim moves the rows that only moved with a scroll region and draws only the rows that changed. Pi draws every row that changed (D205). The screen is the same, and ompig-slim writes a fraction of the bytes to the terminal, which matters most through tmux or ssh. Set `PIG_TUI_SCROLL_ROWS=0` to draw every changed row, as Pi does.

Set `PIG_RENDER_DEBUG` to any value to write render diagnostics. Use it when you
report a display problem.

As in Pi, a row wider than the terminal is fatal only when it reaches a differential render. ompig-slim then writes `pi-tui-crash.log` to the agent directory (the system temp directory when there is none) with the terminal width, the offending row and its width, and every rendered row, restores the terminal, prints the error with the log path, and exits with status 1. The first render, a forced full render, and the full render after a resize emit an over-wide row unchanged.

## Related

- [Keybindings](keybindings.md) lists the actions and their default keys.
- [Config](config.md) lists the files and directories ompig-slim reads.
