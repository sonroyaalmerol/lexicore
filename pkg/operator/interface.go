package operator

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Operator interface {
	Name() string
	Initialize(ctx context.Context, config map[string]any) error
	Sync(ctx context.Context, state *SyncState) error
	PartialSync(ctx context.Context, state *PartialSyncState) error
	Validate(ctx context.Context) error
	Close() error
	ShouldSkipUnchangedSync() bool
}

type SyncState struct {
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool
	Result     *SyncResult
}

type PartialSyncState struct {
	Identities map[string]source.Identity
	Groups     map[string]source.Group
	DryRun     bool
	Result     *SyncResult

	RequestedIdentityUIDs []string
	RequestedGroupGIDs    []string
}

type GroupAttributeMapping struct {
	SourceAttribute string `json:"sourceAttribute"`
	TargetAttribute string `json:"targetAttribute"`
	AggregationMode string `json:"aggregationMode"`
	DefaultValue    any    `json:"defaultValue,omitempty"`
	WeightAttribute string `json:"weightAttribute,omitempty"`
}
