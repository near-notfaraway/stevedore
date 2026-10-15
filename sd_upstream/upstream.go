package sd_upstream

import (
	"fmt"
	"net"
	"strconv"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

const (
	UpstreamTypeRR    = "rr"
	UpstreamTypeCHash = "chash"

	invalidKeyPolicyDrop = "drop"
	invalidKeyPolicyRR   = "rr"

	maxUint64     = 18446744073709551615
	maxRRListSize = 1 << 16
)

type Upstream interface {
	SelectPeer(data []byte) *Peer
	MarkPeerDead(peer *Peer)
}

func NewUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) Upstream {
	switch config.Type {
	case UpstreamTypeRR:
		return newRRUpstream(group)
	case UpstreamTypeCHash:
		return newCHashUpstream(config, group)
	default:
		panic(fmt.Errorf("invalid upstream type: %s", config.Type))
	}
}

func initUpstreamPeers(config *sd_config.UpstreamConfig) (peers []*Peer, backup *Peer) {
	peers = make([]*Peer, 0, len(config.Peers))
	uniqueMap := make(map[string]struct{})

	for id, peerConfig := range config.Peers {
		peerAddr := net.JoinHostPort(peerConfig.IP, strconv.Itoa(peerConfig.Port))
		if peerConfig.Weight < 1 {
			logrus.Panicf("peer %s has invalid weight %d in upstream %s",
				peerAddr, peerConfig.Weight, config.Name)
		}
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

type rrUpstream struct {
	group           *PeerGroup // shared peer control plane
	snapshotVersion uint64     // version used to build rrList
	rrList          []*Peer    // reactor-local weighted round-robin list
	cur             uint64     // reactor-local selection cursor
}

func newRRUpstream(group *PeerGroup) *rrUpstream {
	ups := &rrUpstream{
		group: group,
		cur:   maxUint64,
	}
	ups.syncSnapshot()
	return ups
}

func (u *rrUpstream) SelectPeer(_ []byte) *Peer {
	u.syncSnapshot()
	if len(u.rrList) == 0 {
		return nil
	}
	u.cur++
	if u.cur == maxUint64 {
		u.cur++
	}
	return u.rrList[u.cur%uint64(len(u.rrList))]
}

func (u *rrUpstream) MarkPeerDead(peer *Peer) {
	u.group.markDead(peer)
}

func (u *rrUpstream) syncSnapshot() {
	snapshot := u.group.snapshot()
	if snapshot.version == u.snapshotVersion {
		return
	}
	u.rrList = buildRRList(snapshot.peers)
	u.snapshotVersion = snapshot.version
}

func buildRRList(peers []*Peer) []*Peer {
	weightDivisor := 0
	for _, peer := range peers {
		if peer.weight > 0 {
			weightDivisor = greatestCommonDivisor(weightDivisor, peer.weight)
		}
	}
	if weightDivisor == 0 {
		return nil
	}

	weights := make([]int, len(peers))
	totalWeight := 0
	for i, peer := range peers {
		if peer.weight <= 0 {
			continue
		}
		weights[i] = peer.weight / weightDivisor
		if weights[i] > maxRRListSize-totalWeight {
			panic(fmt.Errorf("normalized rr weight sum exceeds %d", maxRRListSize))
		}
		totalWeight += weights[i]
	}

	rrList := make([]*Peer, totalWeight)
	currentWeights := make([]int, len(peers))
	for pos := range rrList {
		selected := -1
		for i, weight := range weights {
			if weight == 0 {
				continue
			}
			currentWeights[i] += weight
			if selected == -1 || currentWeights[i] > currentWeights[selected] {
				selected = i
			}
		}
		rrList[pos] = peers[selected]
		currentWeights[selected] -= totalWeight
	}
	return rrList
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

type cHashUpstream struct {
	group           *PeerGroup      // shared peer control plane
	snapshotVersion uint64          // version used to build cHash
	cHash           *consistentHash // reactor-local lookup table
	key             keyExtractor    // extracts the payload hash key
	fallback        *rrUpstream     // optional fallback for invalid payload keys
}

func newCHashUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) *cHashUpstream {
	key, err := newKeyExtractor(config)
	if err != nil {
		logrus.Panicf("payload key is invalid in upstream %s: %v", config.Name, err)
	}

	policy := config.InvalidKeyPolicy
	if policy == "" {
		policy = invalidKeyPolicyDrop
	}
	var fallback *rrUpstream
	switch policy {
	case invalidKeyPolicyDrop:
	case invalidKeyPolicyRR:
		fallback = newRRUpstream(group)
	default:
		logrus.Panicf("invalid key policy %s in upstream %s", policy, config.Name)
	}

	ups := &cHashUpstream{
		group:    group,
		cHash:    newConsistentHash(nil),
		key:      key,
		fallback: fallback,
	}
	ups.syncSnapshot()
	return ups
}

func (u *cHashUpstream) SelectPeer(data []byte) *Peer {
	key, ok := u.key.extract(data)
	if !ok {
		if u.fallback != nil {
			return u.fallback.SelectPeer(data)
		}
		return nil
	}
	u.syncSnapshot()
	return u.cHash.selectPeer(key)
}

func (u *cHashUpstream) MarkPeerDead(peer *Peer) {
	u.group.markDead(peer)
}

func (u *cHashUpstream) syncSnapshot() {
	snapshot := u.group.snapshot()
	if snapshot.version == u.snapshotVersion {
		return
	}
	_ = u.cHash.updateLookupTable(snapshot.peers)
	u.snapshotVersion = snapshot.version
}
