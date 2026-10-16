package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestNewPeerGroupConvey(t *testing.T) {
	convey.Convey("Test NewPeerGroup", t, func() {
		convey.Convey("rejects an invalid peer", func() {
			group, err := NewPeerGroup(&sd_config.UpstreamConfig{
				Name: "invalid",
				Type: UpstreamTypeRR,
				Peers: []*sd_config.PeerConfig{
					{IP: "127.0.0.1", Port: 2345, Weight: 0},
				},
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 1,
					SuccessTimes:         1,
					FailedTimes:          1,
				},
			})

			require.Nil(t, group)
			require.Error(t, err)
		})
		convey.Convey("initializes and publishes a snapshot", func() {
			group, err := NewPeerGroup(&sd_config.UpstreamConfig{
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
					SuccessTimes:         1,
					FailedTimes:          1,
				},
			})
			convey.So(err, convey.ShouldBeNil)
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
		convey.Convey("includes a healthy backup", func() {
			primary := &Peer{addr: "127.0.0.1:10001", state: int32(peerAlive)}
			backup := &Peer{addr: "127.0.0.1:10002", state: int32(peerAlive)}
			group := &PeerGroup{
				peers:  []*Peer{primary, backup},
				backup: backup,
			}

			group.refresh()

			require.Equal(t, []*Peer{primary, backup}, group.snapshot().peers)
		})
		convey.Convey("forces backup when all peers are dead", func() {
			primary := &Peer{addr: "127.0.0.1:10001", state: int32(peerDead)}
			backup := &Peer{addr: "127.0.0.1:10002", state: int32(peerDead)}
			group := &PeerGroup{
				peers:  []*Peer{primary, backup},
				backup: backup,
			}

			group.refresh()

			require.Equal(t, []*Peer{backup}, group.snapshot().peers)
			require.Equal(t, peerTemp, peerState(backup.state))
		})
		convey.Convey("publishes an empty snapshot without backup", func() {
			group := &PeerGroup{
				peers: []*Peer{
					{addr: "127.0.0.1:10001", state: int32(peerDead)},
				},
			}

			group.refresh()

			require.Empty(t, group.snapshot().peers)
		})
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

func TestPeerGroupCloseConvey(t *testing.T) {
	convey.Convey("Test PeerGroup.Close", t, func() {
		convey.Convey("closes health checker descriptors", func() {
			group, err := NewPeerGroup(&sd_config.UpstreamConfig{
				Name: "closable",
				Type: UpstreamTypeRR,
				Peers: []*sd_config.PeerConfig{
					{IP: "127.0.0.1", Port: 2345, Weight: 1},
				},
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
					SuccessTimes:         1,
					FailedTimes:          1,
				},
			})
			require.NoError(t, err)
			fd := group.healthChecker.checkFds[0]

			group.Close()
			group.Close()

			_, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
			require.Error(t, err)
		})
	})
}
