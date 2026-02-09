package cache

import (
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/cespare/xxhash/v2"
)

func ComputeHash(data any) (uint64, error) {
	h := xxhash.New()

	switch v := data.(type) {
	case source.Identity:
		hashIdentity(h, v)
	case source.Group:
		hashGroup(h, v)
	}

	return h.Sum64(), nil
}

func hashIdentity(h *xxhash.Digest, identity source.Identity) {
	h.WriteString(identity.UID)
	h.WriteString(identity.Username)
	h.WriteString(identity.Email)
	h.WriteString(identity.DisplayName)
	if identity.Disabled {
		h.WriteString("disabled")
	}
	for _, g := range identity.Groups {
		h.WriteString(g)
	}
	for k, val := range identity.Attributes {
		h.WriteString(k)
		if s, ok := val.(string); ok {
			h.WriteString(s)
		}
	}
}

func hashGroup(h *xxhash.Digest, group source.Group) {
	h.WriteString(group.GID)
	h.WriteString(group.Name)
	h.WriteString(group.Description)
	for _, m := range group.Members {
		h.WriteString(m)
	}
	for k, val := range group.Attributes {
		h.WriteString(k)
		if s, ok := val.(string); ok {
			h.WriteString(s)
		}
	}
}
