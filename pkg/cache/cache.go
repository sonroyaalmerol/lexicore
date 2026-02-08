package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/store"
	"github.com/cespare/xxhash/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

const (
	snapshotPrefix = "/lexicore.io/cache/snapshots"
	lockPrefix     = "/lexicore.io/cache/locks"
)

type Snapshot struct {
	Identities   map[string]*CachedIdentity
	Groups       map[string]*CachedGroup
	Version      int64
	CapturedAt   time.Time
	SourceDigest uint64

	GroupMembership map[string][]string // identity key -> group keys
	GroupMembers    map[string][]string // group key -> identity keys
}

type CachedIdentity struct {
	Data         source.Identity
	Hash         uint64
	LastModified time.Time
	Version      int64

	AffectedByGroups []string
}

type CachedGroup struct {
	Data         source.Group
	Hash         uint64
	LastModified time.Time
	Version      int64

	AffectsIdentities []string
}

type Cache struct {
	etcd     *store.EtcdStore
	logger   *zap.Logger
	current  *Snapshot
	previous *Snapshot
	version  int64
	mu       sync.RWMutex

	cacheKey string
}

func NewCache(etcd *store.EtcdStore, cacheKey string, logger *zap.Logger) *Cache {
	s := &Cache{
		etcd:     etcd,
		logger:   logger,
		cacheKey: cacheKey,
		current: &Snapshot{
			Identities:      make(map[string]*CachedIdentity),
			Groups:          make(map[string]*CachedGroup),
			GroupMembership: make(map[string][]string),
			GroupMembers:    make(map[string][]string),
			Version:         0,
			CapturedAt:      time.Time{},
		},
		version: 0,
	}

	return s
}

func (s *Cache) LoadFromEtcd(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s/%s/current", snapshotPrefix, s.cacheKey)
	resp, err := s.etcd.Client().Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	if len(resp.Kvs) == 0 {
		s.logger.Info("No existing snapshot found in etcd")
		return nil
	}

	var snapshot Snapshot
	if err := json.Unmarshal(resp.Kvs[0].Value, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	s.current = &snapshot
	s.version = snapshot.Version

	s.logger.Info("Loaded snapshot from etcd",
		zap.String("cache_key", s.cacheKey),
		zap.Int64("version", s.version),
		zap.Int("identities", len(s.current.Identities)),
		zap.Int("groups", len(s.current.Groups)))

	return nil
}

func (s *Cache) WatchSnapshots(ctx context.Context) {
	key := fmt.Sprintf("%s/%s/current", snapshotPrefix, s.cacheKey)

	// Get current revision
	resp, err := s.etcd.Client().Get(ctx, key)
	if err != nil {
		s.logger.Error("Failed to get current revision", zap.Error(err))
		return
	}

	rev := resp.Header.Revision + 1

	wch := s.etcd.Client().Watch(ctx, key, clientv3.WithRev(rev))

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping cache watch",
				zap.String("cache_key", s.cacheKey))
			return
		case resp := <-wch:
			if resp.Canceled {
				s.logger.Error("Cache watch canceled",
					zap.String("cache_key", s.cacheKey),
					zap.Error(resp.Err()))
				return
			}

			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					var snapshot Snapshot
					if err := json.Unmarshal(ev.Kv.Value, &snapshot); err != nil {
						s.logger.Error("Failed to unmarshal watched snapshot",
							zap.Error(err))
						continue
					}

					s.mu.Lock()
					// Only update if the watched version is newer
					if snapshot.Version > s.current.Version {
						s.previous = s.current
						s.current = &snapshot
						s.version = snapshot.Version

						s.logger.Info("Updated cache from watch",
							zap.String("cache_key", s.cacheKey),
							zap.Int64("version", s.version))
					}
					s.mu.Unlock()
				}
			}
		}
	}
}

func ComputeHash(data any) (uint64, error) {
	h := xxhash.New()

	switch v := data.(type) {
	case source.Identity:
		h.WriteString(v.UID)
		h.WriteString(v.Username)
		h.WriteString(v.Email)
		h.WriteString(v.DisplayName)
		if v.Disabled {
			h.WriteString("disabled")
		}
		for _, g := range v.Groups {
			h.WriteString(g)
		}
		for k, val := range v.Attributes {
			h.WriteString(k)
			if s, ok := val.(string); ok {
				h.WriteString(s)
			}
		}
	case source.Group:
		h.WriteString(v.GID)
		h.WriteString(v.Name)
		h.WriteString(v.Description)
		for _, m := range v.Members {
			h.WriteString(m)
		}
		for k, val := range v.Attributes {
			h.WriteString(k)
			if s, ok := val.(string); ok {
				h.WriteString(s)
			}
		}
	}

	return h.Sum64(), nil
}

type Diff struct {
	IdentitiesToCreate []*source.Identity
	IdentitiesToUpdate []*source.Identity
	IdentitiesToDelete []*source.Identity
	GroupsToCreate     []*source.Group
	GroupsToUpdate     []*source.Group
	GroupsToDelete     []*source.Group

	IdentitiesToReprocess []*source.Identity

	HasChanges    bool
	TotalChanges  int
	SnapshotDelta int64

	AffectedByGroupChanges bool
}

func buildDependencyGraph(
	identities map[string]*CachedIdentity,
	groups map[string]*CachedGroup,
) (map[string][]string, map[string][]string) {
	groupMembership := make(map[string][]string) // identity -> groups
	groupMembers := make(map[string][]string)    // group -> identities

	for identityKey, cached := range identities {
		validGroups := make([]string, 0, len(cached.Data.Groups))

		for _, groupKey := range cached.Data.Groups {
			if _, exists := groups[groupKey]; !exists {
				// Skip orphaned group reference
				continue
			}

			validGroups = append(validGroups, groupKey)

			groupMembers[groupKey] = append(groupMembers[groupKey], identityKey)
		}

		if len(validGroups) > 0 {
			groupMembership[identityKey] = validGroups
		}
	}

	return groupMembership, groupMembers
}

func (s *Cache) UpdateSnapshot(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (*Diff, error) {
	session, err := concurrency.NewSession(s.etcd.Client(), concurrency.WithTTL(10))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	lockKey := fmt.Sprintf("%s/%s", lockPrefix, s.cacheKey)
	mutex := concurrency.NewMutex(session, lockKey)

	if err := mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock(ctx)

	// Load latest from etcd before updating
	if err := s.LoadFromEtcd(ctx); err != nil {
		s.logger.Warn("Failed to reload from etcd before update",
			zap.Error(err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.version++

	newSnapshot := &Snapshot{
		Identities:      make(map[string]*CachedIdentity, len(identities)),
		Groups:          make(map[string]*CachedGroup, len(groups)),
		GroupMembership: make(map[string][]string),
		GroupMembers:    make(map[string][]string),
		Version:         s.version,
		CapturedAt:      now,
		SourceDigest:    0,
	}

	diff := &Diff{
		IdentitiesToCreate:    make([]*source.Identity, 0),
		IdentitiesToUpdate:    make([]*source.Identity, 0),
		IdentitiesToDelete:    make([]*source.Identity, 0),
		GroupsToCreate:        make([]*source.Group, 0),
		GroupsToUpdate:        make([]*source.Group, 0),
		GroupsToDelete:        make([]*source.Group, 0),
		IdentitiesToReprocess: make([]*source.Identity, 0),
	}

	seenIdentities := make(map[string]bool, len(identities))
	seenGroups := make(map[string]bool, len(groups))
	changedGroups := make(map[string]bool) // Track which groups changed

	// PHASE 1: Process groups first (they may affect identities)
	for key, group := range groups {
		seenGroups[key] = true

		hash, err := ComputeHash(group)
		if err != nil {
			return nil, err
		}

		newSnapshot.Groups[key] = &CachedGroup{
			Data:              group,
			Hash:              hash,
			LastModified:      now,
			Version:           s.version,
			AffectsIdentities: make([]string, 0),
		}

		if cached, exists := s.current.Groups[key]; exists {
			if cached.Hash != hash {
				// Group was modified
				groupCopy := group
				diff.GroupsToUpdate = append(diff.GroupsToUpdate, &groupCopy)
				changedGroups[key] = true
			}
		} else {
			// New group
			groupCopy := group
			diff.GroupsToCreate = append(diff.GroupsToCreate, &groupCopy)
			changedGroups[key] = true
		}
	}

	// Detect deleted groups
	for key, cached := range s.current.Groups {
		if !seenGroups[key] {
			diff.GroupsToDelete = append(diff.GroupsToDelete, &cached.Data)
			changedGroups[key] = true
		}
	}

	// PHASE 2: Process identities
	for key, identity := range identities {
		seenIdentities[key] = true

		hash, err := ComputeHash(identity)
		if err != nil {
			return nil, err
		}

		newSnapshot.Identities[key] = &CachedIdentity{
			Data:             identity,
			Hash:             hash,
			LastModified:     now,
			Version:          s.version,
			AffectedByGroups: identity.Groups,
		}

		if cached, exists := s.current.Identities[key]; exists {
			if cached.Hash != hash {
				// Identity was directly modified
				identityCopy := identity
				diff.IdentitiesToUpdate = append(diff.IdentitiesToUpdate, &identityCopy)
			} else {
				// Identity unchanged, but check if any of its groups changed
				identityAffected := false
				for _, groupKey := range identity.Groups {
					if changedGroups[groupKey] {
						identityAffected = true
						break
					}
				}

				// Also check if identity's group membership changed
				oldGroups := make(map[string]bool)
				for _, g := range cached.Data.Groups {
					oldGroups[g] = true
				}

				newGroups := make(map[string]bool)
				for _, g := range identity.Groups {
					newGroups[g] = true
				}

				membershipChanged := len(oldGroups) != len(newGroups)
				if !membershipChanged {
					for g := range oldGroups {
						if !newGroups[g] {
							membershipChanged = true
							break
						}
					}
				}

				if identityAffected || membershipChanged {
					// Identity needs reprocessing due to group changes
					identityCopy := identity
					diff.IdentitiesToReprocess = append(
						diff.IdentitiesToReprocess,
						&identityCopy,
					)
					diff.AffectedByGroupChanges = true
				}
			}
		} else {
			// New identity
			identityCopy := identity
			diff.IdentitiesToCreate = append(diff.IdentitiesToCreate, &identityCopy)
		}
	}

	// Detect deleted identities
	for key, cached := range s.current.Identities {
		if !seenIdentities[key] {
			diff.IdentitiesToDelete = append(diff.IdentitiesToDelete, &cached.Data)
		}
	}

	// PHASE 3: Build dependency graph
	newSnapshot.GroupMembership, newSnapshot.GroupMembers = buildDependencyGraph(
		newSnapshot.Identities,
		newSnapshot.Groups,
	)

	for groupKey, identityKeys := range newSnapshot.GroupMembers {
		if group, ok := newSnapshot.Groups[groupKey]; ok {
			group.AffectsIdentities = identityKeys
		}
	}

	// Compute overall snapshot digest
	digestHash := xxhash.New()
	for _, cached := range newSnapshot.Identities {
		digestHash.Write([]byte{byte(cached.Hash)})
	}
	for _, cached := range newSnapshot.Groups {
		digestHash.Write([]byte{byte(cached.Hash)})
	}
	newSnapshot.SourceDigest = digestHash.Sum64()

	// Update diff metadata
	diff.TotalChanges = len(diff.IdentitiesToCreate) +
		len(diff.IdentitiesToUpdate) +
		len(diff.IdentitiesToDelete) +
		len(diff.GroupsToCreate) +
		len(diff.GroupsToUpdate) +
		len(diff.GroupsToDelete) +
		len(diff.IdentitiesToReprocess)

	diff.HasChanges = diff.TotalChanges > 0
	diff.SnapshotDelta = s.version - s.current.Version

	// Rotate snapshots
	s.previous = s.current
	s.current = newSnapshot

	if err := s.saveToEtcdUnlocked(ctx); err != nil {
		return nil, fmt.Errorf("failed to save snapshot to etcd: %w", err)
	}

	return diff, nil
}

func (s *Cache) saveToEtcdUnlocked(ctx context.Context) error {
	data, err := json.Marshal(s.current)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	key := fmt.Sprintf("%s/%s/current", snapshotPrefix, s.cacheKey)
	_, err = s.etcd.Client().Put(ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

func (s *Cache) GetCurrentVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Version
}

func (s *Cache) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Version == 0
}

func (s *Cache) GetSnapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &Snapshot{
		Identities:      s.current.Identities,
		Groups:          s.current.Groups,
		Version:         s.current.Version,
		CapturedAt:      s.current.CapturedAt,
		SourceDigest:    s.current.SourceDigest,
		GroupMembership: s.current.GroupMembership,
		GroupMembers:    s.current.GroupMembers,
	}
}

func (s *Cache) Clear(ctx context.Context) error {
	session, err := concurrency.NewSession(s.etcd.Client(), concurrency.WithTTL(10))
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	lockKey := fmt.Sprintf("%s/%s", lockPrefix, s.cacheKey)
	mutex := concurrency.NewMutex(session, lockKey)

	if err := mutex.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = &Snapshot{
		Identities:      make(map[string]*CachedIdentity),
		Groups:          make(map[string]*CachedGroup),
		GroupMembership: make(map[string][]string),
		GroupMembers:    make(map[string][]string),
		Version:         0,
		CapturedAt:      time.Time{},
	}
	s.previous = nil
	s.version = 0

	key := fmt.Sprintf("%s/%s/current", snapshotPrefix, s.cacheKey)
	_, err = s.etcd.Client().Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot from etcd: %w", err)
	}

	return nil
}
