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

func (s *AuthentikSource) SupportsChangeDetection() bool {
	return true
}

func (s *AuthentikSource) GetChangesSince(ctx context.Context, since time.Time) (*source.Changes, time.Time, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	now := time.Now()
	changes := &source.Changes{
		ModifiedIdentities: make([]source.Identity, 0),
		DeletedIdentities:  make([]string, 0),
		ModifiedGroups:     make([]source.Group, 0),
		DeletedGroups:      make([]string, 0),
		FullSync:           false,
	}

	userPage := int32(1)
	for {
		req := client.CoreApi.CoreUsersList(ctx).
			Page(userPage).
			LastUpdatedGt(since)

		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			changes.FullSync = true
			return changes, now, nil
		}

		for _, user := range resp.Results {
			changes.ModifiedIdentities = append(
				changes.ModifiedIdentities,
				s.mapUser(user),
			)
		}

		if resp.Pagination.Next <= 0 {
			break
		}
		userPage = int32(resp.Pagination.Next)
	}

	modifiedGroupIDs, err := s.fetchModifiedGroupsSince(ctx, since)
	if err != nil {
		changes.FullSync = true
		return changes, now, nil
	}

	for groupID := range modifiedGroupIDs {
		group, _, err := client.CoreApi.CoreGroupsRetrieve(ctx, groupID).Execute()
		if err != nil {
			// Group might have been deleted, skip it
			continue
		}
		changes.ModifiedGroups = append(changes.ModifiedGroups, s.mapGroup(*group))
	}

	deletedUsers, deletedGroups, err := s.fetchDeletionsSince(ctx, since)
	if err != nil {
		changes.FullSync = true
		return changes, now, nil
	}

	changes.DeletedIdentities = deletedUsers
	changes.DeletedGroups = deletedGroups

	return changes, now, nil
}

func (s *AuthentikSource) fetchModifiedGroupsSince(
	ctx context.Context,
	since time.Time,
) (map[string]bool, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	modifiedGroupIDs := make(map[string]bool)
	page := int32(1)

	for {
		req := client.EventsApi.EventsEventsList(ctx).
			Page(page).
			Ordering("-created").
			ContextModelName("group")

		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch group events: %w", err)
		}

		foundOlderEvent := false
		for _, event := range resp.Results {
			if event.Created.Before(since) {
				foundOlderEvent = true
				break
			}

			// Look for model_created and model_updated events
			if event.Action == "model_created" || event.Action == "model_updated" {
				if modelRaw, ok := event.Context["model"]; ok {
					if modelMap, ok := modelRaw.(map[string]any); ok {
						if pkRaw, ok := modelMap["pk"]; ok {
							if groupID, ok := pkRaw.(string); ok {
								modifiedGroupIDs[groupID] = true
							}
						}
					}
				}
			}
		}

		if foundOlderEvent || resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	return modifiedGroupIDs, nil
}

func (s *AuthentikSource) fetchDeletionsSince(
	ctx context.Context,
	since time.Time,
) ([]string, []string, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	deletedUserIDs := make(map[string]bool)
	deletedGroupIDs := make(map[string]bool)

	page := int32(1)

	for {
		req := client.EventsApi.EventsEventsList(ctx).
			Page(page).
			Ordering("-created").
			Action("model_deleted")

		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		req = req.ContextModelName("user")

		resp, _, err := req.Execute()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch user deletion events: %w", err)
		}

		foundOlderEvent := false
		for _, event := range resp.Results {
			if event.Created.Before(since) {
				foundOlderEvent = true
				break
			}

			if modelRaw, ok := event.Context["model"]; ok {
				if modelMap, ok := modelRaw.(map[string]any); ok {
					if pkRaw, ok := modelMap["pk"]; ok {
						var userID string
						switch v := pkRaw.(type) {
						case float64:
							userID = strconv.Itoa(int(v))
						case int:
							userID = strconv.Itoa(v)
						case int32:
							userID = strconv.Itoa(int(v))
						case int64:
							userID = strconv.Itoa(int(v))
						case string:
							userID = v
						default:
							continue
						}
						deletedUserIDs[userID] = true
					}
				}
			}
		}

		if foundOlderEvent || resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	page = int32(1)
	for {
		req := client.EventsApi.EventsEventsList(ctx).
			Page(page).
			Ordering("-created").
			Action("model_deleted").
			ContextModelName("group")

		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch group deletion events: %w", err)
		}

		foundOlderEvent := false
		for _, event := range resp.Results {
			if event.Created.Before(since) {
				foundOlderEvent = true
				break
			}

			if modelRaw, ok := event.Context["model"]; ok {
				if modelMap, ok := modelRaw.(map[string]any); ok {
					if pkRaw, ok := modelMap["pk"]; ok {
						if groupID, ok := pkRaw.(string); ok {
							deletedGroupIDs[groupID] = true
						}
					}
				}
			}
		}

		if foundOlderEvent || resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	deletedUsers := make([]string, 0, len(deletedUserIDs))
	for userID := range deletedUserIDs {
		deletedUsers = append(deletedUsers, userID)
	}

	deletedGroups := make([]string, 0, len(deletedGroupIDs))
	for groupID := range deletedGroupIDs {
		deletedGroups = append(deletedGroups, groupID)
	}

	return deletedUsers, deletedGroups, nil
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
