package sd_upstream

import "github.com/pkg/errors"

func time33HashValue(bytes []byte, layer int) uint {
	var hashValue uint
	for l := 0; l < layer; l++ {
		for i := 0; i < len(bytes); i++ {
			hashValue = hashValue*33 + uint(bytes[i])
		}
	}
	return hashValue
}

const LookupTableSize = 997

var NoOnePeerError = errors.New("no one peer in chash")

type ConsistentHash struct {
	lookupTable []*Peer
}

func NewConsistentHash(peers []*Peer) *ConsistentHash {
	c := &ConsistentHash{}
	_ = c.UpdateLookupTable(peers)
	return c
}

func (c *ConsistentHash) UpdateLookupTable(peers []*Peer) error {
	if len(peers) == 0 {
		c.lookupTable = nil
		return NoOnePeerError
	}

	c.lookupTable = make([]*Peer, LookupTableSize)
	if len(peers) == 1 {
		for i := range c.lookupTable {
			c.lookupTable[i] = peers[0]
		}
		return nil
	}

	permutation := make([]int, len(peers)*2)
	for i, peer := range peers {
		offset := time33HashValue([]byte(peer.addr), 1) % uint(len(c.lookupTable))
		skip := time33HashValue([]byte(peer.addr), 2)%(uint(len(c.lookupTable))-1) + 1
		permutation[i*2] = int(offset)
		permutation[i*2+1] = int(skip)
	}

	maxWeight := 0
	for _, peer := range peers {
		if peer.weight > maxWeight {
			maxWeight = peer.weight
		}
	}

	runsSoFar := 0
	next := make([]int, len(peers))
	accumulatedWeights := make([]int, len(peers))
	for {
		for i, peer := range peers {
			accumulatedWeights[i] += peer.weight
			if accumulatedWeights[i] < maxWeight {
				continue
			}
			accumulatedWeights[i] -= maxWeight

			offset := permutation[i*2]
			skip := permutation[i*2+1]
			next[i]++
			pos := (offset + next[i]*skip) % len(c.lookupTable)
			for c.lookupTable[pos] != nil {
				next[i]++
				pos = (offset + next[i]*skip) % len(c.lookupTable)
			}

			c.lookupTable[pos] = peer
			next[i]++
			runsSoFar++
			if runsSoFar == len(c.lookupTable) {
				return nil
			}
		}
	}
}

func (c *ConsistentHash) SelectPeer(key []byte) *Peer {
	if len(c.lookupTable) == 0 {
		return nil
	}
	pos := time33HashValue(key, 1) % uint(len(c.lookupTable))
	return c.lookupTable[pos]
}
