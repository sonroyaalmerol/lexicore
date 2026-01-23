package main

import (
	"context"
	"encoding/json"
	"flag"
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
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "/etc/lexicore/config.yaml", "Path to the configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = config.DefaultConfig()
		} else {
			panic("Failed to load config: " + err.Error())
		}
	}

	logger := initLogger(cfg.Logging)
	defer logger.Sync()

	logger.Info("Starting Lexicore Identity Orchestrator",
		zap.String("addr", cfg.Server.Address),
		zap.String("etcd_mode", getEtcdMode(cfg.Etcd)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var clientEndpoints []string
	if len(cfg.Etcd.Endpoints) > 0 {
		clientEndpoints = cfg.Etcd.Endpoints
		logger.Info("Connecting to external etcd cluster", zap.Strings("endpoints", clientEndpoints))
	} else {
		etcdServer := startEmbeddedHA(cfg.Etcd, logger)
		defer etcdServer.Close()
		clientEndpoints = []string{cfg.Etcd.ClientAddr}
	}

	db, err := store.NewEtcdStore(clientEndpoints, 5*time.Second)
	if err != nil {
		logger.Fatal("Failed to initialize etcd store", zap.Error(err))
	}

	mgr := controller.NewManager(cfg, logger)
	bootstrapStateFromStore(ctx, db, mgr, logger)

	go func() {
		logger.Info("Starting Controller Manager reconciliation loop")
		if err := mgr.Start(ctx); err != nil {
			logger.Error("Controller Manager exited with error", zap.Error(err))
		}
	}()

	mux := http.NewServeMux()
	setupRoutes(mux, ctx, db, mgr, logger)

	srv := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("API server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("Shutdown signal received. Cleaning up...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", zap.Error(err))
	}

	logger.Info("Lexicore shutdown complete")
}

func startEmbeddedHA(c config.EtcdConfig, logger *zap.Logger) *embed.Etcd {
	eCfg := embed.NewConfig()
	eCfg.Name = c.Name
	eCfg.Dir = c.DataDir

	curl, _ := url.Parse(c.ClientAddr)
	purl, _ := url.Parse(c.PeerAddr)

	eCfg.ListenClientUrls = []url.URL{*curl}
	eCfg.AdvertiseClientUrls = []url.URL{*curl}
	eCfg.ListenPeerUrls = []url.URL{*purl}
	eCfg.AdvertisePeerUrls = []url.URL{*purl}

	eCfg.InitialCluster = c.InitialCluster

	e, err := embed.StartEtcd(eCfg)
	if err != nil {
		logger.Fatal("Failed to start embedded etcd", zap.Error(err))
	}

	select {
	case <-e.Server.ReadyNotify():
		logger.Info("Embedded etcd is ready", zap.String("node", c.Name))
	case <-time.After(60 * time.Second):
		logger.Fatal("Embedded etcd startup timed out")
	}

	return e
}

func bootstrapStateFromStore(ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger) {
	sources, err := db.List(ctx, "identitysources")
	if err == nil {
		for _, data := range sources {
			var m manifest.IdentitySource
			if err := json.Unmarshal(data, &m); err == nil {
				src, err := source.Create(ctx, m.Spec.Type, m.Spec.Config)
				if err == nil {
					mgr.RegisterSource(m.Name, src)
					logger.Info("Restored IdentitySource", zap.String("name", m.Name))
				}
			}
		}
	}

	targets, err := db.List(ctx, "synctargets")
	if err == nil {
		for _, data := range targets {
			var m manifest.SyncTarget
			if err := json.Unmarshal(data, &m); err == nil {
				op, err := operator.Create(m.Spec.Operator)
				if err == nil {
					op.Initialize(ctx, m.Spec.Config)
					mgr.RegisterOperator(op)
					mgr.AddSyncTarget(&m)
					logger.Info("Restored SyncTarget", zap.String("name", m.Name))
				}
			}
		}
	}
}

func setupRoutes(mux *http.ServeMux, ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			renderJSON(w, http.StatusOK, mgr.GetIdentitySources())
			return
		}
		if r.Method == http.MethodPost {
			var m manifest.IdentitySource
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				logger.Error("Failed to decode IdentitySource JSON", zap.Error(err))
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "identitysources", m.Name, m); err != nil {
				logger.Error("Failed to persist IdentitySource to store", zap.String("name", m.Name), zap.Error(err))
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			src, err := source.Create(ctx, m.Spec.Type, m.Spec.Config)
			if err != nil {
				logger.Error("Failed to create IdentitySource driver", zap.String("name", m.Name), zap.Error(err))
				http.Error(w, "Failed to initialize driver", http.StatusInternalServerError)
				return
			}

			mgr.RegisterSource(m.Name, src)
			mgr.AddIdentitySource(&m)

			logger.Info("Created IdentitySource", zap.String("name", m.Name), zap.String("type", m.Spec.Type))
			renderJSON(w, http.StatusCreated, m)
			return
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			renderJSON(w, http.StatusOK, mgr.GetSyncTargets())
			return
		}
		if r.Method == http.MethodPost {
			var m manifest.SyncTarget
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				logger.Error("Failed to decode SyncTarget JSON", zap.Error(err))
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "synctargets", m.Name, m); err != nil {
				logger.Error("Failed to persist SyncTarget to store", zap.String("name", m.Name), zap.Error(err))
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			op, err := operator.Create(m.Spec.Operator)
			if err != nil {
				logger.Error("Operator not found", zap.String("operator", m.Spec.Operator), zap.Error(err))
				http.Error(w, "Unknown operator", http.StatusBadRequest)
				return
			}

			if err := op.Initialize(ctx, m.Spec.Config); err != nil {
				logger.Error("Failed to initialize operator", zap.String("operator", m.Spec.Operator), zap.Error(err))
				http.Error(w, "Operator initialization failed", http.StatusInternalServerError)
				return
			}

			mgr.RegisterOperator(op)
			if err := mgr.AddSyncTarget(&m); err != nil {
				logger.Error("Failed to add SyncTarget to manager", zap.String("name", m.Name), zap.Error(err))
				http.Error(w, "Target registration failed", http.StatusInternalServerError)
				return
			}

			logger.Info("Created SyncTarget", zap.String("name", m.Name), zap.String("operator", m.Spec.Operator))
			renderJSON(w, http.StatusCreated, m)
			return
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			name := strings.TrimPrefix(r.URL.Path, "/apis/lexicore.io/v1/identitysources/")
			if err := db.Delete(ctx, "identitysources", name); err != nil {
				logger.Error("Failed to delete IdentitySource from store", zap.String("name", name), zap.Error(err))
				http.Error(w, "Delete failed", http.StatusInternalServerError)
				return
			}
			mgr.RemoveIdentitySource(name)
			logger.Info("Deleted IdentitySource", zap.String("name", name))
			w.WriteHeader(http.StatusOK)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			name := strings.TrimPrefix(r.URL.Path, "/apis/lexicore.io/v1/synctargets/")
			if err := db.Delete(ctx, "synctargets", name); err != nil {
				logger.Error("Failed to delete SyncTarget from store", zap.String("name", name), zap.Error(err))
				http.Error(w, "Delete failed", http.StatusInternalServerError)
				return
			}
			mgr.RemoveSyncTarget(name)
			logger.Info("Deleted SyncTarget", zap.String("name", name))
			w.WriteHeader(http.StatusOK)
		}
	})
}

func initLogger(c config.LoggingConfig) *zap.Logger {
	level, _ := zapcore.ParseLevel(c.Level)
	var zapCfg zap.Config

	if c.Format == "console" {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	l, _ := zapCfg.Build()
	return l
}

func getEtcdMode(c config.EtcdConfig) string {
	if len(c.Endpoints) > 0 {
		return "external"
	}
	return "embedded-ha"
}

func renderJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
