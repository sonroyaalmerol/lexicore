package config

import (
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server                ServerConfig  `yaml:"server"`
	Logging               LoggingConfig `yaml:"logging"`
	Metrics               MetricsConfig `yaml:"metrics"`
	Store                 StoreConfig   `yaml:"store"`
	DefaultSyncPeriod     time.Duration `yaml:"defaultSyncPeriod" envconfig:"DEFAULT_SYNC_PERIOD"`
	Workers               WorkersConfig `yaml:"workers"`
	WebhookDebounceWindow time.Duration `yaml:"webhookDebounceWindow"`
}

type StoreConfig struct {
	Mode string     `yaml:"mode" envconfig:"STORE_MODE"`
	File FileConfig `yaml:"file"`
	Git  GitConfig  `yaml:"git"`
}

type FileConfig struct {
	Dir string `yaml:"dir" envconfig:"STORE_FILE_DIR"`
}

type GitConfig struct {
	RepoURL      string        `yaml:"repoURL" envconfig:"STORE_GIT_REPO_URL"`
	Branch       string        `yaml:"branch" envconfig:"STORE_GIT_BRANCH"`
	LocalDir     string        `yaml:"localDir" envconfig:"STORE_GIT_LOCAL_DIR"`
	Username     string        `yaml:"username" envconfig:"STORE_GIT_USERNAME"`
	Password     string        `yaml:"password" envconfig:"STORE_GIT_PASSWORD"`
	PollInterval time.Duration `yaml:"pollInterval" envconfig:"STORE_GIT_POLL_INTERVAL"`
}

type ServerConfig struct {
	Address    string `yaml:"address" envconfig:"ADDRESS"`
	PluginsDir string `yaml:"pluginsDir" envconfig:"PLUGINS_DIR"`
	AuditsDir  string `yaml:"auditsDir" envconfig:"AUDITS_DIR"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" envconfig:"LEVEL"`
	Format string `yaml:"format" envconfig:"FORMAT"`
	Output string `yaml:"output" envconfig:"OUTPUT"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" envconfig:"ENABLED"`
	Port    int    `yaml:"port" envconfig:"PORT"`
	Path    string `yaml:"path" envconfig:"PATH"`
}

type WorkersConfig struct {
	ReconcileWorkers int `yaml:"reconcileWorkers" envconfig:"RECONCILE_WORKERS"`
	QueueSize        int `yaml:"queueSize" envconfig:"QUEUE_SIZE"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:    ":8080",
			PluginsDir: "/var/lib/lexicore/plugins",
			AuditsDir:  "/var/lib/lexicore/audits",
		},
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
		Store: StoreConfig{
			Mode: "file",
			File: FileConfig{
				Dir: "/etc/lexicore/manifests",
			},
			Git: GitConfig{
				Branch:       "main",
				LocalDir:     "/var/lib/lexicore/git",
				PollInterval: 1 * time.Minute,
			},
		},
		DefaultSyncPeriod: 5 * time.Minute,
		Workers: WorkersConfig{
			ReconcileWorkers: 4,
			QueueSize:        100,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
			return nil, err
		}
	}

	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
