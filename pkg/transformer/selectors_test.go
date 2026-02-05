package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectorTransformer_GroupSelector(t *testing.T) {
	config := map[string]any{
		"groupSelector": "admins",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1", Groups: []string{"admins", "users"}},
		"user2": {Username: "user2", Groups: []string{"users"}},
		"user3": {Username: "user3", Groups: []string{"admins"}},
		"user4": {Username: "user4", Groups: []string{"guests"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Contains(t, filtered, "user1")
	assert.Contains(t, filtered, "user3")
}

func TestSelectorTransformer_NoGroupSelector(t *testing.T) {
	config := map[string]any{}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1", Groups: []string{"admins"}},
		"user2": {Username: "user2", Groups: []string{"users"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
}

func TestSelectorTransformer_EmptyGroups(t *testing.T) {
	config := map[string]any{
		"groupSelector": "admins",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1", Groups: []string{}},
		"user2": {Username: "user2", Groups: nil},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 0)
}

func TestSelectorTransformer_MultipleGroupsPerUser(t *testing.T) {
	config := map[string]any{
		"groupSelector": "developers",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {
			Username: "user1",
			Groups:   []string{"admins", "developers", "users"},
		},
		"user2": {Username: "user2", Groups: []string{"admins", "users"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "user1")
}

func TestSelectorTransformer_EmailDomain(t *testing.T) {
	config := map[string]any{
		"emailDomain": "example.com",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	// Note: emailDomain filtering is not implemented yet
	// This test verifies the config is parsed correctly
	assert.Equal(t, "example.com", ft.emailDomain)
}

func TestSelectorTransformer_BothSelectors(t *testing.T) {
	config := map[string]any{
		"groupSelector": "admins",
		"emailDomain":   "example.com",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	assert.Equal(t, "admins", ft.groupSelector)
	assert.Equal(t, "example.com", ft.emailDomain)
}
