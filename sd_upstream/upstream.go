package sd_upstream

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

const DefaultUpstreamName = "default"

type Upstream interface {
	SelectPeer(data []byte) *Peer
}

type CHashUpstream struct {
	name          string
	peers         []*Peer
	cHash         *ConsistentHash
	keyStart      int
	keyEnd        int
	healthChecker *HealthChecker
}

func NewCHashUpstream(config *sd_config.UpstreamConfig) Upstream {
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

	return &CHashUpstream{
		name:          config.Name,
		peers:         peers,
		keyStart:      keyStart,
		keyEnd:        keyEnd,
		cHash:         NewConsistentHash(peers),
		healthChecker: NewHealthChecker(config.HealthChecker, peers),
	}
}

func (u *CHashUpstream) SelectPeer(data []byte) *Peer {
	return u.cHash.SelectPeer(data[u.keyStart:u.keyEnd])
}
