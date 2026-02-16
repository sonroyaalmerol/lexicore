package authentik

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (s *AuthentikSource) SupportsWebhooks() bool {
	return true
}

func (s *AuthentikSource) ProcessWebhookEvent(ctx context.Context, payload []byte) (*source.WebhookEvent, error) {
	var webhookPayload struct {
		Model  string         `json:"model"`
		Action string         `json:"action"`
		Data   map[string]any `json:"data"`
	}

	if err := json.Unmarshal(payload, &webhookPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	event := &source.WebhookEvent{
		Timestamp: time.Now(),
	}

	switch webhookPayload.Model {
	case "user":
		event.Identity = s.parseIdentityFromWebhook(webhookPayload.Data)
		switch webhookPayload.Action {
		case "created":
			event.Type = source.WebhookIdentityCreated
		case "updated":
			event.Type = source.WebhookIdentityUpdated
		case "deleted":
			event.Type = source.WebhookIdentityDeleted
		default:
			return nil, fmt.Errorf("unknown action: %s", webhookPayload.Action)
		}

	case "group":
		event.Group = s.parseGroupFromWebhook(webhookPayload.Data)
		switch webhookPayload.Action {
		case "created":
			event.Type = source.WebhookGroupCreated
		case "updated":
			event.Type = source.WebhookGroupUpdated
		case "deleted":
			event.Type = source.WebhookGroupDeleted
		default:
			return nil, fmt.Errorf("unknown action: %s", webhookPayload.Action)
		}

	default:
		return nil, fmt.Errorf("unknown model: %s", webhookPayload.Model)
	}

	return event, nil
}

func (s *AuthentikSource) parseIdentityFromWebhook(data map[string]any) *source.Identity {
	pk := s.convertToString(data["pk"])

	identity := &source.Identity{
		UID:        pk,
		Attributes: make(map[string]any),
	}

	if username, ok := data["username"].(string); ok {
		identity.Username = username
	}
	if email, ok := data["email"].(string); ok {
		identity.Email = email
	}
	if name, ok := data["name"].(string); ok {
		identity.DisplayName = name
	}
	if isActive, ok := data["is_active"].(bool); ok {
		identity.Disabled = !isActive
	}
	if groups, ok := data["groups"].([]any); ok {
		for _, g := range groups {
			if gid := s.convertToString(g); gid != "" {
				identity.Groups = append(identity.Groups, gid)
			}
		}
	}
	if attrs, ok := data["attributes"].(map[string]any); ok {
		identity.Attributes = attrs
	}

	return identity
}

func (s *AuthentikSource) parseGroupFromWebhook(data map[string]any) *source.Group {
	pk := s.convertToString(data["pk"])

	group := &source.Group{
		GID:        pk,
		Attributes: make(map[string]any),
	}

	if name, ok := data["name"].(string); ok {
		group.Name = name
	}
	if users, ok := data["users"].([]any); ok {
		for _, u := range users {
			if uid := s.convertToString(u); uid != "" {
				group.Members = append(group.Members, uid)
			}
		}
	}
	if attrs, ok := data["attributes"].(map[string]any); ok {
		group.Attributes = attrs
		if desc, ok := attrs["description"].(string); ok {
			group.Description = desc
		}
	}

	return group
}
