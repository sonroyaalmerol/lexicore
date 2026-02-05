package config

import (
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Server            ServerConfig  `yaml:"server" json:"server"`
	Logging           LoggingConfig `yaml:"logging" json:"logging"`
	Metrics           MetricsConfig `yaml:"metrics" json:"metrics"`
	Etcd              EtcdConfig    `yaml:"etcd" json:"etcd"`
	DefaultSyncPeriod time.Duration `yaml:"defaultSyncPeriod" json:"defaultSyncPeriod"`
	Workers           WorkersConfig `yaml:"workers" json:"workers"`
}

type ServerConfig struct {
	Address     string `yaml:"address" json:"address"`
	HealthCheck bool   `yaml:"healthCheck" json:"healthCheck"`
	Metrics     bool   `yaml:"metrics" json:"metrics"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
	Output string `yaml:"output" json:"output"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Port    int    `yaml:"port" json:"port"`
	Path    string `yaml:"path" json:"path"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints" json:"endpoints"`

	DataDir   string `yaml:"dataDir" json:"dataDir"`
	AutoJoin  bool   `yaml:"autoJoin" json:"autoJoin"`
	Discovery string `yaml:"discovery" json:"discovery"` // "auto", "k8s", "gossip", "static"

	// Static/manual configuration (used when AutoJoin=false or Discovery="static")
	Name           string `yaml:"name" json:"name"`
	PeerAddr       string `yaml:"peerAddr" json:"peerAddr"`
	ClientAddr     string `yaml:"clientAddr" json:"clientAddr"`
	InitialCluster string `yaml:"initialCluster" json:"initialCluster"`

	BindAddr  string   `yaml:"bindAddr" json:"bindAddr"`
	SeedAddrs []string `yaml:"seedAddrs" json:"seedAddrs"`
}

type WorkersConfig struct {
	ReconcileWorkers int `yaml:"reconcileWorkers" json:"reconcileWorkers"`
	QueueSize        int `yaml:"queueSize" json:"queueSize"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:     ":8080",
			HealthCheck: true,
			Metrics:     true,
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
			PeerAddr:       "http://0.0.0.0:2380",
			ClientAddr:     "http://0.0.0.0:2379",
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
	if path == "" {
		return cfg, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = yaml.NewDecoder(f).Decode(cfg)
	return cfg, err
}
