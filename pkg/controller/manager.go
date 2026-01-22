package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"go.uber.org/zap"
)

type Manager struct {
	sources     map[string]source.Source
	operators   map[string]operator.Operator
	syncTargets map[string]*manifest.SyncTarget
	logger      *zap.Logger
	mu          sync.RWMutex
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		sources:     make(map[string]source.Source),
		operators:   make(map[string]operator.Operator),
		syncTargets: make(map[string]*manifest.SyncTarget),
		logger:      logger,
	}
}

func (m *Manager) RegisterSource(name string, src source.Source) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[name] = src
}

func (m *Manager) RegisterOperator(op operator.Operator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operators[op.Name()] = op
}

func (m *Manager) AddSyncTarget(target *manifest.SyncTarget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sources[target.Spec.SourceRef]; !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	if _, ok := m.operators[target.Spec.Operator]; !ok {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	m.syncTargets[target.Name] = target
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting controller manager")

	var wg sync.WaitGroup

	for name, target := range m.syncTargets {
		wg.Add(1)
		go func(name string, target *manifest.SyncTarget) {
			defer wg.Done()
			m.runReconcileLoop(ctx, name, target)
		}(name, target)
	}

	wg.Wait()
	return nil
}

func (m *Manager) runReconcileLoop(
	ctx context.Context,
	name string,
	target *manifest.SyncTarget,
) {
	syncPeriod := 5 * time.Minute // Default, parse from manifest
	ticker := time.NewTicker(syncPeriod)
	defer ticker.Stop()

	m.logger.Info("Starting reconcile loop", zap.String("target", name))

	if err := m.reconcile(ctx, target); err != nil {
		m.logger.Error(
			"Reconciliation failed",
			zap.String("target", name),
			zap.Error(err),
		)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.reconcile(ctx, target); err != nil {
				m.logger.Error(
					"Reconciliation failed",
					zap.String("target", name),
					zap.Error(err),
				)
			}
		}
	}
}

func (m *Manager) reconcile(
	ctx context.Context,
	target *manifest.SyncTarget,
) error {
	m.logger.Info("Reconciling", zap.String("target", target.Name))

	src, ok := m.sources[target.Spec.SourceRef]
	if !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	identities, err := src.GetIdentities(ctx)
	if err != nil {
		return fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := src.GetGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to get groups: %w", err)
	}

	m.logger.Info(
		"Fetched from source",
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	// Apply transformations
	// ... transformer pipeline execution ...

	op, ok := m.operators[target.Spec.Operator]
	if !ok {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	state := &operator.SyncState{
		Identities: identities,
		Groups:     groups,
		DryRun:     target.Spec.DryRun,
	}

	result, err := op.Sync(ctx, state)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	m.logger.Info(
		"Sync completed",
		zap.String("target", target.Name),
		zap.Int("created", result.IdentitiesCreated),
		zap.Int("updated", result.IdentitiesUpdated),
		zap.Int("deleted", result.IdentitiesDeleted),
	)

	return nil
}
