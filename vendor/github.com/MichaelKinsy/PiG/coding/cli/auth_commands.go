package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/jsnumber"
)

type loginCommand string

const (
	authLogin  loginCommand = "login"
	authLogout loginCommand = "logout"
)

type loginCLIOptions struct {
	command  loginCommand
	provider string
	help     bool
	list     bool
	json     bool
	noInput  bool
	// accountKey is --account <key> for a single-account logout.
	accountKey    string
	accountKeySet bool
	invalid       string
}

func parseLoginCommand(args []string) (*loginCLIOptions, bool) {
	if len(args) == 0 {
		return nil, false
	}
	switch args[0] {
	case "login", "logout":
	default:
		return nil, false
	}
	opts := &loginCLIOptions{command: loginCommand(args[0])}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "-h", "--help":
			opts.help = true
		case "--list":
			opts.list = true
		case "--json":
			opts.json = true
		case "--no-input":
			opts.noInput = true
		case "--account":
			index++
			if index >= len(args) || args[index] == "" || strings.HasPrefix(args[index], "-") {
				opts.invalid = "--account"
				continue
			}
			opts.accountKey, opts.accountKeySet = args[index], true
		default:
			if strings.HasPrefix(arg, "-") || opts.provider != "" {
				opts.invalid = arg
				continue
			}
			opts.provider = arg
		}
	}
	return opts, true
}

func runLoginCommand(args []string) int {
	opts, ok := parseLoginCommand(args)
	if !ok {
		return -1
	}
	if opts.invalid != "" || (opts.list && opts.command != authLogin) || (opts.list && opts.provider != "") ||
		(opts.accountKeySet && (opts.command != authLogout || opts.list)) {
		fmt.Fprintf(os.Stderr, "Usage: %s %s [provider] [--list] [--json] [--no-input]\n", codingagent.AppName, opts.command)
		if opts.command == authLogout {
			fmt.Fprintf(os.Stderr, "       %s\n", authLogoutUsage())
		}
		return 2
	}
	if opts.help {
		printLoginCommandHelp(opts.command)
		return 0
	}
	var contributions *authContributionRegistry
	defer func() { closeAuthContributionRegistry(contributions) }()
	if opts.command == authLogin && opts.list {
		var err error
		contributions, err = discoverAuthContributions()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ompig-slim login --list:", err)
			return 1
		}
		return listAuthTargets(opts.json, contributions)
	}
	if opts.noInput && opts.provider == "" {
		fmt.Fprintf(os.Stderr, "ompig-slim %s: --no-input requires an explicit provider; use `ompig-slim login --list --json` to discover targets\n", opts.command)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		printCLIError("%v", err)
		return 1
	}
	agentDir := codingagent.AgentDir()

	switch opts.command {
	case authLogin:
		provider, err := resolveLoginProvider(opts.provider, &contributions)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		// Pi passes the installation's device ID to every login (interactive-mode.ts:6297).
		settings := codingagent.NewSettingsManager(cwd, agentDir)
		cred, err := runOAuthProviderLogin(provider, opts.noInput, settings.GetOrCreateDeviceID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		// Extension-owned credentials report their own path below.
		credentialPath := ai.SQLiteAuthPath(agentDir)
		// pig additive (D40): extension-owned credentials do not open core storage; failed logins do not create it.
		if credentialStore, ok := provider.(ai.OAuthCredentialStore); ok {
			credentialPath, err = credentialStore.StoreOAuthCredentials(cred)
		} else {
			store, openErr := ai.OpenCredentialStore(agentDir)
			if openErr != nil {
				printCLIError("%v", openErr)
				return 1
			}
			defer func() { _ = store.Close() }()
			var credential ai.Credential
			if credential, err = ai.CredentialFromOAuth(cred); err == nil {
				// As interactive login does, store through the cancellable lock, which waits for another process's lock like Pi's credentials.modify.
				_, err = store.Modify(context.Background(), provider.ID(), func(*ai.Credential) (*ai.Credential, error) { return &credential, nil })
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		fmt.Printf("\nCredentials saved to %s\n", credentialPath)
		return 0
	case authLogout:
		providerID := opts.provider
		if providerID == "" {
			contributions, err = discoverAuthContributions()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return 1
			}
			if err := contributions.loadAll(); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return 1
			}
			providers, err := logoutProviders(agentDir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return 1
			}
			if len(providers) == 0 {
				fmt.Fprintln(os.Stderr, "No OAuth providers logged in.")
				return 1
			}
			if len(providers) == 1 {
				providerID = providers[0]
			} else {
				fmt.Println("Select a provider to logout:")
				fmt.Println()
				for i, provider := range providers {
					fmt.Printf("  %d. %s\n", i+1, provider)
				}
				fmt.Println()
				fmt.Printf("Enter number (1-%d): ", len(providers))
				line, err := readLine(os.Stdin)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Error:", err)
					return 1
				}
				idx, ok := menuChoice(line, len(providers))
				if !ok {
					fmt.Fprintln(os.Stderr, "Error: Invalid selection")
					return 1
				}
				providerID = providers[idx-1]
			}
		} else if err := loadDeclaredAuthProvider(opts.provider, &contributions); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		// One logout behavior for `ompig-slim auth logout` and `ompig-slim logout`: the shared handler reports
		// extension-owned removals and the core store's result with Pi's wording.
		logoutArgs := []string{providerID}
		if opts.accountKeySet {
			logoutArgs = append(logoutArgs, "--account", opts.accountKey)
		}
		code := runAuthLogout(logoutArgs, authRunEnv{stdout: os.Stdout, stderr: os.Stderr, agentDir: agentDir, color: chalkStdoutColor()})
		if code == 0 {
			if provider, ok := ai.GetOAuthProvider(providerID); ok {
				if credentialStore, ok := provider.(ai.OAuthCredentialStore); ok {
					if removed, err := credentialStore.DeleteOAuthCredentials(); err != nil {
						fmt.Fprintln(os.Stderr, "Error:", err)
						return 1
					} else if removed {
						fmt.Printf("Credentials removed for %s from the extension store.\n", providerID)
					}
				}
			}
		}
		return code
	default:
		return -1
	}
}

type authTargetListOutput struct {
	Targets     []authTargetItem           `json:"targets"`
	Diagnostics []authInspectionDiagnostic `json:"diagnostics,omitempty"`
}

type authTargetItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CallbackServer bool   `json:"callbackServer"`
}

// pig additive (D40): Pig auth targets are discoverable through a generic,
// current pre-session inventory shared with contributed providers.
func listAuthTargets(jsonMode bool, registry *authContributionRegistry) int {
	targets := registeredAuthTargetItems(registry)
	if jsonMode {
		output := authTargetListOutput{Targets: targets}
		if registry != nil {
			output.Diagnostics = slices.Clone(registry.diagnostics)
		}
		data, err := json.Marshal(output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ompig-slim login --list: encode JSON:", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}
	if registry != nil {
		for _, diagnostic := range registry.diagnostics {
			fmt.Fprintf(os.Stderr, "ompig-slim login --list: extension %q inspection failed: %s\n", diagnostic.Extension, diagnostic.Error)
		}
	}
	if len(targets) == 0 {
		fmt.Println("No authentication targets registered.")
		return 0
	}
	fmt.Println("Authentication targets:")
	for _, target := range targets {
		fmt.Printf("  %s\t%s\n", target.ID, target.Name)
	}
	return 0
}

func printLoginCommandHelp(cmd loginCommand) {
	switch cmd {
	case authLogin:
		fmt.Print("Usage:\n  ompig-slim login [provider] [--no-input]\n  ompig-slim login --list [--json]\n\nLogin to an OAuth provider or list registered auth targets.\n\nExamples:\n  ompig-slim login\n  ompig-slim login github-copilot\n  ompig-slim login --list --json\n")
	case authLogout:
		fmt.Print("Usage:\n  ompig-slim logout [provider] [--account <key>] [--no-input]\n\nRemove stored credentials from the shared agent.db.\n\nExamples:\n  ompig-slim logout\n  ompig-slim logout github-copilot\n")
	}
}

func resolveLoginProvider(input string, registry **authContributionRegistry) (ai.OAuthProviderInterface, error) {
	if input != "" {
		return authProviderFromContribution(input, registry)
	}
	if *registry == nil {
		discovered, err := discoverAuthContributions()
		if err != nil {
			return nil, err
		}
		*registry = discovered
	}
	targets := registeredAuthTargetItems(*registry)
	fmt.Println("Select a provider:")
	fmt.Println()
	for i, target := range targets {
		fmt.Printf("  %d. %s\n", i+1, target.Name)
	}
	fmt.Println()
	fmt.Printf("Enter number (1-%d): ", len(targets))
	line, err := readLine(os.Stdin)
	if err != nil {
		return nil, err
	}
	idx, ok := menuChoice(line, len(targets))
	if !ok {
		return nil, fmt.Errorf("Invalid selection")
	}
	return authProviderFromContribution(targets[idx-1].ID, registry)
}

// logoutProviders lists providers with stored OAuth credentials, matching Pi's interactive
// selection: the store's providers first, then extension-owned OAuth stores, sorted by id.
func logoutProviders(agentDir string) ([]string, error) {
	store, err := ai.OpenCredentialStore(agentDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()
	//portlint:allow ctxbackground the logout picker has no caller context
	stored, err := store.List(context.Background())
	if err != nil {
		return nil, err
	}
	var providers []string
	providersByID := make(map[string]struct{})
	for _, info := range stored {
		if info.Type != ai.CredentialOAuth {
			continue
		}
		providers = append(providers, info.ProviderID)
		providersByID[info.ProviderID] = struct{}{}
	}
	for _, provider := range ai.GetOAuthProviders() {
		if _, exists := providersByID[provider.ID()]; exists {
			continue
		}
		credentialStore, ok := provider.(ai.OAuthCredentialStore)
		if !ok {
			continue
		}
		if _, present := credentialStore.OAuthCredentialStatus(); present {
			providers = append(providers, provider.ID())
			providersByID[provider.ID()] = struct{}{}
		}
	}
	slices.Sort(providers)
	return providers, nil
}

func runOAuthProviderLogin(provider ai.OAuthProviderInterface, noInput bool, getDeviceID func() string) (ai.OAuthCredentials, error) {
	callbacks := ai.OAuthLoginCallbacks{
		GetDeviceID: getDeviceID,
		OnAuth: func(info ai.OAuthAuthInfo) {
			fmt.Printf("\nOpen this URL in your browser:\n%s\n", info.URL)
			if info.Instructions != "" {
				fmt.Println(info.Instructions)
			}
		},
		OnPrompt: func(prompt ai.OAuthPrompt) (string, error) {
			if noInput {
				return "", fmt.Errorf("provider %s requires interactive input; rerun without --no-input", provider.ID())
			}
			question := prompt.Message
			if prompt.AllowEmpty && provider.ID() == "github-copilot" {
				question += " (blank for github.com)"
			}
			if prompt.Placeholder != "" {
				question += fmt.Sprintf(" (%s)", prompt.Placeholder)
			}
			question += ": "
			fmt.Print(question)
			return readLine(os.Stdin)
		},
		OnManualCodeInput: func() (string, error) {
			if noInput {
				return "", fmt.Errorf("provider %s requires a callback code; rerun without --no-input", provider.ID())
			}
			fmt.Print("Enter code or callback URL: ")
			return readLine(os.Stdin)
		},
		OnProgress: func(message string) {
			fmt.Println(message)
		},
		OnDeviceCode: func(info ai.OAuthDeviceCodeInfo) {
			fmt.Printf("\nOpen this URL in your browser:\n%s\n", info.VerificationURI)
			fmt.Printf("Enter code: %s\n", info.UserCode)
		},
		OnSelect: func(prompt ai.OAuthSelectPrompt) (string, error) {
			if noInput {
				return "", fmt.Errorf("provider %s requires a selection; rerun without --no-input", provider.ID())
			}
			fmt.Printf("\n%s\n", prompt.Message)
			for i, opt := range prompt.Options {
				fmt.Printf("  %d. %s\n", i+1, opt.Label)
			}
			fmt.Printf("Enter number (1-%d): ", len(prompt.Options))
			line, err := readLine(os.Stdin)
			if err != nil {
				return "", err
			}
			// cli.ts answerPrompt: Number.parseInt(answer, 10) - 1 indexes the options; anything else is "Invalid selection".
			n, ok := menuChoice(line, len(prompt.Options))
			if !ok {
				return "", fmt.Errorf("Invalid selection")
			}
			return prompt.Options[n-1].ID, nil
		},
	}
	if parityHarnessEnabled() {
		return runParityHarnessOAuthLogin(provider, callbacks)
	}
	return provider.Login(callbacks)
}

func sortedOAuthProviders() []ai.OAuthProviderInterface {
	providers := ai.GetOAuthProviders()
	order := map[string]int{
		"anthropic":      0,
		"github-copilot": 1,
		"openai-codex":   2,
	}
	slices.SortFunc(providers, func(a, b ai.OAuthProviderInterface) int {
		aiOrder, aok := order[a.ID()]
		biOrder, bok := order[b.ID()]
		switch {
		case aok && bok && aiOrder != biOrder:
			return aiOrder - biOrder
		case aok && !bok:
			return -1
		case !aok && bok:
			return 1
		}
		return strings.Compare(a.Name(), b.Name())
	})
	return providers
}

// menuChoice is the 1-based choice a numbered menu answer names, as Pi reads it: Number.parseInt(answer, 10) - 1 indexes the list (cli.ts), so
// the leading digits after JavaScript whitespace and an optional sign count and whatever follows them is ignored, and an answer outside 1..count
// (or with no digits) names nothing.
func menuChoice(answer string, count int) (int, bool) {
	n := jsnumber.ParseInt(answer)
	if !(n >= 1 && n <= float64(count)) { // NaN fails both comparisons
		return 0, false
	}
	return int(n), true
}

func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func runParityHarnessOAuthLogin(provider ai.OAuthProviderInterface, callbacks ai.OAuthLoginCallbacks) (ai.OAuthCredentials, error) {
	switch provider.ID() {
	case "github-copilot":
		var polls int
		return withAuthMockClient(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "github.com" && req.URL.Path == "/login/device/code":
				return authJSONResponse(200, `{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","interval":0,"expires_in":60}`), nil
			case req.URL.Host == "github.com" && req.URL.Path == "/login/oauth/access_token":
				polls++
				if polls == 1 {
					return authJSONResponse(200, `{"error":"authorization_pending"}`), nil
				}
				return authJSONResponse(200, `{"access_token":"ghu_refresh"}`), nil
			case req.URL.Host == "api.github.com" && req.URL.Path == "/copilot_internal/v2/token":
				return authJSONResponse(200, `{"token":"tid=x;proxy-ep=proxy.individual.githubcopilot.com;other=y","expires_at":4102444800}`), nil
			case req.URL.Host == "api.individual.githubcopilot.com" && req.URL.Path == "/models":
				return authJSONResponse(200, `{"data":[{"id":"gpt-4o","model_picker_enabled":true,"policy":{"state":"enabled"},"capabilities":{"supports":{"tool_calls":true}}}]}`), nil
			case req.URL.Host == "api.individual.githubcopilot.com" && strings.HasPrefix(req.URL.Path, "/models/") && strings.HasSuffix(req.URL.Path, "/policy"):
				return authJSONResponse(200, `{"ok":true}`), nil
			default:
				return nil, fmt.Errorf("unexpected oauth request: %s %s", req.Method, req.URL.String())
			}
		}, func() (ai.OAuthCredentials, error) {
			return provider.Login(callbacks)
		})
	default:
		return provider.Login(callbacks)
	}
}

type authRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f authRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func withAuthMockClient(handler func(*http.Request) (*http.Response, error), fn func() (ai.OAuthCredentials, error)) (ai.OAuthCredentials, error) {
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: authRoundTripperFunc(handler)}
	defer func() { http.DefaultClient = oldClient }()
	return fn()
}

func authJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func parityHarnessEnabled() bool {
	return os.Getenv("PIG_PARITY_HARNESS") == "1"
}
