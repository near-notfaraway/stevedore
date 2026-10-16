package sd_upstream

import (
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
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

func TestNewManagerConvey(t *testing.T) {
	convey.Convey("Test NewManager", t, func() {
		convey.Convey("sorts upstream names", func() {
			manager := NewManager(map[string]Upstream{
				"second": &managerStubUpstream{},
				"first":  &managerStubUpstream{},
			}, true)
			convey.So(manager.stats.names, convey.ShouldResemble, []string{"first", "second"})
		})
	})
}

func TestManagerSelectPeerConvey(t *testing.T) {
	convey.Convey("Test Manager.SelectPeer", t, func() {
		convey.Convey("selects a peer and records stats", func() {
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
		})
		convey.Convey("returns false for an unknown upstream", func() {
			manager := NewManager(nil, true)

			peer, ok := manager.SelectPeer("missing", nil)
			require.Nil(t, peer)
			require.False(t, ok)
			require.Empty(t, manager.Stats())
		})
		convey.Convey("counts selection failures", func() {
			manager := NewManager(map[string]Upstream{
				"default": &managerStubUpstream{},
			}, true)

			peer, ok := manager.SelectPeer("default", nil)

			require.True(t, ok)
			require.Nil(t, peer)
			require.Equal(t, uint64(1), manager.Stats()[0].SelectionFailures)
		})
		convey.Convey("delegates and records a selection", func() {
			peer := &Peer{}
			manager := NewManager(map[string]Upstream{
				"default": &managerStubUpstream{peer: peer},
			}, true)
			got, ok := manager.SelectPeer("default", nil)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(got, convey.ShouldEqual, peer)
			convey.So(manager.Stats()[0].Selections, convey.ShouldEqual, 1)
		})
	})
}

func TestManagerMarkPeerDeadConvey(t *testing.T) {
	convey.Convey("Test Manager.MarkPeerDead", t, func() {
		convey.Convey("ignores an unknown upstream", func() {
			manager := NewManager(nil, true)

			require.NotPanics(t, func() {
				manager.MarkPeerDead("missing", &Peer{})
			})
		})
		convey.Convey("delegates and records failure", func() {
			peer := &Peer{}
			stub := &managerStubUpstream{}
			manager := NewManager(map[string]Upstream{"default": stub}, true)
			manager.MarkPeerDead("default", peer)
			convey.So(stub.deadPeer, convey.ShouldEqual, peer)
			convey.So(manager.Stats()[0].PeerFailures, convey.ShouldEqual, 1)
		})
	})
}

func TestManagerStatsConvey(t *testing.T) {
	convey.Convey("Test Manager.Stats", t, func() {
		convey.Convey("stays disabled when configured", func() {
			manager := NewManager(map[string]Upstream{
				"default": &managerStubUpstream{},
			}, false)

			_, ok := manager.SelectPeer("default", nil)
			require.True(t, ok)
			manager.MarkPeerDead("default", nil)

			require.Equal(t, []Stats{{Name: "default"}}, manager.Stats())
		})
		convey.Convey("preserves upstream order", func() {
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
		})
		convey.Convey("supports concurrent selection", func() {
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
		})
		convey.Convey("snapshots counters", func() {
			manager := NewManager(map[string]Upstream{
				"default": &managerStubUpstream{peer: &Peer{}},
			}, true)
			manager.SelectPeer("default", nil)
			convey.So(manager.Stats(), convey.ShouldResemble, []Stats{{
				Name: "default", Selections: 1,
			}})
		})
	})
}
