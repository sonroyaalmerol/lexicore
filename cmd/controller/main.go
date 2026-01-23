package main

import (
	"context"
	"encoding/json"
	"flag"
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

	var endpoints []string
	if len(cfg.Etcd.Endpoints) > 0 {
		endpoints = cfg.Etcd.Endpoints
		logger.Info("Using external etcd", zap.Strings("endpoints", endpoints))
	} else {
		e := startEmbeddedHA(cfg.Etcd, logger)
		defer e.Close()
		endpoints = []string{cfg.Etcd.ClientAddr}
		logger.Info("Using embedded etcd", zap.String("addr", cfg.Etcd.ClientAddr))
	}

	db, err := store.NewEtcdStore(endpoints, 5*time.Second)
	if err != nil {
		logger.Fatal("Store init failed", zap.Error(err))
	}

	mgr := controller.NewManager(cfg, logger)

	go runWatchLoop(ctx, db, mgr, logger, "identitysources")
	go runWatchLoop(ctx, db, mgr, logger, "synctargets")
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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	sCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(sCtx)
}

func runWatchLoop(ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger, kind string) {
	var rev int64 = 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			wch := db.Watch(ctx, kind, rev)
			for resp := range wch {
				if resp.Canceled {
					if resp.Err() == rpctypes.ErrCompacted {
						rev = resp.CompactRevision
						logger.Warn("Watch compacted, resetting revision", zap.Int64("rev", rev))
					}
					break
				}
				for _, ev := range resp.Events {
					rev = ev.Kv.ModRevision + 1
					handleStoreEvent(ctx, mgr, kind, ev, logger)
				}
			}
			time.Sleep(time.Second)
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
		if err := json.Unmarshal(ev.Kv.Value, &m); err == nil {
			if src, err := source.Create(ctx, m.Spec.Type, m.Spec.Config); err == nil {
				mgr.RegisterSource(m.Name, src)
				mgr.AddIdentitySource(&m)
				logger.Info("Resource updated", zap.String("kind", kind), zap.String("name", name))
			}
		}
	} else {
		var m manifest.SyncTarget
		if err := json.Unmarshal(ev.Kv.Value, &m); err != nil {
			if op, err := operator.Create(m.Spec.Operator); err == nil {
				op.Initialize(ctx, m.Spec.Config)
				mgr.RegisterOperator(op)
				mgr.AddSyncTarget(&m)
				logger.Info("Resource updated", zap.String("kind", kind), zap.String("name", name))
			}
		}
	}
}

func runLeaderElection(ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, nodeName string, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
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
				session.Close()
				continue
			}

			logger.Info("Node acquired leadership")
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
				logger.Error("Manager stopped", zap.Error(err))
			}
			session.Close()
		}
	}
}

func setupRoutes(mux *http.ServeMux, ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger) {
	p := manifest.NewParser()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mgr.GetIdentitySources())
			return
		}
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			m, err := p.Parse(b)
			if err != nil {
				http.Error(w, "Invalid manifest", 400)
				return
			}
			src := m.(*manifest.IdentitySource)
			if err := db.Put(ctx, "identitysources", src.Name, src); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", 500)
				return
			}
			w.WriteHeader(201)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mgr.GetSyncTargets())
			return
		}
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			m, err := p.Parse(b)
			if err != nil {
				http.Error(w, "Invalid manifest", 400)
				return
			}
			target := m.(*manifest.SyncTarget)
			if err := db.Put(ctx, "synctargets", target.Name, target); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", 500)
				return
			}
			w.WriteHeader(201)
		}
	})
}

func startEmbeddedHA(c config.EtcdConfig, logger *zap.Logger) *embed.Etcd {
	eCfg := embed.NewConfig()
	eCfg.Name, eCfg.Dir = c.Name, c.DataDir
	cu, _ := url.Parse(c.ClientAddr)
	pu, _ := url.Parse(c.PeerAddr)
	eCfg.ListenClientUrls, eCfg.AdvertiseClientUrls = []url.URL{*cu}, []url.URL{*cu}
	eCfg.ListenPeerUrls, eCfg.AdvertisePeerUrls = []url.URL{*pu}, []url.URL{*pu}
	eCfg.InitialCluster = c.InitialCluster
	e, err := embed.StartEtcd(eCfg)
	if err != nil {
		logger.Fatal("Etcd start fail", zap.Error(err))
	}
	<-e.Server.ReadyNotify()
	return e
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
