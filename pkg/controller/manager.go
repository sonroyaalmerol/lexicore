package controller

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

type Manager struct {
	sources         *xsync.Map[string, source.Source]
	operators       *xsync.Map[string, operator.Operator]
	syncTargets     *xsync.Map[string, *manifest.SyncTarget]
	identitySources *xsync.Map[string, *manifest.IdentitySource]
	reconcilers     *xsync.Map[string, *Reconciler]
	cfg             *config.Config
	logger          *zap.Logger
}

func NewManager(cfg *config.Config, logger *zap.Logger) *Manager {
	return &Manager{
		sources:         xsync.NewMap[string, source.Source](),
		operators:       xsync.NewMap[string, operator.Operator](),
		syncTargets:     xsync.NewMap[string, *manifest.SyncTarget](),
		identitySources: xsync.NewMap[string, *manifest.IdentitySource](),
		reconcilers:     xsync.NewMap[string, *Reconciler](),
		cfg:             cfg,
		logger:          logger,
	}
}

func (m *Manager) GetIdentitySources() []manifest.IdentitySource {
	sources := make([]manifest.IdentitySource, 0, m.sources.Size())

	m.identitySources.Range(func(_ string, value *manifest.IdentitySource) bool {
		sources = append(sources, *value)
		return true
	})

	return sources
}

func (m *Manager) GetSyncTargets() []manifest.SyncTarget {
	targets := make([]manifest.SyncTarget, 0, m.sources.Size())

	m.syncTargets.Range(func(_ string, value *manifest.SyncTarget) bool {
		targets = append(targets, *value)
		return true
	})

	return targets
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

func (m *Manager) AddIdentitySource(source *manifest.IdentitySource) error {
	if _, ok := m.sources.Load(source.Name); !ok {
		return fmt.Errorf("source %s not found", source.Name)
	}

	m.identitySources.Store(source.Name, source)
	return nil
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
	m.logger.Info("Starting controller manager",
		zap.Int("workers", m.cfg.Workers.ReconcileWorkers))

	queue := make(chan string, m.cfg.Workers.QueueSize)

	for i := 0; i < m.cfg.Workers.ReconcileWorkers; i++ {
		go func(workerID int) {
			for {
				select {
				case <-ctx.Done():
					return
				case targetName := <-queue:
					if target, ok := m.syncTargets.Load(targetName); ok {
						m.reconcile(ctx, target)
					}
				}
			}
		}(i)
	}

	m.syncTargets.Range(func(name string, target *manifest.SyncTarget) bool {
		m.createReconciler(target)

		go func(n string) {
			ticker := time.NewTicker(m.cfg.DefaultSyncPeriod)
			defer ticker.Stop()

			queue <- n

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					queue <- n
				}
			}
		}(name)
		return true
	})

	<-ctx.Done()
	return nil
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

func (m *Manager) RemoveIdentitySource(name string) {
	m.identitySources.Delete(name)
	if src, ok := m.sources.LoadAndDelete(name); ok {
		m.logger.Info("Closing identity source driver", zap.String("name", name))
		_ = src.Close()
	}
}

func (m *Manager) RemoveSyncTarget(name string) {
	m.syncTargets.Delete(name)
	m.reconcilers.Delete(name)
	m.logger.Info("Removed SyncTarget from active reconciliation", zap.String("name", name))
}
