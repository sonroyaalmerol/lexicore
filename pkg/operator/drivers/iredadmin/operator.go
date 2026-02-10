package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"golang.org/x/time/rate"
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
	limiter         *rate.Limiter
	domain          string
	deleteOnDisable bool
	deleteOnDelete  bool
	attrPrefix      *string
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

	rateLimit := 50.0
	if rl, ok := o.GetConfig("rateLimit"); ok {
		if rlFloat, ok := rl.(float64); ok && rlFloat > 0 {
			rateLimit = rlFloat
		}
	}

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

	o.limiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))

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

	if err := o.limiter.Wait(ctx); err != nil {
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

func (o *IRedAdminOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	currentUsers, err := o.getUsers(ctx)
	if err != nil {
		return nil, err
	}
	existingUsers := make(map[string]struct{})
	toDelete := make(map[string]struct{})
	for _, mail := range currentUsers {
		existingUsers[mail] = struct{}{}
		if o.deleteOnDelete {
			toDelete[mail] = struct{}{}
		}
	}

	workers := o.getConcurrency()
	result := &operator.SyncResult{}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for uid, id := range state.Identities {
		if id.Email == "" || id.Disabled {
			continue
		}

		if !o.deleteOnDelete {
			// toDelete is empty
			if o.deleteOnDisable && id.Disabled {
				toDelete[id.Email] = struct{}{}
			}
		} else {
			// toDelete contains all existing email users
			if !o.deleteOnDisable {
				delete(toDelete, id.Email)
			} else {
				if !id.Disabled {
					delete(toDelete, id.Email)
				} else {
					continue
				}
			}
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return result, ctx.Err()
		default:
		}

		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			enriched := o.EnrichIdentity(id, state.Groups)
			newUserData := o.identityToUser(enriched)

			o.LogInfo("checking user %s (uid: %s)", id.Email, uid)

			if _, exists := existingUsers[id.Email]; !exists {
				if state.DryRun {
					o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", id.Email, uid)
					result.IdentitiesCreated.Add(1)
				} else {
					if err := o.createUser(ctx, &enriched); err != nil {
						o.LogError(fmt.Errorf("create user %s (uid: %s): %w", id.Email, uid, err))
						result.ErrCount.Add(1)
					} else {
						result.IdentitiesCreated.Add(1)
					}
				}
			} else {
				userData, err := o.getUser(ctx, id.Email)
				if err != nil {
					o.LogError(fmt.Errorf("check user %s (uid: %s): %w", id.Email, uid, err))
					result.ErrCount.Add(1)
					return
				}

				if skipped, err := o.updateUser(ctx, &newUserData, userData, state.DryRun); err != nil {
					o.LogError(fmt.Errorf("update user %s (uid: %s): %w", id.Email, uid, err))
					result.ErrCount.Add(1)
				} else if !skipped {
					result.IdentitiesUpdated.Add(1)
				}
			}
		})
	}

	for email := range toDelete {
		select {
		case <-ctx.Done():
			wg.Wait()
			return result, ctx.Err()
		default:
		}

		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			if state.DryRun {
				o.LogInfo("[DRY RUN] Would delete user %s", email)
				result.IdentitiesDeleted.Add(1)
			} else {
				if err := o.deleteUser(ctx, email, o.keepMailboxDays); err != nil {
					o.LogError(fmt.Errorf("delete user %s: %w", email, err))
					result.ErrCount.Add(1)
				} else {
					result.IdentitiesDeleted.Add(1)
				}
			}
		})
	}
	wg.Wait()

	return result, nil
}

func (o *IRedAdminOperator) Close() error {
	return nil
}
