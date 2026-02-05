package cache

import (
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/gohugoio/hashstructure"
)

type Diff struct {
	IdentitiesToCreate []source.Identity
	IdentitiesToUpdate []source.Identity
	IdentitiesToDelete []source.Identity
	GroupsToCreate     []source.Group
	GroupsToUpdate     []source.Group
	GroupsToDelete     []source.Group
}

func (d *Diff) HasChanges() bool {
	return len(d.IdentitiesToCreate) > 0 ||
		len(d.IdentitiesToUpdate) > 0 ||
		len(d.IdentitiesToDelete) > 0 ||
		len(d.GroupsToCreate) > 0 ||
		len(d.GroupsToUpdate) > 0 ||
		len(d.GroupsToDelete) > 0
}

func (store *Store) CalculateDiff(
	newIdentities map[string]source.Identity,
	newGroups map[string]source.Group,
) (*Diff, error) {
	diff := &Diff{
		IdentitiesToCreate: make([]source.Identity, 0),
		IdentitiesToUpdate: make([]source.Identity, 0),
		IdentitiesToDelete: make([]source.Identity, 0),
		GroupsToCreate:     make([]source.Group, 0),
		GroupsToUpdate:     make([]source.Group, 0),
		GroupsToDelete:     make([]source.Group, 0),
	}

	seenIdentityKeys := make(map[string]bool, len(newIdentities))
	seenGroupKeys := make(map[string]bool, len(newGroups))

	for key, newIdentity := range newIdentities {
		seenIdentityKeys[key] = true
		newHash, err := hashstructure.Hash(newIdentity, nil)
		if err != nil {
			return nil, err
		}

		if oldHash, exists := store.GetIdentityHash(key); exists {
			if oldHash != newHash {
				diff.IdentitiesToUpdate = append(diff.IdentitiesToUpdate, newIdentity)
			}
		} else {
			diff.IdentitiesToCreate = append(diff.IdentitiesToCreate, newIdentity)
		}
	}

	for _, oldKey := range store.GetIdentityKeys() {
		if !seenIdentityKeys[oldKey] {
			diff.IdentitiesToDelete = append(diff.IdentitiesToDelete, source.Identity{
				UID: oldKey,
			})
		}
	}

	for key, newGroup := range newGroups {
		seenGroupKeys[key] = true
		newHash, err := hashstructure.Hash(newGroup, nil)
		if err != nil {
			return nil, err
		}

		if oldHash, exists := store.GetGroupHash(key); exists {
			if oldHash != newHash {
				diff.GroupsToUpdate = append(diff.GroupsToUpdate, newGroup)
			}
		} else {
			diff.GroupsToCreate = append(diff.GroupsToCreate, newGroup)
		}
	}

	for _, oldKey := range store.GetGroupKeys() {
		if !seenGroupKeys[oldKey] {
			diff.GroupsToDelete = append(diff.GroupsToDelete, source.Group{
				GID: oldKey,
			})
		}
	}

	return diff, nil
}
