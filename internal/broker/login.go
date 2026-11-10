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

// loadConnection prefers an explicit environment URL/token pair; otherwise it
// reads the locally saved login. A token-only override supports token rotation.
func loadConnection(dir string) (connection, error) {
	if endpoint := os.Getenv("OMPIG_BROKER_URL"); endpoint != "" {
		return connection{URL: endpoint, Secret: os.Getenv("OMPIG_BROKER_TOKEN")}, nil
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
	if secret := os.Getenv("OMPIG_BROKER_TOKEN"); secret != "" {
		saved.Secret = secret
	}
	return saved, nil
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
// to avoid replacing a running runtime's credential synchronization callback.
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
	if err = client.request(ctx, http.MethodGet, "/v1/items/settings.json", "", "", nil, &item); err != nil {
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
