package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapTransformer_WithDefaults(t *testing.T) {
	config := map[string]any{
		"mappings": map[string]any{
			"domain":  "example.com",
			"maildir": "/var/mail/",
			"quota":   1000,
		},
	}

	mt, err := NewMapTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username:   "user1",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := mt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "example.com", transformed[0].Attributes["domain"])
	assert.Equal(t, "/var/mail/", transformed[0].Attributes["maildir"])
	assert.Equal(t, 1000, transformed[0].Attributes["quota"])
}

func TestMapTransformer_OverwriteExisting(t *testing.T) {
	config := map[string]any{
		"mappings": map[string]any{
			"domain": "newdomain.com",
		},
	}

	mt, err := NewMapTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username: "user1",
			Attributes: map[string]any{
				"domain": "olddomain.com",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := mt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "newdomain.com", transformed[0].Attributes["domain"])
}

func TestMapTransformer_EmptyConfig(t *testing.T) {
	config := map[string]any{}

	mt, err := NewMapTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username: "user1",
			Attributes: map[string]any{
				"existing": "value",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := mt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "value", transformed[0].Attributes["existing"])
}

func TestMapTransformer_NilAttributes(t *testing.T) {
	config := map[string]any{
		"mappings": map[string]any{
			"domain": "example.com",
		},
	}

	mt, err := NewMapTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username:   "user1",
			Attributes: nil,
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := mt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	require.NotNil(t, transformed[0].Attributes)
	assert.Equal(t, "example.com", transformed[0].Attributes["domain"])
}

func TestMapTransformer_MultipleIdentities(t *testing.T) {
	config := map[string]any{
		"mappings": map[string]any{
			"domain": "example.com",
		},
	}

	mt, err := NewMapTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Attributes: make(map[string]any)},
		{Username: "user2", Attributes: make(map[string]any)},
		{Username: "user3", Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := mt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, transformed, 3)
	for _, identity := range transformed {
		assert.Equal(t, "example.com", identity.Attributes["domain"])
	}
}
