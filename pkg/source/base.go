package source

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

type BaseSource struct {
	name   string
	config map[string]any
	mu     sync.RWMutex
	logger *zap.Logger
}

func NewBaseSource(name string, logger *zap.Logger) *BaseSource {
	return &BaseSource{
		name:   name,
		logger: logger,
	}
}

func (b *BaseSource) Name() string {
	return b.name
}

func (b *BaseSource) GetConfig(key string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.config[key]
	return val, ok
}

func (b *BaseSource) GetStringConfig(key string) (string, error) {
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

func (b *BaseSource) GetRawConfig() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.config
}

func (b *BaseSource) SetConfig(config map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}

func (b *BaseSource) LogInfo(s string, v ...any) {
	if b.logger != nil {
		b.logger.Info(fmt.Sprintf(s, v...), zap.String("source", b.name))
	}
}

func (b *BaseSource) LogWarn(s string, v ...any) {
	if b.logger != nil {
		b.logger.Warn(fmt.Sprintf(s, v...), zap.String("source", b.name))
	}
}

func (b *BaseSource) LogError(err error) {
	if err != nil && b.logger != nil {
		b.logger.Error(err.Error(), zap.String("source", b.name))
	}
}
