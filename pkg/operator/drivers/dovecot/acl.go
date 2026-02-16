package dovecot

import (
	"fmt"
	"strings"
)

type ACL struct {
	SharedFolder string
	Mailbox      string
	Rights       map[string]struct{}
	Domain       string
}

func parseACL(aclStr string) *ACL {
	acl := &ACL{
		Rights: make(map[string]struct{}),
	}

	commaIndex := strings.LastIndex(aclStr, ",")
	pathPart := aclStr

	if commaIndex != -1 {
		pathPart = aclStr[:commaIndex]
		permsStr := strings.TrimSpace(aclStr[commaIndex+1:])
		if len(permsStr) > 0 {
			for perm := range strings.FieldsSeq(permsStr) {
				acl.Rights[perm] = struct{}{}
			}
		}
	}

	pathPart = strings.ReplaceAll(pathPart, "\\", "/")

	if atIndex := strings.Index(pathPart, "@"); atIndex != -1 {
		remaining := pathPart[atIndex:]
		slashIndex := strings.Index(remaining, "/")
		if slashIndex != -1 {
			acl.Domain = remaining[:slashIndex]
			pathPart = pathPart[:atIndex] + remaining[slashIndex:]
		} else {
			acl.Domain = remaining
			pathPart = pathPart[:atIndex]
		}
	}

	parts := strings.Split(strings.Trim(pathPart, "/"), "/")

	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		return nil
	}

	acl.SharedFolder = parts[0]
	acl.Mailbox = "INBOX"
	if len(parts) > 1 {
		acl.Mailbox = strings.Join(parts[1:], "/")
	}

	return acl
}

func (a *ACL) Key() string {
	return fmt.Sprintf("%s/%s", a.SharedFolder, a.Mailbox)
}

func (a *ACL) RightsSlice() []string {
	rights := make([]string, 0, len(a.Rights))
	for right := range a.Rights {
		rights = append(rights, right)
	}
	return rights
}

func (a *ACL) String() string {
	rights := a.RightsSlice()
	if len(rights) == 0 {
		return fmt.Sprintf("%s/%s", a.SharedFolder, a.Mailbox)
	}
	return fmt.Sprintf("%s/%s,%s", a.SharedFolder, a.Mailbox, strings.Join(rights, " "))
}

func (a *ACL) Clone() *ACL {
	clone := &ACL{
		SharedFolder: a.SharedFolder,
		Mailbox:      a.Mailbox,
		Domain:       a.Domain,
		Rights:       make(map[string]struct{}, len(a.Rights)),
	}
	for right := range a.Rights {
		clone.Rights[right] = struct{}{}
	}
	return clone
}

func (a *ACL) AddRights(rights []string) {
	for _, right := range rights {
		a.Rights[right] = struct{}{}
	}
}
