package cache

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/store"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

const (
	snapshotPrefix = "/lexicore.io/cache/snapshots"
	lockPrefix     = "/lexicore.io/cache/locks"
)

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
	return &Cache{
		etcd:     etcd,
		logger:   logger,
		cacheKey: cacheKey,
		current:  newEmptySnapshot(),
		version:  0,
	}
}

func (s *Cache) etcdKey() string {
	return fmt.Sprintf("%s/%s/current", snapshotPrefix, s.cacheKey)
}

func (s *Cache) lockKey() string {
	return fmt.Sprintf("%s/%s", lockPrefix, s.cacheKey)
}

func (s *Cache) UpdateSnapshot(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (*Diff, error) {
	return s.withLock(ctx, func() (*Diff, error) {
		return s.updateSnapshotLocked(identities, groups)
	})
}

func (s *Cache) withLock(ctx context.Context, fn func() (*Diff, error)) (*Diff, error) {
	session, err := concurrency.NewSession(s.etcd.Client(), concurrency.WithTTL(10))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	mutex := concurrency.NewMutex(session, s.lockKey())
	if err := mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock(ctx)

	if err := s.LoadFromEtcd(ctx); err != nil {
		s.logger.Warn("Failed to reload from etcd before update", zap.Error(err))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return fn()
}

func (s *Cache) updateSnapshotLocked(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (*Diff, error) {
	now := time.Now()
	s.version++

	newSnapshot := s.prepareSnapshot()
	newSnapshot.Version = s.version
	newSnapshot.CapturedAt = now

	diff := &Diff{
		IdentitiesToCreate:    make([]*source.Identity, 0),
		IdentitiesToUpdate:    make([]*source.Identity, 0),
		IdentitiesToDelete:    make([]*source.Identity, 0),
		GroupsToCreate:        make([]*source.Group, 0),
		GroupsToUpdate:        make([]*source.Group, 0),
		GroupsToDelete:        make([]*source.Group, 0),
		IdentitiesToReprocess: make([]*source.Identity, 0),
	}

	changedGroups := s.processGroups(newSnapshot, groups, now, diff)
	s.processIdentities(newSnapshot, identities, changedGroups, now, diff)
	s.detectDeletedGroups(groups, diff)
	s.detectDeletedIdentities(identities, diff)
	s.finalizeSnapshot(newSnapshot)
	diff.SnapshotDelta = s.version - s.current.Version
	diff.finalize()

	s.previous = s.current
	s.current = newSnapshot

	if err := s.saveToEtcdUnlocked(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to save snapshot to etcd: %w", err)
	}

	return diff, nil
}

func (s *Cache) prepareSnapshot() *Snapshot {
	snapshot := &Snapshot{
		Identities:      make(map[string]*CachedIdentity, len(s.current.Identities)),
		Groups:          make(map[string]*CachedGroup, len(s.current.Groups)),
		GroupMembership: make(map[string][]string),
		GroupMembers:    make(map[string][]string),
	}

	maps.Copy(snapshot.Identities, s.current.Identities)
	maps.Copy(snapshot.Groups, s.current.Groups)

	return snapshot
}

func (s *Cache) processGroups(
	snapshot *Snapshot,
	groups map[string]source.Group,
	now time.Time,
	diff *Diff,
) map[string]bool {
	changedGroups := make(map[string]bool)

	for key, group := range groups {
		hash, _ := ComputeHash(group)

		cached := &CachedGroup{
			Data:              group,
			Hash:              hash,
			LastModified:      now,
			Version:           s.version,
			AffectsIdentities: make([]string, 0),
		}

		if existing, exists := snapshot.Groups[key]; exists {
			if existing.Hash != hash {
				groupCopy := group
				diff.GroupsToUpdate = append(diff.GroupsToUpdate, &groupCopy)
				changedGroups[key] = true
			}
		} else {
			groupCopy := group
			diff.GroupsToCreate = append(diff.GroupsToCreate, &groupCopy)
			changedGroups[key] = true
		}

		snapshot.Groups[key] = cached
	}

	return changedGroups
}

func (s *Cache) processIdentities(
	snapshot *Snapshot,
	identities map[string]source.Identity,
	changedGroups map[string]bool,
	now time.Time,
	diff *Diff,
) {
	processedIdentities := make(map[string]bool)

	for key, identity := range identities {
		processedIdentities[key] = true
		hash, _ := ComputeHash(identity)

		cached := &CachedIdentity{
			Data:             identity,
			Hash:             hash,
			LastModified:     now,
			Version:          s.version,
			AffectedByGroups: identity.Groups,
		}

		if existing, exists := snapshot.Identities[key]; exists {
			if existing.Hash != hash {
				identityCopy := identity
				diff.IdentitiesToUpdate = append(diff.IdentitiesToUpdate, &identityCopy)
			} else if s.isIdentityAffectedByGroupChanges(identity, existing.Data, changedGroups) {
				identityCopy := identity
				diff.IdentitiesToReprocess = append(diff.IdentitiesToReprocess, &identityCopy)
				diff.AffectedByGroupChanges = true
			}
		} else {
			identityCopy := identity
			diff.IdentitiesToCreate = append(diff.IdentitiesToCreate, &identityCopy)
		}

		snapshot.Identities[key] = cached
	}
}

func (s *Cache) isIdentityAffectedByGroupChanges(
	newIdentity, oldIdentity source.Identity,
	changedGroups map[string]bool,
) bool {
	for _, groupKey := range newIdentity.Groups {
		if changedGroups[groupKey] {
			return true
		}
	}

	oldGroups := make(map[string]bool)
	for _, g := range oldIdentity.Groups {
		oldGroups[g] = true
	}

	newGroups := make(map[string]bool)
	for _, g := range newIdentity.Groups {
		newGroups[g] = true
	}

	if len(oldGroups) != len(newGroups) {
		return true
	}

	for g := range oldGroups {
		if !newGroups[g] {
			return true
		}
	}

	return false
}

func (s *Cache) detectDeletedGroups(
	groups map[string]source.Group,
	diff *Diff,
) {
	for key, cached := range s.current.Groups {
		if _, exists := groups[key]; !exists {
			diff.GroupsToDelete = append(diff.GroupsToDelete, &cached.Data)
		}
	}
}

func (s *Cache) detectDeletedIdentities(
	identities map[string]source.Identity,
	diff *Diff,
) {
	for key, cached := range s.current.Identities {
		if _, exists := identities[key]; !exists {
			diff.IdentitiesToDelete = append(diff.IdentitiesToDelete, &cached.Data)
		}
	}
}

func (s *Cache) finalizeSnapshot(snapshot *Snapshot) {
	snapshot.GroupMembership, snapshot.GroupMembers = buildDependencyGraph(
		snapshot.Identities,
		snapshot.Groups,
	)

	for groupKey, identityKeys := range snapshot.GroupMembers {
		if group, ok := snapshot.Groups[groupKey]; ok {
			group.AffectsIdentities = identityKeys
		}
	}

	snapshot.SourceDigest = computeSnapshotDigest(snapshot)
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

	mutex := concurrency.NewMutex(session, s.lockKey())
	if err := mutex.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer mutex.Unlock(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = newEmptySnapshot()
	s.previous = nil
	s.version = 0

	_, err = s.etcd.Client().Delete(ctx, s.etcdKey())
	if err != nil {
		return fmt.Errorf("failed to delete snapshot from etcd: %w", err)
	}

	return nil
}
