package broker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// themeItemGets counts theme item requests so a test can pin the per-poll cost
// rather than infer it from the index contents.
type themeItemGets struct {
	inner http.RoundTripper
	count atomic.Int32
}

// RoundTrip forwards every request and counts only theme item reads.
func (transport *themeItemGets) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/items/themes/") {
		transport.count.Add(1)
	}
	return transport.inner.RoundTrip(request)
}

// TestThemeIndexPrunesAbsentThemes pins the poll cost contract: a theme name
// missing from both the profile and the host has nothing left to reconcile, so
// reconciliation drops its index entry and its ancestry file. Retaining it cost
// one item request per name on every five-second poll forever, which a single
// bulk copy-then-delete of the optional themes left behind for 99 names.
// A first-adoption backup survives the prune because it holds the user's own
// pre-sync bytes, and a name still present on either side keeps its ancestry so
// an unsynced upload or an unpushed deletion is not silently forgotten.
func TestThemeIndexPrunesAbsentThemes(t *testing.T) {
	client, _, _ := testClients(t)
	ctx := context.Background()
	counter := &themeItemGets{inner: client.http.Transport}
	client.http = &http.Client{Transport: counter}
	if err := prepareThemeDir(client.dir); err != nil {
		t.Fatal(err)
	}
	gone, kept := "themes/gone.json", "themes/kept.json"
	for _, name := range []string{gone, kept} {
		mustWrite(t, filepath.Join(client.dir, name), `{"name":"kept"}`)
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, client.statePath(gone+".backup"), `{"name":"adopted"}`)
	if _, err := os.Stat(client.statePath(gone)); err != nil {
		t.Fatalf("no ancestry to prune: %v", err)
	}

	// Deleting locally propagates the tombstone on one poll; the ancestry that
	// records it must survive that poll, because syncFile needs it to know the
	// deletion was pushed. The following poll is the one that can prune.
	if err := os.Remove(filepath.Join(client.dir, gone)); err != nil {
		t.Fatal(err)
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(client.statePath(gone)); err != nil {
		t.Fatalf("tombstone ancestry pruned before it was pushed: %v", err)
	}
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(client.statePath(gone)); !os.IsNotExist(err) {
		t.Fatalf("absent theme kept ancestry: %v", err)
	}
	// The backup holds the user's own pre-sync bytes and may be their only copy,
	// so collecting the ancestry that named it must not collect it too.
	if _, err := os.Stat(client.statePath(gone + ".backup")); err != nil {
		t.Fatalf("prune removed a first-adoption backup: %v", err)
	}
	if _, err := os.Stat(client.statePath(kept)); err != nil {
		t.Fatalf("prune removed live theme ancestry: %v", err)
	}

	index, err := os.ReadFile(client.statePath("theme-index"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), gone) || !strings.Contains(string(index), kept) {
		t.Fatalf("index after prune: %s", index)
	}

	// A pruned name stays gone: it is not rediscovered, so it costs no more
	// requests and its ancestry is not resurrected as an empty tombstone. An idle
	// poll now reads exactly the one live theme, which is the whole point of the
	// prune: the per-poll cost tracks themes that exist, not names ever seen.
	counter.count.Store(0)
	if err := client.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(client.statePath(gone)); !os.IsNotExist(err) {
		t.Fatalf("pruned theme returned: %v", err)
	}
	if got := counter.count.Load(); got != 1 {
		t.Fatalf("idle poll read %d theme documents, want 1", got)
	}
}

// TestThemeIndexKeepsUnreconciledThemes proves the prune only collects names that
// finished their work. A theme still in conflict keeps its index entry and
// ancestry so the retry the index exists for is still possible; dropping it would
// forget both sides of the disagreement.
func TestThemeIndexKeepsUnreconciledThemes(t *testing.T) {
	a, b, _ := testClients(t)
	ctx := context.Background()
	for _, c := range []*Client{a, b} {
		if err := prepareThemeDir(c.dir); err != nil {
			t.Fatal(err)
		}
	}
	name := "themes/contested.json"
	mustWrite(t, filepath.Join(a.dir, name), `{"name":"a"}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(a.dir, name), `{"name":"a2"}`)
	mustWrite(t, filepath.Join(b.dir, name), `{"name":"b2"}`)
	if err := a.SyncFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.SyncFiles(ctx); err == nil || !strings.Contains(err.Error(), "sync conflict") {
		t.Fatalf("expected theme conflict, got %v", err)
	}
	// The conflicted theme is present on both sides, so it is never a prune
	// candidate; the interesting part is that a second poll stays conflict-free
	// about the index and keeps the ancestry available for the retry.
	for i := 0; i < 2; i++ {
		if err := b.SyncFiles(ctx); err == nil || !strings.Contains(err.Error(), "sync conflict") {
			t.Fatalf("poll %d: expected unresolved conflict, got %v", i, err)
		}
	}
	index, err := os.ReadFile(b.statePath("theme-index"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), name) {
		t.Fatalf("conflicted theme left the index: %s", index)
	}
	if _, err := os.Stat(b.statePath(name)); err != nil {
		t.Fatalf("conflicted theme lost ancestry: %v", err)
	}
	// Reconciling to the hosted bytes is the documented recovery, and it still
	// needs the ancestry this prune left alone.
	mustWrite(t, filepath.Join(b.dir, name), `{"name":"a2"}`)
	if err := b.SyncFiles(ctx); err != nil {
		t.Fatalf("retry after reconciling the local copy: %v", err)
	}
	if _, err := os.Stat(b.statePath(name)); err != nil {
		t.Fatalf("reconciled theme lost ancestry: %v", err)
	}
}
