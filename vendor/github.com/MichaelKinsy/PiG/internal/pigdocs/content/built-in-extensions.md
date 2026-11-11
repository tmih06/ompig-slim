# Built-in extensions

ompig-slim contains the Pi-compatible coding agent and generic support for
extensions, Packages, Piglets, Piglet Binaries, and local reference documents.
It activates no optional product extension. Its startup header shows the OM + Pi
mark (D2), and its built-in `pig-login` extension registers `/sprite`.

## Built-in capabilities

| Capability | Purpose |
|---|---|
| `ompig-slim docs` | Materialize and read the documentation that matches the binary. |
| `ompig-slim extension ...` | Author, inspect, validate, and run ordinary extensions. |
| `ompig-slim install` | Install Packages that distribute Resources. |
| `ompig-slim piglet ...` | Validate, register, inspect, and build Piglets. |
| Piglet Binary verification | Verify the embedded Piglet and component closure before startup. |

These capabilities do not select a product composition.

## Optional compositions

A Piglet selects ordinary extension Resources through public Piglet contracts.
This repository ships no composition of its own: a bare `ompig-slim` loads none
of them, and every composition is one you write or install.

A composition is a Piglet YAML file. Point the binary at it directly:

```bash
ompig-slim --piglet ./my-piglet.yaml
```

Or build it into a directly executable binary:

```bash
ompig-slim piglet build ./my-piglet.yaml --format binary --out ./my-agent
```

Installing a Package can make a composition's Resources available. It does not
create, select, or activate the composition.

## Login identity

This build's startup header shows the OM + Pi mark with the version and hints
beside it, where Pi shows its logo (D2). The built-in `pig-login` extension
registers `/sprite`, which chooses one of fifteen built-in sprites or a sprite
an extension registered with `ui.registerSprite`, and saves the choice in
`$PIG_HOME/state/pig-standard/login.json`. Use `/sprite` to open a picker,
`/sprite list` and `/sprite set <id>` for text-based control, and
`/sprite preview [id]` to see a sprite's mascot.

A composition can replace the header with its own login through `ui.setLogin`.
Such an extension installs its own artwork and can replace the built-in `/sprite`
command.

## Additional extensions

An additional extension must work as a source subprocess extension and, when the
composition is built into a binary, as a fused component. Each is an ordinary
extension Resource the Piglet selects; ompig-slim does not import or activate it.
