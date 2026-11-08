// Package modelmetadata fetches the public model-metadata catalog that OMP reads (the
// stencil-pruned models.json, the same rows pi-catalog uses), keeps a durable snapshot, and resolves
// safe per-model patches for missing fields.
//
// Ports packages/coding-agent/src/core/model-metadata.ts, model-metadata-match.ts and
// model-metadata-provider.ts.
//
// Behavior mirrors upstream pi-catalog model-manager/model-cache:
//   - 2h TTL between revalidations, force bypasses it;
//   - ETag conditional GET, 304 refreshes only the freshness timestamp;
//   - a failed fetch or an offline start keeps the last snapshot and marks it stale;
//   - concurrent refreshes share one transport but every caller cancels independently;
//   - rows are parsed once at the network boundary and stored pruned.
//
// The snapshot is global (not per provider): a custom gateway id may match a row published by any
// provider, which is the whole point of fuzzy enrichment.
package modelmetadata

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/klauspost/compress/zstd"
)

const (
	// CatalogURL is the origin OMP reads (MODELS_DEV_URL in pi-catalog's openai-compat discovery).
	CatalogURL = "https://catalog.stencil.so/models.json.zstd"
	// TTL is the revalidation window; matches upstream DEFAULT_CACHE_TTL_MS.
	TTL = 2 * time.Hour
	// Retry is the failed-fetch retry window; matches upstream NON_AUTHORITATIVE_RETRY_MS.
	Retry = 5 * time.Minute
	// attemptTimeout bounds one request (ATTEMPT_TIMEOUT_MS).
	attemptTimeout = 4 * time.Second
	// FileName is the snapshot file, kept beside models-store.json in the agent directory.
	FileName = "models-metadata.json"
)

// Source is where resolved rows came from.
type Source string

// The metadata sources.
const (
	SourceNone    Source = "none"
	SourceBundled Source = "bundled"
	SourceCached  Source = "cached"
	SourceNetwork Source = "network"
)

// MetadataField names a model field a patch may fill, with individual cost rates spelled
// "cost.input" (ModelMetadataField of model-metadata.ts).
type MetadataField string

// The model fields a catalog patch may fill.
const (
	FieldInt              MetadataField = "int"
	FieldTPS              MetadataField = "tps"
	FieldDescription      MetadataField = "description"
	FieldCostInput        MetadataField = "cost.input"
	FieldCostOutput       MetadataField = "cost.output"
	FieldCostCacheRead    MetadataField = "cost.cacheRead"
	FieldCostCacheWrite   MetadataField = "cost.cacheWrite"
	FieldContextWindow    MetadataField = "contextWindow"
	FieldMaxTokens        MetadataField = "maxTokens"
	FieldReasoning        MetadataField = "reasoning"
	FieldThinkingLevelMap MetadataField = "thinkingLevelMap"
	FieldPromptCache      MetadataField = "promptCache"
)

// FieldSet is the set of model fields a models.json entry set explicitly; those never come from
// the catalog.
type FieldSet map[MetadataField]bool

// Has reports whether the entry pinned field.
func (s FieldSet) Has(field MetadataField) bool { return s[field] }

// Patch is the set of values a catalog row contributes to one model; nil members mean "the source
// has none" (ModelMetadataPatch of model-metadata.ts).
type Patch struct {
	Source           Source
	Provider         string
	ModelID          string
	Fields           []MetadataField
	Int              *float64
	TPS              *float64
	Description      *string
	Cost             *PatchCost
	ContextWindow    *int
	MaxTokens        *int
	Reasoning        *bool
	ThinkingLevelMap ai.ThinkingLevelMap
	PromptCache      ai.ModelPromptCache
}

// PatchCost is a partial rate set; a nil member is not inherited.
type PatchCost struct {
	Input      *float64
	Output     *float64
	CacheRead  *float64
	CacheWrite *float64
}

// RefreshResult is the outcome of one refresh attempt.
type RefreshResult struct {
	// Rows are the rows available after the attempt (never erased by a failure).
	Rows   []ai.ModelCatalogMetadataRow
	Source Source
	// Stale reports that the rows are older than the TTL or the last fetch failed.
	Stale     bool
	UpdatedAt *int64
	Error     string
}

// Store is a durable snapshot store; it mirrors the ModelsStore atomic-file convention.
type Store interface {
	Read(ctx context.Context) (*ai.ModelCatalogMetadataSnapshot, error)
	Write(ctx context.Context, snapshot ai.ModelCatalogMetadataSnapshot) error
}

// InMemoryStore keeps the snapshot in memory (tests and one-shot runtimes).
type InMemoryStore struct {
	mu       sync.Mutex
	snapshot *ai.ModelCatalogMetadataSnapshot
}

// NewInMemoryStore returns an empty in-memory snapshot store.
func NewInMemoryStore() *InMemoryStore { return &InMemoryStore{} }

// Read returns a copy of the stored snapshot.
func (s *InMemoryStore) Read(ctx context.Context) (*ai.ModelCatalogMetadataSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		return nil, nil
	}
	copied := cloneSnapshot(*s.snapshot)
	return &copied, nil
}

// Write replaces the stored snapshot.
func (s *InMemoryStore) Write(ctx context.Context, snapshot ai.ModelCatalogMetadataSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = new(cloneSnapshot(snapshot))
	return nil
}

// FileStore is the locked JSON snapshot beside models-store.json; one file, atomic read/write.
type FileStore struct {
	path string

	mu       sync.Mutex
	cached   *ai.ModelCatalogMetadataSnapshot
	revision fileRevision
}

// NewFileStore opens the snapshot at path.
func NewFileStore(path string) *FileStore {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return &FileStore{path: path}
}

// DefaultPath is the snapshot path for an agent directory; an empty directory keeps it in memory.
func DefaultPath(agentDir string) string {
	if agentDir == "" {
		return ""
	}
	return filepath.Join(agentDir, FileName)
}

// cloneSnapshot deep-copies a snapshot so callers cannot mutate stored rows.
func cloneSnapshot(snapshot ai.ModelCatalogMetadataSnapshot) ai.ModelCatalogMetadataSnapshot {
	copied := snapshot
	copied.Rows = make([]ai.ModelCatalogMetadataRow, len(snapshot.Rows))
	for i, row := range snapshot.Rows {
		copied.Rows[i] = cloneRow(row)
	}
	return copied
}

func cloneRow(row ai.ModelCatalogMetadataRow) ai.ModelCatalogMetadataRow {
	copied := row
	if row.ToolCall != nil {
		copied.ToolCall = new(*row.ToolCall)
	}
	if row.Int != nil {
		copied.Int = new(*row.Int)
	}
	if row.TPS != nil {
		copied.TPS = new(*row.TPS)
	}
	if row.Reasoning != nil {
		copied.Reasoning = new(*row.Reasoning)
	}
	copied.Input = append([]string(nil), row.Input...)
	copied.Efforts = append([]string(nil), row.Efforts...)
	if row.Cost != nil {
		cost := *row.Cost
		if row.Cost.Input != nil {
			cost.Input = new(*row.Cost.Input)
		}
		if row.Cost.Output != nil {
			cost.Output = new(*row.Cost.Output)
		}
		if row.Cost.CacheRead != nil {
			cost.CacheRead = new(*row.Cost.CacheRead)
		}
		if row.Cost.CacheWrite != nil {
			cost.CacheWrite = new(*row.Cost.CacheWrite)
		}
		copied.Cost = &cost
	}
	if row.ContextWindow != nil {
		copied.ContextWindow = new(*row.ContextWindow)
	}
	if row.MaxTokens != nil {
		copied.MaxTokens = new(*row.MaxTokens)
	}
	return copied
}

// Read restores the snapshot, validating every row at the disk trust boundary. A corrupt file is
// an empty catalog, not a startup failure.
func (s *FileStore) Read(ctx context.Context) (*ai.ModelCatalogMetadataSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	revision, found, err := fileRevisionOf(s.path)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if found && revision == s.revision {
		cached := s.cached
		s.mu.Unlock()
		if cached == nil {
			return nil, nil
		}
		copied := cloneSnapshot(*cached)
		return &copied, nil
	}
	s.mu.Unlock()
	var parsed *ai.ModelCatalogMetadataSnapshot
	err = withLockedFile(ctx, s.path, func(content []byte, exists bool) error {
		if !exists {
			parsed = nil
			return nil
		}
		snapshot, ok := ai.ParseModelCatalogMetadataSnapshot(content)
		if !ok {
			parsed = nil
			return nil
		}
		parsed = &snapshot
		return nil
	})
	if err != nil {
		return nil, err
	}
	revision, found, revisionErr := fileRevisionOf(s.path)
	s.mu.Lock()
	s.cached = parsed
	if revisionErr == nil && found {
		s.revision = revision
	} else {
		s.revision = fileRevision{}
	}
	s.mu.Unlock()
	if parsed == nil {
		return nil, nil
	}
	copied := cloneSnapshot(*parsed)
	return &copied, nil
}

// Write replaces the snapshot atomically under the file lock.
func (s *FileStore) Write(ctx context.Context, snapshot ai.ModelCatalogMetadataSnapshot) error {
	encoded, err := ai.MarshalModelCatalogMetadataSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("model metadata store: encode: %w", err)
	}
	if err := writeLockedFile(ctx, s.path, encoded); err != nil {
		return err
	}
	stored := cloneSnapshot(snapshot)
	revision, found, revisionErr := fileRevisionOf(s.path)
	s.mu.Lock()
	s.cached = &stored
	if revisionErr == nil && found {
		s.revision = revision
	}
	s.mu.Unlock()
	return nil
}

// Reference is a bundled model consulted for fields the public catalog omits (ModelCatalogReference).
type Reference struct {
	Provider         string
	ID               string
	Name             string
	Reasoning        bool
	ThinkingLevelMap ai.ThinkingLevelMap
	PromptCache      ai.ModelPromptCache
}

// Options configure a Service.
type Options struct {
	Store     Store
	URL       string
	Client    *http.Client
	Now       func() time.Time
	TTL       time.Duration
	Retry     time.Duration
	UserAgent string
	// References are bundled models that may declare data the public catalog does not publish
	// (cache lifetimes). Only declared values cross over; nothing is inferred.
	References func() []Reference
}

type inflightRefresh struct {
	cancel  context.CancelFunc
	done    chan struct{}
	result  RefreshResult
	err     error
	waiters int
}

// Service holds the current catalog rows and refreshes them on demand.
//
// Rows are read on first use and after every refresh; enrichment reads them synchronously so a
// provider's model listing never awaits.
type Service struct {
	store      Store
	url        string
	client     *http.Client
	now        func() time.Time
	ttl        time.Duration
	retry      time.Duration
	userAgent  string
	references func() []Reference

	mu          sync.Mutex
	rows        []ai.ModelCatalogMetadataRow
	index       Index
	generation  uint64
	updatedAt   *int64
	checkedAt   *int64
	etag        string
	source      Source
	lastError   string
	lastFailure *time.Time
	load        *sync.Once
	loadedDone  chan struct{}
	inflight    *inflightRefresh
}

// New builds a service with the documented defaults.
func New(options Options) *Service {
	service := &Service{
		store:      options.Store,
		url:        options.URL,
		client:     options.Client,
		now:        options.Now,
		ttl:        options.TTL,
		retry:      options.Retry,
		userAgent:  options.UserAgent,
		references: options.References,
		source:     SourceNone,
	}
	if service.store == nil {
		service.store = NewInMemoryStore()
	}
	if service.url == "" {
		service.url = CatalogURL
	}
	if service.now == nil {
		service.now = time.Now
	}
	if service.ttl <= 0 {
		service.ttl = TTL
	}
	if service.retry <= 0 {
		service.retry = Retry
	}
	if service.userAgent == "" {
		service.userAgent = "ompig-slim model-metadata"
	}
	if service.references == nil {
		service.references = func() []Reference { return nil }
	}
	service.index = buildIndex(nil)
	service.load = &sync.Once{}
	service.loadedDone = make(chan struct{})
	return service
}

// Rows returns the current rows; empty until Load or a refresh completes.
func (s *Service) Rows() []ai.ModelCatalogMetadataRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ai.ModelCatalogMetadataRow(nil), s.rows...)
}

// Source reports where the current rows came from.
func (s *Service) Source() Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.source
}

// Generation is bumped whenever the row set changes, so derived caches can invalidate cheaply.
func (s *Service) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

// Load restores the durable snapshot once; it is safe to call concurrently.
//
// The shared load is not bound to one caller's signal: a canceled first caller must not leave the
// service permanently empty. Each caller races its own context against the shared read, and the
// store read runs to completion.
func (s *Service) Load(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.load.Do(func() {
		go func() {
			defer close(s.loadedDone)
			snapshot, err := s.store.Read(context.WithoutCancel(ctx))
			if err != nil || snapshot == nil {
				return
			}
			s.mu.Lock()
			s.setRowsLocked(snapshot.Rows)
			fetchedAt, checkedAt := snapshot.FetchedAt, snapshot.CheckedAt
			s.updatedAt = &fetchedAt
			s.checkedAt = &checkedAt
			s.etag = snapshot.ETag
			// Schema 1 stores useful metrics but not discovery eligibility. Retain
			// those observations offline and force an unconditional schema-2 fetch
			// online; a 304 could otherwise leave new models undiscoverable forever.
			if snapshot.Version < ai.ModelCatalogMetadataVersion {
				stale := int64(0)
				s.checkedAt = &stale
				s.etag = ""
			}
			s.source = SourceCached
			s.mu.Unlock()
		}()
	})
	select {
	case <-s.loadedDone:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *Service) setRowsLocked(rows []ai.ModelCatalogMetadataRow) {
	s.rows = rows
	s.index = buildIndex(rows)
	s.generation++
}

func (s *Service) isStaleLocked(now time.Time) bool {
	if s.checkedAt == nil {
		return true
	}
	return now.Sub(time.UnixMilli(*s.checkedAt)) >= s.ttl
}

func (s *Service) shouldFetchLocked(now time.Time) bool {
	if s.lastFailure != nil && now.Sub(*s.lastFailure) < s.retry {
		return false
	}
	return s.isStaleLocked(now)
}

func (s *Service) resultLocked(overrides resultOverride) RefreshResult {
	source := s.source
	if overrides.source != nil {
		source = *overrides.source
	}
	return RefreshResult{
		Rows:      append([]ai.ModelCatalogMetadataRow(nil), s.rows...),
		Source:    source,
		Stale:     overrides.stale,
		UpdatedAt: s.updatedAt,
		Error:     s.lastError,
	}
}

type resultOverride struct {
	stale  bool
	source *Source
}

// RefreshOptions configure one refresh.
type RefreshOptions struct {
	// AllowNetwork false reports the current rows without touching the network (offline start).
	AllowNetwork *bool
	// Force bypasses the TTL and the failure window.
	Force bool
}

// Refresh refreshes the snapshot; offline, fresh, or failed attempts keep the current rows.
func (s *Service) Refresh(ctx context.Context, options RefreshOptions) (RefreshResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Load(ctx); err != nil {
		return RefreshResult{}, err
	}
	allowNetwork := true
	if options.AllowNetwork != nil {
		allowNetwork = *options.AllowNetwork
	}
	if !allowNetwork {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.resultLocked(resultOverride{stale: s.isStaleLocked(s.now())}), nil
	}
	active, created := s.acquireRefresh()
	if created {
		operationCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		active.cancel = cancel
		active.done = make(chan struct{})
		go s.runRefresh(operationCtx, active, options.Force)
	}
	return s.waitFor(ctx, active)
}

// acquireRefresh returns the in-flight refresh, starting bookkeeping for a new one.
func (s *Service) acquireRefresh() (*inflightRefresh, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight != nil {
		return s.inflight, false
	}
	active := &inflightRefresh{}
	s.inflight = active
	return active, true
}

// waitFor shares one transport while keeping each caller's cancellation independent. The request
// aborts only when its last waiter leaves, so one early caller cannot cancel a refresh the others
// are still waiting for.
func (s *Service) waitFor(ctx context.Context, active *inflightRefresh) (RefreshResult, error) {
	s.mu.Lock()
	active.waiters++
	s.mu.Unlock()
	select {
	case <-active.done:
		s.mu.Lock()
		active.waiters--
		result, err := active.result, active.err
		s.mu.Unlock()
		return result, err
	case <-ctx.Done():
		s.mu.Lock()
		active.waiters--
		if active.waiters == 0 && s.inflight == active {
			s.inflight = nil
			if active.cancel != nil {
				active.cancel()
			}
		}
		s.mu.Unlock()
		return RefreshResult{}, context.Cause(ctx)
	}
}

func (s *Service) runRefresh(ctx context.Context, active *inflightRefresh, force bool) {
	defer active.cancel()
	if err := s.Load(ctx); err != nil {
		s.finishRefresh(active, RefreshResult{}, err)
		return
	}
	now := s.now()
	s.mu.Lock()
	if !force && !s.shouldFetchLocked(now) {
		result := s.resultLocked(resultOverride{stale: s.isStaleLocked(now)})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	requestURL, etag, hasRows := s.url, s.etag, len(s.rows) > 0
	s.mu.Unlock()
	response, err := s.request(ctx, requestURL, etag, hasRows)
	if err != nil {
		if ctx.Err() != nil {
			// The last waiter left (or the service is closing). Not a source failure: no retry
			// backoff, and the previous rows stay current.
			s.mu.Lock()
			s.lastError = "metadata catalog refresh aborted"
			result := s.resultLocked(resultOverride{stale: s.isStaleLocked(s.now())})
			s.mu.Unlock()
			s.finishRefresh(active, result, nil)
			return
		}
		s.mu.Lock()
		failure := s.now()
		s.lastFailure = &failure
		s.lastError = err.Error()
		// Keep the previous rows; a failed refresh must never erase data.
		result := s.resultLocked(resultOverride{stale: true})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	defer response.Body.Close()
	checkedAt := s.now().UnixMilli()
	if response.StatusCode == http.StatusNotModified {
		// Unchanged rows: only freshness moves.
		s.mu.Lock()
		updatedAt := s.updatedAt
		s.mu.Unlock()
		if updatedAt != nil {
			if err := s.persist(context.WithoutCancel(ctx), checkedAt, *updatedAt); err != nil {
				s.finishRefresh(active, RefreshResult{}, err)
				return
			}
			s.mu.Lock()
			s.lastError, s.lastFailure, s.source = "", nil, SourceCached
			source := SourceCached
			result := s.resultLocked(resultOverride{stale: false, source: &source})
			s.mu.Unlock()
			s.finishRefresh(active, result, nil)
			return
		}
		// A 304 without a cached body cannot happen (no validator is sent), but treat it as an
		// empty success rather than as a failure.
		if err := s.persist(context.WithoutCancel(ctx), checkedAt, checkedAt); err != nil {
			s.finishRefresh(active, RefreshResult{}, err)
			return
		}
		s.mu.Lock()
		result := s.resultLocked(resultOverride{stale: false})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		s.mu.Lock()
		failure := s.now()
		s.lastFailure = &failure
		s.lastError = fmt.Sprintf("metadata catalog request failed: %d", response.StatusCode)
		result := s.resultLocked(resultOverride{stale: true})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	payload, err := readCatalogPayload(response.Body)
	if err != nil {
		s.mu.Lock()
		failure := s.now()
		s.lastFailure = &failure
		s.lastError = err.Error()
		result := s.resultLocked(resultOverride{stale: true})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	rows := ai.ParseModelCatalogMetadata(payload)
	if len(rows) == 0 {
		s.mu.Lock()
		failure := s.now()
		s.lastFailure = &failure
		s.lastError = "metadata catalog contained no usable rows"
		result := s.resultLocked(resultOverride{stale: true})
		s.mu.Unlock()
		s.finishRefresh(active, result, nil)
		return
	}
	s.mu.Lock()
	s.setRowsLocked(rows)
	s.source = SourceNetwork
	s.lastError = ""
	s.lastFailure = nil
	s.mu.Unlock()
	// The response already arrived: persist regardless of a late caller abort, so the download is
	// never wasted.
	if err := s.persist(context.WithoutCancel(ctx), checkedAt, checkedAt, response.Header.Get("ETag")); err != nil {
		s.finishRefresh(active, RefreshResult{}, err)
		return
	}
	s.mu.Lock()
	source := SourceNetwork
	result := s.resultLocked(resultOverride{stale: false, source: &source})
	s.mu.Unlock()
	s.finishRefresh(active, result, nil)
}

// readCatalogPayload accepts OMP's zstd snapshot and plain-JSON mirrors/tests.
// Sniff frame magic rather than HTTP headers, as upstream does. Bound both wire
// and decoded bodies to 64 MiB (well above the current catalog) so a malformed
// public response cannot allocate unlimited memory or erase a usable snapshot.
func readCatalogPayload(body io.Reader) ([]byte, error) {
	const limit = 64 << 20
	payload, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > limit {
		return nil, fmt.Errorf("metadata catalog exceeds %d bytes", limit)
	}
	if !bytes.HasPrefix(payload, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		return payload, nil
	}
	decoder, err := zstd.NewReader(bytes.NewReader(payload), zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(limit))
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	decoded, err := io.ReadAll(io.LimitReader(decoder, limit+1))
	if err != nil {
		return nil, err
	}
	if len(decoded) > limit {
		return nil, fmt.Errorf("decoded metadata catalog exceeds %d bytes", limit)
	}
	return decoded, nil
}

func (s *Service) finishRefresh(active *inflightRefresh, result RefreshResult, err error) {
	s.mu.Lock()
	if s.inflight == active {
		s.inflight = nil
	}
	active.result, active.err = result, err
	s.mu.Unlock()
	if active.done != nil {
		close(active.done)
	}
}

// persist writes the snapshot once its data is in memory.
func (s *Service) persist(ctx context.Context, checkedAt, fetchedAt int64, etags ...string) error {
	s.mu.Lock()
	s.checkedAt = &checkedAt
	s.updatedAt = &fetchedAt
	if len(etags) > 0 && etags[0] != "" {
		s.etag = etags[0]
	}
	snapshot := ai.ModelCatalogMetadataSnapshot{
		Version:   ai.ModelCatalogMetadataVersion,
		FetchedAt: fetchedAt,
		CheckedAt: checkedAt,
		ETag:      s.etag,
		Rows:      append([]ai.ModelCatalogMetadataRow(nil), s.rows...),
	}
	s.mu.Unlock()
	return s.store.Write(ctx, snapshot)
}

// request sends the conditional GET with the attempt timeout.
func (s *Service) request(ctx context.Context, url, etag string, hasRows bool) (*http.Response, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("User-Agent", s.userAgent)
	if etag != "" && hasRows {
		request.Header.Set("If-None-Match", etag)
	}
	client := s.client
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		cancel()
		return nil, err
	}
	// The timeout must span the body read, which the caller performs.
	response.Body = &cancelOnCloseReadCloser{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

// cancelOnCloseReadCloser releases the attempt's timeout when the body is closed.
type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}
