package sd_upstream

import (
	"github.com/pkg/errors"
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
const LookupTableSize = 997

// common error
var NoOnePeerError = errors.New("no one peer in chash")

type ConsistentHash struct {
	cache            map[string]*Peer
	lookupTable      []*Peer
	lookupTableMutex sync.RWMutex
}

func NewConsistentHash(peers []*Peer) *ConsistentHash {
	c := &ConsistentHash{
		cache:       make(map[string]*Peer),
		lookupTable: make([]*Peer, LookupTableSize),
	}

	// init lookup table
	if err := c.UpdateLookupTable(peers, true); err != nil {
		panic(err)
	}

	return c
}

func (c *ConsistentHash) UpdateLookupTable(peers []*Peer, init bool) error {
	// lock lookup table
	c.lookupTableMutex.Lock()
	defer func() {
		// clean cache
		c.cache = make(map[string]*Peer)
		c.lookupTableMutex.Unlock()
		logrus.Debug("update lookup table succeed")
	}()

	// no peers
	if len(peers) == 0 {
		return NoOnePeerError
	}

	// only one case
	if len(peers) == 1 {
		for i := range c.lookupTable {
			c.lookupTable[i] = peers[0]
		}
		return nil
	}

	// build temp permutation
	permutation := make([]int, len(peers)*2)
	for i, peer := range peers {
		offset := time33HashValue([]byte(peer.addr), 1) % uint(len(c.lookupTable))
		// mod (M - 1) + 1: make sure `skip` is not 0
		skip := time33HashValue([]byte(peer.addr), 2)%(uint(len(c.lookupTable))-1) + 1

		permutation[i*2] = int(offset)
		permutation[i*2+1] = int(skip)
	}

	// get max weight
	maxWeight := 0
	for _, peer := range peers {
		if peer.weight > maxWeight {
			maxWeight = peer.weight
		}
	}

	// clean lookup table if init
	if !init {
		c.lookupTable = make([]*Peer, LookupTableSize)
	}

	// build lookup table
	runsSoFar := 0
	next := make([]int, len(peers))
	for i := range next {
		next[i] = 0
	}
	accumulatedWeights := make([]int, len(peers))
	for i := range accumulatedWeights {
		accumulatedWeights[i] = 0
	}
	for {
		// find next peer
		for i, peer := range peers {
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
	return nil
}

func (c *ConsistentHash) SelectPeer(key []byte) *Peer {
	c.lookupTableMutex.RLock()
	defer c.lookupTableMutex.RUnlock()

	keyStr := string(key)
	if peer, ok := c.cache[keyStr]; ok {
		return peer
	}

	posInLookupTable := time33HashValue(key, 1) % uint(len(c.lookupTable))
	return c.lookupTable[posInLookupTable]
}
