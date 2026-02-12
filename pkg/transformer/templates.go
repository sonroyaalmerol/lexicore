package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
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

	funcMap := utils.CreateFuncMap()

	for key, tmplStr := range templates {
		str, ok := tmplStr.(string)
		if !ok {
			continue
		}

		tmpl, err := template.New(key).
			Funcs(funcMap).
			Option("missingkey=zero").
			Parse(str)
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

			result := buf.String()
			transformed.Attributes[tmplKey] = parseTemplateResult(result)
		}

		identities[key] = transformed
	}

	return identities, groups, nil
}

func parseTemplateResult(result string) any {
	trimmed := strings.TrimSpace(result)

	if trimmed == "" {
		return result
	}

	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{' ||
		trimmed == "true" || trimmed == "false" || trimmed == "null" ||
		(trimmed[0] >= '0' && trimmed[0] <= '9') || trimmed[0] == '-') {

		var jsonValue any
		if err := json.Unmarshal([]byte(trimmed), &jsonValue); err == nil {
			switch v := jsonValue.(type) {
			case []any, map[string]any:
				return normalizeJSONValue(v)
			case float64:
				if float64(int64(v)) == v {
					return int64(v)
				}
				return v
			case bool, nil:
				return v
			}
		}
	}

	return result
}

func normalizeJSONValue(v any) any {
	switch val := v.(type) {
	case []any:
		normalized := make([]any, len(val))
		for i, item := range val {
			normalized[i] = normalizeJSONValue(item)
		}
		return normalized
	case map[string]any:
		normalized := make(map[string]any, len(val))
		for key, item := range val {
			normalized[key] = normalizeJSONValue(item)
		}
		return normalized
	case float64:
		if float64(int64(val)) == val {
			return int64(val)
		}
		return val
	default:
		return val
	}
}
