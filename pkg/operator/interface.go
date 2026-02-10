package operator

import (
	"context"
	"sync/atomic"

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
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool
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
