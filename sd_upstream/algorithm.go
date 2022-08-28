package sd_upstream

import (
	"github.com/sirupsen/logrus"
	"sync"
)

func time33HashValue(bytes []byte, layer int) uint {
	var hashValue uint = 0
	for l := 0; l < layer; l++ {
		for i := 0; i < len(bytes); i++ {
			hashValue = hashValue*33 + uint(bytes[i])
		}
	}
	return hashValue
}

//------------------------------------------------------------------------------
// ConsistentHash: select the peer through the consistent hashing algorithm
//------------------------------------------------------------------------------

// MUST BE A PRIME NUMBER
const lookupTableSize = 4999

type ConsistentHash struct {
	peers            []*Peer
	lookupTable      []*Peer
	lookupTableMutex sync.RWMutex
}

func NewConsistentHash(peers []*Peer) *ConsistentHash {
	c := &ConsistentHash{
		peers:       peers,
		lookupTable: make([]*Peer, lookupTableSize),
	}

	// init lookup table
	c.lookupTableMutex.Lock()
	defer c.lookupTableMutex.Unlock()
	for i := range c.lookupTable {
		c.lookupTable[i] = nil
	}

	return c
}

func (c *ConsistentHash) SelectPeer(key []byte) *Peer {
	c.lookupTableMutex.RLock()
	posInLookupTable := time33HashValue(key, 1) % uint(len(c.lookupTable))
	peer := c.lookupTable[posInLookupTable]
	c.lookupTableMutex.RUnlock()
	return peer
}

func (c *ConsistentHash) UpdateLookupTable() int64 {
	// lock lookup table
	c.lookupTableMutex.Lock()
	defer c.lookupTableMutex.Unlock()

	// get healthy peers
	healthyPeers := make([]*Peer, 0)
	for _, peer := range c.peers {
		if peer.isAlive() {
			healthyPeers = append(healthyPeers, peer)
		}
	}

	// edge case
	if len(healthyPeers) == 1 {
		for i := range c.lookupTable {
			c.lookupTable[i] = healthyPeers[0]
		}
		logrus.Debug("only one peer which is healthy")
		return 1
	}

	// init lookup table
	for i := range c.lookupTable {
		c.lookupTable[i] = nil
	}

	// edge case
	if len(healthyPeers) == 0 {
		// all lookup items must be NULL
		return 0
	}

	// build temp permutation
	permutation := make([]int, len(healthyPeers)*2)
	for i, peer := range healthyPeers {
		offset := time33HashValue([]byte(peer.addr), 1) % uint(len(c.lookupTable))
		// mod (M - 1) + 1: make sure `skip` is not 0
		skip := time33HashValue([]byte(peer.addr), 2)%(uint(len(c.lookupTable))-1) + 1

		permutation[i*2] = int(offset)
		permutation[i*2+1] = int(skip)
	}

	// get max weight
	maxWeight := 0
	for _, peer := range healthyPeers {
		if peer.weight > maxWeight {
			maxWeight = peer.weight
		}
	}

	// build lookup table
	runsSoFar := 0
	next := make([]int, len(healthyPeers))
	for i := range next {
		next[i] = 0
	}
	accumulatedWeights := make([]int, len(healthyPeers))
	for i := range accumulatedWeights {
		accumulatedWeights[i] = 0
	}
	for {
		// find next peer
		for i, peer := range healthyPeers {
			accumulatedWeights[i] += peer.weight
			if accumulatedWeights[i] >= maxWeight {
				accumulatedWeights[i] -= maxWeight

				offset := permutation[i*2]
				skip := permutation[i*2+1]

				// find unused position in lookup table
				next[i] += 1
				posInLookupTable := (offset + next[i]*skip) % len(c.lookupTable)
				for c.lookupTable[posInLookupTable] != nil {
					// oops, this position is already taken
					next[i] += 1
					posInLookupTable = (offset + next[i]*skip) % len(c.lookupTable)
				}

				// assign peer to lookup table
				c.lookupTable[posInLookupTable] = peer

				next[i] += 1
				runsSoFar++
				if runsSoFar == len(c.lookupTable) {
					// we have filled up the lookup table
					goto breakNestedLoop
				}
			}
		}
	}

breakNestedLoop:
	return int64(len(healthyPeers))
}
