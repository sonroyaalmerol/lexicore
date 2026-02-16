package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

type APIResponse[T any] struct {
	Success bool   `json:"_success"`
	Msg     string `json:"_msg"`
	Data    T      `json:"_data"`
}

type UserData struct {
	Mail                  []string `json:"mail"`
	UID                   []string `json:"uid"`
	CN                    []string `json:"cn"`
	GivenName             []string `json:"givenName"`
	SN                    []string `json:"sn"`
	PreferredLanguage     []string `json:"preferredLanguage"`
	MailQuota             []string `json:"mailQuota"`
	AccountStatus         []string `json:"accountStatus"`
	Title                 []string `json:"title"`
	EnabledService        []string `json:"enabledService"`
	ManagedDomains        []string `json:"managed_domains"`
	MailForwardingAddress []string `json:"mailForwardingAddress"`
	MailingAliases        []string `json:"mailing_aliases"`
	MailingLists          []string `json:"mailing_lists"`
	StoredBytes           int64    `json:"stored_bytes"`
	StoredMessages        int      `json:"stored_messages"`
	LastLogin             string   `json:"last_login"`
	HomeDirectory         []string `json:"homeDirectory"`
	ShadowLastChange      []string `json:"shadowLastChange"`
}

type IRedAdminOperator struct {
	*operator.BaseOperator
	Client          *http.Client
	baseURL         string
	domain          string
	deleteOnDisable bool
	deleteOnDelete  bool
	keepMailboxDays int
}

func (o *IRedAdminOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")
	o.domain, _ = o.GetStringConfig("domain")

	jar, _ := cookiejar.New(nil)

	o.deleteOnDisable = false
	if rl, ok := o.GetConfig("deleteOnDisable"); ok {
		if isDelete, ok := rl.(bool); ok && isDelete {
			o.deleteOnDisable = isDelete
		}
	}

	o.deleteOnDelete = false
	if rl, ok := o.GetConfig("deleteOnDelete"); ok {
		if isDelete, ok := rl.(bool); ok && isDelete {
			o.deleteOnDelete = isDelete
		}
	}

	o.keepMailboxDays = 0
	if rl, ok := o.GetConfig("keepMailboxDays"); ok {
		if rlFloat, ok := rl.(int); ok && rlFloat > 0 {
			o.keepMailboxDays = rlFloat
		}
	}

	if o.Client == nil {
		o.Client = &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
		}
	}

	return nil
}

func (o *IRedAdminOperator) Validate(ctx context.Context) error {
	required := []string{"url", "username", "password", "domain"}
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

	if err := o.BaseOperator.GetLimiter().Wait(ctx); err != nil {
		return err
	}

	resp, err := o.Client.PostForm(fmt.Sprintf("%s/api/login", o.baseURL), data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp APIResponse[any]
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	if !apiResp.Success {
		return fmt.Errorf("iredadmin login failed: %s", apiResp.Msg)
	}
	return nil
}

func (o *IRedAdminOperator) Close() error {
	return nil
}
