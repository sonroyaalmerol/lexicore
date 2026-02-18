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
) error {
	if err := o.login(ctx); err != nil {
		return err
	}

	currentUsers, err := o.getUsers(ctx)
	if err != nil {
		return err
	}

	existingUsers := make(map[string]struct{})
	toDelete := make(map[string]struct{})
	var mu sync.Mutex

	for _, mail := range currentUsers {
		existingUsers[mail] = struct{}{}
		if o.deleteOnDelete {
			toDelete[mail] = struct{}{}
		}
	}

	worker := o.newSyncWorker(ctx)

	for uid, id := range state.Identities {
		if id.Email == "" {
			continue
		}

		if id.Disabled {
			if o.deleteOnDisable {
				mu.Lock()
				toDelete[id.Email] = struct{}{}
				mu.Unlock()
			}
			continue
		}

		if o.deleteOnDelete {
			mu.Lock()
			delete(toDelete, id.Email)
			mu.Unlock()
		}

		if !worker.submit(func() {
			o.syncUser(ctx, uid, id, existingUsers, &mu, state.DryRun, state.Result)
		}) {
			break
		}
	}

	mu.Lock()
	deleteList := make([]string, 0, len(toDelete))
	for email := range toDelete {
		deleteList = append(deleteList, email)
	}
	mu.Unlock()

	for _, email := range deleteList {
		if !worker.submit(func() {
			o.handleDelete(ctx, email, email, state.DryRun, state.Result)
		}) {
			break
		}
	}

	worker.wait()
	return nil
}
func (o *IRedAdminOperator) PartialSync(
	ctx context.Context,
	state *operator.PartialSyncState,
) error {
	if err := o.login(ctx); err != nil {
		return err
	}

	worker := o.newSyncWorker(ctx)

	for uid, id := range state.Identities {
		if !worker.submit(func() {
			o.partialSyncUser(ctx, uid, id, state.DryRun, state.Result)
		}) {
			break
		}
	}

	worker.wait()
	return nil
}

type syncWorker struct {
	ctx context.Context
	sem chan struct{}
	wg  sync.WaitGroup
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

	w.wg.Add(1)
	go func() {
		w.sem <- struct{}{}
		defer func() {
			<-w.sem
			w.wg.Done()
		}()
		fn()
	}()
	return true
}

func (w *syncWorker) wait() {
	w.wg.Wait()
}

func (o *IRedAdminOperator) syncUser(
	ctx context.Context,
	uid string,
	id source.Identity,
	existingUsers map[string]struct{},
	mu *sync.Mutex,
	dryRun bool,
	result *operator.SyncResult,
) {
	newUserData := o.identityToUser(id)

	o.LogInfo("checking user %s (uid: %s)", id.Email, uid)

	mu.Lock()
	_, exists := existingUsers[id.Email]
	mu.Unlock()

	if !exists {
		o.handleCreate(ctx, uid, id, dryRun, result)
	} else {
		userData, err := o.getUser(ctx, id.Email)
		if err != nil {
			o.LogError(fmt.Errorf("check user %s (uid: %s): %w", id.Email, uid, err))
			result.Record(operator.ActionSkip, uid, id.Username)
			return
		}

		if err := o.updateUser(ctx, id, result, newUserData, userData, dryRun); err != nil {
			o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordError(operator.ActionUpdate, uid, id.Username, err)
		}
	}
}

func (o *IRedAdminOperator) partialSyncUser(
	ctx context.Context,
	uid string,
	id source.Identity,
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

	newUserData := o.identityToUser(id)

	o.LogInfo("checking user %s (uid: %s) in partial sync", id.Email, uid)

	userData, err := o.getUser(ctx, id.Email)
	if err != nil {
		o.handleCreate(ctx, uid, id, dryRun, result)
	} else {
		if err := o.updateUser(ctx, id, result, newUserData, userData, dryRun); err != nil {
			o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordError(operator.ActionUpdate, uid, id.Username, err)
		}
	}
}

func (o *IRedAdminOperator) handleCreate(
	ctx context.Context,
	uid string,
	id source.Identity,
	dryRun bool,
	result *operator.SyncResult,
) {
	if dryRun {
		o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", id.Email, uid)
	} else {
		if err := o.createUser(ctx, id); err != nil {
			o.LogError(fmt.Errorf("create user %s (uid: %s): %w", id.Email, uid, err))
			result.RecordError(operator.ActionCreate, uid, id.Username, err)
			return
		}
	}
	result.Record(operator.ActionCreate, uid, id.Username)
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
	} else {
		if err := o.deleteUser(ctx, email, o.keepMailboxDays); err != nil {
			o.LogError(fmt.Errorf("delete user %s: %w", email, err))
			result.RecordError(operator.ActionDelete, email, username, err)
			return
		}
	}
	result.Record(operator.ActionDelete, email, username)
}
