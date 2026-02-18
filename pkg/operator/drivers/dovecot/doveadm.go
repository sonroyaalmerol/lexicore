package dovecot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type DoveadmRequest []any

type DoveadmMailbox struct {
	Mailbox string `json:"mailbox"`
}

type DoveadmError struct {
	ExitCode int    `json:"exitCode"`
	Message  string `json:"message,omitempty"`
}

func (e *DoveadmError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("doveadm error (exit %d): %s", e.ExitCode, e.Message)
	}
	return fmt.Sprintf("doveadm error (exit %d)", e.ExitCode)
}

type ACLEntry struct {
	Mailbox      string   `json:"mailbox"`
	SharedFolder string   `json:"sharedFolder"`
	MailboxPath  string   `json:"mailboxPath"`
	ID           string   `json:"id"`
	Global       bool     `json:"global"`
	Rights       []string `json:"rights"`
}

type MailboxACLResponse struct {
	Mailbox string     `json:"mailbox"`
	ACLs    []ACLEntry `json:"acls"`
	Count   int        `json:"count"`
}

type DoveadmACLResponse struct {
	ID     string `json:"id"`
	Global bool   `json:"global"`
	Rights string `json:"rights"`
}

func (o *DovecotOperator) doveadmExec(
	ctx context.Context,
	payload []DoveadmRequest,
) (any, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(
		"Authorization",
		fmt.Sprintf("X-Dovecot-API %s", o.b64Password),
	)

	if err := o.GetLimiter().Wait(ctx); err != nil {
		return nil, err
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rawResponse [][]any
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(rawResponse) == 0 {
		return nil, nil
	}

	packet := rawResponse[0]
	if len(packet) < 2 {
		return nil, fmt.Errorf("malformed response packet")
	}

	responseType, ok := packet[0].(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type format")
	}

	switch responseType {
	case "error":
		var doveErr DoveadmError
		dataBytes, _ := json.Marshal(packet[1])
		json.Unmarshal(dataBytes, &doveErr)
		return nil, &doveErr
	case "doveadmResponse":
		return packet[1], nil
	default:
		return nil, fmt.Errorf("unknown response type: %s", responseType)
	}
}

func (o *DovecotOperator) doveadmListMailboxes(
	ctx context.Context,
	payload []DoveadmRequest,
) ([]string, error) {
	data, err := o.doveadmExec(ctx, payload)
	if err != nil {
		var doveErr *DoveadmError
		if errors.As(err, &doveErr) && doveErr.ExitCode == 67 {
			return nil, nil
		}
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var mailboxes []DoveadmMailbox
	dataBytes, _ := json.Marshal(data)
	if err := json.Unmarshal(dataBytes, &mailboxes); err != nil {
		return nil, fmt.Errorf("failed to parse mailbox data: %w", err)
	}

	result := make([]string, 0, len(mailboxes))
	for _, m := range mailboxes {
		result = append(result, m.Mailbox)
	}
	return result, nil
}

func (o *DovecotOperator) listMailboxesForUser(
	ctx context.Context,
	username string,
	pattern string,
) ([]string, error) {
	return o.doveadmListMailboxes(ctx, []DoveadmRequest{
		{"mailboxList", map[string]any{
			"user":        username,
			"mailboxMask": []string{pattern},
		}, ""},
	})
}

func (o *DovecotOperator) getAllNonPersonalMailbox(
	ctx context.Context,
	username string,
) ([]string, error) {
	mailboxes, err := o.doveadmListMailboxes(ctx, []DoveadmRequest{
		{"mailboxList", map[string]string{"user": username}, ""},
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(mailboxes))
	for _, m := range mailboxes {
		if strings.HasPrefix(m, "Other Users/") {
			result = append(result, m)
		}
	}
	return result, nil
}

func (o *DovecotOperator) getMailboxACLs(
	ctx context.Context,
	fullMailboxPath string,
) (*MailboxACLResponse, error) {
	parts := strings.Split(fullMailboxPath, "/")
	sharedFolder := parts[0]
	mailboxPath := "INBOX"
	if len(parts) > 1 {
		mailboxPath = strings.Join(parts[1:], "/")
	}

	data, err := o.doveadmExec(ctx, []DoveadmRequest{
		{"aclGet", map[string]string{
			"user":    sharedFolder,
			"mailbox": mailboxPath,
		}, ""},
	})
	if err != nil {
		return nil, fmt.Errorf(
			"doveadm error for %s: %w", fullMailboxPath, err,
		)
	}

	acls := []ACLEntry{}
	if data != nil {
		var rawACLs []DoveadmACLResponse
		dataBytes, _ := json.Marshal(data)
		json.Unmarshal(dataBytes, &rawACLs)

		for _, raw := range rawACLs {
			acls = append(acls, ACLEntry{
				Mailbox:      fullMailboxPath,
				SharedFolder: sharedFolder,
				MailboxPath:  mailboxPath,
				ID:           raw.ID,
				Global:       raw.Global,
				Rights:       strings.Fields(raw.Rights),
			})
		}
	}

	return &MailboxACLResponse{
		Mailbox: fullMailboxPath,
		ACLs:    acls,
		Count:   len(acls),
	}, nil
}

func (o *DovecotOperator) setMailboxACL(
	ctx context.Context,
	sharedFolder string,
	mailboxPath string,
	username string,
	rights []string,
) error {
	_, err := o.doveadmExec(ctx, []DoveadmRequest{
		{"aclSet", map[string]string{
			"user":    sharedFolder,
			"mailbox": mailboxPath,
			"id":      fmt.Sprintf("user=%s@%s", username, o.domain),
			"rights":  strings.Join(rights, " "),
		}, ""},
	})
	return err
}

func (o *DovecotOperator) deleteMailboxACL(
	ctx context.Context,
	sharedFolder string,
	mailboxPath string,
	username string,
) error {
	userID := fmt.Sprintf("user=%s@%s", username, o.domain)
	if strings.Contains(username, "@") || username == "anyone" {
		userID = username
	}

	_, err := o.doveadmExec(ctx, []DoveadmRequest{
		{"aclDelete", map[string]string{
			"user":    sharedFolder,
			"mailbox": mailboxPath,
			"id":      userID,
		}, ""},
	})
	return err
}
