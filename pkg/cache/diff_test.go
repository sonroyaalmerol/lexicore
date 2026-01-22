package cache

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
)

func TestCalculateDiff_IdentitiesCreate(t *testing.T) {
	old := []source.Identity{}
	new := []source.Identity{
		{UID: "1", Username: "user1", Email: "user1@example.com"},
		{UID: "2", Username: "user2", Email: "user2@example.com"},
	}

	diff := CalculateDiff(old, new, []source.Group{}, []source.Group{})

	assert.Len(t, diff.IdentitiesToCreate, 2)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_IdentitiesUpdate(t *testing.T) {
	old := []source.Identity{
		{UID: "1", Username: "user1", Email: "old@example.com"},
	}
	new := []source.Identity{
		{UID: "1", Username: "user1", Email: "new@example.com"},
	}

	diff := CalculateDiff(old, new, []source.Group{}, []source.Group{})

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 1)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_IdentitiesDelete(t *testing.T) {
	old := []source.Identity{
		{UID: "1", Username: "user1", Email: "user1@example.com"},
		{UID: "2", Username: "user2", Email: "user2@example.com"},
	}
	new := []source.Identity{
		{UID: "1", Username: "user1", Email: "user1@example.com"},
	}

	diff := CalculateDiff(old, new, []source.Group{}, []source.Group{})

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 1)
	assert.Equal(t, "2", diff.IdentitiesToDelete[0].UID)
	assert.True(t, diff.HasChanges())
}

func TestCalculateDiff_NoChanges(t *testing.T) {
	identities := []source.Identity{
		{UID: "1", Username: "user1", Email: "user1@example.com"},
	}

	diff := CalculateDiff(identities, identities, []source.Group{}, []source.Group{})

	assert.Len(t, diff.IdentitiesToCreate, 0)
	assert.Len(t, diff.IdentitiesToUpdate, 0)
	assert.Len(t, diff.IdentitiesToDelete, 0)
	assert.False(t, diff.HasChanges())
}

func TestCalculateDiff_Groups(t *testing.T) {
	old := []source.Group{
		{GID: "1", Name: "admins"},
	}
	new := []source.Group{
		{GID: "1", Name: "admins"},
		{GID: "2", Name: "users"},
	}

	diff := CalculateDiff([]source.Identity{}, []source.Identity{}, old, new)

	assert.Len(t, diff.GroupsToCreate, 1)
	assert.Equal(t, "users", diff.GroupsToCreate[0].Name)
}
