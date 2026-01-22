package transformer

import (
	"context"
)

type Context struct {
	context.Context
	config map[string]any
	vars   map[string]any
}

func NewContext(ctx context.Context, config map[string]any) *Context {
	return &Context{
		Context: ctx,
		config:  config,
		vars:    make(map[string]any),
	}
}

func (c *Context) GetConfig(key string) (any, bool) {
	val, ok := c.config[key]
	return val, ok
}

func (c *Context) GetConfigString(key string) (string, bool) {
	val, ok := c.config[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

func (c *Context) GetConfigBool(key string) (bool, bool) {
	val, ok := c.config[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

func (c *Context) GetConfigMap(key string) (map[string]any, bool) {
	val, ok := c.config[key]
	if !ok {
		return nil, false
	}
	m, ok := val.(map[string]any)
	return m, ok
}

func (c *Context) SetVar(key string, value any) {
	c.vars[key] = value
}

func (c *Context) GetVar(key string) (any, bool) {
	val, ok := c.vars[key]
	return val, ok
}

func (c *Context) GetVarString(key string) (string, bool) {
	val, ok := c.vars[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}
