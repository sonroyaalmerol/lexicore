package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type APIResponse struct {
	Success bool   `json:"_success"`
	Msg     string `json:"_msg"`
	Data    any    `json:"_data"`
}

type UserData struct {
	Mail              []string `json:"mail"`
	UID               []string `json:"uid"`
	CN                []string `json:"cn"`
	GivenName         []string `json:"givenName"`
	SN                []string `json:"sn"`
	PreferredLanguage []string `json:"preferredLanguage"`
	MailQuota         []string `json:"mailQuota"`
	AccountStatus     []string `json:"accountStatus"`
	Title             []string `json:"title"`
	EnabledService    []string `json:"enabledService"`
	ManagedDomains    []string `json:"managed_domains"`
	MailingAliases    []string `json:"mailing_aliases"`
	MailingLists      []string `json:"mailing_lists"`
	StoredBytes       int64    `json:"stored_bytes"`
	StoredMessages    int      `json:"stored_messages"`
	LastLogin         string   `json:"last_login"`
	HomeDirectory     []string `json:"homeDirectory"`
	ShadowLastChange  []string `json:"shadowLastChange"`
}

type IRedAdminOperator struct {
	*operator.BaseOperator
	Client  *http.Client
	baseURL string
}

func (o *IRedAdminOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")
	return nil
}

func (o *IRedAdminOperator) Validate(ctx context.Context) error {
	required := []string{"url", "username", "password"}
	for _, req := range required {
		if v, _ := o.GetStringConfig(req); v == "" {
			return fmt.Errorf("iredadmin: '%s' is required", req)
		}
	}
	return nil
}

func (o *IRedAdminOperator) login(ctx context.Context) error {
	user, _ := o.GetStringConfig("username")
	pass, _ := o.GetStringConfig("password")

	data := url.Values{}
	data.Set("username", user)
	data.Set("password", pass)

	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + 11)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/login")

	resp, err := o.Client.PostForm(urlBuilder.String(), data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	if !apiResp.Success {
		return fmt.Errorf("iredadmin login failed: %s", apiResp.Msg)
	}
	return nil
}

func (o *IRedAdminOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	result := &operator.SyncResult{
		Errors: make([]error, 0, len(state.Identities)/10),
	}

	for uid, id := range state.Identities {
		if id.Email == "" {
			continue
		}

		if id.Disabled {
			continue
		}

		userData, err := o.getUserData(ctx, id.Email)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("check user %s (uid: %s): %w", id.Email, uid, err))
			continue
		}

		if userData == nil {
			if state.DryRun {
				log.Printf("[DRY RUN] Would create user %s (uid: %s)", id.Email, uid)
				result.IdentitiesCreated++
			} else {
				if err := o.createUser(ctx, &id); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("create user %s (uid: %s): %w", id.Email, uid, err))
				} else {
					result.IdentitiesCreated++
				}
			}
		} else {
			changes := o.detectChanges(&id, userData)
			if len(changes) > 0 {
				if state.DryRun {
					log.Printf("[DRY RUN] Would update user %s (uid: %s) - changes: %v", id.Email, uid, changes)
					result.IdentitiesUpdated++
				} else {
					if err := o.updateUser(ctx, &id, changes); err != nil {
						result.Errors = append(result.Errors, fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
					} else {
						result.IdentitiesUpdated++
					}
				}
			} else {
				log.Printf("User %s (uid: %s) is up to date, no changes needed", id.Email, uid)
			}
		}
	}

	return result, nil
}

func (o *IRedAdminOperator) getUserData(ctx context.Context, email string) (*UserData, error) {
	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + len("/api/user/") + len(email) + 10)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/user/")
	urlBuilder.WriteString(url.PathEscape(email))

	resp, err := o.Client.Get(urlBuilder.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Success bool      `json:"_success"`
		Msg     string    `json:"_msg"`
		Data    *UserData `json:"_data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, nil
	}

	return apiResp.Data, nil
}

func (o *IRedAdminOperator) detectChanges(id *source.Identity, userData *UserData) map[string]string {
	changes := make(map[string]string)

	cn := id.DisplayName
	if cn == "" {
		cn = id.Username
	}
	if len(userData.CN) > 0 && userData.CN[0] != cn {
		changes["cn"] = cn
	}

	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "iredadmin:") {
			fieldName := k[10:]
			newValue := fmt.Sprintf("%v", v)

			existingValue := o.getFieldValue(userData, fieldName)
			if existingValue != newValue {
				changes[fieldName] = newValue
			}
		}
	}

	return changes
}

func (o *IRedAdminOperator) getFieldValue(userData *UserData, fieldName string) string {
	switch fieldName {
	case "cn":
		if len(userData.CN) > 0 {
			return userData.CN[0]
		}
	case "givenName":
		if len(userData.GivenName) > 0 {
			return userData.GivenName[0]
		}
	case "sn":
		if len(userData.SN) > 0 {
			return userData.SN[0]
		}
	case "preferredLanguage":
		if len(userData.PreferredLanguage) > 0 {
			return userData.PreferredLanguage[0]
		}
	case "mailQuota":
		if len(userData.MailQuota) > 0 {
			return userData.MailQuota[0]
		}
	case "accountStatus":
		if len(userData.AccountStatus) > 0 {
			return userData.AccountStatus[0]
		}
	case "title":
		if len(userData.Title) > 0 {
			return userData.Title[0]
		}
	}
	return ""
}

func (o *IRedAdminOperator) createUser(ctx context.Context, id *source.Identity) error {
	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + len("/api/user/") + len(id.Email) + 10)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/user/")
	urlBuilder.WriteString(url.PathEscape(id.Email))

	data := url.Values{}
	cn := id.DisplayName
	if cn == "" {
		cn = id.Username
	}
	data.Set("cn", cn)

	password := "ChangeMe123!"
	if p, ok := id.Attributes["mailPassword"].(string); ok && p != "" {
		password = p
	}
	data.Set("password", password)

	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "iredadmin:") {
			data.Set(k[10:], fmt.Sprintf("%v", v))
		}
	}

	return o.execPost(ctx, urlBuilder.String(), data)
}

func (o *IRedAdminOperator) updateUser(ctx context.Context, id *source.Identity, changes map[string]string) error {
	if len(changes) == 0 {
		return nil
	}

	data := url.Values{}
	for k, v := range changes {
		data.Set(k, v)
	}

	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + len("/api/user/") + len(id.Email) + 10)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/user/")
	urlBuilder.WriteString(url.PathEscape(id.Email))

	req, err := http.NewRequestWithContext(ctx, "PUT", urlBuilder.String(), strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return o.checkResponse(resp)
}

func (o *IRedAdminOperator) execPost(ctx context.Context, url string, data url.Values) error {
	resp, err := o.Client.PostForm(url, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return o.checkResponse(resp)
}

func (o *IRedAdminOperator) checkResponse(resp *http.Response) error {
	var apiResp APIResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse iRedAdmin response: %s", body)
	}

	if !apiResp.Success {
		return fmt.Errorf("iredadmin error: %s", apiResp.Msg)
	}
	return nil
}

func (o *IRedAdminOperator) Close() error { return nil }
