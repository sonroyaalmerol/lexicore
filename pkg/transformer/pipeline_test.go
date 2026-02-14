package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectorTransformer_Transform(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "regex",
				"field":   "username",
				"pattern": "^user[13]$",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1"},
		"user2": {Username: "user2"},
		"user3": {Username: "user3"},
	}

	ctx := NewContext(context.Background(), nil)
	selectored, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, selectored, 2)
	assert.Contains(t, selectored, "user1")
	assert.Contains(t, selectored, "user3")
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

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["fullEmail"])
	assert.Equal(t, "/var/mail/john", transformed["john"].Attributes["maildir"])
}

func TestPipeline_Execute(t *testing.T) {
	configs := []manifest.TransformerConfig{
		{
			Name: "selector",
			Type: "selector",
			Config: map[string]any{
				"selectors": []any{
					map[string]any{
						"type":    "strict",
						"field":   "username",
						"pattern": "user1",
					},
				},
			},
		},
	}

	pipeline, err := NewPipeline(configs, "")
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1", Attributes: make(map[string]any)},
		"user2": {Username: "user2", Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := pipeline.Execute(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "user1")
}
