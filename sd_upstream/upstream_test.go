package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func testPeer(addr string, weight int) *Peer {
	return &Peer{
		addr:   addr,
		weight: weight,
		state:  int32(PeerAlive),
	}
}

func testPeerGroup(version uint64, peers ...*Peer) *PeerGroup {
	group := &PeerGroup{}
	group.snapshot.Store(&PeerSnapshot{
		Version: version,
		Peers:   peers,
	})
	return group
}

func publishTestSnapshot(group *PeerGroup, version uint64, peers ...*Peer) {
	group.snapshot.Store(&PeerSnapshot{
		Version: version,
		Peers:   peers,
	})
}

func TestNewUpstream(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)

	t.Run("rr", func(t *testing.T) {
		upstream := NewUpstream(
			&sd_config.UpstreamConfig{Type: UpstreamTypeRR},
			testPeerGroup(1, peer),
		)

		require.IsType(t, &RRUpstream{}, upstream)
	})

	t.Run("chash", func(t *testing.T) {
		upstream := NewUpstream(
			&sd_config.UpstreamConfig{
				Type:     UpstreamTypeCHash,
				KeyBytes: "0:1",
			},
			testPeerGroup(1, peer),
		)

		require.IsType(t, &CHashUpstream{}, upstream)
	})

	t.Run("invalid type", func(t *testing.T) {
		require.Panics(t, func() {
			NewUpstream(
				&sd_config.UpstreamConfig{Type: "invalid"},
				testPeerGroup(1, peer),
			)
		})
	})
}

func TestInitUpstreamPeersSupportsIPv6(t *testing.T) {
	peers, backup := InitUpstreamPeers(&sd_config.UpstreamConfig{
		Name: "ipv6",
		Peers: []*sd_config.PeerConfig{
			{
				IP:     "2001:db8::10",
				Port:   2345,
				Weight: 1,
				Backup: true,
			},
		},
	})

	require.Len(t, peers, 1)
	require.Same(t, peers[0], backup)
	require.Equal(t, "[2001:db8::10]:2345", peers[0].GetAddr())
	require.IsType(t, &unix.SockaddrInet6{}, peers[0].sockaddr)
}

func TestBuildRRListPreservesWeights(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 3)
	second := testPeer("127.0.0.1:10002", 2)
	third := testPeer("127.0.0.1:10003", 1)

	list := buildRRList([]*Peer{first, second, third})

	require.Len(t, list, 6)
	require.Equal(t, 3, countPeer(list, first))
	require.Equal(t, 2, countPeer(list, second))
	require.Equal(t, 1, countPeer(list, third))
}

func TestBuildRRListDistributesWeightsSmoothly(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 5)
	second := testPeer("127.0.0.1:10002", 1)
	third := testPeer("127.0.0.1:10003", 1)

	list := buildRRList([]*Peer{first, second, third})

	require.Equal(t, []*Peer{
		first,
		first,
		second,
		first,
		third,
		first,
		first,
	}, list)
}

func TestBuildRRListNormalizesEquivalentWeights(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 20000)
	second := testPeer("127.0.0.1:10002", 10000)

	list := buildRRList([]*Peer{first, second})

	require.Len(t, list, 3)
	require.Equal(t, 2, countPeer(list, first))
	require.Equal(t, 1, countPeer(list, second))
}

func TestBuildRRListRejectsOversizedNormalizedWeights(t *testing.T) {
	require.Panics(t, func() {
		buildRRList([]*Peer{
			testPeer("127.0.0.1:10001", 40000),
			testPeer("127.0.0.1:10002", 30001),
		})
	})
}

func TestBuildRRListReturnsNilWithoutPositiveWeight(t *testing.T) {
	require.Nil(t, buildRRList(nil))
	require.Nil(t, buildRRList([]*Peer{
		testPeer("127.0.0.1:10001", 0),
		testPeer("127.0.0.1:10002", -1),
	}))
}

func TestBuildRRListIgnoresNonPositiveWeights(t *testing.T) {
	positive := testPeer("127.0.0.1:10001", 2)

	list := buildRRList([]*Peer{
		positive,
		testPeer("127.0.0.1:10002", 0),
		testPeer("127.0.0.1:10003", -1),
	})

	require.Equal(t, []*Peer{positive}, list)
}

func TestRRUpstreamSelectPeerUsesWeightedCycle(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 2)
	second := testPeer("127.0.0.1:10002", 1)
	upstream := NewRRUpstream(testPeerGroup(1, first, second))

	selected := make([]*Peer, 0, 3)
	for i := 0; i < 3; i++ {
		selected = append(selected, upstream.SelectPeer(nil))
	}

	require.Equal(t, 2, countPeer(selected, first))
	require.Equal(t, 1, countPeer(selected, second))
}

func TestRRUpstreamSelectPeerReturnsNilForEmptySnapshot(t *testing.T) {
	upstream := NewRRUpstream(testPeerGroup(1))

	require.Nil(t, upstream.SelectPeer(nil))
}

func TestRRUpstreamSelectPeerWrapsCursor(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	upstream := NewRRUpstream(testPeerGroup(1, peer))
	upstream.cur = MaxUint64 - 1

	require.Same(t, peer, upstream.SelectPeer(nil))
	require.Zero(t, upstream.cur)
}

func TestRRUpstreamSelectPeerSyncsNewSnapshot(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	group := testPeerGroup(1, first)
	upstream := NewRRUpstream(group)
	require.Same(t, first, upstream.SelectPeer(nil))

	publishTestSnapshot(group, 2, second)

	require.Same(t, second, upstream.SelectPeer(nil))
	require.Equal(t, uint64(2), upstream.snapshotVersion)
}

func TestNewCHashUpstreamRejectsInvalidKeyBytes(t *testing.T) {
	group := testPeerGroup(1, testPeer("127.0.0.1:10001", 1))
	tests := []string{
		"",
		"0",
		"0:1:2",
		"start:1",
		"0:end",
		"1:1",
		"2:1",
	}

	for _, keyBytes := range tests {
		t.Run(keyBytes, func(t *testing.T) {
			require.Panics(t, func() {
				NewCHashUpstream(
					&sd_config.UpstreamConfig{Name: "test", KeyBytes: keyBytes},
					group,
				)
			})
		})
	}
}

func TestNewCHashUpstreamRejectsNegativeKeyStart(t *testing.T) {
	group := testPeerGroup(1, testPeer("127.0.0.1:10001", 1))

	require.Panics(t, func() {
		NewCHashUpstream(
			&sd_config.UpstreamConfig{Name: "test", KeyBytes: "-1:2"},
			group,
		)
	})
}

func TestCHashUpstreamSelectPeerUsesConfiguredPayloadBytes(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	upstream := NewCHashUpstream(
		&sd_config.UpstreamConfig{Name: "test", KeyBytes: "1:3"},
		testPeerGroup(1, first, second),
	)

	selected := upstream.SelectPeer([]byte{0xff, 0x01, 0x02, 0xee})

	require.Same(t, upstream.cHash.SelectPeer([]byte{0x01, 0x02}), selected)
	require.Same(t, selected, upstream.SelectPeer([]byte{0x00, 0x01, 0x02, 0x00}))
}

func TestCHashUpstreamSelectPeerReturnsNilForShortPayload(t *testing.T) {
	upstream := NewCHashUpstream(
		&sd_config.UpstreamConfig{Name: "test", KeyBytes: "1:3"},
		testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
	)

	require.Nil(t, upstream.SelectPeer([]byte{0x01, 0x02}))
}

func TestCHashUpstreamSelectPeerFallsBackToRRForShortPayload(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	upstream := NewCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:             "test",
			KeyBytes:         "1:3",
			InvalidKeyPolicy: InvalidKeyPolicyRR,
		},
		testPeerGroup(1, peer),
	)

	require.Same(t, peer, upstream.SelectPeer([]byte{0x01, 0x02}))
}

func TestNewCHashUpstreamRejectsInvalidKeyPolicy(t *testing.T) {
	require.Panics(t, func() {
		NewCHashUpstream(
			&sd_config.UpstreamConfig{
				Name:             "test",
				KeyBytes:         "0:1",
				InvalidKeyPolicy: "invalid",
			},
			testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
		)
	})
}

func TestCHashUpstreamSelectPeerReturnsNilForEmptySnapshot(t *testing.T) {
	upstream := NewCHashUpstream(
		&sd_config.UpstreamConfig{Name: "test", KeyBytes: "0:1"},
		testPeerGroup(1),
	)

	require.Nil(t, upstream.SelectPeer([]byte{0x01}))
}

func TestCHashUpstreamSelectPeerSyncsNewSnapshot(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	group := testPeerGroup(1, first)
	upstream := NewCHashUpstream(
		&sd_config.UpstreamConfig{Name: "test", KeyBytes: "0:1"},
		group,
	)
	require.Same(t, first, upstream.SelectPeer([]byte{0x01}))

	publishTestSnapshot(group, 2, second)

	require.Same(t, second, upstream.SelectPeer([]byte{0x01}))
	require.Equal(t, uint64(2), upstream.snapshotVersion)
}

func countPeer(peers []*Peer, target *Peer) int {
	count := 0
	for _, peer := range peers {
		if peer == target {
			count++
		}
	}
	return count
}
