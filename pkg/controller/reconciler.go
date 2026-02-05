package controller

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/cache"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/transformer"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Reconciler struct {
	target      *manifest.SyncTarget
	source      source.Source
	operator    operator.Operator
	transformer *transformer.Pipeline
	cache       *cache.Store
	logger      *zap.Logger
}

func NewReconciler(
	target *manifest.SyncTarget,
	src source.Source,
	op operator.Operator,
	logger *zap.Logger,
) (*Reconciler, error) {
	pipeline, err := transformer.NewPipeline(target.Spec.Transformers)
	if err != nil {
		return nil, fmt.Errorf("failed to create transformer pipeline: %w", err)
	}

	return &Reconciler{
		target:      target,
		source:      src,
		operator:    op,
		transformer: pipeline,
		cache:       cache.NewStore(target.Name),
		logger:      logger.With(zap.String("target", target.Name)),
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
	startTime := time.Now()
	r.logger.Info("Starting reconciliation")

	identities, groups, err := r.fetchFromSource(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch from source: %w", err)
	}

	r.logger.Info(
		"Fetched from source",
		zap.Int("identities", len(identities)),
		zap.Int("groups", len(groups)),
	)

	transformedIdentities, transformedGroups, err := r.applyTransformations(
		ctx,
		identities,
		groups,
	)
	if err != nil {
		return fmt.Errorf("failed to apply transformations: %w", err)
	}

	r.logger.Info(
		"Applied transformations",
		zap.Int("identities", len(transformedIdentities)),
		zap.Int("groups", len(transformedGroups)),
	)

	if !r.cache.IsFresh() {
		diff, err := r.cache.CalculateDiff(transformedIdentities, transformedGroups)
		if err != nil {
			return fmt.Errorf("failed to calculate diff: %w", err)
		}

		r.logger.Info(
			"Calculated diff",
			zap.Int("identities_to_create", len(diff.IdentitiesToCreate)),
			zap.Int("identities_to_update", len(diff.IdentitiesToUpdate)),
			zap.Int("identities_to_delete", len(diff.IdentitiesToDelete)),
			zap.Int("groups_to_create", len(diff.GroupsToCreate)),
			zap.Int("groups_to_update", len(diff.GroupsToUpdate)),
			zap.Int("groups_to_delete", len(diff.GroupsToDelete)),
		)

		if !diff.HasChanges() {
			r.logger.Info("No changes detected, skipping sync")
			r.updateStatus(true, "No changes detected", 0, 0)
			return nil
		}
	}

	result, err := r.syncToTarget(ctx, transformedIdentities, transformedGroups)
	if err != nil {
		r.updateStatus(false, fmt.Sprintf("Sync failed: %v", err), 0, 0)
		return fmt.Errorf("failed to sync to target: %w", err)
	}

	if !r.target.Spec.DryRun {
		if err := r.cache.UpdateIdentities(transformedIdentities); err != nil {
			r.logger.Error("Failed to update identity cache", zap.Error(err))
			return fmt.Errorf("failed to update identity cache: %w", err)
		}
		if err := r.cache.UpdateGroups(transformedGroups); err != nil {
			r.logger.Error("Failed to update group cache", zap.Error(err))
			return fmt.Errorf("failed to update group cache: %w", err)
		}
	}

	r.updateStatus(
		true,
		"Sync completed successfully",
		len(transformedIdentities),
		len(transformedGroups),
	)

	r.logger.Info(
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

func (r *Reconciler) fetchFromSource(
	ctx context.Context,
) (map[string]source.Identity, map[string]source.Group, error) {
	identities, err := r.source.GetIdentities(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get identities: %w", err)
	}

	groups, err := r.source.GetGroups(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get groups: %w", err)
	}

	return identities, groups, nil
}

func (r *Reconciler) applyTransformations(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	tctx := transformer.NewContext(ctx, r.target.Spec.Config)

	return r.transformer.Execute(tctx, identities, groups)
}

func (r *Reconciler) syncToTarget(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (*operator.SyncResult, error) {
	state := &operator.SyncState{
		Identities: identities,
		Groups:     groups,
		DryRun:     r.target.Spec.DryRun,
	}

	return r.operator.Sync(ctx, state)
}

func (r *Reconciler) updateStatus(
	success bool,
	message string,
	identityCount int,
	groupCount int,
) {
	r.target.Status.LastSync = v1.NewTime(time.Now())
	if success {
		r.target.Status.Status = "Success"
	} else {
		r.target.Status.Status = "Failed"
	}
	r.target.Status.Message = message
	r.target.Status.IdentityCount = identityCount
	r.target.Status.GroupCount = groupCount
}
