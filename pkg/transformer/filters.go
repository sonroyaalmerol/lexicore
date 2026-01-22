package transformer

import (
	"slices"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type FilterTransformer struct {
	groupFilter string
	emailDomain string
}

func NewFilterTransformer(config map[string]any) (*FilterTransformer, error) {
	ft := &FilterTransformer{}

	if gf, ok := config["groupFilter"].(string); ok {
		ft.groupFilter = gf
	}

	if ed, ok := config["emailDomain"].(string); ok {
		ft.emailDomain = ed
	}

	return ft, nil
}

func (f *FilterTransformer) Transform(
	ctx *Context,
	identities []source.Identity,
	groups []source.Group,
) ([]source.Identity, []source.Group, error) {
	filtered := make([]source.Identity, 0)

	for _, identity := range identities {
		if f.shouldInclude(identity) {
			filtered = append(filtered, identity)
		}
	}

	return filtered, groups, nil
}

func (f *FilterTransformer) shouldInclude(identity source.Identity) bool {
	if f.groupFilter != "" {
		hasGroup := slices.Contains(identity.Groups, f.groupFilter)
		if !hasGroup {
			return false
		}
	}

	if f.emailDomain != "" {
		// Check email domain
		// Implementation here
	}

	return true
}
