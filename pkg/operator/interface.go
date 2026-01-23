package operator

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Operator interface {
	Name() string
	Initialize(ctx context.Context, config map[string]any) error
	Sync(ctx context.Context, state *SyncState) (*SyncResult, error)
	Validate(ctx context.Context) error
	Close() error
}

type SyncState struct {
	Identities []source.Identity
	Groups     []source.Group
	DryRun     bool
}

type SyncResult struct {
	IdentitiesCreated int
	IdentitiesUpdated int
	IdentitiesDeleted int
	GroupsCreated     int
	GroupsUpdated     int
	GroupsDeleted     int
	Errors            []error
}
