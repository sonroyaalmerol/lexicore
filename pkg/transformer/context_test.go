package transformer

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"github.com/stretchr/testify/assert"
)

func cv(v any) manifest.ConfigValue {
	return manifest.NewConfigValue(v)
}

func cvMap(m map[string]any) map[string]manifest.ConfigValue {
	out := make(map[string]manifest.ConfigValue, len(m))
	for k, v := range m {
		out[k] = cv(v)
	}
	return out
}

func TestContext_GetConfig(t *testing.T) {
	config := map[string]manifest.ConfigValue{
		"key1": cv("value1"),
		"key2": cv(true),
		"key3": cv(map[string]any{"nested": "value"}),
	}

	ctx := NewContext(context.Background(), config)

	val, ok := ctx.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	str, ok := ctx.GetConfigString("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", str)

	b, ok := ctx.GetConfigBool("key2")
	assert.True(t, ok)
	assert.True(t, b)

	m, ok := ctx.GetConfigMap("key3")
	assert.True(t, ok)
	assert.Equal(t, "value", m["nested"])

	_, ok = ctx.GetConfig("nonexistent")
	assert.False(t, ok)
}

func TestContext_Variables(t *testing.T) {
	ctx := NewContext(context.Background(), nil)

	ctx.SetVar("test", "value")
	val, ok := ctx.GetVar("test")
	assert.True(t, ok)
	assert.Equal(t, "value", val)

	str, ok := ctx.GetVarString("test")
	assert.True(t, ok)
	assert.Equal(t, "value", str)

	_, ok = ctx.GetVar("nonexistent")
	assert.False(t, ok)
}
