# Mistral browser account identity

Mistral browser sign-in supports multiple accounts through the existing local or
broker credential store. Repeat `/login` → **Sign in with Mistral** for each
account. List and switch accounts with:

```sh
ompig-slim auth accounts --provider mistral
ompig-slim auth use mistral '<account-key-from-listing>'
```

Login fetches `GET https://api.mistral.ai/v1/users/me` with the minted key as a
Bearer token. The returned user ID identifies the account; when present, the
workspace ID qualifies it so separate workspaces remain distinct. Email is
optional and is retained as the display label, not as the unique account key.
Repeated sign-in to the same user/workspace updates its credential.

Identity discovery is best-effort and bounded to five seconds. Rejected keys,
network failures, and invalid identity responses do not fail an otherwise
successful sign-in: a non-secret key fingerprint distinguishes the credential
instead. When discovery fails, different minted keys may appear as separate
entries even if they belong to the same user. Identity requests do not follow
redirects. No live credentials were used to verify these endpoints.

Existing unnamed browser credentials are retained when adding a new account;
their email is not fetched retroactively. Remove an obsolete unnamed entry with
the `/logout` account picker after signing in again. Manually entered API keys
continue to use key-based identities rather than fetching account email.

## Upstream evidence

Inspected mistralai/mistral-vibe at
[`b20313e01d6926d4191da89c32d5da11e3a0a060`](https://github.com/mistralai/mistral-vibe/tree/b20313e01d6926d4191da89c32d5da11e3a0a060).

- [`vibe/core/identity.py`](https://github.com/mistralai/mistral-vibe/blob/b20313e01d6926d4191da89c32d5da11e3a0a060/vibe/core/identity.py)
  defines `/users/me` and the response fields `id`, optional `email`,
  `workspace`, and `organization`. Identity discovery fails open.
- [`vibe/app_server/_identity.py`](https://github.com/mistralai/mistral-vibe/blob/b20313e01d6926d4191da89c32d5da11e3a0a060/vibe/app_server/_identity.py)
  passes the provider's API base to identity discovery.
- [`vibe/core/config/_defaults.py`](https://github.com/mistralai/mistral-vibe/blob/b20313e01d6926d4191da89c32d5da11e3a0a060/vibe/core/config/_defaults.py)
  contains the default Mistral provider API base, `https://api.mistral.ai/v1`.
- [`vibe/setup/auth/http_browser_sign_in_gateway.py`](https://github.com/mistralai/mistral-vibe/blob/b20313e01d6926d4191da89c32d5da11e3a0a060/vibe/setup/auth/http_browser_sign_in_gateway.py)
  exchanges browser authorization for `api_key`, not an email-bearing token.
- [`vibe/setup/auth/whoami.py`](https://github.com/mistralai/mistral-vibe/blob/b20313e01d6926d4191da89c32d5da11e3a0a060/vibe/setup/auth/whoami.py)
  uses `/api/vibe/whoami` for plan and tenant data, not user email.
