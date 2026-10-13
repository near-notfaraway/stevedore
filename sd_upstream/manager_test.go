package sd_upstream

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type managerStubUpstream struct {
	peer     *Peer
	deadPeer *Peer
}

func (u *managerStubUpstream) SelectPeer([]byte) *Peer {
	return u.peer
}

func (u *managerStubUpstream) MarkPeerDead(peer *Peer) {
	u.deadPeer = peer
}

func TestManagerSelectsPeerAndRecordsStats(t *testing.T) {
	peer := &Peer{}
	manager := NewManager(map[string]Upstream{
		"default": &managerStubUpstream{peer: peer},
	}, true)

	selected, ok := manager.SelectPeer("default", nil)

	require.True(t, ok)
	require.Same(t, peer, selected)
	manager.MarkPeerDead("default", peer)
	require.Equal(t, []Stats{
		{Name: "default", Selections: 1, PeerFailures: 1},
	}, manager.Stats())
}

func TestManagerReturnsFalseForUnknownUpstream(t *testing.T) {
	manager := NewManager(nil, true)

	peer, ok := manager.SelectPeer("missing", nil)
	require.Nil(t, peer)
	require.False(t, ok)
	require.Empty(t, manager.Stats())
}

func TestManagerCountsSelectionFailures(t *testing.T) {
	manager := NewManager(map[string]Upstream{
		"default": &managerStubUpstream{},
	}, true)

	peer, ok := manager.SelectPeer("default", nil)

	require.True(t, ok)
	require.Nil(t, peer)
	require.Equal(t, uint64(1), manager.Stats()[0].SelectionFailures)
}

func TestManagerStatsDisabled(t *testing.T) {
	manager := NewManager(map[string]Upstream{
		"default": &managerStubUpstream{},
	}, false)

	_, ok := manager.SelectPeer("default", nil)
	require.True(t, ok)
	manager.MarkPeerDead("default", nil)

	require.Equal(t, []Stats{{Name: "default"}}, manager.Stats())
}

func TestManagerPreservesStatsOrder(t *testing.T) {
	firstPeer := &Peer{}
	first := &managerStubUpstream{peer: firstPeer}
	second := &managerStubUpstream{}
	manager := NewManager(map[string]Upstream{
		"second": second,
		"first":  first,
	}, true)

	selected, ok := manager.SelectPeer("first", nil)
	require.True(t, ok)
	require.Same(t, firstPeer, selected)
	selected, ok = manager.SelectPeer("second", nil)
	require.True(t, ok)
	require.Nil(t, selected)
	manager.MarkPeerDead("first", firstPeer)

	require.Equal(t, []Stats{
		{Name: "first", Selections: 1, PeerFailures: 1},
		{Name: "second", SelectionFailures: 1},
	}, manager.Stats())
	require.Same(t, firstPeer, first.deadPeer)
}

func TestManagerStatsConcurrentWithSelection(t *testing.T) {
	const selections = 1000

	manager := NewManager(map[string]Upstream{
		"default": &managerStubUpstream{peer: &Peer{}},
	}, true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range selections {
			manager.SelectPeer("default", nil)
		}
	}()
	go func() {
		defer wg.Done()
		for range selections {
			manager.Stats()
		}
	}()
	wg.Wait()

	require.Equal(t, uint64(selections), manager.Stats()[0].Selections)
}

func TestManagerMarkPeerDeadIgnoresUnknownUpstream(t *testing.T) {
	manager := NewManager(nil, true)

	require.NotPanics(t, func() {
		manager.MarkPeerDead("missing", &Peer{})
	})
}
