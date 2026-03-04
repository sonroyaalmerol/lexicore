package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	sourceName string
	targetName string
	immediate  bool
	forced     bool
	batchID    string

	// For partial sync
	partialSync  bool
	identityUIDs []string
	groupGIDs    []string
}

type webhookReconcileTask struct {
	sourceName string
	event      *source.WebhookEvent
}

type pendingUpdate struct {
	identities map[string]bool
	groups     map[string]bool
	mu         sync.Mutex
	timer      *time.Timer
}

type ActiveOperator struct {
	operator.Operator
	manifest       *manifest.SyncTarget
	lastReconciled time.Time
	ctx            context.Context
	closeCtx       context.CancelFunc
}

type ActiveSource struct {
	source.Source
	manifest *manifest.IdentitySource
	ctx      context.Context
	closeCtx context.CancelFunc
}

type Manager struct {
	sourceManifestHashes *xsync.Map[string, string]
	targetManifestHashes *xsync.Map[string, string]

	sourceFactory      *xsync.Map[string, func() source.Source]
	operatorFactory    *xsync.Map[string, func() operator.Operator]
	activeOperators    *xsync.Map[string, *ActiveOperator]
	activeSources      *xsync.Map[string, *ActiveSource]
	reconcilingTargets *xsync.Map[string, bool]
	db                 store.Store

	pluginManager *PluginManager

	queue  chan reconcileTask
	cfg    *config.Config
	logger *zap.Logger

	wg          sync.WaitGroup
	shutdownCtx context.Context
	shutdown    context.CancelFunc

	webhookQueue          chan webhookReconcileTask
	pendingWebhookUpdates *xsync.Map[string, *pendingUpdate]

	sourceDataHashes *xsync.Map[string, string]
}

func NewManager(
	ctx context.Context,
	cfg *config.Config,
	db store.Store,
	logger *zap.Logger,
) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{
		db:                    db,
		sourceManifestHashes:  xsync.NewMap[string, string](),
		targetManifestHashes:  xsync.NewMap[string, string](),
		sourceFactory:         xsync.NewMap[string, func() source.Source](),
		operatorFactory:       xsync.NewMap[string, func() operator.Operator](),
		activeOperators:       xsync.NewMap[string, *ActiveOperator](),
		activeSources:         xsync.NewMap[string, *ActiveSource](),
		reconcilingTargets:    xsync.NewMap[string, bool](),
		pluginManager:         newPluginManager(cfg.Server.PluginsDir),
		queue:                 make(chan reconcileTask, cfg.Workers.QueueSize),
		cfg:                   cfg,
		logger:                logger,
		shutdownCtx:           ctx,
		shutdown:              cancel,
		webhookQueue:          make(chan webhookReconcileTask, cfg.Workers.QueueSize),
		pendingWebhookUpdates: xsync.NewMap[string, *pendingUpdate](),
		sourceDataHashes:      xsync.NewMap[string, string](),
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

	m.wg.Go(func() {
		m.webhookProcessor(ctx)
	})

	return m
}

func (m *Manager) reloadFromStore() {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 30*time.Second)
	defer cancel()

	sources, err := m.db.GetIdentitySources(ctx)
	if err != nil {
		m.logger.Error("Failed to load identity sources from store", zap.Error(err))
	} else {
		seen := make(map[string]bool)
		for _, src := range sources {
			seen[src.Name] = true
			if _, ok := m.activeSources.Load(src.Name); !ok {
				if err := m.AddIdentitySource(src); err != nil {
					m.logger.Error("Failed to add identity source",
						zap.String("name", src.Name),
						zap.Error(err))
				}
			}
		}
		m.activeSources.Range(func(name string, _ *ActiveSource) bool {
			if !seen[name] {
				m.RemoveIdentitySource(name)
			}
			return true
		})
	}

	targets, err := m.db.GetSyncTargets(ctx)
	if err != nil {
		m.logger.Error("Failed to load sync targets from store", zap.Error(err))
	} else {
		seen := make(map[string]bool)
		for _, t := range targets {
			seen[t.Name] = true
			if _, ok := m.activeOperators.Load(t.Name); !ok {
				if err := m.AddSyncTarget(t); err != nil {
					m.logger.Error("Failed to add sync target",
						zap.String("name", t.Name),
						zap.Error(err))
				}
			}
		}
		m.activeOperators.Range(func(name string, _ *ActiveOperator) bool {
			if !seen[name] {
				m.RemoveSyncTarget(name)
			}
			return true
		})
	}
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

	activeSource := &ActiveSource{
		Source:   opSrc,
		manifest: src,
		ctx:      ctx,
		closeCtx: cancel,
	}

	m.activeSources.Store(src.Name, activeSource)
	m.logger.Info(
		"Adding new active identity source instance",
		zap.String("src", src.Name),
	)
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

	activeTarget := &ActiveOperator{
		Operator:       op,
		manifest:       target,
		lastReconciled: time.Time{}, // Zero value = never reconciled
		ctx:            ctx,
		closeCtx:       cancel,
	}

	m.activeOperators.Store(target.Name, activeTarget)
	m.logger.Info(
		"Adding new active sync target instance",
		zap.String("target", target.Name),
	)

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

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	m.reloadFromStore()
	m.scheduleReconciliations()

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
			m.reloadFromStore()
			m.scheduleReconciliations()
		}
	}
}

func (m *Manager) scheduleReconciliations() {
	now := time.Now()
	batchID := fmt.Sprintf("batch-%d", now.Unix())

	sourceToTargets := make(map[string][]string)

	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if now.Sub(target.lastReconciled) >= m.cfg.DefaultSyncPeriod {
			sourceRef := target.manifest.Spec.SourceRef
			sourceToTargets[sourceRef] = append(
				sourceToTargets[sourceRef],
				name,
			)
		} else {
			next := target.lastReconciled.Add(m.cfg.DefaultSyncPeriod)
			m.logger.Info(
				"target reconciliation",
				zap.String("target", target.Name()),
				zap.Duration("interval", m.cfg.DefaultSyncPeriod),
				zap.Time("next", next),
				zap.Duration("secondsleftUntilNext", next.Sub(now)),
			)
		}
		return true
	})

	if len(sourceToTargets) == 0 {
		return
	}

	scheduled := 0
	skipped := 0

	for sourceRef, targetNames := range sourceToTargets {
		task := reconcileTask{
			sourceName: sourceRef,
			batchID:    batchID,
		}

		select {
		case m.queue <- task:
			scheduled += len(targetNames)
			m.logger.Debug(
				"Scheduled batch reconciliation",
				zap.String("source", sourceRef),
				zap.Strings("targets", targetNames),
			)
		case <-m.shutdownCtx.Done():
			return
		default:
			skipped += len(targetNames)
			m.logger.Warn(
				"Queue full, skipping batch reconciliation",
				zap.String("source", sourceRef),
				zap.Strings("targets", targetNames),
			)
		}
	}

	if scheduled > 0 || skipped > 0 {
		m.logger.Debug(
			"Scheduled reconciliations",
			zap.Int("scheduled", scheduled),
			zap.Int("skipped", skipped),
			zap.Int("batches", len(sourceToTargets)),
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

			if err := m.executeReconciliation(task); err != nil {
				logger.Error(
					"Reconciliation failed",
					zap.String("source", task.sourceName),
					zap.String("target", task.targetName),
					zap.String("batchID", task.batchID),
					zap.Error(err),
				)
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
		if op.closeCtx != nil {
			op.closeCtx()
		}
	}
}

func (m *Manager) RemoveSyncTarget(name string) {
	op, ok := m.activeOperators.LoadAndDelete(name)
	if ok && op != nil {
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
	op, ok := m.activeOperators.Load(targetName)
	if !ok {
		return fmt.Errorf("target %s not found", targetName)
	}

	select {
	case m.queue <- reconcileTask{sourceName: op.manifest.Spec.SourceRef, targetName: targetName, immediate: true, forced: true}:
		return nil
	case <-m.shutdownCtx.Done():
		return fmt.Errorf("manager is shutting down")
	default:
		return fmt.Errorf("queue is full")
	}
}
