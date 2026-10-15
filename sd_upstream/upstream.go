package sd_upstream

import (
	"fmt"
	"net"
	"strconv"

	"github.com/near-notfaraway/stevedore/sd_config"
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

// ValidateConfig checks one upstream definition without creating runtime resources.
func ValidateConfig(config *sd_config.UpstreamConfig) error {
	peers, _, err := initUpstreamPeers(config)
	if err != nil {
		return err
	}
	if err := validateHealthCheckerConfig(config.HealthChecker); err != nil {
		return err
	}

	switch config.Type {
	case UpstreamTypeRR:
		return nil
	case UpstreamTypeCHash:
		if _, err := newKeyExtractor(config); err != nil {
			return fmt.Errorf("payload key is invalid in upstream %s: %w", config.Name, err)
		}
		switch config.InvalidKeyPolicy {
		case "", invalidKeyPolicyDrop:
			return nil
		case invalidKeyPolicyRR:
			return validateRRListSize(peers)
		default:
			return fmt.Errorf("invalid key policy %s in upstream %s", config.InvalidKeyPolicy, config.Name)
		}
	default:
		return fmt.Errorf("invalid upstream type: %s", config.Type)
	}
}

func NewUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) (Upstream, error) {
	if config == nil {
		return nil, fmt.Errorf("upstream config is required")
	}
	switch config.Type {
	case UpstreamTypeRR:
		return newRRUpstream(group), nil
	case UpstreamTypeCHash:
		return newCHashUpstream(config, group)
	default:
		return nil, fmt.Errorf("invalid upstream type: %s", config.Type)
	}
}

// initUpstreamPeers validates peer definitions and builds primary and backup peers.
func initUpstreamPeers(config *sd_config.UpstreamConfig) (peers []*Peer, backup *Peer, err error) {
	if config == nil {
		return nil, nil, fmt.Errorf("upstream config is required")
	}
	peers = make([]*Peer, 0, len(config.Peers))
	uniqueMap := make(map[string]struct{})

	for id, peerConfig := range config.Peers {
		if peerConfig == nil {
			return nil, nil, fmt.Errorf("peer config is required in upstream %s", config.Name)
		}
		peerAddr := net.JoinHostPort(peerConfig.IP, strconv.Itoa(peerConfig.Port))
		if peerConfig.Weight < 1 {
			return nil, nil, fmt.Errorf(
				"peer %s has invalid weight %d in upstream %s",
				peerAddr,
				peerConfig.Weight,
				config.Name,
			)
		}
		if _, dup := uniqueMap[peerAddr]; dup {
			return nil, nil, fmt.Errorf("duplicated peer %s in upstream %s", peerAddr, config.Name)
		}
		uniqueMap[peerAddr] = struct{}{}

		peer, err := NewPeer(id, peerAddr, peerConfig)
		if err != nil {
			return nil, nil, err
		}
		peers = append(peers, peer)
		if peerConfig.Backup {
			if backup != nil {
				return nil, nil, fmt.Errorf("two peer %s and %s is backup", backup.addr, peer.addr)
			}
			backup = peer
		}
	}
	if config.Type == UpstreamTypeRR {
		if err := validateRRListSize(peers); err != nil {
			return nil, nil, err
		}
	}
	return peers, backup, nil
}

func validateRRListSize(peers []*Peer) error {
	weightDivisor := 0
	for _, peer := range peers {
		weightDivisor = greatestCommonDivisor(weightDivisor, peer.weight)
	}
	if weightDivisor == 0 {
		return nil
	}

	totalWeight := 0
	for _, peer := range peers {
		weight := peer.weight / weightDivisor
		if weight > maxRRListSize-totalWeight {
			return fmt.Errorf("normalized rr weight sum exceeds %d", maxRRListSize)
		}
		totalWeight += weight
	}
	return nil
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

// buildRRList expands normalized peer weights into a smooth round-robin cycle.
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

// newCHashUpstream validates key extraction settings and builds a hash selector.
func newCHashUpstream(config *sd_config.UpstreamConfig, group *PeerGroup) (*cHashUpstream, error) {
	key, err := newKeyExtractor(config)
	if err != nil {
		return nil, fmt.Errorf("payload key is invalid in upstream %s: %w", config.Name, err)
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
		return nil, fmt.Errorf("invalid key policy %s in upstream %s", policy, config.Name)
	}

	ups := &cHashUpstream{
		group:    group,
		cHash:    newConsistentHash(nil),
		key:      key,
		fallback: fallback,
	}
	ups.syncSnapshot()
	return ups, nil
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
