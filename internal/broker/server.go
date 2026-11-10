// Package broker implements a hosted profile store. It never constructs a model
// runtime, calls providers, or refreshes OAuth tokens; clients do all of that.
package broker

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const maxItemBytes = 8 << 20
const leaseDuration = 60 * time.Second

// SharedFiles is the explicit portable configuration allowlist. Neither session
// logs nor the SQLite database itself is ever uploaded.
var SharedFiles = []string{"settings.json", "keybindings.json", "models.json", "models.yml", "models.yaml"}

// Item is an opaque profile document and its content revision. Nil Data is a
// deletion tombstone. Revisions support compare-and-swap, not last-writer-wins.
type Item struct {
	Data     []byte `json:"data"`
	Revision string `json:"revision"`
}

// Store serializes document commits and credential leases for a single user.
// One process must own its directory; the executable enforces a filesystem lock.
// A restart revokes all leases, so old clients cannot commit over a new owner.
type Store struct {
	dir        string
	mu         sync.Mutex
	lease      string
	leaseUntil time.Time
}

// NewHandler opens a storage-only HTTP service. Every data operation requires
// the shared bearer; its holder can download all stored secrets. TLS is external.
func NewHandler(dir, token string) (http.Handler, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("broker token is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// The directory holds every synchronized secret, so its mode is enforced even
	// for a directory an image or an operator created with a wider mode; a host
	// that cannot tighten it must be repaired rather than served from.
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf("make data directory private: %w", err)
	}
	s := &Store{dir: dir}
	digest := sha256.Sum256([]byte(token))
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/items/{name}", s.get)
	routes.HandleFunc("PUT /v1/items/{name}", s.put)
	routes.HandleFunc("POST /v1/auth/lease", s.acquire)
	routes.HandleFunc("PUT /v1/auth/lease", s.renew)
	routes.HandleFunc("DELETE /v1/auth/lease", s.release)
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
	return mux, nil
}

// itemName admits only portable preferences and the logical credential snapshot.
func itemName(name string) bool {
	return name == "credentials.json" || slices.Contains(SharedFiles, name)
}

// read returns a tombstone for absent documents. Callers hold s.mu.
func (s *Store) read(name string) (Item, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return Item{}, err
	}
	return Item{Data: data, Revision: hashData(data)}, nil
}

// get returns a consistent document without logging or interpreting its secrets.
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

// put atomically commits one CAS update. Auth updates additionally require the
// current unexpired lease: refreshing clients cannot overwrite a newer rotation.
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
	if name == "credentials.json" && !s.owns(r) {
		http.Error(w, "credential lease required", 423)
		return
	}
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

// owns checks both lease identity and its deadline while the store mutex is held.
func (s *Store) owns(r *http.Request) bool {
	return s.lease != "" && r.Header.Get("X-Broker-Lease") == s.lease && time.Now().Before(s.leaseUntil)
}

// acquire serializes short credential mutations, never inference requests.
func (s *Store) acquire(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lease != "" && time.Now().Before(s.leaseUntil) {
		http.Error(w, "credentials busy", 423)
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		http.Error(w, "lease unavailable", 500)
		return
	}
	s.lease = hex.EncodeToString(random[:])
	s.leaseUntil = time.Now().Add(leaseDuration)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"lease": s.lease})
}

// renew keeps a live client's mutation exclusive without exposing OAuth data.
func (s *Store) renew(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owns(r) {
		http.Error(w, "lease lost", 423)
		return
	}
	s.leaseUntil = time.Now().Add(leaseDuration)
	w.WriteHeader(204)
}

// release is owner-fenced; a late release cannot remove another client's lease.
func (s *Store) release(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owns(r) {
		s.lease = ""
	}
	w.WriteHeader(204)
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
