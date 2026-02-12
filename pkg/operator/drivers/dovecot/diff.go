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
	}

	currentUserACLs := make(map[string][]string)
	for _, acl := range currentACL.ACLs {
		if email, trimmed := strings.CutPrefix(acl.ID, "user="); trimmed {
			emailParts := strings.SplitN(email, "@", 2)
			if len(emailParts) == 2 {
				username := emailParts[0]
				currentUserACLs[username] = acl.Rights
			}
		}
	}

	for username, desiredRights := range desiredUserACLs {
		currentRights, exists := currentUserACLs[username]
		if !exists {
			diff.ToSet[username] = desiredRights
		} else {
			if !utils.SlicesAreEqual(currentRights, desiredRights) {
				diff.ToSet[username] = desiredRights
			}
		}
	}

	for username := range currentUserACLs {
		if _, desired := desiredUserACLs[username]; !desired {
			diff.ToRemove = append(diff.ToRemove, username)
		}
	}

	return diff
}
