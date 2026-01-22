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

	identities := []source.Identity{
		{Username: "user1", Groups: []string{"admins", "users"}},
		{Username: "user2", Groups: []string{"users"}},
		{Username: "user3", Groups: []string{"admins"}},
		{Username: "user4", Groups: []string{"guests"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Equal(t, "user1", filtered[0].Username)
	assert.Equal(t, "user3", filtered[1].Username)
}

func TestSelectorTransformer_NoGroupSelector(t *testing.T) {
	config := map[string]any{}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Groups: []string{"admins"}},
		{Username: "user2", Groups: []string{"users"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
}

func TestSelectorTransformer_EmptyGroups(t *testing.T) {
	config := map[string]any{
		"groupSelector": "admins",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Groups: []string{}},
		{Username: "user2", Groups: nil},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 0)
}

func TestSelectorTransformer_MultipleGroupsPerUser(t *testing.T) {
	config := map[string]any{
		"groupSelector": "developers",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username: "user1",
			Groups:   []string{"admins", "developers", "users"},
		},
		{Username: "user2", Groups: []string{"admins", "users"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "user1", filtered[0].Username)
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
