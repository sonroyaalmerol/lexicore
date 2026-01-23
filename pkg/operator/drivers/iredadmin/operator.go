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
	client *http.Client
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
	return o.Validate(ctx)
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
	apiURL, _ := o.GetStringConfig("url")
	user, _ := o.GetStringConfig("username")
	pass, _ := o.GetStringConfig("password")

	data := url.Values{}
	data.Set("username", user)
	data.Set("password", pass)

	reqURL := fmt.Sprintf("%s/api/login", strings.TrimSuffix(apiURL, "/"))
	resp, err := o.client.PostForm(reqURL, data)
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

	result := &operator.SyncResult{}
	apiURL, _ := o.GetStringConfig("url")

	for _, id := range state.Identities {
		if id.Email == "" {
			continue
		}

		if state.DryRun {
			result.IdentitiesUpdated++
			continue
		}

		exists, err := o.userExists(ctx, apiURL, id.Email)
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		if !exists {
			if err := o.createUser(ctx, apiURL, id); err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.IdentitiesCreated++
			}
		} else {
			if err := o.updateUser(ctx, apiURL, id); err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.IdentitiesUpdated++
			}
		}
	}

	return result, nil
}

func (o *IRedAdminOperator) userExists(ctx context.Context, apiURL, email string) (bool, error) {
	reqURL := fmt.Sprintf("%s/api/user/%s", strings.TrimSuffix(apiURL, "/"), url.PathEscape(email))
	resp, err := o.client.Get(reqURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	json.NewDecoder(resp.Body).Decode(&apiResp)

	return apiResp.Success, nil
}

func (o *IRedAdminOperator) createUser(ctx context.Context, apiURL string, id source.Identity) error {
	reqURL := fmt.Sprintf("%s/api/user/%s", strings.TrimSuffix(apiURL, "/"), url.PathEscape(id.Email))

	data := url.Values{}
	data.Set("cn", id.DisplayName)
	if id.DisplayName == "" {
		data.Set("cn", id.Username)
	}

	data.Set("password", "ChangeMe123!")
	if p, ok := id.Attributes["mailPassword"].(string); ok {
		data.Set("password", p)
	}

	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "iredadmin:") {
			data.Set(strings.TrimPrefix(k, "iredadmin:"), fmt.Sprintf("%v", v))
		}
	}

	return o.execPost(ctx, reqURL, data)
}

func (o *IRedAdminOperator) updateUser(ctx context.Context, apiURL string, id source.Identity) error {
	reqURL := fmt.Sprintf("%s/api/user/%s", strings.TrimSuffix(apiURL, "/"), url.PathEscape(id.Email))

	data := url.Values{}
	hasChanges := false
	for k, v := range id.Attributes {
		if strings.HasPrefix(k, "iredadmin:") {
			data.Set(strings.TrimPrefix(k, "iredadmin:"), fmt.Sprintf("%v", v))
			hasChanges = true
		}
	}

	if !hasChanges {
		return nil
	}

	req, _ := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(data.Encode()))
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
		return fmt.Errorf("failed to parse iRedAdmin response: %s", string(body))
	}

	if !apiResp.Success {
		return fmt.Errorf("iredadmin error: %s", apiResp.Msg)
	}
	return nil
}

func (o *IRedAdminOperator) Close() error { return nil }
