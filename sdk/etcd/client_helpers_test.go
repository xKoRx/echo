package etcd

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type mockWatcher struct {
	lastKey string
}

func (m *mockWatcher) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	m.lastKey = key
	ch := make(chan clientv3.WatchResponse)
	close(ch)
	return ch
}
