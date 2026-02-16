package operator

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Operator interface {
	Name() string
	Initialize(ctx context.Context, config map[string]any) error
	Sync(ctx context.Context, state *SyncState) (*SyncResult, error)
	PartialSync(ctx context.Context, state *PartialSyncState) (*SyncResult, error)
	Validate(ctx context.Context) error
	Close() error
	ShouldSkipUnchangedSync() bool
}

type PartialSyncState struct {
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool

	RequestedIdentityUIDs []string
	RequestedGroupGIDs    []string
}

type SyncState struct {
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool
}

type GroupAttributeMapping struct {
	SourceAttribute string `json:"sourceAttribute"`
	TargetAttribute string `json:"targetAttribute"`
	AggregationMode string `json:"aggregationMode"`
	DefaultValue    any    `json:"defaultValue,omitempty"`
	WeightAttribute string `json:"weightAttribute,omitempty"`
}
