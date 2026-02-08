package cache

import (
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/cespare/xxhash/v2"
)

type Snapshot struct {
	Identities   map[string]*CachedIdentity
	Groups       map[string]*CachedGroup
	Version      int64 // Monotonic version counter
	CapturedAt   time.Time
	SourceDigest uint64 // Overall hash of the source state
}

type CachedIdentity struct {
	Data         source.Identity
	Hash         uint64
	LastModified time.Time
	Version      int64
}

type CachedGroup struct {
	Data         source.Group
	Hash         uint64
	LastModified time.Time
	Version      int64
}

type Store struct {
	current  *Snapshot
	previous *Snapshot
	version  int64
	mu       sync.RWMutex
}

func NewStore() *Store {
	return &Store{
		current: &Snapshot{
			Identities: make(map[string]*CachedIdentity),
			Groups:     make(map[string]*CachedGroup),
			Version:    0,
			CapturedAt: time.Time{},
		},
		version: 0,
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

	HasChanges    bool
	TotalChanges  int
	SnapshotDelta int64
}

func (s *Store) UpdateSnapshot(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (*Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.version++

	newSnapshot := &Snapshot{
		Identities:   make(map[string]*CachedIdentity, len(identities)),
		Groups:       make(map[string]*CachedGroup, len(groups)),
		Version:      s.version,
		CapturedAt:   now,
		SourceDigest: 0,
	}

	diff := &Diff{
		IdentitiesToCreate: make([]*source.Identity, 0),
		IdentitiesToUpdate: make([]*source.Identity, 0),
		IdentitiesToDelete: make([]*source.Identity, 0),
		GroupsToCreate:     make([]*source.Group, 0),
		GroupsToUpdate:     make([]*source.Group, 0),
		GroupsToDelete:     make([]*source.Group, 0),
	}

	seenIdentities := make(map[string]bool, len(identities))
	seenGroups := make(map[string]bool, len(groups))

	for key, identity := range identities {
		seenIdentities[key] = true

		hash, err := ComputeHash(identity)
		if err != nil {
			return nil, err
		}

		newSnapshot.Identities[key] = &CachedIdentity{
			Data:         identity,
			Hash:         hash,
			LastModified: now,
			Version:      s.version,
		}

		if cached, exists := s.current.Identities[key]; exists {
			if cached.Hash != hash {
				identityCopy := identity
				diff.IdentitiesToUpdate = append(diff.IdentitiesToUpdate, &identityCopy)
			}
			// else: no change, skip
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

	// Process groups
	for key, group := range groups {
		seenGroups[key] = true

		hash, err := ComputeHash(group)
		if err != nil {
			return nil, err
		}

		newSnapshot.Groups[key] = &CachedGroup{
			Data:         group,
			Hash:         hash,
			LastModified: now,
			Version:      s.version,
		}

		if cached, exists := s.current.Groups[key]; exists {
			if cached.Hash != hash {
				// Group was modified
				groupCopy := group
				diff.GroupsToUpdate = append(diff.GroupsToUpdate, &groupCopy)
			}
		} else {
			// New group
			groupCopy := group
			diff.GroupsToCreate = append(diff.GroupsToCreate, &groupCopy)
		}
	}

	// Detect deleted groups
	for key, cached := range s.current.Groups {
		if !seenGroups[key] {
			diff.GroupsToDelete = append(diff.GroupsToDelete, &cached.Data)
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
		len(diff.GroupsToDelete)

	diff.HasChanges = diff.TotalChanges > 0
	diff.SnapshotDelta = s.version - s.current.Version

	// Rotate snapshots
	s.previous = s.current
	s.current = newSnapshot

	return diff, nil
}

func (s *Store) GetCurrentVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Version
}

func (s *Store) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.Version == 0
}

func (s *Store) GetSnapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &Snapshot{
		Identities:   s.current.Identities,
		Groups:       s.current.Groups,
		Version:      s.current.Version,
		CapturedAt:   s.current.CapturedAt,
		SourceDigest: s.current.SourceDigest,
	}
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = &Snapshot{
		Identities: make(map[string]*CachedIdentity),
		Groups:     make(map[string]*CachedGroup),
		Version:    0,
		CapturedAt: time.Time{},
	}
	s.previous = nil
	s.version = 0
}

func (s *Store) GetStats() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return CacheStats{
		IdentityCount:  len(s.current.Identities),
		GroupCount:     len(s.current.Groups),
		CurrentVersion: s.current.Version,
		LastUpdate:     s.current.CapturedAt,
		HasPrevious:    s.previous != nil,
	}
}

type CacheStats struct {
	IdentityCount  int
	GroupCount     int
	CurrentVersion int64
	LastUpdate     time.Time
	HasPrevious    bool
}
