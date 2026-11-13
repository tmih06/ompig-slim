package cli

import (
	"context"
	"fmt"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// authAutoResetUsage returns the account preference command syntax.
func authAutoResetUsage() string {
	return codingagent.AppName + " auth auto-reset <provider> <account> <on|off>"
}

// runAuthAutoReset changes the automatic saved-reset preference of one exact
// stored account without changing provider selection or any secret.
func runAuthAutoReset(args []string, env authRunEnv) int {
	if len(args) != 3 || args[0] == "" || args[1] == "" || (args[2] != "on" && args[2] != "off") {
		env.printError("Usage: " + authAutoResetUsage())
		return 1
	}
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
	providerID, ok := authProviderID(runtime, stored, args[0])
	if !ok {
		env.printError("Unknown provider \"" + widthx.ReplaceControlCharacters(args[0]) + "\".")
		return 1
	}
	accounts, ok := authAccountStore(store)
	if !ok {
		env.printError("This credential store does not support multiple accounts.")
		return 1
	}
	enabled := args[2] == "on"
	_, err = accounts.ModifyAccount(ctx, providerID, new(args[1]), func(credential *ai.Credential) (*ai.Credential, error) {
		if credential == nil {
			return nil, fmt.Errorf("account is not stored")
		}
		ai.SetCredentialAutoReset(credential, enabled)
		return credential, nil
	})
	if err != nil {
		env.printError("Could not update account \"" + widthx.ReplaceControlCharacters(args[1]) + "\": " + err.Error())
		return 1
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Fprintf(env.stdout, "Automatic saved-reset redemption %s for %s/%s.\n", state, providerID, widthx.ReplaceControlCharacters(args[1]))
	return 0
}
