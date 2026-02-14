package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectorTransformer_RegexSelector(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "regex",
				"field":   "username",
				"pattern": "^admin.*",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "admin1"},
		"user2": {Username: "user2"},
		"user3": {Username: "admin_test"},
		"user4": {Username: "guest"},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Contains(t, filtered, "user1")
	assert.Contains(t, filtered, "user3")
}

func TestSelectorTransformer_StrictSelector(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "strict",
				"field":   "username",
				"pattern": "admin",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "admin"},
		"user2": {Username: "admin1"},
		"user3": {Username: "user"},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "user1")
}

func TestSelectorTransformer_EmailRegex(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "regex",
				"field":   "email",
				"pattern": "@example\\.com$",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1", Email: "user1@example.com"},
		"user2": {Username: "user2", Email: "user2@other.com"},
		"user3": {Username: "user3", Email: "user3@example.com"},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Contains(t, filtered, "user1")
	assert.Contains(t, filtered, "user3")
}

func TestSelectorTransformer_NoSelectors(t *testing.T) {
	config := map[string]any{}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "user1"},
		"user2": {Username: "user2"},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 0, "no selectors means no identities match")
}

func TestSelectorTransformer_MultipleSelectors(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "strict",
				"field":   "username",
				"pattern": "admin",
			},
			map[string]any{
				"type":    "regex",
				"field":   "email",
				"pattern": "@example\\.com$",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {Username: "admin", Email: "admin@other.com"},
		"user2": {Username: "user", Email: "user@example.com"},
		"user3": {Username: "guest", Email: "guest@other.com"},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 2, "matches either selector (OR logic)")
	assert.Contains(t, filtered, "user1")
	assert.Contains(t, filtered, "user2")
}

func TestSelectorTransformer_CustomAttribute(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "strict",
				"field":   "department",
				"pattern": "engineering",
			},
		},
	}

	ft, err := NewSelectorTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"user1": {
			Username:   "user1",
			Attributes: map[string]any{"department": "engineering"},
		},
		"user2": {
			Username:   "user2",
			Attributes: map[string]any{"department": "sales"},
		},
		"user3": {
			Username:   "user3",
			Attributes: map[string]any{},
		},
	}

	ctx := NewContext(context.Background(), nil)
	filtered, _, err := ft.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "user1")
}

func TestSelectorTransformer_InvalidRegex(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "regex",
				"field":   "username",
				"pattern": "[invalid(regex",
			},
		},
	}

	_, err := NewSelectorTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex pattern")
}

func TestSelectorTransformer_MissingSelectorType(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"field":   "username",
				"pattern": "admin",
			},
		},
	}

	_, err := NewSelectorTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selector type not specified")
}

func TestSelectorTransformer_UnknownSelectorType(t *testing.T) {
	config := map[string]any{
		"selectors": []any{
			map[string]any{
				"type":    "unknown",
				"field":   "username",
				"pattern": "admin",
			},
		},
	}

	_, err := NewSelectorTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown selector type")
}
