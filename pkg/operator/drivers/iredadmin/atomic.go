package iredadmin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (o *IRedAdminOperator) createUser(
	ctx context.Context,
	id *source.Identity,
) error {
	userURL := o.buildUserURL(id.Email)

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
		fieldName, hasPrefix := strings.CutPrefix(k, o.GetAttributePrefix())
		if !hasPrefix {
			continue
		}
		data.Set(fieldName, fmt.Sprintf("%v", v))
	}

	return o.execPost(ctx, userURL, data)
}

func (o *IRedAdminOperator) updateUser(
	ctx context.Context,
	id *source.Identity,
	changes map[string]string,
) error {
	if len(changes) == 0 {
		return nil
	}

	data := url.Values{}
	for k, v := range changes {
		data.Set(k, v)
	}

	userURL := o.buildUserURL(id.Email)

	if err := o.limiter.Wait(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"PUT",
		userURL,
		strings.NewReader(data.Encode()),
	)
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

func (o *IRedAdminOperator) deleteUser(ctx context.Context, email string) error {
	userURL := o.buildUserURL(email)

	if err := o.limiter.Wait(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", userURL, nil)
	if err != nil {
		return err
	}

	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return o.checkResponse(resp)
}
