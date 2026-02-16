package dovecot

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type syncContext struct {
	allMailboxes           map[string]struct{}
	mailboxDesiredACLs     map[string]map[string][]string
	usersAffectedByMailbox map[string]map[string]struct{}
	userMailboxMap         map[string][]string
	isPartialSync          bool
	partialSyncIdentities  map[string]source.Identity
	mu                     sync.Mutex
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
		allMailboxes:           make(map[string]struct{}),
		mailboxDesiredACLs:     make(map[string]map[string][]string),
		usersAffectedByMailbox: make(map[string]map[string]struct{}),
		userMailboxMap:         make(map[string][]string),
		isPartialSync:          isPartialSync,
		partialSyncIdentities:  partialSyncIdentities,
	}

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
			expandedACLs, err := o.expandACLPatterns(ctx, mergedDesired)
			if err != nil {
				o.LogError(fmt.Errorf("failed to expand ACL patterns for user %s: %w", identity.Username, err))
				result.RecordIdentityError(identity, operator.ActionNoOp, err)
			} else {
				syncCtx.mu.Lock()
				for _, acl := range expandedACLs {
					mailboxKey := acl.Key()
					syncCtx.allMailboxes[mailboxKey] = struct{}{}

					if syncCtx.mailboxDesiredACLs[mailboxKey] == nil {
						syncCtx.mailboxDesiredACLs[mailboxKey] = make(map[string][]string)
					}
					syncCtx.mailboxDesiredACLs[mailboxKey][identity.Username] = acl.RightsSlice()

					if syncCtx.usersAffectedByMailbox[mailboxKey] == nil {
						syncCtx.usersAffectedByMailbox[mailboxKey] = make(map[string]struct{})
					}
					syncCtx.usersAffectedByMailbox[mailboxKey][uid] = struct{}{}
				}
				syncCtx.mu.Unlock()
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

	syncCtx.mu.Lock()
	defer syncCtx.mu.Unlock()

	for _, fullMailbox := range mailboxList {
		trimmed := strings.TrimPrefix(fullMailbox, "Other Users/")
		syncCtx.allMailboxes[trimmed] = struct{}{}

		if syncCtx.usersAffectedByMailbox[trimmed] == nil {
			syncCtx.usersAffectedByMailbox[trimmed] = make(map[string]struct{})
		}
		syncCtx.usersAffectedByMailbox[trimmed][uid] = struct{}{}

		if syncCtx.userMailboxMap[username] == nil {
			syncCtx.userMailboxMap[username] = make([]string, 0)
		}
		syncCtx.userMailboxMap[username] = append(syncCtx.userMailboxMap[username], trimmed)
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
	usersWithChanges := make(map[string]struct{})
	var usersWithChangesMu sync.Mutex

	for mailboxKey := range syncCtx.allMailboxes {
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

				usersWithChangesMu.Lock()
				o.recordUserChanges(mailboxKey, sharedFolder, mailboxPath, diff, syncCtx, identities, usersWithChanges, result)
				usersWithChangesMu.Unlock()
			}
		}) {
			break
		}
	}

	worker.wait()
	return worker.err()
}

func (o *DovecotOperator) getDesiredACLsForMailbox(
	mailboxKey string,
	syncCtx *syncContext,
	identities map[string]source.Identity,
) map[string][]string {
	desiredForMailbox := make(map[string][]string)

	if desired, exists := syncCtx.mailboxDesiredACLs[mailboxKey]; exists {
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
	usersWithChanges map[string]struct{},
	result *operator.SyncResult,
) {
	for uid := range syncCtx.usersAffectedByMailbox[mailboxKey] {
		identity, exists := identities[uid]
		if !exists {
			continue
		}
		username := identity.Username
		if diff.DiffReport[username] != "" {
			usersWithChanges[uid] = struct{}{}
			result.RecordIdentityUpdateManual(uid, username, map[string]string{
				fmt.Sprintf("%s/%s", sharedFolder, mailboxPath): diff.DiffReport[username],
			})
		}
	}
}
