package starlarklib

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeTimeModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "time",
		Members: starlark.StringDict{
			"now":    starlark.NewBuiltin("time.now", timeNow),
			"sleep":  starlark.NewBuiltin("time.sleep", timeSleep),
			"parse":  starlark.NewBuiltin("time.parse", timeParse),
			"format": starlark.NewBuiltin("time.format", timeFormat),
			"unix":   starlark.NewBuiltin("time.unix", timeUnix),
		},
	}
}

func timeNow(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	now := time.Now()
	return starlark.String(now.Format(time.RFC3339)), nil
}

func timeSleep(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var seconds float64

	if err := starlark.UnpackArgs("time.sleep", args, kwargs, "seconds", &seconds); err != nil {
		return nil, err
	}

	duration := time.Duration(seconds * float64(time.Second))
	time.Sleep(duration)

	return starlark.None, nil
}

func timeParse(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var layout, value string

	if err := starlark.UnpackArgs(
		"time.parse",
		args,
		kwargs,
		"layout", &layout,
		"value", &value,
	); err != nil {
		return nil, err
	}

	t, err := time.Parse(layout, value)
	if err != nil {
		return nil, fmt.Errorf("time parse failed: %w", err)
	}

	return starlark.MakeInt64(t.Unix()), nil
}

func timeFormat(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var timestamp int64
	var layout string
	var timezone string = "UTC"

	if err := starlark.UnpackArgs(
		"time.format",
		args,
		kwargs,
		"timestamp", &timestamp,
		"layout", &layout,
		"timezone?", &timezone,
	); err != nil {
		return nil, err
	}

	t := time.Unix(timestamp, 0)

	if timezone != "UTC" && timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone: %w", err)
		}
		t = t.In(loc)
	} else {
		t = t.UTC()
	}

	formatted := t.Format(layout)

	return starlark.String(formatted), nil
}

func timeUnix(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	now := time.Now()
	return starlark.MakeInt64(now.Unix()), nil
}
