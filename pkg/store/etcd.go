package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdStore struct {
	client *clientv3.Client
	prefix string
}

func NewEtcdStore(endpoints []string, timeout time.Duration) (*EtcdStore, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: timeout,
	})
	if err != nil {
		return nil, err
	}
	return &EtcdStore{client: cli, prefix: "/lexicore.io"}, nil
}

func (s *EtcdStore) Put(ctx context.Context, kind string, name string, obj any) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s/%s/%s", s.prefix, kind, name)
	_, err = s.client.Put(ctx, key, string(data))
	return err
}

func (s *EtcdStore) List(ctx context.Context, kind string) ([][]byte, error) {
	key := fmt.Sprintf("%s/%s/", s.prefix, kind)
	resp, err := s.client.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var results [][]byte
	for _, kv := range resp.Kvs {
		results = append(results, kv.Value)
	}
	return results, nil
}

func (s *EtcdStore) Watch(ctx context.Context, kind string) clientv3.WatchChan {
	key := fmt.Sprintf("%s/%s/", s.prefix, kind)
	return s.client.Watch(ctx, key, clientv3.WithPrefix())
}

func (s *EtcdStore) Delete(ctx context.Context, kind string, name string) error {
	key := fmt.Sprintf("%s/%s/%s", s.prefix, kind, name)
	_, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	return nil
}
