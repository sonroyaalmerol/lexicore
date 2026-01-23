package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
)

// DavACL XML mapping for RFC 3744
type DavACL struct {
	XMLName xml.Name `xml:"DAV: acl"`
	ACE     []DavACE `xml:"ace"`
}

type DavACE struct {
	Principal DavPrincipal `xml:"principal"`
	Grant     DavGrant     `xml:"grant"`
}

type DavPrincipal struct {
	Href          string    `xml:"href,omitempty"`
	Authenticated *struct{} `xml:"authenticated,omitempty"`
	All           *struct{} `xml:"all,omitempty"`
}

type DavGrant struct {
	Privileges []DavPrivilege `xml:"privilege"`
}

type DavPrivilege struct {
	// Standard RFC 3744 granular privileges
	Read            *struct{} `xml:"read,omitempty"`
	WriteContent    *struct{} `xml:"write-content,omitempty"`
	WriteProperties *struct{} `xml:"write-properties,omitempty"`
	Bind            *struct{} `xml:"bind,omitempty"`   // Create
	Unbind          *struct{} `xml:"unbind,omitempty"` // Delete
	All             *struct{} `xml:"all,omitempty"`
}

type CalDAVOperator struct {
	*operator.BaseOperator
	client *http.Client
}

func init() {
	operator.Register("caldav-acl", func() operator.Operator {
		return &CalDAVOperator{
			BaseOperator: operator.NewBaseOperator("caldav-acl"),
			client:       &http.Client{Timeout: 30 * time.Second},
		}
	})
}

func (o *CalDAVOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	return o.Validate(ctx)
}

func (o *CalDAVOperator) Sync(ctx context.Context, state *operator.SyncState) (*operator.SyncResult, error) {
	result := &operator.SyncResult{}
	baseURL, _ := o.GetStringConfig("url")

	for _, identity := range state.Identities {
		if identity.Email == "" {
			continue
		}

		folder := "Calendar/personal"
		if f, ok := identity.Attributes["caldav_folder"].(string); ok && f != "" {
			folder = f
		}

		targetURL := fmt.Sprintf("%s/dav/%s/%s/",
			strings.TrimSuffix(baseURL, "/"), identity.Email, strings.Trim(folder, "/"))

		if state.DryRun {
			result.IdentitiesUpdated++
			continue
		}

		aclSpec, ok := identity.Attributes["caldav_acl"].(string)
		if !ok || aclSpec == "" {
			continue
		}

		if err := o.applyACL(ctx, targetURL, aclSpec); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed ACL for %s: %w", identity.Email, err))
		} else {
			result.IdentitiesUpdated++
		}
	}
	return result, nil
}

func (o *CalDAVOperator) applyACL(ctx context.Context, url, spec string) error {
	parts := strings.Split(spec, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid spec format (grantee:p1,p2)")
	}
	grantee, privsRaw := parts[0], parts[1]

	p := DavPrincipal{}
	switch grantee {
	case "authenticated":
		p.Authenticated = &struct{}{}
	case "all":
		p.All = &struct{}{}
	default:
		p.Href = fmt.Sprintf("/SOGo/dav/%s/", grantee)
	}

	var privList []DavPrivilege
	for _, pName := range strings.Split(privsRaw, ",") {
		dp := DavPrivilege{}
		switch strings.TrimSpace(pName) {
		case "read":
			dp.Read = &struct{}{}
		case "write-content":
			dp.WriteContent = &struct{}{}
		case "write-properties":
			dp.WriteProperties = &struct{}{}
		case "bind":
			dp.Bind = &struct{}{}
		case "unbind":
			dp.Unbind = &struct{}{}
		case "all":
			dp.All = &struct{}{}
		}
		privList = append(privList, dp)
	}

	acl := DavACL{
		ACE: []DavACE{{
			Principal: p,
			Grant:     DavGrant{Privileges: privList},
		}},
	}

	body, _ := xml.Marshal(acl)
	req, err := http.NewRequestWithContext(ctx, "ACL", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/xml")
	user, _ := o.GetStringConfig("username")
	pass, _ := o.GetStringConfig("password")
	req.SetBasicAuth(user, pass)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("CalDAV ACL Error %d", resp.StatusCode)
	}
	return nil
}

func (o *CalDAVOperator) Validate(ctx context.Context) error { return nil }
func (o *CalDAVOperator) Close() error                       { return nil }
