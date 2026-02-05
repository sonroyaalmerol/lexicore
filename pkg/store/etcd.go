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

func (s *EtcdStore) Client() *clientv3.Client { return s.client }

func (s *EtcdStore) Put(ctx context.Context, kind, name string, obj any) error {
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
	var res [][]byte
	for _, kv := range resp.Kvs {
		res = append(res, kv.Value)
	}
	return res, nil
}

func (s *EtcdStore) Watch(ctx context.Context, kind string, rev int64) clientv3.WatchChan {
	key := fmt.Sprintf("%s/%s/", s.prefix, kind)
	opts := []clientv3.OpOption{clientv3.WithPrefix()}
	if rev > 0 {
		opts = append(opts, clientv3.WithRev(rev))
	}
	return s.client.Watch(ctx, key, opts...)
}

func (s *EtcdStore) Delete(ctx context.Context, kind, name string) error {
	key := fmt.Sprintf("%s/%s/%s", s.prefix, kind, name)
	_, err := s.client.Delete(ctx, key)
	return err
}

func (s *EtcdStore) Close() error {
	return s.Client().Close()
}
