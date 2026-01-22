package operator

import (
	"fmt"
	"sync"
)

type Registry struct {
	factories map[string]OperatorFactory
	mu        sync.RWMutex
}

type OperatorFactory func() Operator

var globalRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]OperatorFactory),
	}
}

func (r *Registry) Register(name string, factory OperatorFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("operator %s already registered", name)
	}

	r.factories[name] = factory
	return nil
}

func (r *Registry) Create(name string) (Operator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("operator %s not found", name)
	}

	return factory(), nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[name]
	return exists
}

func Register(name string, factory OperatorFactory) error {
	return globalRegistry.Register(name, factory)
}

func Create(name string) (Operator, error) {
	return globalRegistry.Create(name)
}

func List() []string {
	return globalRegistry.List()
}

func Has(name string) bool {
	return globalRegistry.Has(name)
}
