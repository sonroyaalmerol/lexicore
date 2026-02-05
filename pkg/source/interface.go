package source

import (
	"context"
)

type Identity struct {
	UID         string
	Username    string
	Email       string
	Groups      []string
	Attributes  map[string]any
	DisplayName string
}

type Group struct {
	GID         string
	Name        string
	Members     []string
	Attributes  map[string]any
	Description string
}

type Source interface {
	// Connect establishes connection to the source
	Connect(ctx context.Context) error

	// GetIdentities fetches all identities
	GetIdentities(ctx context.Context) (map[string]Identity, error)

	// GetGroups fetches all groups
	GetGroups(ctx context.Context) (map[string]Group, error)

	// Watch returns a channel for change notifications (optional)
	Watch(ctx context.Context) (<-chan Event, error)

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
