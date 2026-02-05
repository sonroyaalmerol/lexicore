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
	templates, ok := config["templates"].(map[string]any)
	if !ok {
		return &TemplateTransformer{templates: make(map[string]*template.Template)}, nil
	}

	tt := &TemplateTransformer{
		templates: make(map[string]*template.Template, len(templates)),
	}

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

	return tt, nil
}

func (t *TemplateTransformer) Transform(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	var buf bytes.Buffer

	for key, identity := range identities {
		transformed := identity
		if transformed.Attributes == nil {
			transformed.Attributes = make(map[string]any, len(t.templates))
		}

		for tmplKey, tmpl := range t.templates {
			buf.Reset()
			if err := tmpl.Execute(&buf, identity); err != nil {
				return nil, nil, fmt.Errorf(
					"failed to execute template %s: %w",
					tmplKey,
					err,
				)
			}
			transformed.Attributes[tmplKey] = buf.String()
		}

		identities[key] = transformed
	}

	return identities, groups, nil
}
