package sd_upstream

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

const (
	UpstreamTypeRR    = "rr"
	UpstreamTypeCHash = "chash"
)

type Upstream interface {
	SelectPeer(data []byte) *Peer
	MarkPeerDead(peer *Peer)
}

func NewUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) Upstream {
	switch config.Type {
	case UpstreamTypeRR:
		return NewRRUpstream(group)
	case UpstreamTypeCHash:
		return NewCHashUpstream(config, group)
	default:
		panic(fmt.Errorf("invalid upstream type: %s", config.Type))
	}
}

func InitUpstreamPeers(config *sd_config.UpstreamConfig) (peers []*Peer, backup *Peer) {
	peers = make([]*Peer, 0, len(config.Peers))
	uniqueMap := make(map[string]struct{})

	for id, peerConfig := range config.Peers {
		if peerConfig.Weight < 1 {
			logrus.Panicf("peer %s:%d has invalid weight %d in upstream %s",
				peerConfig.IP, peerConfig.Port, peerConfig.Weight, config.Name)
		}
		peerAddr := fmt.Sprintf("%s:%d", peerConfig.IP, peerConfig.Port)
		if _, dup := uniqueMap[peerAddr]; dup {
			logrus.Panicf("duplicated peer %s in upstream %s", peerAddr, config.Name)
		}
		uniqueMap[peerAddr] = struct{}{}

		peer := NewPeer(id, peerAddr, peerConfig)
		peers = append(peers, peer)
		if peerConfig.Backup {
			if backup != nil {
				logrus.Panicf("two peer %s and %s is backup", backup.addr, peer.addr)
			}
			backup = peer
		}
	}
	return
}

const MaxUint64 = 18446744073709551615

type RRUpstream struct {
	group           *PeerGroup // shared peer control plane
	snapshotVersion uint64     // version used to build rrList
	rrList          []*Peer    // reactor-local weighted round-robin list
	cur             uint64     // reactor-local selection cursor
}

func NewRRUpstream(group *PeerGroup) *RRUpstream {
	ups := &RRUpstream{
		group: group,
		cur:   MaxUint64,
	}
	ups.syncSnapshot()
	return ups
}

func (u *RRUpstream) SelectPeer(_ []byte) *Peer {
	u.syncSnapshot()
	if len(u.rrList) == 0 {
		return nil
	}
	u.cur++
	if u.cur == MaxUint64 {
		u.cur++
	}
	return u.rrList[u.cur%uint64(len(u.rrList))]
}

func (u *RRUpstream) MarkPeerDead(peer *Peer) {
	u.group.MarkDead(peer)
}

func (u *RRUpstream) syncSnapshot() {
	snapshot := u.group.Snapshot()
	if snapshot.Version == u.snapshotVersion {
		return
	}
	u.rrList = buildRRList(snapshot.Peers)
	u.snapshotVersion = snapshot.Version
}

func buildRRList(peers []*Peer) []*Peer {
	sumWeight := 0
	maxWeight := 0
	for _, peer := range peers {
		if peer.weight > maxWeight {
			maxWeight = peer.weight
		}
		if peer.weight > 0 {
			sumWeight += peer.weight
		}
	}
	if sumWeight == 0 || maxWeight == 0 {
		return nil
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

type CHashUpstream struct {
	group           *PeerGroup      // shared peer control plane
	snapshotVersion uint64          // version used to build cHash
	cHash           *ConsistentHash // reactor-local lookup table
	keyStart        int             // start index used to extract key
	keyEnd          int             // end index used to extract key
}

func NewCHashUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) *CHashUpstream {
	keyIdx := strings.Split(config.KeyBytes, ":")
	if len(keyIdx) != 2 {
		logrus.Panicf("key bytes %s is invalid in upstream %s", config.KeyBytes, config.Name)
	}
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

	ups := &CHashUpstream{
		group:    group,
		cHash:    NewConsistentHash(nil),
		keyStart: keyStart,
		keyEnd:   keyEnd,
	}
	ups.syncSnapshot()
	return ups
}

func (u *CHashUpstream) SelectPeer(data []byte) *Peer {
	if len(data) < u.keyEnd {
		return nil
	}
	u.syncSnapshot()
	return u.cHash.SelectPeer(data[u.keyStart:u.keyEnd])
}

func (u *CHashUpstream) MarkPeerDead(peer *Peer) {
	u.group.MarkDead(peer)
}

func (u *CHashUpstream) syncSnapshot() {
	snapshot := u.group.Snapshot()
	if snapshot.Version == u.snapshotVersion {
		return
	}
	_ = u.cHash.UpdateLookupTable(snapshot.Peers)
	u.snapshotVersion = snapshot.Version
}
