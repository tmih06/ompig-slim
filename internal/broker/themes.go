package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// themeName admits only direct JSON children of the profile's themes directory.
// Separators, traversal and non-theme resources must never reach file operations.
func themeName(name string) bool {
	file, ok := strings.CutPrefix(name, "themes/")
	return ok && len(file) > len(".json") && strings.HasSuffix(file, ".json") &&
		!strings.ContainsAny(file, "/\\\x00")
}

// checkThemeDir reports whether the theme directory is present and usable without
// creating it. A missing directory means no themes: discovery and reads must
// never leave an empty directory behind, so a host that stored no theme keeps a
// storage volume holding only synchronized documents. A symlink or non-directory
// in its place is refused, because every theme operation must stay inside the
// profile. Profile directories are trusted; this does not defend against
// concurrent local attackers swapping paths while the process is operating.
func checkThemeDir(dir string) error {
	info, err := os.Lstat(filepath.Join(dir, "themes"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("themes must be a real directory")
	}
	return nil
}

// prepareThemeDir creates the private theme directory ahead of a write, then
// applies the same refusal. Only writers call it: creating storage is a write
// side effect that a listing or a read must not perform.
func prepareThemeDir(dir string) error {
	if err := os.Mkdir(filepath.Join(dir, "themes"), 0700); err != nil && !os.IsExist(err) {
		return err
	}
	return checkThemeDir(dir)
}

// localThemes discovers direct JSON names without following links or recursing.
// Non-regular JSON-named entries remain visible so reconciliation reports the
// unsafe file rather than silently treating it as a deletion.
func localThemes(dir string) ([]string, error) {
	if err := checkThemeDir(dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "themes"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		name := "themes/" + entry.Name()
		if themeName(name) {
			names = append(names, name)
		}
	}
	return names, nil
}

// themes lists hosted theme names under the preference lock. Deleted names need
// not remain on the server: clients retain their own index to reconcile absence.
func (s *Store) themes(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	names, err := localThemes(s.dir)
	if err != nil {
		http.Error(w, "storage read failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(names)
}

// publishDocument writes one synchronized document into the profile, creating the
// private theme directory first when the document is a theme. Writers own that
// creation: a downloaded theme needs a parent directory, while an unread profile
// stays without one.
func publishDocument(dir, name string, data []byte) error {
	if themeName(name) {
		if err := prepareThemeDir(dir); err != nil {
			return err
		}
	}
	return replaceFile(filepath.Join(dir, name), data)
}

// themeFiles unions local, hosted and previously seen names. Persisting the
// index before reconciliation makes deletions retryable across process restarts;
// a name missing from both the profile and the host has nothing left to
// reconcile, so it leaves the index with its ancestry instead of costing one
// item request on every poll for the rest of the profile's life. One bulk
// copy-then-delete of the optional themes would otherwise leave that permanent
// per-name cost behind.
//
// Ancestry is removed before the index is rewritten so a crash between the two
// steps stays recoverable: the surviving index entry re-derives the same prune
// set on the next poll and rewrites the index again. The reverse order would
// strand ancestry files that no index entry names, which nothing would ever
// collect. Both steps are idempotent.
//
// The caller holds the profile lock; each returned file uses normal per-file CAS.
func (c *Client) themeFiles(ctx context.Context) ([]string, error) {
	local, err := localThemes(c.dir)
	if err != nil {
		return nil, err
	}
	var remote []string
	if err := c.request(ctx, "GET", "/v1/themes", "", nil, &remote); err != nil {
		return nil, err
	}
	path := c.statePath("theme-index")
	data, err := readLocal(path)
	if err != nil {
		return nil, err
	}
	var known []string
	if data != nil {
		if err := json.Unmarshal(data, &known); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Concat(local, remote, known) {
		if !themeName(name) {
			return nil, fmt.Errorf("invalid theme name in sync index")
		}
	}
	// A name present on either side keeps its ancestry: a local-only theme may be
	// an unsynced upload or an unpushed deletion, and a hosted-only one an
	// unsynced download or applied deletion.
	present := make(map[string]struct{}, len(local)+len(remote))
	for _, name := range slices.Concat(local, remote) {
		present[name] = struct{}{}
	}
	var kept, pruned []string
	for _, name := range known {
		if _, ok := present[name]; ok {
			kept = append(kept, name)
			continue
		}
		pruned = append(pruned, name)
	}
	var failures []error
	for _, name := range pruned {
		// Only ancestry is removed. A first-adoption backup holds the user's own
		// pre-sync bytes and may be the only copy of them, so it stays.
		if err := os.Remove(c.statePath(name)); err != nil && !os.IsNotExist(err) {
			// Keep the name so the index still records the work to retry; dropping
			// it here would leak the ancestry file with nothing left to collect it.
			kept = append(kept, name)
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
	}
	names := make([]string, 0, len(local)+len(remote)+len(kept))
	names = append(names, local...)
	names = append(names, remote...)
	names = append(names, kept...)
	slices.Sort(names)
	names = slices.Compact(names)
	next, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}
	if string(next) != string(data) {
		if err := replaceFile(path, next); err != nil {
			return nil, err
		}
	}
	// Pruned names no longer synchronize, so unrelated documents still may: the
	// joined errors name the ancestry that could not be collected, and the next
	// poll retries them because the failure kept their index entries.
	return names, errors.Join(failures...)
}
