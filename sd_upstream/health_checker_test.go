package sd_upstream

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func closeHealthCheckerFDs(checker *healthChecker) {
	for _, fd := range checker.checkFds {
		_ = unix.Close(fd)
	}
}

func TestNewHealthCheckerConvey(t *testing.T) {
	convey.Convey("Test newHealthChecker", t, func() {
		convey.Convey("creates a checker", func() {
			peer := mustNewPeer(t, 0, "127.0.0.1:2345", &sd_config.PeerConfig{Weight: 1})
			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				HeartbeatIntervalSec: 2,
				HeartbeatTimeoutSec:  3,
				SuccessTimes:         4,
				FailedTimes:          5,
			}, []*Peer{peer})
			require.NoError(t, err)
			defer closeHealthCheckerFDs(checker)

			require.Equal(t, 2*time.Second, checker.heartbeatInterval)
			require.Equal(t, 3*time.Second, checker.heartbeatTimeout)
			require.Equal(t, 4, checker.successTimes)
			require.Equal(t, 5, checker.failedTimes)
			require.Len(t, checker.checkFds, 1)
			require.Len(t, checker.succeedCounter, 1)
			require.Len(t, checker.failedCounter, 1)
		})
		convey.Convey("closes descriptors on partial failure", func() {
			socketCalls := 0
			var closedFDs []int
			patches := gomonkey.NewPatches()
			patches.ApplyFunc(
				sd_socket.UDPSocket,
				func(int, bool, bool, bool) (int, error) {
					socketCalls++
					if socketCalls == 1 {
						return 123, nil
					}
					return -1, errors.New("socket failed")
				},
			)
			patches.ApplyFunc(unix.Close, func(fd int) error {
				closedFDs = append(closedFDs, fd)
				return nil
			})
			defer patches.Reset()

			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				HeartbeatIntervalSec: 1,
				SuccessTimes:         1,
				FailedTimes:          1,
			}, []*Peer{
				{id: 0, family: unix.AF_INET, sockaddr: &unix.SockaddrInet4{}},
				{id: 1, family: unix.AF_INET6, sockaddr: &unix.SockaddrInet6{}},
			})

			require.Nil(t, checker)
			require.Error(t, err)
			require.Contains(t, err.Error(), "socket failed")
			require.Equal(t, []int{123}, closedFDs)
		})
		convey.Convey("uses peer address families", func() {
			peers := []*Peer{
				mustNewPeer(t, 0, "127.0.0.1:2345", &sd_config.PeerConfig{Weight: 1}),
				mustNewPeer(t, 1, "[::1]:2345", &sd_config.PeerConfig{Weight: 1}),
			}
			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				HeartbeatIntervalSec: 1,
				SuccessTimes:         1,
				FailedTimes:          1,
			}, peers)
			require.NoError(t, err)
			defer closeHealthCheckerFDs(checker)

			ipv4Addr, err := unix.Getsockname(checker.checkFds[0])
			require.NoError(t, err)
			require.IsType(t, &unix.SockaddrInet4{}, ipv4Addr)

			ipv6Addr, err := unix.Getsockname(checker.checkFds[1])
			require.NoError(t, err)
			require.IsType(t, &unix.SockaddrInet6{}, ipv6Addr)
		})
		convey.Convey("initializes counters without peers", func() {
			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				HeartbeatIntervalSec: 2,
				HeartbeatTimeoutSec:  3,
				SuccessTimes:         1,
				FailedTimes:          1,
			}, nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(checker.heartbeatInterval, convey.ShouldEqual, 2*time.Second)
			convey.So(checker.checkFds, convey.ShouldBeEmpty)
		})
	})
}

func TestHealthCheckerCheckConvey(t *testing.T) {
	convey.Convey("Test healthChecker.check", t, func() {
		convey.Convey("rejects an invalid configured interval", func() {
			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				SuccessTimes: 1,
				FailedTimes:  1,
			}, nil)

			require.Nil(t, checker)
			require.EqualError(t, err, "heartbeat interval must be positive")
		})
		convey.Convey("rejects a non-positive interval", func() {
			convey.So(func() {
				(&healthChecker{}).check(make(chan struct{}, 1))
			}, convey.ShouldPanic)
		})
	})
}

func TestHealthCheckerCheckPeersConvey(t *testing.T) {
	convey.Convey("Test healthChecker.checkPeers", t, func() {
		convey.Convey("notifies state changes", func() {
			peer := &Peer{id: 0, state: int32(peerAlive)}
			checker := &healthChecker{
				peers:          []*Peer{peer},
				checkFds:       []int{-1},
				succeedCounter: []int{0},
				failedCounter:  []int{0},
				failedTimes:    1,
			}
			changedCh := make(chan struct{}, 1)

			checker.checkPeers(changedCh)

			require.False(t, peer.isAlive())
			require.Len(t, changedCh, 1)
		})
		convey.Convey("leaves the channel empty without peers", func() {
			checker := &healthChecker{}
			changed := make(chan struct{}, 1)
			checker.checkPeers(changed)
			convey.So(changed, convey.ShouldHaveLength, 0)
		})
	})
}

func TestHealthCheckerCheckOnePeerConvey(t *testing.T) {
	convey.Convey("Test healthChecker.checkOnePeer", t, func() {
		convey.Convey("handles a configured send failure", func() {
			peer := &Peer{id: 0, state: int32(peerAlive)}
			checker := &healthChecker{
				peers:          []*Peer{peer},
				checkFds:       []int{-1},
				succeedCounter: []int{0},
				failedCounter:  []int{0},
				failedTimes:    1,
			}
			var wg sync.WaitGroup
			wg.Add(1)

			checker.checkOnePeer(peer, &wg)
			wg.Wait()

			require.False(t, peer.isAlive())
			require.Equal(t, int32(1), atomic.LoadInt32(&checker.changedFlag))
		})
		convey.Convey("handles success", func() {
			receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			require.NoError(t, err)
			defer receiver.Close()

			peer := mustNewPeer(t, 0, receiver.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
			peer.setState(peerDead)
			checker, err := newHealthChecker(&sd_config.HealthCheckerConfig{
				HeartbeatIntervalSec: 1,
				HeartbeatTimeoutSec:  0,
				SuccessTimes:         1,
				FailedTimes:          1,
			}, []*Peer{peer})
			require.NoError(t, err)
			defer closeHealthCheckerFDs(checker)
			var wg sync.WaitGroup
			wg.Add(1)

			checker.checkOnePeer(peer, &wg)
			wg.Wait()

			require.True(t, peer.isAlive())
			require.Equal(t, int32(1), atomic.LoadInt32(&checker.changedFlag))
		})
		convey.Convey("handles send failure", func() {
			peer := &Peer{id: 0, state: int32(peerAlive)}
			checker := &healthChecker{
				checkFds:       []int{-1},
				succeedCounter: []int{0},
				failedCounter:  []int{0},
				failedTimes:    1,
			}
			var wg sync.WaitGroup
			wg.Add(1)
			checker.checkOnePeer(peer, &wg)
			wg.Wait()
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
			convey.So(checker.changedFlag, convey.ShouldEqual, 1)
		})
	})
}

func TestHealthCheckerHandleFailedCheckConvey(t *testing.T) {
	convey.Convey("Test healthChecker.handleFailedCheck", t, func() {
		convey.Convey("updates failure state", func() {
			peer := &Peer{id: 0, state: int32(peerAlive)}
			checker := &healthChecker{
				succeedCounter: []int{1},
				failedCounter:  []int{0},
				failedTimes:    2,
			}

			require.False(t, checker.handleFailedCheck(peer))
			require.True(t, peer.isAlive())
			require.Equal(t, 1, checker.failedCounter[0])
			require.Zero(t, checker.succeedCounter[0])

			require.True(t, checker.handleFailedCheck(peer))
			require.False(t, peer.isAlive())
			require.Equal(t, 2, checker.failedCounter[0])
		})
		convey.Convey("transitions after threshold", func() {
			peer := &Peer{id: 0, state: int32(peerAlive)}
			checker := &healthChecker{
				succeedCounter: []int{1}, failedCounter: []int{0}, failedTimes: 1,
			}
			convey.So(checker.handleFailedCheck(peer), convey.ShouldBeTrue)
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
		})
	})
}

func TestHealthCheckerHandleSuccessCheckConvey(t *testing.T) {
	convey.Convey("Test healthChecker.handleSuccessCheck", t, func() {
		convey.Convey("updates success state", func() {
			peer := &Peer{id: 0, state: int32(peerDead)}
			checker := &healthChecker{
				succeedCounter: []int{0},
				failedCounter:  []int{1},
				successTimes:   2,
			}

			require.False(t, checker.handleSuccessCheck(peer))
			require.False(t, peer.isAlive())
			require.Equal(t, 1, checker.succeedCounter[0])
			require.Zero(t, checker.failedCounter[0])

			require.True(t, checker.handleSuccessCheck(peer))
			require.True(t, peer.isAlive())
			require.Equal(t, 2, checker.succeedCounter[0])
		})
		convey.Convey("revives after threshold", func() {
			peer := &Peer{id: 0, state: int32(peerDead)}
			checker := &healthChecker{
				succeedCounter: []int{0}, failedCounter: []int{1}, successTimes: 1,
			}
			convey.So(checker.handleSuccessCheck(peer), convey.ShouldBeTrue)
			convey.So(peer.isAlive(), convey.ShouldBeTrue)
		})
	})
}
