package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

const LoginProviderID = "remote-ompig-slim-broker"
const LoginProviderName = "Remote ompig-slim broker"
const connectionFile = "remote-broker.json"

// connection is device-local bootstrap configuration, deliberately excluded
// from SharedFiles and the credential snapshot so the sync secret never syncs.
type connection struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// Environment variables that configure the connection. They exist for
// unattended clients: a container, a CI job, or a machine provisioned by
// configuration management, none of which can answer the login dialog.
const (
	// EnvURL is the broker origin. Setting it selects environment configuration
	// and overrides a saved connection, because an explicit value in the
	// environment is a deliberate choice for this process.
	EnvURL = "OMPIG_BROKER_URL"
	// EnvToken is the shared secret. Its meaning depends on what else is set: with
	// EnvURL it completes an environment connection, alone it rotates the secret of
	// the saved URL, and with EnvTokenFile it is ignored.
	EnvToken = "OMPIG_BROKER_TOKEN"
	// EnvTokenFile names a file holding the secret, for deployments that mount a
	// secret as a file rather than an environment variable. The filename form keeps
	// the value out of `docker inspect` and a process's environment.
	EnvTokenFile = "OMPIG_BROKER_TOKEN_FILE"
)

// loadConnection resolves the connection from the environment, then the saved
// login. Precedence, in order:
//
//  1. EnvURL plus a secret (EnvToken or EnvTokenFile): a complete override for
//     this process, which never borrows the saved secret.
//  2. EnvToken or EnvTokenFile alone: the saved URL with a rotated secret.
//  3. The saved login file.
//
// A file that cannot be read is an error rather than an empty secret: a client
// configured to read a mounted secret must fail loudly when the mount is missing,
// not silently fall back to the saved connection.
func loadConnection(dir string) (connection, error) {
	secret, err := environmentSecret()
	if err != nil {
		return connection{}, err
	}
	if endpoint := os.Getenv(EnvURL); endpoint != "" {
		return connection{URL: endpoint, Secret: secret}, nil
	}
	data, err := readLocal(filepath.Join(dir, connectionFile))
	if err != nil {
		return connection{}, err
	}
	if data == nil {
		return connection{}, nil
	}
	var saved connection
	if err = json.Unmarshal(data, &saved); err != nil {
		return saved, fmt.Errorf("invalid local broker connection file")
	}
	if saved.URL == "" {
		return saved, fmt.Errorf("local broker connection has no URL")
	}
	if secret != "" {
		saved.Secret = secret
	}
	return saved, nil
}

// environmentSecret reads the secret from EnvToken or, when set, EnvTokenFile.
// The file form wins because an operator mounting a secret is rotating it without
// changing the process definition. Surrounding whitespace is trimmed: a secret
// written with a trailing newline is the normal case for a mounted file.
func environmentSecret() (string, error) {
	if path := strings.TrimSpace(os.Getenv(EnvTokenFile)); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read broker secret file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return os.Getenv(EnvToken), nil
}

// newClient validates connection details without fetching or changing shared
// state; both startup and the login verification use the same URL constraints.
func newClient(dir string, saved connection) (*Client, error) {
	if strings.TrimSpace(saved.Secret) == "" {
		return nil, fmt.Errorf("broker secret is required")
	}
	u, err := url.Parse(saved.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("broker URL must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &Client{base: u.Scheme + "://" + u.Host, token: saved.Secret, dir: dir, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Login prompts for a URL and masked secret through the existing login dialog.
// It verifies bearer access without uploading/downloading profile state, then
// atomically saves a private local connection. Cancellation or failed validation
// leaves any previous connection intact. Activation occurs at the next startup
// to avoid replacing a running runtime's credential store discovery.
func Login(ctx context.Context, dir string, interaction ai.AuthInteraction) error {
	endpoint, err := interaction.Prompt(ctx, ai.AuthTextPrompt{Message: "Broker URL", Placeholder: "https://sync.example.com"})
	if err != nil {
		return err
	}
	secret, err := interaction.Prompt(ctx, ai.AuthSecretPrompt{Message: "Broker secret"})
	if err != nil {
		return err
	}
	client, err := newClient(dir, connection{URL: strings.TrimSpace(endpoint), Secret: secret})
	if err != nil {
		return err
	}
	var item Item
	if err = client.request(ctx, http.MethodGet, "/v1/items/settings.json", "", nil, &item); err != nil {
		return fmt.Errorf("could not verify broker connection: %w", err)
	}
	if err = context.Cause(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(connection{URL: client.base, Secret: secret})
	if err != nil {
		return err
	}
	if err = os.MkdirAll(client.dir, 0700); err != nil {
		return err
	}
	return replaceFile(filepath.Join(client.dir, connectionFile), data)
}
