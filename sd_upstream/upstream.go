package sd_upstream

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	UpstreamTypeRR    = "rr"
	UpstreamTypeCHash = "chash"
)

type Upstream interface {
	SelectPeer(data []byte) *Peer
	ResetPeers()
}

func NewUpstream(config *sd_config.UpstreamConfig) Upstream {
	switch config.Type {
	case UpstreamTypeRR:
		return NewRRUpstream(config)

	case UpstreamTypeCHash:
		return NewCHashUpstream(config)

	default:
		panic(fmt.Errorf("invalid upstream type: %s", config.Type))
	}
}

// Init Peers in Upstream, avoid duplication peers and extract backup peer
// Return peer slice and backup peer
func InitUpstreamPeers(config *sd_config.UpstreamConfig) (peers []*Peer, backup *Peer) {
	peers = make([]*Peer, 0, len(config.Peers))
	uniqueMap := make(map[string]struct{})

	for id, peerConfig := range config.Peers {
		// check upstream addr duplication
		peerAddr := fmt.Sprintf("%s:%d", peerConfig.IP, peerConfig.Port)
		if _, dup := uniqueMap[peerAddr]; dup {
			logrus.Panicf("duplicated peer %s in upstream %s", peerAddr, config.Name)
		}
		uniqueMap[peerAddr] = struct{}{}

		// create peer
		peer := NewPeer(id, peerAddr, peerConfig)
		peers = append(peers, peer)

		// set backup
		if peerConfig.Backup {
			if backup == nil {
				backup = peer
			} else {
				logrus.Panicf("two peer %s and %s is backup", backup.addr, peer.addr)
			}
		}
	}
	return
}

//------------------------------------------------------------------------------
// RRUpstream: Used to select peer through round-robin
//------------------------------------------------------------------------------

const MaxUint64 = 18446744073709551615

type RRUpstream struct {
	name          string         // unique name
	peers         []*Peer        // all peers
	healthyPeers  []*Peer        // healthy peers slice
	backup        *Peer          // backup peer
	healthChecker *HealthChecker // health checker
	rrList        []*Peer        // round robin peers list
	cur           uint64         // mod it for get peer
}

func NewRRUpstream(config *sd_config.UpstreamConfig) *RRUpstream {
	// init upstream
	peers, backup := InitUpstreamPeers(config)
	ups := &RRUpstream{
		name:          config.Name,
		peers:         peers,
		healthyPeers:  make([]*Peer, len(peers)),
		backup:        backup,
		healthChecker: NewHealthChecker(config.HealthChecker, peers),
		cur:           MaxUint64,
	}

	// init rrList and start health check
	ups.rrList = ups.buildRRList(peers)
	changedCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-changedCh:
				ups.ResetPeers()
			}
		}
	}()
	go ups.healthChecker.Check(changedCh)

	return ups
}

func (u *RRUpstream) SelectPeer(data []byte) *Peer {
	cur := atomic.AddUint64(&u.cur, 1)
	if cur == MaxUint64 {
		// avoid max uint64 and 0, mod someone is 0
		cur = atomic.AddUint64(&u.cur, 1)
	}
	return u.rrList[cur%uint64(len(u.rrList))]
}

func (u *RRUpstream) ResetPeers() {
	// get healthy peers
	num := 0
	for _, peer := range u.peers {
		if peer.isAlive() {
			u.healthyPeers[num] = peer
			num += 1
		}
	}

	// set backup
	if num == 0 {
		logrus.Error("use backup peer because of all peers dead")
		u.backup.SetState(PeerTemp)
		u.healthyPeers[num] = u.backup
		num += 1
	}

	// update lookup table
	logrus.Debugf("rehash because a peer state changed")
	u.rrList = u.buildRRList(u.healthyPeers[:num])
}

func (u *RRUpstream) buildRRList(peers []*Peer) []*Peer {
	sumWeight := 0
	maxWeight := 0
	for _, peer := range peers {
		if peer.weight > maxWeight {
			maxWeight = peer.weight
		}
		sumWeight += peer.weight
	}

	idx := 0
	rrList := make([]*Peer, sumWeight)
	for i := 0; i < maxWeight; i++ {
		for _, peer := range peers {
			if peer.weight-i > 0 {
				rrList[idx] = peer
				idx++
			}
		}
	}

	return rrList
}

//------------------------------------------------------------------------------
// CHashUpstream: Used to select peer through consistent hash
//------------------------------------------------------------------------------
type CHashUpstream struct {
	name          string          // unique name
	peers         []*Peer         // all peers
	healthyPeers  []*Peer         // healthy peers slice
	backup        *Peer           // backup peer
	cHash         *ConsistentHash // chash instant
	keyStart      int             // start index used to extract key
	keyEnd        int             // end index used to extract key
	healthChecker *HealthChecker  // health checker
}

func NewCHashUpstream(config *sd_config.UpstreamConfig) *CHashUpstream {
	// init key start and key end
	keyIdx := strings.Split(config.KeyBytes, ":")
	keyStart, err := strconv.Atoi(keyIdx[0])
	if err != nil {
		logrus.Panicf("key start %s is invalid in upstream %s", keyIdx[0], config.Name)
	}
	keyEnd, err := strconv.Atoi(keyIdx[1])
	if err != nil {
		logrus.Panicf("key end %s is invalid in upstream %s", keyIdx[1], config.Name)
	}
	if keyStart >= keyEnd {
		logrus.Panicf("key start %s >= key end %s in upstream %s", keyIdx[0], keyIdx[1], config.Name)
	}

	// init peers and chash, build upstream
	peers, backup := InitUpstreamPeers(config)
	cHash := NewConsistentHash(peers)
	ups := &CHashUpstream{
		name:          config.Name,
		peers:         peers,
		healthyPeers:  make([]*Peer, len(peers)),
		backup:        backup,
		cHash:         cHash,
		keyStart:      keyStart,
		keyEnd:        keyEnd,
		healthChecker: NewHealthChecker(config.HealthChecker, peers),
	}

	// start health check
	changedCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-changedCh:
				ups.ResetPeers()
			}
		}
	}()
	go ups.healthChecker.Check(changedCh)

	return ups
}

func (u *CHashUpstream) SelectPeer(data []byte) *Peer {
	// data too short
	if len(data) < u.keyEnd {
		return nil
	}

	return u.cHash.SelectPeer(data[u.keyStart:u.keyEnd])
}

func (u *CHashUpstream) ResetPeers() {
	// get healthy peers
	num := 0
	for _, peer := range u.peers {
		if peer.isAlive() {
			u.healthyPeers[num] = peer
			num += 1
		}
	}

	// set backup
	if num == 0 {
		logrus.Error("use backup peer because of all peers dead")
		u.backup.SetState(PeerTemp)
		u.healthyPeers[num] = u.backup
		num += 1
	}

	// update lookup table
	logrus.Debugf("rehash because a peer state changed")
	if err := u.cHash.UpdateLookupTable(u.healthyPeers[:num], false); err != nil {
		logrus.Errorf("rehash failed: %v", err)
	}
}
