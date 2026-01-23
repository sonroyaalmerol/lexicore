package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManager_RegisterSource(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{}
	manager.RegisterSource("test-source", src)

	source, _ := manager.sources.Load("test-source")
	assert.Equal(t, manager.sources.Size(), 1)
	assert.Equal(t, src, source)
}

func TestManager_RegisterOperator(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	op := newMockOperator("test-op")
	manager.RegisterOperator(op)

	operator, _ := manager.operators.Load("test-op")

	assert.Equal(t, manager.operators.Size(), 1)
	assert.Equal(t, op, operator)
}

func TestManager_AddSyncTarget(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{}
	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	tests := []struct {
		name      string
		target    *manifest.SyncTarget
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid target",
			target: &manifest.SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
				Spec: manifest.SyncTargetSpec{
					SourceRef: "test-source",
					Operator:  "test-op",
				},
			},
			wantError: false,
		},
		{
			name: "missing source",
			target: &manifest.SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
				Spec: manifest.SyncTargetSpec{
					SourceRef: "nonexistent-source",
					Operator:  "test-op",
				},
			},
			wantError: true,
			errorMsg:  "source nonexistent-source not found",
		},
		{
			name: "missing operator",
			target: &manifest.SyncTarget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
				Spec: manifest.SyncTargetSpec{
					SourceRef: "test-source",
					Operator:  "nonexistent-op",
				},
			},
			wantError: true,
			errorMsg:  "operator nonexistent-op not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.AddSyncTarget(tt.target)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				_, ok := manager.syncTargets.Load(tt.target.Name)
				assert.Equal(t, ok, true)
			}
		})
	}
}

func TestManager_Reconcile(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@example.com",
				Groups:   []string{"admins"},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@example.com",
				Groups:   []string{"users"},
			},
		},
		groups: []source.Group{
			{GID: "100", Name: "admins"},
			{GID: "101", Name: "users"},
		},
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
			DryRun:    false,
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.NoError(t, err)

	assert.True(t, op.syncCalled.Load())
	assert.Equal(t, 2, len(op.lastState.Identities))
	assert.Equal(t, 2, len(op.lastState.Groups))
	assert.False(t, op.lastState.DryRun)
}

func TestManager_ReconcileWithTransformers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@example.com",
				Groups:   []string{"admins", "developers"},
				Attributes: map[string]any{
					"firstName": "Alice",
					"lastName":  "Smith",
				},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@example.com",
				Groups:   []string{"users"},
				Attributes: map[string]any{
					"firstName": "Bob",
					"lastName":  "Jones",
				},
			},
			{
				UID:      "3",
				Username: "charlie",
				Email:    "charlie@example.com",
				Groups:   []string{"developers"},
				Attributes: map[string]any{
					"firstName": "Charlie",
					"lastName":  "Brown",
				},
			},
		},
		groups: []source.Group{
			{GID: "100", Name: "admins"},
			{GID: "101", Name: "users"},
			{GID: "102", Name: "developers"},
		},
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
			Transformers: []manifest.TransformerConfig{
				{
					Name: "selector-developers",
					Type: "selector",
					Config: map[string]any{
						"groupSelector": "developers",
					},
				},
				{
					Name: "generate-display-name",
					Type: "template",
					Config: map[string]any{
						"templates": map[string]any{
							"displayName": "{{.Attributes.firstName}} {{.Attributes.lastName}}",
							"homeDir":     "/home/{{.Username}}",
						},
					},
				},
			},
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.NoError(t, err)

	assert.True(t, op.syncCalled.Load())
	// Should only have alice and charlie (selectored by developers group)
	assert.Equal(t, 2, len(op.lastState.Identities))

	// Verify transformations were applied
	for _, identity := range op.lastState.Identities {
		// Check template transformer generated fields
		assert.Contains(t, identity.Attributes, "displayName")
		assert.Contains(t, identity.Attributes, "homeDir")

		if identity.Username == "alice" {
			assert.Equal(t, "Alice Smith", identity.Attributes["displayName"])
			assert.Equal(t, "/home/alice", identity.Attributes["homeDir"])
		} else if identity.Username == "charlie" {
			assert.Equal(t, "Charlie Brown", identity.Attributes["displayName"])
			assert.Equal(t, "/home/charlie", identity.Attributes["homeDir"])
		}
	}
}

func TestManager_ReconcileWithDryRun(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{UID: "1", Username: "alice", Email: "alice@example.com"},
		},
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
			DryRun:    true,
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.NoError(t, err)

	assert.True(t, op.syncCalled.Load())
	assert.True(t, op.lastState.DryRun)
}

func TestManager_ReconcileWithInvalidTransformer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{UID: "1", Username: "alice", Email: "alice@example.com"},
		},
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
			Transformers: []manifest.TransformerConfig{
				{
					Name:   "invalid",
					Type:   "unknown-type",
					Config: map[string]any{},
				},
			},
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create transformer pipeline")
}

func TestManager_ReconcileWithSourceError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		getIdentitiesError: assert.AnError,
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get identities")
}

func TestManager_ReconcileWithOperatorError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{UID: "1", Username: "alice", Email: "alice@example.com"},
		},
	}

	op := newMockOperator("test-op")
	op.syncError = assert.AnError

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx := context.Background()
	err = manager.reconcile(ctx, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sync")
}

func TestManager_Start(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()
	manager := NewManager(config.DefaultConfig(), logger)

	src := &mockSource{
		identities: []source.Identity{
			{UID: "1", Username: "alice", Email: "alice@example.com"},
		},
	}

	op := newMockOperator("test-op")

	manager.RegisterSource("test-source", src)
	manager.RegisterOperator(op)

	target := &manifest.SyncTarget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-target"},
		Spec: manifest.SyncTargetSpec{
			SourceRef: "test-source",
			Operator:  "test-op",
		},
	}

	err := manager.AddSyncTarget(target)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = manager.Start(ctx)
	}()

	// Wait a bit to ensure at least one reconciliation happens
	time.Sleep(500 * time.Millisecond)

	assert.True(t, op.syncCalled.Load())
}

// Mock implementations

type mockSource struct {
	identities         []source.Identity
	groups             []source.Group
	getIdentitiesError error
	getGroupsError     error
}

func (m *mockSource) Connect(ctx context.Context) error {
	return nil
}

func (m *mockSource) GetIdentities(ctx context.Context) ([]source.Identity, error) {
	if m.getIdentitiesError != nil {
		return nil, m.getIdentitiesError
	}
	return m.identities, nil
}

func (m *mockSource) GetGroups(ctx context.Context) ([]source.Group, error) {
	if m.getGroupsError != nil {
		return nil, m.getGroupsError
	}
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
	syncCalled atomic.Bool
	lastState  *operator.SyncState
	syncResult *operator.SyncResult
	syncError  error
}

func newMockOperator(name string) *mockOperator {
	return &mockOperator{
		BaseOperator: operator.NewBaseOperator(name),
		syncResult: &operator.SyncResult{
			IdentitiesCreated: 1,
			IdentitiesUpdated: 0,
			IdentitiesDeleted: 0,
		},
	}
}

func (m *mockOperator) Initialize(
	ctx context.Context,
	config map[string]any,
) error {
	m.SetConfig(config)
	return nil
}

func (m *mockOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) (*operator.SyncResult, error) {
	m.syncCalled.Store(true)
	m.lastState = state

	if m.syncError != nil {
		return nil, m.syncError
	}

	m.syncResult.IdentitiesCreated = len(state.Identities)
	return m.syncResult, nil
}

func (m *mockOperator) Validate(ctx context.Context) error {
	return nil
}

func (m *mockOperator) Close() error {
	return nil
}
