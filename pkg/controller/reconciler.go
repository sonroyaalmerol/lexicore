package controller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
)

func (m *Manager) computeSourceDataHash(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (string, error) {
	data := struct {
		Identities map[string]source.Identity
		Groups     map[string]source.Group
	}{
		Identities: identities,
		Groups:     groups,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal source data: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

func (m *Manager) hasSourceDataChanged(sourceRef string, currentHash string) bool {
	previousHash, exists := m.sourceDataHashes.Load(sourceRef)
	if !exists {
		return true
	}

	return previousHash != currentHash
}

func (m *Manager) reconcileBatch(sourceRef, batchID string) error {
	src, ok := m.activeSources.Load(sourceRef)
	if !ok {
		return fmt.Errorf("source %s not found", sourceRef)
	}

	startTime := time.Now()
	m.logger.Info(
		"Fetching source data for batch",
		zap.String("source", sourceRef),
		zap.String("batchID", batchID),
	)

	identities, groups, err := m.fetchSourceData(src)
	if err != nil {
		return fmt.Errorf("failed to fetch source data from %s: %w", sourceRef, err)
	}

	m.logger.Info(
		"Fetched source data for batch",
		zap.String("source", sourceRef),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	currentHash, err := m.computeSourceDataHash(identities, groups)
	if err != nil {
		m.logger.Warn("Failed to compute source data hash", zap.Error(err))
	}

	targets := m.getTargetsForSource(sourceRef)
	if len(targets) == 0 {
		m.logger.Warn("No targets found for source", zap.String("source", sourceRef))
		return nil
	}

	dataChanged := m.hasSourceDataChanged(sourceRef, currentHash)

	if !dataChanged {
		skippedCount := 0
		for targetName, target := range targets {
			if target.ShouldSkipUnchangedSync() {
				m.logger.Info(
					"Skipping sync - no source data changes detected",
					zap.String("target", targetName),
					zap.String("source", sourceRef),
				)
				target.lastReconciled = time.Now()
				skippedCount++
			}
		}

		if skippedCount == len(targets) {
			m.logger.Info(
				"All targets skipped - no source data changes",
				zap.String("source", sourceRef),
				zap.Int("skipped", skippedCount),
			)
			return nil
		}
	}

	if dataChanged && currentHash != "" {
		m.sourceDataHashes.Store(sourceRef, currentHash)
	}

	return m.reconcileMultipleTargets(targets, identities, groups, sourceRef, batchID, dataChanged)
}

func (m *Manager) reconcile(targetName string) error {
	target, src, err := m.loadTargetAndSource(targetName)
	if err != nil {
		return err
	}

	identities, groups, err := m.fetchSourceData(src)
	if err != nil {
		return fmt.Errorf("failed to fetch source data: %w", err)
	}

	sourceRef := target.manifest.Spec.SourceRef
	currentHash, err := m.computeSourceDataHash(identities, groups)
	if err != nil {
		m.logger.Warn("Failed to compute source data hash", zap.Error(err))
	}

	dataChanged := m.hasSourceDataChanged(sourceRef, currentHash)

	if !dataChanged && target.ShouldSkipUnchangedSync() {
		m.logger.Info(
			"Skipping sync - no source data changes detected",
			zap.String("target", targetName),
			zap.String("source", sourceRef),
		)
		target.lastReconciled = time.Now()
		return nil
	}

	if dataChanged && currentHash != "" {
		m.sourceDataHashes.Store(sourceRef, currentHash)
	}

	return m.reconcileTarget(targetName, target, identities, groups, false)
}

func (m *Manager) reconcilePartial(targetName string, identityUIDs, groupGIDs []string) error {
	target, src, err := m.loadTargetAndSource(targetName)
	if err != nil {
		return err
	}

	partialFetcher, supportsPartial := src.Source.(source.PartialFetchCapable)
	if !supportsPartial {
		m.logger.Warn(
			"Source doesn't support partial fetch, falling back to full sync",
			zap.String("source", target.manifest.Spec.SourceRef),
		)
		return m.reconcile(targetName)
	}

	startTime := time.Now()
	m.logger.Info(
		"Starting partial reconciliation",
		zap.String("target", targetName),
		zap.Int("identityUIDs", len(identityUIDs)),
		zap.Int("groupGIDs", len(groupGIDs)),
	)

	identities, groups, err := m.fetchPartialSourceData(partialFetcher, identityUIDs, groupGIDs)
	if err != nil {
		return err
	}

	m.logger.Debug(
		"Fetched partial source data",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	return m.reconcileTarget(targetName, target, identities, groups, true, identityUIDs, groupGIDs)
}

func (m *Manager) reconcileTarget(
	targetName string,
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	isPartial bool,
	partialParams ...[]string,
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
		partialParams...,
	)
	if err != nil {
		if isPartial && err.Error() == "partial sync not implemented, use full sync instead" {
			m.logger.Info(
				"Operator doesn't support partial sync, falling back to full sync",
				zap.String("target", targetName),
			)
			return m.reconcile(targetName)
		}

		m.updateTargetStatus(targetName, false, fmt.Sprintf("Sync failed: %v", err), 0, 0)
		return fmt.Errorf("failed to sync to target: %w", err)
	}

	counts := result.SummaryCounts()
	errCount := counts["TOTAL_ERRORS"]
	statusMsg := fmt.Sprintf("%s sync completed successfully", syncType)
	if errCount > 0 {
		statusMsg = fmt.Sprintf("%s sync completed with %d errors", syncType, errCount)
	}

	m.updateTargetStatus(
		targetName,
		errCount == 0,
		statusMsg,
		len(transformedIdentities),
		len(transformedGroups),
	)

	m.logger.Info(
		"Reconciliation completed",
		zap.String("target", targetName),
		zap.String("type", syncType),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities_created", counts["IDENTITY_CREATE"]),
		zap.Int("identities_updated", counts["IDENTITY_UPDATE"]),
		zap.Int("identities_deleted", counts["IDENTITY_DELETE"]),
		zap.Int("group_adds", counts["IDENTITY_GROUP_ADD"]),
		zap.Int("group_rems", counts["IDENTITY_GROUP_REMOVE"]),
		zap.Int("errors", errCount),
	)

	m.generateAuditReportIfNeeded(target, targetName, result)

	return nil
}

func (m *Manager) loadTargetAndSource(targetName string) (*ActiveOperator, *ActiveSource, error) {
	target, ok := m.activeOperators.Load(targetName)
	if !ok {
		return nil, nil, fmt.Errorf("failed to load operator for %s", targetName)
	}

	src, ok := m.activeSources.Load(target.manifest.Spec.SourceRef)
	if !ok {
		return nil, nil, fmt.Errorf(
			"failed to load source %s for %s",
			target.manifest.Spec.SourceRef,
			targetName,
		)
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
		if strPrefix, ok := anyPrefix.(string); ok {
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
	partialParams ...[]string,
) (*operator.SyncResult, error) {
	if isPartial && len(partialParams) == 2 {
		state := &operator.PartialSyncState{
			Identities:            identities,
			Groups:                groups,
			DryRun:                target.manifest.Spec.DryRun,
			RequestedIdentityUIDs: partialParams[0],
			RequestedGroupGIDs:    partialParams[1],
		}
		return target.PartialSync(m.shutdownCtx, state)
	}

	state := &operator.SyncState{
		Identities: identities,
		Groups:     groups,
		DryRun:     target.manifest.Spec.DryRun,
	}
	return target.Sync(m.shutdownCtx, state)
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

func (m *Manager) reconcileMultipleTargets(
	targets map[string]*ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	sourceRef, batchID string,
	dataChanged bool,
) error {
	successCount := 0
	errorCount := 0
	skippedCount := 0

	for targetName, target := range targets {
		if _, loaded := m.reconcilingTargets.LoadOrStore(targetName, true); loaded {
			m.logger.Info(
				"Skipping target in batch - already being reconciled",
				zap.String("target", targetName),
				zap.String("source", sourceRef),
			)
			skippedCount++
			continue
		}

		if !dataChanged && target.ShouldSkipUnchangedSync() {
			m.logger.Info(
				"Skipping target - no source data changes",
				zap.String("target", targetName),
				zap.String("source", sourceRef),
			)
			target.lastReconciled = time.Now()
			m.reconcilingTargets.Delete(targetName)
			skippedCount++
			continue
		}

		err := m.reconcileTarget(targetName, target, identities, groups, false)
		m.reconcilingTargets.Delete(targetName)

		if err != nil {
			m.logger.Error(
				"Target reconciliation failed in batch",
				zap.String("target", targetName),
				zap.String("source", sourceRef),
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
		zap.String("source", sourceRef),
		zap.String("batchID", batchID),
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

func (m *Manager) generateAuditReportIfNeeded(
	target *ActiveOperator,
	targetName string,
	result *operator.SyncResult,
) {
	if target.manifest.Spec.Config["generateAuditReport"] != true {
		return
	}

	if err := os.MkdirAll("/var/lib/lexicore/csv", os.ModeDir); err != nil {
		m.logger.Error("Failed to create audit report directory", zap.Error(err))
		return
	}

	filename := fmt.Sprintf(
		"/var/lib/lexicore/csv/audit_log_%s_%d.xls",
		targetName,
		time.Now().Unix(),
	)
	file, err := os.Create(filename)
	if err != nil {
		m.logger.Error("Failed to create audit report file", zap.Error(err))
		return
	}

	operator.GenerateCSV(file, result.Entries)
}
