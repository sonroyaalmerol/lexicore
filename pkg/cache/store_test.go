package cache

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
)

func TestStore_UpdateAndGetIdentities(t *testing.T) {
	store := NewStore("test")

	identities := []source.Identity{
		{
			UID:      "1",
			Username: "user1",
			Email:    "user1@example.com",
		},
		{
			UID:      "2",
			Username: "user2",
			Email:    "user2@example.com",
		},
	}

	store.UpdateIdentities(identities)

	retrieved := store.GetIdentities()
	assert.Len(t, retrieved, 2)

	identity, exists := store.GetIdentity("1")
	assert.True(t, exists)
	assert.Equal(t, "user1", identity.Username)
}

func TestStore_UpdateAndGetGroups(t *testing.T) {
	store := NewStore("test")

	groups := []source.Group{
		{
			GID:  "1",
			Name: "admins",
		},
		{
			GID:  "2",
			Name: "users",
		},
	}

	store.UpdateGroups(groups)

	retrieved := store.GetGroups()
	assert.Len(t, retrieved, 2)

	group, exists := store.GetGroup("1")
	assert.True(t, exists)
	assert.Equal(t, "admins", group.Name)
}

func TestStore_Clear(t *testing.T) {
	store := NewStore("test")

	identities := []source.Identity{{UID: "1", Username: "user1"}}
	groups := []source.Group{{GID: "1", Name: "admins"}}

	store.UpdateIdentities(identities)
	store.UpdateGroups(groups)

	assert.Len(t, store.GetIdentities(), 1)
	assert.Len(t, store.GetGroups(), 1)

	store.Clear()

	assert.Len(t, store.GetIdentities(), 0)
	assert.Len(t, store.GetGroups(), 0)
}
