package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

type Manager struct {
	sources     *xsync.Map[string, source.Source]
	operators   *xsync.Map[string, operator.Operator]
	syncTargets *xsync.Map[string, *manifest.SyncTarget]
	reconcilers *xsync.Map[string, *Reconciler]
	logger      *zap.Logger
}

func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		sources:     xsync.NewMap[string, source.Source](),
		operators:   xsync.NewMap[string, operator.Operator](),
		syncTargets: xsync.NewMap[string, *manifest.SyncTarget](),
		reconcilers: xsync.NewMap[string, *Reconciler](),
		logger:      logger,
	}
}

func (m *Manager) RegisterSource(name string, src source.Source) {
	old, ok := m.sources.LoadAndStore(name, src)
	if ok {
		_ = old.Close()
	}
}

func (m *Manager) RegisterOperator(op operator.Operator) {
	old, ok := m.operators.LoadAndStore(op.Name(), op)
	if ok {
		_ = old.Close()
	}
}

func (m *Manager) createReconciler(target *manifest.SyncTarget) error {
	var ok bool
	var source source.Source
	var operator operator.Operator

	if source, ok = m.sources.Load(target.Spec.SourceRef); !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	if operator, ok = m.operators.Load(target.Spec.Operator); !ok {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	reconciler, err := NewReconciler(target, source, operator, m.logger)
	if err != nil {
		return err
	}

	m.reconcilers.Store(target.Name, reconciler)

	return nil
}

func (m *Manager) AddSyncTarget(target *manifest.SyncTarget) error {
	if _, ok := m.sources.Load(target.Spec.SourceRef); !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	if _, ok := m.operators.Load(target.Spec.Operator); !ok {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	m.syncTargets.Store(target.Name, target)
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting controller manager")

	var wg sync.WaitGroup
	m.syncTargets.Range(func(name string, target *manifest.SyncTarget) bool {
		err := m.createReconciler(target)
		if err != nil {
			m.logger.Error("Failed to create reconciler",
				zap.String("target", name),
				zap.Error(err))
			return false
		}

		wg.Add(1)
		go func(name string, target *manifest.SyncTarget) {
			defer wg.Done()
			m.runReconcileLoop(ctx, name, target)
		}(name, target)

		return true
	})

	wg.Wait()
	return nil
}

func (m *Manager) runReconcileLoop(
	ctx context.Context,
	name string,
	target *manifest.SyncTarget,
) {
	syncPeriod := 5 * time.Minute
	ticker := time.NewTicker(syncPeriod)
	defer ticker.Stop()

	m.logger.Info("Starting reconcile loop", zap.String("target", name))

	if err := m.reconcile(ctx, target); err != nil {
		m.logger.Error("Reconciliation failed",
			zap.String("target", name),
			zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.reconcile(ctx, target); err != nil {
				m.logger.Error("Reconciliation failed",
					zap.String("target", name),
					zap.Error(err))
			}
		}
	}
}

func (m *Manager) reconcile(
	ctx context.Context,
	target *manifest.SyncTarget,
) error {
	var ok bool
	var reconciler *Reconciler
	for !ok {
		reconciler, ok = m.reconcilers.Load(target.Name)
		if !ok {
			err := m.createReconciler(target)
			if err != nil {
				return err
			}
		}
	}

	if err := reconciler.Reconcile(ctx); err != nil {
		return err
	}

	return nil
}
