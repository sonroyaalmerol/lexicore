package cache

import "codeberg.org/lexicore/lexicore/pkg/source"

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

func CalculateDiff(
	oldIdentities []source.Identity,
	newIdentities []source.Identity,
	oldGroups []source.Group,
	newGroups []source.Group,
) *Diff {
	diff := &Diff{}

	oldIdentityConstant := make(map[string]source.Identity)
	for _, identity := range oldIdentities {
		oldIdentityConstant[identityKey(identity)] = identity
	}

	newIdentityConstant := make(map[string]source.Identity)
	for _, identity := range newIdentities {
		key := identityKey(identity)
		newIdentityConstant[key] = identity

		if oldIdentity, exists := oldIdentityConstant[key]; exists {
			if needsIdentityUpdate(oldIdentity, identity) {
				diff.IdentitiesToUpdate = append(diff.IdentitiesToUpdate, identity)
			}
		} else {
			diff.IdentitiesToCreate = append(diff.IdentitiesToCreate, identity)
		}
	}

	for _, oldIdentity := range oldIdentities {
		if _, exists := newIdentityConstant[identityKey(oldIdentity)]; !exists {
			diff.IdentitiesToDelete = append(diff.IdentitiesToDelete, oldIdentity)
		}
	}

	oldGroupConstant := make(map[string]source.Group)
	for _, group := range oldGroups {
		oldGroupConstant[groupKey(group)] = group
	}

	newGroupConstant := make(map[string]source.Group)
	for _, group := range newGroups {
		key := groupKey(group)
		newGroupConstant[key] = group

		if oldGroup, exists := oldGroupConstant[key]; exists {
			if needsGroupUpdate(oldGroup, group) {
				diff.GroupsToUpdate = append(diff.GroupsToUpdate, group)
			}
		} else {
			diff.GroupsToCreate = append(diff.GroupsToCreate, group)
		}
	}

	for _, oldGroup := range oldGroups {
		if _, exists := newGroupConstant[groupKey(oldGroup)]; !exists {
			diff.GroupsToDelete = append(diff.GroupsToDelete, oldGroup)
		}
	}

	return diff
}

func needsIdentityUpdate(old, new source.Identity) bool {
	return HashIdentity(old) != HashIdentity(new)
}

func needsGroupUpdate(old, new source.Group) bool {
	return HashGroup(old) != HashGroup(new)
}
