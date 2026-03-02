package authentik

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type authentikEvent struct {
	PK      string         `json:"pk"`
	Action  string         `json:"action"`
	App     string         `json:"app"`
	Context map[string]any `json:"context"`
	Created string         `json:"created"`
}

func (s *AuthentikSource) SupportsWebhooks() bool {
	return true
}

func (s *AuthentikSource) ProcessWebhookEvent(ctx context.Context, payload []byte) (*source.WebhookEvent, error) {
	var ev authentikEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	ts := time.Now()
	if ev.Created != "" {
		if t, err := time.Parse(time.RFC3339Nano, ev.Created); err == nil {
			ts = t
		}
	}

	switch ev.Action {
	case "model_created", "model_updated", "model_deleted":
	default:
		return nil, fmt.Errorf("unhandled action: %s", ev.Action)
	}

	modelObj, ok := ev.Context["model"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing or invalid context.model in event %s", ev.PK)
	}

	modelName, _ := modelObj["model_name"].(string)
	modelApp, _ := modelObj["app"].(string)

	event := &source.WebhookEvent{Timestamp: ts}

	switch {
	case modelName == "user" && modelApp == "authentik_core":
		event.Identity = s.parseIdentityFromWebhook(ev.Context)
		switch ev.Action {
		case "model_created":
			event.Type = source.WebhookIdentityCreated
		case "model_updated":
			event.Type = source.WebhookIdentityUpdated
		case "model_deleted":
			event.Type = source.WebhookIdentityDeleted
		}

	case modelName == "group" && modelApp == "authentik_core":
		event.Group = s.parseGroupFromWebhook(ev.Context)
		switch ev.Action {
		case "model_created":
			event.Type = source.WebhookGroupCreated
		case "model_updated":
			event.Type = source.WebhookGroupUpdated
		case "model_deleted":
			event.Type = source.WebhookGroupDeleted
		}

	default:
		return nil, fmt.Errorf(
			"unhandled model: %s/%s",
			modelApp,
			modelName,
		)
	}

	return event, nil
}

func (s *AuthentikSource) parseIdentityFromWebhook(
	ctx map[string]any,
) *source.Identity {
	modelObj, _ := ctx["model"].(map[string]any)
	pk := s.convertToString(modelObj["pk"])

	identity := &source.Identity{
		UID:        pk,
		Attributes: make(map[string]any),
	}

	attrs, _ := ctx["attributes"].(map[string]any)

	if username, ok := attrs["username"].(string); ok {
		identity.Username = username
	}
	if email, ok := attrs["email"].(string); ok {
		identity.Email = email
	}
	if name, ok := attrs["name"].(string); ok {
		identity.DisplayName = name
	}
	if isActive, ok := attrs["is_active"].(bool); ok {
		identity.Disabled = !isActive
	}
	if groups, ok := attrs["groups"].([]any); ok {
		for _, g := range groups {
			if gid := s.convertToString(g); gid != "" {
				identity.Groups = append(identity.Groups, gid)
			}
		}
	}
	if extra, ok := attrs["attributes"].(map[string]any); ok {
		identity.Attributes = extra
	}

	return identity
}

func (s *AuthentikSource) parseGroupFromWebhook(
	ctx map[string]any,
) *source.Group {
	modelObj, _ := ctx["model"].(map[string]any)
	pk := s.convertToString(modelObj["pk"])

	group := &source.Group{
		GID:        pk,
		Attributes: make(map[string]any),
	}

	attrs, _ := ctx["attributes"].(map[string]any)

	if name, ok := attrs["name"].(string); ok {
		group.Name = name
	}
	if users, ok := attrs["users"].([]any); ok {
		for _, u := range users {
			if uid, ok := u.(map[string]any); ok {
				if pkStr := s.convertToString(uid["pk"]); pkStr != "" {
					group.Members = append(group.Members, pkStr)
				}
			} else if pkStr := s.convertToString(u); pkStr != "" {
				group.Members = append(group.Members, pkStr)
			}
		}
	}
	if extra, ok := attrs["attributes"].(map[string]any); ok {
		group.Attributes = extra
		if desc, ok := extra["description"].(string); ok {
			group.Description = desc
		}
	}

	return group
}

func (s *AuthentikSource) parseGroupPK(raw any) string {
	switch v := raw.(type) {
	case map[string]any:
		return s.convertToString(v["pk"])
	case string:
		return v
	case int, int32, int64, float64:
		return s.convertToString(v)
	default:
		return ""
	}
}

func parseUserPK(raw any) string {
	switch v := raw.(type) {
	case map[string]any:
		pk := v["pk"]
		switch p := pk.(type) {
		case float64:
			return strconv.Itoa(int(p))
		case string:
			return p
		}
	case float64:
		return strconv.Itoa(int(v))
	case string:
		return v
	}
	return ""
}
