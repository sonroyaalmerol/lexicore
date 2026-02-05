package controller

import (
	"context"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconciler_Reconcile(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: map[string]source.Identity{
			"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
		},
		groups: map[string]source.Group{
			"1": {GID: "1", Name: "admins"},
		},
	}

	op := newMockOperator("mock")

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

	assert.True(t, op.syncCalled.Load())
	assert.Equal(t, "Success", target.Status.Status)
}

func TestReconciler_Reconcile_NoChanges(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	identities := map[string]source.Identity{
		"1": {UID: "1", Username: "user1", Email: "user1@example.com"},
	}

	src := &mockSource{identities: identities}
	op := newMockOperator("mock")

	target := &manifest.SyncTarget{
		ObjectMeta: v1.ObjectMeta{Name: "test"},
		Spec:       manifest.SyncTargetSpec{},
	}

	reconciler, err := NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	// First reconcile - should sync
	err = reconciler.Reconcile(context.Background())
	require.NoError(t, err)
	assert.True(t, op.syncCalled.Load())

	// Reset sync flag
	op.syncCalled.Store(false)

	// Second reconcile - no changes, should not sync
	err = reconciler.Reconcile(context.Background())
	require.NoError(t, err)
	assert.False(t, op.syncCalled.Load())
}
