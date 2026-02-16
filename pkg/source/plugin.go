package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	starlarklib "codeberg.org/lexicore/lexicore/pkg/plugin"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"go.uber.org/zap"
)

type PluginSource struct {
	*BaseSource
	thread  *starlark.Thread
	globals starlark.StringDict
	module  *starlarkstruct.Module
}

func NewPluginSource(scriptPath string, logger *zap.Logger) (*PluginSource, error) {
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

	requiredFuncs := []string{"initialize", "validate", "get_identities", "get_groups"}
	for _, funcName := range requiredFuncs {
		if _, ok := globals[funcName]; !ok {
			return nil, fmt.Errorf("starlark script missing required function: %s", funcName)
		}
	}

	sourceName := name
	if nameVal, ok := globals["name"]; ok {
		if nameStr, ok := nameVal.(starlark.String); ok {
			sourceName = string(nameStr)
		}
	}

	return &PluginSource{
		BaseSource: NewBaseSource(sourceName, logger),
		thread:     thread,
		globals:    globals,
	}, nil
}

func (s *PluginSource) Initialize(ctx context.Context, config map[string]any) error {
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

func (s *PluginSource) Validate(ctx context.Context) error {
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

func (s *PluginSource) Connect(ctx context.Context) error {
	connectFunc, ok := s.globals["connect"]
	if !ok {
		// Connect is optional, return nil if not implemented
		return nil
	}

	callable, ok := connectFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("connect is not callable")
	}

	configDict := goMapToStarlarkDict(s.config)
	args := starlark.Tuple{configDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	if errStr := extractError(result); errStr != "" {
		return fmt.Errorf("%s", errStr)
	}

	return nil
}

func (s *PluginSource) GetIdentities(ctx context.Context) (map[string]Identity, error) {
	getIdentitiesFunc, ok := s.globals["get_identities"]
	if !ok {
		return nil, fmt.Errorf("get_identities function not found")
	}

	callable, ok := getIdentitiesFunc.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("get_identities is not callable")
	}

	configDict := goMapToStarlarkDict(s.config)
	args := starlark.Tuple{configDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return nil, fmt.Errorf("get_identities failed: %w", err)
	}

	identities, err := starlarkToIdentities(result)
	if err != nil {
		return nil, err
	}

	return identities, nil
}

func (s *PluginSource) GetGroups(ctx context.Context) (map[string]Group, error) {
	getGroupsFunc, ok := s.globals["get_groups"]
	if !ok {
		return nil, fmt.Errorf("get_groups function not found")
	}

	callable, ok := getGroupsFunc.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("get_groups is not callable")
	}

	configDict := goMapToStarlarkDict(s.config)
	args := starlark.Tuple{configDict}

	result, err := starlark.Call(s.thread, callable, args, nil)
	if err != nil {
		return nil, fmt.Errorf("get_groups failed: %w", err)
	}

	groups, err := starlarkToGroups(result)
	if err != nil {
		return nil, err
	}

	return groups, nil
}

func (s *PluginSource) Close() error {
	closeFunc, ok := s.globals["close"]
	if !ok {
		// Close is optional
		return nil
	}

	callable, ok := closeFunc.(starlark.Callable)
	if !ok {
		return fmt.Errorf("close is not callable")
	}

	result, err := starlark.Call(s.thread, callable, nil, nil)
	if err != nil {
		return fmt.Errorf("close failed: %w", err)
	}

	if errStr := extractError(result); errStr != "" {
		return fmt.Errorf("%s", errStr)
	}

	return nil
}

// Helper functions

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

func starlarkToIdentities(v starlark.Value) (map[string]Identity, error) {
	dict, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("get_identities must return a dict")
	}

	identities := make(map[string]Identity)
	for _, item := range dict.Items() {
		uid := string(item[0].(starlark.String))
		identity, err := starlarkToIdentity(item[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse identity %s: %w", uid, err)
		}
		identities[uid] = *identity
	}

	return identities, nil
}

func starlarkToGroups(v starlark.Value) (map[string]Group, error) {
	dict, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("get_groups must return a dict")
	}

	groups := make(map[string]Group)
	for _, item := range dict.Items() {
		gid := string(item[0].(starlark.String))
		group, err := starlarkToGroup(item[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse group %s: %w", gid, err)
		}
		groups[gid] = *group
	}

	return groups, nil
}

func starlarkToIdentity(v starlark.Value) (*Identity, error) {
	dict, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("identity must be a dict")
	}

	identity := &Identity{
		Attributes: make(map[string]any),
	}

	if val, found, _ := dict.Get(starlark.String("uid")); found {
		identity.UID = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("username")); found {
		identity.Username = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("email")); found {
		identity.Email = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("display_name")); found {
		identity.DisplayName = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("disabled")); found {
		identity.Disabled = bool(val.(starlark.Bool))
	}

	if val, found, _ := dict.Get(starlark.String("groups")); found {
		if list, ok := val.(*starlark.List); ok {
			identity.Groups = make([]string, list.Len())
			for i := 0; i < list.Len(); i++ {
				identity.Groups[i] = string(list.Index(i).(starlark.String))
			}
		}
	}

	if val, found, _ := dict.Get(starlark.String("attributes")); found {
		if attrDict, ok := val.(*starlark.Dict); ok {
			identity.Attributes = starlarkDictToGoMap(attrDict)
		}
	}

	return identity, nil
}

func starlarkToGroup(v starlark.Value) (*Group, error) {
	dict, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("group must be a dict")
	}

	group := &Group{
		Attributes: make(map[string]any),
	}

	if val, found, _ := dict.Get(starlark.String("gid")); found {
		group.GID = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("name")); found {
		group.Name = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("description")); found {
		group.Description = string(val.(starlark.String))
	}

	if val, found, _ := dict.Get(starlark.String("members")); found {
		if list, ok := val.(*starlark.List); ok {
			group.Members = make([]string, list.Len())
			for i := 0; i < list.Len(); i++ {
				group.Members[i] = string(list.Index(i).(starlark.String))
			}
		}
	}

	if val, found, _ := dict.Get(starlark.String("attributes")); found {
		if attrDict, ok := val.(*starlark.Dict); ok {
			group.Attributes = starlarkDictToGoMap(attrDict)
		}
	}

	return group, nil
}

func starlarkDictToGoMap(dict *starlark.Dict) map[string]any {
	result := make(map[string]any)
	for _, item := range dict.Items() {
		key := string(item[0].(starlark.String))
		result[key] = starlarkValueToGo(item[1])
	}
	return result
}

func starlarkValueToGo(v starlark.Value) any {
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
			result[i] = starlarkValueToGo(val.Index(i))
		}
		return result
	case *starlark.Dict:
		return starlarkDictToGoMap(val)
	case starlark.NoneType:
		return nil
	default:
		return fmt.Sprintf("%v", v)
	}
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
