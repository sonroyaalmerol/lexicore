package starlarklib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeHashModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "hash",
		Members: starlark.StringDict{
			"md5":    starlark.NewBuiltin("hash.md5", hashMD5),
			"sha1":   starlark.NewBuiltin("hash.sha1", hashSHA1),
			"sha256": starlark.NewBuiltin("hash.sha256", hashSHA256),
			"sha512": starlark.NewBuiltin("hash.sha512", hashSHA512),
		},
	}
}

func hashMD5(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("hash.md5", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := md5.Sum([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}

func hashSHA1(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("hash.sha1", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := sha1.Sum([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}

func hashSHA256(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("hash.sha256", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}

func hashSHA512(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs("hash.sha512", args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := sha512.Sum512([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}
