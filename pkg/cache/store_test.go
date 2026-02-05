package cache

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_UpdateAndGetIdentities(t *testing.T) {
	store := NewStore("test")

	identities := map[string]source.Identity{
		"1": {
			UID:      "1",
			Username: "user1",
			Email:    "user1@example.com",
		},
		"2": {
			UID:      "2",
			Username: "user2",
			Email:    "user2@example.com",
		},
	}

	err := store.UpdateIdentities(identities)
	require.NoError(t, err)

	keys := store.GetIdentityKeys()
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "1")
	assert.Contains(t, keys, "2")

	hash, exists := store.GetIdentityHash("1")
	assert.True(t, exists)
	assert.NotZero(t, hash)
}

func TestStore_UpdateAndGetGroups(t *testing.T) {
	store := NewStore("test")

	groups := map[string]source.Group{
		"1": {
			GID:  "1",
			Name: "admins",
		},
		"2": {
			GID:  "2",
			Name: "users",
		},
	}

	err := store.UpdateGroups(groups)
	require.NoError(t, err)

	keys := store.GetGroupKeys()
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "1")
	assert.Contains(t, keys, "2")

	hash, exists := store.GetGroupHash("1")
	assert.True(t, exists)
	assert.NotZero(t, hash)
}

func TestStore_Clear(t *testing.T) {
	store := NewStore("test")

	identities := map[string]source.Identity{
		"1": {UID: "1", Username: "user1"},
	}
	groups := map[string]source.Group{
		"1": {GID: "1", Name: "admins"},
	}

	err := store.UpdateIdentities(identities)
	require.NoError(t, err)
	err = store.UpdateGroups(groups)
	require.NoError(t, err)

	assert.Len(t, store.GetIdentityKeys(), 1)
	assert.Len(t, store.GetGroupKeys(), 1)

	store.Clear()

	assert.Len(t, store.GetIdentityKeys(), 0)
	assert.Len(t, store.GetGroupKeys(), 0)
}

func TestStore_HashConsistency(t *testing.T) {
	store := NewStore("test")

	identity := source.Identity{
		UID:      "1",
		Username: "user1",
		Email:    "user1@example.com",
	}

	identities := map[string]source.Identity{"1": identity}
	err := store.UpdateIdentities(identities)
	require.NoError(t, err)

	hash1, _ := store.GetIdentityHash("1")

	// Update with same data
	err = store.UpdateIdentities(identities)
	require.NoError(t, err)

	hash2, _ := store.GetIdentityHash("1")

	assert.Equal(t, hash1, hash2, "Hash should be consistent for identical data")
}
