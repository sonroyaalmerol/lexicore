package ldap

import (
	"context"
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"github.com/go-ldap/ldap/v3"
)

func (o *LDAPOperator) PartialSync(ctx context.Context, state *operator.PartialSyncState) error {
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

		if id.Deleted {
			delReq := ldap.NewDelRequest(dn, nil)
			if err := o.conn.Del(delReq); err != nil {
				if !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
					o.LogError(fmt.Errorf("delete %s (uid: %s) failed: %w", dn, uid, err))
					state.Result.RecordError(operator.ActionDelete, id.UID, id.Username, err)
				} else {
					state.Result.Record(operator.ActionDelete, id.UID, id.Username)
				}
			} else {
				state.Result.Record(operator.ActionDelete, id.UID, id.Username)
			}
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
