package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupAggregationTransformer_First(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "userDepartment",
				"aggregationMode": "first",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"department": "Engineering",
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"department": "Sales",
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, "Engineering", result["user1"].Attributes["userDepartment"])
}

func TestGroupAggregationTransformer_Last(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "userDepartment",
				"aggregationMode": "last",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"department": "Engineering",
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"department": "Sales",
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, "Sales", result["user1"].Attributes["userDepartment"])
}

func TestGroupAggregationTransformer_Max(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "priority",
				"targetAttribute": "maxPriority",
				"aggregationMode": "max",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"priority": 5,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"priority": 10,
			},
		},
		"group3": {
			GID: "group3",
			Attributes: map[string]any{
				"priority": 3,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2", "group3"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, 10, result["user1"].Attributes["maxPriority"])
}

func TestGroupAggregationTransformer_Min(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "priority",
				"targetAttribute": "minPriority",
				"aggregationMode": "min",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"priority": 5,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"priority": 10,
			},
		},
		"group3": {
			GID: "group3",
			Attributes: map[string]any{
				"priority": 3,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2", "group3"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, 3, result["user1"].Attributes["minPriority"])
}

func TestGroupAggregationTransformer_Append(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "role",
				"targetAttribute": "roles",
				"aggregationMode": "append",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"role": "admin",
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"role": "user",
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	roles := result["user1"].Attributes["roles"].([]any)
	assert.Len(t, roles, 2)
	assert.Contains(t, roles, "admin")
	assert.Contains(t, roles, "user")
}

func TestGroupAggregationTransformer_UniqueAppend(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "role",
				"targetAttribute": "roles",
				"aggregationMode": "uniqueAppend",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"role": "admin",
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"role": "admin",
			},
		},
		"group3": {
			GID: "group3",
			Attributes: map[string]any{
				"role": "user",
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2", "group3"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	roles := result["user1"].Attributes["roles"].([]any)
	assert.Len(t, roles, 2)
	assert.Contains(t, roles, "admin")
	assert.Contains(t, roles, "user")
}

func TestGroupAggregationTransformer_Weighted(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "primaryDepartment",
				"aggregationMode": "weighted",
				"weightAttribute": "weight",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"department": "Engineering",
				"weight":     5.0,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"department": "Sales",
				"weight":     10.0,
			},
		},
		"group3": {
			GID: "group3",
			Attributes: map[string]any{
				"department": "Marketing",
				"weight":     3.0,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2", "group3"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, "Sales", result["user1"].Attributes["primaryDepartment"])
}

func TestGroupAggregationTransformer_Any(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "isActive",
				"targetAttribute": "hasActiveGroup",
				"aggregationMode": "any",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"isActive": false,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"isActive": true,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, true, result["user1"].Attributes["hasActiveGroup"])
}

func TestGroupAggregationTransformer_All(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "isActive",
				"targetAttribute": "allGroupsActive",
				"aggregationMode": "all",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	t.Run("all true", func(t *testing.T) {
		groups := map[string]source.Group{
			"group1": {
				GID: "group1",
				Attributes: map[string]any{
					"isActive": true,
				},
			},
			"group2": {
				GID: "group2",
				Attributes: map[string]any{
					"isActive": true,
				},
			},
		}

		identities := map[string]source.Identity{
			"user1": {
				UID:        "user1",
				Username:   "user1",
				Groups:     []string{"group1", "group2"},
				Attributes: make(map[string]any),
			},
		}

		ctx := NewContext(context.Background(), nil)
		result, _, err := gat.Transform(ctx, identities, groups)

		require.NoError(t, err)
		assert.Equal(t, true, result["user1"].Attributes["allGroupsActive"])
	})

	t.Run("one false", func(t *testing.T) {
		groups := map[string]source.Group{
			"group1": {
				GID: "group1",
				Attributes: map[string]any{
					"isActive": true,
				},
			},
			"group2": {
				GID: "group2",
				Attributes: map[string]any{
					"isActive": false,
				},
			},
		}

		identities := map[string]source.Identity{
			"user1": {
				UID:        "user1",
				Username:   "user1",
				Groups:     []string{"group1", "group2"},
				Attributes: make(map[string]any),
			},
		}

		ctx := NewContext(context.Background(), nil)
		result, _, err := gat.Transform(ctx, identities, groups)

		require.NoError(t, err)
		assert.Equal(t, false, result["user1"].Attributes["allGroupsActive"])
	})
}

func TestGroupAggregationTransformer_AnyFalse(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "isActive",
				"targetAttribute": "hasInactiveGroup",
				"aggregationMode": "anyFalse",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"isActive": true,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"isActive": false,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, true, result["user1"].Attributes["hasInactiveGroup"])
}

func TestGroupAggregationTransformer_AllFalse(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "isActive",
				"targetAttribute": "allGroupsInactive",
				"aggregationMode": "allFalse",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	t.Run("all false", func(t *testing.T) {
		groups := map[string]source.Group{
			"group1": {
				GID: "group1",
				Attributes: map[string]any{
					"isActive": false,
				},
			},
			"group2": {
				GID: "group2",
				Attributes: map[string]any{
					"isActive": false,
				},
			},
		}

		identities := map[string]source.Identity{
			"user1": {
				UID:        "user1",
				Username:   "user1",
				Groups:     []string{"group1", "group2"},
				Attributes: make(map[string]any),
			},
		}

		ctx := NewContext(context.Background(), nil)
		result, _, err := gat.Transform(ctx, identities, groups)

		require.NoError(t, err)
		assert.Equal(t, true, result["user1"].Attributes["allGroupsInactive"])
	})

	t.Run("one true", func(t *testing.T) {
		groups := map[string]source.Group{
			"group1": {
				GID: "group1",
				Attributes: map[string]any{
					"isActive": false,
				},
			},
			"group2": {
				GID: "group2",
				Attributes: map[string]any{
					"isActive": true,
				},
			},
		}

		identities := map[string]source.Identity{
			"user1": {
				UID:        "user1",
				Username:   "user1",
				Groups:     []string{"group1", "group2"},
				Attributes: make(map[string]any),
			},
		}

		ctx := NewContext(context.Background(), nil)
		result, _, err := gat.Transform(ctx, identities, groups)

		require.NoError(t, err)
		assert.Equal(t, false, result["user1"].Attributes["allGroupsInactive"])
	})
}

func TestGroupAggregationTransformer_DefaultValue(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "userDepartment",
				"aggregationMode": "first",
				"defaultValue":    "Unknown",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID:        "group1",
			Attributes: map[string]any{},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, "Unknown", result["user1"].Attributes["userDepartment"])
}

func TestGroupAggregationTransformer_MultipleMappings(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "userDepartment",
				"aggregationMode": "first",
			},
			map[string]any{
				"sourceAttribute": "priority",
				"targetAttribute": "maxPriority",
				"aggregationMode": "max",
			},
			map[string]any{
				"sourceAttribute": "isActive",
				"targetAttribute": "hasActiveGroup",
				"aggregationMode": "any",
			},
		},
	}

	gat, err := NewGroupAggregationTransformer(config)
	require.NoError(t, err)

	groups := map[string]source.Group{
		"group1": {
			GID: "group1",
			Attributes: map[string]any{
				"department": "Engineering",
				"priority":   5,
				"isActive":   false,
			},
		},
		"group2": {
			GID: "group2",
			Attributes: map[string]any{
				"department": "Sales",
				"priority":   10,
				"isActive":   true,
			},
		},
	}

	identities := map[string]source.Identity{
		"user1": {
			UID:        "user1",
			Username:   "user1",
			Groups:     []string{"group1", "group2"},
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	result, _, err := gat.Transform(ctx, identities, groups)

	require.NoError(t, err)
	assert.Equal(t, "Engineering", result["user1"].Attributes["userDepartment"])
	assert.Equal(t, 10, result["user1"].Attributes["maxPriority"])
	assert.Equal(t, true, result["user1"].Attributes["hasActiveGroup"])
}

func TestGroupAggregationTransformer_InvalidAggregationMode(t *testing.T) {
	config := map[string]any{
		"mappings": []any{
			map[string]any{
				"sourceAttribute": "department",
				"targetAttribute": "userDepartment",
				"aggregationMode": "invalid",
			},
		},
	}

	_, err := NewGroupAggregationTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid aggregation mode")
}
