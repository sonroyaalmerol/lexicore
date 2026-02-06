package starlarklib

import (
	"encoding/json"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeJSONModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "json",
		Members: starlark.StringDict{
			"encode": starlark.NewBuiltin("json.encode", jsonEncode),
			"decode": starlark.NewBuiltin("json.decode", jsonDecode),
		},
	}
}

func jsonEncode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var value starlark.Value
	var indent int

	if err := starlark.UnpackArgs(
		"json.encode",
		args,
		kwargs,
		"value", &value,
		"indent?", &indent,
	); err != nil {
		return nil, err
	}

	goValue := starlarkToGoValue(value)

	var data []byte
	var err error

	if indent > 0 {
		data, err = json.MarshalIndent(goValue, "", string(make([]byte, indent)))
	} else {
		data, err = json.Marshal(goValue)
	}

	if err != nil {
		return nil, fmt.Errorf("json encode failed: %w", err)
	}

	return starlark.String(data), nil
}

func jsonDecode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("json.decode", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	var result any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("json decode failed: %w", err)
	}

	return goValueToStarlark(result), nil
}

