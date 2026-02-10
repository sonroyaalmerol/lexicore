package controller

import (
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
)

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

	pipeline, err := transformer.NewPipeline(target.manifest.Spec.Transformers)
	if err != nil {
		return fmt.Errorf("failed to create transformer pipeline: %w", err)
	}

	startTime := time.Now()
	m.logger.Info("Starting reconciliation", zap.String("target", targetName))

	identities, err := src.GetIdentities(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := src.GetGroups(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get groups: %w", err)
	}

	m.logger.Info(
		"Fetched full data from source",
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
