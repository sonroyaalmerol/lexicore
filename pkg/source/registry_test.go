package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock source for testing
type mockSource struct {
	connected bool
}

func newMockSource(config map[string]any) (Source, error) {
	return &mockSource{}, nil
}

func (m *mockSource) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *mockSource) GetIdentities(ctx context.Context) (map[string]Identity, error) {
	return map[string]Identity{}, nil
}

func (m *mockSource) GetGroups(ctx context.Context) (map[string]Group, error) {
	return map[string]Group{}, nil
}

func (m *mockSource) Watch(ctx context.Context) (<-chan Event, error) {
	return make(chan Event), nil
}

func (m *mockSource) Close() error {
	m.connected = false
	return nil
}

func TestSourceRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	err := registry.Register("mock", newMockSource)
	require.NoError(t, err)

	err = registry.Register("mock", newMockSource)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestSourceRegistry_Create(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock", newMockSource)

	src, err := registry.Create(context.Background(), "mock", map[string]any{})
	require.NoError(t, err)
	assert.NotNil(t, src)

	_, err = registry.Create(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
}

func TestSourceRegistry_List(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock1", newMockSource)
	registry.Register("mock2", newMockSource)

	list := registry.List()
	assert.Len(t, list, 2)
	assert.Contains(t, list, "mock1")
	assert.Contains(t, list, "mock2")
}

func TestSourceRegistry_Has(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock", newMockSource)

	assert.True(t, registry.Has("mock"))
	assert.False(t, registry.Has("nonexistent"))
}
