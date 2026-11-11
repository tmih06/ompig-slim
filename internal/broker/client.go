package broker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// Client synchronizes local profile state, never model traffic. The mutex and
// filesystem lock serialize sibling sessions sharing the same local profile.
type Client struct {
	base, token, dir string
	http             *http.Client
	mu               sync.Mutex

	snapshotMu   sync.Mutex
	snapshotData []vaultEntry
	snapshotAt   time.Time
	snapshotLoad bool
}

// fileState records the common ancestor for preference document reconciliation.
type fileState struct {
	Remote string `json:"remote"`
	Local  string `json:"local"`
}

// Connect opts the existing local profile into hosted storage. First connection
// adopts existing remote preferences (backing up local differences), or seeds an
// empty host. Provider configuration is copied unchanged: inference stays local.
func Connect(ctx context.Context, dir string) (*Client, error) {
	saved, err := loadConnection(dir)
	if err != nil {
		installFailedStore(dir, err)
		return nil, err
	}
	if saved.URL == "" {
		return nil, nil
	}
	c, err := newClient(dir, saved)
	if err != nil {
		installFailedStore(dir, err)
		return nil, err
	}
	// Install before any filesystem or network work can fail.
	installRemoteStore(c)
	dir = c.dir
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = c.SyncFiles(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

const snapshotCacheTTL = time.Second

// snapshot returns a short-lived shared credential view. Holding the mutex
// across the request makes concurrent provider availability checks single-flight;
// the TTL also joins sequential checks in the same startup pass. Mutations
// invalidate the view, while the short bound preserves live broker semantics for
// changes made by another client.
func (c *Client) snapshot(ctx context.Context) ([]vaultEntry, error) {
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	if c.snapshotLoad && time.Since(c.snapshotAt) < snapshotCacheTTL {
		return append([]vaultEntry(nil), c.snapshotData...), nil
	}
	var entries []vaultEntry
	if err := c.request(ctx, "GET", "/v1/snapshot", "", nil, &entries); err != nil {
		return nil, err
	}
	c.snapshotData = append(c.snapshotData[:0], entries...)
	c.snapshotAt = time.Now()
	c.snapshotLoad = true
	return append([]vaultEntry(nil), c.snapshotData...), nil
}

// invalidateSnapshot prevents a successful local mutation from returning the
// pre-mutation credential view to the next caller.
func (c *Client) invalidateSnapshot() {
	c.snapshotMu.Lock()
	c.snapshotLoad = false
	c.snapshotData = nil
	c.snapshotMu.Unlock()
}

// request sends preference or vault operations with bounded response size and
// latency. It never follows redirects that could disclose bearer or credentials.
func (c *Client) request(ctx context.Context, method, path, revision string, body any, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	if revision != "" {
		req.Header.Set("If-Match", `"`+revision+`"`)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == 412 {
		return fmt.Errorf("broker revision conflict")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("broker %s: HTTP %d", path, response.StatusCode)
	}
	if result != nil {
		dec := json.NewDecoder(io.LimitReader(response.Body, 12<<20))
		if err := dec.Decode(result); err != nil {
			return err
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			if err == nil {
				return fmt.Errorf("broker response contains multiple JSON values")
			}
			return err
		}
		return nil
	}
	// Drain the remaining body so the connection stays reusable; an unread body
	// would make net/http close it and cost every later reconcile a handshake.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return nil
}

// hashData fingerprints absent and present content for CAS/local dirty checks.
func hashData(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// locked serializes profile operations across goroutines and local processes.
func (c *Client) locked(ctx context.Context, run func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock := flock.New(filepath.Join(c.dir, ".broker-sync.lock"))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("profile sync lock unavailable")
	}
	defer lock.Unlock()
	return run()
}

// statePath separates synchronization ancestry for different hosted locations.
func (c *Client) statePath(name string) string {
	return filepath.Join(c.dir, ".broker-sync-"+hashData([]byte(c.base))[:16]+"-"+name)
}

// loadState treats only a missing ancestry file as first connection. Corrupt
// ancestry must not silently turn an unsynced edit into an authoritative pull.
func (c *Client) loadState(name string) (fileState, bool, error) {
	var state fileState
	data, err := os.ReadFile(c.statePath(name))
	if os.IsNotExist(err) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	err = json.Unmarshal(data, &state)
	return state, true, err
}

// saveState publishes ancestry only after its corresponding data is persisted.
func (c *Client) saveState(name string, state fileState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return replaceFile(c.statePath(name), data)
}

// readLocal reads bounded regular files; symlinks are refused rather than
// reading secrets outside the opted-in profile through an unexpected link.
func readLocal(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxItemBytes {
		return nil, fmt.Errorf("not a bounded regular profile file: %s", path)
	}
	return os.ReadFile(path)
}

// SyncFiles reconciles independent documents with CAS. Concurrent edits of the
// same file fail explicitly and leave both copies intact; unrelated files merge.
// Deletions are synchronized once a common ancestor has been recorded.
func (c *Client) SyncFiles(ctx context.Context) error {
	return c.locked(ctx, func() error {
		for _, name := range SharedFiles {
			local, err := readLocal(filepath.Join(c.dir, name))
			if err != nil {
				return err
			}
			state, known, err := c.loadState(name)
			if err != nil {
				return err
			}
			var remote Item
			if err = c.request(ctx, "GET", "/v1/items/"+name, "", nil, &remote); err != nil {
				return err
			}
			localHash := hashData(local)
			// The loaded ancestor is already durable; an idle poll must not
			// replace and fsync it again when neither side has changed.
			if known && localHash == state.Local && remote.Revision == state.Remote {
				continue
			}
			if !known {
				if remote.Data != nil && !bytes.Equal(local, remote.Data) {
					if local != nil {
						if err = replaceFile(c.statePath(name+".backup"), local); err != nil {
							return err
						}
					}
					if err = replaceFile(filepath.Join(c.dir, name), remote.Data); err != nil {
						return err
					}
					local = remote.Data
				} else if remote.Data == nil && local != nil {
					if err = c.request(ctx, "PUT", "/v1/items/"+name, remote.Revision, Item{Data: local}, nil); err != nil {
						return err
					}
					remote.Data = local
					remote.Revision = localHash
				}
			} else {
				localChanged := localHash != state.Local
				remoteChanged := remote.Revision != state.Remote
				if localChanged && remoteChanged && !bytes.Equal(local, remote.Data) {
					return fmt.Errorf("sync conflict in %s: local and hosted copies retained; reconcile them before retrying", name)
				}
				if localChanged && !bytes.Equal(local, remote.Data) {
					if err = c.request(ctx, "PUT", "/v1/items/"+name, remote.Revision, Item{Data: local}, nil); err != nil {
						return err
					}
					remote.Data = local
					remote.Revision = localHash
				} else if remoteChanged {
					if remote.Data == nil {
						err = os.Remove(filepath.Join(c.dir, name))
						if os.IsNotExist(err) {
							err = nil
						}
					} else {
						err = replaceFile(filepath.Join(c.dir, name), remote.Data)
					}
					if err != nil {
						return err
					}
					local = remote.Data
				}
			}
			if err = c.saveState(name, fileState{Remote: remote.Revision, Local: hashData(local)}); err != nil {
				return err
			}
		}
		return nil
	})
}

// RunSync periodically reconciles preferences between local
// requests. Runtime settings/catalog reload behavior remains PiG's own; a new
// session/startup always sees the synchronized files. Report receives no secrets.
func (c *Client) RunSync(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.SyncFiles(ctx); err != nil {
				report(err)
				continue
			}
		}
	}
}

// Start wires preference reconciliation and an idempotent exit flush into the
// CLI. A configured but unavailable broker leaves credential access remote-only:
// the CLI can repair the connection, but must not silently use a local vault.
func Start(ctx context.Context, dir string, stderr io.Writer) (func() error, error) {
	startup, cancel := context.WithTimeout(ctx, 45*time.Second)
	client, err := Connect(startup, dir)
	cancel()
	if err != nil {
		fmt.Fprintln(stderr, "profile sync inactive:", err)
		return nil, nil
	}
	if client == nil {
		return nil, nil
	}
	background, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		last := ""
		client.RunSync(background, func(err error) {
			if text := err.Error(); text != last {
				fmt.Fprintln(stderr, "profile sync:", text)
				last = text
			}
		})
	}()
	var once sync.Once
	var finalErr error
	return func() error {
		once.Do(func() {
			stop()
			<-done
			flush, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			finalErr = client.SyncFiles(flush)
		})
		return finalErr
	}, nil
}
