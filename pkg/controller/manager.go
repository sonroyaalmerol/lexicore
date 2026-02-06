package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/cache"
	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	adop "codeberg.org/lexicore/lexicore/pkg/operator/drivers/ad"
	dovecotop "codeberg.org/lexicore/lexicore/pkg/operator/drivers/dovecot"
	iredadminop "codeberg.org/lexicore/lexicore/pkg/operator/drivers/iredadmin"
	ldapop "codeberg.org/lexicore/lexicore/pkg/operator/drivers/ldap"
	"codeberg.org/lexicore/lexicore/pkg/source"
	authentiksrc "codeberg.org/lexicore/lexicore/pkg/source/authentik"
	ldapsrc "codeberg.org/lexicore/lexicore/pkg/source/ldap"
	"codeberg.org/lexicore/lexicore/pkg/store"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

type reconcileTask struct {
	targetName string
	immediate  bool
}

type ActiveOperator struct {
	operator.Operator
	manifest *manifest.SyncTarget
	cache    *cache.Store
}

type ActiveSource struct {
	source.Source
	manifest *manifest.IdentitySource
	cache    *cache.Store
}

type Manager struct {
	sourceFactory   *xsync.Map[string, func() source.Source]
	operatorFactory *xsync.Map[string, func() operator.Operator]
	activeOperators *xsync.Map[string, *ActiveOperator]
	activeSources   *xsync.Map[string, *ActiveSource]
	db              *store.EtcdStore

	pluginManager *PluginManager

	queue  chan reconcileTask
	cfg    *config.Config
	logger *zap.Logger

	wg          sync.WaitGroup
	shutdownCtx context.Context
	shutdown    context.CancelFunc
}

func NewManager(ctx context.Context, cfg *config.Config, db *store.EtcdStore, logger *zap.Logger) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{
		db:              db,
		sourceFactory:   xsync.NewMap[string, func() source.Source](),
		operatorFactory: xsync.NewMap[string, func() operator.Operator](),
		activeOperators: xsync.NewMap[string, *ActiveOperator](),
		activeSources:   xsync.NewMap[string, *ActiveSource](),
		pluginManager:   NewPluginManager("/var/lib/lexicore/plugins"),
		queue:           make(chan reconcileTask, cfg.Workers.QueueSize),
		cfg:             cfg,
		logger:          logger,
		shutdownCtx:     ctx,
		shutdown:        cancel,
	}

	// First-party operators
	m.RegisterOperator("active-directory", func() operator.Operator {
		return &adop.ADOperator{
			BaseOperator: operator.NewBaseOperator("active-directory"),
		}
	})
	m.RegisterOperator("dovecot-acl", func() operator.Operator {
		return &dovecotop.DovecotOperator{
			BaseOperator: operator.NewBaseOperator("dovecot-acl"),
			Client: &http.Client{
				Timeout: 15 * time.Second,
			},
		}
	})
	m.RegisterOperator("iredadmin", func() operator.Operator {
		jar, _ := cookiejar.New(nil)

		return &iredadminop.IRedAdminOperator{
			BaseOperator: operator.NewBaseOperator("iredadmin"),
			Client: &http.Client{
				Jar:     jar,
				Timeout: 15 * time.Second,
			},
		}
	})
	m.RegisterOperator("ldap", func() operator.Operator {
		return &ldapop.LDAPOperator{
			BaseOperator: operator.NewBaseOperator("ldap"),
		}
	})

	// First-party sources
	m.RegisterSource("ldap", func() source.Source {
		return &ldapsrc.LDAPSource{
			BaseSource: source.NewBaseSource("ldap"),
		}
	})
	m.RegisterSource("authentik", func() source.Source {
		return &authentiksrc.AuthentikSource{
			BaseSource: source.NewBaseSource("authentik"),
		}
	})

	m.loadDatabase()

	go m.runWatchLoop("identitysources")
	go m.runWatchLoop("synctargets")
	go m.runLeaderElection(cfg.Etcd.Name)

	return m
}

func (m *Manager) GetIdentitySources() []manifest.IdentitySource {
	sources := make([]manifest.IdentitySource, 0, m.activeSources.Size())

	m.activeSources.Range(func(_ string, value *ActiveSource) bool {
		sources = append(sources, *value.manifest)
		return true
	})

	return sources
}

func (m *Manager) GetSyncTargets() []manifest.SyncTarget {
	targets := make([]manifest.SyncTarget, 0, m.activeOperators.Size())

	m.activeOperators.Range(func(_ string, value *ActiveOperator) bool {
		targets = append(targets, *value.manifest)
		return true
	})

	return targets
}

func (m *Manager) RegisterSource(name string, src func() source.Source) {
	m.sourceFactory.Store(name, src)
}

func (m *Manager) RegisterOperator(name string, op func() operator.Operator) {
	m.operatorFactory.Store(name, op)
}

func (m *Manager) AddIdentitySource(src *manifest.IdentitySource) error {
	newSource, ok := m.sourceFactory.Load(src.Spec.Type)
	if !ok {
		return fmt.Errorf("source %s not found", src.Spec.Type)
	}

	var opSrc source.Source
	if src.Spec.PluginSource != nil {
		var err error
		opSrc, err = m.pluginManager.LoadPluginSource(
			m.shutdownCtx,
			src.Spec.PluginSource,
		)
		if err != nil {
			return fmt.Errorf("failed to load plugin: %w", err)
		}
	} else {
		opSrc = newSource()
	}

	err := opSrc.Initialize(m.shutdownCtx, src.Spec.Config)
	if err != nil {
		return fmt.Errorf("failed to initialize source operator: %w", err)
	}

	activeSource := &ActiveSource{
		Source:   opSrc,
		manifest: src,
		cache:    cache.NewStore(),
	}

	m.activeSources.Store(src.Name, activeSource)
	return nil
}

func (m *Manager) AddSyncTarget(target *manifest.SyncTarget) error {
	if _, ok := m.activeSources.Load(target.Spec.SourceRef); !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	newTarget, ok := m.operatorFactory.Load(target.Spec.Operator)
	if !ok && target.Spec.PluginSource == nil {
		return fmt.Errorf("operator %s not found", target.Spec.Operator)
	}

	var op operator.Operator
	if target.Spec.PluginSource != nil {
		var err error
		op, err = m.pluginManager.LoadPluginOperator(
			m.shutdownCtx,
			target.Spec.PluginSource,
		)
		if err != nil {
			return fmt.Errorf("failed to load plugin: %w", err)
		}
	} else {
		op = newTarget()
	}

	if err := op.Initialize(m.shutdownCtx, target.Spec.Config); err != nil {
		return fmt.Errorf("failed to initialize plugin operator: %w", err)
	}

	activeTarget := &ActiveOperator{
		Operator: op,
		manifest: target,
		cache:    cache.NewStore(),
	}

	_, existed := m.activeOperators.LoadAndStore(target.Name, activeTarget)
	if !existed {
		// Start ticker for this target if manager is already running
		select {
		case <-m.shutdownCtx.Done():
			// Manager is shut down, don't start ticker
		default:
			m.startTargetTicker(target.Name)
		}
	}

	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting controller manager",
		zap.Int("workers", m.cfg.Workers.ReconcileWorkers))

	for i := 0; i < m.cfg.Workers.ReconcileWorkers; i++ {
		m.wg.Add(1)
		go m.worker(ctx, i)
	}

	m.activeOperators.Range(func(name string, _ *ActiveOperator) bool {
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

			if err := m.Reconcile(task.targetName); err != nil {
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
				if _, ok := m.activeOperators.Load(targetName); !ok {
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

func (m *Manager) GetIdentitySource(name string) (source.Source, bool) {
	return m.activeSources.Load(name)
}

func (m *Manager) RemoveIdentitySource(name string) {
	m.activeSources.Delete(name)
}

func (m *Manager) RemoveSyncTarget(name string) {
	m.activeOperators.Delete(name)
	m.logger.Info("Removed SyncTarget from active reconciliation", zap.String("name", name))
}

func (m *Manager) TriggerReconciliation(targetName string) error {
	if _, ok := m.activeOperators.Load(targetName); !ok {
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
