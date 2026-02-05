package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateTransformer_BasicTemplate(t *testing.T) {
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

func TestTemplateTransformer_MultipleFields(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"displayName": "{{.Username}} ({{.Email}})",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Email:      "john@example.com",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"john (john@example.com)",
		transformed["john"].Attributes["displayName"],
	)
}

func TestTemplateTransformer_AttributeAccess(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"summary": "User {{.Username}} from {{.Attributes.department}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"department": "Engineering",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"User john from Engineering",
		transformed["john"].Attributes["summary"],
	)
}

func TestTemplateTransformer_EmptyConfig(t *testing.T) {
	config := map[string]any{}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"existing": "value",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "value", transformed["john"].Attributes["existing"])
}

func TestTemplateTransformer_InvalidTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"invalid": "{{.Username",
		},
	}

	_, err := NewTemplateTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestTemplateTransformer_NonStringTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"number": 123,
			"valid":  "{{.Username}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	// Should skip non-string templates
	assert.Len(t, tt.templates, 1)
	assert.NotNil(t, tt.templates["valid"])
	assert.Nil(t, tt.templates["number"])
}

func TestTemplateTransformer_NilAttributes(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@example.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Attributes: nil,
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	require.NotNil(t, transformed["john"].Attributes)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
}

func TestTemplateTransformer_MultipleIdentities(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@example.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {Username: "john", Attributes: make(map[string]any)},
		"jane": {Username: "jane", Attributes: make(map[string]any)},
		"bob":  {Username: "bob", Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, transformed, 3)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
	assert.Equal(t, "jane@example.com", transformed["jane"].Attributes["email"])
	assert.Equal(t, "bob@example.com", transformed["bob"].Attributes["email"])
}

func TestTemplateTransformer_ComplexTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"path": "/home/{{.Username}}/{{.Attributes.project}}/data",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"project": "myapp",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "/home/john/myapp/data", transformed["john"].Attributes["path"])
}

func TestTemplateTransformer_OverwriteExisting(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@newdomain.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"email": "john@olddomain.com",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"john@newdomain.com",
		transformed["john"].Attributes["email"],
	)
}
