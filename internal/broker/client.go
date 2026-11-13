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
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
// A preference failure returns the configured client alongside the error so its
// owner can retry; invalid connection configuration returns no client.
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
	return c, c.SyncFiles(ctx)
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
	// Hash nested theme names to keep ancestry flat and within filename limits,
	// including first-adoption backups. Existing preference paths stay intact.
	if strings.HasPrefix(name, "themes/") {
		name = "theme-" + hashData([]byte(name))
	}
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
// same file fail explicitly and leave both copies intact; unrelated files still
// reconcile. Deletions synchronize once a common ancestor has been recorded.
// Documents run concurrently under the same profile lock: their ancestry and
// paths are independent, so network RTT is paid once per phase, not per file.
// Theme discovery adds direct JSON files and remembered deletions to the fixed
// allowlist. Discovery failures do not prevent fixed documents from syncing.
// All failures are returned in file order after every document completes.
func (c *Client) SyncFiles(ctx context.Context) error {
	_, err := c.SyncFilesChanged(ctx)
	return err
}

// SyncFilesChanged is SyncFiles reporting which profile documents this call
// rewrote on disk, so a running session can re-read the configuration it
// already loaded. A name counts only when the file's bytes changed: an idle poll
// and a document this client just uploaded are both absent, which keeps a
// watcher from re-applying settings on every tick or echoing its own write.
func (c *Client) SyncFilesChanged(ctx context.Context) ([]string, error) {
	var changed []string
	err := c.locked(ctx, func() error {
		themes, themeErr := c.themeFiles(ctx)
		names := append(append([]string(nil), SharedFiles...), themes...)
		failures := make([]error, len(names)+1)
		if themeErr != nil {
			failures[len(names)] = fmt.Errorf("themes: %w", themeErr)
		}
		wrote := make([]bool, len(names))
		var workers sync.WaitGroup
		for i, name := range names {
			workers.Go(func() {
				if err := c.syncFile(ctx, name, &wrote[i]); err != nil {
					failures[i] = fmt.Errorf("%s: %w", name, err)
				}
			})
		}
		workers.Wait()
		for i, name := range names {
			if wrote[i] {
				changed = append(changed, name)
			}
		}
		return errors.Join(failures...)
	})
	return changed, err
}

// syncFile reconciles one allowlisted document and persists its ancestry only
// after successful data writes. changed reports that the hosted document replaced
// the local file, which a running session must re-read; an upload or an idle
// reconcile leaves it false because the bytes on disk are already what this client
// wrote. The caller holds the cross-process profile lock.
func (c *Client) syncFile(ctx context.Context, name string, changed *bool) error {
	local, err := readLocal(filepath.Join(c.dir, name))
	if err != nil {
		return err
	}
	state, known, err := c.loadState(name)
	if err != nil {
		return err
	}
	var remote Item
	if err = c.request(ctx, "GET", "/v1/items/"+url.PathEscape(name), "", nil, &remote); err != nil {
		return err
	}
	localHash := hashData(local)
	// The loaded ancestor is already durable; an idle poll must not
	// replace and fsync it again when neither side has changed.
	if known && localHash == state.Local && remote.Revision == state.Remote {
		return nil
	}
	if !known {
		if remote.Data != nil && !bytes.Equal(local, remote.Data) {
			if local != nil {
				if err = replaceFile(c.statePath(name+".backup"), local); err != nil {
					return err
				}
			}
			if err = publishDocument(c.dir, name, remote.Data); err != nil {
				return err
			}
			*changed = true
			local = remote.Data
		} else if remote.Data == nil && local != nil {
			if err = c.request(ctx, "PUT", "/v1/items/"+url.PathEscape(name), remote.Revision, Item{Data: local}, nil); err != nil {
				return err
			}
			remote.Data = local
			remote.Revision = localHash
		}
	} else {
		localChanged := localHash != state.Local
		remoteChanged := remote.Revision != state.Remote
		if localChanged && remoteChanged && !bytes.Equal(local, remote.Data) {
			return errors.New("sync conflict: local and hosted copies retained; reconcile them before retrying")
		}
		if localChanged && !bytes.Equal(local, remote.Data) {
			if err = c.request(ctx, "PUT", "/v1/items/"+url.PathEscape(name), remote.Revision, Item{Data: local}, nil); err != nil {
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
				err = publishDocument(c.dir, name, remote.Data)
			}
			if err != nil {
				return err
			}
			*changed = true
			local = remote.Data
		}
	}
	return c.saveState(name, fileState{Remote: remote.Revision, Local: hashData(local)})
}

// RunSyncChanged periodically reconciles preferences between local requests,
// reporting a poll that replaced documents on disk to notify before report sees
// that poll's error. Only a poll that rewrote a local file notifies: an idle tick
// and this client's own upload leave the bytes already what they were, and
// notifying on every tick would re-apply settings five seconds at a time.
//
// Report receives each poll's result (nil on success), without secrets, and is
// never called after return. Runtime settings/catalog reload behavior remains
// PiG's own: a new session/startup always sees the synchronized files, and a
// running session re-reads them through the notification.
func (c *Client) RunSyncChanged(ctx context.Context, report func(error), notify func()) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := c.SyncFilesChanged(ctx)
			if notify != nil && len(changed) > 0 {
				notify()
			}
			report(err)
		}
	}
}

// Start wires preference reconciliation and an idempotent exit flush into the
// CLI. A configured but unavailable broker leaves credential access remote-only:
// the CLI can repair the connection, but must not silently use a local vault.
// Recoverable preference/network failures keep retrying until shutdown; invalid
// bootstrap configuration has no usable client and requires repair and restart.
func Start(ctx context.Context, dir string, stderr io.Writer) (func() error, error) {
	finish, err := StartNotifying(ctx, dir, stderr, nil)
	return finish, err
}

// StartNotifying is Start reporting that a poll replaced profile documents on
// disk, so the caller can have a running session re-read its configuration.
//
// notify runs on the synchronization worker, never after the returned finish
// completed, and only for a poll that actually rewrote a local file. It is a
// signal, not a payload: the receiver re-reads the documents from disk, so a
// coalesced or repeated signal costs one redundant read and never a lost change.
// It must not block for long, because the poll that produced it is still holding
// the profile lock. Nil leaves the worker with nothing to report, which is what a
// non-interactive command wants.
func StartNotifying(ctx context.Context, dir string, stderr io.Writer, notify func()) (func() error, error) {
	startup, cancel := context.WithTimeout(ctx, 45*time.Second)
	client, err := Connect(startup, dir)
	cancel()
	if err != nil {
		if client == nil {
			fmt.Fprintln(stderr, "profile sync inactive:", err)
		} else {
			fmt.Fprintln(stderr, "profile sync pending (will retry):", err)
		}
	}
	if client == nil {
		return nil, nil
	}
	// Only the worker writes synced. The exit flush reads it after joining the
	// worker, preserving repair-command exit status until initial sync succeeds.
	synced := err == nil
	background, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		last := ""
		client.RunSyncChanged(background, func(err error) {
			if err == nil {
				synced = true
				last = ""
				return
			}
			if text := err.Error(); text != last {
				fmt.Fprintln(stderr, "profile sync:", text)
				last = text
			}
		}, notify)
	}()
	var once sync.Once
	var finalErr error
	return func() error {
		once.Do(func() {
			stop()
			<-done
			flush, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := client.SyncFiles(flush); synced {
				finalErr = err
			}
		})
		return finalErr
	}, nil
}
