package authentik

import (
	"context"
	"fmt"
	"strconv"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (s *AuthentikSource) GetIdentitiesByUIDs(ctx context.Context, uids []string) (map[string]source.Identity, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	identities := make(map[string]source.Identity, len(uids))

	for _, uid := range uids {
		pk, err := strconv.ParseInt(uid, 10, 32)
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

	return identities, nil
}

func (s *AuthentikSource) GetGroupsByGIDs(ctx context.Context, gids []string) (map[string]source.Group, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	groups := make(map[string]source.Group, len(gids))

	for _, gid := range gids {
		group, _, err := client.CoreApi.CoreGroupsRetrieve(ctx, gid).Execute()
		if err != nil {
			s.LogError(fmt.Errorf("failed to fetch group %s: %w", gid, err))
			continue
		}

		groups[gid] = s.mapGroup(*group)
	}

	return groups, nil
}
