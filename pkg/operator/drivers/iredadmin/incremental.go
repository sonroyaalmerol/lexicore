package iredadmin

import (
	"context"
	"fmt"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *IRedAdminOperator) processCreates(
	ctx context.Context,
	state *operator.IncrementalSyncState,
	result *operator.SyncResult,
	workers int,
) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, id := range state.IdentitiesToCreate {
		if id.Email == "" || id.Disabled {
			continue
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		wg.Add(1)
		go func(identity source.Identity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if state.DryRun {
				o.LogInfo("[DRY RUN] Would create user %s", identity.Email)
				result.IdentitiesCreated.Add(1)
			} else {
				enriched := o.EnrichIdentity(identity, state.AllGroups)
				if err := o.createUser(ctx, &enriched); err != nil {
					o.LogError(fmt.Errorf("create user %s: %w", identity.Email, err))
					result.ErrCount.Add(1)
				} else {
					result.IdentitiesCreated.Add(1)
				}
			}
		}(id)
	}

	wg.Wait()
}

func (o *IRedAdminOperator) processUpdates(
	ctx context.Context,
	state *operator.IncrementalSyncState,
	result *operator.SyncResult,
	workers int,
) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, id := range state.IdentitiesToUpdate {
		if id.Email == "" {
			continue
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		wg.Add(1)
		go func(identity source.Identity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			enriched := o.EnrichIdentity(identity, state.AllGroups)

			if state.DryRun {
				o.LogInfo("[DRY RUN] Would update user %s", identity.Email)
				result.IdentitiesUpdated.Add(1)
			} else {
				userData, err := o.getUserData(ctx, identity.Email)
				if err != nil {
					o.LogError(fmt.Errorf("get user data %s: %w", identity.Email, err))
					result.ErrCount.Add(1)
					return
				}

				if userData == nil {
					if err := o.createUser(ctx, &enriched); err != nil {
						o.LogError(fmt.Errorf("create missing user %s: %w", identity.Email, err))
						result.ErrCount.Add(1)
					} else {
						result.IdentitiesUpdated.Add(1)
					}
					return
				}

				changes := o.detectChanges(&enriched, userData)
				if len(changes) > 0 {
					if err := o.updateUser(ctx, &enriched, changes); err != nil {
						o.LogError(fmt.Errorf("update user %s: %w", identity.Email, err))
						result.ErrCount.Add(1)
					} else {
						result.IdentitiesUpdated.Add(1)
					}
				}
			}
		}(id)
	}

	wg.Wait()
}

func (o *IRedAdminOperator) processReprocesses(
	ctx context.Context,
	state *operator.IncrementalSyncState,
	result *operator.SyncResult,
	workers int,
) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, id := range state.IdentitiesToReprocess {
		if id.Email == "" || id.Disabled {
			continue
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		wg.Add(1)
		go func(identity source.Identity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			enriched := o.EnrichIdentity(identity, state.AllGroups)

			if state.DryRun {
				o.LogInfo("[DRY RUN] Would reprocess user %s due to group changes", identity.Email)
				result.IdentitiesReprocessed.Add(1)
			} else {
				userData, err := o.getUserData(ctx, identity.Email)
				if err != nil {
					o.LogError(fmt.Errorf("get user data for reprocess %s: %w", identity.Email, err))
					result.ErrCount.Add(1)
					return
				}

				if userData == nil {
					o.LogInfo("User %s doesn't exist, skipping reprocessing", identity.Email)
					return
				}

				changes := o.detectChanges(&enriched, userData)
				if len(changes) > 0 {
					if err := o.updateUser(ctx, &enriched, changes); err != nil {
						o.LogError(fmt.Errorf("reprocess user %s: %w", identity.Email, err))
						result.ErrCount.Add(1)
					} else {
						result.IdentitiesReprocessed.Add(1)
					}
				}
			}
		}(id)
	}

	wg.Wait()
}

func (o *IRedAdminOperator) processDeletes(
	ctx context.Context,
	state *operator.IncrementalSyncState,
	result *operator.SyncResult,
	workers int,
) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, id := range state.IdentitiesToDelete {
		if id.Email == "" {
			continue
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		wg.Add(1)
		go func(identity source.Identity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if state.DryRun {
				o.LogInfo("[DRY RUN] Would delete user %s", identity.Email)
				result.IdentitiesDeleted.Add(1)
			} else {
				if err := o.deleteUser(ctx, identity.Email); err != nil {
					o.LogError(fmt.Errorf("delete user %s: %w", identity.Email, err))
					result.ErrCount.Add(1)
				} else {
					result.IdentitiesDeleted.Add(1)
				}
			}
		}(id)
	}

	wg.Wait()
}
