package controller

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
)

func (m *Manager) executeReconciliation(task reconcileTask) error {
	if task.batchID != "" {
		return m.executeBatch(task)
	}
	return m.executeSingle(task)
}

func (m *Manager) executeBatch(task reconcileTask) error {
	if _, loaded := m.reconcilingTargets.LoadOrStore(
		"batch:"+task.sourceName, true,
	); loaded {
		m.logger.Info(
			"Skipping batch - already in progress",
			zap.String("source", task.sourceName),
		)
		return nil
	}
	defer m.reconcilingTargets.Delete("batch:" + task.sourceName)

	m.refreshSourceManifest(task.sourceName)
	m.refreshAllTargetManifests()

	src, ok := m.activeSources.Load(task.sourceName)
	if !ok {
		return fmt.Errorf("source %s not found", task.sourceName)
	}

	identities, groups, err := m.getIdentitiesAndGroups(src, nil, nil)
	if err != nil {
		return fmt.Errorf(
			"failed to fetch source data from %s: %w",
			task.sourceName,
			err,
		)
	}

	currentHash, hashErr := m.computeSourceDataHash(identities, groups)
	if hashErr != nil {
		m.logger.Warn(
			"Failed to compute source data hash",
			zap.Error(hashErr),
		)
	}

	targets := m.getTargetsForSource(task.sourceName)
	if len(targets) == 0 {
		m.logger.Warn(
			"No targets found for source",
			zap.String("source", task.sourceName),
		)
		return nil
	}

	successCount := int32(0)
	errorCount := int32(0)
	skippedCount := int32(0)

	var batchWg sync.WaitGroup

	lastReconciled := time.Now()

	for targetName, target := range targets {
		if _, loaded := m.reconcilingTargets.LoadOrStore(
			targetName, true,
		); loaded {
			m.logger.Info(
				"Skipping target in batch - already being reconciled",
				zap.String("target", targetName),
			)
			skippedCount++
			continue
		}

		dataChanged := hashErr != nil ||
			m.hasSourceDataChanged(targetName, currentHash)

		if !dataChanged &&
			target.ShouldSkipUnchangedSync() &&
			!task.forced {
			m.logger.Info(
				"Skipping target - no source data changes",
				zap.String("target", targetName),
			)
			target.lastReconciled = lastReconciled
			m.reconcilingTargets.Delete(targetName)
			skippedCount++
			continue
		}

		if hashErr == nil && currentHash != "" {
			m.sourceDataHashes.Store(targetName, currentHash)
		}

		batchWg.Go(func() {
			err := m.reconcileTarget(
				targetName,
				target,
				identities,
				groups,
				false,
				nil,
				nil,
			)
			m.reconcilingTargets.Delete(targetName)

			if err != nil {
				m.logger.Error(
					"Target reconciliation failed in batch",
					zap.String("target", targetName),
					zap.Error(err),
				)
				atomic.AddInt32(&errorCount, 1)
			} else {
				target.lastReconciled = lastReconciled
				atomic.AddInt32(&successCount, 1)
			}
		})
	}

	batchWg.Wait()

	m.logger.Info(
		"Batch reconciliation completed",
		zap.String("source", task.sourceName),
		zap.String("batchID", task.batchID),
		zap.Int32("successful", successCount),
		zap.Int32("failed", errorCount),
		zap.Int32("skipped", skippedCount),
	)

	if errorCount > 0 {
		return fmt.Errorf(
			"batch completed with %d/%d failures",
			errorCount,
			len(targets)-int(skippedCount),
		)
	}

	return nil
}

func (m *Manager) executeSingle(task reconcileTask) error {
	if _, loaded := m.reconcilingTargets.LoadOrStore(
		task.targetName, true,
	); loaded {
		m.logger.Info(
			"Skipping reconciliation - already in progress",
			zap.String("target", task.targetName),
		)
		return nil
	}
	defer m.reconcilingTargets.Delete(task.targetName)

	target, ok := m.activeOperators.Load(task.targetName)
	if !ok {
		return fmt.Errorf("target %s not found", task.targetName)
	}

	m.refreshTargetManifest(task.targetName, target)

	src, ok := m.activeSources.Load(target.manifest.Spec.SourceRef)
	if !ok {
		return fmt.Errorf(
			"source %s not found for target %s",
			target.manifest.Spec.SourceRef,
			task.targetName,
		)
	}

	m.refreshSourceManifest(src.manifest.Name)

	lastReconciled := time.Now()

	isPartial := task.partialSync
	var identities map[string]source.Identity
	var groups map[string]source.Group
	var err error

	if isPartial {
		_, supportsPartial := src.Source.(source.PartialFetchCapable)
		if !supportsPartial {
			m.logger.Warn(
				"Source doesn't support partial fetch, falling back to full",
				zap.String("source", target.manifest.Spec.SourceRef),
			)
			isPartial = false
		}
	}

	if isPartial {
		fetcher := src.Source.(source.PartialFetchCapable)
		identities, groups, err = m.getIdentitiesAndGroups(
			src,
			fetcher,
			&partialFetchOpts{
				identityUIDs: task.identityUIDs,
				groupGIDs:    task.groupGIDs,
			},
		)
	} else {
		identities, groups, err = m.getIdentitiesAndGroups(src, nil, nil)
	}

	if err != nil {
		return fmt.Errorf("failed to fetch source data: %w", err)
	}

	if !isPartial {
		currentHash, hashErr := m.computeSourceDataHash(
			identities, groups,
		)
		if hashErr != nil {
			m.logger.Warn(
				"Failed to compute source data hash",
				zap.Error(hashErr),
			)
		}

		dataChanged := hashErr != nil ||
			m.hasSourceDataChanged(task.targetName, currentHash)

		if !dataChanged &&
			target.ShouldSkipUnchangedSync() &&
			!task.forced {
			m.logger.Info(
				"Skipping sync - no source data changes detected",
				zap.String("target", task.targetName),
			)
			target.lastReconciled = lastReconciled
			return nil
		}

		if hashErr == nil && currentHash != "" {
			m.sourceDataHashes.Store(task.targetName, currentHash)
		}
	}

	err = m.reconcileTarget(
		task.targetName,
		target,
		identities,
		groups,
		isPartial,
		task.identityUIDs,
		task.groupGIDs,
	)
	if err != nil {
		return err
	}

	target.lastReconciled = lastReconciled
	return nil
}

type partialFetchOpts struct {
	identityUIDs []string
	groupGIDs    []string
}

func (m *Manager) getIdentitiesAndGroups(
	src *ActiveSource,
	partialFetcher source.PartialFetchCapable,
	partial *partialFetchOpts,
) (
	map[string]source.Identity,
	map[string]source.Group,
	error,
) {
	if partial != nil && partialFetcher != nil {
		identities, err := partialFetcher.GetIdentitiesByUIDs(
			m.shutdownCtx, partial.identityUIDs,
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get identities: %w", err,
			)
		}

		groups, err := partialFetcher.GetGroupsByGIDs(
			m.shutdownCtx, partial.groupGIDs,
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get groups: %w", err,
			)
		}

		additionalGIDs := make(map[string]bool)
		for _, identity := range identities {
			for _, gid := range identity.Groups {
				if _, exists := groups[gid]; !exists {
					additionalGIDs[gid] = true
				}
			}
		}
		if len(additionalGIDs) > 0 {
			gids := make([]string, 0, len(additionalGIDs))
			for gid := range additionalGIDs {
				gids = append(gids, gid)
			}
			extra, err := partialFetcher.GetGroupsByGIDs(
				m.shutdownCtx, gids,
			)
			if err != nil {
				m.logger.Warn(
					"Failed to fetch additional groups for enrichment",
					zap.Error(err),
				)
			} else {
				maps.Copy(groups, extra)
			}
		}

		return identities, groups, nil
	}

	identities, err := src.GetIdentities(m.shutdownCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := src.GetGroups(m.shutdownCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get groups: %w", err)
	}

	return identities, groups, nil
}

func (m *Manager) reconcileTarget(
	targetName string,
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	isPartial bool,
	identityUIDs []string,
	groupGIDs []string,
) error {
	startTime := time.Now()
	syncType := "full"
	if isPartial {
		syncType = "partial"
	}

	m.logger.Info(
		"Starting reconciliation",
		zap.String("target", targetName),
		zap.String("type", syncType),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	attrPrefix := ""
	if anyPrefix, ok := target.manifest.Spec.Config["attributePrefix"]; ok {
		if strPrefix, ok := anyPrefix.Value().(string); ok {
			attrPrefix = strPrefix
		}
	}

	pipeline, err := transformer.NewPipeline(
		target.manifest.Spec.Transformers, attrPrefix,
	)
	if err != nil {
		return fmt.Errorf("failed to build transformer pipeline: %w", err)
	}

	tctx := transformer.NewContext(
		m.shutdownCtx, target.manifest.Spec.Config,
	)
	identities, groups, err = pipeline.Execute(tctx, identities, groups)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	auditor := operator.NewSyncResult(m.logger, target.Name())

	if isPartial {
		state := &operator.PartialSyncState{
			Identities:            identities,
			Groups:                groups,
			DryRun:                target.manifest.Spec.DryRun,
			Result:                auditor,
			RequestedIdentityUIDs: identityUIDs,
			RequestedGroupGIDs:    groupGIDs,
		}
		err = target.PartialSync(m.shutdownCtx, state)
	} else {
		state := &operator.SyncState{
			Identities: identities,
			Groups:     groups,
			DryRun:     target.manifest.Spec.DryRun,
			Result:     auditor,
		}
		err = target.Sync(m.shutdownCtx, state)
	}

	if err != nil {
		if isPartial &&
			err.Error() == "partial sync not implemented, use full sync instead" {
			m.logger.Info(
				"Operator doesn't support partial sync, falling back to full",
				zap.String("target", targetName),
			)
			fullState := &operator.SyncState{
				Identities: identities,
				Groups:     groups,
				DryRun:     target.manifest.Spec.DryRun,
				Result:     auditor,
			}
			err = target.Sync(m.shutdownCtx, fullState)
		}

		if err != nil {
			m.updateTargetStatus(
				targetName, false,
				fmt.Sprintf("Sync failed: %v", err), 0, 0,
			)
			return fmt.Errorf("failed to sync to target: %w", err)
		}
	}

	counts := auditor.Counts()
	errCount := counts["ERRORS"]
	statusMsg := fmt.Sprintf("%s sync completed successfully", syncType)
	if errCount > 0 {
		statusMsg = fmt.Sprintf(
			"%s sync completed with %d errors", syncType, errCount,
		)
	}

	m.updateTargetStatus(
		targetName,
		errCount == 0,
		statusMsg,
		len(identities),
		len(groups),
	)

	m.logger.Info(
		"Reconciliation completed",
		zap.String("target", targetName),
		zap.String("type", syncType),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("updated", counts[string(operator.ActionUpdate)]),
		zap.Int("skipped", counts[string(operator.ActionSkip)]),
		zap.Int("errors", errCount),
	)

	m.generateAuditReport(target, targetName, auditor)

	return nil
}

func (m *Manager) refreshSourceManifest(sourceName string) {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 15*time.Second)
	defer cancel()

	fresh, err := m.loadManifestFromStore(ctx, sourceName, true)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh source manifest, using cached",
			zap.String("source", sourceName),
			zap.Error(err),
		)
		return
	}

	src, ok := m.activeSources.Load(sourceName)
	if !ok {
		return
	}

	newHash, err := m.computeManifestHash(fresh)
	if err != nil {
		m.logger.Warn("Failed to hash source manifest", zap.Error(err))
		return
	}

	if prev, exists := m.sourceManifestHashes.Load(sourceName); !exists || prev != newHash {
		m.logger.Info(
			"Source manifest changed, invalidating source data caches",
			zap.String("source", sourceName),
		)
		m.activeOperators.Range(func(name string, t *ActiveOperator) bool {
			if t.manifest.Spec.SourceRef == sourceName {
				m.sourceDataHashes.Delete(name)
			}
			return true
		})
		m.sourceManifestHashes.Store(sourceName, newHash)
	}

	src.manifest = fresh.(*manifest.IdentitySource)
}

func (m *Manager) refreshTargetManifest(
	targetName string,
	target *ActiveOperator,
) {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 15*time.Second)
	defer cancel()

	fresh, err := m.loadManifestFromStore(ctx, targetName, false)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh target manifest, using cached",
			zap.String("target", targetName),
			zap.Error(err),
		)
		return
	}

	newHash, err := m.computeManifestHash(fresh)
	if err != nil {
		m.logger.Warn("Failed to hash target manifest", zap.Error(err))
		return
	}

	if prev, exists := m.targetManifestHashes.Load(targetName); !exists || prev != newHash {
		m.logger.Info(
			"Target manifest changed, invalidating source data cache",
			zap.String("target", targetName),
		)
		m.sourceDataHashes.Delete(targetName)
		m.targetManifestHashes.Store(targetName, newHash)
	}

	target.manifest = fresh.(*manifest.SyncTarget)
}

func (m *Manager) refreshAllTargetManifests() {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 15*time.Second)
	defer cancel()

	allTargets, err := m.db.GetSyncTargets(ctx)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh target manifests",
			zap.Error(err),
		)
		return
	}

	freshByName := make(map[string]*manifest.SyncTarget, len(allTargets))
	for _, t := range allTargets {
		freshByName[t.Name] = t
	}

	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		fresh, ok := freshByName[name]
		if !ok {
			return true
		}

		newHash, err := m.computeManifestHash(fresh)
		if err != nil {
			m.logger.Warn("Failed to hash target manifest", zap.Error(err))
			return true
		}

		if prev, exists := m.targetManifestHashes.Load(name); !exists || prev != newHash {
			m.logger.Info(
				"Target manifest changed, invalidating source data cache",
				zap.String("target", name),
			)
			m.sourceDataHashes.Delete(name)
			m.targetManifestHashes.Store(name, newHash)
		}

		target.manifest = fresh
		return true
	})
}

func (m *Manager) loadManifestFromStore(
	ctx context.Context,
	name string,
	isSource bool,
) (any, error) {
	if isSource {
		sources, err := m.db.GetIdentitySources(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load sources: %w", err)
		}
		for _, s := range sources {
			if s.Name == name {
				return s, nil
			}
		}
		return nil, fmt.Errorf("source %s not found in store", name)
	}

	targets, err := m.db.GetSyncTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load targets: %w", err)
	}
	for _, t := range targets {
		if t.Name == name {
			return t, nil
		}
	}
	return nil, fmt.Errorf("target %s not found in store", name)
}

func (m *Manager) getTargetsForSource(
	sourceRef string,
) map[string]*ActiveOperator {
	targets := make(map[string]*ActiveOperator)
	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if target.manifest.Spec.SourceRef == sourceRef {
			targets[name] = target
		}
		return true
	})
	return targets
}
