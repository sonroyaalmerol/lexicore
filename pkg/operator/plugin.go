package operator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	starlarklib "codeberg.org/lexicore/lexicore/pkg/plugin"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"go.uber.org/zap"
)

type PluginOperator struct {
	*BaseOperator
	thread  *starlark.Thread
	globals starlark.StringDict
	module  *starlarkstruct.Module
}

func NewPluginOperator(scriptPath string, logger *zap.Logger) (*PluginOperator, error) {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read starlark script: %w", err)
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
		return nil, fmt.Errorf("failed to execute starlark script: %w", err)
	}

	requiredFuncs := []string{"initialize", "sync", "validate"}
	for _, funcName := range requiredFuncs {
		if _, ok := globals[funcName]; !ok {
			return nil, fmt.Errorf("starlark script missing required function: %s", funcName)
		}
	}

	operatorName := name
	if nameVal, ok := globals["name"]; ok {
		if nameStr, ok := nameVal.(starlark.String); ok {
			operatorName = string(nameStr)
		}
	}

	return &PluginOperator{
		BaseOperator: NewBaseOperator(operatorName, logger),
		thread:       thread,
		globals:      globals,
	}, nil
}

func (s *PluginOperator) Initialize(ctx context.Context, config map[string]any) error {
	s.SetConfig(config)

	initFunc, ok := s.globals["initialize"]
	if !ok {
		return fmt.Errorf("initialize function not found")
	}

	callable, ok := initFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("initialize is not callable")
	}

	configDict := goMapToPluginDict(config)

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

func (s *PluginOperator) Validate(ctx context.Context) error {
	validateFunc, ok := s.globals["validate"]
	if !ok {
		return fmt.Errorf("validate function not found")
	}

	callable, ok := validateFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("validate is not callable")
	}

	configDict := goMapToPluginDict(s.config)
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

func (s *PluginOperator) Sync(ctx context.Context, state *SyncState) error {
	syncFunc, ok := s.globals["sync"]
	if !ok {
		return fmt.Errorf("sync function not found")
	}

	callable, ok := syncFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("sync is not callable")
	}

	stateDict := syncStateToPlugin(state)
	args := starlark.Tuple{stateDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	syncResult, err := starlarkToSyncResult(result)
	if err != nil {
		return err
	}

	state.Result = syncResult

	return nil
}

func (s *PluginOperator) Close() error {
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

func goMapToPluginDict(m map[string]any) *starlark.Dict {
	dict := starlark.NewDict(len(m))
	for k, v := range m {
		dict.SetKey(starlark.String(k), goValueToPlugin(v))
	}
	return dict
}

func goValueToPlugin(v any) starlark.Value {
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
			list[i] = goValueToPlugin(item)
		}
		return starlark.NewList(list)
	case map[string]any:
		return goMapToPluginDict(val)
	default:
		return starlark.String(fmt.Sprintf("%v", v))
	}
}

func syncStateToPlugin(state *SyncState) *starlark.Dict {
	dict := starlark.NewDict(3)

	identities := starlark.NewDict(len(state.Identities))
	for uid, identity := range state.Identities {
		identities.SetKey(starlark.String(uid), identityToPlugin(&identity))
	}
	dict.SetKey(starlark.String("identities"), identities)

	groups := starlark.NewDict(len(state.Groups))
	for gid, group := range state.Groups {
		groups.SetKey(starlark.String(gid), groupToPlugin(&group))
	}
	dict.SetKey(starlark.String("groups"), groups)

	dict.SetKey(starlark.String("dry_run"), starlark.Bool(state.DryRun))

	return dict
}

func identityToPlugin(id *source.Identity) *starlark.Dict {
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

	dict.SetKey(starlark.String("attributes"), goMapToPluginDict(id.Attributes))
	return dict
}

func groupToPlugin(g *source.Group) *starlark.Dict {
	dict := starlark.NewDict(10)
	dict.SetKey(starlark.String("gid"), starlark.String(g.GID))
	dict.SetKey(starlark.String("name"), starlark.String(g.Name))
	dict.SetKey(starlark.String("description"), starlark.String(g.Description))

	members := make([]starlark.Value, len(g.Members))
	for i, member := range g.Members {
		members[i] = starlark.String(member)
	}
	dict.SetKey(starlark.String("members"), starlark.NewList(members))
	dict.SetKey(starlark.String("attributes"), goMapToPluginDict(g.Attributes))

	return dict
}

func starlarkToSyncResult(v starlark.Value) (*SyncResult, error) {
	_, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("sync must return a dict")
	}

	result := &SyncResult{}

	// if val, found, _ := dict.Get(starlark.String("identities_created")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.IdentitiesCreated.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("identities_updated")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.IdentitiesUpdated.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("identities_deleted")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.IdentitiesDeleted.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("groups_created")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.GroupsCreated.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("groups_updated")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.GroupsUpdated.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("groups_deleted")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.GroupsDeleted.Store(i)
	// 	}
	// }

	// if val, found, _ := dict.Get(starlark.String("errors_count")); found {
	// 	if intVal, ok := val.(starlark.Int); ok {
	// 		i, _ := intVal.Uint64()
	// 		result.ErrCount.Store(i)
	// 	}
	// }

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
