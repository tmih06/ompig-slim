package ai

import "context"

// ModelsStoreSnapshotReader returns a caller-owned mutable snapshot. Stores
// implementing this optional contract already isolate their reads, so provider
// refresh need not serialize and decode the entire catalog a second time.
// Ordinary ModelsStore implementations retain defensive cloning.
type ModelsStoreSnapshotReader interface {
	ReadSnapshot(context.Context, string) (*ModelsStoreEntry, error)
}

// ReadModelsStoreSnapshot isolates one provider's mutable refresh input.
func ReadModelsStoreSnapshot(ctx context.Context, store ModelsStore, providerID string) (*ModelsStoreEntry, error) {
	if snapshots, ok := store.(ModelsStoreSnapshotReader); ok {
		return snapshots.ReadSnapshot(ctx, providerID)
	}
	entry, err := store.Read(ctx, providerID)
	if err != nil || entry == nil {
		return entry, err
	}
	return new(entry.Clone()), nil
}

// ReadSnapshot returns the fresh catalog produced by Read; no store-owned model
// or unknown-field data escapes the file decoder.
func (s *FileModelsStore) ReadSnapshot(ctx context.Context, providerID string) (*ModelsStoreEntry, error) {
	return s.Read(ctx, providerID)
}

// ReadSnapshot returns the isolated copy already produced by the memory store.
func (s *InMemoryModelsStore) ReadSnapshot(ctx context.Context, providerID string) (*ModelsStoreEntry, error) {
	return s.Read(ctx, providerID)
}
