package sd_upstream

import (
	"sync"
	"sync/atomic"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

type peerSnapshot struct {
	version uint64
	peers   []*Peer
}

type PeerGroup struct {
	peers         []*Peer        // all peers managed by the shared control plane
	backup        *Peer          // fallback peer used when all peers are unavailable
	healthChecker *healthChecker // single health checker shared by all reactors
	updateMu      sync.Mutex     // serializes immutable snapshot publication
	version       uint64         // monotonically increasing snapshot version
	snapshotValue atomic.Value   // stores *peerSnapshot
	publisherWG   sync.WaitGroup
}

// NewPeerGroup builds shared peer state and starts its health update loops.
func NewPeerGroup(config *sd_config.UpstreamConfig) (*PeerGroup, error) {
	peers, backup, err := initUpstreamPeers(config)
	if err != nil {
		return nil, err
	}
	checker, err := newHealthChecker(config.HealthChecker, peers)
	if err != nil {
		return nil, err
	}
	group := &PeerGroup{
		peers:         peers,
		backup:        backup,
		healthChecker: checker,
	}
	group.refresh()

	changedCh := make(chan struct{}, 1)
	group.healthChecker.start(changedCh)
	group.publisherWG.Add(1)
	go func() {
		defer group.publisherWG.Done()
		for {
			select {
			case <-changedCh:
				group.refresh()
			case <-group.healthChecker.stopCh:
				return
			}
		}
	}()
	return group, nil
}

func (g *PeerGroup) Close() {
	g.healthChecker.close()
	g.publisherWG.Wait()
}

func (g *PeerGroup) snapshot() *peerSnapshot {
	return g.snapshotValue.Load().(*peerSnapshot)
}

func (g *PeerGroup) markDead(peer *Peer) {
	peer.setState(peerDead)
	g.refresh()
}

// refresh publishes a new immutable snapshot of currently selectable peers.
func (g *PeerGroup) refresh() {
	g.updateMu.Lock()
	defer g.updateMu.Unlock()

	healthy := make([]*Peer, 0, len(g.peers))
	for _, peer := range g.peers {
		if peer.isAlive() {
			healthy = append(healthy, peer)
		}
	}
	if len(healthy) == 0 && g.backup != nil {
		logrus.Error("use backup peer because all peers are unavailable")
		g.backup.setState(peerTemp)
		healthy = append(healthy, g.backup)
	}

	g.version++
	g.snapshotValue.Store(&peerSnapshot{
		version: g.version,
		peers:   healthy,
	})
}
