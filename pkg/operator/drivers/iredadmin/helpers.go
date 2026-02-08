package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (o *IRedAdminOperator) getFieldValue(
	userData *UserData,
	fieldName string,
) string {
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

func (o *IRedAdminOperator) execPost(
	ctx context.Context,
	url string,
	data url.Values,
) error {
	if err := o.limiter.Wait(ctx); err != nil {
		return err
	}

	resp, err := o.Client.PostForm(url, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return o.checkResponse(resp)
}

func (o *IRedAdminOperator) checkResponse(resp *http.Response) error {
	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to parse iRedAdmin response: %s", body)
	}

	if !apiResp.Success {
		return fmt.Errorf("iredadmin error: %s", apiResp.Msg)
	}
	return nil
}

func (o *IRedAdminOperator) buildAPIURL(path string) string {
	var b strings.Builder
	b.Grow(len(o.baseURL) + len(path))
	b.WriteString(o.baseURL)
	b.WriteString(path)
	return b.String()
}

func (o *IRedAdminOperator) buildUserURL(email string) string {
	escaped := url.PathEscape(email)
	var b strings.Builder
	b.Grow(len(o.baseURL) + 11 + len(escaped))
	b.WriteString(o.baseURL)
	b.WriteString("/api/user/")
	b.WriteString(escaped)
	return b.String()
}

func (o *IRedAdminOperator) getConcurrency() int {
	workers := 10
	if w, ok := o.GetConfig("concurrency"); ok {
		if wInt, ok := w.(int); ok && wInt > 0 {
			workers = wInt
		} else if wFloat, ok := w.(float64); ok && wFloat > 0 {
			workers = int(wFloat)
		}
	}
	return workers
}
