package sd_upstream

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

//------------------------------------------------------------------------------
// healthChecker: Used to update peer's health state
//------------------------------------------------------------------------------

type healthChecker struct {
	peers             []*Peer       // all peers
	heartbeatInterval time.Duration // check interval
	heartbeatTimeout  time.Duration // check timeout
	successTimes      int           // set peer alive if exceeds it
	failedTimes       int           // set peer dead if exceeds it
	succeedCounter    []int         // succeed times counter for peers
	failedCounter     []int         // failed times counter for peers
	checkFds          []int         // used to send check packet
	counterMu         sync.Mutex    // counter lock
	changedFlag       int32         // atomic flag indicating a state change in this round
	stopCh            chan struct{}
	closeOnce         sync.Once
	runWG             sync.WaitGroup
}

// newHealthChecker validates health settings and opens one check socket per peer.
func newHealthChecker(config *sd_config.HealthCheckerConfig, peers []*Peer) (*healthChecker, error) {
	if err := validateHealthCheckerConfig(config); err != nil {
		return nil, err
	}

	// init counter
	succeedCounter := make([]int, len(peers))
	failedCounter := make([]int, len(peers))

	// init check fd
	checkFds := make([]int, len(peers))
	for _, peer := range peers {
		fd, err := sd_socket.UDPSocket(peer.GetFamily(), false, false, false)
		if err != nil {
			for _, initializedFD := range checkFds[:peer.id] {
				_ = unix.Close(initializedFD)
			}
			return nil, fmt.Errorf("init fd for health checker failed: %w", err)
		}
		if err := unix.Connect(fd, peer.sockaddr); err != nil {
			logrus.Errorf("connect upstream %s failed: %v", peer.addr, err)
		}
		checkFds[peer.id] = fd
	}

	return &healthChecker{
		peers:             peers,
		heartbeatInterval: time.Second * time.Duration(config.HeartbeatIntervalSec),
		heartbeatTimeout:  time.Second * time.Duration(config.HeartbeatTimeoutSec),
		successTimes:      config.SuccessTimes,
		failedTimes:       config.FailedTimes,
		failedCounter:     failedCounter,
		succeedCounter:    succeedCounter,
		checkFds:          checkFds,
		stopCh:            make(chan struct{}),
	}, nil
}

func validateHealthCheckerConfig(config *sd_config.HealthCheckerConfig) error {
	if config == nil {
		return fmt.Errorf("health checker config is required")
	}
	if config.HeartbeatIntervalSec <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if config.HeartbeatTimeoutSec < 0 {
		return fmt.Errorf("heartbeat timeout must not be negative")
	}
	if config.SuccessTimes <= 0 {
		return fmt.Errorf("success times must be positive")
	}
	if config.FailedTimes <= 0 {
		return fmt.Errorf("failed times must be positive")
	}
	return nil
}

func (c *healthChecker) start(changedCh chan<- struct{}) {
	c.runWG.Add(1)
	go func() {
		defer c.runWG.Done()
		c.check(changedCh)
	}()
}

func (c *healthChecker) close() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
		c.runWG.Wait()
		for _, fd := range c.checkFds {
			_ = unix.Close(fd)
		}
	})
}

func (c *healthChecker) check(changedCh chan<- struct{}) {
	// check first time immediately
	c.checkPeers(changedCh)
	tick := time.NewTicker(c.heartbeatInterval)
	defer tick.Stop()

	// check periodically
	for {
		select {
		case <-tick.C:
			c.checkPeers(changedCh)
		case <-c.stopCh:
			return
		}
	}
}

func (c *healthChecker) checkPeers(changedCh chan<- struct{}) {
	// check peers one by one concurrently
	atomic.StoreInt32(&c.changedFlag, 0)
	wg := &sync.WaitGroup{}
	for _, peer := range c.peers {
		wg.Add(1)
		go c.checkOnePeer(peer, wg)
	}
	wg.Wait()

	// if has a peer state changed
	if atomic.LoadInt32(&c.changedFlag) != 0 {
		select {
		case changedCh <- struct{}{}:
		case <-c.stopCh:
		}
	}
}

// checkOnePeer sends one heartbeat and updates the peer state counters.
func (c *healthChecker) checkOnePeer(peer *Peer, wg *sync.WaitGroup) {
	defer wg.Done()
	checkBuf := []byte("heartbeat")
	checkFd := c.checkFds[peer.id]

	// send heartbeat packet
	err := unix.Send(checkFd, checkBuf, 0)
	if err != nil {
		logrus.Debugf("result of check on upstream %s is failed ", peer.addr)
		if c.handleFailedCheck(peer) {
			atomic.StoreInt32(&c.changedFlag, 1)
		}
		return
	}

	// after heartbeat timeout, determining failure based on fd error
	timer := time.NewTimer(c.heartbeatTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-c.stopCh:
		return
	}
	v, err := unix.GetsockoptInt(checkFd, unix.SOL_SOCKET, unix.SO_ERROR)
	if err != nil || v != 0 {
		logrus.Debugf("result of check on upstream %s is failed ", peer.addr)
		if c.handleFailedCheck(peer) {
			atomic.StoreInt32(&c.changedFlag, 1)
		}
		return
	}

	logrus.Debugf("result of check on upstream %s is succeed ", peer.addr)
	if c.handleSuccessCheck(peer) {
		atomic.StoreInt32(&c.changedFlag, 1)
	}
}

func (c *healthChecker) handleFailedCheck(peer *Peer) bool {
	// update counter
	c.counterMu.Lock()
	defer c.counterMu.Unlock()
	c.failedCounter[peer.id] += 1
	c.succeedCounter[peer.id] = 0

	// if peer is alive and exceeds failed times
	if peer.isAlive() && c.failedCounter[peer.id] >= c.failedTimes {
		peer.setState(peerDead)
		return true
	}
	return false
}

func (c *healthChecker) handleSuccessCheck(peer *Peer) bool {
	// update counter
	c.counterMu.Lock()
	defer c.counterMu.Unlock()
	c.succeedCounter[peer.id] += 1
	c.failedCounter[peer.id] = 0

	// if peer is not alive and exceeds succeed times
	if !peer.isAlive() && c.succeedCounter[peer.id] >= c.successTimes {
		peer.setState(peerAlive)
		return true
	}
	return false
}
