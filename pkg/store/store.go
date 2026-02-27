package store

import (
	"context"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
)

type Store interface {
	GetIdentitySources(ctx context.Context) ([]*manifest.IdentitySource, error)
	GetSyncTargets(ctx context.Context) ([]*manifest.SyncTarget, error)
	Watch(ctx context.Context, onChange func()) error
}
