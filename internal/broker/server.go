// Package broker owns a SQLite credential vault and synchronized preferences.
// Provider inference stays on clients; only OAuth refresh executes on the host.
package broker

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
)

const maxItemBytes = 8 << 20

// SharedFiles is the portable configuration allowlist. Credentials are served
// only by vault APIs, never as a downloadable raw database or document.
var SharedFiles = []string{"settings.json", "keybindings.json", "models.json", "models.yml", "models.yaml"}

// Item is a preference document with a CAS revision; nil Data is a tombstone.
type Item struct {
	Data     []byte `json:"data"`
	Revision string `json:"revision"`
}

// Store owns one vault and serializes preference commits. One process must own
// its directory; the executable enforces a filesystem lock. Close after draining
// HTTP and stopping RunRefresher, so no refresh loses its persistence target.
type Store struct {
	dir       string
	mu        sync.Mutex
	vault     *ai.SQLiteAuthStorage
	handler   http.Handler
	lifecycle sync.RWMutex
	closed    bool
}

// NewHandler opens the canonical SQLite vault. Every data operation requires
// the shared bearer; TLS is operator-owned.
func NewHandler(dir, token string) (*Store, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("broker token is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf("make data directory private: %w", err)
	}
	vault, err := ai.OpenLocalCredentialStore(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, vault: vault}
	digest := sha256.Sum256([]byte(token))
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/items/{name}", s.get)
	routes.HandleFunc("PUT /v1/items/{name}", s.put)
	routes.HandleFunc("GET /v1/snapshot", s.snapshot)
	routes.HandleFunc("POST /v1/credential", s.credential)
	routes.HandleFunc("POST /v1/credential/refresh", s.refresh)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok\n") })
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		actual := sha256.Sum256([]byte(strings.TrimPrefix(raw, "Bearer ")))
		if !strings.HasPrefix(raw, "Bearer ") || subtle.ConstantTimeCompare(digest[:], actual[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		routes.ServeHTTP(w, r)
	}))
	s.handler = mux
	return s, nil
}

// ServeHTTP admits authenticated vault and preference requests, never inference.
// Holding the lifecycle read lock keeps SQLite alive through any rotated-token
// persistence, even if HTTP shutdown times out and disconnects the caller.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed {
		http.Error(w, "broker closed", http.StatusServiceUnavailable)
		return
	}
	s.handler.ServeHTTP(w, r)
}

// Close waits for admitted HTTP work and closes SQLite once. Callers must stop
// RunRefresher first; new HTTP requests are refused after the vault is closed.
func (s *Store) Close() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.vault.Close()
}

// itemName admits only portable preference documents.
func itemName(name string) bool { return slices.Contains(SharedFiles, name) }

// read returns a tombstone for absent preferences. Callers hold s.mu.
func (s *Store) read(name string) (Item, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return Item{}, err
	}
	return Item{Data: data, Revision: hashData(data)}, nil
}

// get returns a consistent preference document without interpreting its values.
func (s *Store) get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !itemName(name) {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.read(name)
	if err != nil {
		http.Error(w, "storage read failed", 500)
		return
	}
	w.Header().Set("ETag", `"`+item.Revision+`"`)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// put commits one allowlisted preference with CAS; stale writes never overwrite.
func (s *Store) put(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !itemName(name) {
		http.NotFound(w, r)
		return
	}
	var next Item
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<20))
	if dec.Decode(&next) != nil || dec.Decode(new(any)) != io.EOF || len(next.Data) > maxItemBytes {
		http.Error(w, "invalid item", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.read(name)
	if err != nil {
		http.Error(w, "storage read failed", 500)
		return
	}
	if r.Header.Get("If-Match") != `"`+current.Revision+`"` {
		http.Error(w, "revision conflict", 412)
		return
	}
	if next.Data == nil {
		err = os.Remove(filepath.Join(s.dir, name))
		if os.IsNotExist(err) {
			err = nil
		}
	} else {
		err = replaceFile(filepath.Join(s.dir, name), next.Data)
	}
	if err != nil {
		http.Error(w, "storage write failed", 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// replaceFile publishes a fully written private file atomically; fsync ensures
// acknowledged credential rotations survive an ordinary service restart.
func replaceFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".broker-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
