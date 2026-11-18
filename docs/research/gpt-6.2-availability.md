# GPT-6.2 availability (primary-source check)

## Confirmed findings

- **Release:** OpenAI announced "Introducing GPT-6.2 Astra and Sol" on
  2026-11-17 17:45 GMT. The official news feed describes it as available
  "across ChatGPT Work, Codex, and the API." The GPT-6.2 System Card, published
  the same date, says "today, we are releasing GPT-6.2 Astra and GPT-6.2 Sol."
  The API changelog records the models on 2026-11-17.
- **Canonical model IDs:** `gpt-6.2-astra` and `gpt-6.2-sol`. The API model
  catalog lists both under flagship models; each model page gives one ID and
  no aliases. "GPT-6.2" by itself is the family name, not a documented API ID.
- **OpenAI API:** Both models are available through `v1/responses` and
  `v1/chat/completions`, with Batch support; the model docs recommend Responses
  for tools such as web search and computer use.
  Requests setting reasoning effort to `none` are rejected. Standard short
  context API prices are $8/$40 per 1M input/output tokens for Astra and
  $2/$10 for Sol. Access uses a normal API key and API billing; the migration
  guide describes ZDR as a separate option for approved organizations.
- **ChatGPT subscriptions and Codex:** Both models are documented as available
  on Plus, Pro, Business, Enterprise, and Edu in ChatGPT Work (web, mobile,
  desktop) and in the Codex app and CLI. They are off by default in Enterprise
  and Edu workspaces until an administrator enables them. Codex CLI accepts
  `codex --model gpt-6.2-sol` and `model = "gpt-6.2-sol"` in `config.toml`.
  API-key sessions in Codex follow API model availability; API-key billing is
  separate from a ChatGPT subscription allowance.
- **Surface distinction:** These are work/agent models. The official guidance
  here names ChatGPT Work and Codex, not the regular ChatGPT Chat model picker.

## Sources

- OpenAI news feed: https://openai.com/news/rss.xml
- Launch announcement: https://openai.com/index/introducing-gpt-6-2-astra-sol/
- GPT-6.2 System Card: https://deploymentsafety.openai.com/gpt-6-2-astra-sol
- API changelog: https://developers.openai.com/api/docs/changelog.md
- Astra model page: https://developers.openai.com/api/docs/models/gpt-6.2-astra.md
- Sol model page: https://developers.openai.com/api/docs/models/gpt-6.2-sol.md
- All models: https://developers.openai.com/api/docs/models/all.md
- API pricing: https://developers.openai.com/api/docs/pricing.md
- Migration guide: https://developers.openai.com/api/docs/guides/latest-model
- ChatGPT/Codex models: https://learn.chatgpt.com/docs/models.md
- ChatGPT/Codex pricing and feature availability: https://learn.chatgpt.com/docs/pricing.md

## Limitations

- The launch announcement page and the Help Center article on models in
  ChatGPT returned HTTP 403 from this environment. The announcement URL and
  date are confirmed by the accessible OpenAI news feed; no claim here depends
  on the unreadable page body.
- No account-specific model picker, API request, or credential was used, so
  per-account or regional rollout differences were not observed. Plan and
  workspace requirements above are documented requirements, not a guarantee of
  availability in every client.

## Initial ompig-slim diagnosis

- The bundled catalog is pinned to `@oh-my-pi/pi-catalog` 18.14.2, and its
  OpenAI and Codex rows include GPT-6.1 but no GPT-6.2. See the
  [manifest](../../vendor/github.com/MichaelKinsy/PiG/ai/omp_catalog_data/manifest.json)
  and [embedded catalog owner](../../vendor/github.com/MichaelKinsy/PiG/ai/omp_catalog.go).
- The OpenAI API provider can learn new model IDs from the shared public
  catalog. `update --models` forces a refresh, persisting those rows for the
  next process. See [public catalog membership](../../vendor/github.com/MichaelKinsy/PiG/internal/codingagent/public_model_catalog.go)
  and the [refresh command](../../vendor/github.com/MichaelKinsy/PiG/coding/cli/package_model_catalogs.go).
- Codex is marked `accountScoped: true` in the
  [provider policy](../../vendor/github.com/MichaelKinsy/PiG/ai/omp_catalog_data/provider-policy.json),
  so the public feed cannot add subscription models. The inherited remote
  overlay is also skipped for OMP-owned providers unless explicitly configured
  ([registry](../../vendor/github.com/MichaelKinsy/PiG/internal/codingagent/remote_catalog_registry.go)).
  `codex-json` copies the bundled Codex roster at initialization
  ([implementation](../../vendor/github.com/MichaelKinsy/PiG/ai/omp_codex_json.go)).
  Those providers need a maintained catalog update or an explicit
  [custom model definition](../custom-models.md); refreshing the public feed
  alone does not add GPT-6.2 to their rosters.

### Observed verification

Using `bin/ompig-slim` version 0.2.1 with a temporary home and agent directory,
no real credentials, and a dummy OpenAI key only for offline listing:

1. The bundled OpenAI listing contained GPT-6.1, not GPT-6.2.
2. `update --models`, run without any credentials, succeeded and saved public
   metadata for `gpt-6.2-sol` and `gpt-6.2-astra`.
3. A subsequent offline `--list-models 'openai gpt-6.2'` listed both models
   with a 1,050,000-token context and 128,000-token output limit.

These observations predate the automatic discovery change. They verify public
catalog discovery and persistence, not successful inference or account entitlement.
The diagnosis did not change runtime code or real user configuration.

The current behavior is described in [model discovery](../model-discovery.md).
Codex now discovers its account roster rather than requiring a bundled catalog
update for each new model.
