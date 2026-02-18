package ldap

import (
	"context"
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type LDAPOperator struct {
	*operator.BaseOperator
	conn *ldap.Conn
}

func (o *LDAPOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	return nil
}

func (o *LDAPOperator) Validate(ctx context.Context) error {
	required := []string{"url", "bindDN", "bindPassword", "userBaseDN"}
	for _, req := range required {
		if v, _ := o.GetStringConfig(req); v == "" {
			return fmt.Errorf("ldap-operator: '%s' is required", req)
		}
	}
	return nil
}

func (o *LDAPOperator) Connect(ctx context.Context) error {
	addr, _ := o.GetStringConfig("url")
	bindDN, _ := o.GetStringConfig("bindDN")
	pass, _ := o.GetStringConfig("bindPassword")

	l, err := ldap.DialURL(addr)
	if err != nil {
		return fmt.Errorf("failed to dial LDAP: %w", err)
	}

	if err := l.Bind(bindDN, pass); err != nil {
		l.Close()
		return fmt.Errorf("ldap-operator bind failed: %w", err)
	}

	o.conn = l
	return nil
}

func (o *LDAPOperator) buildDN(id source.Identity) string {
	userBaseDN, _ := o.GetStringConfig("userBaseDN")
	rdnAttr, _ := o.GetStringConfig("rdnAttribute")
	if rdnAttr == "" {
		rdnAttr = "uid"
	}

	rdnValue := id.Username
	if id.UID != "" {
		rdnValue = id.UID
	}

	var dnBuilder strings.Builder
	dnBuilder.Grow(len(rdnAttr) + 1 + len(rdnValue) + 1 + len(userBaseDN))
	dnBuilder.WriteString(rdnAttr)
	dnBuilder.WriteByte('=')
	dnBuilder.WriteString(rdnValue)
	dnBuilder.WriteByte(',')
	dnBuilder.WriteString(userBaseDN)
	return dnBuilder.String()
}

func (o *LDAPOperator) Sync(ctx context.Context, state *operator.SyncState) error {
	if err := o.Connect(ctx); err != nil {
		return err
	}
	defer o.Close()

	for uid, id := range state.Identities {
		dn := o.buildDN(id)

		if state.DryRun {
			o.LogInfo("[DRY RUN] Would sync user %s (uid: %s)", dn, uid)
			continue
		}

		search := ldap.NewSearchRequest(
			dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			0, 0, false, "(objectClass=*)", []string{"dn"}, nil,
		)
		sr, err := o.conn.Search(search)

		if err != nil || len(sr.Entries) == 0 {
			if err := o.createEntry(dn, &id); err != nil {
				o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", dn, uid, err))
				state.Result.RecordError(operator.ActionCreate, id.UID, id.Username, err)
			} else {
				state.Result.Record(operator.ActionCreate, id.UID, id.Username)
			}
		} else {
			changes, err := o.updateEntry(dn, &id)
			if err != nil {
				o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", dn, uid, err))
				state.Result.RecordError(operator.ActionUpdate, id.UID, id.Username, err)
			} else if len(changes) > 0 {
				state.Result.Record(operator.ActionUpdate, id.UID, id.Username, changes...)
			}
		}
	}

	return nil
}

func (o *LDAPOperator) createEntry(dn string, id *source.Identity) error {
	addReq := ldap.NewAddRequest(dn, nil)

	classes := []string{"top", "person", "organizationalPerson", "inetOrgPerson"}
	if customClasses, ok := o.GetConfig("objectClasses"); ok {
		if c, ok := customClasses.([]string); ok {
			classes = c
		}
	}
	addReq.Attribute("objectClass", classes)

	addReq.Attribute("cn", []string{id.Username})
	addReq.Attribute("sn", []string{id.Username})
	if id.Email != "" {
		addReq.Attribute("mail", []string{id.Email})
	}

	for k, v := range id.Attributes {
		addReq.Attribute(k, []string{fmt.Sprintf("%v", v)})
	}

	return o.conn.Add(addReq)
}

func (o *LDAPOperator) updateEntry(dn string, id *source.Identity) ([]operator.Change, error) {
	modReq := ldap.NewModifyRequest(dn, nil)
	var changes []operator.Change

	for k, v := range id.Attributes {
		val := fmt.Sprintf("%v", v)
		modReq.Replace(k, []string{val})
		changes = append(changes, operator.AttrChange(k, "", val))
	}

	if len(changes) == 0 {
		return nil, nil
	}

	return changes, o.conn.Modify(modReq)
}

func (o *LDAPOperator) Close() error {
	if o.conn != nil {
		return o.conn.Close()
	}
	return nil
}
