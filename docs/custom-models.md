# Custom models

A custom model is a provider entry in `~/.pi/agent/models.json` (JSON) or
`~/.pi/agent/models.yml` (YAML). It is how an endpoint this binary does not ship
— a gateway, a proxy, a local server, a newly released model — becomes selectable
without a rebuild.

Two files can exist; only one is read.

| File | Format | Read when |
|---|---|---|
| `models.yml` | YAML | present |
| `models.yaml` | YAML | present, and no `models.yml` |
| `models.json` | JSON, comments and trailing commas allowed | otherwise |

A stale `models.json` beside a `models.yml` is ignored without a warning, so a
provider that "disappeared" usually means a YAML file took over. Project-local
`.pi/models.json` is not read; the file is global. `OMPIG_SLIM_CODING_AGENT_DIR`
relocates the whole agent directory.

## Shape

```json
{
  "providers": {
    "my-gateway": {
      "name": "My Gateway",
      "api": "openai-completions",
      "baseUrl": "https://gateway.internal/v1",
      "apiKey": "$MY_GATEWAY_KEY",
      "models": [
        {
          "id": "vendor/model-name",
          "name": "Vendor Model",
          "reasoning": true,
          "contextWindow": 200000,
          "maxTokens": 32000,
          "input": ["text", "image"],
          "cost": { "input": 0.5, "output": 1.5, "cacheRead": 0.05, "cacheWrite": 0.5 }
        }
      ]
    }
  }
}
```

`providers` is required and is the only required top-level member; a `$schema`
member is accepted and ignored.

## Provider fields

| Field | Meaning |
|---|---|
| `name` | display name |
| `api` | wire protocol; see the table below |
| `baseUrl` | endpoint root, for this provider's custom **and built-in** models |
| `apiKey` | literal, `$VAR`, or `!command` (see Credentials) |
| `headers` | extra request headers, interpolated like `apiKey` |
| `authHeader` | send the key as `Authorization: Bearer <key>` as well |
| `compat` | protocol quirks, shared by this provider's models |
| `models` | custom model definitions |
| `modelOverrides` | per-model adjustments to models that already exist |
| `insecure` | skip TLS verification for this endpoint (self-signed/internal CA) |
| `oauth` | `"radius"` only; a Radius gateway login flow, and it needs `baseUrl` |

`api` is the protocol, not the vendor. These are the values with a transport in
the binary:

`openai-completions`, `openai-responses`, `azure-openai-responses`,
`openai-codex-responses`, `anthropic-messages`, `google-generative-ai`,
`google-vertex`, `bedrock-converse-stream`, `mistral-conversations`,
`pi-messages`.

An unknown `api` is **not** an error. It is accepted by the schema and then
handled by the fallback branch, so a typo such as `openai-completionss` quietly
speaks OpenAI Completions to `baseUrl` — with the wrong request shape if the
endpoint is not actually OpenAI-compatible. When a custom endpoint misbehaves,
read `api` first.

A provider that defines custom models must also name `api` and `baseUrl`, at the
provider or model level; either one missing rejects the whole file (see
[Failures](#failures)).

## Model fields

| Field | Default | Notes |
|---|---|---|
| `id` | required | the value sent as `model` |
| `name` | `id` | display name |
| `api`, `baseUrl` | inherited | override the provider's for this model |
| `reasoning` | `false` | gates every reasoning parameter |
| `thinkingLevelMap` | provider default | per-level wire value; `null` marks a level unsupported |
| `contextWindow` | `128000` | must be > 0 |
| `maxTokens` | `16384` | must be > 0; the output limit |
| `input` | `["text"]` | only `text` and `image` are accepted |
| `cost` | none | needs all four of `input`, `output`, `cacheRead`, `cacheWrite` |
| `samplingParams` | none | sent verbatim, e.g. `temperature`, `top_p` |
| `samplingParamsByThinkingLevel` | none | per-level sampling overrides |
| `headers`, `compat` | none | merged over the provider's |
| `inputLimits`, `promptCache` | none | image resize caps; cache lifetime hints |

Leaving `contextWindow` and `maxTokens` out is the most common mistake: the model
then reports 128K/16.4K in `--list-models` and the harness compacts far earlier
than the endpoint can actually handle.

`reasoning: true` without `thinkingLevelMap` still sends `reasoning_effort`:
`medium` by default and the requested level for `:high` and friends. A level the
map omits keeps the provider default.

## Credentials

`apiKey` and every header value resolve through one rule set:

| Value | Resolves to |
|---|---|
| `sk-abc123` | the literal string |
| `$MY_KEY`, `${MY_KEY}` | that environment variable |
| `!op read op://vault/key` | the command's trimmed stdout (10 s timeout, cached per process) |

A bare name is **literal**: `"apiKey": "OPENAI_API_KEY"` sends the string
`OPENAI_API_KEY`, not the environment variable. The `$` is required. `$$` and
`$!` escape a literal `$` and `!`.

An unset variable does not fall back to anything:

```text
$ ompig-slim auth print-api-key --provider my-gateway
Error: Failed to resolve credential
```

and a header fails the request with
`API key auth failed for provider my-gateway: Failed to resolve provider "my-gateway" header "X-A" from environment variable: MISSING`.
The key is resolved at request time, so a missing variable shows up on the first
turn, not at startup.

`authHeader` matters only where the API module does not add a bearer itself:
`anthropic-messages` sends `x-api-key`, and `authHeader: true` adds
`Authorization: Bearer …` on top. `openai-completions` already sends the bearer,
so the flag changes nothing there.

An endpoint that wants no authentication still needs a value in `apiKey`; the
request path refuses to run without one (`No API key found for my-gateway.`). A
placeholder such as `"apiKey": "not-needed"` is the usual workaround, and
`apiKey: "!echo not-needed"` keeps it out of the file entirely.

A key can also be stored once instead of written into the file:
`ompig-slim auth login my-gateway --api-key` (interactive) saves it in
`agent.db`, and `ompig-slim auth check --provider my-gateway` reports
`ready`, `not_ready`, or `invalid`.

## Built-in providers

A `providers` entry named after a shipped provider extends it instead of replacing
it, which covers a proxy, a mirror, or a region:

```json
{
  "providers": {
    "anthropic": { "baseUrl": "https://anthropic-proxy.internal" }
  }
}
```

- `baseUrl` applies to that provider's built-in models too.
- `models` **adds** entries beside the catalog ones.
- `modelOverrides` adjusts a model that already exists, keyed by its ID:

  ```json
  { "providers": { "anthropic": { "modelOverrides": {
      "claude-sonnet-4-5-20250929": { "contextWindow": 200000 }
  } } } }
  ```

`modelOverrides` never creates a model. A key that matches nothing is silently
ignored.

## Selecting one

```sh
ompig-slim --list-models                    # every selectable provider/model
ompig-slim --list-models my-gateway         # fuzzy filter
ompig-slim --model my-gateway/vendor/model-name
ompig-slim --model vendor/model-name        # unique match, provider prefix optional
ompig-slim --model my-gateway/vendor/model-name:high   # with a thinking level
```

A custom model can be a saved default or role in `settings.json`
(`defaultProvider`/`defaultModel`, `modelRoles`), and then reached with
`--model @role` or the picker. See [model roles](model-roles.md).

`--api-key <key>` overrides the file's key for one run, and requires `--model` or
`--provider` to say which provider it is for.

Editing the file affects the next process; a running session keeps the models it
loaded.

## Failures

The file is validated as a whole, and one bad member rejects every provider in it:

```text
$ ompig-slim --list-models
Warning: errors loading models.json:
Invalid models.json schema:
  - providers.my-gateway.models.0.contextWindow: must be number
File: /home/ubuntu/.pi/agent/models.json
```

The path in the message names the exact member. Other messages this produces:

| Message | Cause |
|---|---|
| `must have required properties providers` | no `providers` key |
| `must have required properties id` | a model without `id` |
| `must have required properties output, cacheRead, cacheWrite` | partial `cost` |
| `must be equal to one of the allowed values` | `input` other than `text`/`image` |
| `Failed to parse models.json: …` | syntax error, with line and column |
| `no "api" specified. Set at provider or model level.` | custom models with no `api` |
| `"baseUrl" is required when defining custom models.` | custom models with no `baseUrl` |
| `Provider "x": …` | the failure is scoped to one provider, but it still blocks the file |

Symptom to cause, when a custom provider does not show up at all:

1. `models.yml` exists and shadows `models.json`.
2. The file is in a project `.pi/` instead of `~/.pi/agent/`.
3. The file failed validation — the warning appears on stderr, which is easy to
   miss in a TUI.
4. The provider has an empty `models` array and no `baseUrl`, so it contributes
   nothing on its own.

`ompig-slim diagnose` lists only built-in providers, so it is not a way to check
a custom entry. `--list-models` is.
