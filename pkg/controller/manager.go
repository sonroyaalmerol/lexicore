package controller

import (
	"context"
	"fmt"
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
	manifest       *manifest.SyncTarget
	cache          *cache.Cache
	lastReconciled time.Time
	ctx            context.Context
	closeCtx       context.CancelFunc
}

type ActiveSource struct {
	source.Source
	manifest *manifest.IdentitySource
	cache    *cache.Cache
	ctx      context.Context
	closeCtx context.CancelFunc
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

func NewManager(
	ctx context.Context,
	cfg *config.Config,
	db *store.EtcdStore,
	logger *zap.Logger,
) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{
		db:              db,
		sourceFactory:   xsync.NewMap[string, func() source.Source](),
		operatorFactory: xsync.NewMap[string, func() operator.Operator](),
		activeOperators: xsync.NewMap[string, *ActiveOperator](),
		activeSources:   xsync.NewMap[string, *ActiveSource](),
		pluginManager:   newPluginManager(cfg.Server.PluginsDir),
		queue:           make(chan reconcileTask, cfg.Workers.QueueSize),
		cfg:             cfg,
		logger:          logger,
		shutdownCtx:     ctx,
		shutdown:        cancel,
	}

	// First-party operators
	m.RegisterOperator("active-directory", func() operator.Operator {
		return &adop.ADOperator{
			BaseOperator: operator.NewBaseOperator("active-directory", m.logger),
		}
	})
	m.RegisterOperator("dovecot-acl", func() operator.Operator {
		return &dovecotop.DovecotOperator{
			BaseOperator: operator.NewBaseOperator("dovecot-acl", m.logger),
		}
	})
	m.RegisterOperator("iredadmin", func() operator.Operator {
		return &iredadminop.IRedAdminOperator{
			BaseOperator: operator.NewBaseOperator("iredadmin", m.logger),
		}
	})
	m.RegisterOperator("ldap", func() operator.Operator {
		return &ldapop.LDAPOperator{
			BaseOperator: operator.NewBaseOperator("ldap", m.logger),
		}
	})

	// First-party sources
	m.RegisterSource("ldap", func() source.Source {
		return &ldapsrc.LDAPSource{
			BaseSource: source.NewBaseSource("ldap", m.logger),
		}
	})
	m.RegisterSource("authentik", func() source.Source {
		return &authentiksrc.AuthentikSource{
			BaseSource: source.NewBaseSource("authentik", m.logger),
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
	var newSource func() source.Source

	if src.Spec.Type != "plugin" {
		var ok bool
		newSource, ok = m.sourceFactory.Load(src.Spec.Type)
		if !ok {
			return fmt.Errorf("source %s not found", src.Spec.Type)
		}
	}

	ctx, cancel := context.WithCancel(m.shutdownCtx)

	var opSrc source.Source
	if src.Spec.PluginSource != nil {
		var err error
		opSrc, err = m.pluginManager.loadPluginSource(
			ctx,
			src.Spec.PluginSource,
			m.logger,
		)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to load plugin: %w", err)
		}
	} else {
		if newSource == nil {
			cancel()
			return fmt.Errorf("source %s not found", src.Spec.Type)
		}
		opSrc = newSource()
	}

	err := opSrc.Initialize(ctx, src.Spec.Config)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to initialize source operator: %w", err)
	}

	cacheStore := cache.NewCache(m.db, fmt.Sprintf("source/%s", src.Name), m.logger)
	if err := cacheStore.LoadFromEtcd(ctx); err != nil {
		m.logger.Error("failed to load cache from etcd", zap.Error(err))
	}
	go cacheStore.WatchSnapshots(ctx)

	activeSource := &ActiveSource{
		Source:   opSrc,
		manifest: src,
		cache:    cacheStore,
		ctx:      ctx,
		closeCtx: cancel,
	}

	m.activeSources.Store(src.Name, activeSource)
	return nil
}

func (m *Manager) AddSyncTarget(target *manifest.SyncTarget) error {
	if _, ok := m.activeSources.Load(target.Spec.SourceRef); !ok {
		return fmt.Errorf("source %s not found", target.Spec.SourceRef)
	}

	ctx, cancel := context.WithCancel(m.shutdownCtx)

	var newTarget func() operator.Operator

	if target.Spec.Operator != "plugin" {
		var ok bool
		newTarget, ok = m.operatorFactory.Load(target.Spec.Operator)
		if !ok && target.Spec.PluginSource == nil {
			cancel()
			return fmt.Errorf("operator %s not found", target.Spec.Operator)
		}
	}

	var op operator.Operator
	if target.Spec.PluginSource != nil {
		var err error
		op, err = m.pluginManager.loadPluginOperator(
			ctx,
			target.Spec.PluginSource,
			m.logger,
		)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to load plugin: %w", err)
		}
	} else {
		if newTarget == nil {
			cancel()
			return fmt.Errorf("operator %s not found", target.Spec.Operator)
		}
		op = newTarget()
	}

	if err := op.Initialize(ctx, target.Spec.Config); err != nil {
		cancel()
		return fmt.Errorf("failed to initialize plugin operator: %w", err)
	}

	cacheStore := cache.NewCache(m.db, fmt.Sprintf("operator/%s", target.Name), m.logger)
	if err := cacheStore.LoadFromEtcd(ctx); err != nil {
		m.logger.Error("failed to load cache from etcd", zap.Error(err))
	}
	go cacheStore.WatchSnapshots(ctx)

	activeTarget := &ActiveOperator{
		Operator:       op,
		manifest:       target,
		cache:          cacheStore,
		lastReconciled: time.Time{}, // Zero value = never reconciled
		ctx:            ctx,
		closeCtx:       cancel,
	}

	m.activeOperators.Store(target.Name, activeTarget)

	select {
	case m.queue <- reconcileTask{targetName: target.Name, immediate: true}:
	case <-m.shutdownCtx.Done():
	default:
		m.logger.Warn(
			"Queue full, new target will be reconciled on next tick",
			zap.String("target", target.Name),
		)
	}

	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info(
		"Starting controller manager",
		zap.Int("workers", m.cfg.Workers.ReconcileWorkers),
	)

	for i := 0; i < m.cfg.Workers.ReconcileWorkers; i++ {
		m.wg.Add(1)
		go m.worker(ctx, i)
	}

	m.wg.Add(1)
	go m.globalTicker(ctx)

	<-ctx.Done()

	m.logger.Info("Shutting down controller manager")
	m.shutdown()

	close(m.queue)

	m.wg.Wait()

	m.logger.Info("Controller manager stopped")
	return nil
}

func (m *Manager) globalTicker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.DefaultSyncPeriod)
	defer ticker.Stop()

	m.logger.Info(
		"Global ticker started",
		zap.Duration("interval", m.cfg.DefaultSyncPeriod),
	)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Global ticker stopped")
			return
		case <-ticker.C:
			m.scheduleReconciliations()
		}
	}
}

func (m *Manager) scheduleReconciliations() {
	now := time.Now()
	scheduled := 0
	skipped := 0

	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if now.Sub(target.lastReconciled) >= m.cfg.DefaultSyncPeriod {
			select {
			case m.queue <- reconcileTask{targetName: name, immediate: false}:
				scheduled++
			case <-m.shutdownCtx.Done():
				return false
			default:
				skipped++
				m.logger.Warn(
					"Queue full, skipping reconciliation",
					zap.String("target", name),
				)
			}
		}
		return true
	})

	if scheduled > 0 || skipped > 0 {
		m.logger.Debug(
			"Scheduled reconciliations",
			zap.Int("scheduled", scheduled),
			zap.Int("skipped", skipped),
		)
	}
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

			if err := m.reconcile(task.targetName); err != nil {
				logger.Error(
					"Reconciliation failed",
					zap.String("target", task.targetName),
					zap.Error(err),
				)
			} else {
				logger.Debug(
					"Reconciliation completed",
					zap.String("target", task.targetName),
				)

				if target, ok := m.activeOperators.Load(task.targetName); ok {
					target.lastReconciled = time.Now()
				}
			}
		}
	}
}

func (m *Manager) GetIdentitySource(name string) (source.Source, bool) {
	return m.activeSources.Load(name)
}

func (m *Manager) RemoveIdentitySource(name string) {
	op, ok := m.activeSources.LoadAndDelete(name)
	if ok && op != nil {
		if op.cache != nil {
			op.cache.Clear(context.Background())
		}
		if op.closeCtx != nil {
			op.closeCtx()
		}
	}
}

func (m *Manager) RemoveSyncTarget(name string) {
	op, ok := m.activeOperators.LoadAndDelete(name)
	if ok && op != nil {
		if op.cache != nil {
			op.cache.Clear(context.Background())
		}
		if op.closeCtx != nil {
			op.closeCtx()
		}
	}

	m.logger.Info(
		"Removed SyncTarget from active reconciliation",
		zap.String("name", name),
	)
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
