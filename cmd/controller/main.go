package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/goccy/go-yaml"

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
			if err := writeDefaultConfig(*configPath, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write default config: %v\n", err)
			} else {
				fmt.Printf("Generated default config at %s\n", *configPath)
			}
		} else {
			panic(err)
		}
	}

	logger := initLogger(cfg.Logging)
	defer logger.Sync()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	db, err := initStore(cfg)
	if err != nil {
		logger.Fatal("Failed to initialize store", zap.Error(err))
	}

	mgr := controller.NewManager(ctx, cfg, db, logger)

	mux := http.NewServeMux()
	api.SetupRoutes(mux, ctx, mgr, logger)

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

	mgr.Start(ctx)
	<-ctx.Done()
	logger.Info("Shutting down...")

	sCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(sCtx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	}

	logger.Info("Shutdown complete")
}

func writeDefaultConfig(path string, cfg *config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return yaml.NewEncoder(f).Encode(cfg)
}

func initStore(cfg *config.Config) (store.Store, error) {
	switch cfg.Store.Mode {
	case "file":
		return store.NewFileStore(cfg.Store.File.Dir), nil
	case "git":
		return store.NewGitStore(
			cfg.Store.Git.RepoURL,
			cfg.Store.Git.Branch,
			cfg.Store.Git.LocalDir,
			cfg.Store.Git.Username,
			cfg.Store.Git.Password,
			cfg.Store.Git.PollInterval,
		)
	default:
		return nil, fmt.Errorf("unknown store mode: %q", cfg.Store.Mode)
	}
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
