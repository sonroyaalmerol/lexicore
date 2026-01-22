package operator

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Operator interface {
	Name() string

	// Initialize sets up the operator with configuration
	Initialize(ctx context.Context, config map[string]any) error

	// Sync performs the synchronization
	Sync(ctx context.Context, state *SyncState) (*SyncResult, error)

	// Validate checks if the operator can connect and has proper permissions
	Validate(ctx context.Context) error

	// Close cleans up resources
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
