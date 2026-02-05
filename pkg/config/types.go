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
	Name        string `yaml:"name" json:"name"`
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
	// External etcd endpoints
	Endpoints []string `yaml:"endpoints" json:"endpoints"`

	// Kubernetes service discovery
	UseKubernetesDiscovery bool   `yaml:"useKubernetesDiscovery" json:"useKubernetesDiscovery"`
	KubernetesServiceName  string `yaml:"kubernetesServiceName" json:"kubernetesServiceName"`
	KubernetesNamespace    string `yaml:"kubernetesNamespace" json:"kubernetesNamespace"`

	// Embedded etcd (for manual deployment)
	DataDir        string `yaml:"dataDir" json:"dataDir"`
	Name           string `yaml:"name" json:"name"`
	PeerAddr       string `yaml:"peerAddr" json:"peerAddr"`
	ClientAddr     string `yaml:"clientAddr" json:"clientAddr"`
	InitialCluster string `yaml:"initialCluster" json:"initialCluster"`
}

type WorkersConfig struct {
	ReconcileWorkers int `yaml:"reconcileWorkers" json:"reconcileWorkers"`
	QueueSize        int `yaml:"queueSize" json:"queueSize"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:     ":8080",
			Name:        "lexicore",
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
			// Auto-detect Kubernetes
			UseKubernetesDiscovery: true,
			KubernetesServiceName:  "lexicore-etcd",
			KubernetesNamespace:    "default",
			// Fallback to embedded
			DataDir:        "lexicore.etcd",
			Name:           "node-1",
			PeerAddr:       "http://localhost:2380",
			ClientAddr:     "http://localhost:2379",
			InitialCluster: "node-1=http://localhost:2380",
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
