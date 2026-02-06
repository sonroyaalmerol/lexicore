package authentik

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	authentik "goauthentik.io/api/v3"
)

type Config struct {
	URL      string
	Token    string
	PageSize int32
}

type AuthentikSource struct {
	*source.BaseSource

	mu     sync.Mutex
	config *Config
	client *authentik.APIClient
}

func (o *AuthentikSource) Initialize(ctx context.Context, config map[string]any) error {
	o.SetConfig(config)
	return o.Validate(ctx)
}

func (o *AuthentikSource) Validate(ctx context.Context) error {
	url, err := o.GetStringConfig("url")
	if err != nil {
		return err
	}

	token, err := o.GetStringConfig("token")
	if err != nil {
		return err
	}

	pageSize := int32(100)
	pageSizeRaw, ok := o.GetConfig("pageSize")
	if ok {
		if pageSize, ok = pageSizeRaw.(int32); !ok {
			pageSize = int32(100)
		}
	}

	o.mu.Lock()
	o.config = &Config{
		URL:      url,
		Token:    token,
		PageSize: pageSize,
	}

	apiConfig := authentik.NewConfiguration()
	apiConfig.Servers = authentik.ServerConfigurations{
		{
			URL: url,
		},
	}
	apiConfig.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", token))

	o.client = authentik.NewAPIClient(apiConfig)
	o.mu.Unlock()
	return nil
}

func (s *AuthentikSource) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _, err := s.client.CoreApi.CoreUsersMeRetrieve(ctx).Execute()
	if err != nil {
		return fmt.Errorf("failed to connect to authentik: %w", err)
	}
	return nil
}

func (s *AuthentikSource) GetIdentities(ctx context.Context) (map[string]source.Identity, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	identities := make(map[string]source.Identity)
	page := int32(1)

	for {
		req := client.CoreApi.CoreUsersList(ctx).Page(page)
		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch users: %w", err)
		}

		for _, user := range resp.Results {
			identities[strconv.Itoa(int(user.Pk))] = s.mapUser(user)
		}

		if resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	return identities, nil
}

func (s *AuthentikSource) GetGroups(ctx context.Context) (map[string]source.Group, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	groups := make(map[string]source.Group)
	page := int32(1)

	for {
		req := client.CoreApi.CoreGroupsList(ctx).Page(page)
		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch groups: %w", err)
		}

		for _, grp := range resp.Results {
			groups[grp.Pk] = s.mapGroup(grp)
		}

		if resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	return groups, nil
}

func (s *AuthentikSource) Watch(ctx context.Context) (<-chan source.Event, error) {
	events := make(chan source.Event)

	go func() {
		defer close(events)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				users, _ := s.GetIdentities(ctx)
				for _, u := range users {
					events <- source.Event{
						Type:      source.EventUpdate,
						Identity:  &u,
						Timestamp: time.Now().Unix(),
					}
				}
			}
		}
	}()

	return events, nil
}

func (s *AuthentikSource) mapUser(u authentik.User) source.Identity {
	email := ""
	if u.Email != nil {
		email = *u.Email
	}

	return source.Identity{
		UID:         strconv.Itoa(int(u.Pk)),
		Username:    u.Username,
		Email:       email,
		DisplayName: u.Name,
		Groups:      u.Groups,
		Attributes:  u.Attributes,
		Disabled:    !u.GetIsActive(),
	}
}

func (s *AuthentikSource) mapGroup(g authentik.Group) source.Group {
	members := make([]string, len(g.Users))
	for i, u := range g.Users {
		members[i] = fmt.Sprintf("%v", u)
	}

	description := ""
	if d, ok := g.Attributes["description"].(string); ok {
		description = d
	}

	return source.Group{
		GID:         g.Pk,
		Name:        g.Name,
		Members:     members,
		Description: description,
		Attributes:  g.Attributes,
	}
}

func (s *AuthentikSource) Close() error {
	return nil
}
