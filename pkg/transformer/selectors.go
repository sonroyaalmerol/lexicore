package transformer

import (
	"slices"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type SelectorTransformer struct {
	groupSelector string
	emailDomain   string
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
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	for key, identity := range identities {
		if !f.shouldIncludeUser(identity) {
			delete(identities, key)
		}
	}
	for key, group := range groups {
		if !f.shouldIncludeGroup(group) {
			delete(groups, key)
		}
	}

	return identities, groups, nil
}

func (f *SelectorTransformer) shouldIncludeUser(identity source.Identity) bool {
	if f.groupSelector != "" {
		if !slices.Contains(identity.Groups, f.groupSelector) {
			return false
		}
	}

	if f.emailDomain != "" {
		// Check email domain
		// Implementation here
	}

	return true
}

func (f *SelectorTransformer) shouldIncludeGroup(group source.Group) bool {
	if f.groupSelector != "" {
		if group.Name != f.groupSelector {
			return false
		}
	}

	return true
}
