package cli

// Ports the personal fork's cli/auth-login.ts. Unlike the stock top-level
// `login` command, `auth login` uses the composed provider's OAuth or API-key
// setup and supports automatic browser/alternative-method selection.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding"
	"github.com/MichaelKinsy/PiG/internal/codingagent"
	"github.com/MichaelKinsy/PiG/internal/nodeurl"
	"github.com/MichaelKinsy/PiG/tui/widthx"
	"golang.org/x/term"
)

type authLoginRequest struct {
	provider          string
	apiKey, noBrowser bool
}

func authLoginUsage() string {
	return codingagent.AppName + " auth login <provider> [--api-key] [--no-browser]"
}

func parseAuthLoginRequest(args []string) (authLoginRequest, error) {
	var request authLoginRequest
	for _, arg := range args {
		switch arg {
		case "--api-key":
			request.apiKey = true
		case "--no-browser":
			request.noBrowser = true
		default:
			if strings.HasPrefix(arg, "-") || request.provider != "" {
				return request, fmt.Errorf("Usage: %s", authLoginUsage())
			}
			request.provider = arg
		}
	}
	if request.provider == "" {
		return request, fmt.Errorf("Usage: %s", authLoginUsage())
	}
	if request.apiKey && request.noBrowser {
		return request, errors.New("--no-browser only applies to account sign-in; omit it with --api-key.")
	}
	return request, nil
}

func selectAuthLoginType(provider *ai.ModelsProvider, apiKey bool) (ai.AuthType, error) {
	if !apiKey && provider.Auth.OAuth != nil {
		return ai.CredentialOAuth, nil
	}
	if provider.Auth.APIKey != nil && provider.Auth.APIKey.Login != nil {
		return ai.CredentialAPIKey, nil
	}
	if apiKey && provider.Auth.OAuth != nil {
		return "", fmt.Errorf("%s has no API key login. Omit --api-key to sign in.", provider.Name)
	}
	method := "ambient credentials"
	if provider.Auth.APIKey != nil {
		method = provider.Auth.APIKey.Name
	}
	return "", fmt.Errorf("%s is configured outside %s (%s).", provider.Name, codingagent.AppName, method)
}

func runAuthLogin(args []string, env authRunEnv) int {
	request, err := parseAuthLoginRequest(args)
	if err != nil {
		env.printError(err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	store, err := ai.OpenCredentialStore(env.agentDir)
	if err != nil {
		env.printError(err.Error())
		return 1
	}
	defer func() { _ = store.Close() }()
	path := codingagent.ModelsPath(env.agentDir)
	runtime, err := coding.CreateModelRuntime(ctx, coding.CreateModelRuntimeOptions{Credentials: store, ModelsPath: new(&path), RefreshOnCreate: new(false)})
	if err != nil {
		env.printError(err.Error())
		return 1
	}
	defer runtime.Close()
	var provider *ai.ModelsProvider
	providers := runtime.GetProviders()
	normalized := strings.ToLower(strings.TrimSpace(request.provider))
	for _, candidate := range providers {
		if strings.ToLower(candidate.ID) == normalized || strings.ToLower(candidate.Name) == normalized {
			provider = candidate
			break
		}
	}
	if provider == nil {
		names := make([]string, 0, len(providers))
		for _, candidate := range providers {
			if candidate.Auth.OAuth != nil || (candidate.Auth.APIKey != nil && candidate.Auth.APIKey.Login != nil) {
				names = append(names, candidate.ID)
			}
		}
		slices.Sort(names)
		env.printError(fmt.Sprintf("Unknown provider %q.", widthx.ReplaceControlCharacters(request.provider)))
		env.printError("Providers: " + strings.Join(names, ", "))
		return 1
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		env.printError(fmt.Sprintf("%q requires an interactive terminal", authLoginUsage()))
		return 1
	}
	kind, err := selectAuthLoginType(provider, request.apiKey)
	if err != nil {
		env.printError(err.Error())
		return 1
	}
	terminal := newAuthLoginTerminal(env.stdout)
	cwd, _ := os.Getwd()
	settings := codingagent.NewSettingsManager(cwd, env.agentDir)
	_, err = runtime.Login(ctx, provider.ID, kind, authLoginInteraction(request, kind, terminal), ai.LoginOptions{GetDeviceID: settings.GetOrCreateDeviceID})
	if err != nil {
		var synchronization *coding.CredentialSynchronizationError
		switch {
		case errors.As(err, &synchronization):
			env.printError(fmt.Sprintf("Logged in to %s, but local model state could not be synchronized: %v", provider.Name, err))
		case ctx.Err() != nil || err.Error() == "Login cancelled":
			env.printError("Login cancelled.")
		default:
			env.printError(fmt.Sprintf("Failed to log in to %s: %v", provider.Name, err))
		}
		return 1
	}
	verb := "Logged in to "
	if kind == ai.CredentialAPIKey {
		verb = "Saved API key for "
	}
	terminal.write(verb + provider.Name + ". Credentials saved.")
	refreshCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := runtime.Refresh(refreshCtx, ai.ModelsRefreshOptions{Providers: []string{provider.ID}, AllowNetwork: new(true)})
	switch {
	case ctx.Err() != nil:
		terminal.write(fmt.Sprintf("Skipped the %s model catalog refresh; %s refreshes it on next start.", provider.Name, codingagent.AppName))
	case result.Aborted:
		terminal.write(fmt.Sprintf("The %s model catalog refresh timed out; %s refreshes it on next start.", provider.Name, codingagent.AppName))
	case len(result.Errors) > 0:
		terminal.write(fmt.Sprintf("The %s model catalog could not be refreshed; %s retries on next start.", provider.Name, codingagent.AppName))
	}
	if provider.ID == "radius" {
		configureAuthRadiusMCP(provider, env, terminal)
	}
	return 0
}

func authLoginInteraction(request authLoginRequest, kind ai.AuthType, terminal *authLoginTerminal) ai.AuthInteraction {
	var skipped []string
	return ai.AuthInteraction{
		Prompt: func(ctx context.Context, prompt ai.AuthPrompt) (string, error) {
			switch p := prompt.(type) {
			case ai.AuthSelectPrompt:
				options := p.Options
				if kind == ai.CredentialOAuth && len(options) > 0 {
					if !request.noBrowser {
						skipped = skipped[:0]
						for _, option := range options[1:] {
							skipped = append(skipped, option.Label)
						}
						return options[0].ID, nil
					}
					if len(options) == 1 {
						return options[0].ID, nil
					}
					options = options[1:]
					if len(options) == 1 {
						terminal.write(p.Message + " " + options[0].Label)
						return options[0].ID, nil
					}
				}
				terminal.write(p.Message)
				for index, option := range options {
					label := fmt.Sprintf("  %d. %s", index+1, option.Label)
					if option.Description != "" {
						label += " - " + option.Description
					}
					terminal.write(label)
				}
				for {
					answer, err := terminal.question(ctx, fmt.Sprintf("Select 1-%d: ", len(options)), false)
					if err != nil {
						return "", err
					}
					index, err := strconv.Atoi(strings.TrimSpace(answer))
					if err == nil && index >= 1 && index <= len(options) {
						return options[index-1].ID, nil
					}
					terminal.write(fmt.Sprintf("Enter a number from 1 to %d.", len(options)))
				}
			case ai.AuthTextPrompt:
				return terminal.question(ctx, authLoginPrompt(p.Message, p.Placeholder), false)
			case ai.AuthSecretPrompt:
				return terminal.question(ctx, authLoginPrompt(p.Message, p.Placeholder), true)
			case ai.AuthManualCodePrompt:
				return terminal.question(ctx, authLoginPrompt(p.Message, p.Placeholder), false)
			default:
				return "", fmt.Errorf("unsupported authentication prompt %T", prompt)
			}
		},
		Notify: func(event ai.AuthEvent) {
			switch event := event.(type) {
			case ai.AuthURLEvent:
				terminal.write("Open this URL to sign in:\n  " + authLoginDisplayURL(event.URL))
				if event.Instructions != "" {
					terminal.write(event.Instructions)
				}
				if !request.noBrowser {
					openAuthLoginURL(event.URL)
				}
				rerun := fmt.Sprintf("\"%s auth login %s --no-browser\"", codingagent.AppName, request.provider)
				if len(skipped) == 1 {
					terminal.write(fmt.Sprintf("To use %q instead, run %s", skipped[0], rerun))
				} else if len(skipped) > 1 {
					quoted := make([]string, len(skipped))
					for i, label := range skipped {
						quoted[i] = strconv.Quote(label)
					}
					terminal.write("Other sign-in methods: " + strings.Join(quoted, ", ") + ". Run " + rerun + " to choose one.")
				}
			case ai.AuthDeviceCodeEvent:
				terminal.write("Open " + authLoginDisplayURL(event.VerificationURI) + " and enter code: " + event.UserCode)
				if !request.noBrowser && (event.OpenBrowserOnConfirm == nil || *event.OpenBrowserOnConfirm) {
					openAuthLoginURL(event.VerificationURI)
				}
				terminal.write("Waiting for authentication...")
			case ai.AuthInfoEvent:
				terminal.write(event.Message)
				for _, link := range event.Links {
					label := ""
					if link.Label != "" {
						label = link.Label + ": "
					}
					terminal.write("  " + label + authLoginDisplayURL(link.URL))
				}
			case ai.AuthProgressEvent:
				terminal.write(event.Message)
			}
		},
	}
}

func authLoginPrompt(message, placeholder string) string {
	if placeholder != "" {
		message += " (" + placeholder + ")"
	}
	return message + ": "
}
func authLoginDisplayURL(raw string) string {
	parsed, err := nodeurl.ParseHTTPHref(raw)
	if err == nil {
		return parsed.Href
	}
	return widthx.ReplaceControlCharacters(raw)
}
func openAuthLoginURL(raw string) {
	if parsed, err := nodeurl.ParseHTTPHref(raw); err == nil {
		codingagent.OpenBrowser(parsed.Href)
	}
}

type authLoginTerminal struct {
	output   io.Writer
	terminal *term.Terminal
	input    *authLoginInput
}
type authLoginInput struct {
	ctx    context.Context
	output io.Writer
}

func (input *authLoginInput) Read(buffer []byte) (int, error) {
	return readAuthLoginInput(input.ctx, buffer)
}
func (input *authLoginInput) Write(buffer []byte) (int, error) { return input.output.Write(buffer) }
func newAuthLoginTerminal(output io.Writer) *authLoginTerminal {
	input := &authLoginInput{output: output}
	return &authLoginTerminal{output: output, input: input, terminal: term.NewTerminal(input, "")}
}
func (terminal *authLoginTerminal) write(message string) {
	_, _ = fmt.Fprintln(terminal.output, message)
}
func (terminal *authLoginTerminal) question(ctx context.Context, prompt string, secret bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", context.Cause(ctx)
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), state) }()
	terminal.input.ctx = ctx
	var answer string
	if secret {
		answer, err = terminal.terminal.ReadPassword(prompt)
	} else {
		terminal.terminal.SetPrompt(prompt)
		answer, err = terminal.terminal.ReadLine()
	}
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if errors.Is(err, io.EOF) {
		return "", errors.New("Login cancelled")
	}
	return answer, err
}
