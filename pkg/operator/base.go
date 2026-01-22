package operator

import (
	"fmt"
	"sync"
)

type BaseOperator struct {
	name   string
	config map[string]any
	mu     sync.RWMutex
}

func NewBaseOperator(name string) *BaseOperator {
	return &BaseOperator{
		name: name,
	}
}

func (b *BaseOperator) Name() string {
	return b.name
}

func (b *BaseOperator) GetConfig(key string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.config[key]
	return val, ok
}

func (b *BaseOperator) GetStringConfig(key string) (string, error) {
	val, ok := b.GetConfig(key)
	if !ok {
		return "", fmt.Errorf("config key %s not found", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("config key %s is not a string", key)
	}
	return str, nil
}

func (b *BaseOperator) SetConfig(config map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}
