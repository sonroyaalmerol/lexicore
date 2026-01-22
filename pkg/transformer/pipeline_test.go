package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterTransformer_Transform(t *testing.T) {
	config := map[string]any{
		"groupFilter": "admins",
	}

	ft, err := NewFilterTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Groups: []string{"admins", "users"}},
		{Username: "user2", Groups: []string{"users"}},
		{Username: "user3", Groups: []string{"admins"}},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Equal(t, "user1", filtered[0].Username)
	assert.Equal(t, "user3", filtered[1].Username)
}

func TestMapTransformer_Transform(t *testing.T) {
	config := map[string]any{
		"mappings": map[string]any{
			"domain":  "example.com",
			"maildir": "/var/mail/{{username}}",
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
}

func TestTemplateTransformer_Transform(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"fullEmail": "{{.Username}}@example.com",
			"maildir":   "/var/mail/{{.Username}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{
			Username:   "john",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "john@example.com", transformed[0].Attributes["fullEmail"])
	assert.Equal(t, "/var/mail/john", transformed[0].Attributes["maildir"])
}

func TestPipeline_Execute(t *testing.T) {
	configs := []manifest.TransformerConfig{
		{
			Name: "filter",
			Type: "filter",
			Config: map[string]any{
				"groupFilter": "admins",
			},
		},
		{
			Name: "map",
			Type: "map",
			Config: map[string]any{
				"mappings": map[string]any{
					"domain": "example.com",
				},
			},
		},
	}

	pipeline, err := NewPipeline(configs)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Groups: []string{"admins"}, Attributes: make(map[string]any)},
		{Username: "user2", Groups: []string{"users"}, Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := pipeline.Execute(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "user1", result[0].Username)
	assert.Equal(t, "example.com", result[0].Attributes["domain"])
}
