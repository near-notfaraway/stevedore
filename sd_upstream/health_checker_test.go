package sd_upstream

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestNewHealthChecker(t *testing.T) {
	peer := NewPeer(0, "127.0.0.1:2345", &sd_config.PeerConfig{Weight: 1})
	checker := NewHealthChecker(&sd_config.HealthCheckerConfig{
		HeartbeatIntervalSec: 2,
		HeartbeatTimeoutSec:  3,
		SuccessTimes:         4,
		FailedTimes:          5,
	}, []*Peer{peer})
	defer closeHealthCheckerFDs(checker)

	require.Equal(t, 2*time.Second, checker.heartbeatInterval)
	require.Equal(t, 3*time.Second, checker.heartbeatTimeout)
	require.Equal(t, 4, checker.successTimes)
	require.Equal(t, 5, checker.failedTimes)
	require.Len(t, checker.checkFds, 1)
	require.Len(t, checker.succeedCounter, 1)
	require.Len(t, checker.failedCounter, 1)
}

func TestHealthCheckerCheckRejectsNonPositiveInterval(t *testing.T) {
	checker := &HealthChecker{}

	require.Panics(t, func() {
		checker.Check(make(chan struct{}, 1))
	})
}

func TestHealthCheckerHandleFailedCheck(t *testing.T) {
	peer := &Peer{id: 0, state: int32(PeerAlive)}
	checker := &HealthChecker{
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
}

func TestHealthCheckerHandleSuccessCheck(t *testing.T) {
	peer := &Peer{id: 0, state: int32(PeerDead)}
	checker := &HealthChecker{
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
}

func TestHealthCheckerCheckOnePeerHandlesSendFailure(t *testing.T) {
	peer := &Peer{id: 0, state: int32(PeerAlive)}
	checker := &HealthChecker{
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
}

func TestHealthCheckerCheckOnePeerHandlesSuccess(t *testing.T) {
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer receiver.Close()

	peer := NewPeer(0, receiver.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
	peer.SetState(PeerDead)
	checker := NewHealthChecker(&sd_config.HealthCheckerConfig{
		HeartbeatTimeoutSec: 0,
		SuccessTimes:        1,
		FailedTimes:         1,
	}, []*Peer{peer})
	defer closeHealthCheckerFDs(checker)
	var wg sync.WaitGroup
	wg.Add(1)

	checker.checkOnePeer(peer, &wg)
	wg.Wait()

	require.True(t, peer.isAlive())
	require.Equal(t, int32(1), atomic.LoadInt32(&checker.changedFlag))
}

func TestHealthCheckerCheckPeersNotifiesStateChange(t *testing.T) {
	peer := &Peer{id: 0, state: int32(PeerAlive)}
	checker := &HealthChecker{
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
}

func closeHealthCheckerFDs(checker *HealthChecker) {
	for _, fd := range checker.checkFds {
		_ = unix.Close(fd)
	}
}
