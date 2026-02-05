package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/snapshot"
	"go.uber.org/zap"
)

type SnapshotManager struct {
	dataDir   string
	backupDir string
	retention time.Duration
	logger    *zap.Logger
}

func NewSnapshotManager(dataDir, backupDir string, retention time.Duration, logger *zap.Logger) *SnapshotManager {
	return &SnapshotManager{
		dataDir:   dataDir,
		backupDir: backupDir,
		retention: retention,
		logger:    logger,
	}
}

func (sm *SnapshotManager) TakeSnapshot(ctx context.Context, endpoint string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	snapshotPath := filepath.Join(sm.backupDir, fmt.Sprintf("snapshot-%s.db", timestamp))

	if err := os.MkdirAll(sm.backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	sm.logger.Info("Taking etcd snapshot",
		zap.String("endpoint", endpoint),
		zap.String("path", snapshotPath))

	cfg := clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 10 * time.Second,
	}

	version, err := snapshot.SaveWithVersion(ctx, sm.logger, cfg, snapshotPath)
	if err != nil {
		return "", fmt.Errorf("failed to save snapshot: %w", err)
	}

	sm.logger.Info("Snapshot saved successfully",
		zap.String("path", snapshotPath),
		zap.String("version", version))

	return snapshotPath, nil
}

func (sm *SnapshotManager) CleanupOldSnapshots() error {
	entries, err := os.ReadDir(sm.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-sm.retention)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".db" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(sm.backupDir, entry.Name())
			sm.logger.Info("Removing old snapshot", zap.String("path", path))
			if err := os.Remove(path); err != nil {
				sm.logger.Warn("Failed to remove old snapshot",
					zap.String("path", path),
					zap.Error(err))
			}
		}
	}

	return nil
}
