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
	var mu sync.Mutex

	for _, mail := range currentUsers {
		existingUsers[mail] = struct{}{}
	}

	worker := o.newSyncWorker(ctx)

	for uid, id := range state.Identities {
		if id.Email == "" {
			continue
		}

		if !worker.submit(func() {
			o.syncUser(ctx, uid, id, existingUsers, &mu, state.DryRun, state.Result)
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
		result.RecordError(operator.ActionSkip, uid, id.Username, fmt.Errorf("user not found"))
		return
	}

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

func (o *IRedAdminOperator) partialSyncUser(
	ctx context.Context,
	uid string,
	id source.Identity,
	dryRun bool,
	result *operator.SyncResult,
) {
	if id.Email == "" {
		o.LogWarn("Skipping identity %s: no email address", uid)
		return
	}

	newUserData := o.identityToUser(id)

	o.LogInfo("checking user %s (uid: %s) in partial sync", id.Email, uid)

	userData, err := o.getUser(ctx, id.Email)
	if err != nil {
		result.RecordError(operator.ActionSkip, uid, id.Username, err)
		return
	}

	if err := o.updateUser(ctx, id, result, newUserData, userData, dryRun); err != nil {
		o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
		result.RecordError(operator.ActionUpdate, uid, id.Username, err)
	}
}
