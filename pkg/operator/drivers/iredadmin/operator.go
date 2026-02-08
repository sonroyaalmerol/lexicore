package iredadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"golang.org/x/time/rate"
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
	limiter *rate.Limiter
}

func (o *IRedAdminOperator) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	if err := o.Validate(ctx); err != nil {
		return err
	}

	apiURL, _ := o.GetStringConfig("url")
	o.baseURL = strings.TrimSuffix(apiURL, "/")

	rateLimit := 50.0
	if rl, ok := o.GetConfig("rateLimit"); ok {
		if rlFloat, ok := rl.(float64); ok && rlFloat > 0 {
			rateLimit = rlFloat
		}
	}

	o.limiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))

	if o.Client == nil {
		o.Client = &http.Client{
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

	loginURL := o.buildAPIURL("/api/login")

	if err := o.limiter.Wait(ctx); err != nil {
		return err
	}

	resp, err := o.Client.PostForm(loginURL, data)
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

func (o *IRedAdminOperator) SupportsIncrementalSync() bool {
	return true
}

func (o *IRedAdminOperator) SyncIncremental(
	ctx context.Context,
	state *operator.IncrementalSyncState,
) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	workers := o.getConcurrency()
	result := &operator.SyncResult{}

	o.processCreates(ctx, state, result, workers)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	o.processUpdates(ctx, state, result, workers)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	o.processReprocesses(ctx, state, result, workers)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	o.processDeletes(ctx, state, result, workers)

	return result, nil
}

func (o *IRedAdminOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) (*operator.SyncResult, error) {
	if err := o.login(ctx); err != nil {
		return nil, err
	}

	workers := o.getConcurrency()
	result := &operator.SyncResult{}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for uid, id := range state.Identities {
		if id.Email == "" || id.Disabled {
			continue
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			return result, ctx.Err()
		default:
		}

		wg.Add(1)
		go func(userID string, identity source.Identity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			enriched := o.EnrichIdentity(identity, state.Groups)

			userData, err := o.getUserData(ctx, identity.Email)
			if err != nil {
				o.LogError(fmt.Errorf("check user %s (uid: %s): %w", identity.Email, userID, err))
				result.ErrCount.Add(1)
				return
			}

			if userData == nil {
				if state.DryRun {
					o.LogInfo("[DRY RUN] Would create user %s (uid: %s)", identity.Email, userID)
					result.IdentitiesCreated.Add(1)
				} else {
					if err := o.createUser(ctx, &enriched); err != nil {
						o.LogError(fmt.Errorf("create user %s (uid: %s): %w", identity.Email, userID, err))
						result.ErrCount.Add(1)
					} else {
						result.IdentitiesCreated.Add(1)
					}
				}
			} else {
				changes := o.detectChanges(&enriched, userData)
				if len(changes) > 0 {
					if state.DryRun {
						o.LogInfo("[DRY RUN] Would update user %s (uid: %s) - changes: %v",
							identity.Email, userID, changes)
						result.IdentitiesUpdated.Add(1)
					} else {
						if err := o.updateUser(ctx, &enriched, changes); err != nil {
							o.LogError(fmt.Errorf("update user %s (uid: %s): %w", identity.Email, userID, err))
							result.ErrCount.Add(1)
						} else {
							result.IdentitiesUpdated.Add(1)
						}
					}
				}
			}
		}(uid, id)
	}

	wg.Wait()

	return result, nil
}

func (o *IRedAdminOperator) getUserData(
	ctx context.Context,
	email string,
) (*UserData, error) {
	userURL := o.buildUserURL(email)

	if err := o.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	resp, err := o.Client.Get(userURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp struct {
		Success bool      `json:"_success"`
		Msg     string    `json:"_msg"`
		Data    *UserData `json:"_data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse user data response: %w", err)
	}

	if !apiResp.Success {
		return nil, nil
	}

	return apiResp.Data, nil
}

func (o *IRedAdminOperator) detectChanges(
	id *source.Identity,
	userData *UserData,
) map[string]string {
	changes := make(map[string]string)

	cn := id.DisplayName
	if cn == "" {
		cn = id.Username
	}
	if len(userData.CN) > 0 && userData.CN[0] != cn {
		changes["cn"] = cn
	}

	for k, v := range id.Attributes {
		fieldName, hasPrefix := strings.CutPrefix(k, o.GetAttributePrefix())
		if !hasPrefix {
			continue
		}

		newValue := fmt.Sprintf("%v", v)
		existingValue := o.getFieldValue(userData, fieldName)

		if existingValue != newValue {
			changes[fieldName] = newValue
		}
	}

	return changes
}

func (o *IRedAdminOperator) Close() error {
	return nil
}
