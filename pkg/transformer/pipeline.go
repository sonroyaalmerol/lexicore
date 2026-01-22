package transformer

import (
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
)

type Transformer interface {
	Transform(ctx *Context, identities []source.Identity, groups []source.Group) ([]source.Identity, []source.Group, error)
}

type Pipeline struct {
	transformers []Transformer
}

func NewPipeline(configs []manifest.TransformerConfig) (*Pipeline, error) {
	pipeline := &Pipeline{}

	for _, config := range configs {
		transformer, err := createTransformer(config)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create transformer %s: %w",
				config.Name,
				err,
			)
		}
		pipeline.transformers = append(pipeline.transformers, transformer)
	}

	return pipeline, nil
}

func (p *Pipeline) Execute(
	ctx *Context,
	identities []source.Identity,
	groups []source.Group,
) ([]source.Identity, []source.Group, error) {
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
	case "constant":
		return NewConstantTransformer(config.Config)
	case "template":
		return NewTemplateTransformer(config.Config)
	case "sanitizer":
		return NewSanitizerTransformer(config.Config)
	default:
		return nil, fmt.Errorf("unknown transformer type: %s", config.Type)
	}
}
