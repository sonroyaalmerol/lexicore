package discovery

import (
	"context"
)

type Discovery interface {
	GetPeers(ctx context.Context) ([]string, error)
	GetNodeName() string
	GetNodeIP() string
}

type DynamicDiscovery interface {
	Discovery
	StartMembershipSync(ctx context.Context, callbacks MembershipCallbacks) error
	Close() error
}

type MembershipCallbacks struct {
	OnMemberJoin  func(name, peerURL string) error
	OnMemberLeave func(name string) error
}

type StaticDiscovery interface {
	Discovery
}

