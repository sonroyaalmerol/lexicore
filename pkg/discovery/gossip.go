package discovery

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/hashicorp/memberlist"
)

type GossipDiscovery struct {
	list     *memberlist.Memberlist
	peers    []string
	mu       sync.RWMutex
	nodeName string
	nodeIP   string
}

func NewGossipDiscovery(bindAddr string, seedAddrs []string) (*GossipDiscovery, error) {
	nodeName := getEnvOrDefault("NODE_NAME", getHostname())
	nodeIP := getEnvOrDefault("NODE_IP", bindAddr)

	g := &GossipDiscovery{
		peers:    []string{},
		nodeName: nodeName,
		nodeIP:   nodeIP,
	}

	cfg := memberlist.DefaultLANConfig()
	cfg.Name = nodeName
	cfg.BindAddr = bindAddr
	cfg.Events = &eventDelegate{d: g}

	list, err := memberlist.Create(cfg)
	if err != nil {
		return nil, err
	}

	g.list = list

	if len(seedAddrs) > 0 {
		_, err = list.Join(seedAddrs)
		if err != nil {
			return nil, fmt.Errorf("failed to join cluster: %w", err)
		}
	}

	return g, nil
}

type eventDelegate struct {
	d *GossipDiscovery
}

func (e *eventDelegate) NotifyJoin(node *memberlist.Node) {
	e.d.mu.Lock()
	defer e.d.mu.Unlock()

	peer := fmt.Sprintf("%s=http://%s:2380", node.Name, node.Addr)
	e.d.peers = append(e.d.peers, peer)
}

func (e *eventDelegate) NotifyLeave(node *memberlist.Node) {
	e.d.mu.Lock()
	defer e.d.mu.Unlock()

	target := fmt.Sprintf("%s=http://%s:2380", node.Name, node.Addr)
	for i, peer := range e.d.peers {
		if peer == target {
			e.d.peers = append(e.d.peers[:i], e.d.peers[i+1:]...)
			break
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

func (g *GossipDiscovery) Close() error {
	if g.list != nil {
		return g.list.Shutdown()
	}
	return nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "node-unknown"
	}
	return hostname
}
