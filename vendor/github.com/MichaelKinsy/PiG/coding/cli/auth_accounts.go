package cli

// Ports packages/coding-agent/src/cli/auth-accounts.ts and the account branch of auth-logout.ts:
// `auth accounts`, `auth use`, and `auth logout [--account <key>]`.
//
// These read and write the same credential store the agent uses, and match providers against the
// composed provider catalog (exact id or display name, case-insensitively), falling back to a
// stored provider id for providers only an extension registers. A store without multi-account
// support reports that instead of pretending an unknown account exists.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// authAccountsTimeout bounds a listing command, as upstream's AbortSignal.timeout(15_000) does.
const authAccountsTimeout = 15 * time.Second

func authAccountsUsage() string {
	return codingagent.AppName + " auth accounts [--provider <provider>] [--json]"
}

func authUseUsage() string { return codingagent.AppName + " auth use <provider> <account>" }

func authLogoutUsage() string {
	return codingagent.AppName + " auth logout <provider> [--account <key>]"
}

// authAccountRow is one `auth accounts` line. The active account is marked with "*".
func authAccountRow(account ai.CredentialAccountInfo) string {
	marker := " "
	if account.Active {
		marker = "*"
	}
	label := ""
	if account.Label != "" {
		label = " (" + widthx.ReplaceControlCharacters(account.Label) + ")"
	}
	key := "<no identity>"
	if account.AccountKey != nil {
		key = widthx.ReplaceControlCharacters(*account.AccountKey)
	}
	return fmt.Sprintf("%s %s  %s%s  [%s]", marker, account.ProviderID, key, label, account.Type)
}

// authProviderID resolves a provider reference the way upstream findMatchingProviders does: the
// first composed provider whose id or display name matches exactly, ignoring case and surrounding
// whitespace; otherwise a stored provider id with that exact spelling.
func authProviderID(runtime *codingagent.RequestAuthRuntime, stored []ai.CredentialInfo, providerRef string) (string, bool) {
	normalized := strings.ToLower(widthx.JSTrim(providerRef))
	if normalized == "" {
		return "", false
	}
	if runtime != nil {
		for _, provider := range runtime.GetProviders() {
			if strings.ToLower(provider.ID) == normalized || strings.ToLower(provider.Name) == normalized {
				return provider.ID, true
			}
		}
	}
	for _, entry := range stored {
		if entry.ProviderID == providerRef {
			return entry.ProviderID, true
		}
	}
	return "", false
}

// authAccountStore returns the multi-account capability of the store, reporting false for a store
// that holds one credential per provider.
func authAccountStore(store ai.CredentialStore) (ai.AccountCredentialStore, bool) {
	if wrapper, ok := store.(interface {
		AccountStore() (ai.AccountCredentialStore, bool)
	}); ok {
		return wrapper.AccountStore()
	}
	accounts, ok := store.(ai.AccountCredentialStore)
	return accounts, ok && accounts != nil
}

// openAuthCommandRuntime opens the credential store and a request-auth runtime over it, without
// network access, for the account commands.
func openAuthCommandRuntime(ctx context.Context, agentDir string) (*codingagent.RequestAuthRuntime, ai.CredentialStore, error) {
	store, err := ai.OpenCredentialStore(agentDir)
	if err != nil {
		return nil, nil, err
	}
	runtime, err := codingagent.NewRequestAuthRuntime(ctx, codingagent.RequestAuthRuntimeOptions{
		Credentials: store,
		AgentDir:    agentDir,
		ModelsStore: ai.NewInMemoryModelsStore(),
	})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	return runtime, store, nil
}

// runAuthAccounts implements `auth accounts`. It returns the process exit code: 1 when --provider
// names an unknown provider or the arguments are malformed.
func runAuthAccounts(args []string, env authRunEnv) int {
	var (
		providerRef string
		jsonOutput  bool
	)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--json":
			jsonOutput = true
		case arg == "--provider":
			index++
			if index >= len(args) || args[index] == "" {
				env.printError("Usage: " + authAccountsUsage())
				return 1
			}
			providerRef = args[index]
		default:
			env.printError("Usage: " + authAccountsUsage())
			return 1
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), authAccountsTimeout)
	defer cancel()
	runtime, store, err := openAuthCommandRuntime(ctx, env.agentDir)
	if err != nil {
		env.printError(fmt.Sprintf("Could not read stored credentials: %v", err))
		return 1
	}
	// The command owns the store it opened; the request-auth runtime does not close supplied storage.
	if closer, ok := store.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}

	accounts, _ := listCredentialAccounts(ctx, store)
	if len(accounts) == 0 {
		stored, _ := store.List(ctx)
		if len(stored) == 0 {
			fmt.Fprintln(env.stdout, "No stored accounts.")
		} else {
			fmt.Fprintln(env.stdout, "This credential store does not support multiple accounts; use `auth check` for stored providers.")
		}
		return 0
	}
	filtered := accounts
	if providerRef != "" {
		stored, _ := store.List(ctx)
		providerID, ok := authProviderID(runtime, stored, providerRef)
		if !ok {
			env.printError("Unknown provider \"" + widthx.ReplaceControlCharacters(providerRef) + "\".")
			return 1
		}
		filtered = make([]ai.CredentialAccountInfo, 0, len(accounts))
		for _, account := range accounts {
			if account.ProviderID == providerID {
				filtered = append(filtered, account)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(env.stdout, "No stored accounts for %s.\n", widthx.ReplaceControlCharacters(providerRef))
			return 0
		}
	}

	if jsonOutput {
		// JSON.stringify(filtered, null, 2): two-space indentation, no HTML escaping.
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(filtered); err != nil {
			env.printError(fmt.Sprintf("Could not encode accounts: %v", err))
			return 1
		}
		fmt.Fprint(env.stdout, buffer.String())
		return 0
	}
	for _, account := range filtered {
		fmt.Fprintln(env.stdout, authAccountRow(account))
	}
	return 0
}

// listCredentialAccounts lists stored accounts, tolerating a store without the capability.
func listCredentialAccounts(ctx context.Context, store ai.CredentialStore) ([]ai.CredentialAccountInfo, bool) {
	accounts, ok := authAccountStore(store)
	if !ok {
		return nil, false
	}
	listed, err := accounts.ListAccounts(ctx)
	if err != nil {
		return nil, true
	}
	return listed, true
}

// runAuthUse implements `auth use <provider> <account>`.
func runAuthUse(args []string, env authRunEnv) int {
	if len(args) < 2 || len(args) > 2 || args[0] == "" || args[1] == "" {
		env.printError("Usage: " + authUseUsage())
		return 1
	}
	providerRef, accountKey := args[0], args[1]

	ctx, cancel := context.WithTimeout(context.Background(), authAccountsTimeout)
	defer cancel()
	runtime, store, err := openAuthCommandRuntime(ctx, env.agentDir)
	if err != nil {
		env.printError(fmt.Sprintf("Could not read stored credentials: %v", err))
		return 1
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}

	stored, _ := store.List(ctx)
	providerID, ok := authProviderID(runtime, stored, providerRef)
	if !ok {
		env.printError("Unknown provider \"" + widthx.ReplaceControlCharacters(providerRef) + "\".")
		return 1
	}

	accounts, accountsOK := authAccountStore(store)
	selected := false
	if accountsOK {
		selected, err = accounts.SelectAccount(ctx, providerID, accountKey)
		if err != nil {
			env.printError(fmt.Sprintf("Could not select account: %v", err))
			return 1
		}
	}
	if !selected {
		env.printError("No stored account \"" + widthx.ReplaceControlCharacters(accountKey) + "\" for " + providerID + ".")
		if accountsOK {
			if listed, listErr := accounts.ListAccounts(ctx); listErr == nil {
				available := storedAccountKeys(listed, providerID)
				if len(available) > 0 {
					env.printError("Stored accounts: " + strings.Join(available, ", "))
				}
			}
		}
		return 1
	}
	fmt.Fprintf(env.stdout, "Now using account \"%s\" for %s.\n", widthx.ReplaceControlCharacters(accountKey), providerID)
	return 0
}

// runAuthLogout implements `auth logout <provider> [--account <key>]`, the single logout path for
// both `auth logout` and the top-level `logout` command. Environment variables and models.yml
// configuration stay, as with /logout.
func runAuthLogout(args []string, env authRunEnv) int {
	var (
		accountKey    string
		accountKeySet bool
		positional    []string
	)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--account" {
			index++
			if index >= len(args) || args[index] == "" {
				env.printError("Usage: " + authLogoutUsage())
				return 1
			}
			accountKey, accountKeySet = args[index], true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			env.printError("Usage: " + authLogoutUsage())
			return 1
		}
		positional = append(positional, arg)
	}
	if len(positional) != 1 || positional[0] == "" {
		env.printError("Usage: " + authLogoutUsage())
		return 1
	}
	providerRef := positional[0]

	ctx, cancel := context.WithTimeout(context.Background(), authAccountsTimeout)
	defer cancel()
	runtime, store, err := openAuthCommandRuntime(ctx, env.agentDir)
	if err != nil {
		env.printError(fmt.Sprintf("Could not read stored credentials: %v", err))
		return 1
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}

	stored, listErr := store.List(ctx)
	if listErr != nil {
		env.printError(fmt.Sprintf("Could not read stored credentials: %v", listErr))
		return 1
	}
	providerID, known := authProviderID(runtime, stored, providerRef)
	var credential *ai.CredentialInfo
	for index := range stored {
		if stored[index].ProviderID == providerID {
			credential = &stored[index]
			break
		}
	}
	if !known && credential == nil {
		env.printError("Unknown provider \"" + widthx.ReplaceControlCharacters(providerRef) + "\".")
		env.printError("Stored credentials: " + strings.Join(sortedProviderIDs(stored), ", "))
		return 1
	}
	providerName := providerRef
	if runtime != nil {
		if provider := runtime.GetProvider(providerID); provider != nil && provider.Name != "" {
			providerName = provider.Name
		}
	}
	if credential == nil {
		fmt.Fprintf(env.stdout, "No stored credentials for %s. Environment variables and models.yml config are unchanged.\n", providerName)
		return 0
	}

	if accountKeySet {
		return removeOneAccount(ctx, env, store, credential.ProviderID, providerName, accountKey)
	}

	if err := store.Delete(ctx, credential.ProviderID); err != nil {
		env.printError(fmt.Sprintf("Logout failed: %v", err))
		return 1
	}
	if credential.Type == ai.CredentialOAuth {
		fmt.Fprintf(env.stdout, "Logged out of %s.\n", providerName)
	} else {
		fmt.Fprintf(env.stdout, "Removed stored API key for %s. Environment variables and models.yml config are unchanged.\n", providerName)
	}
	return 0
}

// removeOneAccount removes one stored account, keeping the provider's others. It falls back with an
// explicit message when the store has no multi-account support, so the user is not told "no such
// account" on a store whose only account is the provider itself.
func removeOneAccount(ctx context.Context, env authRunEnv, store ai.CredentialStore, providerID, providerName, accountKey string) int {
	accounts, ok := authAccountStore(store)
	if !ok {
		env.printError(fmt.Sprintf("This credential store does not support multiple accounts; use %q.", codingagent.AppName+" auth logout "+providerID))
		return 1
	}
	listed, err := accounts.ListAccounts(ctx)
	if err != nil {
		env.printError(fmt.Sprintf("Could not read stored credentials: %v", err))
		return 1
	}
	if len(listed) == 0 {
		env.printError(fmt.Sprintf("This credential store does not support multiple accounts; use %q.", codingagent.AppName+" auth logout "+providerID))
		return 1
	}
	removed, err := accounts.DeleteAccount(ctx, providerID, accountKey)
	if err != nil {
		env.printError(fmt.Sprintf("Logout failed: %v", err))
		return 1
	}
	if !removed {
		env.printError("No stored account \"" + widthx.ReplaceControlCharacters(accountKey) + "\" for " + providerName + ".")
		available := storedAccountKeys(listed, providerID)
		if len(available) > 0 {
			env.printError("Stored accounts: " + strings.Join(available, ", "))
		}
		return 1
	}
	remaining := 0
	for _, account := range listed {
		if account.ProviderID == providerID && (account.AccountKey == nil || *account.AccountKey != accountKey) {
			remaining++
		}
	}
	if remaining > 0 {
		fmt.Fprintf(env.stdout, "Removed account \"%s\" for %s. %d remaining.\n", widthx.ReplaceControlCharacters(accountKey), providerName, remaining)
	} else {
		fmt.Fprintf(env.stdout, "Removed account \"%s\" for %s. No accounts remain for it.\n", widthx.ReplaceControlCharacters(accountKey), providerName)
	}
	return 0
}

// storedAccountKeys lists one provider's account keys in store order for the
// use/logout diagnostics. A nil key keeps its distinct identity-less label.
func storedAccountKeys(accounts []ai.CredentialAccountInfo, providerID string) []string {
	keys := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account.ProviderID != providerID {
			continue
		}
		key := "<no identity>"
		if account.AccountKey != nil {
			key = *account.AccountKey
		}
		keys = append(keys, key)
	}
	return keys
}

func sortedProviderIDs(infos []ai.CredentialInfo) []string {
	ids := make([]string, 0, len(infos))
	seen := map[string]bool{}
	for _, info := range infos {
		if seen[info.ProviderID] {
			continue
		}
		seen[info.ProviderID] = true
		ids = append(ids, info.ProviderID)
	}
	if len(ids) == 0 {
		return []string{"none"}
	}
	sort.Strings(ids)
	return ids
}
