package controller

import (
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/cache"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
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

	// Check if we can do incremental sync
	sourceCache := src.cache
	targetCache := target.cache
	canDoIncremental := !targetCache.IsEmpty()

	// Try incremental fetch if source supports it
	var identities map[string]source.Identity
	var groups map[string]source.Group
	var fetchType string

	if canDoIncremental {
		if detector, ok := src.Source.(source.ChangeDetector); ok && detector.SupportsChangeDetection() {
			snapshot := sourceCache.GetSnapshot()
			changes, _, err := detector.GetChangesSince(m.shutdownCtx, snapshot.CapturedAt)

			if err == nil && !changes.FullSync {
				// Successfully got incremental changes
				identities, groups = m.applyChangesToSnapshot(sourceCache, changes)
				fetchType = "incremental"

				m.logger.Info(
					"Fetched incremental changes from source",
					zap.String("target", targetName),
					zap.Int("modified_identities", len(changes.ModifiedIdentities)),
					zap.Int("deleted_identities", len(changes.DeletedIdentities)),
					zap.Int("modified_groups", len(changes.ModifiedGroups)),
					zap.Int("deleted_groups", len(changes.DeletedGroups)),
				)
			} else {
				// Fall back to full sync
				canDoIncremental = false
				m.logger.Info(
					"Falling back to full sync",
					zap.String("target", targetName),
					zap.Error(err),
				)
			}
		}
	}

	// Full fetch if incremental not available
	if !canDoIncremental || identities == nil {
		fetchType = "full"
		identities, err = src.GetIdentities(m.shutdownCtx)
		if err != nil {
			return fmt.Errorf("failed to get identities: %w", err)
		}

		groups, err = src.GetGroups(m.shutdownCtx)
		if err != nil {
			return fmt.Errorf("failed to get groups: %w", err)
		}

		m.logger.Info(
			"Fetched full data from source",
			zap.String("target", targetName),
			zap.String("fetch_type", fetchType),
			zap.Int("identities", len(identities)),
			zap.Int("groups", len(groups)),
		)
	}

	tctx := transformer.NewContext(m.shutdownCtx, target.manifest.Spec.Config)
	transformedIdentities, transformedGroups, err := pipeline.Execute(
		tctx,
		identities,
		groups,
	)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	// Compute diff against target cache
	diff, err := targetCache.UpdateSnapshot(transformedIdentities, transformedGroups)
	if err != nil {
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	// Update source cache
	if fetchType == "full" {
		_, err = sourceCache.UpdateSnapshot(identities, groups)
		if err != nil {
			m.logger.Warn(
				"Failed to update source cache",
				zap.String("target", targetName),
				zap.Error(err),
			)
		}
	}

	m.logger.Info(
		"Computed changes",
		zap.String("target", targetName),
		zap.String("fetch_type", fetchType),
		zap.Int("identities_to_create", len(diff.IdentitiesToCreate)),
		zap.Int("identities_to_update", len(diff.IdentitiesToUpdate)),
		zap.Int("identities_to_delete", len(diff.IdentitiesToDelete)),
		zap.Int("groups_to_create", len(diff.GroupsToCreate)),
		zap.Int("groups_to_update", len(diff.GroupsToUpdate)),
		zap.Int("groups_to_delete", len(diff.GroupsToDelete)),
		zap.Int("total_changes", diff.TotalChanges),
	)

	// Skip sync if no changes
	if !diff.HasChanges {
		m.logger.Info(
			"No changes detected, skipping sync",
			zap.String("target", targetName),
		)
		m.updateTargetStatus(
			targetName,
			true,
			"No changes detected",
			len(transformedIdentities),
			len(transformedGroups),
		)
		return nil
	}

	// Perform sync - use incremental if operator supports it
	var result *operator.SyncResult

	if incrementalOp, ok := target.Operator.(operator.IncrementalOperator); ok && incrementalOp.SupportsIncrementalSync() {
		// Use incremental sync
		incrementalState := &operator.IncrementalSyncState{
			IdentitiesToCreate: toIdentitySlice(diff.IdentitiesToCreate),
			IdentitiesToUpdate: toIdentitySlice(diff.IdentitiesToUpdate),
			IdentitiesToDelete: toIdentitySlice(diff.IdentitiesToDelete),
			GroupsToCreate:     toGroupSlice(diff.GroupsToCreate),
			GroupsToUpdate:     toGroupSlice(diff.GroupsToUpdate),
			GroupsToDelete:     toGroupSlice(diff.GroupsToDelete),
			DryRun:             target.manifest.Spec.DryRun,
		}

		result, err = incrementalOp.SyncIncremental(m.shutdownCtx, incrementalState)
		if err != nil {
			m.updateTargetStatus(
				targetName,
				false,
				fmt.Sprintf("Incremental sync failed: %v", err),
				0,
				0,
			)
			return fmt.Errorf("failed to perform incremental sync: %w", err)
		}

		m.logger.Info("Used incremental sync", zap.String("target", targetName))
	} else {
		// Fall back to full sync
		state := &operator.SyncState{
			Identities: transformedIdentities,
			Groups:     transformedGroups,
			DryRun:     target.manifest.Spec.DryRun,
		}

		result, err = target.Sync(m.shutdownCtx, state)
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

		m.logger.Info("Used full sync", zap.String("target", targetName))
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
		zap.String("fetch_type", fetchType),
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

func (m *Manager) applyChangesToSnapshot(
	cache *cache.Store,
	changes *source.Changes,
) (map[string]source.Identity, map[string]source.Group) {
	snapshot := cache.GetSnapshot()

	// Start with current snapshot data
	identities := make(map[string]source.Identity, len(snapshot.Identities))
	for key, cached := range snapshot.Identities {
		identities[key] = cached.Data
	}

	groups := make(map[string]source.Group, len(snapshot.Groups))
	for key, cached := range snapshot.Groups {
		groups[key] = cached.Data
	}

	// Apply modifications
	for _, identity := range changes.ModifiedIdentities {
		key := identityKey(identity)
		identities[key] = identity
	}

	for _, group := range changes.ModifiedGroups {
		key := groupKey(group)
		groups[key] = group
	}

	// Apply deletions
	for _, uid := range changes.DeletedIdentities {
		delete(identities, uid)
	}

	for _, gid := range changes.DeletedGroups {
		delete(groups, gid)
	}

	return identities, groups
}

func identityKey(identity source.Identity) string {
	if identity.UID != "" {
		return identity.UID
	}
	if identity.Username != "" {
		return identity.Username
	}
	return identity.Email
}

func groupKey(group source.Group) string {
	if group.GID != "" {
		return group.GID
	}
	return group.Name
}

func toIdentitySlice(ptrs []*source.Identity) []source.Identity {
	result := make([]source.Identity, len(ptrs))
	for i, ptr := range ptrs {
		result[i] = *ptr
	}
	return result
}

func toGroupSlice(ptrs []*source.Group) []source.Group {
	result := make([]source.Group, len(ptrs))
	for i, ptr := range ptrs {
		result[i] = *ptr
	}
	return result
}
