package dovecot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

type DoveadmRequest []any

type DovecotOperator struct {
	*operator.BaseOperator
	client *http.Client
}

func init() {
	operator.Register("dovecot-acl", func() operator.Operator {
		return &DovecotOperator{
			BaseOperator: operator.NewBaseOperator("dovecot-acl"),
			client: &http.Client{
				Timeout: 15 * time.Second,
			},
		}
	})
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
	result := &operator.SyncResult{}
	apiURL, _ := o.GetStringConfig("url")

	for _, identity := range state.Identities {
		err := o.exec(ctx, apiURL, identity.Username, "mailboxCreate", map[string]any{
			"user":    identity.Username,
			"mailbox": "INBOX", // Ensure at least INBOX exists
		}, state.DryRun)

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("user %s mailbox init failed: %w", identity.Username, err))
			continue
		}

		if aclStr, ok := identity.Attributes["dovecot_acl"].(string); ok && aclStr != "" {
			err = o.exec(ctx, apiURL, identity.Username, "aclSet", map[string]any{
				"user":    identity.Username,
				"mailbox": "INBOX",
				"right":   aclStr,
			}, state.DryRun)

			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("user %s acl sync failed: %w", identity.Username, err))
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

	payload := []DoveadmRequest{
		{command, params, "lexicore-sync-" + user},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if apiKey, ok := o.GetStringConfig("api_key"); ok == nil && apiKey != "" {
		req.Header.Set("Authorization", "X-Dovecot-API "+apiKey)
	} else if password, ok := o.GetStringConfig("password"); ok == nil && password != "" {
		req.SetBasicAuth("doveadm", password)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dovecot api error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (o *DovecotOperator) Close() error { return nil }
