package operator

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/utils"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type BaseOperator struct {
	name    string
	config  map[string]any
	mu      sync.RWMutex
	logger  *zap.Logger
	limiter *rate.Limiter

	attrPrefix  *string
	limiterOnce sync.Once
}

func NewBaseOperator(name string, logger *zap.Logger) *BaseOperator {
	op := &BaseOperator{
		name:   name,
		logger: logger,
	}

	return op
}

func (b *BaseOperator) Name() string {
	return b.name
}

func (b *BaseOperator) GetConcurrency() int {
	workers := 10
	if w, ok := b.GetConfig("concurrency"); ok {
		if wInt, ok := w.(int); ok && wInt > 0 {
			workers = wInt
		} else if wFloat, ok := w.(float64); ok && wFloat > 0 {
			workers = int(wFloat)
		}
	}
	return workers
}

func (b *BaseOperator) GetLimiter() *rate.Limiter {
	b.limiterOnce.Do(func() {
		rateLimit := 50.0
		if rl, ok := b.GetConfig("rateLimit"); ok {
			if rlFloat, ok := rl.(float64); ok && rlFloat > 0 {
				rateLimit = rlFloat
			}
		}

		b.limiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))
	})

	return b.limiter
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

func (b *BaseOperator) GetTemplatedStringConfig(key string, attr map[string]any) (string, error) {
	str, err := b.GetStringConfig(key)
	if err != nil {
		return "", err
	}

	tmpl := template.New(key).
		Funcs(utils.CreateFuncMap()).
		Option("missingkey=zero")

	var buf bytes.Buffer

	parser, err := tmpl.Parse(str)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	err = parser.Execute(&buf, attr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	return buf.String(), nil
}

func (b *BaseOperator) SetConfig(config map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}

func (b *BaseOperator) LogInfo(s string, v ...any) {
	if b.logger != nil {
		b.logger.Info(fmt.Sprintf(s, v...), zap.String("operator", b.name))
	}
}

func (b *BaseOperator) LogWarn(s string, v ...any) {
	if b.logger != nil {
		b.logger.Warn(fmt.Sprintf(s, v...), zap.String("operator", b.name))
	}
}

func (b *BaseOperator) LogError(err error) {
	if err != nil && b.logger != nil {
		b.logger.Error(err.Error(), zap.String("operator", b.name))
	}
}

func (b *BaseOperator) PartialSync(ctx context.Context, state *PartialSyncState) error {
	return fmt.Errorf("partial sync not implemented, use full sync instead")
}

func (b *BaseOperator) ShouldSkipUnchangedSync() bool {
	skipUnchanged, ok := b.GetConfig("skipUnchangedSync")
	if !ok {
		return false
	}

	if boolVal, ok := skipUnchanged.(bool); ok {
		return boolVal
	}

	return false
}
