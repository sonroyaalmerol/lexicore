package transformer

import (
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type PrefixTransformer struct {
	prefix string
}

func NewPrefixTransformer(prefix string) (*PrefixTransformer, error) {
	return &PrefixTransformer{prefix: prefix}, nil
}

func (f *PrefixTransformer) Transform(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	if f.prefix == "" {
		return identities, groups, nil
	}

	for k, identity := range identities {
		newAttrs := make(map[string]any, len(identity.Attributes))
		for ak, v := range identity.Attributes {
			fieldName, hasPrefix := strings.CutPrefix(ak, f.prefix)
			if hasPrefix {
				newAttrs[fieldName] = v
			}
		}
		identity.Attributes = newAttrs
		identities[k] = identity
	}

	return identities, groups, nil
}
