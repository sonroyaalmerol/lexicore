package ldap

import (
	"context"
	"fmt"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"github.com/go-ldap/ldap/v3"
)

func (o *LDAPOperator) PartialSync(ctx context.Context, state *operator.PartialSyncState) (*operator.SyncResult, error) {
	if err := o.Connect(ctx); err != nil {
		return nil, err
	}
	defer o.Close()

	res := &operator.SyncResult{}

	userBaseDN, _ := o.GetStringConfig("userBaseDN")
	rdnAttr, _ := o.GetStringConfig("rdnAttribute")
	if rdnAttr == "" {
		rdnAttr = "uid"
	}

	for uid, id := range state.Identities {
		enriched := o.EnrichIdentity(id, state.Groups)

		rdnValue := enriched.Username
		if enriched.UID != "" {
			rdnValue = enriched.UID
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
			res.RecordIdentityUpdate(id, nil)
			continue
		}

		if enriched.Deleted {
			delReq := ldap.NewDelRequest(dn, nil)
			if err := o.conn.Del(delReq); err != nil {
				if !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
					o.LogError(fmt.Errorf("delete %s (uid: %s) failed: %w", dn, uid, err))
					res.RecordIdentityError(enriched, operator.ActionDelete, err)
				} else {
					res.RecordIdentityDelete(id.Username)
				}
			} else {
				res.RecordIdentityDelete(id.Username)
			}
			continue
		}

		search := ldap.NewSearchRequest(
			dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			0, 0, false, "(objectClass=*)", []string{"dn"}, nil,
		)
		sr, err := o.conn.Search(search)

		if err != nil || len(sr.Entries) == 0 {
			if err := o.createEntry(dn, &enriched); err != nil {
				o.LogError(fmt.Errorf("create %s (uid: %s) failed: %w", dn, uid, err))
				res.RecordIdentityError(enriched, operator.ActionUpdate, err)
			} else {
				res.RecordIdentityCreate(id)
			}
		} else {
			if err := o.updateEntry(dn, &enriched); err != nil {
				o.LogError(fmt.Errorf("update %s (uid: %s) failed: %w", dn, uid, err))
				res.RecordIdentityError(enriched, operator.ActionUpdate, err)
			} else {
				res.RecordIdentityUpdate(id, nil)
			}
		}
	}

	return res, nil
}
