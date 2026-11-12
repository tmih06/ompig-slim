package broker

import (
	"context"
	"encoding/json"
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
// index before reconciliation makes deletions retryable across process restarts.
// The caller holds the profile lock; each returned file uses normal per-file CAS.
func (c *Client) themeFiles(ctx context.Context) ([]string, error) {
	names, err := localThemes(c.dir)
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
	names = append(append(names, remote...), known...)
	for _, name := range names {
		if !themeName(name) {
			return nil, fmt.Errorf("invalid theme name in sync index")
		}
	}
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
	return names, nil
}
