package dovecot

import (
	"context"
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/utils"
)

type aclData struct {
	SharedFolder string
	Mailbox      string
	Perms        map[string]struct{}
}

func (o *DovecotOperator) expandACLPatterns(ctx context.Context, acls []string) ([]string, error) {
	expanded := make([]string, 0, len(acls))

	for _, acl := range acls {
		if strings.ContainsAny(acl, "*%") {
			commaIndex := strings.Index(acl, ",")
			permsPart := ""

			if commaIndex != -1 {
				permsPart = strings.TrimSpace(acl[commaIndex:])
				acl = strings.TrimSpace(acl[:commaIndex])
			}

			domainPart := ""

			acl = strings.ReplaceAll(acl, "\\", "/")

			if atIndex := strings.Index(acl, "@"); atIndex != -1 {
				remaining := acl[atIndex:]
				slashIndex := strings.Index(remaining, "/")
				if slashIndex != -1 {
					domainPart = remaining[:slashIndex]
					acl = acl[:atIndex] + remaining[slashIndex:]
				} else {
					domainPart = remaining
					acl = acl[:atIndex]
				}
			}

			parts := strings.Split(strings.Trim(acl, "/"), "/")

			if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
				continue
			}

			sharedFolder := parts[0]
			mailbox := "INBOX"
			if len(parts) > 1 {
				mailbox = strings.Join(parts[1:], "/")
			}

			if len(parts) > 1 {
				mailbox = parts[len(parts)-1]
			}

			mailboxes, err := o.listMailboxesForUser(ctx, sharedFolder, mailbox)
			if err != nil {
				o.LogError(fmt.Errorf("failed to expand pattern %s for user %s: %w", mailbox, sharedFolder, err))
				continue
			}

			currentExpanded := make([]string, 0, len(mailboxes))
			for _, mailbox := range mailboxes {
				expandedACL := fmt.Sprintf("%s%s/%s%s", sharedFolder, domainPart, mailbox, permsPart)
				currentExpanded = append(currentExpanded, expandedACL)
			}

			expanded = utils.ConcatUnique(expanded, currentExpanded)
		} else {
			expanded = append(expanded, acl)
		}
	}

	return expanded, nil
}

func (o *DovecotOperator) mergeSharedMailAcls(acls []string) []string {
	perms := make([]string, 0, len(acls))

	permsAny, ok := o.GetConfig("defaultAcls")
	if !ok {
		perms = []string{"lookup", "read", "write", "write-seen"}
	} else if anyArr, isArr := permsAny.([]any); isArr {
		for _, anyVal := range anyArr {
			if strVal, isStr := anyVal.(string); isStr {
				perms = append(perms, strVal)
			}
		}
	}
	if len(acls) == 0 {
		return nil
	}

	sharedMailAclsMap := make(map[string]*aclData)

	for _, acl := range acls {
		currentPerms := make([]string, len(perms))
		copy(currentPerms, perms)

		commaIndex := strings.LastIndex(acl, ",")
		pathPart := acl
		if commaIndex != -1 {
			pathPart = acl[:commaIndex]
			permsStr := strings.TrimSpace(acl[commaIndex+1:])
			if len(permsStr) > 0 {
				currentPerms = strings.Fields(permsStr)
			}
		}

		pathPart = strings.ReplaceAll(pathPart, "\\", "/")
		slashIndex := strings.Index(pathPart, "/")

		sharedFolder := pathPart
		mailbox := "INBOX"

		if slashIndex != -1 {
			sharedFolder = pathPart[:slashIndex]
			mailbox = pathPart[slashIndex+1:]
		}

		mapKey := fmt.Sprintf("%s/%s", sharedFolder, mailbox)

		if existing, exists := sharedMailAclsMap[mapKey]; exists {
			for _, p := range currentPerms {
				existing.Perms[p] = struct{}{}
			}
		} else {
			permMap := make(map[string]struct{})
			for _, p := range currentPerms {
				permMap[p] = struct{}{}
			}
			sharedMailAclsMap[mapKey] = &aclData{
				SharedFolder: sharedFolder,
				Mailbox:      mailbox,
				Perms:        permMap,
			}
		}
	}

	result := make([]string, 0, len(sharedMailAclsMap))
	for _, data := range sharedMailAclsMap {
		pSlice := make([]string, 0, len(data.Perms))
		for p := range data.Perms {
			pSlice = append(pSlice, p)
		}

		permsJoined := strings.Join(pSlice, " ")
		result = append(result, fmt.Sprintf("%s/%s,%s", data.SharedFolder, data.Mailbox, permsJoined))
	}

	return result
}

func (o *DovecotOperator) parseACLString(aclStr string) (mailboxKey string, rights []string) {
	aclPath := aclStr

	commaIndex := strings.Index(aclPath, ",")
	if commaIndex != -1 {
		permsPart := strings.TrimSpace(aclPath[commaIndex+1:])
		aclPath = strings.TrimSpace(aclPath[:commaIndex])
		if len(permsPart) > 0 {
			rights = strings.Fields(permsPart)
		}
	}

	aclPath = strings.ReplaceAll(aclPath, "\\", "/")

	if atIndex := strings.Index(aclPath, "@"); atIndex != -1 {
		remaining := aclPath[atIndex:]
		slashIndex := strings.Index(remaining, "/")
		if slashIndex != -1 {
			aclPath = aclPath[:atIndex] + remaining[slashIndex:]
		} else {
			aclPath = aclPath[:atIndex]
		}
	}

	parts := strings.Split(strings.Trim(aclPath, "/"), "/")

	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		return "", nil
	}

	sharedFolder := parts[0]
	mailbox := "INBOX"
	if len(parts) > 1 {
		mailbox = strings.Join(parts[1:], "/")
	}

	mailboxKey = fmt.Sprintf("%s/%s", sharedFolder, mailbox)

	return mailboxKey, rights
}
