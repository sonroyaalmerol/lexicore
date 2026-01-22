package transformer

import (
	"maps"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type MapTransformer struct {
	mappings map[string]string
	defaults map[string]any
}

func NewMapTransformer(config map[string]any) (*MapTransformer, error) {
	mt := &MapTransformer{
		mappings: make(map[string]string),
		defaults: make(map[string]any),
	}

	if mappings, ok := config["mappings"].(map[string]any); ok {
		maps.Copy(mt.defaults, mappings)
	}

	return mt, nil
}

func (m *MapTransformer) Transform(
	ctx *Context,
	identities []source.Identity,
	groups []source.Group,
) ([]source.Identity, []source.Group, error) {
	transformedIdentities := make([]source.Identity, len(identities))

	for i, identity := range identities {
		transformed := identity
		if transformed.Attributes == nil {
			transformed.Attributes = make(map[string]any)
		}

		for targetKey, sourceKey := range m.mappings {
			if val, ok := identity.Attributes[sourceKey]; ok {
				transformed.Attributes[targetKey] = val
			}
		}

		maps.Copy(transformed.Attributes, m.defaults)

		transformedIdentities[i] = transformed
	}

	return transformedIdentities, groups, nil
}
