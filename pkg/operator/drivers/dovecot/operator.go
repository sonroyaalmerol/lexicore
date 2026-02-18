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
	baseURL         string
	domain          string
	Client          *http.Client
	b64Password     string
	removeAnyoneACL bool
}

func (o *DovecotOperator) Initialize(
	ctx context.Context,
	config map[string]any,
) error {
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

	anyone, hasAnyone := o.GetConfig("removeAnyoneACL")
	if hasAnyone {
		var ok bool
		o.removeAnyoneACL, ok = anyone.(bool)
		if !ok {
			o.removeAnyoneACL = false
		}
	}
	return nil
}

func (o *DovecotOperator) Validate(
	ctx context.Context,
) error {
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
	sharedFolder string,
	mailboxPath string,
	diff *MailboxDiff,
) error {
	syncCtx := syncCtxFrom(ctx)
	mailboxKey := fmt.Sprintf("%s/%s", sharedFolder, mailboxPath)

	for username, rights := range diff.ToSet {
		if syncCtx.state.DryRun {
			o.LogInfo(
				"[DRY RUN] Would set %s rights for %s to %s",
				rights, username, mailboxKey,
			)
		} else {
			if err := o.setMailboxACL(ctx, sharedFolder, mailboxPath, username, rights); err != nil {
				return fmt.Errorf("failed to set ACL for %s: %w", username, err)
			}
		}
	}

	for _, username := range diff.ToRemove {
		if syncCtx.state.DryRun {
			o.LogInfo(
				"[DRY RUN] Would remove rights for %s from %s",
				username, mailboxKey,
			)
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
