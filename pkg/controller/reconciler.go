// reconcile.go
package controller

import (
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
)

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

	identities, err := src.GetIdentities(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get identities from %s: %w", sourceRef, err)
	}

	groups, err := src.GetGroups(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get groups from %s: %w", sourceRef, err)
	}

	m.logger.Info(
		"Fetched source data for batch",
		zap.String("source", sourceRef),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	var targets []*ActiveOperator
	var targetNames []string

	m.activeOperators.Range(func(name string, target *ActiveOperator) bool {
		if target.manifest.Spec.SourceRef == sourceRef {
			targets = append(targets, target)
			targetNames = append(targetNames, name)
		}
		return true
	})

	if len(targets) == 0 {
		m.logger.Warn(
			"No targets found for source",
			zap.String("source", sourceRef),
		)
		return nil
	}

	successCount := 0
	errorCount := 0

	for i, target := range targets {
		targetName := targetNames[i]

		if err := m.reconcileTarget(targetName, target, identities, groups); err != nil {
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
		zap.Duration("totalDuration", time.Since(startTime)),
	)

	if errorCount > 0 {
		return fmt.Errorf(
			"batch reconciliation completed with %d/%d failures",
			errorCount,
			len(targets),
		)
	}

	return nil
}

func (m *Manager) reconcile(targetName string) error {
	target, ok := m.activeOperators.Load(targetName)
	if !ok {
		return fmt.Errorf("failed to load operator for %s", targetName)
	}

	src, ok := m.activeSources.Load(target.manifest.Spec.SourceRef)
	if !ok {
		return fmt.Errorf(
			"failed to load source %s for %s",
			target.manifest.Spec.SourceRef,
			targetName,
		)
	}

	identities, err := src.GetIdentities(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := src.GetGroups(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get groups: %w", err)
	}

	return m.reconcileTarget(targetName, target, identities, groups)
}

func (m *Manager) reconcileTarget(
	targetName string,
	target *ActiveOperator,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) error {
	pipeline, err := transformer.NewPipeline(target.manifest.Spec.Transformers)
	if err != nil {
		return fmt.Errorf("failed to create transformer pipeline: %w", err)
	}

	startTime := time.Now()
	m.logger.Info("Starting reconciliation", zap.String("target", targetName))

	m.logger.Info(
		"Processing source data",
		zap.String("target", targetName),
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	tctx := transformer.NewContext(m.shutdownCtx, target.manifest.Spec.Config)
	transformedIdentities, transformedGroups, err := pipeline.Execute(
		tctx,
		identities,
		groups,
	)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	state := &operator.SyncState{
		Identities: transformedIdentities,
		Groups:     transformedGroups,
		DryRun:     target.manifest.Spec.DryRun,
	}

	result, err := target.Sync(m.shutdownCtx, state)
	if err != nil {
		m.updateTargetStatus(
			targetName,
			false,
			fmt.Sprintf("Sync failed: %v", err),
			0,
			0,
		)
		return fmt.Errorf("failed to sync to target: %w", err)
	}

	m.updateTargetStatus(
		targetName,
		true,
		"Sync completed successfully",
		len(transformedIdentities),
		len(transformedGroups),
	)

	m.logger.Info(
		"Reconciliation completed",
		zap.String("target", targetName),
		zap.Duration("duration", time.Since(startTime)),
		zap.Uint64("identities_created", result.IdentitiesCreated.Load()),
		zap.Uint64("identities_updated", result.IdentitiesUpdated.Load()),
		zap.Uint64("identities_deleted", result.IdentitiesDeleted.Load()),
		zap.Uint64("identities_reprocessed", result.IdentitiesReprocessed.Load()),
		zap.Uint64("groups_created", result.GroupsCreated.Load()),
		zap.Uint64("groups_updated", result.GroupsUpdated.Load()),
		zap.Uint64("groups_deleted", result.GroupsDeleted.Load()),
		zap.Uint64("errors", result.ErrCount.Load()),
	)

	if result.ErrCount.Load() > 0 {
		return fmt.Errorf("sync completed with %d errors", result.ErrCount.Load())
	}

	return nil
}
