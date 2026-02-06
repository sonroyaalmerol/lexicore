package cache

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateDiff_IdentitiesCreate(t *testing.T) {
	store := NewStore()
	new := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
		"2": {UID: "2", Username: "user2", Email: "user2@example.com"},
	}

	diff, err := store.CalculateDiff(new, map[string]source.Group{})
	require.NoError(t, err)

	assert.Len(t, diff.IdentitiesToCreate, 2)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_IdentitiesUpdate(t *testing.T) {
	store := NewStore()
	old := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "old@example.com"},
	}
	err := store.UpdateIdentities(old)
	require.NoError(t, err)

	new := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "new@example.com"},
	}

	diff, err := store.CalculateDiff(new, map[string]source.Group{})
	require.NoError(t, err)

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 1)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_IdentitiesDelete(t *testing.T) {
	store := NewStore()
	old := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
		"2": {UID: "2", Username: "user2", Email: "user2@example.com"},
	}
	err := store.UpdateIdentities(old)
	require.NoError(t, err)

	new := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
	}

	diff, err := store.CalculateDiff(new, map[string]source.Group{})
	require.NoError(t, err)

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 1)
	assert.Equal(t, "2", diff.IdentitiesToDelete[0].UID)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_NoChanges(t *testing.T) {
	store := NewStore()
	identities := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
	}
	err := store.UpdateIdentities(identities)
	require.NoError(t, err)

	diff, err := store.CalculateDiff(identities, map[string]source.Group{})
	require.NoError(t, err)

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.False(t, diff.HasChanges())
}

func TestCalculateDiff_Groups(t *testing.T) {
	store := NewStore()
	old := map[string]source.Group{
		"1": {GID: "1", Name: "admins"},
	}
	err := store.UpdateGroups(old)
	require.NoError(t, err)

	new := map[string]source.Group{
		"1": {GID: "1", Name: "admins"},
		"2": {GID: "2", Name: "users"},
	}

	diff, err := store.CalculateDiff(map[string]source.Identity{}, new)
	require.NoError(t, err)

	assert.Len(t, diff.GroupsToCreate, 1)
	assert.Equal(t, "users", diff.GroupsToCreate[0].Name)
}
