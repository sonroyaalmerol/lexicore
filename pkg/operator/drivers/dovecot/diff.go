package dovecot

import (
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
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
	return o.calculateDiff(sharedFolder, mailboxPath, currentACL, desiredUserACLs, nil)
}

func (o *DovecotOperator) calculatePartialMailboxDiff(
	sharedFolder string,
	mailboxPath string,
	currentACL *MailboxACLResponse,
	desiredUserACLs map[string][]string,
	partialSyncIdentities map[string]source.Identity,
) *MailboxDiff {
	return o.calculateDiff(sharedFolder, mailboxPath, currentACL, desiredUserACLs, partialSyncIdentities)
}

func (o *DovecotOperator) calculateDiff(
	sharedFolder string,
	mailboxPath string,
	currentACL *MailboxACLResponse,
	desiredUserACLs map[string][]string,
	partialSyncIdentities map[string]source.Identity,
) *MailboxDiff {
	diff := &MailboxDiff{
		MailboxKey: fmt.Sprintf("%s/%s", sharedFolder, mailboxPath),
		ToSet:      make(map[string][]string),
		ToRemove:   []string{},
		DiffReport: make(map[string]string),
	}

	currentUserACLs, anyoneACLs := o.parseCurrentACLs(currentACL)

	o.calculateACLsToSet(diff, desiredUserACLs, currentUserACLs, anyoneACLs)
	o.calculateACLsToRemove(diff, currentUserACLs, desiredUserACLs, partialSyncIdentities)

	return diff
}

func (o *DovecotOperator) parseCurrentACLs(currentACL *MailboxACLResponse) (map[string][]string, []string) {
	currentUserACLs := make(map[string][]string)
	anyoneACLs := make([]string, 0, 11)

	for _, acl := range currentACL.ACLs {
		if !o.ignoreAnyone {
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
			diff.DiffReport[username] = utils.DiffArrString(effectiveRights, desiredRights)
		}
	}
}

func (o *DovecotOperator) calculateACLsToRemove(
	diff *MailboxDiff,
	currentUserACLs map[string][]string,
	desiredUserACLs map[string][]string,
	partialSyncIdentities map[string]source.Identity,
) {
	isPartialSync := partialSyncIdentities != nil

	for username := range currentUserACLs {
		if _, desired := desiredUserACLs[username]; desired {
			continue
		}

		if isPartialSync && !o.isUsernameInPartialSync(username, partialSyncIdentities) {
			continue
		}

		diff.ToRemove = append(diff.ToRemove, username)
		if isPartialSync {
			diff.DiffReport[username] = "REMOVE_ALL"
		}
	}
}

func (o *DovecotOperator) isUsernameInPartialSync(username string, partialSyncIdentities map[string]source.Identity) bool {
	for _, identity := range partialSyncIdentities {
		if identity.Username == username {
			return true
		}
	}
	return false
}
