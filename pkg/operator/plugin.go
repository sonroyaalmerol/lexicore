package operator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	starlarklib "codeberg.org/lexicore/lexicore/pkg/operator/plugin"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

type StarlarkOperator struct {
	*BaseOperator
	thread  *starlark.Thread
	globals starlark.StringDict
	module  *starlarkstruct.Module
}

func registerStarlarkOperator(scriptPath string) error {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read starlark script: %w", err)
	}

	name := filepath.Base(scriptPath)
	thread := &starlark.Thread{Name: name}

	predeclared := starlarklib.MakeBuiltins(context.Background())

	opts := &syntax.FileOptions{
		Set:             true,
		GlobalReassign:  true,
		TopLevelControl: true,
		Recursion:       true,
	}

	globals, err := starlark.ExecFileOptions(opts, thread, scriptPath, content, predeclared)
	if err != nil {
		return fmt.Errorf("failed to execute starlark script: %w", err)
	}

	requiredFuncs := []string{"initialize", "sync", "validate"}
	for _, funcName := range requiredFuncs {
		if _, ok := globals[funcName]; !ok {
			return fmt.Errorf("starlark script missing required function: %s", funcName)
		}
	}

	operatorName := name
	if nameVal, ok := globals["name"]; ok {
		if nameStr, ok := nameVal.(starlark.String); ok {
			operatorName = string(nameStr)
		}
	}

	Register(operatorName, func() Operator {
		return &StarlarkOperator{
			BaseOperator: NewBaseOperator(operatorName),
			thread:       thread,
			globals:      globals,
		}
	})

	return nil
}

func (s *StarlarkOperator) Initialize(ctx context.Context, config map[string]any) error {
	s.SetConfig(config)

	initFunc, ok := s.globals["initialize"]
	if !ok {
		return fmt.Errorf("initialize function not found")
	}

	callable, ok := initFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("initialize is not callable")
	}

	configDict := goMapToStarlarkDict(config)

	args := starlark.Tuple{configDict}
	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	if errStr := extractError(result); errStr != "" {
		return fmt.Errorf("%s", errStr)
	}

	return nil
}

func (s *StarlarkOperator) Validate(ctx context.Context) error {
	validateFunc, ok := s.globals["validate"]
	if !ok {
		return fmt.Errorf("validate function not found")
	}

	callable, ok := validateFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("validate is not callable")
	}

	configDict := goMapToStarlarkDict(s.config)
	args := starlark.Tuple{configDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return fmt.Errorf("validate failed: %w", err)
	}

	if errStr := extractError(result); errStr != "" {
		return fmt.Errorf("%s", errStr)
	}

	return nil
}

func (s *StarlarkOperator) Sync(ctx context.Context, state *SyncState) (*SyncResult, error) {
	syncFunc, ok := s.globals["sync"]
	if !ok {
		return nil, fmt.Errorf("sync function not found")
	}

	callable, ok := syncFunc.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("sync is not callable")
	}

	stateDict := syncStateToStarlark(state)
	args := starlark.Tuple{stateDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}

	syncResult, err := starlarkToSyncResult(result)
	if err != nil {
		return nil, err
	}

	return syncResult, nil
}

func (s *StarlarkOperator) Close() error {
	if closeFunc, ok := s.globals["close"]; ok {
		if callable, ok := closeFunc.(starlark.Callable); ok {
			_, err := starlark.Call(s.thread, callable, nil, nil)
			if err != nil {
				return fmt.Errorf("close failed: %w", err)
			}
		}
	}
	return nil
}

func goMapToStarlarkDict(m map[string]any) *starlark.Dict {
	dict := starlark.NewDict(len(m))
	for k, v := range m {
		dict.SetKey(starlark.String(k), goValueToStarlark(v))
	}
	return dict
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
		return goMapToStarlarkDict(val)
	default:
		return starlark.String(fmt.Sprintf("%v", v))
	}
}

func syncStateToStarlark(state *SyncState) *starlark.Dict {
	dict := starlark.NewDict(3)

	identities := starlark.NewDict(len(state.Identities))
	for uid, identity := range state.Identities {
		identities.SetKey(starlark.String(uid), identityToStarlark(&identity))
	}
	dict.SetKey(starlark.String("identities"), identities)

	groups := starlark.NewDict(len(state.Groups))
	for gid, group := range state.Groups {
		groups.SetKey(starlark.String(gid), groupToStarlark(&group))
	}
	dict.SetKey(starlark.String("groups"), groups)

	dict.SetKey(starlark.String("dry_run"), starlark.Bool(state.DryRun))

	return dict
}

func identityToStarlark(id *source.Identity) *starlark.Dict {
	dict := starlark.NewDict(10)
	dict.SetKey(starlark.String("uid"), starlark.String(id.UID))
	dict.SetKey(starlark.String("username"), starlark.String(id.Username))
	dict.SetKey(starlark.String("email"), starlark.String(id.Email))
	dict.SetKey(starlark.String("display_name"), starlark.String(id.DisplayName))
	dict.SetKey(starlark.String("disabled"), starlark.Bool(id.Disabled))

	groups := make([]starlark.Value, len(id.Groups))
	for i, g := range id.Groups {
		groups[i] = starlark.String(g)
	}
	dict.SetKey(starlark.String("groups"), starlark.NewList(groups))

	dict.SetKey(starlark.String("attributes"), goMapToStarlarkDict(id.Attributes))
	return dict
}

func groupToStarlark(g *source.Group) *starlark.Dict {
	dict := starlark.NewDict(10)
	dict.SetKey(starlark.String("gid"), starlark.String(g.GID))
	dict.SetKey(starlark.String("name"), starlark.String(g.Name))
	dict.SetKey(starlark.String("description"), starlark.String(g.Description))

	members := make([]starlark.Value, len(g.Members))
	for i, member := range g.Members {
		members[i] = starlark.String(member)
	}
	dict.SetKey(starlark.String("members"), starlark.NewList(members))
	dict.SetKey(starlark.String("attributes"), goMapToStarlarkDict(g.Attributes))

	return dict
}

func starlarkToSyncResult(v starlark.Value) (*SyncResult, error) {
	dict, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("sync must return a dict")
	}

	result := &SyncResult{
		Errors: make([]error, 0),
	}

	if val, found, _ := dict.Get(starlark.String("identities_created")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.IdentitiesCreated = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("identities_updated")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.IdentitiesUpdated = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("identities_deleted")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.IdentitiesDeleted = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("groups_created")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.GroupsCreated = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("groups_updated")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.GroupsUpdated = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("groups_deleted")); found {
		if intVal, ok := val.(starlark.Int); ok {
			i, _ := intVal.Int64()
			result.GroupsDeleted = int(i)
		}
	}

	if val, found, _ := dict.Get(starlark.String("errors")); found {
		if listVal, ok := val.(*starlark.List); ok {
			for i := 0; i < listVal.Len(); i++ {
				if errStr, ok := listVal.Index(i).(starlark.String); ok {
					result.Errors = append(result.Errors, fmt.Errorf("%s", errStr))
				}
			}
		}
	}

	return result, nil
}

func extractError(v starlark.Value) string {
	if v == nil || v == starlark.None {
		return ""
	}

	if dict, ok := v.(*starlark.Dict); ok {
		if errVal, found, _ := dict.Get(starlark.String("error")); found {
			if errStr, ok := errVal.(starlark.String); ok {
				return string(errStr)
			}
		}
	}

	return ""
}
