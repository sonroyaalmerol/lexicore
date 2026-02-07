package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"go.uber.org/zap"
)

type GossipDiscovery struct {
	list      *memberlist.Memberlist
	peers     []string
	mu        sync.RWMutex
	nodeName  string
	nodeIP    string
	logger    *zap.Logger
	stopCh    chan struct{}
	callbacks MembershipCallbacks
}

func NewGossipDiscovery(bindAddr string, seedAddrs []string, logger *zap.Logger) (*GossipDiscovery, error) {
	nodeName := getHostname()
	nodeIP := bindAddr

	if logger == nil {
		logger = zap.NewNop()
	}

	g := &GossipDiscovery{
		peers:    []string{},
		nodeName: nodeName,
		nodeIP:   nodeIP,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}

	cfg := memberlist.DefaultLANConfig()
	cfg.Name = nodeName
	cfg.BindAddr = bindAddr
	cfg.Events = &eventDelegate{d: g}
	cfg.LogOutput = &zapLogAdapter{logger: logger}

	list, err := memberlist.Create(cfg)
	if err != nil {
		return nil, err
	}

	g.list = list

	selfPeer := fmt.Sprintf("%s=http://%s:2380", nodeName, nodeIP)
	g.peers = append(g.peers, selfPeer)

	if len(seedAddrs) > 0 {
		_, err = list.Join(seedAddrs)
		if err != nil {
			return nil, fmt.Errorf("failed to join cluster: %w", err)
		}

		for _, member := range list.Members() {
			if member.Name != nodeName {
				peer := fmt.Sprintf("%s=http://%s:2380", member.Name, member.Addr)
				g.addPeerIfNotExists(peer)
			}
		}
	}

	return g, nil
}

type eventDelegate struct {
	d *GossipDiscovery
}

func (e *eventDelegate) NotifyJoin(node *memberlist.Node) {
	e.d.mu.Lock()
	peer := fmt.Sprintf("%s=http://%s:2380", node.Name, node.Addr)
	e.d.addPeerIfNotExists(peer)
	e.d.mu.Unlock()

	if e.d.callbacks.OnMemberJoin != nil {
		peerURL := fmt.Sprintf("http://%s:2380", node.Addr)
		if err := e.d.callbacks.OnMemberJoin(node.Name, peerURL); err != nil {
			e.d.logger.Warn("Member join callback failed",
				zap.String("member", node.Name),
				zap.Error(err))
		}
	}
}

func (e *eventDelegate) NotifyLeave(node *memberlist.Node) {
	e.d.mu.Lock()
	target := fmt.Sprintf("%s=http://%s:2380", node.Name, node.Addr)
	for i, peer := range e.d.peers {
		if peer == target {
			e.d.peers = append(e.d.peers[:i], e.d.peers[i+1:]...)
			break
		}
	}
	e.d.mu.Unlock()

	if e.d.callbacks.OnMemberLeave != nil {
		if err := e.d.callbacks.OnMemberLeave(node.Name); err != nil {
			e.d.logger.Warn("Member leave callback failed",
				zap.String("member", node.Name),
				zap.Error(err))
		}
	}
}

func (e *eventDelegate) NotifyUpdate(node *memberlist.Node) {}

func (g *GossipDiscovery) GetPeers(ctx context.Context) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]string, len(g.peers))
	copy(result, g.peers)
	return result, nil
}

func (g *GossipDiscovery) GetNodeName() string {
	return g.nodeName
}

func (g *GossipDiscovery) GetNodeIP() string {
	return g.nodeIP
}

func (g *GossipDiscovery) StartMembershipSync(ctx context.Context, callbacks MembershipCallbacks) error {
	g.mu.Lock()
	g.callbacks = callbacks
	g.mu.Unlock()

	g.logger.Info("Started gossip membership sync")

	// The callbacks are now registered and will be triggered by NotifyJoin/NotifyLeave
	go g.reconciliationLoop(ctx)

	return nil
}

func (g *GossipDiscovery) reconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			g.logger.Info("Stopping gossip reconciliation loop")
			return
		case <-g.stopCh:
			return
		case <-ticker.C:
			// Periodic health check - ensure gossip is still healthy
			if g.list != nil {
				members := g.list.Members()
				g.logger.Debug("Gossip cluster status",
					zap.Int("member_count", len(members)))
			}
		}
	}
}

func (g *GossipDiscovery) Close() error {
	close(g.stopCh)
	if g.list != nil {
		return g.list.Shutdown()
	}
	return nil
}

func (g *GossipDiscovery) addPeerIfNotExists(peer string) {
	for _, p := range g.peers {
		if p == peer {
			return
		}
	}
	g.peers = append(g.peers, peer)
}

type zapLogAdapter struct {
	logger *zap.Logger
}

func (z *zapLogAdapter) Write(p []byte) (n int, err error) {
	z.logger.Debug(string(p))
	return len(p), nil
}
