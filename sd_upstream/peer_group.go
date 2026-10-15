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
}

func NewPeerGroup(config *sd_config.UpstreamConfig) *PeerGroup {
	peers, backup := initUpstreamPeers(config)
	group := &PeerGroup{
		peers:         peers,
		backup:        backup,
		healthChecker: newHealthChecker(config.HealthChecker, peers),
	}
	group.refresh()

	changedCh := make(chan struct{}, 1)
	go group.healthChecker.check(changedCh)
	go func() {
		for range changedCh {
			group.refresh()
		}
	}()
	return group
}

func (g *PeerGroup) snapshot() *peerSnapshot {
	return g.snapshotValue.Load().(*peerSnapshot)
}

func (g *PeerGroup) markDead(peer *Peer) {
	peer.setState(peerDead)
	g.refresh()
}

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
