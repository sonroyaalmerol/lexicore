package dovecot

import (
	"context"
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *DovecotOperator) expandACLPatterns(ctx context.Context, acls []*ACL) ([]*ACL, error) {
	expanded := make([]*ACL, 0, len(acls))

	for _, acl := range acls {
		if strings.ContainsAny(acl.Mailbox, "*%") {
			mailboxes, err := o.listMailboxesForUser(ctx, acl.SharedFolder, acl.Mailbox)
			if err != nil {
				o.LogError(fmt.Errorf("failed to expand pattern %s for user %s: %w", acl.Mailbox, acl.SharedFolder, err))
				continue
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

func (o *DovecotOperator) hasACLs(identity source.Identity) bool {
	_, ok := identity.Attributes["acls"]
	return ok
}

func (o *DovecotOperator) extractACLs(identity source.Identity) []*ACL {
	aclsAny, ok := identity.Attributes["acls"]
	if !ok {
		return nil
	}

	aclsAnyArr, isArr := aclsAny.([]any)
	if !isArr {
		return nil
	}

	acls := make([]*ACL, 0, len(aclsAnyArr))
	for _, anyVal := range aclsAnyArr {
		strVal, isStr := anyVal.(string)
		if !isStr {
			continue
		}
		acl := parseACL(strVal)
		if acl != nil {
			acls = append(acls, acl)
		}
	}

	return acls
}
