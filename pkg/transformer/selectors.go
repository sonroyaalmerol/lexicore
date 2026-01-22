package transformer

import (
	"slices"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type SelectorTransformer struct {
	groupSelector string
	emailDomain     string
}

func NewSelectorTransformer(config map[string]any) (*SelectorTransformer, error) {
	ft := &SelectorTransformer{}

	if gf, ok := config["groupSelector"].(string); ok {
		ft.groupSelector = gf
	}

	if ed, ok := config["emailDomain"].(string); ok {
		ft.emailDomain = ed
	}

	return ft, nil
}

func (f *SelectorTransformer) Transform(
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

func (f *SelectorTransformer) shouldInclude(identity source.Identity) bool {
	if f.groupSelector != "" {
		hasGroup := slices.Contains(identity.Groups, f.groupSelector)
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
