package authentik

import (
	"context"
	"fmt"
	"strconv"

	"codeberg.org/lexicore/lexicore/pkg/source"
	authentik "goauthentik.io/api/v3"
)

func (s *AuthentikSource) GetIdentitiesByUIDs(ctx context.Context, uids []string) (map[string]source.Identity, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	_, childToParents, err := s.fetchGroups(ctx)
	if err != nil {
		return nil, err
	}

	identities := make(map[string]source.Identity, len(uids))

	for _, uid := range uids {
		pk, err := strconv.Atoi(uid)
		if err != nil {
			s.LogWarn("Invalid UID format: %s", uid)
			continue
		}

		user, _, err := client.CoreApi.CoreUsersRetrieve(ctx, int32(pk)).Execute()
		if err != nil {
			s.LogError(fmt.Errorf("failed to fetch user %s: %w", uid, err))
			continue
		}

		identities[uid] = s.mapUser(*user)
	}

	flattenIdentityGroups(identities, childToParents)

	return identities, nil
}

func (s *AuthentikSource) GetGroupsByGIDs(ctx context.Context, gids []string) (map[string]source.Group, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	groupCache := make(map[string]*authentik.Group, len(gids))

	var fetchGroup func(gid string) (*authentik.Group, error)
	fetchGroup = func(gid string) (*authentik.Group, error) {
		if g, ok := groupCache[gid]; ok {
			return g, nil
		}
		g, _, err := client.CoreApi.CoreGroupsRetrieve(ctx, gid).Execute()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch group %s: %w", gid, err)
		}
		groupCache[gid] = g
		return g, nil
	}

	var collectAncestors func(gid string, visited map[string]struct{}) ([]string, error)
	collectAncestors = func(gid string, visited map[string]struct{}) ([]string, error) {
		g, err := fetchGroup(gid)
		if err != nil {
			return nil, err
		}

		var ancestors []string
		for _, parentGID := range g.GetParents() {
			if _, seen := visited[parentGID]; seen {
				continue
			}
			visited[parentGID] = struct{}{}
			ancestors = append(ancestors, parentGID)

			deeper, err := collectAncestors(parentGID, visited)
			if err != nil {
				return nil, err
			}
			ancestors = append(ancestors, deeper...)
		}
		return ancestors, nil
	}

	for _, gid := range gids {
		if _, err := fetchGroup(gid); err != nil {
			s.LogError(err)
		}
	}

	groups := make(map[string]source.Group, len(gids))

	for _, gid := range gids {
		raw, ok := groupCache[gid]
		if !ok {
			continue
		}

		mapped := s.mapGroup(*raw)
		groups[gid] = mapped

		visited := map[string]struct{}{gid: {}}
		ancestors, err := collectAncestors(gid, visited)
		if err != nil {
			s.LogError(err)
			continue
		}

		for _, ancestorGID := range ancestors {
			ancestorRaw, err := fetchGroup(ancestorGID)
			if err != nil {
				s.LogError(err)
				continue
			}

			ancestor, exists := groups[ancestorGID]
			if !exists {
				ancestor = s.mapGroup(*ancestorRaw)
			}

			existing := make(map[string]struct{}, len(ancestor.Members))
			for _, m := range ancestor.Members {
				existing[m] = struct{}{}
			}

			changed := false
			for _, member := range mapped.Members {
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

	return groups, nil
}
