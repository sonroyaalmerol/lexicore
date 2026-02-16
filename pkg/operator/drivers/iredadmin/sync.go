package iredadmin

import (
	"context"
	"fmt"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *IRedAdminOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	currentUsers, err := o.getUsers(ctx)
	if err != nil {
		return nil, err
	}

	existingUsers := make(map[string]struct{})
	toDelete := make(map[string]struct{})
	for _, mail := range currentUsers {
		existingUsers[mail] = struct{}{}
		if o.deleteOnDelete {
			toDelete[mail] = struct{}{}
		}
	}

	result := &operator.SyncResult{}
	worker := o.newSyncWorker(ctx)

	for uid, id := range state.Identities {
		if id.Email == "" || id.Disabled {
			continue
		}

		if !o.deleteOnDelete {
			if o.deleteOnDisable && id.Disabled {
				toDelete[id.Email] = struct{}{}
			}
		} else {
			if !o.deleteOnDisable {
				delete(toDelete, id.Email)
			} else {
				if !id.Disabled {
					delete(toDelete, id.Email)
				} else {
					continue
				}
			}
		}

		if !worker.submit(func() {
			o.syncUser(ctx, uid, id, existingUsers, state.Groups, state.DryRun, result)
		}) {
			break
		}
	}

	for email := range toDelete {
		if !worker.submit(func() {
			o.handleDelete(ctx, email, email, state.DryRun, result)
		}) {
			break
		}
	}

	worker.wait()
	return result, worker.err()
}

func (o *IRedAdminOperator) PartialSync(
	ctx context.Context,
	state *operator.PartialSyncState,
) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	result := &operator.SyncResult{}
	worker := o.newSyncWorker(ctx)

	for uid, id := range state.Identities {
		if !worker.submit(func() {
			o.partialSyncUser(ctx, uid, id, state.Groups, state.DryRun, result)
		}) {
			break
		}
	}

	worker.wait()
	return result, worker.err()
}

type syncWorker struct {
	ctx    context.Context
	sem    chan struct{}
	wg     sync.WaitGroup
	cancel bool
	mu     sync.Mutex
}

func (o *IRedAdminOperator) newSyncWorker(ctx context.Context) *syncWorker {
	workers := o.GetConcurrency()
	return &syncWorker{
		ctx: ctx,
		sem: make(chan struct{}, workers),
	}
}

func (w *syncWorker) submit(fn func()) bool {
	select {
	case <-w.ctx.Done():
		return false
	default:
	}

	w.wg.Go(func() {
		w.sem <- struct{}{}
		defer func() { <-w.sem }()
		fn()
	})
	return true
}

func (w *syncWorker) wait() {
	w.wg.Wait()
}

func (w *syncWorker) err() error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	default:
		return nil
	}
}

func (o *IRedAdminOperator) syncUser(
	ctx context.Context,
	uid string,
	id source.Identity,
	existingUsers map[string]struct{},
	groups map[string]source.Group,
	dryRun bool,
	result *operator.SyncResult,
) {
	enriched := o.EnrichIdentity(id, groups)
	newUserData := o.identityToUser(enriched)

	o.LogInfo("checking user %s (uid: %s)", id.Email, uid)

	_, exists := existingUsers[id.Email]
	if !exists {
		o.handleCreate(ctx, uid, &enriched, dryRun, result)
	} else {
		userData, err := o.getUser(ctx, id.Email)
		if err != nil {
			o.LogError(fmt.Errorf("check user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordIdentityError(enriched, operator.ActionNoOp, err)
			return
		}

		if err := o.updateUser(ctx, result, &newUserData, userData, dryRun); err != nil {
			o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordIdentityError(enriched, operator.ActionUpdate, err)
		}
	}
}

func (o *IRedAdminOperator) partialSyncUser(
	ctx context.Context,
	uid string,
	id source.Identity,
	groups map[string]source.Group,
	dryRun bool,
	result *operator.SyncResult,
) {
	if id.Deleted {
		o.handleDelete(ctx, id.Email, id.Username, dryRun, result)
		return
	}

	if id.Email == "" || (id.Disabled && o.deleteOnDisable) {
		if id.Email != "" {
			o.handleDelete(ctx, id.Email, id.Username, dryRun, result)
		}
		return
	}

	if id.Email == "" {
		o.LogWarn("Skipping identity %s: no email address", uid)
		return
	}

	enriched := o.EnrichIdentity(id, groups)
	newUserData := o.identityToUser(enriched)

	o.LogInfo("checking user %s (uid: %s) in partial sync", id.Email, uid)

	userData, err := o.getUser(ctx, id.Email)
	if err != nil {
		o.handleCreate(ctx, uid, &enriched, dryRun, result)
	} else {
		if err := o.updateUser(ctx, result, &newUserData, userData, dryRun); err != nil {
			o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordIdentityError(enriched, operator.ActionUpdate, err)
		}
	}
}

func (o *IRedAdminOperator) handleCreate(
	ctx context.Context,
	uid string,
	id *source.Identity,
	dryRun bool,
	result *operator.SyncResult,
) {
	if dryRun {
		o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", id.Email, uid)
		result.RecordIdentityCreate(*id)
	} else {
		if err := o.createUser(ctx, id); err != nil {
			o.LogError(fmt.Errorf("create user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordIdentityError(*id, operator.ActionCreate, err)
		} else {
			result.RecordIdentityCreate(*id)
		}
	}
}

func (o *IRedAdminOperator) handleDelete(
	ctx context.Context,
	email string,
	username string,
	dryRun bool,
	result *operator.SyncResult,
) {
	if dryRun {
		o.LogInfo("[DRY RUN] Would delete user %s", email)
		result.RecordIdentityDelete(username)
	} else {
		if err := o.deleteUser(ctx, email, o.keepMailboxDays); err != nil {
			o.LogError(fmt.Errorf("delete user %s: %w", email, err))
			result.RecordIdentityErrorManual(email, operator.ActionDelete, err)
		} else {
			result.RecordIdentityDelete(username)
		}
	}
}
