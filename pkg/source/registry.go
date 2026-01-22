package source

import (
	"context"
	"fmt"
	"sync"
)

type Registry struct {
	factories map[string]SourceFactory
	mu        sync.RWMutex
}

type SourceFactory func(config map[string]any) (Source, error)

var globalRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]SourceFactory),
	}
}

func (r *Registry) Register(sourceType string, factory SourceFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[sourceType]; exists {
		return fmt.Errorf("source type %s already registered", sourceType)
	}

	r.factories[sourceType] = factory
	return nil
}

func (r *Registry) Create(
	ctx context.Context,
	sourceType string,
	config map[string]any,
) (Source, error) {
	r.mu.RLock()
	factory, exists := r.factories[sourceType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("source type %s not found", sourceType)
	}

	source, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	if err := source.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to source: %w", err)
	}

	return source, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for sourceType := range r.factories {
		types = append(types, sourceType)
	}
	return types
}

func (r *Registry) Has(sourceType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[sourceType]
	return exists
}

func Register(sourceType string, factory SourceFactory) error {
	return globalRegistry.Register(sourceType, factory)
}

func Create(
	ctx context.Context,
	sourceType string,
	config map[string]any,
) (Source, error) {
	return globalRegistry.Create(ctx, sourceType, config)
}

func List() []string {
	return globalRegistry.List()
}

func Has(sourceType string) bool {
	return globalRegistry.Has(sourceType)
}
