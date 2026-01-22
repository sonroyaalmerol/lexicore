package operator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock operator for testing
type mockOperator struct {
	*BaseOperator
}

func newMockOperator() Operator {
	return &mockOperator{
		BaseOperator: NewBaseOperator("mock"),
	}
}

func (m *mockOperator) Initialize(ctx context.Context, config map[string]any) error {
	m.SetConfig(config)
	return nil
}

func (m *mockOperator) Sync(ctx context.Context, state *SyncState) (*SyncResult, error) {
	return &SyncResult{}, nil
}

func (m *mockOperator) Validate(ctx context.Context) error {
	return nil
}

func (m *mockOperator) Close() error {
	return nil
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	err := registry.Register("mock", newMockOperator)
	require.NoError(t, err)

	// Test duplicate registration
	err = registry.Register("mock", newMockOperator)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Create(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock", newMockOperator)

	op, err := registry.Create("mock")
	require.NoError(t, err)
	assert.NotNil(t, op)
	assert.Equal(t, "mock", op.Name())

	// Test creating non-existent operator
	_, err = registry.Create("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock1", newMockOperator)
	registry.Register("mock2", newMockOperator)

	list := registry.List()
	assert.Len(t, list, 2)
	assert.Contains(t, list, "mock1")
	assert.Contains(t, list, "mock2")
}

func TestRegistry_Has(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock", newMockOperator)

	assert.True(t, registry.Has("mock"))
	assert.False(t, registry.Has("nonexistent"))
}
