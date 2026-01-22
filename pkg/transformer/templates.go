package transformer

import (
	"bytes"
	"fmt"
	"html/template"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type TemplateTransformer struct {
	templates map[string]*template.Template
}

func NewTemplateTransformer(config map[string]any) (*TemplateTransformer, error) {
	tt := &TemplateTransformer{
		templates: make(map[string]*template.Template),
	}

	if templates, ok := config["templates"].(map[string]any); ok {
		for key, tmplStr := range templates {
			str, ok := tmplStr.(string)
			if !ok {
				continue
			}

			tmpl, err := template.New(key).Parse(str)
			if err != nil {
				return nil, fmt.Errorf("failed to parse template %s: %w", key, err)
			}
			tt.templates[key] = tmpl
		}
	}

	return tt, nil
}

func (t *TemplateTransformer) Transform(
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

		for key, tmpl := range t.templates {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, identity); err != nil {
				return nil, nil, fmt.Errorf(
					"failed to execute template %s: %w",
					key,
					err,
				)
			}
			transformed.Attributes[key] = buf.String()
		}

		transformedIdentities[i] = transformed
	}

	return transformedIdentities, groups, nil
}
