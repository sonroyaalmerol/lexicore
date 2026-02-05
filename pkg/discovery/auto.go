package discovery

import (
	"go.uber.org/zap"
)

func Auto(logger *zap.Logger) (Discovery, error) {
	if k8s, err := NewK8sDiscovery(); err == nil {
		return k8s, nil
	}

	bindAddr := getEnvOrDefault("BIND_ADDR", "0.0.0.0")
	seeds := parseSeeds(getEnvOrDefault("SEED_ADDRS", ""))

	return NewGossipDiscovery(bindAddr, seeds, logger)
}
