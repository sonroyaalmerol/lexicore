package source

import (
	"context"
	"time"
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
	Attributes  map[string]any
	Description string
}

type Changes struct {
	ModifiedIdentities []Identity
	DeletedIdentities  []string // UIDs
	ModifiedGroups     []Group
	DeletedGroups      []string // GIDs
	FullSync           bool     // If true, this is a full snapshot
}

type ChangeDetector interface {
	// SupportsChangeDetection returns true if the source can efficiently
	// detect changes since a given timestamp
	SupportsChangeDetection() bool

	// GetChangesSince returns only items modified after the given time
	// Returns the changes and the current server timestamp
	GetChangesSince(ctx context.Context, since time.Time) (*Changes, time.Time, error)
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

type Event struct {
	Type      EventType // Add, Update, Delete
	Identity  *Identity
	Group     *Group
	Timestamp int64
}

type EventType string

const (
	EventAdd    EventType = "Add"
	EventUpdate EventType = "Update"
	EventDelete EventType = "Delete"
)
