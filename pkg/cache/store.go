package cache

import (
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/gohugoio/hashstructure"
)

type Store struct {
	identities map[string]uint64
	groups     map[string]uint64
	lastSet    time.Time
	mu         sync.RWMutex
}

func NewStore() *Store {
	return &Store{
		identities: make(map[string]uint64),
		groups:     make(map[string]uint64),
	}
}

func (s *Store) IsFresh() bool {
	return s.lastSet.IsZero()
}

func (s *Store) GetIdentityHash(key string) (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash, ok := s.identities[key]
	return hash, ok
}

func (s *Store) UpdateIdentities(identities map[string]source.Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastSet = time.Now()
	s.identities = make(map[string]uint64, len(identities))
	for key, identity := range identities {
		hash, err := hashstructure.Hash(identity, nil)
		if err != nil {
			return err
		}
		s.identities[key] = hash
	}
	return nil
}

func (s *Store) GetGroupHash(key string) (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash, ok := s.groups[key]
	return hash, ok
}

func (s *Store) UpdateGroups(groups map[string]source.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.groups = make(map[string]uint64, len(groups))
	for key, group := range groups {
		hash, err := hashstructure.Hash(group, nil)
		if err != nil {
			return err
		}
		s.groups[key] = hash
	}
	return nil
}

func (s *Store) GetIdentityKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.identities))
	for key := range s.identities {
		keys = append(keys, key)
	}
	return keys
}

func (s *Store) GetGroupKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.groups))
	for key := range s.groups {
		keys = append(keys, key)
	}
	return keys
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastSet = time.Time{}
	s.identities = make(map[string]uint64)
	s.groups = make(map[string]uint64)
}

func IdentityKey(identity source.Identity) string {
	if identity.UID != "" {
		return identity.UID
	}
	if identity.Username != "" {
		return identity.Username
	}
	return identity.Email
}

func GroupKey(group source.Group) string {
	if group.GID != "" {
		return group.GID
	}
	return group.Name
}
