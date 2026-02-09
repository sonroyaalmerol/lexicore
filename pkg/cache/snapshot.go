package cache

import (
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/cespare/xxhash/v2"
)

type Snapshot struct {
	Identities      map[string]*CachedIdentity
	Groups          map[string]*CachedGroup
	Version         int64
	CapturedAt      time.Time
	SourceDigest    uint64
	GroupMembership map[string][]string
	GroupMembers    map[string][]string
}

type CachedIdentity struct {
	Data             source.Identity
	Hash             uint64
	LastModified     time.Time
	Version          int64
	AffectedByGroups []string
}

type CachedGroup struct {
	Data              source.Group
	Hash              uint64
	LastModified      time.Time
	Version           int64
	AffectsIdentities []string
}

func newEmptySnapshot() *Snapshot {
	return &Snapshot{
		Identities:      make(map[string]*CachedIdentity),
		Groups:          make(map[string]*CachedGroup),
		GroupMembership: make(map[string][]string),
		GroupMembers:    make(map[string][]string),
		Version:         0,
		CapturedAt:      time.Time{},
	}
}

func buildDependencyGraph(
	identities map[string]*CachedIdentity,
	groups map[string]*CachedGroup,
) (map[string][]string, map[string][]string) {
	groupMembership := make(map[string][]string)
	groupMembers := make(map[string][]string)

	for identityKey, cached := range identities {
		validGroups := make([]string, 0, len(cached.Data.Groups))

		for _, groupKey := range cached.Data.Groups {
			if _, exists := groups[groupKey]; !exists {
				continue
			}

			validGroups = append(validGroups, groupKey)
			groupMembers[groupKey] = append(groupMembers[groupKey], identityKey)
		}

		if len(validGroups) > 0 {
			groupMembership[identityKey] = validGroups
		}
	}

	return groupMembership, groupMembers
}

func computeSnapshotDigest(snapshot *Snapshot) uint64 {
	h := xxhash.New()

	for _, cached := range snapshot.Identities {
		h.Write([]byte{byte(cached.Hash)})
	}
	for _, cached := range snapshot.Groups {
		h.Write([]byte{byte(cached.Hash)})
	}

	return h.Sum64()
}
