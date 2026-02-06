package dovecot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

type DoveadmRequest []any

type DovecotOperator struct {
	*operator.BaseOperator
	Client *http.Client
}

func (o *DovecotOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	return o.Validate(ctx)
}

func (o *DovecotOperator) Validate(ctx context.Context) error {
	url, err := o.GetStringConfig("url")
	if err != nil || url == "" {
		return fmt.Errorf("dovecot: 'url' is required (e.g., http://localhost:8080/doveadm/v1)")
	}
	return nil
}

func (o *DovecotOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	result := &operator.SyncResult{
		Errors: make([]error, 0, len(state.Identities)/10),
	}
	apiURL, _ := o.GetStringConfig("url")

	for uid, identity := range state.Identities {
		err := o.exec(ctx, apiURL, identity.Username, "mailboxCreate", map[string]any{
			"user":    identity.Username,
			"mailbox": "INBOX",
		}, state.DryRun)

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("user %s (uid: %s) mailbox init failed: %w", identity.Username, uid, err))
			continue
		}

		if aclStr, ok := identity.Attributes["dovecot_acl"].(string); ok && aclStr != "" {
			err = o.exec(ctx, apiURL, identity.Username, "aclSet", map[string]any{
				"user":    identity.Username,
				"mailbox": "INBOX",
				"right":   aclStr,
			}, state.DryRun)

			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("user %s (uid: %s) acl sync failed: %w", identity.Username, uid, err))
				continue
			}
		}

		result.IdentitiesUpdated++
	}

	return result, nil
}

func (o *DovecotOperator) exec(ctx context.Context, apiURL, user, command string, params map[string]any, dryRun bool) error {
	if dryRun {
		return nil
	}

	var tagBuilder strings.Builder
	tagBuilder.Grow(len("lexicore-sync-") + len(user))
	tagBuilder.WriteString("lexicore-sync-")
	tagBuilder.WriteString(user)

	payload := []DoveadmRequest{
		{command, params, tagBuilder.String()},
	}

	buf := bytes.NewBuffer(make([]byte, 0, 256))
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if apiKey, ok := o.GetStringConfig("api_key"); ok == nil && apiKey != "" {
		var authBuilder strings.Builder
		authBuilder.Grow(16 + len(apiKey))
		authBuilder.WriteString("X-Dovecot-API ")
		authBuilder.WriteString(apiKey)
		req.Header.Set("Authorization", authBuilder.String())
	} else if password, ok := o.GetStringConfig("password"); ok == nil && password != "" {
		req.SetBasicAuth("doveadm", password)
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dovecot api error (%d): %s", resp.StatusCode, body)
	}

	return nil
}

func (o *DovecotOperator) Close() error { return nil }
