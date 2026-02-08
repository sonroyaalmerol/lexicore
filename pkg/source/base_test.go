package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseSource_Name(t *testing.T) {
	op := NewBaseSource("test-source", nil)
	assert.Equal(t, "test-source", op.Name())
}

func TestBaseSource_Config(t *testing.T) {
	op := NewBaseSource("test", nil)

	config := map[string]any{
		"key1": "value1",
		"key2": 42,
	}

	op.SetConfig(config)

	val, ok := op.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	str, err := op.GetStringConfig("key1")
	assert.NoError(t, err)
	assert.Equal(t, "value1", str)

	_, ok = op.GetConfig("nonexistent")
	assert.False(t, ok)

	_, err = op.GetStringConfig("key2")
	assert.Error(t, err)
}
