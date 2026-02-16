package source

import (
	"context"
	"time"
)

type WebhookCapable interface {
	SupportsWebhooks() bool

	ProcessWebhookEvent(ctx context.Context, payload []byte) (*WebhookEvent, error)
}

type WebhookEvent struct {
	Type      WebhookEventType
	Identity  *Identity
	Group     *Group
	Timestamp time.Time
}

type WebhookEventType string

const (
	WebhookIdentityCreated WebhookEventType = "identity.created"
	WebhookIdentityUpdated WebhookEventType = "identity.updated"
	WebhookIdentityDeleted WebhookEventType = "identity.deleted"
	WebhookGroupCreated    WebhookEventType = "group.created"
	WebhookGroupUpdated    WebhookEventType = "group.updated"
	WebhookGroupDeleted    WebhookEventType = "group.deleted"
)
