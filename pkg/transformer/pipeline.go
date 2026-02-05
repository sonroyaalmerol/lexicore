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

func NewPipeline(configs []manifest.TransformerConfig) (*Pipeline, error) {
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

	return &Pipeline{transformers: transformers}, nil
}

func (p *Pipeline) Execute(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	var err error

	for _, transformer := range p.transformers {
		identities, groups, err = transformer.Transform(ctx, identities, groups)
		if err != nil {
			return nil, nil, err
		}
	}

	return identities, groups, nil
}

func createTransformer(config manifest.TransformerConfig) (Transformer, error) {
	switch config.Type {
	case "selector":
		return NewSelectorTransformer(config.Config)
	case "template":
		return NewTemplateTransformer(config.Config)
	case "sanitizer":
		return NewSanitizerTransformer(config.Config)
	default:
		return nil, fmt.Errorf("unknown transformer type: %s", config.Type)
	}
}
