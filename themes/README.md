# Optional themes

These 100 palettes are repository assets, not embedded in the binary or installed
by default. The built-in themes are `system`, `dark`, and `light`; the default
follows the terminal's light/dark appearance.

From a checkout, try one for a single run:

```sh
ompig-slim --theme ./themes/titanium.json --use-theme titanium
```

Install only the palettes you want into your agent profile:

```sh
mkdir -p ~/.pi/agent/themes
cp themes/titanium.json ~/.pi/agent/themes/
```

Then select `titanium` in `/settings`, or run
`ompig-slim --use-theme titanium`. Use your configured agent directory instead
of `~/.pi/agent` when you have overridden it. To install the whole collection:

```sh
cp themes/*.json ~/.pi/agent/themes/
```

An existing saved selection such as `titanium` now requires installing its file.
An unavailable selection uses the existing system-theme fallback; choose `dark`
or `light` in `/settings` if you do not want to install a palette.

The files are the individual documents from oh-my-pi's
`packages/tui/src/theme/defaults.json`, distributed under MIT. See the root
[LICENSE](../LICENSE) and [NOTICE](../NOTICE). No installer or executable extension
is needed; these files use the normal custom-theme loader.
