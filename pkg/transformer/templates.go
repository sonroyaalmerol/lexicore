package transformer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"golang.org/x/text/cases"
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

	funcMap := createFuncMap()

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

func createFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": cases.Title,
		"trim":  strings.TrimSpace,
		"trimPrefix": func(prefix, s string) string {
			return strings.TrimPrefix(s, prefix)
		},
		"trimSuffix": func(suffix, s string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"split": func(sep, s string) []string {
			return strings.Split(s, sep)
		},
		"join": func(sep string, list any) string {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return fmt.Sprint(list)
			}
			parts := make([]string, v.Len())
			for i := 0; i < v.Len(); i++ {
				parts[i] = fmt.Sprint(v.Index(i).Interface())
			}
			return strings.Join(parts, sep)
		},
		"contains": func(substr, s string) bool {
			return strings.Contains(s, substr)
		},
		"hasPrefix": func(prefix, s string) bool {
			return strings.HasPrefix(s, prefix)
		},
		"hasSuffix": func(suffix, s string) bool {
			return strings.HasSuffix(s, suffix)
		},
		"repeat": func(count int, s string) string {
			return strings.Repeat(s, count)
		},
		"b64enc": func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		},
		"b64dec": func(s string) (string, error) {
			data, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
		"toString": func(v any) string {
			return fmt.Sprintf("%v", v)
		},
		"toJson": func(v any) (string, error) {
			data, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
		"toPrettyJson": func(v any) (string, error) {
			data, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
		"default": func(defaultVal any, val any) any {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},
		"coalesce": func(vals ...any) any {
			for _, val := range vals {
				if val != nil && val != "" {
					return val
				}
			}
			return nil
		},
		"ternary": func(trueVal, falseVal any, condition bool) any {
			if condition {
				return trueVal
			}
			return falseVal
		},
		"empty": func(val any) bool {
			if val == nil {
				return true
			}
			v := reflect.ValueOf(val)
			switch v.Kind() {
			case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
				return v.Len() == 0
			case reflect.Bool:
				return !v.Bool()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return v.Int() == 0
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return v.Uint() == 0
			case reflect.Float32, reflect.Float64:
				return v.Float() == 0
			default:
				return false
			}
		},
		"list": func(vals ...any) []any {
			return vals
		},
		"first": func(list any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			if v.Len() == 0 {
				return nil
			}
			return v.Index(0).Interface()
		},
		"last": func(list any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			if v.Len() == 0 {
				return nil
			}
			return v.Index(v.Len() - 1).Interface()
		},
		"rest": func(list any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			if v.Len() == 0 {
				return nil
			}
			return v.Slice(1, v.Len()).Interface()
		},
		"initial": func(list any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			if v.Len() == 0 {
				return nil
			}
			return v.Slice(0, v.Len()-1).Interface()
		},
		"append": func(list any, item any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			return reflect.Append(v, reflect.ValueOf(item)).Interface()
		},
		"prepend": func(list any, item any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			newSlice := reflect.MakeSlice(v.Type(), 0, v.Len()+1)
			newSlice = reflect.Append(newSlice, reflect.ValueOf(item))
			for i := 0; i < v.Len(); i++ {
				newSlice = reflect.Append(newSlice, v.Index(i))
			}
			return newSlice.Interface()
		},
		"uniq": func(list any) any {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil
			}
			seen := make(map[any]bool)
			result := reflect.MakeSlice(v.Type(), 0, v.Len())
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i).Interface()
				if !seen[item] {
					seen[item] = true
					result = reflect.Append(result, v.Index(i))
				}
			}
			return result.Interface()
		},
		"regexMatch": func(pattern, s string) bool {
			matched, _ := regexp.MatchString(pattern, s)
			return matched
		},
		"regexReplaceAll": func(pattern, replacement, s string) string {
			re := regexp.MustCompile(pattern)
			return re.ReplaceAllString(s, replacement)
		},
		"quote": func(s string) string {
			return fmt.Sprintf("%q", s)
		},
		"squote": func(s string) string {
			return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "\\'"))
		},
		"array": func(vals ...any) []any {
			return vals
		},
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict requires an even number of arguments")
			}
			result := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				result[key] = pairs[i+1]
			}
			return result, nil
		},
	}
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
