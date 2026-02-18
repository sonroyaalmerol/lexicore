package transformer

import (
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Transformer interface {
	Transform(ctx *Context, identities map[string]source.Identity, groups map[string]source.Group) (map[string]source.Identity, map[string]source.Group, error)
}

type Pipeline struct {
	transformers []Transformer
}

func NewPipeline(configs []manifest.TransformerConfig, prefix string) (*Pipeline, error) {
	transformers := make([]Transformer, 0, len(configs))

	for _, config := range configs {
		transformer, err := createTransformer(config)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create transformer %s: %w",
				config.Name,
				err,
			)
		}
		transformers = append(transformers, transformer)
	}

	prefixTransformer, err := NewPrefixTransformer(prefix)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create prefix transformer: %w",
			err,
		)
	}

	transformers = append(transformers, prefixTransformer)

	return &Pipeline{transformers: transformers}, nil
}

func (p *Pipeline) Execute(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	var err error

	newIdentities := make(map[string]source.Identity, len(identities))
	for k, v := range identities {
		newIdentities[k] = v.DeepCopy()
	}

	newGroups := make(map[string]source.Group, len(groups))
	for k, v := range groups {
		newGroups[k] = v.DeepCopy()
	}

	for _, transformer := range p.transformers {
		newIdentities, newGroups, err = transformer.Transform(ctx, newIdentities, newGroups)
		if err != nil {
			return nil, nil, err
		}
	}

	return newIdentities, newGroups, nil
}

func createTransformer(config manifest.TransformerConfig) (Transformer, error) {
	switch config.Type {
	case "selector":
		return NewSelectorTransformer(config.Config)
	case "template":
		return NewTemplateTransformer(config.Config)
	case "sanitizer":
		return NewSanitizerTransformer(config.Config)
	case "groupAggregation":
		return NewGroupAggregationTransformer(config.Config)
	default:
		return nil, fmt.Errorf("unknown transformer type: %s", config.Type)
	}
}
