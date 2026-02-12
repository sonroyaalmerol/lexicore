package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
)

func CreateFuncMap() template.FuncMap {
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
