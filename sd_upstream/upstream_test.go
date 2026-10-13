package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func testPeer(addr string, weight int) *Peer {
	return &Peer{
		addr:   addr,
		weight: weight,
		state:  int32(peerAlive),
	}
}

func testPeerGroup(version uint64, peers ...*Peer) *PeerGroup {
	group := &PeerGroup{}
	group.snapshotValue.Store(&peerSnapshot{
		version: version,
		peers:   peers,
	})
	return group
}

func publishTestSnapshot(group *PeerGroup, version uint64, peers ...*Peer) {
	group.snapshotValue.Store(&peerSnapshot{
		version: version,
		peers:   peers,
	})
}

func fixedKeyExtractorConfig(keyBytes, keyMask string) *sd_config.KeyExtractorConfig {
	return &sd_config.KeyExtractorConfig{
		Type: keyExtractorTypeFixed,
		Fixed: &sd_config.FixedKeyConfig{
			KeyBytes: keyBytes,
			KeyMask:  keyMask,
		},
	}
}

func TestNewUpstream(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)

	t.Run("rr", func(t *testing.T) {
		upstream := NewUpstream(
			&sd_config.UpstreamConfig{Type: UpstreamTypeRR},
			testPeerGroup(1, peer),
		)

		require.IsType(t, &rrUpstream{}, upstream)
	})

	t.Run("chash", func(t *testing.T) {
		upstream := NewUpstream(
			&sd_config.UpstreamConfig{
				Type:         UpstreamTypeCHash,
				KeyExtractor: fixedKeyExtractorConfig("0:1", ""),
			},
			testPeerGroup(1, peer),
		)

		require.IsType(t, &cHashUpstream{}, upstream)
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
	peers, backup := initUpstreamPeers(&sd_config.UpstreamConfig{
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
	upstream := newRRUpstream(testPeerGroup(1, first, second))

	selected := make([]*Peer, 0, 3)
	for i := 0; i < 3; i++ {
		selected = append(selected, upstream.SelectPeer(nil))
	}

	require.Equal(t, 2, countPeer(selected, first))
	require.Equal(t, 1, countPeer(selected, second))
}

func TestRRUpstreamSelectPeerReturnsNilForEmptySnapshot(t *testing.T) {
	upstream := newRRUpstream(testPeerGroup(1))

	require.Nil(t, upstream.SelectPeer(nil))
}

func TestRRUpstreamSelectPeerWrapsCursor(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	upstream := newRRUpstream(testPeerGroup(1, peer))
	upstream.cur = maxUint64 - 1

	require.Same(t, peer, upstream.SelectPeer(nil))
	require.Zero(t, upstream.cur)
}

func TestRRUpstreamSelectPeerSyncsNewSnapshot(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	group := testPeerGroup(1, first)
	upstream := newRRUpstream(group)
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
				newCHashUpstream(
					&sd_config.UpstreamConfig{
						Name:         "test",
						KeyExtractor: fixedKeyExtractorConfig(keyBytes, ""),
					},
					group,
				)
			})
		})
	}
}

func TestNewCHashUpstreamRejectsNegativeKeyStart(t *testing.T) {
	group := testPeerGroup(1, testPeer("127.0.0.1:10001", 1))

	require.Panics(t, func() {
		newCHashUpstream(
			&sd_config.UpstreamConfig{
				Name:         "test",
				KeyExtractor: fixedKeyExtractorConfig("-1:2", ""),
			},
			group,
		)
	})
}

func TestCHashUpstreamSelectPeerUsesConfiguredPayloadBytes(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:         "test",
			KeyExtractor: fixedKeyExtractorConfig("1:3", ""),
		},
		testPeerGroup(1, first, second),
	)

	selected := upstream.SelectPeer([]byte{0xff, 0x01, 0x02, 0xee})

	require.Same(t, upstream.cHash.selectPeer([]byte{0x01, 0x02}), selected)
	require.Same(t, selected, upstream.SelectPeer([]byte{0x00, 0x01, 0x02, 0x00}))
}

func TestCHashUpstreamSelectPeerUsesLengthFieldKey(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name: "test",
			KeyExtractor: &sd_config.KeyExtractorConfig{
				Type: "length_field",
				LengthField: &sd_config.LengthFieldKeyConfig{
					KeyStart:     2,
					LengthStart:  0,
					LengthBytes:  1,
					ByteOrder:    "big",
					MaxKeyLength: 8,
				},
			},
		},
		testPeerGroup(1, first, second),
	)
	payload := []byte{0x03, 0xaa, 0x11, 0x22, 0x33}

	require.Same(t, upstream.cHash.selectPeer([]byte{0x11, 0x22, 0x33}), upstream.SelectPeer(payload))
}

func TestNewCHashUpstreamRejectsMismatchedKeyExtractorConfig(t *testing.T) {
	lengthField := &sd_config.LengthFieldKeyConfig{
		KeyStart:     2,
		LengthStart:  0,
		LengthBytes:  1,
		ByteOrder:    "big",
		MaxKeyLength: 8,
	}
	tests := map[string]*sd_config.KeyExtractorConfig{
		"fixed with length field": {
			Type:        "fixed",
			Fixed:       &sd_config.FixedKeyConfig{KeyBytes: "0:4"},
			LengthField: lengthField,
		},
		"length field with fixed": {
			Type:        "length_field",
			Fixed:       &sd_config.FixedKeyConfig{KeyBytes: "0:4"},
			LengthField: lengthField,
		},
	}

	for name, keyConfig := range tests {
		t.Run(name, func(t *testing.T) {
			require.Panics(t, func() {
				newCHashUpstream(
					&sd_config.UpstreamConfig{
						Name:         "test",
						KeyExtractor: keyConfig,
					},
					testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
				)
			})
		})
	}
}

func TestNewCHashUpstreamRequiresKeyExtractor(t *testing.T) {
	require.Panics(t, func() {
		newCHashUpstream(
			&sd_config.UpstreamConfig{
				Name: "test",
			},
			testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
		)
	})
}

func TestNewCHashUpstreamRejectsInvalidKeyExtractor(t *testing.T) {
	tests := map[string]*sd_config.KeyExtractorConfig{
		"missing type": {},
		"unknown type": {Type: "unknown"},
		"missing fixed config": {
			Type: "fixed",
		},
		"missing length field config": {
			Type: "length_field",
		},
	}

	for name, keyConfig := range tests {
		t.Run(name, func(t *testing.T) {
			require.Panics(t, func() {
				newCHashUpstream(
					&sd_config.UpstreamConfig{Name: "test", KeyExtractor: keyConfig},
					testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
				)
			})
		})
	}
}

func TestCHashUpstreamSelectPeerReturnsNilForShortPayload(t *testing.T) {
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:         "test",
			KeyExtractor: fixedKeyExtractorConfig("1:3", ""),
		},
		testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
	)

	require.Nil(t, upstream.SelectPeer([]byte{0x01, 0x02}))
}

func TestCHashUpstreamSelectPeerFallsBackToRRForShortPayload(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:             "test",
			KeyExtractor:     fixedKeyExtractorConfig("1:3", ""),
			InvalidKeyPolicy: invalidKeyPolicyRR,
		},
		testPeerGroup(1, peer),
	)

	require.Same(t, peer, upstream.SelectPeer([]byte{0x01, 0x02}))
}

func TestNewCHashUpstreamRejectsInvalidKeyPolicy(t *testing.T) {
	require.Panics(t, func() {
		newCHashUpstream(
			&sd_config.UpstreamConfig{
				Name:             "test",
				KeyExtractor:     fixedKeyExtractorConfig("0:1", ""),
				InvalidKeyPolicy: "invalid",
			},
			testPeerGroup(1, testPeer("127.0.0.1:10001", 1)),
		)
	})
}

func TestCHashUpstreamSelectPeerReturnsNilForEmptySnapshot(t *testing.T) {
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:         "test",
			KeyExtractor: fixedKeyExtractorConfig("0:1", ""),
		},
		testPeerGroup(1),
	)

	require.Nil(t, upstream.SelectPeer([]byte{0x01}))
}

func TestCHashUpstreamSelectPeerSyncsNewSnapshot(t *testing.T) {
	first := testPeer("127.0.0.1:10001", 1)
	second := testPeer("127.0.0.1:10002", 1)
	group := testPeerGroup(1, first)
	upstream := newCHashUpstream(
		&sd_config.UpstreamConfig{
			Name:         "test",
			KeyExtractor: fixedKeyExtractorConfig("0:1", ""),
		},
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

func TestNewUpstreamConvey(t *testing.T) {
	convey.Convey("Test NewUpstream", t, func() {
		convey.Convey("selects the round-robin implementation", func() {
			group := testPeerGroup(1, testPeer("127.0.0.1:10001", 1))
			got := NewUpstream(&sd_config.UpstreamConfig{Type: UpstreamTypeRR}, group)
			convey.So(got, convey.ShouldHaveSameTypeAs, &rrUpstream{})
		})
	})
}

func TestInitUpstreamPeersConvey(t *testing.T) {
	convey.Convey("Test initUpstreamPeers", t, func() {
		convey.Convey("builds primary and backup peers", func() {
			peers, backup := initUpstreamPeers(&sd_config.UpstreamConfig{
				Name: "test",
				Peers: []*sd_config.PeerConfig{
					{IP: "127.0.0.1", Port: 10001, Weight: 1},
					{IP: "127.0.0.1", Port: 10002, Weight: 1, Backup: true},
				},
			})
			convey.So(peers, convey.ShouldHaveLength, 2)
			convey.So(backup, convey.ShouldEqual, peers[1])
		})
	})
}

func TestNewRRUpstreamConvey(t *testing.T) {
	convey.Convey("Test newRRUpstream", t, func() {
		convey.Convey("synchronizes the initial snapshot", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			upstream := newRRUpstream(testPeerGroup(2, peer))
			convey.So(upstream.snapshotVersion, convey.ShouldEqual, 2)
			convey.So(upstream.rrList, convey.ShouldResemble, []*Peer{peer})
		})
	})
}

func TestRrUpstreamSelectPeerConvey(t *testing.T) {
	convey.Convey("Test rrUpstream.SelectPeer", t, func() {
		convey.Convey("advances through the list", func() {
			first := testPeer("127.0.0.1:10001", 1)
			second := testPeer("127.0.0.1:10002", 1)
			upstream := newRRUpstream(testPeerGroup(1, first, second))
			convey.So(upstream.SelectPeer(nil), convey.ShouldEqual, first)
			convey.So(upstream.SelectPeer(nil), convey.ShouldEqual, second)
		})
	})
}

func TestRrUpstreamMarkPeerDeadConvey(t *testing.T) {
	convey.Convey("Test rrUpstream.MarkPeerDead", t, func() {
		convey.Convey("updates the group", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			group := &PeerGroup{peers: []*Peer{peer}}
			group.refresh()
			newRRUpstream(group).MarkPeerDead(peer)
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
		})
	})
}

func TestRrUpstreamSyncSnapshotConvey(t *testing.T) {
	convey.Convey("Test rrUpstream.syncSnapshot", t, func() {
		convey.Convey("rebuilds on version change", func() {
			first := testPeer("127.0.0.1:10001", 1)
			group := testPeerGroup(1, first)
			upstream := newRRUpstream(group)
			second := testPeer("127.0.0.1:10002", 1)
			publishTestSnapshot(group, 2, second)
			upstream.syncSnapshot()
			convey.So(upstream.snapshotVersion, convey.ShouldEqual, 2)
			convey.So(upstream.rrList, convey.ShouldResemble, []*Peer{second})
		})
	})
}

func TestBuildRRListConvey(t *testing.T) {
	convey.Convey("Test buildRRList", t, func() {
		convey.Convey("expands normalized weights", func() {
			first := testPeer("127.0.0.1:10001", 2)
			second := testPeer("127.0.0.1:10002", 1)
			got := buildRRList([]*Peer{first, second})
			convey.So(got, convey.ShouldHaveLength, 3)
			convey.So(countPeer(got, first), convey.ShouldEqual, 2)
		})
	})
}

func TestGreatestCommonDivisorConvey(t *testing.T) {
	convey.Convey("Test greatestCommonDivisor", t, func() {
		convey.Convey("returns the common divisor", func() {
			convey.So(greatestCommonDivisor(18, 12), convey.ShouldEqual, 6)
		})
	})
}

func TestNewCHashUpstreamConvey(t *testing.T) {
	convey.Convey("Test newCHashUpstream", t, func() {
		convey.Convey("creates a fixed-key implementation", func() {
			upstream := newCHashUpstream(&sd_config.UpstreamConfig{
				Name: "hash",
				KeyExtractor: &sd_config.KeyExtractorConfig{
					Type:  keyExtractorTypeFixed,
					Fixed: &sd_config.FixedKeyConfig{KeyBytes: "0:1"},
				},
			}, testPeerGroup(1, testPeer("127.0.0.1:10001", 1)))
			convey.So(upstream.cHash, convey.ShouldNotBeNil)
			convey.So(upstream.key, convey.ShouldHaveSameTypeAs, &fixedKeyExtractor{})
		})
	})
}

func TestCHashUpstreamSelectPeerConvey(t *testing.T) {
	convey.Convey("Test cHashUpstream.SelectPeer", t, func() {
		convey.Convey("hashes a valid key", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			upstream := newCHashUpstream(&sd_config.UpstreamConfig{
				Name: "hash",
				KeyExtractor: &sd_config.KeyExtractorConfig{
					Type:  keyExtractorTypeFixed,
					Fixed: &sd_config.FixedKeyConfig{KeyBytes: "0:1"},
				},
			}, testPeerGroup(1, peer))
			convey.So(upstream.SelectPeer([]byte{1}), convey.ShouldEqual, peer)
		})
	})
}

func TestCHashUpstreamMarkPeerDeadConvey(t *testing.T) {
	convey.Convey("Test cHashUpstream.MarkPeerDead", t, func() {
		convey.Convey("updates the group", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			group := &PeerGroup{peers: []*Peer{peer}}
			group.refresh()
			upstream := &cHashUpstream{group: group}
			upstream.MarkPeerDead(peer)
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
		})
	})
}

func TestCHashUpstreamSyncSnapshotConvey(t *testing.T) {
	convey.Convey("Test cHashUpstream.syncSnapshot", t, func() {
		convey.Convey("updates the lookup table", func() {
			peer := testPeer("127.0.0.1:10001", 1)
			group := testPeerGroup(2, peer)
			upstream := &cHashUpstream{group: group, cHash: newConsistentHash(nil)}
			upstream.syncSnapshot()
			convey.So(upstream.snapshotVersion, convey.ShouldEqual, 2)
			convey.So(upstream.cHash.selectPeer([]byte("key")), convey.ShouldEqual, peer)
		})
	})
}
