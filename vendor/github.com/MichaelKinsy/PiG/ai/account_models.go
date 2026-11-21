package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
)

// accountModelCatalog holds credential-scoped live model rosters per endpoint:
// the account discovery shared by Codex and Devin. Each provider supplies how
// its endpoint normalizes, which credential fields identify an account, which
// models are built in, and how a roster is fetched.
type accountModelCatalog struct {
	providerID string
	// scopeLabel names the roster kind inside scope ids, which persist as
	// store keys and so must stay stable.
	scopeLabel string
	endpoint   func(baseURL string) (string, error)
	// identity is a stable, non-secret account key; false when the credential
	// has none and the scope falls back to the access token.
	identity func(Credential) (string, bool)
	builtin  func(modelID string) bool
	mu       sync.RWMutex
	rosters  map[string]accountModelRoster
}

type accountModelRoster struct {
	endpoint string
	models   []*Model
}

// accountModelFetch fetches the live roster of one credential; etag may be "".
type accountModelFetch func(ctx context.Context, baseURL string, credential *Credential) (models []*Model, etag string, err error)

// withAccountModelDiscovery adds account-scoped live roster refresh to a
// provider. Its existing transport and static catalog remain the trusted
// baseline.
func withAccountModelDiscovery(provider *ModelsProvider, catalog *accountModelCatalog, fetch accountModelFetch) *ModelsProvider {
	catalog.rosters = map[string]accountModelRoster{}
	baseGetModels := provider.GetModels
	baseGetAllModels := provider.GetAllModels
	baseFilter := provider.FilterModels
	baseFilterAll := provider.FilterAllModels
	provider.ModelCatalog = catalog
	provider.GetModels = func() ([]*Model, error) {
		rosters := catalog.Snapshots(provider.BaseURL)
		if len(rosters) == 0 {
			if baseGetModels == nil {
				return nil, nil
			}
			return baseGetModels()
		}
		return accountRosterUnion(rosters), nil
	}
	provider.GetAllModels = func() ([]AnyModel, error) {
		var all []AnyModel
		if baseGetAllModels != nil {
			var err error
			all, err = baseGetAllModels()
			if err != nil {
				return nil, err
			}
		} else if baseGetModels != nil {
			chat, err := baseGetModels()
			if err != nil {
				return nil, err
			}
			all = AnyModels(chat)
		}
		rosters := catalog.Snapshots(provider.BaseURL)
		if len(rosters) == 0 {
			return all, nil
		}
		all = slices.DeleteFunc(all, func(model AnyModel) bool { return IsModelType(model, ModelTypeChat) })
		return append(all, AnyModels(accountRosterUnion(rosters))...), nil
	}
	provider.FilterModels = func(models []*Model, credential *Credential) []*Model {
		scope := catalog.Scope(provider.BaseURL, credential)
		for _, roster := range catalog.Snapshots(provider.BaseURL) {
			if roster.ScopeID == scope.ID {
				if baseFilter != nil {
					return baseFilter(roster.Models, credential)
				}
				return roster.Models
			}
		}
		if baseGetModels != nil {
			if baseline, err := baseGetModels(); err == nil {
				if baseFilter != nil {
					return baseFilter(baseline, credential)
				}
				return baseline
			}
		}
		if baseFilter != nil {
			return baseFilter(models, credential)
		}
		return models
	}
	provider.FilterAllModels = func(models []AnyModel, credential *Credential) []AnyModel {
		if baseFilterAll != nil {
			models = baseFilterAll(models, credential)
		}
		chat, err := provider.GetModels()
		if err != nil {
			chat = nil
		}
		if provider.FilterModels != nil {
			chat = provider.FilterModels(chat, credential)
		}
		filtered := make([]AnyModel, 0, len(models)+len(chat))
		for _, model := range models {
			if !IsModelType(model, ModelTypeChat) {
				filtered = append(filtered, model)
			}
		}
		return append(filtered, AnyModels(chat)...)
	}
	provider.RefreshModels = func(refresh RefreshModelsContext) error {
		signal := refresh.Signal
		if signal == nil {
			signal = context.Background()
		}
		scope := catalog.Scope(refresh.BaseURL, refresh.Credential)
		if scope.ID == "" {
			return nil
		}
		publishRoster := func(models []*Model, persist *ModelsStoreEntry) error {
			update := func() { catalog.store(refresh.BaseURL, scope.ID, models) }
			if refresh.Publish == nil {
				update()
				return nil
			}
			_, err := refresh.Publish(ModelsPublication{Persist: persist, Update: update})
			return err
		}
		if refresh.Stored != nil {
			models, err := decodeModelsCatalog(refresh.Stored.Models, provider.ID)
			if err != nil {
				return err
			}
			if err := publishRoster(chatModels(models), nil); err != nil {
				return err
			}
		}
		if !refresh.AllowNetwork || signal.Err() != nil || refresh.Credential.Type != CredentialOAuth {
			return nil
		}
		models, etag, err := fetch(signal, refresh.BaseURL, refresh.Credential)
		if err != nil {
			return err
		}
		if signal.Err() != nil {
			return context.Cause(signal)
		}
		entry := &ModelsStoreEntry{Models: AnyModels(models), CheckedAt: new(float64(time.Now().UnixMilli())), ETag: etag}
		return publishRoster(models, entry)
	}
	return provider
}

// endpointID is the digest rosters record their endpoint under.
func (c *accountModelCatalog) endpointID(baseURL string) (string, bool) {
	endpoint, err := c.endpoint(baseURL)
	if err != nil {
		return "", false
	}
	return accountModelsDigest(c.providerID, endpoint), true
}

func (c *accountModelCatalog) Scope(baseURL string, credential *Credential) ModelsCatalogScope {
	if credential == nil || credential.Type != CredentialOAuth {
		return ModelsCatalogScope{}
	}
	endpointID, ok := c.endpointID(baseURL)
	if !ok {
		return ModelsCatalogScope{}
	}
	identity, stable := c.identity(*credential)
	if !stable {
		if credential.Access == "" {
			return ModelsCatalogScope{}
		}
		encoded, _ := json.Marshal([]string{"ephemeral-access-token", credential.Access})
		identity = string(encoded)
	}
	id := c.providerID + ":" + c.scopeLabel + ":" + accountModelsDigest(c.providerID, endpointID+"\x00"+identity)
	scope := ModelsCatalogScope{ID: id}
	if stable {
		scope.StoreKey = id
	}
	return scope
}

func (c *accountModelCatalog) Snapshots(baseURL string) []ModelsCatalogSnapshot {
	endpointID, ok := c.endpointID(baseURL)
	if !ok {
		return nil
	}
	c.mu.RLock()
	var snapshots []ModelsCatalogSnapshot
	for scopeID, roster := range c.rosters {
		if roster.endpoint == endpointID {
			snapshots = append(snapshots, ModelsCatalogSnapshot{ScopeID: scopeID, Models: cloneAccountModels(roster.models)})
		}
	}
	c.mu.RUnlock()
	slices.SortFunc(snapshots, func(a, b ModelsCatalogSnapshot) int { return strings.Compare(a.ScopeID, b.ScopeID) })
	return snapshots
}

func (c *accountModelCatalog) RetainScopes(baseURL string, scopeIDs []string) {
	endpointID, ok := c.endpointID(baseURL)
	if !ok {
		return
	}
	keep := make(map[string]struct{}, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		keep[scopeID] = struct{}{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for scopeID, roster := range c.rosters {
		if roster.endpoint == endpointID {
			if _, ok := keep[scopeID]; !ok {
				delete(c.rosters, scopeID)
			}
		}
	}
}

func (c *accountModelCatalog) ModelAvailability(baseURL string, credential *Credential, modelID string) (known, available bool) {
	scope := c.Scope(baseURL, credential)
	if scope.ID == "" {
		return false, true
	}
	endpointID, ok := c.endpointID(baseURL)
	if !ok {
		return false, true
	}
	c.mu.RLock()
	if roster, ok := c.rosters[scope.ID]; ok && roster.endpoint == endpointID {
		available := containsAccountModel(roster.models, modelID)
		c.mu.RUnlock()
		return true, available
	}
	discovered := false
	for _, roster := range c.rosters {
		if roster.endpoint == endpointID && containsAccountModel(roster.models, modelID) {
			discovered = true
			break
		}
	}
	c.mu.RUnlock()
	if discovered && !c.builtin(modelID) {
		return true, false
	}
	return false, true
}

func (c *accountModelCatalog) store(baseURL, scopeID string, models []*Model) {
	endpointID, ok := c.endpointID(baseURL)
	if !ok || scopeID == "" {
		return
	}
	c.mu.Lock()
	c.rosters[scopeID] = accountModelRoster{endpoint: endpointID, models: cloneAccountModels(models)}
	c.mu.Unlock()
}

func cloneAccountModels(models []*Model) []*Model {
	copied := ModelsStoreEntry{Models: AnyModels(models)}.Clone()
	return chatModels(copied.Models)
}

func accountRosterUnion(rosters []ModelsCatalogSnapshot) []*Model {
	models := []*Model{}
	seen := map[string]struct{}{}
	for _, roster := range rosters {
		for _, model := range roster.Models {
			if _, exists := seen[model.ID]; exists {
				continue
			}
			seen[model.ID] = struct{}{}
			models = append(models, model)
		}
	}
	return models
}

func containsAccountModel(models []*Model, modelID string) bool {
	return slices.ContainsFunc(models, func(model *Model) bool { return model.ID == modelID })
}

func accountModelsDigest(providerID, identity string) string {
	digest := sha256.Sum256([]byte(providerID + "\x00" + identity))
	return hex.EncodeToString(digest[:])
}
