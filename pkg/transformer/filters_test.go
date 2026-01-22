package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterTransformer_GroupFilter(t *testing.T) {
	config := map[string]any{
		"groupFilter": "admins",
	}

	ft, err := NewFilterTransformer(config)
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

func TestFilterTransformer_NoGroupFilter(t *testing.T) {
	config := map[string]any{}

	ft, err := NewFilterTransformer(config)
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

func TestFilterTransformer_EmptyGroups(t *testing.T) {
	config := map[string]any{
		"groupFilter": "admins",
	}

	ft, err := NewFilterTransformer(config)
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

func TestFilterTransformer_MultipleGroupsPerUser(t *testing.T) {
	config := map[string]any{
		"groupFilter": "developers",
	}

	ft, err := NewFilterTransformer(config)
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

func TestFilterTransformer_EmailDomain(t *testing.T) {
	config := map[string]any{
		"emailDomain": "example.com",
	}

	ft, err := NewFilterTransformer(config)
	require.NoError(t, err)

	// Note: emailDomain filtering is not implemented yet
	// This test verifies the config is parsed correctly
	assert.Equal(t, "example.com", ft.emailDomain)
}

func TestFilterTransformer_BothFilters(t *testing.T) {
	config := map[string]any{
		"groupFilter": "admins",
		"emailDomain": "example.com",
	}

	ft, err := NewFilterTransformer(config)
	require.NoError(t, err)

	assert.Equal(t, "admins", ft.groupFilter)
	assert.Equal(t, "example.com", ft.emailDomain)
}
