package source

import (
	"context"
	"maps"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
)

type SourceData struct {
	Identities map[string]Identity
	Groups     map[string]Group
}

type Identity struct {
	UID         string
	Username    string
	Email       string
	Groups      []string
	Attributes  map[string]any
	DisplayName string
	Disabled    bool
}

type Group struct {
	GID         string
	Name        string
	Members     []string
	Parents     []string
	Attributes  map[string]any
	Description string
}

func (id Identity) DeepCopy() Identity {
	clone := id

	clone.Groups = make([]string, len(id.Groups))
	copy(clone.Groups, id.Groups)

	clone.Attributes = make(map[string]any, len(id.Attributes))
	maps.Copy(clone.Attributes, id.Attributes)

	return clone
}

func (g Group) DeepCopy() Group {
	clone := g

	clone.Members = make([]string, len(g.Members))
	copy(clone.Members, g.Members)

	clone.Parents = make([]string, len(g.Parents))
	copy(clone.Parents, g.Parents)

	clone.Attributes = make(map[string]any, len(g.Attributes))
	maps.Copy(clone.Attributes, g.Attributes)

	return clone
}

type Source interface {
	Name() string
	Initialize(ctx context.Context, config map[string]manifest.ConfigValue) error
	Validate(ctx context.Context) error
	Connect(ctx context.Context) error
	GetIdentities(ctx context.Context) (map[string]Identity, error)
	GetGroups(ctx context.Context) (map[string]Group, error)
	Close() error
}

type PartialFetchCapable interface {
	GetIdentitiesByUIDs(ctx context.Context, uids []string) (map[string]Identity, error)
	GetGroupsByGIDs(ctx context.Context, gids []string) (map[string]Group, error)
}
