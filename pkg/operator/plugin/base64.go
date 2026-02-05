package starlarklib

import (
	"encoding/base64"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeBase64Module() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "base64",
		Members: starlark.StringDict{
			"encode": starlark.NewBuiltin("base64.encode", base64Encode),
			"decode": starlark.NewBuiltin("base64.decode", base64Decode),
		},
	}
}

func base64Encode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("base64.encode", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(data))
	return starlark.String(encoded), nil
}

func base64Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("base64.decode", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	return starlark.String(decoded), nil
}
