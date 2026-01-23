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
		"groupSelector": "admins",
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := []source.Identity{
		{Username: "user1", Groups: []string{"admins", "users"}},
		{Username: "user2", Groups: []string{"users"}},
		{Username: "user3", Groups: []string{"admins"}},
	}

	ctx := NewContext(context.Background(), nil)
	selectored, _, err := ft.Transform(ctx, identities, []source.Group{})

	require.NoError(t, err)
	assert.Len(t, selectored, 2)
	assert.Equal(t, "user1", selectored[0].Username)
	assert.Equal(t, "user3", selectored[1].Username)
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
			Name: "selector",
			Type: "selector",
			Config: map[string]any{
				"groupSelector": "admins",
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
}
