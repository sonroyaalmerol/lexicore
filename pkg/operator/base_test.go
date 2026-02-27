package operator

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"github.com/stretchr/testify/assert"
)

func cv(v any) manifest.ConfigValue {
	return manifest.NewConfigValue(v)
}

func TestBaseOperator_Name(t *testing.T) {
	op := NewBaseOperator("test-operator", nil)
	assert.Equal(t, "test-operator", op.Name())
}

func TestBaseOperator_Config(t *testing.T) {
	op := NewBaseOperator("test", nil)

	config := map[string]manifest.ConfigValue{
		"key1": cv("value1"),
		"key2": cv(42),
	}

	op.SetConfig(config)

	val, ok := op.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val.Value())

	str, err := op.GetStringConfig("key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", str)

	_, ok = op.GetConfig("nonexistent")
	assert.False(t, ok)

	s, err := op.GetStringConfig("key2")
	assert.Equal(t, "42", s)
}
