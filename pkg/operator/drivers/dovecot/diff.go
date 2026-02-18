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
	Old        map[string]string
	New        map[string]string
}

func (o *DovecotOperator) calculateDiff(
	sharedFolder string,
	mailboxPath string,
	currentACL *MailboxACLResponse,
	desiredUserACLs map[string][]string,
) *MailboxDiff {
	diff := &MailboxDiff{
		MailboxKey: fmt.Sprintf("%s/%s", sharedFolder, mailboxPath),
		ToSet:      make(map[string][]string),
		ToRemove:   []string{},
		Old:        make(map[string]string),
		New:        make(map[string]string),
	}

	currentUserACLs, anyoneACLs := o.parseCurrentACLs(currentACL)

	o.calculateACLsToSet(diff, desiredUserACLs, currentUserACLs, anyoneACLs)
	o.calculateACLsToRemove(diff, desiredUserACLs, currentUserACLs, anyoneACLs)

	return diff
}

func (o *DovecotOperator) parseCurrentACLs(currentACL *MailboxACLResponse) (map[string][]string, []string) {
	currentUserACLs := make(map[string][]string)
	anyoneACLs := make([]string, 0, 11)

	for _, acl := range currentACL.ACLs {
		if !o.removeAnyoneACL {
			if acl.ID == "anyone" {
				anyoneACLs = utils.ConcatUnique(anyoneACLs, acl.Rights)
				continue
			}
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

	return currentUserACLs, anyoneACLs
}

func (o *DovecotOperator) calculateACLsToSet(
	diff *MailboxDiff,
	desiredUserACLs map[string][]string,
	currentUserACLs map[string][]string,
	anyoneACLs []string,
) {
	for username, desiredRights := range desiredUserACLs {
		effectiveRights := anyoneACLs
		currentRights, ok := currentUserACLs[username]
		if ok {
			effectiveRights = utils.ConcatUnique(currentRights, anyoneACLs)
		}

		if !utils.SlicesAreEqual(effectiveRights, desiredRights) {
			diff.ToSet[username] = desiredRights
			diff.New[username] = utils.SliceToSortedString(desiredRights, ",")
			diff.Old[username] = utils.SliceToSortedString(effectiveRights, ",")
		}
	}
}

func (o *DovecotOperator) calculateACLsToRemove(
	diff *MailboxDiff,
	desiredUserACLs map[string][]string,
	currentUserACLs map[string][]string,
	anyoneACLs []string,
) {
	if len(anyoneACLs) > 0 && o.removeAnyoneACL {
		diff.ToRemove = append(diff.ToRemove, "anyone")
		diff.New["anyone"] = ""
		diff.Old["anyone"] = utils.SliceToSortedString(anyoneACLs, ",")
	}

	for username := range currentUserACLs {
		desiredRights, desired := desiredUserACLs[username]
		if desired {
			continue
		}

		effectiveRights := anyoneACLs
		currentRights, ok := currentUserACLs[username]
		if ok {
			effectiveRights = utils.ConcatUnique(currentRights, anyoneACLs)
		}

		diff.New[username] = utils.SliceToSortedString(desiredRights, ",")
		diff.Old[username] = utils.SliceToSortedString(effectiveRights, ",")

		diff.ToRemove = append(diff.ToRemove, username)
	}
}
