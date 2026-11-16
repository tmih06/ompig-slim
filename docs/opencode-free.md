# Use OpenCode's free models

Run a chat model directly from ompig-slim:

```sh
ompig-slim --model opencode-free/deepseek-v4.1-flash-free
```

No login, extension, OpenCode installation, Bun, additional binary, or local
server is required. ompig-slim executes its own tools and parses the upstream
OpenAI stream. The existing `opencode` and `opencode-go` providers are unchanged.
`--offline` disables startup discovery, not explicitly requested inference.

## Find models in the hub

1. Run `/model`.
2. Press Right to focus the model browser.
3. Search for `opencode-free`.

The catalog contains 14 chat models and Jev as a judge model. Filter by
**judge** to find **Jev 1.13 Free**. Jev cannot be selected as the chat model.

When building from source, run `make install` to update the installed command.
`make build` updates only `bin/ompig-slim`. Restart existing sessions after
installation.

## Free model catalog

All IDs below use the `opencode-free/` provider prefix. Catalog prices are zero.
Chat limits come from the [OpenCode models endpoint](https://opencode.ai/inference/v1/models).
The catalog also retains models from the [documented free-model list](https://opencode.ai/docs/console/models/).

| Model ID | Kind | Context tokens | Maximum output tokens |
| --- | --- | ---: | ---: |
| `big-pickle` | chat | 200000 | 32000 |
| `mimo-v2.6-flash-free` | chat | 200000 | 32000 |
| `deepseek-v4.1-flash-free` | chat | 200000 | 32000 |
| `glm-5.3-flash-free` | chat | 200000 | 32000 |
| `iceman-free` | chat | 262144 | 131072 |
| `inkling-2-small-free` | chat | 1048576 | 131072 |
| `beam-free` | chat | 1048576 | 131072 |
| `flint-40b-free` | chat | 262144 | 65536 |
| `longcat-2.5-free` | chat | 1000000 | 262144 |
| `hy4-flash-free` | chat | 1048576 | 65536 |
| `nemotron-3-ultra-free` | chat | 128000* | 16384* |
| `nemotron-3.5-lightning-free` | chat | 262144 | 262144 |
| `nemotron-3.5-super-vl-free` | chat | 1048576 | 131072 |
| `minimax-m3.1-flash-free` | chat | 1000000 | 512000 |
| `jev-1.13-free` | judge | 32000 | Not applicable |

*OpenCode documents Nemotron 3 Ultra but omits it from current discovery.
Its route returned `400 Model is unavailable` during verification. Its local
limits are conservative defaults, not measured upstream limits. The entry stays
visible so the documented catalog is complete. Registration cannot restore an
unavailable upstream model.

Chat models expose text and image inputs where the native runtime and upstream
both support them. Audio and video input formats are not exposed. Reasoning
effort is sent only for models that advertise it, and only with a value that
model publishes: OpenCode's per-model effort list decides which levels the
`/model` manager and `--thinking` accept. A model that reasons without an
effort control sends no effort. The runtime applies its normal
output budgets, including an explicit caller budget when supplied.

## Use Jev as a judge

Jev uses the existing classifier API, not chat completions. Assign
`opencode-free/jev-1.13-free` to a judge role, or call it from codemode:

```js
const result = await models.classify("opencode-free/jev-1.13-free", {
	state: { answer: 4 },
	questions: {
		correct: {
			type: "bool",
			instructions: "Is the answer to 2 + 2 correct?",
			criteria: { true: "Correct", false: "Incorrect" },
		},
	},
});
text(result);
```

The native classifier sends this request to
`https://opencode.ai/inference/systemone/v1/systemone` with the public free-tier
credential. It returns typed answers and probabilities through the existing
judge-model pipeline.

## Compatibility limits

Chat requests go directly to
`https://opencode.ai/inference/openai/v1/chat/completions`. Both APIs use fresh
session-affinity identifiers, OpenCode-compatible headers, and a shared native
Go uTLS client matching Bun 1.4.4's TLS profile. Certificate verification remains
enabled. The provider does not read saved OpenCode credentials or sessions.

Chat requests prepend the complete embedded OpenCode system preamble, with the
selected model's identity. The preamble costs roughly 3,000 context tokens and
describes tools that ompig-slim does not implement. An appended instruction
restricts calls to the real tools supplied with the request. Caller tools,
results, and instructions remain intact. The gateway may reject requests
without tools. Jev requests do not use the chat preamble.

Requests have a five-minute timeout and cancel with the caller's context. The
client does not follow redirects. Quota and inference errors use the standard
runtime error and retry policy. OpenCode controls availability, quotas, routing,
and billing. This experimental compatibility client is not an official
OpenCode client. Gateway changes can break it. Review the service's terms before
use.

## Verify the implementation

```sh
make check
make smoke
python3 scripts/smoke-test-opencode-free.py ./bin/ompig-slim --live
```

The smoke test uses an isolated profile and an empty executable search path.
It checks CLI discovery and searches for every model in the real `/model` hub.
With `--live`, it asks each chat model to read a random temporary file through
ompig-slim's `read` tool. It reports the known unavailable Nemotron 3 Ultra route
separately. Other failures fail the test.

The Go regression suite checks all chat routes and Jev's native judge route
against an HTTP fixture. It checks model metadata, keyless availability,
selected identities, caller tools, fresh affinity headers, request budgets,
reasoning compatibility, and quota errors.

The implementation is in `vendor/github.com/MichaelKinsy/PiG/ai/opencode_free*.go`.
The preamble and identifier encoding derive from OpenCode at
`b75b1583962ddc9ad2a057d3a3641b770321aa9e`. The preamble comes from a synthetic
session, with temporary working paths replaced by `.`. The MIT notice is in
`vendor/github.com/MichaelKinsy/PiG/ai/opencode_free_data/LICENSE`.
