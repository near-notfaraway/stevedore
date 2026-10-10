package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
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
		DefaultUpstream: "missing",
		Routes: []sd_config.RouteConfig{
			{
				KeyBytes: "0:1",
				Operator: "==",
				Value:    "0x01",
				Upstream: "target",
			},
		},
	}

	registry := NewRegistry(config)

	require.Empty(t, registry.configs)
	require.Empty(t, registry.groups)
	require.Equal(t, "missing", registry.defaultUpstream)
	require.Len(t, registry.routes, 1)
}

func TestRegistryNewManager(t *testing.T) {
	peer := testPeer("127.0.0.1:10001", 1)
	config := &sd_config.UpstreamConfig{
		Name: "default",
		Type: UpstreamTypeRR,
	}
	registry := &Registry{
		configs:         []*sd_config.UpstreamConfig{config},
		groups:          map[string]*PeerGroup{"default": testPeerGroup(1, peer)},
		defaultUpstream: "default",
	}

	manager := registry.NewManager()

	require.IsType(t, &RRUpstream{}, manager.defaultUpstream)
	require.Same(t, manager.defaultUpstream, manager.upstreams["default"])
}

func TestManagerRouteUpstreamUsesFirstMatchingRoute(t *testing.T) {
	first := &stubUpstream{name: "first"}
	second := &stubUpstream{name: "second"}
	manager := &Manager{
		upstreams: map[string]Upstream{
			"first":  first,
			"second": second,
		},
		routes: []*Route{
			NewRoute(0, sd_config.RouteConfig{KeyBytes: "0:1", Operator: "==", Value: "0x01", Upstream: "first"}),
			NewRoute(1, sd_config.RouteConfig{KeyBytes: "0:1", Operator: "==", Value: "0x01", Upstream: "second"}),
		},
	}

	require.Same(t, first, manager.RouteUpstream([]byte{0x01}))
}

func TestManagerRouteUpstreamUsesDefaultWithoutMatch(t *testing.T) {
	fallback := &stubUpstream{name: "default"}
	manager := &Manager{
		defaultUpstream: fallback,
		upstreams:       map[string]Upstream{},
		routes: []*Route{
			NewRoute(0, sd_config.RouteConfig{KeyBytes: "0:1", Operator: "==", Value: "0x01", Upstream: "target"}),
		},
	}

	require.Same(t, fallback, manager.RouteUpstream([]byte{0x02}))
}

func TestManagerRouteUpstreamStopsAtMissingFirstMatch(t *testing.T) {
	second := &stubUpstream{name: "second"}
	manager := &Manager{
		upstreams: map[string]Upstream{
			"second": second,
		},
		routes: []*Route{
			NewRoute(0, sd_config.RouteConfig{KeyBytes: "0:1", Operator: "==", Value: "0x01", Upstream: "deny"}),
			NewRoute(1, sd_config.RouteConfig{KeyBytes: "0:1", Operator: "==", Value: "0x01", Upstream: "second"}),
		},
	}

	require.Nil(t, manager.RouteUpstream([]byte{0x01}))
}
