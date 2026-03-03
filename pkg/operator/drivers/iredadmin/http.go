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
)

type attributeConfig struct {
	apiParam      string
	onUpdate      bool
	userDataField func(UserData) []string
	isSlice       bool
}

var attributeConfigs = map[string]attributeConfig{
	AttributeLanguage: {
		apiParam:      "language",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.PreferredLanguage },
		isSlice:       false,
	},
	AttributeQuota: {
		apiParam:      "quota",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.MailQuota },
		isSlice:       false,
	},
	AttributeGivenName: {
		apiParam:      "givenName",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.GivenName },
		isSlice:       false,
	},
	AttributeSN: {
		apiParam:      "sn",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.SN },
		isSlice:       false,
	},
	AttributeStatus: {
		apiParam:      "accountStatus",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.AccountStatus },
		isSlice:       false,
	},
	AttributeEnabledServices: {
		apiParam:      "services",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.EnabledService },
		isSlice:       true,
	},
	AttributeForwardingAddresses: {
		apiParam:      "forwarding",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.MailForwardingAddress },
		isSlice:       true,
	},
	AttributeAliases: {
		apiParam:      "aliases",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.MailingAliases },
		isSlice:       true,
	},
	AttributeCN: {
		apiParam:      "name",
		onUpdate:      true,
		userDataField: func(u UserData) []string { return u.CN },
		isSlice:       false,
	},
}

func formatAttrValue(attrVal any) string {
	switch v := attrVal.(type) {
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = utils.AppendUnique(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", attrVal)
	}
}

func (o *IRedAdminOperator) updateUser(
	ctx context.Context,
	id source.Identity,
	result *operator.SyncResult,
	newUser UserData,
	current UserData,
	dryRun bool,
) error {
	if len(newUser.UID) == 0 || len(newUser.Mail) == 0 {
		return fmt.Errorf("invalid user data: missing UID or Mail")
	}

	data := url.Values{}
	var changes []operator.Change
	mail := getStringFromArray(current.Mail)

	for attrKey, config := range attributeConfigs {
		if !config.onUpdate {
			continue
		}

		currentVal := config.userDataField(current)
		newVal := config.userDataField(newUser)

		if len(newVal) == 0 {
			continue
		}

		var isDifferent bool
		if config.isSlice {
			isDifferent = !utils.SlicesAreEqual(currentVal, newVal)
		} else {
			isDifferent = !stringIsEqual(currentVal, newVal)
		}

		if isDifferent {
			data.Set(config.apiParam, strings.Join(newVal, ","))
			changes = append(changes, operator.AttrChange(
				attrKey,
				utils.SliceToSortedString(currentVal, ","),
				utils.SliceToSortedString(newVal, ","),
			))
		}
	}

	addedLists, deletedLists := utils.StringArrDiff(current.MailingLists, newUser.MailingLists)

	for _, ml := range addedLists {
		changes = append(changes, operator.MembershipAdded(ml))
	}
	for _, ml := range deletedLists {
		changes = append(changes, operator.MembershipRemoved(ml))
	}

	if len(changes) == 0 {
		return nil
	}

	path := fmt.Sprintf("%s/api/user/%s", o.baseURL, url.PathEscape(mail))

	if len(data) > 0 {
		if dryRun {
			o.LogInfo("[DRY RUN] Would update user %s (uid: %s)", mail, getStringFromArray(current.UID))
		} else {
			if err := o.doRequest(ctx, "PUT", path, data); err != nil {
				return err
			}
		}
	}

	if !dryRun {
		for _, ml := range addedLists {
			if err := o.subscribeToMailingList(ctx, ml, []string{mail}); err != nil {
				o.LogError(fmt.Errorf("failed to subscribe user to list %s: %v", ml, err))
			}
		}
		for _, ml := range deletedLists {
			if err := o.unsubscribeFromMailingList(ctx, ml, []string{mail}); err != nil {
				o.LogError(fmt.Errorf("failed to unsubscribe user from list %s: %v", ml, err))
			}
		}
	}

	result.Record(operator.ActionUpdate, newUser.UID[0], newUser.Mail[0], changes...)
	return nil
}

func (o *IRedAdminOperator) getUsers(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/api/users/%s?email_only=yes", o.baseURL, url.PathEscape(o.domain))
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

func (o *IRedAdminOperator) getUser(ctx context.Context, email string) (UserData, error) {
	path := fmt.Sprintf("%s/api/user/%s", o.baseURL, url.PathEscape(email))
	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return UserData{}, err
	}

	resp, err := execHttp[UserData](o, ctx, req)
	if err != nil {
		return UserData{}, err
	}

	return resp.Data, nil
}

func (o *IRedAdminOperator) disableUser(
	ctx context.Context,
	uid string,
	id source.Identity,
	dryRun bool,
	result *operator.SyncResult,
) error {
	userData, err := o.getUser(ctx, id.Email)
	if err != nil {
		return fmt.Errorf("failed to get user data: %w", err)
	}

	currentStatus := getStringFromArray(userData.AccountStatus)
	if currentStatus == "disabled" {
		return nil
	}

	if dryRun {
		o.LogInfo("[DRY RUN] Would disable user %s (uid: %s)", id.Email, uid)
	} else {
		data := url.Values{}
		data.Set("accountStatus", "disabled")

		path := fmt.Sprintf("%s/api/user/%s", o.baseURL, url.PathEscape(id.Email))
		if err := o.doRequest(ctx, "PUT", path, data); err != nil {
			return err
		}
	}

	result.Record(
		operator.ActionUpdate, uid, id.Username,
		operator.AttrChange(AttributeStatus, currentStatus, "disabled"),
	)
	return nil
}

func (o *IRedAdminOperator) subscribeToMailingList(ctx context.Context, mailingList string, users []string) error {
	params := url.Values{}
	params.Set("add_subscribers", strings.Join(users, ","))
	params.Set("require_confirm", "no")

	path := fmt.Sprintf("%s/api/ml/%s", o.baseURL, url.PathEscape(mailingList))
	return o.doRequest(ctx, "PUT", path, params)
}

func (o *IRedAdminOperator) unsubscribeFromMailingList(ctx context.Context, mailingList string, users []string) error {
	params := url.Values{}
	params.Set("remove_subscribers", strings.Join(users, ","))

	path := fmt.Sprintf("%s/api/ml/%s", o.baseURL, url.PathEscape(mailingList))
	return o.doRequest(ctx, "PUT", path, params)
}

func (o *IRedAdminOperator) doRequest(ctx context.Context, method, path string, params url.Values) error {
	var body *bytes.Buffer
	if params != nil {
		body = bytes.NewBufferString(params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return err
	}

	if params != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	_, err = execHttp[any](o, ctx, req)
	return err
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
