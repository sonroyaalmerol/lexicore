package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

type reconcileTask struct {
	targetName string
	immediate  bool
}

type Manager struct {
	sources         *xsync.Map[string, source.Source]
	operators       *xsync.Map[string, operator.Operator]
	syncTargets     *xsync.Map[string, *manifest.SyncTarget]
	identitySources *xsync.Map[string, *manifest.IdentitySource]
	reconcilers     *xsync.Map[string, *Reconciler]

	pluginManager *operator.PluginManager

	queue  chan reconcileTask
	cfg    *config.Config
	logger *zap.Logger

	wg          sync.WaitGroup
	shutdownCtx context.Context
	shutdown    context.CancelFunc
}

func NewManager(cfg *config.Config, logger *zap.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		sources:         xsync.NewMap[string, source.Source](),
		operators:       xsync.NewMap[string, operator.Operator](),
		syncTargets:     xsync.NewMap[string, *manifest.SyncTarget](),
		identitySources: xsync.NewMap[string, *manifest.IdentitySource](),
		reconcilers:     xsync.NewMap[string, *Reconciler](),
		pluginManager:   operator.NewPluginManager("/var/lib/lexicore/plugins"),
		queue:           make(chan reconcileTask, cfg.Workers.QueueSize),
		cfg:             cfg,
		logger:          logger,
		shutdownCtx:     ctx,
		shutdown:        cancel,
	}
}

func (m *Manager) GetIdentitySources() []manifest.IdentitySource {
	sources := make([]manifest.IdentitySource, 0, m.identitySources.Size())

	m.identitySources.Range(func(_ string, value *manifest.IdentitySource) bool {
		sources = append(sources, *value)
		return true
	})

	return sources
}

func (m *Manager) GetSyncTargets() []manifest.SyncTarget {
	targets := make([]manifest.SyncTarget, 0, m.syncTargets.Size())

	m.syncTargets.Range(func(_ string, value *manifest.SyncTarget) bool {
		targets = append(targets, *value)
		return true
	})

	return targets
}

func (m *Manager) RegisterSource(name string, src source.Source) {
	old, ok := m.sources.LoadAndStore(name, src)
	if ok && old != nil {
		_ = old.Close()
	}
}

func (m *Manager) RegisterOperator(op operator.Operator) {
	old, ok := m.operators.LoadAndStore(op.Name(), op)
	if ok && old != nil {
		_ = old.Close()
	}
}

func (m *Manager) AddIdentitySource(src *manifest.IdentitySource) error {
	if _, ok := m.sources.Load(src.Name); !ok {
		return fmt.Errorf("source %s not found", src.Name)
	}

	m.identitySources.Store(src.Name, src)
	return nil
}

func (m *Manager) AddSyncTarget(target *manifest.SyncTarget) error {
	if _, ok := m.sources.Load(target.Spec.SourceRef); !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	if target.Spec.PluginSource != nil {
		status, err := m.pluginManager.LoadPlugin(
			m.shutdownCtx,
			target.Spec.PluginSource,
		)

		target.Status.PluginStatus = status

		if err != nil {
			return fmt.Errorf("failed to load plugin: %w", err)
		}
	}

	if _, ok := m.operators.Load(target.Spec.Operator); !ok {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	m.syncTargets.Store(target.Name, target)

	// Start ticker for this target if manager is already running
	select {
	case <-m.shutdownCtx.Done():
		// Manager is shut down, don't start ticker
	default:
		m.startTargetTicker(target.Name)
	}

	return nil
}

func (m *Manager) getOrCreateReconciler(target *manifest.SyncTarget) (*Reconciler, error) {
	if reconciler, ok := m.reconcilers.Load(target.Name); ok {
		return reconciler, nil
	}

	src, ok := m.sources.Load(target.Spec.SourceRef)
	if !ok {
		return nil, fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	op, ok := m.operators.Load(target.Spec.Operator)
	if !ok {
		return nil, fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	reconciler, err := NewReconciler(target, src, op, m.logger)
	if err != nil {
		return nil, err
	}

	actual, loaded := m.reconcilers.LoadOrStore(target.Name, reconciler)
	if loaded {
		return actual, nil
	}

	return reconciler, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting controller manager",
		zap.Int("workers", m.cfg.Workers.ReconcileWorkers))

	for i := 0; i < m.cfg.Workers.ReconcileWorkers; i++ {
		m.wg.Add(1)
		go m.worker(ctx, i)
	}

	m.syncTargets.Range(func(name string, _ *manifest.SyncTarget) bool {
		m.startTargetTicker(name)
		return true
	})

	<-ctx.Done()

	m.logger.Info("Shutting down controller manager")
	m.shutdown()

	close(m.queue)

	m.wg.Wait()

	m.logger.Info("Controller manager stopped")
	return nil
}

func (m *Manager) worker(ctx context.Context, workerID int) {
	defer m.wg.Done()

	logger := m.logger.With(zap.Int("worker", workerID))
	logger.Debug("Worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Worker stopping due to context cancellation")
			return
		case task, ok := <-m.queue:
			if !ok {
				logger.Debug("Worker stopping due to closed queue")
				return
			}

			target, ok := m.syncTargets.Load(task.targetName)
			if !ok {
				logger.Warn("Target not found, skipping",
					zap.String("target", task.targetName))
				continue
			}

			if err := m.reconcile(ctx, target); err != nil {
				logger.Error("Reconciliation failed",
					zap.String("target", task.targetName),
					zap.Error(err))
			} else {
				logger.Debug("Reconciliation completed",
					zap.String("target", task.targetName))
			}
		}
	}
}

func (m *Manager) startTargetTicker(targetName string) {
	m.wg.Go(func() {
		ticker := time.NewTicker(m.cfg.DefaultSyncPeriod)
		defer ticker.Stop()

		logger := m.logger.With(zap.String("target", targetName))
		logger.Debug("Ticker started")

		select {
		case m.queue <- reconcileTask{targetName: targetName, immediate: true}:
		case <-m.shutdownCtx.Done():
			return
		}

		for {
			select {
			case <-m.shutdownCtx.Done():
				logger.Debug("Ticker stopped")
				return
			case <-ticker.C:
				// Check if target still exists before queuing
				if _, ok := m.syncTargets.Load(targetName); !ok {
					logger.Debug("Target removed, stopping ticker")
					return
				}

				select {
				case m.queue <- reconcileTask{targetName: targetName, immediate: false}:
				case <-m.shutdownCtx.Done():
					return
				default:
					logger.Warn("Queue full, skipping reconciliation")
				}
			}
		}
	})
}

func (m *Manager) reconcile(ctx context.Context, target *manifest.SyncTarget) error {
	reconciler, err := m.getOrCreateReconciler(target)
	if err != nil {
		return fmt.Errorf("failed to get reconciler for %s: %w", target.Name, err)
	}

	return reconciler.Reconcile(ctx)
}

func (m *Manager) GetIdentitySource(name string) (source.Source, bool) {
	return m.sources.Load(name)
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

func (m *Manager) TriggerReconciliation(targetName string) error {
	if _, ok := m.syncTargets.Load(targetName); !ok {
		return fmt.Errorf("target %s not found", targetName)
	}

	select {
	case m.queue <- reconcileTask{targetName: targetName, immediate: true}:
		return nil
	case <-m.shutdownCtx.Done():
		return fmt.Errorf("manager is shutting down")
	default:
		return fmt.Errorf("queue is full")
	}
}
