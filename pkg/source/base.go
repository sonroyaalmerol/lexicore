package source

import (
	"fmt"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"go.uber.org/zap"
)

type BaseSource struct {
	name   string
	config map[string]manifest.ConfigValue
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

func (b *BaseSource) GetConfig(key string) (manifest.ConfigValue, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.config[key]
	return val, ok
}

func (b *BaseSource) GetStringConfig(key string) (string, error) {
	config, ok := b.GetConfig(key)
	if !ok || config.String() == "" {
		return "", fmt.Errorf("config not found: %s", key)
	}
	return config.String(), nil
}

func (b *BaseSource) GetRawConfig() map[string]manifest.ConfigValue {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.config
}

func (b *BaseSource) SetConfig(config map[string]manifest.ConfigValue) {
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
