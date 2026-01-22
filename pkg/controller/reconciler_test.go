package controller

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mock implementations
type mockSource struct {
	identities []source.Identity
	groups     []source.Group
}

func (m *mockSource) Connect(ctx context.Context) error {
	return nil
}

func (m *mockSource) GetIdentities(ctx context.Context) ([]source.Identity, error) {
	return m.identities, nil
}

func (m *mockSource) GetGroups(ctx context.Context) ([]source.Group, error) {
	return m.groups, nil
}

func (m *mockSource) Watch(ctx context.Context) (<-chan source.Event, error) {
	return make(chan source.Event), nil
}

func (m *mockSource) Close() error {
	return nil
}

type mockOperator struct {
	*operator.BaseOperator
	syncCalled bool
	syncResult *operator.SyncResult
}

func newMockOperator() *mockOperator {
	return &mockOperator{
		BaseOperator: operator.NewBaseOperator("mock"),
		syncResult: &operator.SyncResult{
			IdentitiesCreated: 1,
		},
	}
}

func (m *mockOperator) Initialize(ctx context.Context, config map[string]any) error {
	m.SetConfig(config)
	return nil
}

func (m *mockOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	m.syncCalled = true
	return m.syncResult, nil
}

func (m *mockOperator) Validate(ctx context.Context) error {
	return nil
}

func (m *mockOperator) Close() error {
	return nil
}

func TestReconciler_Reconcile(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{UID: "1", Username: "user1", Email: "user1@example.com"},
		},
		groups: []source.Group{
			{GID: "1", Name: "admins"},
		},
	}

	op := newMockOperator()

	target := &manifest.SyncTarget{
		ObjectMeta: v1.ObjectMeta{Name: "test"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "mock",
		},
	}

	reconciler, err := NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	err = reconciler.Reconcile(context.Background())
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
}

func TestReconciler_Reconcile_NoChanges(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	identities := []source.Identity{
		{UID: "1", Username: "user1", Email: "user1@example.com"},
	}

	src := &mockSource{identities: identities}
	op := newMockOperator()

	target := &manifest.SyncTarget{
		ObjectMeta: v1.ObjectMeta{Name: "test"},
		Spec:       manifest.SyncTargetSpec{},
	}

	reconciler, err := NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	// First reconcile - should sync
	err = reconciler.Reconcile(context.Background())
	require.NoError(t, err)
	assert.True(t, op.syncCalled)

	// Reset sync flag
	op.syncCalled = false

	// Second reconcile - no changes, should not sync
	err = reconciler.Reconcile(context.Background())
	require.NoError(t, err)
	assert.False(t, op.syncCalled)
}
