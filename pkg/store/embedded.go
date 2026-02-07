package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/discovery"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
)

func StartEmbeddedHA(c config.EtcdConfig, logger *zap.Logger) *embed.Etcd {
	eCfg := embed.NewConfig()
	eCfg.Dir = c.DataDir
	eCfg.LogLevel = "warn"

	var nodeName, nodeIP, initialCluster string
	var clusterState string
	var disco discovery.Discovery

	if c.AutoJoin {
		var err error

		switch c.Discovery {
		case "k8s":
			disco, err = discovery.NewK8sDiscovery()
		case "gossip":
			disco, err = discovery.NewGossipDiscovery(c.BindAddr, c.SeedAddrs, logger)
		case "auto":
			disco, err = discovery.Auto(logger)
		default:
			logger.Fatal("Invalid discovery mode",
				zap.String("mode", c.Discovery))
		}

		if err != nil {
			logger.Fatal("Discovery initialization failed",
				zap.String("mode", c.Discovery),
				zap.Error(err))
		}

		nodeName = disco.GetNodeName()
		nodeIP = disco.GetNodeIP()

		// Give gossip time to discover peers
		time.Sleep(2 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		peers, err := disco.GetPeers(ctx)
		cancel()

		if err != nil {
			logger.Warn("Failed to discover peers, starting as new cluster",
				zap.Error(err))
			peers = []string{}
		}

		selfPeer := fmt.Sprintf("%s=http://%s:2380", nodeName, nodeIP)
		existingCluster := shouldJoinExisting(selfPeer, peers, c.DataDir, logger)

		if existingCluster {
			initialCluster = strings.Join(peers, ",")
			clusterState = "existing"

			logger.Info("Joining existing cluster",
				zap.String("node", nodeName),
				zap.String("ip", nodeIP),
				zap.Int("peers", len(peers)),
				zap.String("cluster", initialCluster))
		} else {
			initialCluster = selfPeer
			clusterState = "new"

			logger.Info("Starting new cluster",
				zap.String("node", nodeName),
				zap.String("ip", nodeIP))
		}
	} else {
		nodeName = c.Name
		initialCluster = c.InitialCluster
		clusterState = "new"

		logger.Info("Starting with static configuration",
			zap.String("node", nodeName),
			zap.String("cluster", initialCluster))
	}

	eCfg.Name = nodeName

	if c.AutoJoin {
		clientURL := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:2379", nodeIP)}
		peerURL := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:2380", nodeIP)}

		eCfg.ListenClientUrls = []url.URL{clientURL}
		eCfg.AdvertiseClientUrls = []url.URL{clientURL}
		eCfg.ListenPeerUrls = []url.URL{peerURL}
		eCfg.AdvertisePeerUrls = []url.URL{peerURL}
	} else {
		cu, err := url.Parse(c.ClientAddr)
		if err != nil {
			logger.Fatal("Invalid client URL", zap.Error(err))
		}

		pu, err := url.Parse(c.PeerAddr)
		if err != nil {
			logger.Fatal("Invalid peer URL", zap.Error(err))
		}

		eCfg.ListenClientUrls = []url.URL{*cu}
		eCfg.AdvertiseClientUrls = []url.URL{*cu}
		eCfg.ListenPeerUrls = []url.URL{*pu}
		eCfg.AdvertisePeerUrls = []url.URL{*pu}
	}

	eCfg.InitialCluster = initialCluster
	eCfg.ClusterState = clusterState

	eCfg.MaxSnapFiles = 5
	eCfg.MaxWalFiles = 5
	eCfg.SnapshotCount = 10000
	eCfg.AutoCompactionRetention = "1h"
	eCfg.AutoCompactionMode = "periodic"

	logger.Info("Starting embedded etcd",
		zap.String("name", eCfg.Name),
		zap.String("data_dir", eCfg.Dir),
		zap.String("client_urls", eCfg.ListenClientUrls[0].String()),
		zap.String("peer_urls", eCfg.ListenPeerUrls[0].String()),
		zap.String("initial_cluster", eCfg.InitialCluster),
		zap.String("cluster_state", eCfg.ClusterState))

	e, err := embed.StartEtcd(eCfg)
	if err != nil {
		logger.Fatal("Etcd start failed", zap.Error(err))
	}

	select {
	case <-e.Server.ReadyNotify():
		logger.Info("Embedded etcd ready",
			zap.String("node", eCfg.Name),
			zap.String("cluster_state", clusterState))
	case <-time.After(60 * time.Second):
		e.Close()
		logger.Fatal("Etcd failed to become ready within timeout")
	case <-e.Server.StopNotify():
		logger.Fatal("Etcd stopped unexpectedly during startup")
	}

	if c.AutoJoin {
		if dynDisco, ok := disco.(discovery.DynamicDiscovery); ok {
			mgr, err := discovery.NewEtcdMembershipManager(e, dynDisco, nodeName, logger)
			if err != nil {
				logger.Fatal("Failed to create membership manager", zap.Error(err))
			}

			ctx := context.Background()
			if err := mgr.Start(ctx); err != nil {
				logger.Fatal("Failed to start membership manager", zap.Error(err))
			}
		}
	}

	return e
}

func shouldJoinExisting(selfPeer string, peers []string, dataDir string, logger *zap.Logger) bool {
	memberDir := filepath.Join(dataDir, "member")
	if info, err := os.Stat(memberDir); err == nil && info.IsDir() {
		logger.Info("Found existing member data, restarting")
		return true
	}

	otherPeers := 0
	for _, peer := range peers {
		if peer != selfPeer {
			otherPeers++
		}
	}

	return otherPeers > 0
}

func GetEtcdEndpoints(cfg config.EtcdConfig, logger *zap.Logger) []string {
	if len(cfg.Endpoints) > 0 {
		logger.Info("Using external etcd endpoints",
			zap.Strings("endpoints", cfg.Endpoints))
		return cfg.Endpoints
	}

	if cfg.AutoJoin {
		nodeIP := os.Getenv("POD_IP")
		if nodeIP == "" {
			nodeIP = os.Getenv("NODE_IP")
		}
		if nodeIP == "" {
			nodeIP = "localhost"
		}
		endpoint := fmt.Sprintf("http://%s:2379", nodeIP)
		logger.Info("Using embedded etcd endpoint",
			zap.String("endpoint", endpoint))
		return []string{endpoint}
	}

	cu, err := url.Parse(cfg.ClientAddr)
	if err != nil {
		logger.Fatal("Invalid client URL", zap.Error(err))
	}

	endpoint := cu.String()
	logger.Info("Using static etcd endpoint",
		zap.String("endpoint", endpoint))
	return []string{endpoint}
}
