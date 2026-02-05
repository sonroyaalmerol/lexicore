package discovery

import (
	"context"
	"os"
	"strings"
)

type Discovery interface {
	GetPeers(ctx context.Context) ([]string, error)
	GetNodeName() string
	GetNodeIP() string
}

func Auto() (Discovery, error) {
	if k8s, err := NewK8sDiscovery(); err == nil {
		return k8s, nil
	}

	bindAddr := getEnvOrDefault("BIND_ADDR", "0.0.0.0")
	seeds := parseSeeds(getEnvOrDefault("SEED_ADDRS", ""))

	return NewGossipDiscovery(bindAddr, seeds)
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func parseSeeds(seedStr string) []string {
	if seedStr == "" {
		return []string{}
	}

	seeds := strings.Split(seedStr, ",")
	result := make([]string, 0, len(seeds))

	for _, seed := range seeds {
		trimmed := strings.TrimSpace(seed)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
