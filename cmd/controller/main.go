package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codeberg.org/lexicore/lexicore/pkg/controller"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/source/ldap"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration")
	manifestsPath := flag.String(
		"manifests",
		"./manifests",
		"Path to manifests directory",
	)
	flag.Parse()

	// Setup logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Create controller manager
	mgr := controller.NewManager(logger)

	// Register operators
	mgr.RegisterOperator(dovecot.NewDovecotOperator())
	// Register more operators...

	// Load and register sources from manifests
	sources, err := loadSources(*manifestsPath)
	if err != nil {
		logger.Fatal("Failed to load sources", zap.Error(err))
	}

	ctx := context.Background()
	for name, sourceManifest := range sources {
		src, err := createSource(ctx, sourceManifest)
		if err != nil {
			logger.Fatal(
				"Failed to create source",
				zap.String("name", name),
				zap.Error(err),
			)
		}
		mgr.RegisterSource(name, src)
	}

	// Load and add sync targets
	targets, err := loadSyncTargets(*manifestsPath)
	if err != nil {
		logger.Fatal("Failed to load sync targets", zap.Error(err))
	}

	for _, target := range targets {
		if err := mgr.AddSyncTarget(target); err != nil {
			logger.Fatal(
				"Failed to add sync target",
				zap.String("name", target.Name),
				zap.Error(err),
			)
		}
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Start manager
	if err := mgr.Start(ctx); err != nil {
		logger.Fatal("Manager failed", zap.Error(err))
	}
}

func createSource(
	ctx context.Context,
	manifest *manifest.IdentitySource,
) (source.Source, error) {
	switch manifest.Spec.Type {
	case "ldap":
		// Parse config into ldap.Config
		config := &ldap.Config{
			// ... parse from manifest.Spec.Config
		}
		src := ldap.NewLDAPSource(config)
		if err := src.Connect(ctx); err != nil {
			return nil, err
		}
		return src, nil
	default:
		return nil, fmt.Errorf("unknown source type: %s", manifest.Spec.Type)
	}
}

func loadConfig(path string) (*Config, error) {
	// Load main configuration
	return nil, nil
}

func loadSources(path string) (map[string]*manifest.IdentitySource, error) {
	// Load source manifests from directory
	return nil, nil
}

func loadSyncTargets(path string) ([]*manifest.SyncTarget, error) {
	// Load sync target manifests from directory
	return nil, nil
}
