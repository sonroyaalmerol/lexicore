package ad

import (
	"context"
	"fmt"

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
		o.syncIdentity(uid, &enriched, searchBaseDN, state.DryRun, res, false)
	}

	return res, nil
}

func (o *ADOperator) PartialSync(ctx context.Context, state *operator.PartialSyncState) (*operator.SyncResult, error) {
	searchBaseDN, _ := o.GetStringConfig("searchBaseDN")
	res := &operator.SyncResult{}

	for uid, id := range state.Identities {
		enriched := o.EnrichIdentity(id, state.Groups)
		o.syncIdentity(uid, &enriched, searchBaseDN, state.DryRun, res, true)
	}

	return res, nil
}

func (o *ADOperator) syncIdentity(
	uid string,
	enriched *source.Identity,
	searchBaseDN string,
	dryRun bool,
	res *operator.SyncResult,
	allowDelete bool,
) {
	o.LogInfo("checking user %s (uid: %s)", enriched.Email, uid)

	if allowDelete && enriched.Deleted {
		if err := o.deleteUser(res, searchBaseDN, enriched, dryRun); err != nil {
			o.LogError(fmt.Errorf("delete %s (uid: %s) failed: %w", enriched.Username, uid, err))
			res.RecordIdentityError(*enriched, operator.ActionDelete, err)
		}
		return
	}

	userBaseDN, ok := o.getUserBaseDN(enriched)
	if !ok {
		return
	}

	desiredDN := o.buildDN(*enriched, userBaseDN)
	attributesToSearch := o.buildAttributesList(enriched)

	sr, err := o.searchUser(searchBaseDN, enriched.Username, attributesToSearch)
	if err != nil {
		o.LogError(fmt.Errorf("search failed for %s (uid: %s): %w", enriched.Username, uid, err))
		res.RecordIdentityError(*enriched, operator.ActionNoOp, err)
		return
	}

	var currentMemberOf []string
	if len(sr.Entries) == 0 {
		if dryRun {
			o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", enriched.Email, uid)
		} else {
			if err := o.createUser(desiredDN, enriched); err != nil {
				o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", enriched.Username, uid, err))
				res.RecordIdentityError(*enriched, operator.ActionCreate, err)
				return
			}
			currentMemberOf = make([]string, 0)
		}
		res.RecordIdentityCreate(*enriched)
	} else {
		if err := o.updateUser(res, desiredDN, sr.Entries[0], enriched, dryRun); err != nil {
			o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
			res.RecordIdentityError(*enriched, operator.ActionUpdate, err)
		}
		currentMemberOf = sr.Entries[0].GetAttributeValues("memberOf")
	}

	if err := o.syncGroups(res, desiredDN, currentMemberOf, enriched, dryRun); err != nil {
		o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
		res.RecordIdentityError(*enriched, operator.ActionUpdate, fmt.Errorf("group sync failed: %w", err))
	}
}

func (o *ADOperator) getUserBaseDN(enriched *source.Identity) (string, bool) {
	userBaseDNRaw, ok := enriched.Attributes["baseDN"]
	if !ok {
		o.LogInfo("baseDN does not exist")
		return "", false
	}

	userBaseDN, ok := userBaseDNRaw.(string)
	if !ok {
		o.LogInfo("baseDN is not a string (%T): %v", userBaseDNRaw, userBaseDNRaw)
		return "", false
	}

	return userBaseDN, true
}

func (o *ADOperator) buildAttributesList(enriched *source.Identity) []string {
	attributesToSearch := []string{"dn", "memberOf"}
	for k := range enriched.Attributes {
		if k == "baseDN" || k == "dn" || k == "adGroups" {
			continue
		}
		attributesToSearch = append(attributesToSearch, k)
	}
	return attributesToSearch
}

func (o *ADOperator) searchUser(searchBaseDN, username string, attributes []string) (*ldap.SearchResult, error) {
	search := ldap.NewSearchRequest(
		searchBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", username),
		attributes,
		nil,
	)

	return o.conn.Search(search)
}

func (o *ADOperator) deleteUser(res *operator.SyncResult, searchBaseDN string, id *source.Identity, isDryRun bool) error {
	sr, err := o.searchUser(searchBaseDN, id.Username, []string{"dn"})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		res.RecordIdentityDelete(id.Username)
		return nil
	}

	userDN := sr.Entries[0].DN

	if isDryRun {
		o.LogInfo("[DRY RUN] Would delete user %s (DN: %s)", id.Username, userDN)
		res.RecordIdentityDelete(id.Username)
		return nil
	}

	delReq := ldap.NewDelRequest(userDN, nil)
	if err := o.conn.Del(delReq); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	res.RecordIdentityDelete(id.Username)
	o.LogInfo("Deleted user %s (DN: %s)", id.Username, userDN)
	return nil
}

func (o *ADOperator) Close() error {
	if o.conn != nil {
		o.conn.Close()
	}
	return nil
}
