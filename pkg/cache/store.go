package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Store struct {
	name       string
	identities map[string]source.Identity
	groups     map[string]source.Group
	mu         sync.RWMutex
}

func NewStore(name string) *Store {
	return &Store{
		name:       name,
		identities: make(map[string]source.Identity),
		groups:     make(map[string]source.Group),
	}
}

func (s *Store) GetIdentities() []source.Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	identities := make([]source.Identity, 0, len(s.identities))
	for _, identity := range s.identities {
		identities = append(identities, identity)
	}
	return identities
}

func (s *Store) GetIdentity(key string) (source.Identity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	identity, ok := s.identities[key]
	return identity, ok
}

func (s *Store) UpdateIdentities(identities []source.Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.identities = make(map[string]source.Identity)
	for _, identity := range identities {
		key := identityKey(identity)
		s.identities[key] = identity
	}
}

func (s *Store) GetGroups() []source.Group {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make([]source.Group, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}
	return groups
}

func (s *Store) GetGroup(key string) (source.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.groups[key]
	return group, ok
}

func (s *Store) UpdateGroups(groups []source.Group) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.groups = make(map[string]source.Group)
	for _, group := range groups {
		key := groupKey(group)
		s.groups[key] = group
	}
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.identities = make(map[string]source.Identity)
	s.groups = make(map[string]source.Group)
}

func identityKey(identity source.Identity) string {
	if identity.UID != "" {
		return identity.UID
	}
	if identity.Username != "" {
		return identity.Username
	}
	return identity.Email
}

func groupKey(group source.Group) string {
	if group.GID != "" {
		return group.GID
	}
	return group.Name
}

func HashIdentity(identity source.Identity) string {
	data, _ := json.Marshal(identity)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func HashGroup(group source.Group) string {
	data, _ := json.Marshal(group)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
