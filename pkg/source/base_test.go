package source

import (
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"github.com/stretchr/testify/assert"
)

func TestBaseSource_Name(t *testing.T) {
	op := NewBaseSource("test-source", nil)
	assert.Equal(t, "test-source", op.Name())
}

func TestBaseSource_Config(t *testing.T) {
	op := NewBaseSource("test", nil)

	config := map[string]manifest.ConfigValue{
		"key1": manifest.NewConfigValue("value1"),
		"key2": manifest.NewConfigValue(42),
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
