package sd_upstream

import (
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

const maxConsistentHashMigrationRatio = 0.16

func TestConsistentHashEmptyPeers(t *testing.T) {
	hash := newConsistentHash(nil)

	require.Nil(t, hash.selectPeer([]byte("key")))
	require.ErrorIs(t, hash.updateLookupTable(nil), noPeerError)
	require.Nil(t, hash.lookupTable)
}

func TestConsistentHashSinglePeerOwnsEntireTable(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	hash := newConsistentHash([]*Peer{peer})

	require.Len(t, hash.lookupTable, lookupTableSize)
	for _, selected := range hash.lookupTable {
		require.Same(t, peer, selected)
	}
	require.Same(t, peer, hash.selectPeer([]byte("first-key")))
	require.Same(t, peer, hash.selectPeer([]byte("second-key")))
}

func TestConsistentHashBuildIsDeterministic(t *testing.T) {
	peers := []*Peer{
		testPeer("127.0.0.1:10001", 2),
		testPeer("127.0.0.1:10002", 1),
		testPeer("127.0.0.1:10003", 1),
	}

	first := newConsistentHash(peers)
	second := newConsistentHash(peers)

	require.Equal(t, first.lookupTable, second.lookupTable)
	for _, key := range [][]byte{
		[]byte("connection-1"),
		[]byte("connection-2"),
		{0x00, 0x01, 0x02, 0x03},
	} {
		require.Same(t, first.selectPeer(key), second.selectPeer(key))
	}
}

func TestConsistentHashDistributesTableByWeight(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 3)
	second := testPeer("127.0.0.1:10002", 1)
	hash := newConsistentHash([]*Peer{first, second})

	firstSlots := countPeer(hash.lookupTable, first)
	secondSlots := countPeer(hash.lookupTable, second)

	require.Len(t, hash.lookupTable, lookupTableSize)
	require.InDelta(t, float64(lookupTableSize)*0.75, firstSlots, 1)
	require.InDelta(t, float64(lookupTableSize)*0.25, secondSlots, 1)
	require.Equal(t, lookupTableSize, firstSlots+secondSlots)
}

func TestConsistentHashUpdateReplacesPreviousPeers(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	hash := newConsistentHash([]*Peer{first})

	require.NoError(t, hash.updateLookupTable([]*Peer{second}))

	require.NotContains(t, hash.lookupTable, first)
	require.Same(t, second, hash.selectPeer([]byte("key")))
}

func TestConsistentHashEmptyUpdateClearsPreviousTable(t *testing.T) {
	hash := newConsistentHash([]*Peer{
		testPeer("127.0.0.1:10001", 1),
	})

	require.ErrorIs(t, hash.updateLookupTable(nil), noPeerError)
	require.Nil(t, hash.lookupTable)
	require.Nil(t, hash.selectPeer([]byte("key")))
}

func TestConsistentHashLimitsMigrationWhenPeerAdded(t *testing.T) {
	peers := makeEqualWeightPeers(8)
	before := newConsistentHash(peers)
	after := newConsistentHash(append(peers, testPeer("10.0.0.9:2345", 1)))

	require.LessOrEqual(
		t,
		float64(countMigratedSlots(before, after))/float64(lookupTableSize),
		maxConsistentHashMigrationRatio,
	)
}

func TestConsistentHashLimitsMigrationWhenPeerRemoved(t *testing.T) {
	peers := makeEqualWeightPeers(8)
	before := newConsistentHash(peers)
	after := newConsistentHash(peers[:len(peers)-1])

	require.LessOrEqual(
		t,
		float64(countMigratedSlots(before, after))/float64(lookupTableSize),
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

func countMigratedSlots(before, after *consistentHash) int {
	migrated := 0
	for i, peer := range before.lookupTable {
		if peer != after.lookupTable[i] {
			migrated++
		}
	}
	return migrated
}

func TestNewConsistentHashConvey(t *testing.T) {
	convey.Convey("Test newConsistentHash", t, func() {
		convey.Convey("builds a lookup table", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			hash := newConsistentHash([]*Peer{peer})
			convey.So(hash.lookupTable, convey.ShouldHaveLength, lookupTableSize)
			convey.So(hash.lookupTable[0], convey.ShouldEqual, peer)
		})
	})
}

func TestConsistentHashUpdateLookupTableConvey(t *testing.T) {
	convey.Convey("Test consistentHash.updateLookupTable", t, func() {
		convey.Convey("rejects no peers", func() {
			hash := &consistentHash{}
			convey.So(hash.updateLookupTable(nil), convey.ShouldEqual, noPeerError)
			convey.So(hash.lookupTable, convey.ShouldBeNil)
		})
	})
}

func TestConsistentHashSelectPeerConvey(t *testing.T) {
	convey.Convey("Test consistentHash.selectPeer", t, func() {
		convey.Convey("returns the hashed peer", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			hash := newConsistentHash([]*Peer{peer})
			convey.So(hash.selectPeer([]byte("key")), convey.ShouldEqual, peer)
		})
	})
}

func TestTime33HashValueConvey(t *testing.T) {
	convey.Convey("Test time33HashValue", t, func() {
		convey.Convey("is deterministic across layers", func() {
			convey.So(time33HashValue([]byte("key"), 1), convey.ShouldEqual, time33HashValue([]byte("key"), 1))
			convey.So(time33HashValue([]byte("key"), 2), convey.ShouldNotEqual, time33HashValue([]byte("key"), 1))
		})
	})
}
