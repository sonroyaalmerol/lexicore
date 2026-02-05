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

func init() {
	operator.Register("ldap", func() operator.Operator {
		return &LDAPOperator{
			BaseOperator: operator.NewBaseOperator("ldap"),
		}
	})
}

func (o *LDAPOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	return o.Validate(ctx)
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

func (o *LDAPOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	if err := o.Connect(ctx); err != nil {
		return nil, err
	}
	defer o.Close()

	res := &operator.SyncResult{
		Errors: make([]error, 0, len(state.Identities)/10),
	}

	userBaseDN, _ := o.GetStringConfig("userBaseDN")
	rdnAttr, _ := o.GetStringConfig("rdnAttribute")
	if rdnAttr == "" {
		rdnAttr = "uid"
	}

	for uid, id := range state.Identities {
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
		dn := dnBuilder.String()

		if state.DryRun {
			res.IdentitiesUpdated++
			continue
		}

		search := ldap.NewSearchRequest(
			dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			0, 0, false, "(objectClass=*)", []string{"dn"}, nil,
		)
		sr, err := o.conn.Search(search)

		if err != nil || len(sr.Entries) == 0 {
			if err := o.createEntry(dn, &id); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("create %s (uid: %s) failed: %w", dn, uid, err))
			} else {
				res.IdentitiesCreated++
			}
		} else {
			if err := o.updateEntry(dn, &id); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("update %s (uid: %s) failed: %w", dn, uid, err))
			} else {
				res.IdentitiesUpdated++
			}
		}
	}

	return res, nil
}

func (o *LDAPOperator) createEntry(dn string, id *source.Identity) error {
	addReq := ldap.NewAddRequest(dn, nil)

	// Default object classes for a generic user
	classes := []string{"top", "person", "organizationalPerson", "inetOrgPerson"}
	if customClasses, ok := o.GetConfig("objectClasses"); ok {
		if c, ok := customClasses.([]string); ok {
			classes = c
		}
	}
	addReq.Attribute("objectClass", classes)

	// Standard attributes
	addReq.Attribute("cn", []string{id.Username})
	addReq.Attribute("sn", []string{id.Username})
	if id.Email != "" {
		addReq.Attribute("mail", []string{id.Email})
	}

	// Dynamic attributes from Lexicore
	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "ldap:") {
			attrName := k[5:] // "ldap:" is 5 bytes
			addReq.Attribute(attrName, []string{fmt.Sprintf("%v", v)})
		}
	}

	return o.conn.Add(addReq)
}

func (o *LDAPOperator) updateEntry(dn string, id *source.Identity) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "ldap:") {
			attrName := k[5:]
			modReq.Replace(attrName, []string{fmt.Sprintf("%v", v)})
			hasChanges = true
		}
	}

	if !hasChanges {
		return nil
	}

	return o.conn.Modify(modReq)
}

func (o *LDAPOperator) Close() error {
	if o.conn != nil {
		return o.conn.Close()
	}
	return nil
}
