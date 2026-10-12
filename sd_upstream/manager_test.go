package sd_upstream

import (
	"fmt"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/stretchr/testify/require"
)

type stubUpstream struct {
	name string
}

func (u *stubUpstream) SelectPeer([]byte) *Peer {
	return nil
}

func (u *stubUpstream) MarkPeerDead(*Peer) {}

func TestNewRegistryWithoutUpstreams(t *testing.T) {
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

	registry := NewRegistry(config)

	require.Empty(t, registry.configs)
	require.Empty(t, registry.groups)
	require.Empty(t, registry.router.Route([]byte{0x01}))
	require.Empty(t, registry.router.Route([]byte{0x02}))
}

func TestNewRegistryRejectsMissingDefaultUpstream(t *testing.T) {
	require.PanicsWithValue(t, `default upstream "missing" does not exist`, func() {
		NewRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "missing",
		})
	})
}

func TestNewRegistryRejectsMissingRouteUpstream(t *testing.T) {
	require.PanicsWithValue(t, `upstream "missing" for route 0 does not exist`, func() {
		NewRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "default",
			Upstreams: []*sd_config.UpstreamConfig{
				{Name: "default", Type: UpstreamTypeRR},
			},
			Routes: []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
					},
					Upstream: "missing",
				},
			},
		})
	})
}

func TestNewRegistryRejectsReservedDropUpstreamName(t *testing.T) {
	for _, name := range []string{"", sd_route.DropUpstream} {
		require.PanicsWithValue(t, fmt.Sprintf("upstream name %q is reserved", name), func() {
			NewRegistry(&sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
				Upstreams: []*sd_config.UpstreamConfig{
					{Name: name, Type: UpstreamTypeRR},
				},
			})
		})
	}
}

func TestNewRegistryRejectsDuplicateUpstreamNames(t *testing.T) {
	require.PanicsWithValue(t, `upstream name "default" is duplicated`, func() {
		NewRegistry(&sd_config.UploadConfig{
			DefaultUpstream: "default",
			Upstreams: []*sd_config.UpstreamConfig{
				{Name: "default", Type: UpstreamTypeRR},
				{Name: "default", Type: UpstreamTypeRR},
			},
		})
	})
}

func TestRegistryNewManager(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	config := &sd_config.UpstreamConfig{
		Name: "default",
		Type: UpstreamTypeRR,
	}
	registry := &Registry{
		configs: []*sd_config.UpstreamConfig{config},
		groups:  map[string]*PeerGroup{"default": testPeerGroup(1, peer)},
		router:  sd_route.NewRouter("default", nil),
	}

	manager := registry.NewManager()

	require.IsType(t, &RRUpstream{}, manager.upstreams["default"])
	require.Same(t, manager.upstreams["default"], manager.RouteUpstream(nil))
}

func TestRegistryExposesRouteStats(t *testing.T) {
	router := sd_route.NewRouter(sd_route.DropUpstream, nil)
	registry := &Registry{router: router}

	provider, ok := any(registry).(interface {
		EnableRouteStats()
		RouteStats() sd_route.Stats
	})
	require.True(t, ok)
	provider.EnableRouteStats()
	router.Route(nil)

	stats := provider.RouteStats()
	require.Equal(t, uint64(1), stats.DefaultMatches)
	require.Equal(t, uint64(1), stats.Drops)
}

func TestManagerRouteUpstreamUsesFirstMatchingRoute(t *testing.T) {
	first := &stubUpstream{name: "first"}
	second := &stubUpstream{name: "second"}
	manager := &Manager{
		upstreams: map[string]Upstream{
			"first":  first,
			"second": second,
		},
		router: sd_route.NewRouter("", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "first",
			},
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "second",
			},
		}),
	}

	require.Same(t, first, manager.RouteUpstream([]byte{0x01}))
}

func TestManagerRouteUpstreamUsesDefaultWithoutMatch(t *testing.T) {
	fallback := &stubUpstream{name: "default"}
	manager := &Manager{
		upstreams: map[string]Upstream{"default": fallback},
		router: sd_route.NewRouter("default", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "target",
			},
		}),
	}

	require.Same(t, fallback, manager.RouteUpstream([]byte{0x02}))
}

func TestManagerRouteUpstreamStopsAtMissingFirstMatch(t *testing.T) {
	second := &stubUpstream{name: "second"}
	manager := &Manager{
		upstreams: map[string]Upstream{
			"second": second,
		},
		router: sd_route.NewRouter("", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "deny",
			},
			{
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "second",
			},
		}),
	}

	require.Nil(t, manager.RouteUpstream([]byte{0x01}))
}
