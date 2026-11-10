package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SetupOptions is one non-interactive connection change. Every field is
// optional; the command applies what it is given and leaves the rest as it was.
//
// This is the provisioning path: a container, a CI job, or a machine configured
// by configuration management cannot answer a login dialog, and editing a JSON
// file by hand is easy to get subtly wrong. Every validation the interactive
// login performs happens here too, so a scripted setup cannot save a connection
// that would be rejected at the next startup.
type SetupOptions struct {
	// URL is the broker origin. An empty URL keeps the saved one; a non-empty one
	// replaces it after the same validation the login dialog applies.
	URL string
	// Token is the shared secret. Empty reads the environment (see [EnvToken] and
	// [EnvTokenFile]) so a script can export the secret once instead of repeating
	// it in argv, where every process on the machine can read it.
	Token string
	// TokenFile reads the secret from a file, trimmed. It takes precedence over
	// Token, matching the environment's own rule.
	TokenFile string
	// Stdin reads the secret when neither Token, TokenFile nor the environment
	// supplies one, which keeps the secret out of argv and out of shell history.
	Stdin io.Reader
	// Verify checks the connection against the service before saving. The
	// interactive login always verifies; a script may pass false to provision
	// before the service exists, and the next startup reports the failure.
	Verify bool
	// Force overwrites an existing different connection instead of refusing.
	Force bool
	// Clear removes the saved connection, disconnecting the device. Other fields
	// are ignored when it is set.
	Clear bool
}

// SetupResult reports what Setup did, so a caller prints facts rather than
// guessing at them.
type SetupResult struct {
	// URL is the connection now saved, or empty after Clear.
	URL string
	// Changed reports whether the saved file was written or removed.
	Changed bool
	// Verified reports whether the service answered the verification request.
	Verified bool
}

// Setup writes or removes the device-local broker connection without an
// interactive terminal.
//
// It is the programmatic equivalent of `/login remote-ompig-slim-broker`, with one
// deliberate difference: `Verify` is the caller's choice. The interactive flow
// verifies because a user typing a URL deserves immediate feedback; a provisioning
// script may reasonably write the connection before the broker is reachable, and a
// later startup reports `profile sync inactive:` rather than failing the machine.
//
// The saved file is mode 0600 and carries the secret, so this never prints it.
func Setup(ctx context.Context, dir string, options SetupOptions) (SetupResult, error) {
	path := filepath.Join(dir, connectionFile)
	if options.Clear {
		return clearConnection(path)
	}

	saved, err := readSavedConnection(path)
	if err != nil {
		return SetupResult{}, err
	}
	url := strings.TrimSpace(options.URL)
	if url == "" {
		url = saved.URL
	}
	if url == "" {
		return SetupResult{}, fmt.Errorf("a broker URL is required: pass --url or keep an existing connection")
	}
	secret, err := setupSecret(options)
	if err != nil {
		return SetupResult{}, err
	}
	if secret == "" {
		secret = saved.Secret
	}
	if secret == "" {
		return SetupResult{}, fmt.Errorf("a broker secret is required: pass --token-file, set %s, or pipe the secret on stdin", EnvTokenFile)
	}

	// Validate exactly as the login dialog and every startup do, so a scripted
	// setup cannot save something the next run would reject.
	client, err := newClient(dir, connection{URL: url, Secret: secret})
	if err != nil {
		return SetupResult{}, err
	}
	if saved.URL == client.base && saved.Secret == secret {
		// An identical setup is a no-op: rewriting the file would change its mtime
		// for no reason and make an idempotent provisioning run look like a change.
		return SetupResult{URL: client.base}, nil
	}
	if saved.URL != "" && saved.URL != client.base && !options.Force {
		return SetupResult{}, fmt.Errorf("a different connection is already saved (%s); pass --force to replace it", saved.URL)
	}

	verified := false
	if options.Verify {
		var item Item
		if err := client.request(ctx, "GET", "/v1/items/settings.json", "", "", nil, &item); err != nil {
			return SetupResult{}, fmt.Errorf("could not verify broker connection: %w", err)
		}
		verified = true
	}

	data, err := json.Marshal(connection{URL: client.base, Secret: secret})
	if err != nil {
		return SetupResult{}, err
	}
	if err := os.MkdirAll(client.dir, 0700); err != nil {
		return SetupResult{}, err
	}
	if err := replaceFile(path, data); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{URL: client.base, Changed: true, Verified: verified}, nil
}

// clearConnection removes the saved connection. A missing file is not an error:
// the post-condition a script wants is "no saved connection", not "a file was
// deleted".
func clearConnection(path string) (SetupResult, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return SetupResult{}, err
	}
	return SetupResult{Changed: true}, nil
}

// readSavedConnection reads the saved connection, returning an empty value when
// the file is absent. A corrupt or oversized file is an error: silently treating
// it as absent would overwrite a connection the user still has, and the secret in
// it is not recoverable from anywhere else.
func readSavedConnection(path string) (connection, error) {
	data, err := readLocal(path)
	if err != nil {
		return connection{}, err
	}
	if data == nil {
		return connection{}, nil
	}
	var saved connection
	if err := json.Unmarshal(data, &saved); err != nil {
		return connection{}, fmt.Errorf("invalid local broker connection file")
	}
	return saved, nil
}

// setupSecret resolves the secret from the option, then the environment, then
// stdin. Reading stdin is what lets `--token-file /dev/stdin` or a plain pipe
// supply the secret without it appearing in `ps`.
func setupSecret(options SetupOptions) (string, error) {
	if path := strings.TrimSpace(options.TokenFile); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read broker secret file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if token := strings.TrimSpace(options.Token); token != "" {
		return token, nil
	}
	secret, err := environmentSecret()
	if err != nil {
		return "", err
	}
	if secret != "" {
		return secret, nil
	}
	if options.Stdin != nil {
		data, err := io.ReadAll(io.LimitReader(options.Stdin, 64<<10))
		if err != nil {
			return "", fmt.Errorf("read broker secret from stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

// ShowConnection reports the saved connection without its secret, so a script or
// a `status` command can confirm what a machine is pointed at.
func ShowConnection(dir string) (connection, error) {
	saved, err := readSavedConnection(filepath.Join(dir, connectionFile))
	if err != nil {
		return connection{}, err
	}
	saved.Secret = ""
	return saved, nil
}
