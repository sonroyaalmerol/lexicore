package ad

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/go-ldap/ldap/v3"
)

type ADOperator struct {
	*operator.BaseOperator
	conn *ldap.Conn
}

func (o *ADOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	return nil
}

func (o *ADOperator) Validate(ctx context.Context) error {
	required := []string{"url", "bindDN", "bindPassword", "userBaseDN", "domain"}
	for _, req := range required {
		if v, _ := o.GetStringConfig(req); v == "" {
			return fmt.Errorf("ad-operator: '%s' is a required config field", req)
		}
	}
	return nil
}

func (o *ADOperator) Connect(ctx context.Context) error {
	addr, _ := o.GetStringConfig("url")
	bindDN, _ := o.GetStringConfig("bindDN")
	pass, _ := o.GetStringConfig("bindPassword")

	l, err := ldap.DialURL(addr)
	if err != nil {
		return fmt.Errorf("failed to dial AD: %w", err)
	}

	if err := l.Bind(bindDN, pass); err != nil {
		l.Close()
		return fmt.Errorf("ad-operator bind failed: %w", err)
	}

	o.conn = l
	return nil
}

func (o *ADOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	if err := o.Connect(ctx); err != nil {
		return nil, err
	}
	defer o.Close()

	res := &operator.SyncResult{}
	userBaseDN, _ := o.GetStringConfig("userBaseDN")

	for uid, id := range state.Identities {
		enriched := o.EnrichIdentity(id, state.Groups)

		dn := o.buildDN(enriched, userBaseDN)

		if state.DryRun {
			res.IdentitiesUpdated.Add(1)
			continue
		}

		search := ldap.NewSearchRequest(
			userBaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", enriched.Username),
			[]string{"dn", "userAccountControl"},
			nil,
		)

		sr, err := o.conn.Search(search)
		if err != nil {
			o.LogError(fmt.Errorf("search failed for %s (uid: %s): %w", enriched.Username, uid, err))
			res.ErrCount.Add(1)
			continue
		}

		if len(sr.Entries) == 0 {
			if err := o.createUser(dn, &enriched); err != nil {
				o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", enriched.Username, uid, err))
				res.ErrCount.Add(1)
			} else {
				res.IdentitiesCreated.Add(1)
			}
		} else {
			existingDN := sr.Entries[0].DN
			if err := o.updateUser(existingDN, &enriched); err != nil {
				o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
				res.ErrCount.Add(1)
			} else {
				res.IdentitiesUpdated.Add(1)
			}
		}
	}

	return res, nil
}

func (o *ADOperator) buildDN(id source.Identity, userBaseDN string) string {
	cn := id.DisplayName
	if cn == "" {
		cn = id.Username
	}
	return fmt.Sprintf("CN=%s,%s", cn, userBaseDN)
}

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

	if id.Email != "" {
		addReq.Attribute("mail", []string{id.Email})
	}
	if id.DisplayName != "" {
		addReq.Attribute("displayName", []string{id.DisplayName})
	}

	for k, v := range id.Attributes {
		attrName, hasPrefix := strings.CutPrefix(k, o.GetAttributePrefix())
		if !hasPrefix {
			continue
		}
		addReq.Attribute(attrName, []string{fmt.Sprintf("%v", v)})
	}

	if err := o.conn.Add(addReq); err != nil {
		return err
	}

	if initPass, ok := o.GetStringConfig("defaultPassword"); ok == nil {
		if err := o.setPassword(dn, initPass); err != nil {
			return err
		}
	}

	uacMod := ldap.NewModifyRequest(dn, nil)
	uacMod.Replace("userAccountControl", []string{"512"})
	return o.conn.Modify(uacMod)
}

func (o *ADOperator) setPassword(dn, password string) error {
	quoted := make([]rune, 0, len(password)+2)
	quoted = append(quoted, '"')
	quoted = append(quoted, []rune(password)...)
	quoted = append(quoted, '"')

	utf16Pass := utf16.Encode(quoted)
	b := make([]byte, len(utf16Pass)*2)
	for i, v := range utf16Pass {
		b[i*2] = byte(v)
		b[i*2+1] = byte(v >> 8)
	}

	passMod := ldap.NewModifyRequest(dn, nil)
	passMod.Replace("unicodePwd", []string{string(b)})
	if err := o.conn.Modify(passMod); err != nil {
		return fmt.Errorf("failed to set initial password: %w", err)
	}
	return nil
}

func (o *ADOperator) updateUser(dn string, id *source.Identity) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	for k, v := range id.Attributes {
		adKey, hasPrefix := strings.CutPrefix(k, o.GetAttributePrefix())
		if !hasPrefix {
			continue
		}

		val := fmt.Sprintf("%v", v)
		if val != "" {
			modReq.Replace(adKey, []string{val})
			hasChanges = true
		}
	}

	if !hasChanges {
		return nil
	}

	return o.conn.Modify(modReq)
}

func (o *ADOperator) Close() error {
	if o.conn != nil {
		o.conn.Close()
	}
	return nil
}
