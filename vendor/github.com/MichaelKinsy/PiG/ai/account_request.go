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
