package ad

import (
	"context"
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/operator"
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
	required := []string{"url", "bindDN", "bindPassword", "searchBaseDN", "domain"}
	for _, req := range required {
		if v, _ := o.GetStringConfig(req); v == "" {
			return fmt.Errorf("ad-operator: '%s' is a required config field", req)
		}
	}

	if err := o.Connect(ctx); err != nil {
		return err
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
	searchBaseDN, _ := o.GetStringConfig("searchBaseDN")

	res := &operator.SyncResult{}
	for uid, id := range state.Identities {
		enriched := o.EnrichIdentity(id, state.Groups)

		o.LogInfo("checking user %s (uid: %s)", id.Email, uid)

		attributesToSearch := []string{"dn", "memberOf"}
		for k := range enriched.Attributes {
			if k == "baseDN" || k == "dn" || k == "adGroups" {
				continue
			}
			attributesToSearch = append(attributesToSearch, k)
		}

		userBaseDNRaw, ok := enriched.Attributes["baseDN"]
		if !ok {
			o.LogInfo("baseDN does not exist")
			continue
		}

		userBaseDN, ok := userBaseDNRaw.(string)
		if !ok {
			o.LogInfo("baseDN is not a string (%T): %v", userBaseDNRaw, userBaseDNRaw)
			continue
		}

		desiredDN := o.buildDN(enriched, userBaseDN)

		search := ldap.NewSearchRequest(
			searchBaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", enriched.Username),
			attributesToSearch,
			nil,
		)

		sr, err := o.conn.Search(search)
		if err != nil {
			o.LogError(fmt.Errorf("search failed for %s (uid: %s): %w", enriched.Username, uid, err))
			res.RecordIdentityError(enriched, operator.ActionNoOp, err)
			continue
		}

		var currentMemberOf []string
		if len(sr.Entries) == 0 {
			if state.DryRun {
				o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", id.Email, uid)
			} else {
				if err := o.createUser(desiredDN, &enriched); err != nil {
					o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", enriched.Username, uid, err))
					res.RecordIdentityError(enriched, operator.ActionCreate, err)
					continue
				}
				currentMemberOf = make([]string, 0)
			}
			res.RecordIdentityCreate(enriched)
		} else {
			if err := o.updateUser(res, desiredDN, sr.Entries[0], &enriched, state.DryRun); err != nil {
				o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
				res.RecordIdentityError(enriched, operator.ActionUpdate, err)
			}
			currentMemberOf = sr.Entries[0].GetAttributeValues("memberOf")
		}

		err = o.syncGroups(res, desiredDN, currentMemberOf, &enriched, state.DryRun)
		if err != nil {
			o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
			res.RecordIdentityError(enriched, operator.ActionUpdate, fmt.Errorf("group sync failed: %w", err))
		}
	}

	return res, nil
}

func (o *ADOperator) Close() error {
	if o.conn != nil {
		o.conn.Close()
	}
	return nil
}
