package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"
)

type EtcdMembershipManager struct {
	etcd      *embed.Etcd
	client    *clientv3.Client
	discovery DynamicDiscovery
	nodeName  string
	logger    *zap.Logger
	members   map[string]*memberState
	stopCh    chan struct{}
}

type memberState struct {
	ID        uint64
	Name      string
	IsLearner bool
	AddedAt   time.Time
}

func NewEtcdMembershipManager(
	etcd *embed.Etcd,
	discovery DynamicDiscovery,
	nodeName string,
	logger *zap.Logger,
) (*EtcdMembershipManager, error) {

	if logger == nil {
		logger = zap.NewNop()
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcd.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	mgr := &EtcdMembershipManager{
		etcd:      etcd,
		client:    cli,
		discovery: discovery,
		nodeName:  nodeName,
		logger:    logger,
		members:   make(map[string]*memberState),
		stopCh:    make(chan struct{}),
	}

	if err := mgr.syncClusterMembers(); err != nil {
		logger.Warn("Failed to sync initial cluster members", zap.Error(err))
	}

	return mgr, nil
}

func (m *EtcdMembershipManager) Start(ctx context.Context) error {
	callbacks := MembershipCallbacks{
		OnMemberJoin:  m.handleMemberJoin,
		OnMemberLeave: m.handleMemberLeave,
	}

	if err := m.discovery.StartMembershipSync(ctx, callbacks); err != nil {
		return fmt.Errorf("failed to start discovery sync: %w", err)
	}

	go m.promotionLoop(ctx)

	m.logger.Info("Started etcd membership manager")
	return nil
}

func (m *EtcdMembershipManager) handleMemberJoin(name, peerURL string) error {
	if name == m.nodeName {
		return nil
	}

	if _, known := m.members[name]; known {
		return nil
	}

	m.logger.Info("Discovered new member, adding to etcd cluster",
		zap.String("member", name),
		zap.String("peer_url", peerURL))

	memberID, err := m.addMemberAsLearner(name, peerURL)
	if err != nil {
		return err
	}

	m.members[name] = &memberState{
		ID:        memberID,
		Name:      name,
		IsLearner: true,
		AddedAt:   time.Now(),
	}

	return nil
}

func (m *EtcdMembershipManager) handleMemberLeave(name string) error {
	if name == m.nodeName {
		return nil
	}

	state, known := m.members[name]
	if !known {
		return nil
	}

	m.logger.Warn("Member left gossip cluster",
		zap.String("member", name),
		zap.Uint64("etcd_id", state.ID))

	// Note: We log but don't automatically remove from etcd
	// Automatic removal can be dangerous - may want manual intervention
	// return m.removeMember(state.ID, name)

	return nil
}

func (m *EtcdMembershipManager) addMemberAsLearner(name string, peerURL string) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := m.client.MemberAddAsLearner(ctx, []string{peerURL})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			m.logger.Info("Member already exists in cluster",
				zap.String("member", name))
			return 0, nil
		}

		m.logger.Error("Failed to add member as learner",
			zap.String("member", name),
			zap.String("peer_url", peerURL),
			zap.Error(err))
		return 0, err
	}

	m.logger.Info("Added member as learner",
		zap.String("member", name),
		zap.Uint64("id", resp.Member.ID))

	return resp.Member.ID, nil
}

func (m *EtcdMembershipManager) promotionLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-m.etcd.Server.StopNotify():
			return
		case <-ticker.C:
			m.promoteReadyLearners()
		}
	}
}

func (m *EtcdMembershipManager) promoteReadyLearners() {
	for name, state := range m.members {
		if !state.IsLearner {
			continue
		}

		// Wait at least 30 seconds before promoting
		if time.Since(state.AddedAt) < 30*time.Second {
			continue
		}

		if err := m.promoteLearner(state.ID, name); err == nil {
			state.IsLearner = false
		}
	}
}

func (m *EtcdMembershipManager) promoteLearner(memberID uint64, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.client.MemberPromote(ctx, memberID)
	if err != nil {
		m.logger.Warn("Failed to promote learner",
			zap.String("member", name),
			zap.Uint64("id", memberID),
			zap.Error(err))
		return err
	}

	m.logger.Info("Promoted learner to voting member",
		zap.String("member", name),
		zap.Uint64("id", memberID))

	return nil
}

func (m *EtcdMembershipManager) removeMember(memberID uint64, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.client.MemberRemove(ctx, memberID)
	if err != nil {
		m.logger.Error("Failed to remove member",
			zap.String("member", name),
			zap.Uint64("id", memberID),
			zap.Error(err))
		return err
	}

	delete(m.members, name)

	m.logger.Info("Removed member from cluster",
		zap.String("member", name),
		zap.Uint64("id", memberID))

	return nil
}

func (m *EtcdMembershipManager) syncClusterMembers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	memberList, err := m.client.MemberList(ctx)
	if err != nil {
		return err
	}

	for _, member := range memberList.Members {
		if member.Name != "" && member.Name != m.nodeName {
			m.members[member.Name] = &memberState{
				ID:        member.ID,
				Name:      member.Name,
				IsLearner: member.IsLearner,
				AddedAt:   time.Now(),
			}
		}
	}

	m.logger.Info("Synced cluster members",
		zap.Int("count", len(m.members)))

	return nil
}

func (m *EtcdMembershipManager) Stop() error {
	close(m.stopCh)
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}
