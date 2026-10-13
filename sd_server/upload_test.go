package sd_server

import (
	"fmt"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/stretchr/testify/require"
)

type stubUpstream struct {
	name     string
	peer     *sd_upstream.Peer
	deadPeer *sd_upstream.Peer
}

func (u *stubUpstream) SelectPeer([]byte) *sd_upstream.Peer {
	return u.peer
}

func (u *stubUpstream) MarkPeerDead(peer *sd_upstream.Peer) {
	u.deadPeer = peer
}

func newTestUploadManager(upstreams map[string]sd_upstream.Upstream, router *sd_route.Router) *uploadManager {
	return &uploadManager{
		upstreamManager: sd_upstream.NewManager(upstreams, false),
		router:          router,
	}
}

func TestNewUploadRegistryWithoutUpstreams(t *testing.T) {
	config := &sd_config.UploadConfig{
		DefaultUpstream: sd_route.DropUpstream,
		Routes: []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
				},
				Upstream: "",
			},
		},
	}

	registry := newUploadRegistry(config, false)
	manager := registry.newManager()

	require.Empty(t, registry.upstreamRegistry.definitions)
	require.Empty(t, manager.routeUpstream([]byte{0x01}))
	require.Empty(t, manager.routeUpstream([]byte{0x02}))
}

func TestNewUploadRegistryRejectsMissingDefaultUpstream(t *testing.T) {
	require.PanicsWithValue(t, `default upstream "missing" does not exist`, func() {
		newUploadRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "missing",
		}, false)
	})
}

func TestNewUploadRegistryRejectsMissingRouteUpstream(t *testing.T) {
	require.PanicsWithValue(t, `upstream "missing" for route 0 does not exist`, func() {
		newUploadRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "default",
			Upstreams: []*sd_config.UpstreamConfig{
				{Name: "default", Type: sd_upstream.UpstreamTypeRR},
			},
			Routes: []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
					},
					Upstream: "missing",
				},
			},
		}, false)
	})
}

func TestNewUploadRegistryRejectsReservedDropUpstreamName(t *testing.T) {
	for _, name := range []string{"", sd_route.DropUpstream} {
		require.PanicsWithValue(t, fmt.Sprintf("upstream name %q is reserved", name), func() {
			newUploadRegistry(&sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
				Upstreams: []*sd_config.UpstreamConfig{
					{Name: name, Type: sd_upstream.UpstreamTypeRR},
				},
			}, false)
		})
	}
}

func TestNewUploadRegistryRejectsDuplicateUpstreamNames(t *testing.T) {
	require.PanicsWithValue(t, `upstream name "default" is duplicated`, func() {
		newUploadRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "default",
			Upstreams: []*sd_config.UpstreamConfig{
				{Name: "default", Type: sd_upstream.UpstreamTypeRR},
				{Name: "default", Type: sd_upstream.UpstreamTypeRR},
			},
		}, false)
	})
}

func TestUploadRegistryNewManager(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: "default",
		Upstreams: []*sd_config.UpstreamConfig{
			{
				Name: "default",
				Type: sd_upstream.UpstreamTypeRR,
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
				},
			},
		},
	}, false)

	firstManager := registry.newManager()
	secondManager := registry.newManager()

	require.NotSame(t, firstManager.upstreamManager, secondManager.upstreamManager)
	upstreamName := firstManager.routeUpstream(nil)
	_, ok := firstManager.selectPeer(upstreamName, nil)
	require.True(t, ok)
	upstreamName = secondManager.routeUpstream(nil)
	_, ok = secondManager.selectPeer(upstreamName, nil)
	require.True(t, ok)
}

func TestUploadRegistryPreservesLengthFieldKeyExtractorConfig(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: "default",
		Upstreams: []*sd_config.UpstreamConfig{
			{
				Name: "default",
				Type: sd_upstream.UpstreamTypeCHash,
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
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
				},
			},
		},
	}, false)

	require.NotPanics(t, func() {
		registry.newManager()
	})
}

func TestUploadManagerStatsIncludeRouteStats(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: sd_route.DropUpstream,
	}, true)
	manager := registry.newManager()
	manager.routeUpstream(nil)

	stats := manager.stats().Route
	require.Equal(t, uint64(1), stats.DefaultRoutes)
	require.Equal(t, uint64(1), stats.Drops)
}

func TestUploadManagerUpstreamStatsDisabledByDefault(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: "default",
		Upstreams: []*sd_config.UpstreamConfig{
			{
				Name: "default",
				Type: sd_upstream.UpstreamTypeRR,
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
				},
			},
		},
	}, false)
	manager := registry.newManager()
	upstreamName := manager.routeUpstream(nil)
	peer, ok := manager.selectPeer(upstreamName, nil)

	require.True(t, ok)
	require.Nil(t, peer)
	require.Equal(t, []sd_upstream.Stats{
		{Name: "default"},
	}, manager.stats().Upstreams)
}

func TestUploadManagerMarksPeerDead(t *testing.T) {
	peer := &sd_upstream.Peer{}
	upstream := &stubUpstream{}
	manager := &uploadManager{
		upstreamManager: sd_upstream.NewManager(map[string]sd_upstream.Upstream{
			"default": upstream,
		}, true),
		router: sd_route.NewRouter("default", nil, true),
	}

	manager.markPeerDead("default", peer)

	require.Same(t, peer, upstream.deadPeer)
	require.Equal(t, uint64(1), manager.stats().Upstreams[0].PeerFailures)
}

func TestServerAggregatesUpstreamStatsAcrossManagers(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: "default",
		Upstreams: []*sd_config.UpstreamConfig{
			{
				Name: "default",
				Type: sd_upstream.UpstreamTypeRR,
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
				},
			},
		},
	}, true)

	firstManager := registry.newManager()
	secondManager := registry.newManager()
	firstName := firstManager.routeUpstream(nil)
	firstPeer, ok := firstManager.selectPeer(firstName, nil)
	require.True(t, ok)
	secondName := secondManager.routeUpstream(nil)
	secondPeer, ok := secondManager.selectPeer(secondName, nil)
	require.True(t, ok)
	require.Nil(t, firstPeer)
	require.Nil(t, secondPeer)

	managerStats := []sd_upstream.Stats{
		{
			Name:              "default",
			SelectionFailures: 1,
		},
	}
	require.Equal(t, managerStats, firstManager.stats().Upstreams)
	require.Equal(t, managerStats, secondManager.stats().Upstreams)
	server := &Server{
		uploadRegistry: registry,
		reactors: []*Reactor{
			{upstreamMgr: firstManager},
			{upstreamMgr: secondManager},
		},
	}
	require.Equal(t, []sd_upstream.Stats{
		{
			Name:              "default",
			SelectionFailures: 2,
		},
	}, server.uploadStats().Upstreams)
}

func TestServerAggregatesRouteStatsAcrossManagers(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: sd_route.DropUpstream,
	}, true)
	first := registry.newManager()
	second := registry.newManager()

	first.routeUpstream(nil)
	second.routeUpstream(nil)

	require.Equal(t, uint64(1), first.stats().Route.DefaultRoutes)
	require.Equal(t, uint64(1), first.stats().Route.Drops)
	require.Equal(t, uint64(1), second.stats().Route.DefaultRoutes)
	require.Equal(t, uint64(1), second.stats().Route.Drops)
	server := &Server{
		uploadRegistry: registry,
		reactors: []*Reactor{
			{upstreamMgr: first},
			{upstreamMgr: second},
		},
	}
	require.Equal(t, uint64(2), server.uploadStats().Route.DefaultRoutes)
	require.Equal(t, uint64(2), server.uploadStats().Route.Drops)
}

func TestUploadManagerUsesFirstMatchingRoute(t *testing.T) {
	firstPeer := &sd_upstream.Peer{}
	first := &stubUpstream{name: "first", peer: firstPeer}
	second := &stubUpstream{name: "second", peer: &sd_upstream.Peer{}}
	manager := newTestUploadManager(
		map[string]sd_upstream.Upstream{
			"first":  first,
			"second": second,
		},
		sd_route.NewRouter("", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "first",
			},
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "second",
			},
		}, false),
	)

	upstreamName := manager.routeUpstream([]byte{0x01})
	peer, ok := manager.selectPeer(upstreamName, nil)
	require.True(t, ok)
	require.Same(t, firstPeer, peer)
}

func TestUploadManagerUsesDefaultWithoutMatch(t *testing.T) {
	peer := &sd_upstream.Peer{}
	fallback := &stubUpstream{name: "default", peer: peer}
	manager := newTestUploadManager(
		map[string]sd_upstream.Upstream{"default": fallback},
		sd_route.NewRouter("default", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "target",
			},
		}, false),
	)

	upstreamName := manager.routeUpstream([]byte{0x02})
	selected, ok := manager.selectPeer(upstreamName, nil)
	require.True(t, ok)
	require.Same(t, peer, selected)
}

func TestUploadManagerStopsAtMissingFirstMatch(t *testing.T) {
	second := &stubUpstream{name: "second"}
	manager := newTestUploadManager(
		map[string]sd_upstream.Upstream{
			"second": second,
		},
		sd_route.NewRouter("", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "deny",
			},
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "second",
			},
		}, false),
	)

	upstreamName := manager.routeUpstream([]byte{0x01})
	_, ok := manager.selectPeer(upstreamName, nil)
	require.False(t, ok)
}
