package dovecot

import (
	"fmt"
	"strings"
)

type aclData struct {
	SharedFolder string
	Mailbox      string
	Perms        map[string]struct{}
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
		commaIndex := strings.LastIndex(acl, ",")
		pathPart := acl
		if commaIndex != -1 {
			pathPart = acl[:commaIndex]
			permsStr := strings.TrimSpace(acl[commaIndex+1:])
			if len(permsStr) > 0 {
				perms = strings.Fields(permsStr)
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
			for _, p := range perms {
				existing.Perms[p] = struct{}{}
			}
		} else {
			permMap := make(map[string]struct{})
			for _, p := range perms {
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

func (o *DovecotOperator) parseACLString(aclStr string) (mailboxKey string, rights []string, noPropagate bool) {
	aclPath := aclStr
	noPropagate = false

	if strings.Contains(aclPath, "!no-propagate") {
		noPropagate = true
		aclPath = strings.ReplaceAll(aclPath, "!no-propagate", "")
		aclPath = strings.TrimSpace(aclPath)
	}

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
		return "", nil, false
	}

	sharedFolder := parts[0]
	mailbox := "INBOX"
	if len(parts) > 1 {
		mailbox = strings.Join(parts[1:], "/")
	}

	mailboxKey = fmt.Sprintf("%s/%s", sharedFolder, mailbox)
	return mailboxKey, rights, noPropagate
}
