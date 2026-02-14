package dovecot

import (
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/utils"
)

type MailboxDiff struct {
	MailboxKey string
	ToSet      map[string][]string
	ToRemove   []string
	DiffReport map[string]string
}

func (o *DovecotOperator) calculateMailboxDiff(
	sharedFolder string,
	mailboxPath string,
	currentACL *MailboxACLResponse,
	desiredUserACLs map[string][]string,
) *MailboxDiff {
	diff := &MailboxDiff{
		MailboxKey: fmt.Sprintf("%s/%s", sharedFolder, mailboxPath),
		ToSet:      make(map[string][]string),
		ToRemove:   []string{},
		DiffReport: make(map[string]string),
	}

	currentUserACLs := make(map[string][]string)
	anyoneACLs := make([]string, 0, 11)

	for _, acl := range currentACL.ACLs {
		if acl.ID == "anyone" {
			anyoneACLs = utils.ConcatUnique(anyoneACLs, acl.Rights)
			continue
		}
		if email, trimmed := strings.CutPrefix(acl.ID, "user="); trimmed {
			emailParts := strings.SplitN(email, "@", 2)
			username := emailParts[0]
			if strings.TrimSpace(username) == "" {
				username = email
			}
			currentUserACLs[username] = acl.Rights
		}
	}

	for username, desiredRights := range desiredUserACLs {
		effectiveRights := anyoneACLs
		currentRights, ok := currentUserACLs[username]
		if ok {
			effectiveRights = utils.ConcatUnique(currentRights, anyoneACLs)
		}

		if !utils.SlicesAreEqual(effectiveRights, desiredRights) {
			diff.ToSet[username] = desiredRights
			diff.DiffReport[username] = utils.DiffArrString(effectiveRights, desiredRights)
		}
	}

	for username := range currentUserACLs {
		if _, desired := desiredUserACLs[username]; !desired {
			diff.ToRemove = append(diff.ToRemove, username)
		}
	}

	return diff
}
