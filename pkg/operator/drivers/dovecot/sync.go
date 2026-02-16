package dovecot

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/puzpuzpuz/xsync/v4"
)

type syncContext struct {
	allMailboxes           *xsync.Map[string, struct{}]
	mailboxDesiredACLs     *xsync.Map[string, map[string][]string]
	usersAffectedByMailbox *xsync.Map[string, map[string]struct{}]
	userMailboxMap         *xsync.Map[string, []string]
	isPartialSync          bool
	partialSyncIdentities  map[string]source.Identity

	expansionCache *xsync.Map[string, []string]
}

type syncWorker struct {
	ctx context.Context
	sem chan struct{}
	wg  sync.WaitGroup
}

func (o *DovecotOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	return o.performSync(ctx, state.Identities, state.Groups, state.DryRun, false, nil)
}

func (o *DovecotOperator) PartialSync(ctx context.Context, state *operator.PartialSyncState) (*operator.SyncResult, error) {
	return o.performSync(ctx, state.Identities, state.Groups, state.DryRun, true, state.Identities)
}

func (o *DovecotOperator) performSync(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	isDryRun bool,
	isPartialSync bool,
	partialSyncIdentities map[string]source.Identity,
) (*operator.SyncResult, error) {
	result := &operator.SyncResult{}

	syncCtx := &syncContext{
		allMailboxes:           xsync.NewMap[string, struct{}](),
		mailboxDesiredACLs:     xsync.NewMap[string, map[string][]string](),
		usersAffectedByMailbox: xsync.NewMap[string, map[string]struct{}](),
		userMailboxMap:         xsync.NewMap[string, []string](),
		isPartialSync:          isPartialSync,
		partialSyncIdentities:  partialSyncIdentities,
		expansionCache:         xsync.NewMap[string, []string](),
	}
	defer func() {
		syncCtx.expansionCache.Clear()
		syncCtx.allMailboxes.Clear()
		syncCtx.mailboxDesiredACLs.Clear()
		syncCtx.usersAffectedByMailbox.Clear()
		syncCtx.userMailboxMap.Clear()
	}()

	if err := o.processIdentities(ctx, identities, groups, syncCtx, result); err != nil {
		return result, err
	}

	if err := o.applyACLChanges(ctx, syncCtx, identities, isDryRun, result); err != nil {
		return result, err
	}

	return result, nil
}

func (o *DovecotOperator) newSyncWorker(ctx context.Context) *syncWorker {
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

func (o *DovecotOperator) processIdentities(
	ctx context.Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
	syncCtx *syncContext,
	result *operator.SyncResult,
) error {
	worker := o.newSyncWorker(ctx)

	for uid, identity := range identities {
		if !worker.submit(func() {
			o.processIdentity(ctx, uid, identity, groups, syncCtx, result)
		}) {
			break
		}
	}

	worker.wait()
	return worker.err()
}

func (o *DovecotOperator) processIdentity(
	ctx context.Context,
	uid string,
	identity source.Identity,
	groups map[string]source.Group,
	syncCtx *syncContext,
	result *operator.SyncResult,
) {
	enriched := o.EnrichIdentity(identity, groups)

	if !enriched.Deleted && o.hasACLs(enriched) {
		acls := o.extractACLs(enriched)
		if acls != nil {
			o.LogInfo("checking user %s (uid: %s)", identity.Email, uid)

			mergedDesired := o.mergeSharedMailAcls(acls)
			expandedACLs, err := o.expandACLPatterns(ctx, syncCtx, mergedDesired)
			if err != nil {
				o.LogError(fmt.Errorf("failed to expand ACL patterns for user %s: %w", identity.Username, err))
				result.RecordIdentityError(identity, operator.ActionNoOp, err)
			} else {
				for _, acl := range expandedACLs {
					mailboxKey := acl.Key()
					syncCtx.allMailboxes.Store(mailboxKey, struct{}{})

					syncCtx.mailboxDesiredACLs.Compute(mailboxKey, func(existing map[string][]string, loaded bool) (map[string][]string, xsync.ComputeOp) {
						if !loaded {
							existing = make(map[string][]string)
						}
						existing[identity.Username] = acl.RightsSlice()
						return existing, xsync.UpdateOp
					})

					syncCtx.usersAffectedByMailbox.Compute(mailboxKey, func(existing map[string]struct{}, loaded bool) (map[string]struct{}, xsync.ComputeOp) {
						if !loaded {
							existing = make(map[string]struct{})
						}
						existing[uid] = struct{}{}
						return existing, xsync.UpdateOp
					})
				}
			}
		}
	}

	o.fetchAndRecordUserMailboxes(ctx, uid, identity.Username, enriched, syncCtx, result)
}

func (o *DovecotOperator) fetchAndRecordUserMailboxes(
	ctx context.Context,
	uid string,
	username string,
	identity source.Identity,
	syncCtx *syncContext,
	result *operator.SyncResult,
) {
	mailboxList, err := o.getAllNonPersonalMailbox(ctx, username)
	if err != nil {
		o.LogError(fmt.Errorf("user %s (uid: %s) mailbox list failed: %w", username, uid, err))
		result.RecordIdentityError(identity, operator.ActionNoOp, err)
		return
	}

	for _, fullMailbox := range mailboxList {
		trimmed := strings.TrimPrefix(fullMailbox, "Other Users/")
		syncCtx.allMailboxes.Store(trimmed, struct{}{})

		syncCtx.usersAffectedByMailbox.Compute(trimmed, func(existing map[string]struct{}, loaded bool) (map[string]struct{}, xsync.ComputeOp) {
			if !loaded {
				existing = make(map[string]struct{})
			}
			existing[uid] = struct{}{}
			return existing, xsync.UpdateOp
		})

		syncCtx.userMailboxMap.Compute(username, func(existing []string, loaded bool) ([]string, xsync.ComputeOp) {
			if !loaded {
				existing = make([]string, 0)
			}
			existing = append(existing, trimmed)
			return existing, xsync.UpdateOp
		})
	}
}

func (o *DovecotOperator) applyACLChanges(
	ctx context.Context,
	syncCtx *syncContext,
	identities map[string]source.Identity,
	isDryRun bool,
	result *operator.SyncResult,
) error {
	worker := o.newSyncWorker(ctx)
	usersWithChanges := xsync.NewMap[string, struct{}]()

	syncCtx.allMailboxes.Range(func(mailboxKey string, _ struct{}) bool {
		if !worker.submit(func() {
			parts := strings.SplitN(mailboxKey, "/", 2)
			if len(parts) != 2 {
				return
			}

			sharedFolder := parts[0]
			mailboxPath := parts[1]

			currentACL, err := o.getMailboxACLs(ctx, mailboxKey)
			if err != nil {
				o.LogError(fmt.Errorf("acl get for %s failed: %w", mailboxKey, err))
				return
			}

			desiredForMailbox := o.getDesiredACLsForMailbox(mailboxKey, syncCtx, identities)

			diff := o.calculateMailboxDiffByType(
				sharedFolder,
				mailboxPath,
				currentACL,
				desiredForMailbox,
				syncCtx,
			)

			if len(diff.ToSet) > 0 || len(diff.ToRemove) > 0 {
				if err := o.applyMailboxACLs(ctx, result, sharedFolder, mailboxPath, diff, isDryRun); err != nil {
					o.LogError(fmt.Errorf("failed to apply ACLs for %s: %w", mailboxKey, err))
					return
				}

				o.recordUserChanges(mailboxKey, sharedFolder, mailboxPath, diff, syncCtx, identities, usersWithChanges, result)
			}
		}) {
			return false
		}
		return true
	})

	worker.wait()
	return worker.err()
}

func (o *DovecotOperator) getDesiredACLsForMailbox(
	mailboxKey string,
	syncCtx *syncContext,
	identities map[string]source.Identity,
) map[string][]string {
	desiredForMailbox := make(map[string][]string)

	if desired, exists := syncCtx.mailboxDesiredACLs.Load(mailboxKey); exists {
		if syncCtx.isPartialSync {
			for username, rights := range desired {
				if o.isUserInPartialSync(username, syncCtx.partialSyncIdentities, identities) {
					desiredForMailbox[username] = rights
				}
			}
		} else {
			desiredForMailbox = desired
		}
	}

	return desiredForMailbox
}

func (o *DovecotOperator) isUserInPartialSync(
	username string,
	partialSyncIdentities map[string]source.Identity,
	identities map[string]source.Identity,
) bool {
	if _, inPartialSync := partialSyncIdentities[username]; inPartialSync {
		return true
	}

	for _, identity := range identities {
		if identity.Username == username {
			return true
		}
	}

	return false
}

func (o *DovecotOperator) calculateMailboxDiffByType(
	sharedFolder string,
	mailboxPath string,
	currentACL *MailboxACLResponse,
	desiredUserACLs map[string][]string,
	syncCtx *syncContext,
) *MailboxDiff {
	if syncCtx.isPartialSync {
		return o.calculatePartialMailboxDiff(
			sharedFolder,
			mailboxPath,
			currentACL,
			desiredUserACLs,
			syncCtx.partialSyncIdentities,
		)
	}

	return o.calculateMailboxDiff(
		sharedFolder,
		mailboxPath,
		currentACL,
		desiredUserACLs,
	)
}

func (o *DovecotOperator) recordUserChanges(
	mailboxKey string,
	sharedFolder string,
	mailboxPath string,
	diff *MailboxDiff,
	syncCtx *syncContext,
	identities map[string]source.Identity,
	usersWithChanges *xsync.Map[string, struct{}],
	result *operator.SyncResult,
) {
	if affectedUsers, exists := syncCtx.usersAffectedByMailbox.Load(mailboxKey); exists {
		for uid := range affectedUsers {
			identity, exists := identities[uid]
			if !exists {
				continue
			}
			username := identity.Username
			if diff.DiffReport[username] != "" {
				usersWithChanges.Store(uid, struct{}{})
				result.RecordIdentityUpdateManual(uid, username, map[string]string{
					fmt.Sprintf("%s/%s", sharedFolder, mailboxPath): diff.DiffReport[username],
				})
			}
		}
	}
}
