package dovecot

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

type DovecotOperator struct {
	*operator.BaseOperator
	baseURL      string
	domain       string
	Client       *http.Client
	b64Password  string
	ignoreAnyone bool
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
			Timeout: 120 * time.Second,
		}
	}

	o.domain, _ = o.GetStringConfig("domain")

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")

	apiKey, _ := o.GetStringConfig("apiKey")
	o.b64Password = base64.URLEncoding.EncodeToString([]byte(apiKey))

	anyone, hasAnyone := o.GetConfig("ignoreAnyone")
	if hasAnyone {
		var ok bool
		o.ignoreAnyone, ok = anyone.(bool)
		if !ok {
			o.ignoreAnyone = false
		}
	}
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
