package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/api"
	"codeberg.org/lexicore/lexicore/pkg/config"
	"codeberg.org/lexicore/lexicore/pkg/controller"
	"codeberg.org/lexicore/lexicore/pkg/store"

	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		etcdServer = store.StartEmbeddedHA(cfg.Etcd, logger)
		defer etcdServer.Close()

		logger.Info("Starting auto-discovery of nodes")
		endpoints = store.GetEtcdEndpoints(cfg.Etcd, logger)
		logger.Info("Using embedded etcd",
			zap.Strings("endpoints", endpoints))
	}

	// Wait a moment for etcd to stabilize
	time.Sleep(2 * time.Second)

	if etcdServer != nil {
		snapshotMgr := store.NewSnapshotManager(
			cfg.Etcd.DataDir,
			filepath.Join(cfg.Etcd.DataDir, "snapshots"),
			7*24*time.Hour,
			logger,
		)

		go func() {
			ticker := time.NewTicker(6 * time.Hour)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					snapCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
					_, err := snapshotMgr.TakeSnapshot(snapCtx, endpoints[0])
					if err != nil {
						logger.Error("Failed to take snapshot", zap.Error(err))
					}
					cancel()

					if err := snapshotMgr.CleanupOldSnapshots(); err != nil {
						logger.Warn("Failed to cleanup old snapshots", zap.Error(err))
					}
				}
			}
		}()
	}

	db, err := store.NewEtcdStore(endpoints, 5*time.Second)
	if err != nil {
		logger.Fatal("Store init failed", zap.Error(err))
	}
	defer db.Close()

	mgr := controller.NewManager(ctx, cfg, db, logger)

	mux := http.NewServeMux()
	api.SetupRoutes(mux, ctx, db, mgr, logger)

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

	sCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(sCtx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	}

	logger.Info("Shutdown complete")
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
