package operator

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type IncrementalOperator interface {
	Operator

	// SupportsIncrementalSync returns true if the operator can efficiently
	// apply only changed items
	SupportsIncrementalSync() bool

	// SyncIncremental applies only the changes specified
	SyncIncremental(ctx context.Context, changes *IncrementalSyncState) (*SyncResult, error)
}

type Operator interface {
	Name() string
	Initialize(ctx context.Context, config map[string]any) error
	Sync(ctx context.Context, state *SyncState) (*SyncResult, error)
	Validate(ctx context.Context) error
	Close() error
}

type SyncState struct {
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool
}

// IncrementalSyncState contains only items that need to be created/updated/deleted
type IncrementalSyncState struct {
	IdentitiesToCreate []source.Identity
	IdentitiesToUpdate []source.Identity
	IdentitiesToDelete []source.Identity
	GroupsToCreate     []source.Group
	GroupsToUpdate     []source.Group
	GroupsToDelete     []source.Group
	DryRun             bool
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
