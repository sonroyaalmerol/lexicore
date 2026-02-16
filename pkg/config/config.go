package config

import (
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server                ServerConfig  `yaml:"server" json:"server"`
	Logging               LoggingConfig `yaml:"logging" json:"logging"`
	Metrics               MetricsConfig `yaml:"metrics" json:"metrics"`
	Etcd                  EtcdConfig    `yaml:"etcd" json:"etcd"`
	DefaultSyncPeriod     time.Duration `yaml:"defaultSyncPeriod" json:"defaultSyncPeriod" envconfig:"DEFAULT_SYNC_PERIOD"`
	Workers               WorkersConfig `yaml:"workers" json:"workers"`
	WebhookDebounceWindow time.Duration `yaml:"webhookDebounceWindow"`
}

type ServerConfig struct {
	Address     string `yaml:"address" json:"address" envconfig:"ADDRESS"`
	HealthCheck bool   `yaml:"healthCheck" json:"healthCheck" envconfig:"HEALTH_CHECK"`
	Metrics     bool   `yaml:"metrics" json:"metrics" envconfig:"METRICS"`
	PluginsDir  string `yaml:"pluginsDir" json:"pluginsDir" envconfig:"PLUGINS_DIR"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" json:"level" envconfig:"LEVEL"`
	Format string `yaml:"format" json:"format" envconfig:"FORMAT"`
	Output string `yaml:"output" json:"output" envconfig:"OUTPUT"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled" envconfig:"ENABLED"`
	Port    int    `yaml:"port" json:"port" envconfig:"PORT"`
	Path    string `yaml:"path" json:"path" envconfig:"PATH"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints" json:"endpoints" envconfig:"ENDPOINTS"`

	DataDir   string `yaml:"dataDir" json:"dataDir" envconfig:"DATA_DIR"`
	AutoJoin  bool   `yaml:"autoJoin" json:"autoJoin" envconfig:"AUTO_JOIN"`
	Discovery string `yaml:"discovery" json:"discovery" envconfig:"DISCOVERY"`

	// Static/manual configuration (used when AutoJoin=false or Discovery="static")
	Name           string `yaml:"name" json:"name" envconfig:"NAME"`
	PeerAddr       string `yaml:"peerAddr" json:"peerAddr" envconfig:"PEER_ADDR"`
	ClientAddr     string `yaml:"clientAddr" json:"clientAddr" envconfig:"CLIENT_ADDR"`
	InitialCluster string `yaml:"initialCluster" json:"initialCluster" envconfig:"INITIAL_CLUSTER"`

	BindAddr  string   `yaml:"bindAddr" json:"bindAddr" envconfig:"BIND_ADDR"`
	SeedAddrs []string `yaml:"seedAddrs" json:"seedAddrs" envconfig:"SEED_ADDRS"`
}

type WorkersConfig struct {
	ReconcileWorkers int `yaml:"reconcileWorkers" json:"reconcileWorkers" envconfig:"RECONCILE_WORKERS"`
	QueueSize        int `yaml:"queueSize" json:"queueSize" envconfig:"QUEUE_SIZE"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:     ":8080",
			HealthCheck: true,
			Metrics:     true,
			PluginsDir:  "/var/lib/lexicore/plugins",
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
		Etcd: EtcdConfig{
			Endpoints:      []string{}, // Empty means use embedded
			DataDir:        "/var/lib/lexicore/etcd",
			AutoJoin:       false,
			Discovery:      "static",
			Name:           "node-1",
			PeerAddr:       "http://localhost:2380",
			ClientAddr:     "http://localhost:2379",
			InitialCluster: "node-1=http://localhost:2380",
			BindAddr:       "0.0.0.0",
			SeedAddrs:      []string{},
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
