package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type APIResponse struct {
	Success bool   `json:"_success"`
	Msg     string `json:"_msg"`
	Data    any    `json:"_data"`
}

type IRedAdminOperator struct {
	*operator.BaseOperator
	client  *http.Client
	baseURL string
}

func init() {
	operator.Register("iredadmin", func() operator.Operator {
		jar, _ := cookiejar.New(nil)
		return &IRedAdminOperator{
			BaseOperator: operator.NewBaseOperator("iredadmin"),
			client: &http.Client{
				Jar:     jar,
				Timeout: 15 * time.Second,
			},
		}
	})
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

func (o *IRedAdminOperator) Login(ctx context.Context) error {
	user, _ := o.GetStringConfig("username")
	pass, _ := o.GetStringConfig("password")

	data := url.Values{}
	data.Set("username", user)
	data.Set("password", pass)

	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + 11)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/login")

	resp, err := o.client.PostForm(urlBuilder.String(), data)
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
	if err := o.Login(ctx); err != nil {
		return nil, err
	}

	result := &operator.SyncResult{
		Errors: make([]error, 0, len(state.Identities)/10),
	}

	for uid, id := range state.Identities {
		if id.Email == "" {
			continue
		}

		if state.DryRun {
			result.IdentitiesUpdated++
			continue
		}

		exists, err := o.userExists(ctx, id.Email)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("check user %s (uid: %s): %w", id.Email, uid, err))
			continue
		}

		if !exists {
			if err := o.createUser(ctx, &id); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("create user %s (uid: %s): %w", id.Email, uid, err))
			} else {
				result.IdentitiesCreated++
			}
		} else {
			if err := o.updateUser(ctx, &id); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
			} else {
				result.IdentitiesUpdated++
			}
		}
	}

	return result, nil
}

func (o *IRedAdminOperator) userExists(ctx context.Context, email string) (bool, error) {
	var urlBuilder strings.Builder
	urlBuilder.Grow(len(o.baseURL) + len("/api/user/") + len(email) + 10)
	urlBuilder.WriteString(o.baseURL)
	urlBuilder.WriteString("/api/user/")
	urlBuilder.WriteString(url.PathEscape(email))

	resp, err := o.client.Get(urlBuilder.String())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	json.NewDecoder(resp.Body).Decode(&apiResp)

	return apiResp.Success, nil
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
			data.Set(k[10:], fmt.Sprintf("%v", v)) // "iredadmin:" is 10 bytes
		}
	}

	return o.execPost(ctx, urlBuilder.String(), data)
}

func (o *IRedAdminOperator) updateUser(ctx context.Context, id *source.Identity) error {
	data := url.Values{}
	hasChanges := false

	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "iredadmin:") {
			data.Set(k[10:], fmt.Sprintf("%v", v))
			hasChanges = true
		}
	}

	if !hasChanges {
		return nil
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

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return o.checkResponse(resp)
}

func (o *IRedAdminOperator) execPost(ctx context.Context, url string, data url.Values) error {
	resp, err := o.client.PostForm(url, data)
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
