package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
)

func (m *Manager) computeManifestHash(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func (m *Manager) computeSourceDataHash(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (string, error) {
	h := sha256.New()

	keys := make([]string, 0, len(identities)+len(groups))

	for k := range identities {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		jsonData, err := json.Marshal(identities[k])
		if err != nil {
			return "", fmt.Errorf("failed to marshal identity: %w", err)
		}
		h.Write([]byte(k))
		h.Write(jsonData)
	}

	keys = keys[:0]
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		jsonData, err := json.Marshal(groups[k])
		if err != nil {
			return "", fmt.Errorf("failed to marshal group: %w", err)
		}
		h.Write([]byte(k))
		h.Write(jsonData)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (m *Manager) hasSourceDataChanged(targetName string, currentHash string) bool {
	previousHash, exists := m.sourceDataHashes.Load(targetName)
	if !exists {
		return true
	}

	return previousHash != currentHash
}

func (m *Manager) reconcileBatch(task reconcileTask) error {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 15*time.Second)
	defer cancel()

	src, ok := m.activeSources.Load(task.targetName)
	if !ok {
		return fmt.Errorf("source %s not found", task.targetName)
	}

	freshSourceManifest, err := m.loadSourceManifest(ctx, task.targetName)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh source manifest, using cached version",
			zap.String("source", task.targetName),
			zap.Error(err),
		)
	} else {
		newHash, err := m.computeManifestHash(freshSourceManifest)
		if err != nil {
			m.logger.Warn("Failed to hash source manifest", zap.Error(err))
		} else {
			if prev, exists := m.sourceManifestHashes.Load(task.targetName); !exists || prev != newHash {
				m.logger.Info(
					"Source manifest changed, invalidating source data cache",
					zap.String("source", task.targetName),
				)
				m.activeOperators.Range(func(name string, t *ActiveOperator) bool {
					if t.manifest.Spec.SourceRef == task.targetName {
						m.sourceDataHashes.Delete(name)
					}
					return true
				})
				m.sourceManifestHashes.Store(task.targetName, newHash)
			}
		}
		src.manifest = freshSourceManifest
	}

	allTargetManifests, err := m.db.GetSyncTargets(ctx)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh target manifests for batch, using cached versions",
			zap.String("source", task.targetName),
			zap.Error(err),
		)
	} else {
		freshByName := make(map[string]*manifest.SyncTarget, len(allTargetManifests))
		for _, t := range allTargetManifests {
			freshByName[t.Name] = t
		}
		m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
			if fresh, ok := freshByName[name]; ok {
				newHash, err := m.computeManifestHash(fresh)
				if err != nil {
					m.logger.Warn("Failed to hash target manifest", zap.Error(err))
				} else {
					if prev, exists := m.targetManifestHashes.Load(name); !exists || prev != newHash {
						m.logger.Info(
							"Target manifest changed, invalidating source data cache",
							zap.String("target", name),
						)
						m.sourceDataHashes.Delete(name)
						m.targetManifestHashes.Store(name, newHash)
					}
				}
				target.manifest = fresh
			}
			return true
		})
	}

	startTime := time.Now()
	m.logger.Info(
		"Fetching source data for batch",
		zap.String("source", task.targetName),
		zap.String("batchID", task.batchID),
	)

	identities, groups, err := m.fetchSourceData(src)
	if err != nil {
		return fmt.Errorf("failed to fetch source data from %s: %w", task.targetName, err)
	}

	m.logger.Info(
		"Fetched source data for batch",
		zap.String("source", task.targetName),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	currentHash, hashErr := m.computeSourceDataHash(identities, groups)
	if hashErr != nil {
		m.logger.Warn("Failed to compute source data hash", zap.Error(hashErr))
	}

	targets := m.getTargetsForSource(task.targetName)
	if len(targets) == 0 {
		m.logger.Warn("No targets found for source", zap.String("source", task.targetName))
		return nil
	}

	return m.reconcileMultipleTargets(task, targets, identities, groups, currentHash, hashErr)
}

func (m *Manager) reconcile(task reconcileTask) error {
	target, src, err := m.loadTargetAndSource(task.targetName)
	if err != nil {
		return err
	}

	identities, groups, err := m.fetchSourceData(src)
	if err != nil {
		return fmt.Errorf("failed to fetch source data: %w", err)
	}

	currentHash, hashErr := m.computeSourceDataHash(identities, groups)
	if hashErr != nil {
		m.logger.Warn("Failed to compute source data hash", zap.Error(hashErr))
	}

	dataChanged := hashErr != nil || m.hasSourceDataChanged(task.targetName, currentHash)

	if !dataChanged && target.ShouldSkipUnchangedSync() && !task.forced {
		m.logger.Info(
			"Skipping sync - no source data changes detected",
			zap.String("target", task.targetName),
			zap.String("source", target.manifest.Spec.SourceRef),
		)
		target.lastReconciled = time.Now()
		return nil
	}

	if hashErr == nil && currentHash != "" {
		m.sourceDataHashes.Store(task.targetName, currentHash)
	}

	return m.reconcileTarget(task, target, identities, groups, false)
}

func (m *Manager) reconcileMultipleTargets(
	task reconcileTask,
	targets map[string]*ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	currentHash string,
	hashErr error,
) error {
	successCount := 0
	errorCount := 0
	skippedCount := 0

	for targetName, target := range targets {
		if _, loaded := m.reconcilingTargets.LoadOrStore(targetName, true); loaded {
			m.logger.Info(
				"Skipping target in batch - already being reconciled",
				zap.String("target", targetName),
				zap.String("source", task.targetName),
			)
			skippedCount++
			continue
		}

		dataChanged := hashErr != nil || m.hasSourceDataChanged(targetName, currentHash)

		if !dataChanged && target.ShouldSkipUnchangedSync() && !task.forced {
			m.logger.Info(
				"Skipping target - no source data changes",
				zap.String("target", targetName),
				zap.String("source", task.targetName),
			)
			target.lastReconciled = time.Now()
			m.reconcilingTargets.Delete(targetName)
			skippedCount++
			continue
		}

		if hashErr == nil && currentHash != "" {
			m.sourceDataHashes.Store(targetName, currentHash)
		}

		err := m.reconcileTarget(task, target, identities, groups, false)
		m.reconcilingTargets.Delete(targetName)

		if err != nil {
			m.logger.Error(
				"Target reconciliation failed in batch",
				zap.String("target", targetName),
				zap.String("source", task.targetName),
				zap.Error(err),
			)
			errorCount++
		} else {
			target.lastReconciled = time.Now()
			successCount++
		}
	}

	m.logger.Info(
		"Batch reconciliation completed",
		zap.String("source", task.targetName),
		zap.String("batchID", task.batchID),
		zap.Int("successful", successCount),
		zap.Int("failed", errorCount),
		zap.Int("skipped", skippedCount),
	)

	if errorCount > 0 {
		return fmt.Errorf(
			"batch reconciliation completed with %d/%d failures",
			errorCount,
			len(targets)-skippedCount,
		)
	}

	return nil
}

func (m *Manager) reconcilePartial(task reconcileTask) error {
	target, src, err := m.loadTargetAndSource(task.targetName)
	if err != nil {
		return err
	}

	partialFetcher, supportsPartial := src.Source.(source.PartialFetchCapable)
	if !supportsPartial {
		m.logger.Warn(
			"Source doesn't support partial fetch, falling back to full sync",
			zap.String("source", target.manifest.Spec.SourceRef),
		)
		return m.reconcile(task)
	}

	startTime := time.Now()
	m.logger.Info(
		"Starting partial reconciliation",
		zap.String("target", task.targetName),
		zap.Int("identityUIDs", len(task.identityUIDs)),
		zap.Int("groupGIDs", len(task.groupGIDs)),
	)

	identities, groups, err := m.fetchPartialSourceData(partialFetcher, task.identityUIDs, task.groupGIDs)
	if err != nil {
		return err
	}

	m.logger.Debug(
		"Fetched partial source data",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	return m.reconcileTarget(task, target, identities, groups, true)
}

func (m *Manager) reconcileTarget(
	task reconcileTask,
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	isPartial bool,
) error {
	startTime := time.Now()
	syncType := "full"
	if isPartial {
		syncType = "partial"
	}

	m.logger.Info(
		"Starting reconciliation",
		zap.String("target", task.targetName),
		zap.String("type", syncType),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	transformedIdentities, transformedGroups, err := m.applyTransformations(
		target,
		identities,
		groups,
	)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	result, err := m.syncToTarget(
		target,
		transformedIdentities,
		transformedGroups,
		isPartial,
		task.identityUIDs,
		task.groupGIDs,
	)
	if err != nil {
		if isPartial && err.Error() == "partial sync not implemented, use full sync instead" {
			m.logger.Info(
				"Operator doesn't support partial sync, falling back to full sync",
				zap.String("target", task.targetName),
			)
			return m.reconcile(task)
		}

		m.updateTargetStatus(task.targetName, false, fmt.Sprintf("Sync failed: %v", err), 0, 0)
		return fmt.Errorf("failed to sync to target: %w", err)
	}

	counts := result.Counts()
	errCount := counts["ERRORS"]
	statusMsg := fmt.Sprintf("%s sync completed successfully", syncType)
	if errCount > 0 {
		statusMsg = fmt.Sprintf("%s sync completed with %d errors", syncType, errCount)
	}

	m.updateTargetStatus(
		task.targetName,
		errCount == 0,
		statusMsg,
		len(transformedIdentities),
		len(transformedGroups),
	)

	m.logger.Info(
		"Reconciliation completed",
		zap.String("target", task.targetName),
		zap.String("type", syncType),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("updated", counts[string(operator.ActionUpdate)]),
		zap.Int("skipped", counts[string(operator.ActionSkip)]),
		zap.Int("errors", errCount),
	)

	m.generateAuditReportIfNeeded(target, task.targetName, result)

	return nil
}

func (m *Manager) loadTargetManifest(
	ctx context.Context,
	targetName string,
) (*manifest.SyncTarget, error) {
	targets, err := m.db.GetSyncTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load sync targets: %w", err)
	}
	for _, t := range targets {
		if t.Name == targetName {
			return t, nil
		}
	}
	return nil, fmt.Errorf("target %s not found in store", targetName)
}

func (m *Manager) loadSourceManifest(
	ctx context.Context,
	sourceRef string,
) (*manifest.IdentitySource, error) {
	sources, err := m.db.GetIdentitySources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity sources: %w", err)
	}
	for _, s := range sources {
		if s.Name == sourceRef {
			return s, nil
		}
	}
	return nil, fmt.Errorf("source %s not found in store", sourceRef)
}

func (m *Manager) loadTargetAndSource(
	targetName string,
) (*ActiveOperator, *ActiveSource, error) {
	ctx, cancel := context.WithTimeout(m.shutdownCtx, 15*time.Second)
	defer cancel()

	target, ok := m.activeOperators.Load(targetName)
	if !ok {
		return nil, nil, fmt.Errorf("failed to load operator for %s", targetName)
	}

	freshManifest, err := m.loadTargetManifest(ctx, targetName)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh target manifest, using cached version",
			zap.String("target", targetName),
			zap.Error(err),
		)
	} else {
		newHash, err := m.computeManifestHash(freshManifest)
		if err != nil {
			m.logger.Warn("Failed to hash target manifest", zap.Error(err))
		} else {
			if prev, exists := m.targetManifestHashes.Load(targetName); !exists || prev != newHash {
				m.logger.Info(
					"Target manifest changed, invalidating source data cache",
					zap.String("target", targetName),
				)
				m.sourceDataHashes.Delete(targetName)
				m.targetManifestHashes.Store(targetName, newHash)
			}
		}
		target.manifest = freshManifest
	}

	src, ok := m.activeSources.Load(target.manifest.Spec.SourceRef)
	if !ok {
		return nil, nil, fmt.Errorf(
			"failed to load source %s for %s",
			target.manifest.Spec.SourceRef,
			targetName,
		)
	}

	freshSourceManifest, err := m.loadSourceManifest(ctx, src.manifest.Name)
	if err != nil {
		m.logger.Warn(
			"Failed to refresh source manifest, using cached version",
			zap.String("source", src.manifest.Name),
			zap.Error(err),
		)
	} else {
		newHash, err := m.computeManifestHash(freshSourceManifest)
		if err != nil {
			m.logger.Warn("Failed to hash source manifest", zap.Error(err))
		} else {
			sourceKey := src.manifest.Name
			if prev, exists := m.sourceManifestHashes.Load(sourceKey); !exists || prev != newHash {
				m.logger.Info(
					"Source manifest changed, invalidating source data cache",
					zap.String("source", sourceKey),
				)
				m.sourceDataHashes.Delete(targetName)
				m.sourceManifestHashes.Store(sourceKey, newHash)
			}
		}
		src.manifest = freshSourceManifest
	}

	return target, src, nil
}

func (m *Manager) fetchSourceData(src *ActiveSource) (
	map[string]source.Identity,
	map[string]source.Group,
	error,
) {
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

func (m *Manager) fetchPartialSourceData(
	partialFetcher source.PartialFetchCapable,
	identityUIDs, groupGIDs []string,
) (
	map[string]source.Identity,
	map[string]source.Group,
	error,
) {
	identities, err := partialFetcher.GetIdentitiesByUIDs(m.shutdownCtx, identityUIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := partialFetcher.GetGroupsByGIDs(m.shutdownCtx, groupGIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get groups: %w", err)
	}

	additionalGroups, err := m.fetchAdditionalGroupsForEnrichment(
		partialFetcher,
		identities,
		groups,
	)
	if err != nil {
		m.logger.Warn("Failed to fetch additional groups for enrichment", zap.Error(err))
	} else {
		maps.Copy(groups, additionalGroups)
	}

	return identities, groups, nil
}

func (m *Manager) fetchAdditionalGroupsForEnrichment(
	partialFetcher source.PartialFetchCapable,
	identities map[string]source.Identity,
	existingGroups map[string]source.Group,
) (map[string]source.Group, error) {
	additionalGroupGIDs := make(map[string]bool)
	for _, identity := range identities {
		for _, gid := range identity.Groups {
			if _, exists := existingGroups[gid]; !exists {
				additionalGroupGIDs[gid] = true
			}
		}
	}

	if len(additionalGroupGIDs) == 0 {
		return nil, nil
	}

	gids := make([]string, 0, len(additionalGroupGIDs))
	for gid := range additionalGroupGIDs {
		gids = append(gids, gid)
	}

	return partialFetcher.GetGroupsByGIDs(m.shutdownCtx, gids)
}

func (m *Manager) applyTransformations(
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (
	map[string]source.Identity,
	map[string]source.Group,
	error,
) {
	attrPrefix := ""
	if anyPrefix, ok := target.manifest.Spec.Config["attributePrefix"]; ok {
		if strPrefix, ok := anyPrefix.Value().(string); ok {
			attrPrefix = strPrefix
		}
	}

	pipeline, err := transformer.NewPipeline(target.manifest.Spec.Transformers, attrPrefix)
	if err != nil {
		return nil, nil, err
	}

	tctx := transformer.NewContext(m.shutdownCtx, target.manifest.Spec.Config)
	return pipeline.Execute(tctx, identities, groups)
}

func (m *Manager) syncToTarget(
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	isPartial bool,
	identityUIDs []string,
	groupGIDs []string,
) (*operator.SyncResult, error) {
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
		return auditor, target.PartialSync(m.shutdownCtx, state)
	}

	state := &operator.SyncState{
		Identities: identities,
		Groups:     groups,
		DryRun:     target.manifest.Spec.DryRun,
		Result:     auditor,
	}
	return auditor, target.Sync(m.shutdownCtx, state)
}

func (m *Manager) getTargetsForSource(sourceRef string) map[string]*ActiveOperator {
	targets := make(map[string]*ActiveOperator)
	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if target.manifest.Spec.SourceRef == sourceRef {
			targets[name] = target
		}
		return true
	})
	return targets
}

func (m *Manager) generateAuditReportIfNeeded(
	target *ActiveOperator,
	targetName string,
	result *operator.SyncResult,
) {
	if target.manifest.Spec.Config["generateAuditReport"].Value() != true {
		return
	}

	counts := result.Counts()
	if counts[string(operator.ActionUpdate)] == 0 {
		m.logger.Info(
			"Skipping audit report - no changes detected",
			zap.String("target", targetName),
		)
		return
	}

	switch m.cfg.Audit.Mode {
	case "email":
		m.sendAuditEmail(targetName, result)
	default:
		m.saveAuditFile(target, targetName, result)
	}
}

func (m *Manager) saveAuditFile(
	target *ActiveOperator,
	targetName string,
	result *operator.SyncResult,
) {
	if err := os.MkdirAll(m.cfg.Audit.XLSDir, 0755); err != nil {
		m.logger.Error("Failed to create audit report directory", zap.Error(err))
		return
	}

	filename := fmt.Sprintf("audit_log_%s_%d.xlsx", targetName, time.Now().Unix())
	fullPath := filepath.Join(m.cfg.Audit.XLSDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		m.logger.Error("Failed to create audit report file", zap.Error(err))
		return
	}
	defer file.Close()

	if err := operator.ExportToExcel(file, result.Entries()); err != nil {
		m.logger.Error("Failed to write audit report", zap.Error(err))
	}
}

func (m *Manager) sendAuditEmail(targetName string, result *operator.SyncResult) {
	cfg := m.cfg.Audit.Email
	sender := operator.NewEmailAuditSender(
		cfg.SMTP.Host,
		cfg.SMTP.Port,
		cfg.SMTP.Username,
		cfg.SMTP.Password,
		cfg.SMTP.TLSMode,
		cfg.From,
		cfg.To,
		cfg.SubjectFmt,
	)

	if err := sender.Send(targetName, result.Entries()); err != nil {
		m.logger.Error(
			"Failed to send audit report email",
			zap.String("target", targetName),
			zap.Error(err),
		)
		return
	}

	m.logger.Info(
		"Audit report sent via email",
		zap.String("target", targetName),
		zap.Strings("to", cfg.To),
	)
}
