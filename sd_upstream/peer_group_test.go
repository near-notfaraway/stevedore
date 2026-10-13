package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestPeerGroupRefreshIncludesHealthyBackup(t *testing.T) {
	primary := &Peer{addr: "127.0.0.1:10001", state: int32(peerAlive)}
	backup := &Peer{addr: "127.0.0.1:10002", state: int32(peerAlive)}
	group := &PeerGroup{
		peers:  []*Peer{primary, backup},
		backup: backup,
	}

	group.refresh()

	require.Equal(t, []*Peer{primary, backup}, group.snapshot().peers)
}

func TestPeerGroupRefreshForcesBackupWhenAllPeersAreDead(t *testing.T) {
	primary := &Peer{addr: "127.0.0.1:10001", state: int32(peerDead)}
	backup := &Peer{addr: "127.0.0.1:10002", state: int32(peerDead)}
	group := &PeerGroup{
		peers:  []*Peer{primary, backup},
		backup: backup,
	}

	group.refresh()

	require.Equal(t, []*Peer{backup}, group.snapshot().peers)
	require.Equal(t, peerTemp, peerState(backup.state))
}

func TestPeerGroupRefreshPublishesEmptySnapshotWithoutBackup(t *testing.T) {
	group := &PeerGroup{
		peers: []*Peer{
			{addr: "127.0.0.1:10001", state: int32(peerDead)},
		},
	}

	group.refresh()

	require.Empty(t, group.snapshot().peers)
}

func TestNewPeerGroupConvey(t *testing.T) {
	convey.Convey("Test NewPeerGroup", t, func() {
		convey.Convey("initializes and publishes a snapshot", func() {
			group := NewPeerGroup(&sd_config.UpstreamConfig{
				HealthChecker: &sd_config.HealthCheckerConfig{HeartbeatIntervalSec: 3600},
			})
			convey.So(group, convey.ShouldNotBeNil)
			convey.So(group.snapshot(), convey.ShouldNotBeNil)
			convey.So(group.snapshot().peers, convey.ShouldBeEmpty)
		})
	})
}

func TestPeerGroupSnapshotConvey(t *testing.T) {
	convey.Convey("Test PeerGroup.snapshot", t, func() {
		convey.Convey("loads the current snapshot", func() {
			group := &PeerGroup{}
			want := &peerSnapshot{version: 1}
			group.snapshotValue.Store(want)
			convey.So(group.snapshot(), convey.ShouldEqual, want)
		})
	})
}

func TestPeerGroupMarkDeadConvey(t *testing.T) {
	convey.Convey("Test PeerGroup.markDead", t, func() {
		convey.Convey("updates peer state and snapshot", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			group := &PeerGroup{peers: []*Peer{peer}}
			group.refresh()
			group.markDead(peer)
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
			convey.So(group.snapshot().peers, convey.ShouldBeEmpty)
		})
	})
}

func TestPeerGroupRefreshConvey(t *testing.T) {
	convey.Convey("Test PeerGroup.refresh", t, func() {
		convey.Convey("publishes healthy peers", func() {
			alive := testPeer("127.0.0.1:10001", 1)
			dead := testPeer("127.0.0.1:10002", 1)
			dead.setState(peerDead)
			group := &PeerGroup{peers: []*Peer{alive, dead}}
			group.refresh()
			convey.So(group.snapshot().version, convey.ShouldEqual, 1)
			convey.So(group.snapshot().peers, convey.ShouldResemble, []*Peer{alive})
		})
	})
}
