package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

func (mgr *Manager) loadDatabase() error {
	sources, err := mgr.db.List(mgr.shutdownCtx, "identitysources")
	if err != nil {
		return fmt.Errorf("failed to list identity sources: %w", err)
	}

	for _, data := range sources {
		var m manifest.IdentitySource
		if err := json.Unmarshal(data, &m); err != nil {
			mgr.logger.Error("Failed to unmarshal identity source", zap.Error(err))
			continue
		}

		if err := mgr.AddIdentitySource(&m); err != nil {
			mgr.logger.Error("Failed to add identity source", zap.String("name", m.Name), zap.Error(err))
			continue
		}

		mgr.logger.Info("Loaded identity source", zap.String("name", m.Name))
	}

	targets, err := mgr.db.List(mgr.shutdownCtx, "synctargets")
	if err != nil {
		return fmt.Errorf("failed to list sync targets: %w", err)
	}

	for _, data := range targets {
		var m manifest.SyncTarget
		if err := json.Unmarshal(data, &m); err != nil {
			mgr.logger.Error("Failed to unmarshal sync target", zap.Error(err))
			continue
		}

		if err := mgr.AddSyncTarget(&m); err != nil {
			mgr.logger.Error("Failed to add sync target", zap.String("name", m.Name), zap.Error(err))
			continue
		}

		mgr.logger.Info("Loaded sync target", zap.String("name", m.Name))
	}

	return nil
}

func (mgr *Manager) runWatchLoop(kind string) {
	logger := mgr.logger.With(zap.String("kind", kind))
	var rev int64 = 0

	for {
		select {
		case <-mgr.shutdownCtx.Done():
			logger.Info("Watch loop stopping")
			return
		default:
			wch := mgr.db.Watch(mgr.shutdownCtx, kind, rev)
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
					mgr.handleStoreEvent(kind, ev, logger)
				}
			}

			// Check context before sleeping
			select {
			case <-mgr.shutdownCtx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (mgr *Manager) handleStoreEvent(kind string, ev *clientv3.Event, logger *zap.Logger) {
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

		if err := mgr.AddSyncTarget(&m); err != nil {
			logger.Error("Failed to add sync target", zap.String("name", name), zap.Error(err))
			return
		}

		logger.Info("Sync target updated", zap.String("name", name))
	}
}

func (mgr *Manager) runLeaderElection(nodeName string) {
	for {
		select {
		case <-mgr.shutdownCtx.Done():
			mgr.logger.Info("Leader election stopping")
			return
		default:
			session, err := concurrency.NewSession(mgr.db.Client(), concurrency.WithTTL(15))
			if err != nil {
				mgr.logger.Error("Election session failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			election := concurrency.NewElection(session, "/lexicore/leader")
			if err := election.Campaign(mgr.shutdownCtx, nodeName); err != nil {
				mgr.logger.Debug("Campaign failed, retrying", zap.Error(err))
				session.Close()
				time.Sleep(time.Second)
				continue
			}

			mgr.logger.Info("Node acquired leadership", zap.String("node", nodeName))

			runCtx, cancel := context.WithCancel(mgr.shutdownCtx)

			go func() {
				select {
				case <-session.Done():
					mgr.logger.Warn("Leader session expired, stopping reconciliation")
					cancel()
				case <-runCtx.Done():
				}
			}()

			if err := mgr.Start(runCtx); err != nil {
				mgr.logger.Error("Manager stopped with error", zap.Error(err))
			}

			cancel()
			session.Close()

			mgr.logger.Info("Leadership released")

			select {
			case <-mgr.shutdownCtx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
		}
	}
}
