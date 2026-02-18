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
	expansionCache         *xsync.Map[string, []string]
	state                  *operator.SyncState
}

type syncContextKey struct{}

func syncCtxFrom(ctx context.Context) *syncContext {
	return ctx.Value(syncContextKey{}).(*syncContext)
}

type syncWorker struct {
	ctx context.Context
	sem chan struct{}
	wg  sync.WaitGroup
}

func (o *DovecotOperator) newSyncWorker(ctx context.Context) *syncWorker {
	return &syncWorker{
		ctx: ctx,
		sem: make(chan struct{}, o.GetConcurrency()),
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

func (w *syncWorker) waitErr() error {
	w.wg.Wait()
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	default:
		return nil
	}
}

func (o *DovecotOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) error {
	sc := &syncContext{
		allMailboxes:           xsync.NewMap[string, struct{}](),
		mailboxDesiredACLs:     xsync.NewMap[string, map[string][]string](),
		usersAffectedByMailbox: xsync.NewMap[string, map[string]struct{}](),
		userMailboxMap:         xsync.NewMap[string, []string](),
		expansionCache:         xsync.NewMap[string, []string](),
		state:                  state,
	}
	defer func() {
		sc.expansionCache.Clear()
		sc.allMailboxes.Clear()
		sc.mailboxDesiredACLs.Clear()
		sc.usersAffectedByMailbox.Clear()
		sc.userMailboxMap.Clear()
	}()

	ctx = context.WithValue(ctx, syncContextKey{}, sc)

	if err := o.processIdentities(ctx); err != nil {
		return err
	}
	return o.applyACLChanges(ctx)
}

func (o *DovecotOperator) processIdentities(ctx context.Context) error {
	sc := syncCtxFrom(ctx)
	worker := o.newSyncWorker(ctx)

	for uid, identity := range sc.state.Identities {
		if !worker.submit(func() {
			o.processIdentity(ctx, uid, identity)
		}) {
			break
		}
	}

	return worker.waitErr()
}

func (o *DovecotOperator) processIdentity(
	ctx context.Context,
	uid string,
	identity source.Identity,
) {
	sc := syncCtxFrom(ctx)

	if !identity.Deleted {
		if aclsAny, ok := identity.Attributes["acls"]; ok {
			if aclsArr, isArr := aclsAny.([]any); isArr {
				acls := make([]*ACL, 0, len(aclsArr))
				for _, v := range aclsArr {
					if s, isStr := v.(string); isStr {
						if acl := parseACL(s); acl != nil {
							acls = append(acls, acl)
						}
					}
				}

				if len(acls) > 0 {
					o.LogInfo(
						"checking user %s (uid: %s)",
						identity.Email, uid,
					)

					merged := o.mergeSharedMailAcls(acls)
					expanded, err := o.expandACLPatterns(ctx, merged)
					if err != nil {
						o.LogError(fmt.Errorf(
							"failed to expand ACL patterns for user %s: %w",
							identity.Username, err,
						))
						sc.state.Result.RecordError(
							operator.ActionSkip,
							uid, identity.Username, err,
						)
					} else {
						for _, acl := range expanded {
							mailboxKey := acl.Key()
							sc.allMailboxes.Store(mailboxKey, struct{}{})

							sc.mailboxDesiredACLs.Compute(
								mailboxKey,
								func(existing map[string][]string, loaded bool) (map[string][]string, xsync.ComputeOp) {
									if !loaded {
										existing = make(map[string][]string)
									}
									existing[identity.Username] = acl.RightsSlice()
									return existing, xsync.UpdateOp
								},
							)

							sc.usersAffectedByMailbox.Compute(
								mailboxKey,
								func(existing map[string]struct{}, loaded bool) (map[string]struct{}, xsync.ComputeOp) {
									if !loaded {
										existing = make(map[string]struct{})
									}
									existing[uid] = struct{}{}
									return existing, xsync.UpdateOp
								},
							)
						}
					}
				}
			}
		}
	}

	username := identity.Username
	mailboxList, err := o.getAllNonPersonalMailbox(ctx, username)
	if err != nil {
		o.LogError(fmt.Errorf(
			"user %s (uid: %s) mailbox list failed: %w",
			username, uid, err,
		))
		sc.state.Result.RecordError(
			operator.ActionSkip, uid, username, err,
		)
		return
	}

	for _, fullMailbox := range mailboxList {
		trimmed := strings.TrimPrefix(fullMailbox, "Other Users/")
		sc.allMailboxes.Store(trimmed, struct{}{})

		sc.usersAffectedByMailbox.Compute(
			trimmed,
			func(existing map[string]struct{}, loaded bool) (map[string]struct{}, xsync.ComputeOp) {
				if !loaded {
					existing = make(map[string]struct{})
				}
				existing[uid] = struct{}{}
				return existing, xsync.UpdateOp
			},
		)

		sc.userMailboxMap.Compute(
			username,
			func(existing []string, loaded bool) ([]string, xsync.ComputeOp) {
				if !loaded {
					existing = make([]string, 0)
				}
				existing = append(existing, trimmed)
				return existing, xsync.UpdateOp
			},
		)
	}
}

func (o *DovecotOperator) applyACLChanges(ctx context.Context) error {
	sc := syncCtxFrom(ctx)
	worker := o.newSyncWorker(ctx)

	changes := make(map[string][]operator.Change)

	sc.allMailboxes.Range(func(mailboxKey string, _ struct{}) bool {
		if !worker.submit(func() {
			parts := strings.SplitN(mailboxKey, "/", 2)
			if len(parts) != 2 {
				return
			}

			sharedFolder := parts[0]
			mailboxPath := parts[1]

			currentACL, err := o.getMailboxACLs(ctx, mailboxKey)
			if err != nil {
				o.LogError(fmt.Errorf(
					"acl get for %s failed: %w", mailboxKey, err,
				))
				return
			}

			desiredForMailbox := make(map[string][]string)
			if desired, exists := sc.mailboxDesiredACLs.Load(mailboxKey); exists {
				desiredForMailbox = desired
			}

			diff := o.calculateDiff(
				sharedFolder, mailboxPath,
				currentACL, desiredForMailbox,
			)

			if len(diff.ToSet) == 0 && len(diff.ToRemove) == 0 {
				return
			}

			if err := o.applyMailboxACLs(ctx, sharedFolder, mailboxPath, diff); err != nil {
				o.LogError(fmt.Errorf(
					"failed to apply ACLs for %s: %w", mailboxKey, err,
				))
				return
			}

			affectedUsers, exists := sc.usersAffectedByMailbox.Load(mailboxKey)
			if !exists {
				return
			}

			aclPath := fmt.Sprintf("%s/%s", sharedFolder, mailboxPath)
			for uid := range affectedUsers {
				identity, ok := sc.state.Identities[uid]
				if !ok {
					continue
				}
				oldAcls := diff.Old[identity.Username]
				newAcls := diff.New[identity.Username]

				if len(oldAcls)+len(newAcls) > 0 {
					changes[uid] = append(
						changes[uid],
						operator.AttrChange(aclPath, diff.Old[identity.Username], diff.New[identity.Username]),
					)
				}
			}
		}) {
			return false
		}
		return true
	})

	err := worker.waitErr()

	for uid, change := range changes {
		identity := sc.state.Identities[uid]
		sc.state.Result.Record(
			operator.ActionUpdate, uid, identity.Username,
			change...,
		)
	}

	return err
}
