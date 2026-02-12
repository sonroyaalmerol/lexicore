package ad

import (
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/text/encoding/unicode"
)

func (o *ADOperator) createUser(dn string, id *source.Identity) error {
	domain, _ := o.GetStringConfig("domain")

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"top", "person", "organizationalPerson", "user"})
	addReq.Attribute("sAMAccountName", []string{id.Username})

	var upnBuilder strings.Builder
	upnBuilder.Grow(len(id.Username) + len(domain) + 1)
	upnBuilder.WriteString(id.Username)
	upnBuilder.WriteByte('@')
	upnBuilder.WriteString(domain)
	addReq.Attribute("userPrincipalName", []string{upnBuilder.String()})
	addReq.Attribute("pwdLastSet", []string{"0"})

	if id.DisplayName != "" {
		addReq.Attribute("displayName", []string{id.DisplayName})
	}

	for k, v := range id.Attributes {
		addReq.Attribute(k, []string{fmt.Sprintf("%v", v)})
	}

	if err := o.conn.Add(addReq); err != nil {
		return err
	}

	if initPass, ok := o.GetTemplatedStringConfig("defaultPassword", id.Attributes); ok == nil {
		if err := o.setPassword(dn, initPass); err != nil {
			return err
		}
	}

	return nil
}

func (o *ADOperator) setPassword(dn, password string) error {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	pwdEncoded, _ := utf16.NewEncoder().String(fmt.Sprintf("\"%s\"", password))
	passReq := ldap.NewModifyRequest(dn, []ldap.Control{})
	passReq.Replace("unicodePwd", []string{pwdEncoded})
	if err := o.conn.Modify(passReq); err != nil {
		return fmt.Errorf("error setting user password: %v\n", err)
	}
	return nil
}

func (o *ADOperator) updateUser(res *operator.SyncResult, desiredDN string, entry *ldap.Entry, id *source.Identity, isDryRun bool) error {
	modReq := ldap.NewModifyRequest(entry.DN, nil)
	hasChanges := false

	diff := make(map[string]string)
	defer func() {
		if len(diff) > 0 {
			res.RecordIdentityUpdate(*id, diff)
		}
	}()

	for k, v := range id.Attributes {
		if k == "baseDN" || k == "adGroups" {
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
				diff[k] = utils.DiffArrString(currentVal, vArrStr)
				if isDryRun {
					o.LogInfo("[DRY RUN] Would set %s to %v for %s", k, vArrStr, id.Username)
				} else {
					modReq.Replace(k, vArrStr)
				}
				hasChanges = true

			}
		} else {
			currentVal := entry.GetAttributeValue(k)
			val := fmt.Sprintf("%v", v)
			if val != "" && currentVal != val {
				diff[k] = utils.DiffString(currentVal, val)
				if isDryRun {
					o.LogInfo("[DRY RUN] Would set %s to %v for %s", k, val, id.Username)
				} else {
					modReq.Replace(k, []string{val})
				}
				hasChanges = true
			}
		}
	}
	if !hasChanges {
		return nil
	}

	if !isDryRun {
		err := o.conn.Modify(modReq)
		if err != nil {
			return err
		}
	}

	if entry.DN == desiredDN {
		return nil
	}

	dnSplit := strings.SplitN(entry.DN, ",", 2)
	if len(dnSplit) != 2 {
		return nil
	}
	desiredDnSplit := strings.SplitN(desiredDN, ",", 2)
	if len(desiredDnSplit) != 2 {
		return nil
	}

	diff["dn"] = utils.DiffString(entry.DN, desiredDN)
	if isDryRun {
		o.LogInfo("[DRY RUN] Would move %s from %s to %s", dnSplit[0], entry.DN, desiredDnSplit[1])
	} else {
		moveReq := ldap.NewModifyDNRequest(entry.DN, dnSplit[0], true, desiredDnSplit[1])
		err := o.conn.ModifyDN(moveReq)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *ADOperator) syncGroups(res *operator.SyncResult, userDN string, currentMemberOf []string, user *source.Identity, isDryRun bool) error {
	desiredGroups := make(map[string]bool)
	groupDNAny, ok := user.Attributes["adGroups"]
	if !ok {
		return nil
	}

	groupDN, ok := groupDNAny.(string)
	if ok {
		desiredGroups[groupDN] = true
	} else {
		groupDNAnyArr, ok := groupDNAny.([]any)
		if !ok {
			return nil
		}

		for _, dnAny := range groupDNAnyArr {
			if dnStr, isStr := dnAny.(string); isStr {
				desiredGroups[dnStr] = true
			}
		}
	}

	currentGroups := make(map[string]bool)
	for _, memberOf := range currentMemberOf {
		currentGroups[memberOf] = true
	}

	for groupDN := range desiredGroups {
		if !currentGroups[groupDN] {
			res.RecordGroupMembership(*user, groupDN, operator.ActionGroupAdd)
			if isDryRun {
				o.LogInfo("[DRY RUN] Would add %s to group %s", userDN, groupDN)
			} else {
				grpReq := ldap.NewModifyRequest(groupDN, []ldap.Control{})
				grpReq.Add("member", []string{userDN})
				if err := o.conn.Modify(grpReq); err != nil {
					return fmt.Errorf("error adding user to group %s: %v", groupDN, err)
				}
			}
		}
	}

	for groupDN := range currentGroups {
		if !desiredGroups[groupDN] {
			res.RecordGroupMembership(*user, groupDN, operator.ActionGroupRem)
			if isDryRun {
				o.LogInfo("[DRY RUN] Would remove %s from group %s", userDN, groupDN)
			} else {
				grpReq := ldap.NewModifyRequest(groupDN, []ldap.Control{})
				grpReq.Delete("member", []string{userDN})
				if err := o.conn.Modify(grpReq); err != nil {
					return fmt.Errorf("error removing user from group %s: %v", groupDN, err)
				}
			}
		}
	}

	return nil
}
