package broker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupEnv clears every environment source so a case only sees what it sets.
// A test that forgot this would read the developer's own connection.
func setupEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{EnvURL, EnvToken, EnvTokenFile} {
		t.Setenv(name, "")
	}
}

// savedConnection reads the connection file a setup wrote.
func savedConnection(t *testing.T, dir string) connection {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, connectionFile))
	if err != nil {
		t.Fatalf("read connection: %v", err)
	}
	var saved connection
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode connection: %v", err)
	}
	return saved
}

// TestSetupProvisionsWithoutATerminal is the point of the command: a script that
// cannot answer the login dialog can still configure a machine.
func TestSetupProvisionsWithoutATerminal(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	result, err := Setup(context.Background(), dir, SetupOptions{URL: "https://sync.example.com", Token: "sync-secret"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !result.Changed || result.URL != "https://sync.example.com" {
		t.Fatalf("result = %+v", result)
	}
	if result.Verified {
		t.Fatal("Setup claimed verification it did not perform")
	}
	saved := savedConnection(t, dir)
	if saved.URL != "https://sync.example.com" || saved.Secret != "sync-secret" {
		t.Fatalf("saved = %+v", saved)
	}
	// The file carries the shared secret, so its mode is part of the contract.
	info, err := os.Stat(filepath.Join(dir, connectionFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("connection file mode = %o, want 600", mode)
	}
}

// TestSetupReadsTheSecretFromEverySource pins the precedence the usage text
// promises, because a provisioning script picks one of these forms and a silent
// reordering would point a client at the wrong secret.
func TestSetupReadsTheSecretFromEverySource(t *testing.T) {
	cases := map[string]struct {
		options SetupOptions
		env     map[string]string
		write   string
		want    string
	}{
		"--token": {
			options: SetupOptions{Token: "from-flag"},
			want:    "from-flag",
		},
		"--token-file": {
			options: SetupOptions{TokenFile: "SECRETFILE"},
			write:   "from-file\n",
			want:    "from-file",
		},
		"OMPIG_BROKER_TOKEN_FILE": {
			env:   map[string]string{EnvTokenFile: "SECRETFILE"},
			write: "from-env-file\n",
			want:  "from-env-file",
		},
		"OMPIG_BROKER_TOKEN": {
			env:  map[string]string{EnvToken: "from-env"},
			want: "from-env",
		},
		"stdin": {
			options: SetupOptions{},
			want:    "from-stdin",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			setupEnv(t)
			dir := t.TempDir()
			secretPath := filepath.Join(dir, "secret")
			if testCase.write != "" {
				if err := os.WriteFile(secretPath, []byte(testCase.write), 0o600); err != nil {
					t.Fatalf("write secret: %v", err)
				}
			}
			for key, value := range testCase.env {
				if value == "SECRETFILE" {
					value = secretPath
				}
				t.Setenv(key, value)
			}
			options := testCase.options
			options.URL = "https://sync.example.com"
			if strings.HasPrefix(name, "--token-file") {
				options.TokenFile = secretPath
			}
			if name == "stdin" {
				options.Stdin = strings.NewReader("from-stdin\n")
			}
			if _, err := Setup(context.Background(), dir, options); err != nil {
				t.Fatalf("Setup: %v", err)
			}
			if got := savedConnection(t, dir).Secret; got != testCase.want {
				t.Fatalf("secret = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestSetupRefusesToReplaceWithoutForce keeps a provisioning run from silently
// repointing a machine that is already synchronized somewhere else. Both copies of
// a profile diverging is exactly the state the conflict rules exist to avoid.
func TestSetupRefusesToReplaceWithoutForce(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://one.example.com", Token: "s"}); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://two.example.com", Token: "s"}); err == nil {
		t.Fatal("a different URL must require --force")
	}
	if got := savedConnection(t, dir).URL; got != "https://one.example.com" {
		t.Fatalf("the refused setup changed the connection: %q", got)
	}
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://two.example.com", Token: "s", Force: true}); err != nil {
		t.Fatalf("forced setup: %v", err)
	}
	if got := savedConnection(t, dir).URL; got != "https://two.example.com" {
		t.Fatalf("force did not replace the connection: %q", got)
	}
}

// TestSetupIsIdempotent pins that re-running the same provisioning twice reports
// no change, so a converge-in-place script is quiet and its file mtime is stable.
func TestSetupIsIdempotent(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	options := SetupOptions{URL: "https://sync.example.com", Token: "sync-secret"}
	if _, err := Setup(context.Background(), dir, options); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	before, err := os.Stat(filepath.Join(dir, connectionFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	result, err := Setup(context.Background(), dir, options)
	if err != nil {
		t.Fatalf("second setup: %v", err)
	}
	if result.Changed {
		t.Fatal("an identical setup reported a change")
	}
	after, err := os.Stat(filepath.Join(dir, connectionFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an identical setup rewrote the connection file")
	}
}

// TestSetupRotatesTheSecretOfASavedURL covers the token-rotation case: the URL
// stays, only the secret changes, and no force is needed because the destination
// is the same service.
func TestSetupRotatesTheSecretOfASavedURL(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://sync.example.com", Token: "old"}); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	result, err := Setup(context.Background(), dir, SetupOptions{Token: "new"})
	if err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if !result.Changed || result.URL != "https://sync.example.com" {
		t.Fatalf("result = %+v", result)
	}
	if got := savedConnection(t, dir).Secret; got != "new" {
		t.Fatalf("secret = %q, want the rotated one", got)
	}
}

// TestSetupValidatesTheURL pins that the scripted path applies the login dialog's
// constraints, so it cannot save something the next startup would refuse.
func TestSetupValidatesTheURL(t *testing.T) {
	setupEnv(t)
	for _, url := range []string{
		"sync.example.com",
		"ftp://sync.example.com",
		"https://sync.example.com/v1",
		"https://user:pass@sync.example.com",
		"https://sync.example.com?a=1",
	} {
		t.Run(url, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Setup(context.Background(), dir, SetupOptions{URL: url, Token: "s"}); err == nil {
				t.Fatalf("Setup accepted %q", url)
			}
			if _, err := os.Stat(filepath.Join(dir, connectionFile)); !os.IsNotExist(err) {
				t.Fatal("a rejected URL wrote a connection")
			}
		})
	}
}

// TestSetupRequiresAURLAndSecret keeps a half-configured device from being saved:
// a connection with no secret would fail every startup with a less specific error.
func TestSetupRequiresAURLAndSecret(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{Token: "s"}); err == nil {
		t.Fatal("Setup accepted a connection with no URL")
	}
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://sync.example.com"}); err == nil {
		t.Fatal("Setup accepted a connection with no secret")
	}
}

// TestSetupVerifiesWhenAsked proves the optional check really contacts the
// service, and that a refusal leaves the previous connection untouched.
func TestSetupVerifiesWhenAsked(t *testing.T) {
	setupEnv(t)
	_, _, server := testClients(t)
	dir := t.TempDir()
	result, err := Setup(context.Background(), dir, SetupOptions{URL: server.URL, Token: "sync-secret", Verify: true})
	if err != nil {
		t.Fatalf("verified setup: %v", err)
	}
	if !result.Verified {
		t.Fatal("Setup did not report the verification it performed")
	}
	if got := savedConnection(t, dir).URL; got != server.URL {
		t.Fatalf("saved URL = %q", got)
	}

	// A wrong secret must fail verification and leave the good connection alone.
	other := t.TempDir()
	if _, err := Setup(context.Background(), other, SetupOptions{URL: server.URL, Token: "sync-secret"}); err != nil {
		t.Fatalf("baseline setup: %v", err)
	}
	if _, err := Setup(context.Background(), other, SetupOptions{URL: server.URL, Token: "wrong", Verify: true, Force: true}); err == nil {
		t.Fatal("verification accepted a wrong secret")
	}
	if got := savedConnection(t, other).Secret; got != "sync-secret" {
		t.Fatalf("a failed verification replaced the connection: %q", got)
	}
}

// TestClearRemovesTheConnection pins that clearing is idempotent and leaves the
// downloaded profile in place: disconnecting must not delete a user's settings.
func TestClearRemovesTheConnection(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://sync.example.com", Token: "s"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := Setup(context.Background(), dir, SetupOptions{Clear: true})
		if err != nil {
			t.Fatalf("clear %d: %v", attempt, err)
		}
		if !result.Changed {
			t.Fatalf("clear %d reported no change", attempt)
		}
		if _, err := os.Stat(filepath.Join(dir, connectionFile)); !os.IsNotExist(err) {
			t.Fatal("clear left the connection file")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("clear removed local profile state: %v", err)
	}
}

// TestConnectionSourcesAreOrdered pins the documented precedence over the whole
// load path, not only Setup: the environment is what an unattended client uses, so
// its override rules must hold at startup too.
func TestConnectionSourcesAreOrdered(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://saved.example.com", Token: "saved-secret"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// The saved connection stands alone.
	if got, err := loadConnection(dir); err != nil || got.URL != "https://saved.example.com" || got.Secret != "saved-secret" {
		t.Fatalf("saved only: %+v, %v", got, err)
	}

	// A token alone rotates the saved secret without touching the URL.
	t.Setenv(EnvToken, "rotated")
	if got, err := loadConnection(dir); err != nil || got.URL != "https://saved.example.com" || got.Secret != "rotated" {
		t.Fatalf("token only: %+v, %v", got, err)
	}

	// A URL in the environment overrides the saved connection entirely and never
	// borrows the saved secret.
	t.Setenv(EnvURL, "https://env.example.com")
	t.Setenv(EnvToken, "")
	if got, err := loadConnection(dir); err != nil || got.URL != "https://env.example.com" || got.Secret != "" {
		t.Fatalf("url override must not borrow the saved secret: %+v, %v", got, err)
	}

	secretPath := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	t.Setenv(EnvTokenFile, secretPath)
	if got, err := loadConnection(dir); err != nil || got.URL != "https://env.example.com" || got.Secret != "file-secret" {
		t.Fatalf("token file: %+v, %v", got, err)
	}
}

// TestMissingSecretFileFailsLoudly pins that a client pointed at a mounted secret
// refuses instead of falling back: a silent fallback would sync to the saved
// service with the saved secret while the operator believes the mount is in use.
func TestMissingSecretFileFailsLoudly(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://saved.example.com", Token: "saved-secret"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv(EnvTokenFile, filepath.Join(t.TempDir(), "absent"))
	if _, err := loadConnection(dir); err == nil {
		t.Fatal("a missing secret file must be an error, not a fallback")
	}
}

// TestShowConnectionNeverPrintsTheSecret pins the redaction: a `status`-style
// command is safe to run in a log.
func TestShowConnectionNeverPrintsTheSecret(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	if _, err := Setup(context.Background(), dir, SetupOptions{URL: "https://sync.example.com", Token: "top-secret"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	shown, err := ShowConnection(dir)
	if err != nil {
		t.Fatalf("ShowConnection: %v", err)
	}
	if shown.URL != "https://sync.example.com" || shown.Secret != "" {
		t.Fatalf("shown = %+v", shown)
	}
}
