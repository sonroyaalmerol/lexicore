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
	Audit                 AuditConfig   `yaml:"audits"`
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

type AuditConfig struct {
	Mode   string     `yaml:"mode" envconfig:"AUDIT_MODE"`
	XLSDir string     `yaml:"xlsDir" envconfig:"AUDIT_XLS_DIR"`
	Email  AuditEmail `yaml:"email" envconfig:"AUDIT_EMAIL"`
}

type AuditEmail struct {
	SMTP       SMTPConfig `yaml:"smtp"`
	From       string     `yaml:"from" envconfig:"AUDIT_EMAIL_FROM"`
	To         []string   `yaml:"to" envconfig:"AUDIT_EMAIL_TO"`
	SubjectFmt string     `yaml:"subjectFmt" envconfig:"AUDIT_EMAIL_SUBJECT_FMT"`
}

type SMTPConfig struct {
	Host     string `yaml:"host" envconfig:"SMTP_HOST"`
	Port     int    `yaml:"port" envconfig:"SMTP_PORT"`
	Username string `yaml:"username" envconfig:"SMTP_USERNAME"`
	Password string `yaml:"password" envconfig:"SMTP_PASSWORD"`
	TLS      bool   `yaml:"tls" envconfig:"SMTP_TLS"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:    ":8080",
			PluginsDir: "/var/lib/lexicore/plugins",
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
		Audit: AuditConfig{
			Mode:   "file",
			XLSDir: "/var/lib/lexicore/audits",
			Email: AuditEmail{
				SubjectFmt: "Lexicore Audit Report — %s",
				SMTP: SMTPConfig{
					Port: 587,
					TLS:  true,
				},
			},
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
