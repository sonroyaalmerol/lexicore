package operator

import (
	"context"
	"sync/atomic"

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

	// Identities that need reprocessing due to group attribute changes
	// The operator can recalculate derived attributes using the group context
	IdentitiesToReprocess []source.Identity

	// Full group context for deriving attributes
	// Available even in incremental mode for attribute computation
	AllGroups map[string]source.Group

	DryRun bool
}

type SyncResult struct {
	IdentitiesCreated     atomic.Uint64
	IdentitiesUpdated     atomic.Uint64
	IdentitiesDeleted     atomic.Uint64
	GroupsCreated         atomic.Uint64
	GroupsUpdated         atomic.Uint64
	GroupsDeleted         atomic.Uint64
	IdentitiesReprocessed atomic.Uint64
	ErrCount              atomic.Uint64
}

type GroupAttributeMapping struct {
	SourceAttribute string `json:"sourceAttribute"`
	TargetAttribute string `json:"targetAttribute"`
	AggregationMode string `json:"aggregationMode"` // first, last, max, min, append, override, weighted
	DefaultValue    any    `json:"defaultValue,omitempty"`
	WeightAttribute string `json:"weightAttribute,omitempty"`
}
