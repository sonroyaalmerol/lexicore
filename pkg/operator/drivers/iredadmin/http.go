package iredadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
	"github.com/sethvargo/go-password/password"
)

func (o *IRedAdminOperator) createUser(
	ctx context.Context,
	id *source.Identity,
) error {
	res, err := password.Generate(64, 10, 10, false, false)
	if err != nil {
		return err
	}

	escaped := url.PathEscape(id.Email)
	var param = url.Values{}
	param.Set("name", id.DisplayName)
	param.Set("password", res)

	var paramForUpdate = url.Values{}
	hasUpdate := false
	hasMailingList := false

	for k, v := range id.Attributes {
		switch k {
		case AttributeLanguage:
			param.Set("language", fmt.Sprintf("%v", v))
		case AttributeQuota:
			param.Set("quota", fmt.Sprintf("%v", v))
		case AttributeGivenName:
			paramForUpdate.Set("gn", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeSN:
			paramForUpdate.Set("sn", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeStatus:
			paramForUpdate.Set("accountStatus", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeEnabledServices:
			paramForUpdate.Set("services", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeForwardingAddresses:
			paramForUpdate.Set("forwarding", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeAliases:
			paramForUpdate.Set("aliases", fmt.Sprintf("%v", v))
			hasUpdate = true
		case AttributeMailingLists:
			hasMailingList = true
		}
	}

	var payload = bytes.NewBufferString(param.Encode())
	path := fmt.Sprintf("%s/api/user/%s", o.baseURL, escaped)

	req, err := http.NewRequestWithContext(ctx, "POST", path, payload)
	if err != nil {
		return err
	}

	_, err = execHttp[any](o, ctx, req)
	if err != nil {
		return err
	}

	if hasUpdate {
		var updatePayload = bytes.NewBufferString(paramForUpdate.Encode())
		req, err = http.NewRequestWithContext(ctx, "PUT", path, updatePayload)
		if err != nil {
			return err
		}

		_, err = execHttp[any](o, ctx, req)
		if err != nil {
			return err
		}
	}

	if hasMailingList {
		mls, ok := id.Attributes[AttributeMailingLists].([]string)
		if ok {
			for _, ml := range mls {
				err := o.subscribeUserToList(ctx, ml, []string{id.Email})
				if err != nil {
					o.LogError(fmt.Errorf("failed to subscribe user to list: %v", err))
				}
			}
		}
	}

	return nil
}

func (o *IRedAdminOperator) updateUser(
	ctx context.Context,
	result *operator.SyncResult,
	newUser *UserData,
	current *UserData,
	dryRun bool,
) error {
	data := url.Values{}

	hasDiff := false
	hasDiffMl := false

	diff := make(map[string]string)
	defer func() {
		if len(diff) > 0 {
			result.RecordIdentityUpdateManual(newUser.UID[0], newUser.Mail[0], diff)
		}
	}()
	mail := getStringFromArray(current.Mail)

	newCN := getStringFromArray(newUser.CN)
	if getStringFromArray(current.CN) != newCN {
		hasDiff = true
		data.Set("name", newCN)
	}

	if !stringIsEqual(current.GivenName, newUser.GivenName) && len(newUser.GivenName) > 0 {
		hasDiff = true
		data.Set("givenName", strings.Join(newUser.GivenName, ","))
		diff["givenName"] = utils.DiffString(getStringFromArray(current.GivenName), getStringFromArray(newUser.GivenName))
	}
	if !stringIsEqual(current.SN, newUser.SN) && len(newUser.SN) > 0 {
		hasDiff = true
		data.Set("sn", strings.Join(newUser.SN, ","))
		diff["sn"] = utils.DiffString(getStringFromArray(current.SN), getStringFromArray(newUser.SN))
	}
	if !stringIsEqual(current.PreferredLanguage, newUser.PreferredLanguage) && len(newUser.PreferredLanguage) > 0 {
		hasDiff = true
		data.Set("language", strings.Join(newUser.PreferredLanguage, ","))
		diff["language"] = utils.DiffString(getStringFromArray(current.PreferredLanguage), getStringFromArray(newUser.PreferredLanguage))
	}
	if !stringIsEqual(current.MailQuota, newUser.MailQuota) && len(newUser.MailQuota) > 0 {
		hasDiff = true
		data.Set("quota", strings.Join(newUser.MailQuota, ","))
		diff["quota"] = utils.DiffString(getStringFromArray(current.MailQuota), getStringFromArray(newUser.MailQuota))
	}
	if !stringIsEqual(current.AccountStatus, newUser.AccountStatus) && len(newUser.AccountStatus) > 0 {
		hasDiff = true
		data.Set("accountStatus", strings.Join(newUser.AccountStatus, ","))
		diff["accountStatus"] = utils.DiffString(getStringFromArray(current.AccountStatus), getStringFromArray(newUser.AccountStatus))
	}
	if !utils.SlicesAreEqual(current.EnabledService, newUser.EnabledService) {
		hasDiff = true
		data.Set("services", strings.Join(newUser.EnabledService, ","))
		diff["services"] = utils.DiffArrString(current.EnabledService, newUser.EnabledService)
	}
	if !utils.SlicesAreEqual(current.MailingAliases, newUser.MailingAliases) {
		hasDiff = true
		data.Set("aliases", strings.Join(newUser.MailingAliases, ","))
		diff["aliases"] = utils.DiffArrString(current.MailingAliases, newUser.MailingAliases)
	}
	if !utils.SlicesAreEqual(current.MailForwardingAddress, newUser.MailForwardingAddress) {
		hasDiff = true
		data.Set("forwarding", strings.Join(newUser.MailForwardingAddress, ","))
		diff["forwarding"] = utils.DiffArrString(current.MailForwardingAddress, newUser.MailForwardingAddress)
	}
	if !utils.SlicesAreEqual(current.MailingLists, newUser.MailingLists) {
		hasDiffMl = true
		diff["mailingLists"] = utils.DiffArrString(current.MailingLists, newUser.MailingLists)
	}

	if !hasDiff && !hasDiffMl {
		return nil
	}

	escaped := url.PathEscape(mail)
	path := fmt.Sprintf("%s/api/user/%s", o.baseURL, escaped)

	if hasDiff {
		if !dryRun {
			req, err := http.NewRequestWithContext(ctx, "PUT", path, bytes.NewBufferString(data.Encode()))
			if err != nil {
				return err
			}

			_, err = execHttp[any](o, ctx, req)
			if err != nil {
				return err
			}
		} else {
			o.LogInfo("[DRY RUN] Would update user %s (uid: %s) | Encoded changes: %s",
				mail, getStringFromArray(current.UID), data.Encode())
		}
	}

	if !hasDiffMl {
		return nil
	}

	added, deleted := utils.StringArrDiff(current.MailingLists, newUser.MailingLists)
	for _, ml := range added {
		if dryRun {
			o.LogInfo("[DRY RUN] Would subscribe user %s (uid: %s) to %s",
				mail, getStringFromArray(current.UID), ml)
			continue
		}
		err := o.subscribeUserToList(ctx, ml, []string{mail})
		if err != nil {
			o.LogError(fmt.Errorf("failed to subscribe user to list: %v", err))
		}
	}

	for _, ml := range deleted {
		if dryRun {
			o.LogInfo("[DRY RUN] Would unsubscribe user %s (uid: %s) from %s",
				mail, getStringFromArray(current.UID), ml)
			continue
		}
		err := o.unsubscribeUserFromList(ctx, ml, []string{mail})
		if err != nil {
			o.LogError(fmt.Errorf("failed to unsubscribe user to list: %v", err))
		}
	}

	return nil
}

func (o *IRedAdminOperator) deleteUser(ctx context.Context, email string, days int) error {
	escaped := url.PathEscape(email)
	path := fmt.Sprintf("%s/api/user/%s/keep_mailbox_days/%d", o.baseURL, escaped, days)

	req, err := http.NewRequestWithContext(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}

	_, err = execHttp[any](o, ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (o *IRedAdminOperator) getUsers(ctx context.Context) ([]string, error) {
	escaped := url.PathEscape(o.domain)
	path := fmt.Sprintf("%s/api/users/%s?email_only=yes", o.baseURL, escaped)
	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := execHttp[[]string](o, ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

func (o *IRedAdminOperator) getUser(ctx context.Context, email string) (*UserData, error) {
	escaped := url.PathEscape(email)
	path := fmt.Sprintf("%s/api/user/%s", o.baseURL, escaped)
	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := execHttp[UserData](o, ctx, req)
	if err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

func (o *IRedAdminOperator) subscribeUserToList(ctx context.Context, mailingList string, users []string) error {
	escaped := url.PathEscape(mailingList)
	subs := strings.Join(users, ",")

	var param = url.Values{}
	param.Set("add_subscribers", subs)
	param.Set("require_confirm", "no")
	var payload = bytes.NewBufferString(param.Encode())

	path := fmt.Sprintf("%s/api/ml/%s", o.baseURL, escaped)
	req, err := http.NewRequestWithContext(ctx, "PUT", path, payload)
	if err != nil {
		return err
	}

	_, err = execHttp[any](o, ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (o *IRedAdminOperator) unsubscribeUserFromList(ctx context.Context, mailingList string, users []string) error {
	escaped := url.PathEscape(mailingList)
	subs := strings.Join(users, ",")

	var param = url.Values{}
	param.Set("remove_subscribers", subs)
	var payload = bytes.NewBufferString(param.Encode())

	path := fmt.Sprintf("%s/api/ml/%s", o.baseURL, escaped)
	req, err := http.NewRequestWithContext(ctx, "PUT", path, payload)
	if err != nil {
		return err
	}

	_, err = execHttp[any](o, ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func execHttp[T any](
	o *IRedAdminOperator,
	ctx context.Context,
	req *http.Request,
) (*APIResponse[T], error) {
	if err := o.GetLimiter().Wait(ctx); err != nil {
		return nil, err
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse[T]

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse iRedAdmin response: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("iredadmin error: %s", apiResp.Msg)
	}

	return &apiResp, nil
}
