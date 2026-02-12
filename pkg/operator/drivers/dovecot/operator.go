package dovecot

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

type DovecotOperator struct {
	*operator.BaseOperator
	baseURL     string
	Client      *http.Client
	b64Password string
}

type mailboxListResult struct {
	uid       string
	username  string
	mailboxes []string
	err       error
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

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")

	apiKey, _ := o.GetStringConfig("apiKey")
	o.b64Password = base64.URLEncoding.EncodeToString([]byte(apiKey))

	return nil
}

func (o *DovecotOperator) Validate(ctx context.Context) error {
	url, err := o.GetStringConfig("url")
	if err != nil || url == "" {
		return fmt.Errorf("dovecot: 'url' is required (e.g., http://localhost:8080/doveadm/v1)")
	}
	return nil
}

func (o *DovecotOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	result := &operator.SyncResult{}

	allMailboxes := make(map[string]struct{})
	mailboxDesiredACLs := make(map[string]map[string][]string)
	mailboxNoPropagate := make(map[string]bool)
	usersAffectedByMailbox := make(map[string]map[string]struct{})

	workers := o.GetConcurrency()
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	mailboxListChan := make(chan mailboxListResult, len(state.Identities))

	for uid, identity := range state.Identities {
		enriched := o.EnrichIdentity(identity, state.Groups)

		aclsAny, ok := enriched.Attributes[o.getACLAttrKey()]
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

		desiredAcls := o.mergeSharedMailAcls(acls)

		for _, aclStr := range desiredAcls {
			mailboxKey, rights, noPropagate := o.parseACLString(aclStr)
			if mailboxKey == "" {
				continue
			}

			allMailboxes[mailboxKey] = struct{}{}

			if mailboxDesiredACLs[mailboxKey] == nil {
				mailboxDesiredACLs[mailboxKey] = make(map[string][]string)
			}
			mailboxDesiredACLs[mailboxKey][identity.Username] = rights

			if noPropagate {
				mailboxNoPropagate[mailboxKey] = true
			}

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
			result.ErrCount.Add(1)
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

	expandedACLs := make(map[string]map[string][]string)
	for mailboxKey := range allMailboxes {
		expandedACLs[mailboxKey] = make(map[string][]string)
	}

	for mailboxKey, userACLs := range mailboxDesiredACLs {
		maps.Copy(expandedACLs[mailboxKey], userACLs)
	}

	for mailboxKey := range allMailboxes {
		parts := strings.SplitN(mailboxKey, "/", 2)
		if len(parts) != 2 {
			continue
		}

		sharedFolder := parts[0]
		mailboxPath := parts[1]

		if mailboxPath == "INBOX" {
			continue
		}

		pathParts := strings.Split(mailboxPath, "/")
		for i := len(pathParts) - 1; i > 0; i-- {
			parentPath := strings.Join(pathParts[:i], "/")
			parentKey := fmt.Sprintf("%s/%s", sharedFolder, parentPath)

			if mailboxNoPropagate[parentKey] {
				break
			}

			if parentACLs, exists := mailboxDesiredACLs[parentKey]; exists {
				for username, rights := range parentACLs {
					if _, hasExplicit := mailboxDesiredACLs[mailboxKey][username]; !hasExplicit {
						if expandedACLs[mailboxKey] == nil {
							expandedACLs[mailboxKey] = make(map[string][]string)
						}
						if _, alreadyInherited := expandedACLs[mailboxKey][username]; !alreadyInherited {
							expandedACLs[mailboxKey][username] = rights
						}
					}
				}
			}
		}

		inboxKey := fmt.Sprintf("%s/INBOX", sharedFolder)
		if !mailboxNoPropagate[inboxKey] {
			if inboxACLs, exists := mailboxDesiredACLs[inboxKey]; exists {
				for username, rights := range inboxACLs {
					if _, hasExplicit := mailboxDesiredACLs[mailboxKey][username]; !hasExplicit {
						if _, alreadyInherited := expandedACLs[mailboxKey][username]; !alreadyInherited {
							if expandedACLs[mailboxKey] == nil {
								expandedACLs[mailboxKey] = make(map[string][]string)
							}
							expandedACLs[mailboxKey][username] = rights
						}
					}
				}
			}
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
			fullMailboxPath := fmt.Sprintf("Other Users/%s", mailboxKey)

			currentACL, err := o.getMailboxACLs(ctx, fullMailboxPath)
			if err != nil {
				o.LogError(fmt.Errorf("acl get for %s failed: %w", fullMailboxPath, err))
				result.ErrCount.Add(1)
				return
			}

			desiredForMailbox := expandedACLs[mailboxKey]
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
				if err := o.applyMailboxACLs(ctx, sharedFolder, mailboxPath, diff, state.DryRun); err != nil {
					o.LogError(fmt.Errorf("failed to apply ACLs for %s: %w", mailboxKey, err))
					result.ErrCount.Add(1)
					return
				}

				usersWithChangesMu.Lock()
				for uid := range usersAffectedByMailbox[mailboxKey] {
					usersWithChanges[uid] = struct{}{}
				}
				usersWithChangesMu.Unlock()
			}
		})
	}

	wg2.Wait()

	result.IdentitiesUpdated.Add(uint64(len(usersWithChanges)))

	return result, nil
}

func (o *DovecotOperator) applyMailboxACLs(
	ctx context.Context,
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
