package discovery

import (
	"go.uber.org/zap"
)

func Auto(bindAddr string, seedAddrs []string, logger *zap.Logger) (Discovery, error) {
	if k8s, err := NewK8sDiscovery(); err == nil {
		return k8s, nil
	}

	return NewGossipDiscovery(bindAddr, seedAddrs, logger)
}
