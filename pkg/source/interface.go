package source

import (
	"context"
	"maps"
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
	Deleted     bool
}

type Group struct {
	GID         string
	Name        string
	Members     []string
	Attributes  map[string]any
	Description string
	Deleted     bool
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

	clone.Attributes = make(map[string]any, len(g.Attributes))
	maps.Copy(clone.Attributes, g.Attributes)

	return clone
}

type Source interface {
	Name() string
	Initialize(ctx context.Context, config map[string]any) error
	Validate(ctx context.Context) error

	// Connect establishes connection to the source
	Connect(ctx context.Context) error

	// GetIdentities fetches all identities
	GetIdentities(ctx context.Context) (map[string]Identity, error)

	// GetGroups fetches all groups
	GetGroups(ctx context.Context) (map[string]Group, error)

	// Close closes the connection
	Close() error
}

type PartialFetchCapable interface {
	// GetIdentitiesByUIDs fetches specific identities by UID
	GetIdentitiesByUIDs(ctx context.Context, uids []string) (map[string]Identity, error)

	// GetGroupsByGIDs fetches specific groups by GID
	GetGroupsByGIDs(ctx context.Context, gids []string) (map[string]Group, error)
}
