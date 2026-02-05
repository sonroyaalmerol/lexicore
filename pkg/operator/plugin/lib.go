package starlarklib

import (
	"bytes"
	"context"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func MakeBuiltins(ctx context.Context) starlark.StringDict {
	return starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"print":  starlark.NewBuiltin("print", starlarkPrint),
		"http":   makeHTTPModule(),
		"ldap":   makeLDAPModule(ctx),
		"sql":    makeSQLModule(ctx),
		"json":   makeJSONModule(),
		"base64": makeBase64Module(),
		"time":   makeTimeModule(),
		"hash":   makeHashModule(),
	}
}

func starlarkPrint(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	sep := " "
	if err := starlark.UnpackArgs("print", nil, kwargs, "sep?", &sep); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	for i, v := range args {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(v.String())
	}
	fmt.Println(buf.String())
	return starlark.None, nil
}

func starlarkToGoValue(v starlark.Value) any {
	switch val := v.(type) {
	case starlark.String:
		return string(val)
	case starlark.Int:
		i, _ := val.Int64()
		return i
	case starlark.Float:
		return float64(val)
	case starlark.Bool:
		return bool(val)
	case *starlark.List:
		result := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			result[i] = starlarkToGoValue(val.Index(i))
		}
		return result
	case *starlark.Dict:
		result := make(map[string]any)
		for _, item := range val.Items() {
			key := item[0]
			value := item[1]
			if keyStr, ok := key.(starlark.String); ok {
				result[string(keyStr)] = starlarkToGoValue(value)
			}
		}
		return result
	case starlark.NoneType:
		return nil
	default:
		return val.String()
	}
}

func goValueToStarlark(v any) starlark.Value {
	if v == nil {
		return starlark.None
	}

	switch val := v.(type) {
	case string:
		return starlark.String(val)
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case float64:
		return starlark.Float(val)
	case bool:
		return starlark.Bool(val)
	case []any:
		list := make([]starlark.Value, len(val))
		for i, item := range val {
			list[i] = goValueToStarlark(item)
		}
		return starlark.NewList(list)
	case map[string]any:
		dict := starlark.NewDict(len(val))
		for k, v := range val {
			dict.SetKey(starlark.String(k), goValueToStarlark(v))
		}
		return dict
	default:
		return starlark.String(fmt.Sprintf("%v", v))
	}
}
