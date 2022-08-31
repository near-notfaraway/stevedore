package sd_upstream

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"sync"
	"time"
)

//------------------------------------------------------------------------------
// HealthChecker: Used to update peer's health state
//------------------------------------------------------------------------------

type HealthChecker struct {
	peers             []*Peer
	heartbeatInterval time.Duration
	successTimes      int
	failedTimes       int
	succeedCounter    []int
	failedCounter     []int
	checkFds          []int
	counterMu         sync.Mutex
}

func NewHealthChecker(config *sd_config.HealthCheckerConfig, peers []*Peer) *HealthChecker {
	// init counter
	succeedCounter := make([]int, len(peers))
	failedCounter := make([]int, len(peers))

	// init check fd
	checkFds := make([]int, len(peers))
	for _, peer := range peers {
		fd, err := sd_socket.UDPSocket(unix.AF_INET, false, false, false)
		if err != nil {
			logrus.Panic("init fd for health checker failed: %w", err)
		}
		err = sd_socket.SetSocketTimeout(fd, config.HeartbeatTimeoutSec, config.HeartbeatTimeoutSec)
		if err != nil {
			logrus.Panic("set fd timeout for health checker failed: %w", err)
		}
		checkFds[peer.id] = fd
	}

	return &HealthChecker{
		peers:             peers,
		heartbeatInterval: time.Second * time.Duration(config.HeartbeatIntervalSec),
		successTimes:      config.SuccessTimes,
		failedTimes:       config.FailedTimes,
		failedCounter:     failedCounter,
		succeedCounter:    succeedCounter,
		checkFds:          checkFds,
	}
}

func (c *HealthChecker) Check(changedCh chan<- struct{}) {
	tick := time.NewTicker(c.heartbeatInterval)
	defer tick.Stop()

	wg := &sync.WaitGroup{}
	for range tick.C {
		for _, peer := range c.peers {
			wg.Add(1)
			go c.checkOnePeer(peer, wg, changedCh)
		}
		wg.Wait()
	}
}

func (c *HealthChecker) checkOnePeer(peer *Peer, wg *sync.WaitGroup, changedCh chan<- struct{}) {
	defer wg.Done()
	checkBuf := []byte("check")
	checkFd := c.checkFds[peer.id]

	err := unix.Sendto(checkFd, checkBuf, 0, peer.sockaddr)
	if err != nil {
		logrus.Debugf("result of check on upstream %s is failed ", peer.addr)
		if c.handleFailedCheck(peer) {
			changedCh <- struct{}{}
		}
		return
	}

	logrus.Debugf("result of check on upstream %s is succeed ", peer.addr)
	if c.handleSuccessCheck(peer) {
		changedCh <- struct{}{}
	}
}

func (c *HealthChecker) handleFailedCheck(peer *Peer) bool {
	c.counterMu.Lock()
	defer c.counterMu.Unlock()
	c.failedCounter[peer.id] += 1
	c.succeedCounter[peer.id] = 0
	if peer.isAlive() && c.failedCounter[peer.id] >= c.failedTimes {
		peer.SetState(PeerDead)
		return true
	}
	return false
}

func (c *HealthChecker) handleSuccessCheck(peer *Peer) bool {
	c.counterMu.Lock()
	defer c.counterMu.Unlock()
	c.succeedCounter[peer.id] += 1
	c.failedCounter[peer.id] = 0
	if !peer.isAlive() && c.succeedCounter[peer.id] >= c.successTimes {
		peer.SetState(PeerAlive)
		return true
	}
	return false
}
