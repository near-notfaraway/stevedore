package sd_upstream

import (
	"sort"
	"sync/atomic"
)

type Manager struct {
	upstreams   []Upstream
	upstreamIDs map[string]int
	stats       *managerStats
}

type managerStats struct {
	enabled bool
	names   []string
	stats   []upstreamStats
}

type upstreamStats struct {
	selections        atomic.Uint64
	selectionFailures atomic.Uint64
	peerFailures      atomic.Uint64
}

type Stats struct {
	Name              string
	Selections        uint64
	SelectionFailures uint64
	PeerFailures      uint64
}

func NewManager(upstreamsByName map[string]Upstream, statsEnabled bool) *Manager {
	names := make([]string, 0, len(upstreamsByName))
	for name := range upstreamsByName {
		names = append(names, name)
	}
	sort.Strings(names)

	manager := &Manager{
		upstreams:   make([]Upstream, len(names)),
		upstreamIDs: make(map[string]int, len(names)),
		stats: &managerStats{
			enabled: statsEnabled,
			names:   names,
			stats:   make([]upstreamStats, len(names)),
		},
	}
	for id, name := range names {
		manager.upstreams[id] = upstreamsByName[name]
		manager.upstreamIDs[name] = id
	}
	return manager
}

func (m *Manager) SelectPeer(name string, data []byte) (*Peer, bool) {
	id, ok := m.upstreamIDs[name]
	if !ok {
		return nil, false
	}
	peer := m.upstreams[id].SelectPeer(data)
	if m.stats.enabled {
		stats := &m.stats.stats[id]
		if peer == nil {
			stats.selectionFailures.Add(1)
		} else {
			stats.selections.Add(1)
		}
	}
	return peer, true
}

func (m *Manager) MarkPeerDead(name string, peer *Peer) {
	id, ok := m.upstreamIDs[name]
	if !ok {
		return
	}
	if m.stats.enabled {
		m.stats.stats[id].peerFailures.Add(1)
	}
	m.upstreams[id].MarkPeerDead(peer)
}

func (m *Manager) Stats() []Stats {
	stats := make([]Stats, len(m.stats.names))
	for i, name := range m.stats.names {
		stat := &m.stats.stats[i]
		stats[i] = Stats{
			Name:              name,
			Selections:        stat.selections.Load(),
			SelectionFailures: stat.selectionFailures.Load(),
			PeerFailures:      stat.peerFailures.Load(),
		}
	}
	return stats
}
