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

	source, ok := m.activeSources.Load(target.manifest.Spec.SourceRef)
	if !ok {
		return fmt.Errorf("failed to load source %s for %s", target.manifest.Spec.SourceRef, targetName)
	}

	pipeline, err := transformer.NewPipeline(target.manifest.Spec.Transformers)
	if err != nil {
		return fmt.Errorf("failed to create transformer pipeline: %w", err)
	}

	startTime := time.Now()
	m.logger.Info("Starting reconciliation")

	identities, err := source.GetIdentities(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := source.GetGroups(m.shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to get groups: %w", err)
	}

	m.logger.Info(
		"Fetched from source",
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	tctx := transformer.NewContext(m.shutdownCtx, target.manifest.Spec.Config)
	transformedIdentities, transformedGroups, err := pipeline.Execute(tctx, identities, groups)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	m.logger.Info(
		"Applied transformations",
		zap.Int("identities", len(transformedIdentities)),
		zap.Int("groups", len(transformedGroups)),
	)

	state := &operator.SyncState{
		Identities: identities,
		Groups:     groups,
		DryRun:     target.manifest.Spec.DryRun,
	}

	result, err := target.Sync(m.shutdownCtx, state)
	if err != nil {
		m.updateTargetStatus(targetName, false, fmt.Sprintf("Sync failed: %v", err), 0, 0)
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
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("identities_created", result.IdentitiesCreated),
		zap.Int("identities_updated", result.IdentitiesUpdated),
		zap.Int("identities_deleted", result.IdentitiesDeleted),
		zap.Int("groups_created", result.GroupsCreated),
		zap.Int("groups_updated", result.GroupsUpdated),
		zap.Int("groups_deleted", result.GroupsDeleted),
		zap.Int("errors", len(result.Errors)),
	)

	if len(result.Errors) > 0 {
		return fmt.Errorf("sync completed with %d errors", len(result.Errors))
	}

	return nil
}
