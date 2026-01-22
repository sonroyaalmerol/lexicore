package config

import (
	"time"
)

// Config represents the main orchestrator configuration
type Config struct {
	// Server configuration
	Server ServerConfig `yaml:"server" json:"server"`

	// Manifests directory
	ManifestsDir string `yaml:"manifestsDir" json:"manifestsDir"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging" json:"logging"`

	// Metrics configuration
	Metrics MetricsConfig `yaml:"metrics" json:"metrics"`

	// Default sync period
	DefaultSyncPeriod time.Duration `yaml:"defaultSyncPeriod" json:"defaultSyncPeriod"`

	// Workers configuration
	Workers WorkersConfig `yaml:"workers" json:"workers"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	// HTTP server address
	Address string `yaml:"address" json:"address"`

	// Enable health checks
	HealthCheck bool `yaml:"healthCheck" json:"healthCheck"`

	// Enable metrics endpoint
	Metrics bool `yaml:"metrics" json:"metrics"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	// Log level (debug, info, warn, error)
	Level string `yaml:"level" json:"level"`

	// Log format (json, console)
	Format string `yaml:"format" json:"format"`

	// Output path (stdout, stderr, or file path)
	Output string `yaml:"output" json:"output"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	// Enable metrics collection
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Metrics port
	Port int `yaml:"port" json:"port"`

	// Metrics path
	Path string `yaml:"path" json:"path"`
}

// WorkersConfig holds worker configuration
type WorkersConfig struct {
	// Number of reconciliation workers
	ReconcileWorkers int `yaml:"reconcileWorkers" json:"reconcileWorkers"`

	// Worker queue size
	QueueSize int `yaml:"queueSize" json:"queueSize"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:     ":8080",
			HealthCheck: true,
			Metrics:     true,
		},
		ManifestsDir: "./manifests",
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
		DefaultSyncPeriod: 5 * time.Minute,
		Workers: WorkersConfig{
			ReconcileWorkers: 4,
			QueueSize:        100,
		},
	}
}
