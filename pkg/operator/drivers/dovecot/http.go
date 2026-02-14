package dovecot

import (
	"bytes"
	"context"
	"encoding/json"
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

func (o *DovecotOperator) listMailboxesForUser(ctx context.Context, username string, pattern string) ([]string, error) {
	payload := []DoveadmRequest{
		{"mailboxList", map[string]any{
			"user":          username,
			"subscriptions": false,
			"mailboxMask": []string{
				pattern,
			},
		}, ""},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("X-Dovecot-API %s", o.b64Password))

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

		if doveErr.ExitCode == 67 {
			o.LogError(fmt.Errorf("user %s not found in Dovecot", username))
			return nil, nil
		}
		return nil, fmt.Errorf("doveadm error: %v", packet[1])

	case "doveadmResponse":
		var mailboxes []DoveadmMailbox
		dataBytes, _ := json.Marshal(packet[1])
		if err := json.Unmarshal(dataBytes, &mailboxes); err != nil {
			return nil, fmt.Errorf("failed to parse mailbox data: %w", err)
		}

		result := make([]string, 0, len(mailboxes))
		for _, m := range mailboxes {
			result = append(result, m.Mailbox)
		}

		return result, nil

	default:
		return nil, fmt.Errorf("unknown response type: %s", responseType)
	}
}

func (o *DovecotOperator) getAllNonPersonalMailbox(ctx context.Context, username string) ([]string, error) {
	payload := []DoveadmRequest{
		{"mailboxList", map[string]string{"user": username}, ""},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("X-Dovecot-API %s", o.b64Password))

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

		if doveErr.ExitCode == 67 {
			o.LogError(fmt.Errorf("user %s not found in Dovecot\n", username))
			return nil, nil
		}
		return nil, fmt.Errorf("doveadm error: %v", packet[1])

	case "doveadmResponse":
		var mailboxes []DoveadmMailbox
		dataBytes, _ := json.Marshal(packet[1])
		if err := json.Unmarshal(dataBytes, &mailboxes); err != nil {
			return nil, fmt.Errorf("failed to parse mailbox data: %w", err)
		}

		otherUsersMailboxes := make([]string, 0, len(mailboxes))
		for _, m := range mailboxes {
			if strings.HasPrefix(m.Mailbox, "Other Users/") {
				otherUsersMailboxes = append(otherUsersMailboxes, m.Mailbox)
			}
		}

		return otherUsersMailboxes, nil

	default:
		return nil, fmt.Errorf("unknown response type: %s", responseType)
	}
}

func (o *DovecotOperator) getMailboxACLs(ctx context.Context, fullMailboxPath string) (*MailboxACLResponse, error) {
	parts := strings.Split(fullMailboxPath, "/")

	sharedFolder := parts[0]
	mailboxPath := "INBOX"
	if len(parts) > 1 {
		mailboxPath = strings.Join(parts[1:], "/")
	}

	payload := []DoveadmRequest{
		{
			"aclGet", map[string]string{
				"user":    sharedFolder,
				"mailbox": mailboxPath,
			},
			"",
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("X-Dovecot-API %s", o.b64Password))

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
		return nil, err
	}

	acls := []ACLEntry{}

	if len(rawResponse) > 0 {
		packet := rawResponse[0]
		responseType := packet[0].(string)

		if responseType == "error" {
			return nil, fmt.Errorf("doveadm error for %s: %v", fullMailboxPath, packet[1])
		}

		if responseType == "doveadmResponse" {
			var rawACLs []DoveadmACLResponse
			dataBytes, _ := json.Marshal(packet[1])
			json.Unmarshal(dataBytes, &rawACLs)

			for _, raw := range rawACLs {
				entry := ACLEntry{
					Mailbox:      fullMailboxPath,
					SharedFolder: sharedFolder,
					MailboxPath:  mailboxPath,
					ID:           raw.ID,
					Global:       raw.Global,
					Rights:       strings.Fields(raw.Rights),
				}
				acls = append(acls, entry)
			}
		}
	}

	result := &MailboxACLResponse{
		Mailbox: fullMailboxPath,
		ACLs:    acls,
		Count:   len(acls),
	}

	return result, nil
}

func (o *DovecotOperator) setMailboxACL(ctx context.Context, sharedFolder string, mailboxPath string, username string, rights []string) error {
	userID := fmt.Sprintf("user=%s@%s", username, o.domain)
	rightsStr := strings.Join(rights, " ")

	payload := []DoveadmRequest{
		{
			"aclSet",
			map[string]string{
				"user":    sharedFolder,
				"mailbox": mailboxPath,
				"id":      userID,
				"rights":  rightsStr,
			},
			"",
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("X-Dovecot-API %s", o.b64Password))

	if err := o.GetLimiter().Wait(ctx); err != nil {
		return err
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var rawResponse [][]any
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(rawResponse) > 0 {
		packet := rawResponse[0]
		if len(packet) >= 1 {
			responseType, ok := packet[0].(string)
			if ok && responseType == "error" {
				return fmt.Errorf("doveadm error setting ACL: %v", packet[1])
			}
		}
	}

	return nil
}

func (o *DovecotOperator) deleteMailboxACL(ctx context.Context, sharedFolder string, mailboxPath string, username string) error {
	userID := fmt.Sprintf("user=%s@%s", username, o.domain)
	if strings.Contains(username, "@") {
		userID = username
	}

	payload := []DoveadmRequest{
		{
			"aclDelete",
			map[string]string{
				"user":    sharedFolder,
				"mailbox": mailboxPath,
				"id":      userID,
			},
			"",
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("X-Dovecot-API %s", o.b64Password))

	if err := o.GetLimiter().Wait(ctx); err != nil {
		return err
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var rawResponse [][]any
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(rawResponse) > 0 {
		packet := rawResponse[0]
		if len(packet) >= 1 {
			responseType, ok := packet[0].(string)
			if ok && responseType == "error" {
				return fmt.Errorf("doveadm error deleting ACL: %v", packet[1])
			}
		}
	}

	return nil
}
