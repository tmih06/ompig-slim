package broker

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestThemeSync pins discovery, opaque file transport, first-adoption backups,
// independent conflicts and deletion propagation after recreating the client.
func TestThemeSync(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	for _, c := range []*Client{a, b} {
		if err := prepareThemeDir(c.dir); err != nil {
			t.Fatal(err)
		}
	}
	name := "themes/night #1?.json"
	mustWrite(t, filepath.Join(a.dir, name), `{"name":"night","vars":{"bg":"#000000"}}`)
	mustWrite(t, filepath.Join(b.dir, name), `{"name":"old"}`)
	mustWrite(t, filepath.Join(a.dir, "themes", "ignored.txt"), "local")
	for _, c := range []*Client{a, b} {
		if err := c.SyncFiles(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := os.ReadFile(filepath.Join(a.dir, name))
	got, err := os.ReadFile(filepath.Join(b.dir, name))
	if err != nil || string(got) != string(want) {
		t.Fatalf("theme download: %s, %v", got, err)
	}
	backup, err := os.ReadFile(b.statePath(name + ".backup"))
	if err != nil || string(backup) != `{"name":"old"}` {
		t.Fatalf("adoption backup: %s, %v", backup, err)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "themes", "ignored.txt")); !os.IsNotExist(err) {
		t.Fatalf("non-theme synchronized: %v", err)
	}
	mustWrite(t, filepath.Join(a.dir, name), `{"name":"a"}`)
	mustWrite(t, filepath.Join(b.dir, name), `{"name":"b"}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(b.dir, "keybindings.json"), `{}`)
	mustWrite(t, filepath.Join(b.dir, "themes", "other.json"), `{"name":"other"}`)
	if err := b.SyncFiles(ctx); err == nil || !strings.Contains(err.Error(), "sync conflict") {
		t.Fatalf("expected theme conflict, got %v", err)
	}
	got, _ = os.ReadFile(filepath.Join(b.dir, name))
	if string(got) != `{"name":"b"}` {
		t.Fatal("conflict overwrote local theme")
	}
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "themes", "other.json")); err != nil {
		t.Fatal("theme conflict blocked independent theme", err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "keybindings.json")); err != nil {
		t.Fatal("theme conflict blocked keybindings", err)
	}
	mustWrite(t, filepath.Join(b.dir, name), `{"name":"a"}`)
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(a.dir, name)); err != nil {
		t.Fatal(err)
	}
	// Recreating the client proves deletion discovery depends on durable state,
	// not an in-memory list of themes from a previous synchronization.
	a = &Client{base: a.base, token: a.token, dir: a.dir, http: a.http}
	for _, c := range []*Client{a, b} {
		if err := c.SyncFiles(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(b.dir, name)); !os.IsNotExist(err) {
		t.Fatalf("theme deletion not synchronized: %v", err)
	}
}

// TestThemePathSafety refuses traversal, non-JSON paths and symlink escapes on
// both sides; a failed theme must not prevent independent settings from syncing.
func TestThemePathSafety(t *testing.T) {
	for _, name := range []string{"themes/../secret.json", "themes/sub/a.json", "themes/a.txt", "themes/a\\b.json", "themes/.json", "../themes/a.json"} {
		if itemName(name) {
			t.Errorf("unsafe name admitted: %q", name)
		}
	}
	a, b, _ := testClients(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.json"), "secret")
	if err := os.Symlink(outside, filepath.Join(a.dir, "themes")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(a.dir, "settings.json"), `{"theme":"dark"}`)
	if err := a.SyncFiles(context.Background()); err == nil {
		t.Fatal("accepted symlink theme directory")
	}
	if err := b.SyncFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "settings.json")); err != nil {
		t.Fatal("theme failure blocked settings", err)
	}
	if err := os.Remove(filepath.Join(a.dir, "themes")); err != nil {
		t.Fatal(err)
	}
	if err := prepareThemeDir(a.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(a.dir, "themes", "escape.json")); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncFiles(context.Background()); err == nil {
		t.Fatal("accepted symlink theme file")
	}
	store, err := NewHandler(t.TempDir(), "sync-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.Symlink(outside, filepath.Join(store.dir, "themes")); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(store)
	defer server.Close()
	a.base = server.URL
	var item Item
	if err := a.request(context.Background(), "GET", "/v1/items/themes/secret.json", "", nil, &item); err == nil {
		t.Fatal("server followed symlink directory")
	}
}

// TestThemeDiscoveryCreatesNoStorage pins the deployment volume contract: a host
// and a client that have seen no theme must not acquire an empty themes directory
// merely by listing, probing or reconciling. Creating the directory on read put a
// `themes` entry in the broker data volume, which the container and cluster suites
// reject as unsynchronized state. Only writing a theme may create it.
func TestThemeDiscoveryCreatesNoStorage(t *testing.T) {
	host, err := NewHandler(t.TempDir(), "sync-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	server := httptest.NewServer(host)
	defer server.Close()
	client := &Client{base: server.URL, token: "sync-secret", dir: t.TempDir(), http: server.Client()}

	var names []string
	if err := client.request(context.Background(), "GET", "/v1/themes", "", nil, &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("empty host listed themes: %v", names)
	}
	var item Item
	if err := client.request(context.Background(), "GET", "/v1/items/themes/absent.json", "", nil, &item); err != nil {
		t.Fatal(err)
	}
	if item.Data != nil {
		t.Fatalf("absent theme has data: %s", item.Data)
	}
	if err := client.SyncFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{host.dir, client.dir} {
		if _, err := os.Stat(filepath.Join(dir, "themes")); !os.IsNotExist(err) {
			t.Errorf("discovery created theme storage in %s: %v", dir, err)
		}
	}

	// A theme that really exists still creates the directory and syncs.
	if err := prepareThemeDir(client.dir); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(client.dir, "themes", "night.json"), `{"name":"night"}`)
	if err := client.SyncFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(host.dir, "themes", "night.json")); err != nil {
		t.Fatalf("theme upload: %v", err)
	}
}
