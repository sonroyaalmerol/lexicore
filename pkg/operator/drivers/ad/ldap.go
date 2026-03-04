package ad

import (
	"fmt"
	"strconv"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
	"github.com/go-ldap/ldap/v3"
)

const (
	uacAccountDisable = 0x0002
)

func isDisabled(uac int) bool {
	return uac&uacAccountDisable != 0
}

func setDisabled(uac int, disabled bool) int {
	if disabled {
		return uac | uacAccountDisable
	}
	return uac &^ uacAccountDisable
}

func (o *ADOperator) enableUser(
	conn *ldap.Conn,
	res *operator.SyncResult,
	entry *ldap.Entry,
	id source.Identity,
	dryRun bool,
) error {
	currentStr := entry.GetAttributeValue("userAccountControl")
	currentUAC, err := strconv.Atoi(currentStr)
	if err != nil {
		return fmt.Errorf("invalid userAccountControl value %q: %w", currentStr, err)
	}

	if !isDisabled(currentUAC) {
		return nil
	}

	newUAC := setDisabled(currentUAC, false)
	newStr := strconv.Itoa(newUAC)

	if dryRun {
		o.LogInfo(
			"[DRY RUN] Would enable user %s (userAccountControl: %s -> %s)",
			id.Username, currentStr, newStr,
		)
	} else {
		modReq := ldap.NewModifyRequest(entry.DN, nil)
		modReq.Replace("userAccountControl", []string{newStr})
		if err := conn.Modify(modReq); err != nil {
			return fmt.Errorf("error enabling user %s: %w", id.Username, err)
		}
	}

	res.Record(
		operator.ActionUpdate, id.UID, id.Username,
		operator.AttrChange("userAccountControl", currentStr, newStr),
	)
	return nil
}

func (o *ADOperator) disableUser(
	conn *ldap.Conn,
	res *operator.SyncResult,
	entry *ldap.Entry,
	id source.Identity,
	dryRun bool,
) error {
	currentStr := entry.GetAttributeValue("userAccountControl")
	currentUAC, err := strconv.Atoi(currentStr)
	if err != nil {
		return fmt.Errorf("invalid userAccountControl value %q: %w", currentStr, err)
	}

	if isDisabled(currentUAC) {
		return nil
	}

	newUAC := setDisabled(currentUAC, true)
	newStr := strconv.Itoa(newUAC)

	if dryRun {
		o.LogInfo(
			"[DRY RUN] Would disable user %s (userAccountControl: %s -> %s)",
			id.Username, currentStr, newStr,
		)
	} else {
		modReq := ldap.NewModifyRequest(entry.DN, nil)
		modReq.Replace("userAccountControl", []string{newStr})
		if err := conn.Modify(modReq); err != nil {
			return fmt.Errorf("error disabling user %s: %w", id.Username, err)
		}
	}

	res.Record(
		operator.ActionUpdate, id.UID, id.Username,
		operator.AttrChange("userAccountControl", currentStr, newStr),
	)
	return nil
}

func (o *ADOperator) updateUser(
	conn *ldap.Conn,
	res *operator.SyncResult,
	baseUserDN string,
	entry ldap.Entry,
	id source.Identity,
	isDryRun bool,
) (string, error) {
	modReq := ldap.NewModifyRequest(entry.DN, nil)
	var changes []operator.Change

	for k, v := range id.Attributes {
		if k == "baseDN" || k == "adGroups" || k == "disabled" || k == "cn" {
			continue
		}

		if vArr, isArr := v.([]any); isArr {
			vArrStr := make([]string, 0, len(vArr))
			for _, anyVal := range vArr {
				if strVal, isStr := anyVal.(string); isStr {
					vArrStr = append(vArrStr, strVal)
				}
			}

			currentVal := entry.GetAttributeValues(k)
			if !utils.SlicesAreEqual(vArrStr, currentVal) {
				changes = append(changes, operator.AttrChange(
					k,
					strings.Join(currentVal, ","),
					strings.Join(vArrStr, ","),
				))
				if !isDryRun {
					modReq.Replace(k, vArrStr)
				}
			}
		} else {
			currentVal := entry.GetAttributeValue(k)
			val := fmt.Sprintf("%v", v)
			if val != "" && currentVal != val {
				changes = append(changes, operator.AttrChange(k, currentVal, val))
				if !isDryRun {
					modReq.Replace(k, []string{val})
				}
			}
		}
	}

	if !isDryRun && len(modReq.Changes) > 0 {
		if err := conn.Modify(modReq); err != nil {
			return entry.DN, err
		}
	}

	newDN := entry.DN

	dnSplit := strings.SplitN(entry.DN, ",", 2)
	desiredDN := fmt.Sprintf("%s,%s", dnSplit[0], baseUserDN)

	if entry.DN != desiredDN {
		desiredDnSplit := strings.SplitN(desiredDN, ",", 2)
		if len(dnSplit) == 2 && len(desiredDnSplit) == 2 {
			changes = append(changes, operator.AttrChange("dn", entry.DN, desiredDN))
			if !isDryRun {
				moveReq := ldap.NewModifyDNRequest(entry.DN, dnSplit[0], true, desiredDnSplit[1])
				if err := conn.ModifyDN(moveReq); err != nil {
					return entry.DN, err
				}
			}
			newDN = desiredDN
		}
	}

	if len(changes) > 0 {
		res.Record(operator.ActionUpdate, id.UID, id.Username, changes...)
	}

	return newDN, nil
}

func (o *ADOperator) syncGroups(
	conn *ldap.Conn,
	res *operator.SyncResult,
	userDN string,
	currentMemberOf []string,
	user source.Identity,
	isDryRun bool,
) error {
	groupDNAny, ok := user.Attributes["adGroups"]
	if !ok {
		return nil
	}

	desiredGroups := make(map[string]bool)
	switch v := groupDNAny.(type) {
	case string:
		desiredGroups[v] = true
	case []any:
		for _, dnAny := range v {
			if dnStr, isStr := dnAny.(string); isStr {
				desiredGroups[dnStr] = true
			}
		}
	default:
		return nil
	}

	currentGroups := make(map[string]bool, len(currentMemberOf))
	for _, memberOf := range currentMemberOf {
		currentGroups[memberOf] = true
	}

	var changes []operator.Change

	for groupDN := range desiredGroups {
		if currentGroups[groupDN] {
			continue
		}
		changes = append(changes, operator.MembershipAdded(groupDN))
		if isDryRun {
			o.LogInfo("[DRY RUN] Would add %s to group %s", userDN, groupDN)
			continue
		}
		grpReq := ldap.NewModifyRequest(groupDN, []ldap.Control{})
		grpReq.Add("member", []string{userDN})
		if err := conn.Modify(grpReq); err != nil {
			return fmt.Errorf("error adding user to group %s: %v", groupDN, err)
		}
	}

	for groupDN := range currentGroups {
		if desiredGroups[groupDN] {
			continue
		}
		changes = append(changes, operator.MembershipRemoved(groupDN))
		if isDryRun {
			o.LogInfo("[DRY RUN] Would remove %s from group %s", userDN, groupDN)
			continue
		}
		grpReq := ldap.NewModifyRequest(groupDN, []ldap.Control{})
		grpReq.Delete("member", []string{userDN})
		if err := conn.Modify(grpReq); err != nil {
			return fmt.Errorf("error removing user from group %s: %v", groupDN, err)
		}
	}

	if len(changes) > 0 {
		res.Record(operator.ActionUpdate, user.UID, user.Username, changes...)
	}
	return nil
}
