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

func (o *ADOperator) createUser(conn *ldap.Conn, dn string, id source.Identity) error {
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

	if err := conn.Add(addReq); err != nil {
		return err
	}

	if initPass, ok := o.GetTemplatedStringConfig("defaultPassword", id.Attributes); ok == nil {
		if err := o.setPassword(conn, dn, initPass); err != nil {
			return err
		}
	}

	return nil
}

func (o *ADOperator) setPassword(conn *ldap.Conn, dn, password string) error {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	pwdEncoded, _ := utf16.NewEncoder().String(fmt.Sprintf("\"%s\"", password))
	passReq := ldap.NewModifyRequest(dn, []ldap.Control{})
	passReq.Replace("unicodePwd", []string{pwdEncoded})
	if err := conn.Modify(passReq); err != nil {
		return fmt.Errorf("error setting user password: %v\n", err)
	}
	return nil
}

func (o *ADOperator) updateUser(
	conn *ldap.Conn,
	res *operator.SyncResult,
	desiredDN string,
	entry ldap.Entry,
	id source.Identity,
	isDryRun bool,
) error {
	modReq := ldap.NewModifyRequest(entry.DN, nil)
	var changes []operator.Change

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

	if len(changes) == 0 {
		return nil
	}

	if !isDryRun {
		if err := conn.Modify(modReq); err != nil {
			return err
		}
	}

	if entry.DN != desiredDN {
		dnSplit := strings.SplitN(entry.DN, ",", 2)
		desiredDnSplit := strings.SplitN(desiredDN, ",", 2)
		if len(dnSplit) == 2 && len(desiredDnSplit) == 2 {
			changes = append(changes, operator.AttrChange("dn", entry.DN, desiredDN))
			if !isDryRun {
				moveReq := ldap.NewModifyDNRequest(entry.DN, dnSplit[0], true, desiredDnSplit[1])
				if err := conn.ModifyDN(moveReq); err != nil {
					return err
				}
			}
		}
	}

	res.Record(operator.ActionUpdate, id.UID, id.Username, changes...)
	return nil
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
