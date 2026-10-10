package sd_upstream

import (
	"sync"
	"sync/atomic"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

type PeerSnapshot struct {
	Version uint64
	Peers   []*Peer
}

type PeerGroup struct {
	peers         []*Peer        // all peers managed by the shared control plane
	backup        *Peer          // fallback peer used when all peers are unavailable
	healthChecker *HealthChecker // single health checker shared by all reactors
	updateMu      sync.Mutex     // serializes immutable snapshot publication
	version       uint64         // monotonically increasing snapshot version
	snapshot      atomic.Value   // stores *PeerSnapshot
}

func NewPeerGroup(config *sd_config.UpstreamConfig) *PeerGroup {
	peers, backup := InitUpstreamPeers(config)
	group := &PeerGroup{
		peers:         peers,
		backup:        backup,
		healthChecker: NewHealthChecker(config.HealthChecker, peers),
	}
	group.refresh()

	changedCh := make(chan struct{}, 1)
	go group.healthChecker.Check(changedCh)
	go func() {
		for range changedCh {
			group.refresh()
		}
	}()
	return group
}

func (g *PeerGroup) Snapshot() *PeerSnapshot {
	return g.snapshot.Load().(*PeerSnapshot)
}

func (g *PeerGroup) MarkDead(peer *Peer) {
	peer.SetState(PeerDead)
	g.refresh()
}

func (g *PeerGroup) refresh() {
	g.updateMu.Lock()
	defer g.updateMu.Unlock()

	healthy := make([]*Peer, 0, len(g.peers))
	for _, peer := range g.peers {
		if peer == g.backup {
			continue
		}
		if peer.isAlive() {
			healthy = append(healthy, peer)
		}
	}
	if len(healthy) == 0 && g.backup != nil {
		logrus.Error("use backup peer because all peers are unavailable")
		g.backup.SetState(PeerTemp)
		healthy = append(healthy, g.backup)
	}

	g.version++
	g.snapshot.Store(&PeerSnapshot{
		Version: g.version,
		Peers:   healthy,
	})
}
