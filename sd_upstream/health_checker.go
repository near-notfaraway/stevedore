package sd_upstream

import (
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
}

func newHealthChecker(config *sd_config.HealthCheckerConfig, peers []*Peer) *healthChecker {
	// init counter
	succeedCounter := make([]int, len(peers))
	failedCounter := make([]int, len(peers))

	// init check fd
	checkFds := make([]int, len(peers))
	for _, peer := range peers {
		fd, err := sd_socket.UDPSocket(peer.GetFamily(), false, false, false)
		if err != nil {
			logrus.Panicf("init fd for health checker failed: %v", err)
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
	}
}

func (c *healthChecker) check(changedCh chan<- struct{}) {
	// check first time immediately
	c.checkPeers(changedCh)
	tick := time.NewTicker(c.heartbeatInterval)
	defer tick.Stop()

	// check periodically
	for range tick.C {
		c.checkPeers(changedCh)
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
		changedCh <- struct{}{}
	}
}

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
	<-timer.C
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
