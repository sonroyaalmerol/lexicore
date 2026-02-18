package dovecot

import (
	"context"
	"fmt"
	"strings"
)

func (o *DovecotOperator) expandACLPatterns(
	ctx context.Context,
	acls []*ACL,
) ([]*ACL, error) {
	sc := syncCtxFrom(ctx)
	expanded := make([]*ACL, 0, len(acls))

	for _, acl := range acls {
		if strings.ContainsAny(acl.Mailbox, "*%") {
			mailboxes, hasCached := sc.expansionCache.Load(acl.Key())
			if !hasCached {
				var err error
				mailboxes, err = o.listMailboxesForUser(
					ctx, acl.SharedFolder, acl.Mailbox,
				)
				if err != nil {
					o.LogError(fmt.Errorf(
						"failed to expand pattern %s for user %s: %w",
						acl.Mailbox, acl.SharedFolder, err,
					))
					continue
				}
				sc.expansionCache.Store(acl.Key(), mailboxes)
			}

			for _, mailbox := range mailboxes {
				expandedACL := acl.Clone()
				expandedACL.Mailbox = mailbox
				expanded = append(expanded, expandedACL)
			}
		} else {
			expanded = append(expanded, acl)
		}
	}

	return expanded, nil
}

func (o *DovecotOperator) mergeSharedMailAcls(acls []*ACL) []*ACL {
	defaultPerms := []string{"lookup", "read", "write", "write-seen"}

	permsAny, ok := o.GetConfig("defaultAcls")
	if ok {
		if anyArr, isArr := permsAny.([]any); isArr {
			defaultPerms = make([]string, 0, len(anyArr))
			for _, anyVal := range anyArr {
				if strVal, isStr := anyVal.(string); isStr {
					defaultPerms = append(defaultPerms, strVal)
				}
			}
		}
	}

	if len(acls) == 0 {
		return nil
	}

	aclMap := make(map[string]*ACL)

	for _, acl := range acls {
		key := acl.Key()

		if existing, exists := aclMap[key]; exists {
			for right := range acl.Rights {
				existing.Rights[right] = struct{}{}
			}
		} else {
			merged := acl.Clone()
			if len(merged.Rights) == 0 {
				merged.AddRights(defaultPerms)
			}
			aclMap[key] = merged
		}
	}

	result := make([]*ACL, 0, len(aclMap))
	for _, acl := range aclMap {
		result = append(result, acl)
	}

	return result
}
