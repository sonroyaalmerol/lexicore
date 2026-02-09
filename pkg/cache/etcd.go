package cache

import (
	"context"
	"encoding/json"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

func (s *Cache) LoadFromEtcd(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp, err := s.etcd.Client().Get(ctx, s.etcdKey())
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	if len(resp.Kvs) == 0 {
		s.logger.Info("No existing snapshot found in etcd")
		return nil
	}

	var snapshot Snapshot
	if err := json.Unmarshal(resp.Kvs[0].Value, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	s.current = &snapshot
	s.version = snapshot.Version

	s.logger.Info("Loaded snapshot from etcd",
		zap.String("cache_key", s.cacheKey),
		zap.Int64("version", s.version),
		zap.Int("identities", len(s.current.Identities)),
		zap.Int("groups", len(s.current.Groups)))

	return nil
}

func (s *Cache) saveToEtcdUnlocked(ctx context.Context) error {
	data, err := json.Marshal(s.current)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	_, err = s.etcd.Client().Put(ctx, s.etcdKey(), string(data))
	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

func (s *Cache) WatchSnapshots(ctx context.Context) {
	key := s.etcdKey()

	resp, err := s.etcd.Client().Get(ctx, key)
	if err != nil {
		s.logger.Error("Failed to get current revision", zap.Error(err))
		return
	}

	rev := resp.Header.Revision + 1
	wch := s.etcd.Client().Watch(ctx, key, clientv3.WithRev(rev))

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping cache watch", zap.String("cache_key", s.cacheKey))
			return
		case resp := <-wch:
			if resp.Canceled {
				s.logger.Error("Cache watch canceled",
					zap.String("cache_key", s.cacheKey),
					zap.Error(resp.Err()))
				return
			}

			for _, ev := range resp.Events {
				if ev.Type == clientv3.EventTypePut {
					s.handleWatchUpdate(ev.Kv.Value)
				}
			}
		}
	}
}

func (s *Cache) handleWatchUpdate(data []byte) {
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		s.logger.Error("Failed to unmarshal watched snapshot", zap.Error(err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if snapshot.Version > s.current.Version {
		s.previous = s.current
		s.current = &snapshot
		s.version = snapshot.Version

		s.logger.Info("Updated cache from watch",
			zap.String("cache_key", s.cacheKey),
			zap.Int64("version", s.version))
	}
}
