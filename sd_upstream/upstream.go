package sd_upstream

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

type Upstream interface {
	SelectPeer(data []byte) *Peer
}

//------------------------------------------------------------------------------
// RRUpstream: Used to select peer through round-robin
//------------------------------------------------------------------------------

type RRUpstream struct {
}

func (u *RRUpstream) SelectPeer(data []byte) *Peer {
	return nil
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

func NewCHashUpstream(config *sd_config.UpstreamConfig) Upstream {
	// init upstreams and its peers
	var backup *Peer
	peers := make([]*Peer, 0, len(config.Peers))
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

	// init chash and build upstream
	cHash := NewConsistentHash(peers)
	changedCh := make(chan struct{})
	upstream := &CHashUpstream{
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
	go func() {
		for {
			select {
			case <-changedCh:
				logrus.Debugf("rehash because a peer state changed")
				upstream.rehash()
			}
		}
	}()
	go upstream.healthChecker.Check(changedCh)

	return upstream
}

func (u *CHashUpstream) SelectPeer(data []byte) *Peer {
	// data too short
	if len(data) < u.keyEnd {
		return nil
	}

	return u.cHash.SelectPeer(data[u.keyStart:u.keyEnd])
}

func (u *CHashUpstream) rehash() {
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
	if err := u.cHash.UpdateLookupTable(u.healthyPeers[:num], false); err != nil {
		panic(err)
	}
}
