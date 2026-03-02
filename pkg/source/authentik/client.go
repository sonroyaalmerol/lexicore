package authentik

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
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

func (o *AuthentikSource) Initialize(ctx context.Context, config map[string]manifest.ConfigValue) error {
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
		switch v := pageSizeRaw.Value().(type) {
		case int:
			pageSize = int32(v)
		case int32:
			pageSize = v
		case int64:
			pageSize = int32(v)
		case float64:
			pageSize = int32(v)
		default:
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

func (s *AuthentikSource) fetchGroups(ctx context.Context) (map[string]source.Group, map[string][]string, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	groups := make(map[string]source.Group)
	childToParents := make(map[string][]string)
	page := int32(1)

	for {
		req := client.CoreApi.CoreGroupsList(ctx).Page(page)
		if config.PageSize > 0 {
			req = req.PageSize(config.PageSize)
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch groups: %w", err)
		}

		for _, grp := range resp.Results {
			groups[grp.Pk] = s.mapGroup(grp)
			parents := grp.GetParents()
			if len(parents) > 0 {
				childToParents[grp.Pk] = parents
			}
		}

		if resp.Pagination.Next <= 0 {
			break
		}
		page = int32(resp.Pagination.Next)
	}

	return groups, childToParents, nil
}

func (s *AuthentikSource) GetGroups(ctx context.Context) (map[string]source.Group, error) {
	groups, childToParents, err := s.fetchGroups(ctx)
	if err != nil {
		return nil, err
	}

	flattenGroupMembers(groups, childToParents)

	return groups, nil
}

func (s *AuthentikSource) GetIdentities(ctx context.Context) (map[string]source.Identity, error) {
	s.mu.Lock()
	client := s.client
	config := s.config
	s.mu.Unlock()

	_, childToParents, err := s.fetchGroups(ctx)
	if err != nil {
		return nil, err
	}

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

	flattenIdentityGroups(identities, childToParents)

	return identities, nil
}

func flattenGroupMembers(
	groups map[string]source.Group,
	childToParents map[string][]string,
) {
	var allAncestors func(gid string, visited map[string]struct{}) []string
	allAncestors = func(gid string, visited map[string]struct{}) []string {
		var result []string
		for _, parentGID := range childToParents[gid] {
			if _, seen := visited[parentGID]; seen {
				continue
			}
			visited[parentGID] = struct{}{}
			result = append(result, parentGID)
			result = append(result, allAncestors(parentGID, visited)...)
		}
		return result
	}

	for gid, grp := range groups {
		ancestors := allAncestors(gid, map[string]struct{}{gid: {}})

		for _, ancestorGID := range ancestors {
			ancestor, ok := groups[ancestorGID]
			if !ok {
				continue
			}

			existing := make(map[string]struct{}, len(ancestor.Members))
			for _, m := range ancestor.Members {
				existing[m] = struct{}{}
			}

			changed := false
			for _, member := range grp.Members {
				if _, dup := existing[member]; !dup {
					ancestor.Members = append(ancestor.Members, member)
					existing[member] = struct{}{}
					changed = true
				}
			}

			if changed {
				groups[ancestorGID] = ancestor
			}
		}
	}
}

func flattenIdentityGroups(
	identities map[string]source.Identity,
	childToParents map[string][]string,
) {
	for uid, identity := range identities {
		allGroups := make(map[string]struct{})
		queue := append([]string{}, identity.Groups...)

		for len(queue) > 0 {
			gid := queue[0]
			queue = queue[1:]
			if _, seen := allGroups[gid]; seen {
				continue
			}
			allGroups[gid] = struct{}{}
			queue = append(queue, childToParents[gid]...)
		}

		flat := make([]string, 0, len(allGroups))
		for gid := range allGroups {
			flat = append(flat, gid)
		}
		identity.Groups = flat
		identities[uid] = identity
	}
}

func (s *AuthentikSource) convertToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int32:
		return strconv.Itoa(int(val))
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatInt(int64(val), 10)
	default:
		return ""
	}
}

func (s *AuthentikSource) mapUser(u authentik.User) source.Identity {
	return source.Identity{
		UID:         strconv.Itoa(int(u.Pk)),
		Username:    u.GetUsername(),
		Email:       u.GetEmail(),
		DisplayName: u.GetName(),
		Groups:      u.GetGroups(),
		Attributes:  u.GetAttributes(),
		Disabled:    !u.GetIsActive(),
	}
}

func (s *AuthentikSource) mapGroup(g authentik.Group) source.Group {
	members := make([]string, len(g.Users))
	for i, userPK := range g.Users {
		members[i] = strconv.Itoa(int(userPK))
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
