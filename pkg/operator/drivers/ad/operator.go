package ad

import (
	"context"
	"fmt"
	"sync"

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

func (o *ADOperator) getConnection() (*ldap.Conn, error) {
	addr, _ := o.GetStringConfig("url")
	bindDN, _ := o.GetStringConfig("bindDN")
	pass, _ := o.GetStringConfig("bindPassword")

	l, err := ldap.DialURL(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial AD: %w", err)
	}

	if err := l.Bind(bindDN, pass); err != nil {
		l.Close()
		return nil, fmt.Errorf("ad-operator bind failed: %w", err)
	}

	return l, nil
}

func (o *ADOperator) Sync(ctx context.Context, state *operator.SyncState) error {
	searchBaseDN, _ := o.GetStringConfig("searchBaseDN")

	concurrency := o.GetConcurrency()
	if concurrency <= 1 {
		for uid, id := range state.Identities {
			o.syncIdentity(o.conn, uid, id, searchBaseDN, state.DryRun, state.Result, false)
		}
		return nil
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for uid, id := range state.Identities {
		wg.Add(1)
		go func() {
			sem <- struct{}{}
			defer func() {
				<-sem
				wg.Done()
			}()

			conn, err := o.getConnection()
			if err != nil {
				o.LogError(fmt.Errorf("failed to get connection for %s: %w", uid, err))
				return
			}
			defer conn.Close()

			o.syncIdentity(conn, uid, id, searchBaseDN, state.DryRun, state.Result, false)
		}()
	}

	wg.Wait()

	return nil
}

func (o *ADOperator) PartialSync(ctx context.Context, state *operator.PartialSyncState) error {
	searchBaseDN, _ := o.GetStringConfig("searchBaseDN")

	concurrency := o.GetConcurrency()
	if concurrency <= 1 {
		for uid, id := range state.Identities {
			o.syncIdentity(o.conn, uid, id, searchBaseDN, state.DryRun, state.Result, true)
		}
		return nil
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for uid, id := range state.Identities {
		wg.Add(1)
		go func() {
			sem <- struct{}{}
			defer func() {
				<-sem
				wg.Done()
			}()

			conn, err := o.getConnection()
			if err != nil {
				o.LogError(fmt.Errorf("failed to get connection for %s: %w", uid, err))
				return
			}
			defer conn.Close()

			o.syncIdentity(conn, uid, id, searchBaseDN, state.DryRun, state.Result, true)
		}()
	}

	wg.Wait()

	return nil
}

func (o *ADOperator) syncIdentity(
	conn *ldap.Conn,
	uid string,
	enriched source.Identity,
	searchBaseDN string,
	dryRun bool,
	res *operator.SyncResult,
	allowDelete bool,
) {
	o.LogInfo("checking user %s (uid: %s)", enriched.Email, uid)

	if allowDelete && enriched.Deleted {
		if err := o.deleteUser(conn, res, searchBaseDN, uid, enriched, dryRun); err != nil {
			o.LogError(fmt.Errorf("delete %s (uid: %s) failed: %w", enriched.Username, uid, err))
			res.RecordError(operator.ActionDelete, uid, enriched.Username, err)
		}
		return
	}

	userBaseDN, ok := o.getUserBaseDN(enriched)
	if !ok {
		return
	}

	desiredDN := o.buildDN(enriched, userBaseDN)
	attributesToSearch := o.buildAttributesList(enriched)

	sr, err := o.searchUser(conn, searchBaseDN, enriched.Username, attributesToSearch)
	if err != nil {
		o.LogError(fmt.Errorf("search failed for %s (uid: %s): %w", enriched.Username, uid, err))
		res.RecordError(operator.ActionSkip, uid, enriched.Username, err)
		return
	}

	var currentMemberOf []string
	if len(sr.Entries) == 0 {
		if dryRun {
			o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", enriched.Email, uid)
		} else {
			if err := o.createUser(conn, desiredDN, enriched); err != nil {
				o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", enriched.Username, uid, err))
				res.RecordError(operator.ActionCreate, uid, enriched.Username, err)
				return
			}
			currentMemberOf = make([]string, 0)
		}
		res.Record(operator.ActionCreate, uid, enriched.Username)
	} else {
		if err := o.updateUser(conn, res, desiredDN, *sr.Entries[0], enriched, dryRun); err != nil {
			o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", enriched.Username, uid, err))
			res.RecordError(operator.ActionUpdate, uid, enriched.Username, err)
		}
		currentMemberOf = sr.Entries[0].GetAttributeValues("memberOf")
	}

	if err := o.syncGroups(conn, res, desiredDN, currentMemberOf, enriched, dryRun); err != nil {
		o.LogError(fmt.Errorf("group sync %s (uid: %s) failed: %w", enriched.Username, uid, err))
		res.RecordError(operator.ActionUpdate, uid, enriched.Username, fmt.Errorf("group sync failed: %w", err))
	}
}

func (o *ADOperator) getUserBaseDN(enriched source.Identity) (string, bool) {
	userBaseDNRaw, ok := enriched.Attributes["baseDN"]
	if !ok {
		o.LogInfo("baseDN does not exist for %s, skipping", enriched.Username)
		return "", false
	}

	userBaseDN, ok := userBaseDNRaw.(string)
	if !ok {
		o.LogInfo("baseDN is not a string (%T): %v", userBaseDNRaw, userBaseDNRaw)
		return "", false
	}

	return userBaseDN, true
}

func (o *ADOperator) buildAttributesList(enriched source.Identity) []string {
	attributesToSearch := []string{"dn", "memberOf"}
	for k := range enriched.Attributes {
		if k == "baseDN" || k == "dn" || k == "adGroups" {
			continue
		}
		attributesToSearch = append(attributesToSearch, k)
	}
	return attributesToSearch
}

func (o *ADOperator) searchUser(conn *ldap.Conn, searchBaseDN, username string, attributes []string) (*ldap.SearchResult, error) {
	search := ldap.NewSearchRequest(
		searchBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", username),
		attributes,
		nil,
	)

	return conn.Search(search)
}

func (o *ADOperator) deleteUser(
	conn *ldap.Conn,
	res *operator.SyncResult,
	searchBaseDN string,
	uid string,
	id source.Identity,
	isDryRun bool,
) error {
	sr, err := o.searchUser(conn, searchBaseDN, id.Username, []string{"dn"})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		res.Record(operator.ActionDelete, uid, id.Username)
		return nil
	}

	userDN := sr.Entries[0].DN

	if isDryRun {
		o.LogInfo("[DRY RUN] Would delete user %s (DN: %s)", id.Username, userDN)
		res.Record(operator.ActionDelete, uid, id.Username)
		return nil
	}

	delReq := ldap.NewDelRequest(userDN, nil)
	if err := conn.Del(delReq); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	res.Record(operator.ActionDelete, uid, id.Username)
	o.LogInfo("Deleted user %s (DN: %s)", id.Username, userDN)
	return nil
}

func (o *ADOperator) Close() error {
	if o.conn != nil {
		o.conn.Close()
	}
	return nil
}
