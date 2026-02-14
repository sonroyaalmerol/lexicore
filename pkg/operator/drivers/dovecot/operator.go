package dovecot

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type DovecotOperator struct {
	*operator.BaseOperator
	baseURL     string
	domain      string
	Client      *http.Client
	b64Password string
}

type mailboxListResult struct {
	uid       string
	username  string
	mailboxes []string
	err       error
	identity  source.Identity
}

func (o *DovecotOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	if o.Client == nil {
		o.Client = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
		}
	}

	o.domain, _ = o.GetStringConfig("domain")

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")

	apiKey, _ := o.GetStringConfig("apiKey")
	o.b64Password = base64.URLEncoding.EncodeToString([]byte(apiKey))

	return nil
}

func (o *DovecotOperator) Validate(ctx context.Context) error {
	required := []string{"url", "apiKey", "domain"}
	for _, req := range required {
		if v, _ := o.GetStringConfig(req); v == "" {
			return fmt.Errorf("dovecot-acl: '%s' is required", req)
		}
	}
	return nil
}

func (o *DovecotOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	result := &operator.SyncResult{}

	allMailboxes := make(map[string]struct{})
	mailboxDesiredACLs := make(map[string]map[string][]string)
	usersAffectedByMailbox := make(map[string]map[string]struct{})

	workers := o.GetConcurrency()
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	mailboxListChan := make(chan mailboxListResult, len(state.Identities))

	for uid, identity := range state.Identities {
		enriched := o.EnrichIdentity(identity, state.Groups)

		aclsAny, ok := enriched.Attributes["acls"]
		if !ok {
			continue
		}

		aclsAnyArr, isArr := aclsAny.([]any)
		if !isArr {
			continue
		}

		acls := make([]string, 0, len(aclsAnyArr))
		for _, anyVal := range aclsAnyArr {
			strVal, isStr := anyVal.(string)
			if !isStr {
				continue
			}
			acls = append(acls, strVal)
		}

		o.LogInfo("checking user %s (uid: %s)", identity.Email, uid)

		expandedACLs, err := o.expandACLPatterns(ctx, acls)
		if err != nil {
			o.LogError(fmt.Errorf("failed to expand ACL patterns for user %s: %w", identity.Username, err))
			result.RecordIdentityError(identity, operator.ActionNoOp, err)
			continue
		}

		desiredAcls := o.mergeSharedMailAcls(expandedACLs)

		o.LogInfo("desiredAcls: %v", desiredAcls)

		for _, aclStr := range desiredAcls {
			mailboxKey, rights := o.parseACLString(aclStr)
			if mailboxKey == "" {
				continue
			}

			o.LogInfo("mailboxKey: %v", mailboxKey)

			allMailboxes[mailboxKey] = struct{}{}

			if mailboxDesiredACLs[mailboxKey] == nil {
				mailboxDesiredACLs[mailboxKey] = make(map[string][]string)
			}
			mailboxDesiredACLs[mailboxKey][identity.Username] = rights

			if usersAffectedByMailbox[mailboxKey] == nil {
				usersAffectedByMailbox[mailboxKey] = make(map[string]struct{})
			}
			usersAffectedByMailbox[mailboxKey][uid] = struct{}{}
		}

		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			mailboxList, err := o.getAllNonPersonalMailbox(ctx, identity.Username)
			mailboxListChan <- mailboxListResult{
				uid:       uid,
				username:  identity.Username,
				mailboxes: mailboxList,
				err:       err,
				identity:  identity,
			}
		})
	}

	go func() {
		wg.Wait()
		close(mailboxListChan)
	}()

	for res := range mailboxListChan {
		if res.err != nil {
			o.LogError(fmt.Errorf("user %s (uid: %s) mailbox list failed: %w", res.username, res.uid, res.err))
			result.RecordIdentityError(res.identity, operator.ActionNoOp, res.err)
			continue
		}

		for _, fullMailbox := range res.mailboxes {
			trimmed := strings.TrimPrefix(fullMailbox, "Other Users/")
			allMailboxes[trimmed] = struct{}{}

			if usersAffectedByMailbox[trimmed] == nil {
				usersAffectedByMailbox[trimmed] = make(map[string]struct{})
			}
			usersAffectedByMailbox[trimmed][res.uid] = struct{}{}
		}
	}

	var usersWithChangesMu sync.Mutex
	usersWithChanges := make(map[string]struct{})

	var wg2 sync.WaitGroup

	for mailboxKey := range allMailboxes {
		select {
		case <-ctx.Done():
			wg2.Wait()
			return result, ctx.Err()
		default:
		}

		wg2.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			parts := strings.SplitN(mailboxKey, "/", 2)
			if len(parts) != 2 {
				return
			}

			sharedFolder := parts[0]
			mailboxPath := parts[1]
			currentACL, err := o.getMailboxACLs(ctx, mailboxKey)
			if err != nil {
				o.LogError(fmt.Errorf("acl get for %s failed: %w", mailboxKey, err))
				return
			}

			desiredForMailbox := mailboxDesiredACLs[mailboxKey]
			if desiredForMailbox == nil {
				desiredForMailbox = make(map[string][]string)
			}

			diff := o.calculateMailboxDiff(
				sharedFolder,
				mailboxPath,
				currentACL,
				desiredForMailbox,
			)

			if len(diff.ToSet) > 0 || len(diff.ToRemove) > 0 {
				if err := o.applyMailboxACLs(ctx, result, sharedFolder, mailboxPath, diff, state.DryRun); err != nil {
					o.LogError(fmt.Errorf("failed to apply ACLs for %s: %w", mailboxKey, err))
					return
				}

				usersWithChangesMu.Lock()
				for uid := range usersAffectedByMailbox[mailboxKey] {
					username := state.Identities[uid].Username
					if diff.DiffReport[username] != "" {
						usersWithChanges[uid] = struct{}{}
						result.RecordIdentityUpdateManual(uid, username, map[string]string{
							fmt.Sprintf("%s/%s", sharedFolder, mailboxPath): diff.DiffReport[username],
						})
					}
				}
				usersWithChangesMu.Unlock()
			}
		})
	}

	wg2.Wait()

	return result, nil
}

func (o *DovecotOperator) applyMailboxACLs(
	ctx context.Context,
	result *operator.SyncResult,
	sharedFolder string,
	mailboxPath string,
	diff *MailboxDiff,
	isDryRun bool,
) error {
	for username, rights := range diff.ToSet {
		if isDryRun {
			o.LogInfo("[DRY RUN] Would set %s rights for %s to %s/%s", rights, username, sharedFolder, mailboxPath)
		} else {
			if err := o.setMailboxACL(ctx, sharedFolder, mailboxPath, username, rights); err != nil {
				return fmt.Errorf("failed to set ACL for %s: %w", username, err)
			}
		}
	}

	for _, username := range diff.ToRemove {
		result.RecordIdentityUpdateManual(username, username, map[string]string{
			fmt.Sprintf("%s/%s", sharedFolder, mailboxPath): "REMOVE_ALL",
		})
		if isDryRun {
			o.LogInfo("[DRY RUN] Would remove rights for %s from %s/%s", username, sharedFolder, mailboxPath)
		} else {
			if err := o.deleteMailboxACL(ctx, sharedFolder, mailboxPath, username); err != nil {
				return fmt.Errorf("failed to delete ACL for %s: %w", username, err)
			}
		}
	}

	return nil
}

func (o *DovecotOperator) Close() error {
	return nil
}
