package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/controller"
	"codeberg.org/lexicore/lexicore/pkg/discovery"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/store"

	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	_ "codeberg.org/lexicore/lexicore/pkg/operator/drivers/ad"
	_ "codeberg.org/lexicore/lexicore/pkg/operator/drivers/caldav"
	_ "codeberg.org/lexicore/lexicore/pkg/operator/drivers/dovecot"
	_ "codeberg.org/lexicore/lexicore/pkg/operator/drivers/iredadmin"
	_ "codeberg.org/lexicore/lexicore/pkg/operator/drivers/ldap"
	_ "codeberg.org/lexicore/lexicore/pkg/source/authentik"
	_ "codeberg.org/lexicore/lexicore/pkg/source/ldap"
)

func main() {
	configPath := flag.String("config", "/etc/lexicore/config.yaml", "Path to config")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = config.DefaultConfig()
		} else {
			panic(err)
		}
	}

	logger := initLogger(cfg.Logging)
	defer logger.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var etcdServer *embed.Etcd
	var endpoints []string

	if len(cfg.Etcd.Endpoints) > 0 {
		endpoints = cfg.Etcd.Endpoints
		logger.Info("Using external etcd cluster",
			zap.Strings("endpoints", endpoints))
	} else {
		etcdServer = startEmbeddedHA(cfg.Etcd, logger)
		defer etcdServer.Close()

		endpoints = getEtcdEndpoints(cfg.Etcd, logger)
	}

	// Wait a moment for etcd to stabilize
	time.Sleep(2 * time.Second)

	db, err := store.NewEtcdStore(endpoints, 5*time.Second)
	if err != nil {
		logger.Fatal("Store init failed", zap.Error(err))
	}
	defer db.Close()

	mgr := controller.NewManager(cfg, logger)

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	go runWatchLoop(watchCtx, db, mgr, logger, "identitysources")
	go runWatchLoop(watchCtx, db, mgr, logger, "synctargets")

	go runLeaderElection(ctx, db, mgr, cfg.Etcd.Name, logger)

	mux := http.NewServeMux()
	setupRoutes(mux, ctx, db, mgr, logger)

	srv := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("Starting HTTP server", zap.String("addr", cfg.Server.Address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down...")

	cancelWatch()

	sCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(sCtx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	}

	logger.Info("Shutdown complete")
}

func runWatchLoop(ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger, kind string) {
	logger = logger.With(zap.String("kind", kind))
	var rev int64 = 0

	for {
		select {
		case <-ctx.Done():
			logger.Info("Watch loop stopping")
			return
		default:
			wch := db.Watch(ctx, kind, rev)
			for resp := range wch {
				if resp.Canceled {
					if resp.Err() == rpctypes.ErrCompacted {
						rev = resp.CompactRevision
						logger.Warn("Watch compacted, resetting revision", zap.Int64("rev", rev))
					} else {
						logger.Error("Watch canceled", zap.Error(resp.Err()))
					}
					break
				}

				for _, ev := range resp.Events {
					rev = ev.Kv.ModRevision + 1
					handleStoreEvent(ctx, mgr, kind, ev, logger)
				}
			}

			// Check context before sleeping
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func handleStoreEvent(ctx context.Context, mgr *controller.Manager, kind string, ev *clientv3.Event, logger *zap.Logger) {
	parts := strings.Split(string(ev.Kv.Key), "/")
	name := parts[len(parts)-1]

	if ev.Type == mvccpb.DELETE {
		if kind == "identitysources" {
			mgr.RemoveIdentitySource(name)
		} else {
			mgr.RemoveSyncTarget(name)
		}
		logger.Info("Resource deleted", zap.String("kind", kind), zap.String("name", name))
		return
	}

	if kind == "identitysources" {
		var m manifest.IdentitySource
		if err := json.Unmarshal(ev.Kv.Value, &m); err != nil {
			logger.Error("Failed to unmarshal identity source", zap.String("name", name), zap.Error(err))
			return
		}

		src, err := source.Create(ctx, m.Spec.Type, m.Spec.Config)
		if err != nil {
			logger.Error("Failed to create source", zap.String("name", name), zap.Error(err))
			return
		}

		mgr.RegisterSource(m.Name, src)
		if err := mgr.AddIdentitySource(&m); err != nil {
			logger.Error("Failed to add identity source", zap.String("name", name), zap.Error(err))
			return
		}

		logger.Info("Identity source updated", zap.String("name", name))
	} else {
		var m manifest.SyncTarget
		if err := json.Unmarshal(ev.Kv.Value, &m); err != nil {
			logger.Error("Failed to unmarshal sync target", zap.String("name", name), zap.Error(err))
			return
		}

		op, err := operator.Create(m.Spec.Operator)
		if err != nil {
			logger.Error("Failed to create operator", zap.String("name", name), zap.Error(err))
			return
		}

		if err := op.Initialize(ctx, m.Spec.Config); err != nil {
			logger.Error("Failed to initialize operator", zap.String("name", name), zap.Error(err))
			return
		}

		mgr.RegisterOperator(op)
		if err := mgr.AddSyncTarget(&m); err != nil {
			logger.Error("Failed to add sync target", zap.String("name", name), zap.Error(err))
			return
		}

		logger.Info("Sync target updated", zap.String("name", name))
	}
}

func runLeaderElection(ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, nodeName string, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Leader election stopping")
			return
		default:
			session, err := concurrency.NewSession(db.Client(), concurrency.WithTTL(15))
			if err != nil {
				logger.Error("Election session failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			election := concurrency.NewElection(session, "/lexicore/leader")
			if err := election.Campaign(ctx, nodeName); err != nil {
				logger.Debug("Campaign failed, retrying", zap.Error(err))
				session.Close()
				time.Sleep(time.Second)
				continue
			}

			logger.Info("Node acquired leadership", zap.String("node", nodeName))

			runCtx, cancel := context.WithCancel(ctx)

			go func() {
				select {
				case <-session.Done():
					logger.Warn("Leader session expired, stopping reconciliation")
					cancel()
				case <-runCtx.Done():
				}
			}()

			if err := mgr.Start(runCtx); err != nil {
				logger.Error("Manager stopped with error", zap.Error(err))
			}

			cancel()
			session.Close()

			logger.Info("Leadership released")

			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
		}
	}
}

func setupRoutes(mux *http.ServeMux, ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger) {
	p := manifest.NewParser()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mgr.GetIdentitySources()); err != nil {
				logger.Error("Failed to encode response", zap.Error(err))
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}

		case http.MethodPost:
			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			m, err := p.Parse(b)
			if err != nil {
				logger.Error("Failed to parse manifest", zap.Error(err))
				http.Error(w, "Invalid manifest", http.StatusBadRequest)
				return
			}

			src, ok := m.(*manifest.IdentitySource)
			if !ok {
				http.Error(w, "Invalid manifest type", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "identitysources", src.Name, src); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": src.Name})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mgr.GetSyncTargets()); err != nil {
				logger.Error("Failed to encode response", zap.Error(err))
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}

		case http.MethodPost:
			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			m, err := p.Parse(b)
			if err != nil {
				logger.Error("Failed to parse manifest", zap.Error(err))
				http.Error(w, "Invalid manifest", http.StatusBadRequest)
				return
			}

			target, ok := m.(*manifest.SyncTarget)
			if !ok {
				http.Error(w, "Invalid manifest type", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "synctargets", target.Name, target); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": target.Name})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/apis/lexicore.io/v1/synctargets/")
		parts := strings.Split(path, "/")

		if len(parts) != 2 || parts[1] != "reconcile" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		targetName := parts[0]
		if targetName == "" {
			http.Error(w, "Target name required", http.StatusBadRequest)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual reconciliation triggered",
			zap.String("target", targetName),
			zap.String("remote_addr", r.RemoteAddr))

		if err := mgr.TriggerReconciliation(targetName); err != nil {
			logger.Error("Failed to trigger reconciliation",
				zap.String("target", targetName),
				zap.Error(err))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"error":  err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "queued",
			"target": targetName,
		})
	})

	mux.HandleFunc("/apis/lexicore.io/v1/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Bulk reconciliation triggered", zap.String("remote_addr", r.RemoteAddr))

		targets := mgr.GetSyncTargets()
		queued := 0
		failed := []string{}

		for _, target := range targets {
			if err := mgr.TriggerReconciliation(target.Name); err != nil {
				logger.Warn("Failed to queue target for reconciliation",
					zap.String("target", target.Name),
					zap.Error(err))
				failed = append(failed, target.Name)
			} else {
				queued++
			}
		}

		w.Header().Set("Content-Type", "application/json")

		if len(failed) == 0 {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "queued",
				"count":  queued,
			})
		} else {
			w.WriteHeader(http.StatusPartialContent)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "partial",
				"queued": queued,
				"failed": failed,
			})
		}
	})
}

func startEmbeddedHA(c config.EtcdConfig, logger *zap.Logger) *embed.Etcd {
	eCfg := embed.NewConfig()
	eCfg.Dir = c.DataDir
	eCfg.LogLevel = "warn" // Reduce etcd noise in logs

	var nodeName, nodeIP, initialCluster string
	var clusterState string

	if c.AutoJoin {
		var disco discovery.Discovery
		var err error

		switch c.Discovery {
		case "k8s":
			disco, err = discovery.NewK8sDiscovery()
		case "gossip":
			disco, err = discovery.NewGossipDiscovery(c.BindAddr, c.SeedAddrs)
		case "auto":
			disco, err = discovery.Auto()
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		peers, err := disco.GetPeers(ctx)
		if err != nil {
			logger.Warn("Failed to discover peers, starting as new cluster",
				zap.Error(err))
			peers = []string{}
		}

		selfPeer := fmt.Sprintf("%s=http://%s:2380", nodeName, nodeIP)

		if len(peers) > 0 {
			found := false
			for _, peer := range peers {
				if peer == selfPeer {
					found = true
					break
				}
			}

			if !found {
				peers = append(peers, selfPeer)
			}

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

		if g, ok := disco.(*discovery.GossipDiscovery); ok {
			defer g.Close()
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

	return e
}

func getEtcdEndpoints(cfg config.EtcdConfig, logger *zap.Logger) []string {
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

func initLogger(c config.LoggingConfig) *zap.Logger {
	lvl, _ := zapcore.ParseLevel(c.Level)
	cfg := zap.NewProductionConfig()
	if c.Format == "console" {
		cfg = zap.NewDevelopmentConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	l, _ := cfg.Build()
	return l
}
