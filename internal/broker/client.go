package broker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/gofrs/flock"
)

// Client synchronizes local profile state, never model traffic. The mutex and
// filesystem lock serialize sibling sessions sharing the same local profile.
type Client struct {
	base, token, dir string
	http             *http.Client
	mu               sync.Mutex
}

// fileState is the common ancestor for three-way synchronization. Auth records
// both remote revision and local export hash so interrupted rotations are never
// silently overwritten by an older hosted copy on the next startup.
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
		return nil, err
	}
	if saved.URL == "" {
		return nil, nil
	}
	c, err := newClient(dir, saved)
	if err != nil {
		return nil, err
	}
	dir = c.dir
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = c.SyncFiles(ctx); err != nil {
		return nil, err
	}
	store, err := ai.OpenCredentialStore(dir)
	if err != nil {
		return nil, err
	}
	err = c.syncCredentials(ctx, store, func(context.Context) error { return nil })
	store.Close()
	if err != nil {
		return nil, err
	}
	// Installed only after initial pull/migration, before any runtime is opened.
	ai.CredentialStoreSync = func(ctx context.Context, s *ai.SQLiteAuthStorage, run func(context.Context) error) error {
		path, _ := filepath.Abs(s.Path())
		if path != filepath.Join(c.dir, "agent.db") {
			return run(ctx)
		}
		return c.syncCredentials(ctx, s, run)
	}
	return c, nil
}

// request sends storage metadata only, bounds response size/latency, and never
// follows redirects that could disclose the sync bearer or uploaded credentials.
func (c *Client) request(ctx context.Context, method, path, lease, revision string, body any, result any) error {
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
	if lease != "" {
		req.Header.Set("X-Broker-Lease", lease)
	}
	if revision != "" {
		req.Header.Set("If-Match", `"`+revision+`"`)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == 423 {
		return errBusy
	}
	if response.StatusCode == 412 {
		return fmt.Errorf("broker revision conflict")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("broker %s: HTTP %d", path, response.StatusCode)
	}
	if result != nil {
		data, err := io.ReadAll(io.LimitReader(response.Body, 12<<20))
		if err != nil {
			return err
		}
		return json.Unmarshal(data, result)
	}
	// Drain the remaining body so the connection stays reusable; an unread body
	// would make net/http close it and cost every later reconcile a handshake.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return nil
}

var errBusy = errors.New("broker credential lease busy or lost")

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
			if err = c.request(ctx, "GET", "/v1/items/"+name, "", "", nil, &remote); err != nil {
				return err
			}
			localHash := hashData(local)
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
					if err = c.request(ctx, "PUT", "/v1/items/"+name, "", remote.Revision, Item{Data: local}, nil); err != nil {
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
					if err = c.request(ctx, "PUT", "/v1/items/"+name, "", remote.Revision, Item{Data: local}, nil); err != nil {
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

// withLease coordinates only credential storage. The callback and OAuth refresh
// remain on the client. A lost lease fences publication; locally completed
// rotations are retained for explicit recovery, never silently replayed.
func (c *Client) withLease(ctx context.Context, run func(context.Context, string) error) error {
	acquire, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var response struct {
		Lease string `json:"lease"`
	}
	for {
		err := c.request(acquire, "POST", "/v1/auth/lease", "", "", nil, &response)
		if err == nil {
			break
		}
		if !errors.Is(err, errBusy) {
			return err
		}
		select {
		case <-acquire.Done():
			return context.Cause(acquire)
		case <-time.After(100 * time.Millisecond):
		}
	}
	leased, cancelLease := context.WithCancelCause(ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := c.request(leased, "PUT", "/v1/auth/lease", response.Lease, "", nil, nil); err != nil {
					cancelLease(err)
					return
				}
			}
		}
	}()
	// Keep the local mutation context alive on heartbeat failure: a refresh
	// may already have spent a single-use token, so its result must be saved.
	err := run(ctx, response.Lease)
	close(stop)
	<-done
	err = errors.Join(err, context.Cause(leased))
	cancelLease(nil)
	release, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer releaseCancel()
	c.request(release, "DELETE", "/v1/auth/lease", response.Lease, "", nil, nil)
	return err
}

// syncCredentials resolves interrupted writes before pulling newer state, then
// publishes the mutation synchronously. A refresh callback is invoked once only.
// Conflicting local rotations are preserved, never overwritten or replayed.
func (c *Client) syncCredentials(ctx context.Context, s *ai.SQLiteAuthStorage, run func(context.Context) error) error {
	return c.locked(ctx, func() error {
		if run == nil {
			return c.pullCredentials(ctx, s)
		}
		return c.withLease(ctx, func(ctx context.Context, lease string) error {
			local, err := s.ExportCredentials(ctx)
			if err != nil {
				return err
			}
			state, known, err := c.loadState("credentials")
			if err != nil {
				return err
			}
			var remote Item
			if err = c.request(ctx, "GET", "/v1/items/credentials.json", "", "", nil, &remote); err != nil {
				return err
			}
			if known && hashData(local) != state.Local {
				if remote.Revision != state.Remote && !bytes.Equal(remote.Data, local) {
					return fmt.Errorf("credential sync conflict: unsent local credentials retained; do not repeat login/refresh until reconciled")
				}
				if err = c.request(ctx, "PUT", "/v1/items/credentials.json", lease, remote.Revision, Item{Data: local}, nil); err != nil {
					return err
				}
				remote = Item{Data: local, Revision: hashData(local)}
			}
			if remote.Data == nil {
				if err = c.request(ctx, "PUT", "/v1/items/credentials.json", lease, remote.Revision, Item{Data: local}, nil); err != nil {
					return err
				}
				remote = Item{Data: local, Revision: hashData(local)}
			} else {
				if !known && !bytes.Equal(local, remote.Data) {
					if err = replaceFile(c.statePath("credentials.backup"), local); err != nil {
						return err
					}
				}
				if err = s.ImportCredentials(ctx, remote.Data); err != nil {
					return err
				}
			}
			local, err = s.ExportCredentials(ctx)
			if err != nil {
				return err
			}
			if err = c.saveState("credentials", fileState{Remote: remote.Revision, Local: hashData(local)}); err != nil {
				return err
			}
			operationErr := run(ctx)
			updated, err := s.ExportCredentials(ctx)
			if err != nil {
				return errors.Join(operationErr, err)
			}
			if !bytes.Equal(updated, local) {
				if err = c.request(ctx, "PUT", "/v1/items/credentials.json", lease, remote.Revision, Item{Data: updated}, nil); err != nil {
					return errors.Join(operationErr, fmt.Errorf("local credentials saved but upload failed (retained for recovery): %w", err))
				}
				if err = c.saveState("credentials", fileState{Remote: hashData(updated), Local: hashData(updated)}); err != nil {
					return errors.Join(operationErr, err)
				}
			}
			return operationErr
		})
	})
}

// pullCredentials adopts a newer hosted snapshot during idle reconciliation.
// It holds no lease: inference never needs one, and an idle session must not
// block the machine that is actually rotating a token. Unsent local state is
// left alone and reported, so a pull can never discard a pending rotation.
func (c *Client) pullCredentials(ctx context.Context, s *ai.SQLiteAuthStorage) error {
	local, err := s.ExportCredentials(ctx)
	if err != nil {
		return err
	}
	state, known, err := c.loadState("credentials")
	if err != nil {
		return err
	}
	if known && hashData(local) != state.Local {
		return fmt.Errorf("credential sync paused: unsent local credentials are waiting to be published")
	}
	var remote Item
	if err = c.request(ctx, "GET", "/v1/items/credentials.json", "", "", nil, &remote); err != nil {
		return err
	}
	if remote.Data == nil || remote.Revision == state.Remote {
		return nil
	}
	if err = s.ImportCredentials(ctx, remote.Data); err != nil {
		return err
	}
	local, err = s.ExportCredentials(ctx)
	if err != nil {
		return err
	}
	return c.saveState("credentials", fileState{Remote: remote.Revision, Local: hashData(local)})
}

// RunSync periodically reconciles preferences and credentials between local
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
			store, err := ai.OpenCredentialStore(c.dir)
			if err == nil {
				err = c.syncCredentials(ctx, store, nil)
				store.Close()
			}
			if err != nil {
				report(err)
			}
		}
	}
}

// Start wires background reconciliation and an idempotent exit flush into the
// CLI. With neither saved login nor environment URL there is no network work
// and no exit hook. A configured but unusable service is reported on stderr and
// startup continues local-only: a down laptop connection must never make
// `--version`, `/login` or the repair path itself unreachable. Leaving the sync
// callback uninstalled keeps credential writes local, and the first successful
// reconnect reports a divergence as a conflict instead of overwriting it.
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
