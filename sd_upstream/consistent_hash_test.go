package sd_upstream

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const maxConsistentHashMigrationRatio = 0.16

func TestConsistentHashEmptyPeers(t *testing.T) {
	hash := NewConsistentHash(nil)

	require.Nil(t, hash.SelectPeer([]byte("key")))
	require.ErrorIs(t, hash.UpdateLookupTable(nil), NoOnePeerError)
	require.Nil(t, hash.lookupTable)
}

func TestConsistentHashSinglePeerOwnsEntireTable(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	hash := NewConsistentHash([]*Peer{peer})

	require.Len(t, hash.lookupTable, LookupTableSize)
	for _, selected := range hash.lookupTable {
		require.Same(t, peer, selected)
	}
	require.Same(t, peer, hash.SelectPeer([]byte("first-key")))
	require.Same(t, peer, hash.SelectPeer([]byte("second-key")))
}

func TestConsistentHashBuildIsDeterministic(t *testing.T) {
	peers := []*Peer{
		testPeer("127.0.0.1:10001", 2),
		testPeer("127.0.0.1:10002", 1),
		testPeer("127.0.0.1:10003", 1),
	}

	first := NewConsistentHash(peers)
	second := NewConsistentHash(peers)

	require.Equal(t, first.lookupTable, second.lookupTable)
	for _, key := range [][]byte{
		[]byte("connection-1"),
		[]byte("connection-2"),
		{0x00, 0x01, 0x02, 0x03},
	} {
		require.Same(t, first.SelectPeer(key), second.SelectPeer(key))
	}
}

func TestConsistentHashDistributesTableByWeight(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 3)
	second := testPeer("127.0.0.1:10002", 1)
	hash := NewConsistentHash([]*Peer{first, second})

	firstSlots := countPeer(hash.lookupTable, first)
	secondSlots := countPeer(hash.lookupTable, second)

	require.Len(t, hash.lookupTable, LookupTableSize)
	require.InDelta(t, float64(LookupTableSize)*0.75, firstSlots, 1)
	require.InDelta(t, float64(LookupTableSize)*0.25, secondSlots, 1)
	require.Equal(t, LookupTableSize, firstSlots+secondSlots)
}

func TestConsistentHashUpdateReplacesPreviousPeers(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	hash := NewConsistentHash([]*Peer{first})

	require.NoError(t, hash.UpdateLookupTable([]*Peer{second}))

	require.NotContains(t, hash.lookupTable, first)
	require.Same(t, second, hash.SelectPeer([]byte("key")))
}

func TestConsistentHashEmptyUpdateClearsPreviousTable(t *testing.T) {
	hash := NewConsistentHash([]*Peer{
		testPeer("127.0.0.1:10001", 1),
	})

	require.ErrorIs(t, hash.UpdateLookupTable(nil), NoOnePeerError)
	require.Nil(t, hash.lookupTable)
	require.Nil(t, hash.SelectPeer([]byte("key")))
}

func TestConsistentHashLimitsMigrationWhenPeerAdded(t *testing.T) {
	peers := makeEqualWeightPeers(8)
	before := NewConsistentHash(peers)
	after := NewConsistentHash(append(peers, testPeer("10.0.0.9:2345", 1)))

	require.LessOrEqual(
		t,
		float64(countMigratedSlots(before, after))/float64(LookupTableSize),
		maxConsistentHashMigrationRatio,
	)
}

func TestConsistentHashLimitsMigrationWhenPeerRemoved(t *testing.T) {
	peers := makeEqualWeightPeers(8)
	before := NewConsistentHash(peers)
	after := NewConsistentHash(peers[:len(peers)-1])

	require.LessOrEqual(
		t,
		float64(countMigratedSlots(before, after))/float64(LookupTableSize),
		maxConsistentHashMigrationRatio,
	)
}

func makeEqualWeightPeers(count int) []*Peer {
	peers := make([]*Peer, count)
	for i := range peers {
		peers[i] = testPeer(fmt.Sprintf("10.0.0.%d:2345", i+1), 1)
	}
	return peers
}

func countMigratedSlots(before, after *ConsistentHash) int {
	migrated := 0
	for i, peer := range before.lookupTable {
		if peer != after.lookupTable[i] {
			migrated++
		}
	}
	return migrated
}
