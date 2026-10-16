package sd_server

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/smartystreets/goconvey/convey"
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
		upstreamMgr: sd_upstream.NewManager(upstreams, false),
		router:      router,
	}
}

func mustNewRouter(
	t testing.TB,
	defaultUpstream string,
	configs []sd_config.RouteConfig,
	statsEnabled bool,
) *sd_route.Router {
	t.Helper()
	router, err := sd_route.NewRouter(defaultUpstream, configs, statsEnabled)
	require.NoError(t, err)
	return router
}

func mustNewUploadRegistry(
	t testing.TB,
	config *sd_config.UploadConfig,
	statsEnabled bool,
) *uploadRegistry {
	t.Helper()
	registry, err := newUploadRegistry(config, statsEnabled)
	require.NoError(t, err)
	t.Cleanup(registry.close)
	return registry
}

func mustNewUploadManager(t testing.TB, registry *uploadRegistry) *uploadManager {
	t.Helper()
	manager, err := registry.newManager()
	require.NoError(t, err)
	return manager
}

func mustNewPeer(t testing.TB, id int, addr string, config *sd_config.PeerConfig) *sd_upstream.Peer {
	t.Helper()
	peer, err := sd_upstream.NewPeer(id, addr, config)
	require.NoError(t, err)
	return peer
}

func TestNewUploadRegistryConvey(t *testing.T) {
	convey.Convey("Test newUploadRegistry", t, func() {
		convey.Convey("builds router and definitions", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: "default",
				Upstreams: []*sd_config.UpstreamConfig{{
					Name: "default", Type: sd_upstream.UpstreamTypeRR,
					HealthChecker: &sd_config.HealthCheckerConfig{
						HeartbeatIntervalSec: 3600,
						SuccessTimes:         1,
						FailedTimes:          1,
					},
				}},
			}, true)
			convey.So(registry.router, convey.ShouldNotBeNil)
			convey.So(registry.upstreamRegistry.definitions, convey.ShouldHaveLength, 1)
		})
		convey.Convey("supports configs without upstreams", func() {
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

			registry := mustNewUploadRegistry(t, config, false)
			manager := mustNewUploadManager(t, registry)

			require.Empty(t, registry.upstreamRegistry.definitions)
			require.Empty(t, manager.routeUpstream([]byte{0x01}))
			require.Empty(t, manager.routeUpstream([]byte{0x02}))
		})
		convey.Convey("rejects a missing default upstream", func() {
			registry, err := newUploadRegistry(&sd_config.UploadConfig{
				DefaultUpstream: "missing",
			}, false)

			require.Nil(t, registry)
			require.EqualError(t, err, `default upstream "missing" does not exist`)
		})
		convey.Convey("rejects a missing route upstream", func() {
			registry, err := newUploadRegistry(&sd_config.UploadConfig{
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

			require.Nil(t, registry)
			require.EqualError(t, err, `upstream "missing" for route 0 does not exist`)
		})
		convey.Convey("rejects reserved upstream names", func() {
			for _, name := range []string{"", sd_route.DropUpstream} {
				registry, err := newUploadRegistry(&sd_config.UploadConfig{
					DefaultUpstream: sd_route.DropUpstream,
					Upstreams: []*sd_config.UpstreamConfig{
						{Name: name, Type: sd_upstream.UpstreamTypeRR},
					},
				}, false)

				require.Nil(t, registry)
				require.EqualError(t, err, `upstream name "`+name+`" is reserved`)
			}
		})
		convey.Convey("rejects duplicate upstream names", func() {
			registry, err := newUploadRegistry(&sd_config.UploadConfig{
				DefaultUpstream: "default",
				Upstreams: []*sd_config.UpstreamConfig{
					{Name: "default", Type: sd_upstream.UpstreamTypeRR},
					{Name: "default", Type: sd_upstream.UpstreamTypeRR},
				},
			}, false)

			require.Nil(t, registry)
			require.EqualError(t, err, `upstream name "default" is duplicated`)
		})
		convey.Convey("preserves length-field extractor config", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
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
							SuccessTimes:         1,
							FailedTimes:          1,
						},
					},
				},
			}, false)

			require.NotPanics(t, func() {
				mustNewUploadManager(t, registry)
			})
		})
	})
}

func TestUploadRegistryNewManagerConvey(t *testing.T) {
	convey.Convey("Test uploadRegistry.newManager", t, func() {
		convey.Convey("clones runtime state", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, false)
			manager := mustNewUploadManager(t, registry)
			convey.So(manager, convey.ShouldNotBeNil)
			convey.So(manager.router == registry.router, convey.ShouldBeFalse)
		})
		convey.Convey("creates independent runtime managers", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: "default",
				Upstreams: []*sd_config.UpstreamConfig{
					{
						Name: "default",
						Type: sd_upstream.UpstreamTypeRR,
						HealthChecker: &sd_config.HealthCheckerConfig{
							HeartbeatIntervalSec: 3600,
							SuccessTimes:         1,
							FailedTimes:          1,
						},
					},
				},
			}, false)

			firstManager := mustNewUploadManager(t, registry)
			secondManager := mustNewUploadManager(t, registry)

			require.NotSame(t, firstManager.upstreamMgr, secondManager.upstreamMgr)
			upstreamName := firstManager.routeUpstream(nil)
			_, ok := firstManager.selectPeer(upstreamName, nil)
			require.True(t, ok)
			upstreamName = secondManager.routeUpstream(nil)
			_, ok = secondManager.selectPeer(upstreamName, nil)
			require.True(t, ok)
		})
	})
}

func TestUpstreamRegistryNewManagerConvey(t *testing.T) {
	convey.Convey("Test upstreamRegistry.newManager", t, func() {
		convey.Convey("builds named upstreams", func() {
			stub := &stubUpstream{}
			patches := gomonkey.ApplyFuncReturn(sd_upstream.NewUpstream, stub, nil)
			defer patches.Reset()
			registry := &upstreamRegistry{definitions: []upstreamDefinition{{name: "default"}}}
			manager, err := registry.newManager()
			convey.So(err, convey.ShouldBeNil)
			_, ok := manager.SelectPeer("default", nil)
			convey.So(ok, convey.ShouldBeTrue)
		})
	})
}

func TestUpstreamDefinitionNewUpstreamConvey(t *testing.T) {
	convey.Convey("Test upstreamDefinition.newUpstream", t, func() {
		convey.Convey("delegates configuration", func() {
			stub := &stubUpstream{}
			patches := gomonkey.ApplyFuncReturn(sd_upstream.NewUpstream, stub, nil)
			defer patches.Reset()
			definition := &upstreamDefinition{name: "default", upstreamType: sd_upstream.UpstreamTypeRR}
			upstream, err := definition.newUpstream()
			convey.So(err, convey.ShouldBeNil)
			convey.So(upstream, convey.ShouldEqual, stub)
		})
	})
}

func TestUploadManagerRouteUpstreamConvey(t *testing.T) {
	convey.Convey("Test uploadManager.routeUpstream", t, func() {
		convey.Convey("delegates to router", func() {
			manager := newTestUploadManager(nil, mustNewRouter(t, "default", nil, false))
			convey.So(manager.routeUpstream(nil), convey.ShouldEqual, "default")
		})
		convey.Convey("uses the first matching route", func() {
			firstPeer := &sd_upstream.Peer{}
			first := &stubUpstream{name: "first", peer: firstPeer}
			second := &stubUpstream{name: "second", peer: &sd_upstream.Peer{}}
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{
					"first":  first,
					"second": second,
				},
				mustNewRouter(t, "", []sd_config.RouteConfig{
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
		})
		convey.Convey("uses the default without a match", func() {
			peer := &sd_upstream.Peer{}
			fallback := &stubUpstream{name: "default", peer: peer}
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{"default": fallback},
				mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("stops at a missing first match", func() {
			second := &stubUpstream{name: "second"}
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{
					"second": second,
				},
				mustNewRouter(t, "", []sd_config.RouteConfig{
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
		})
	})
}

func TestUploadManagerSelectPeerConvey(t *testing.T) {
	convey.Convey("Test uploadManager.selectPeer", t, func() {
		convey.Convey("delegates to manager", func() {
			peer := &sd_upstream.Peer{}
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{"default": &stubUpstream{peer: peer}},
				mustNewRouter(t, "default", nil, false),
			)
			got, ok := manager.selectPeer("default", nil)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(got, convey.ShouldEqual, peer)
		})
	})
}

func TestUploadManagerMarkPeerDeadConvey(t *testing.T) {
	convey.Convey("Test uploadManager.markPeerDead", t, func() {
		convey.Convey("delegates to manager", func() {
			peer := &sd_upstream.Peer{}
			stub := &stubUpstream{}
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{"default": stub},
				mustNewRouter(t, "default", nil, false),
			)
			manager.markPeerDead("default", peer)
			convey.So(stub.deadPeer, convey.ShouldEqual, peer)
		})
		convey.Convey("records peer failure stats", func() {
			peer := &sd_upstream.Peer{}
			upstream := &stubUpstream{}
			manager := &uploadManager{
				upstreamMgr: sd_upstream.NewManager(map[string]sd_upstream.Upstream{
					"default": upstream,
				}, true),
				router: mustNewRouter(t, "default", nil, true),
			}

			manager.markPeerDead("default", peer)

			require.Same(t, peer, upstream.deadPeer)
			require.Equal(t, uint64(1), manager.stats().Upstreams[0].PeerFailures)
		})
	})
}

func TestUploadManagerStatsConvey(t *testing.T) {
	convey.Convey("Test uploadManager.stats", t, func() {
		convey.Convey("combines route and upstream stats", func() {
			manager := newTestUploadManager(
				map[string]sd_upstream.Upstream{"default": &stubUpstream{}},
				mustNewRouter(t, "default", nil, true),
			)
			stats := manager.stats()
			convey.So(stats.Route.Routes, convey.ShouldBeEmpty)
			convey.So(stats.Upstreams, convey.ShouldHaveLength, 1)
		})
		convey.Convey("includes route stats", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, true)
			manager := mustNewUploadManager(t, registry)
			manager.routeUpstream(nil)

			stats := manager.stats().Route
			require.Equal(t, uint64(1), stats.DefaultRoutes)
			require.Equal(t, uint64(1), stats.Drops)
		})
		convey.Convey("keeps upstream stats disabled by default", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: "default",
				Upstreams: []*sd_config.UpstreamConfig{
					{
						Name: "default",
						Type: sd_upstream.UpstreamTypeRR,
						HealthChecker: &sd_config.HealthCheckerConfig{
							HeartbeatIntervalSec: 3600,
							SuccessTimes:         1,
							FailedTimes:          1,
						},
					},
				},
			}, false)
			manager := mustNewUploadManager(t, registry)
			upstreamName := manager.routeUpstream(nil)
			peer, ok := manager.selectPeer(upstreamName, nil)

			require.True(t, ok)
			require.Nil(t, peer)
			require.Equal(t, []sd_upstream.Stats{
				{Name: "default"},
			}, manager.stats().Upstreams)
		})
	})
}

func TestValidateUpstreamTargetsConvey(t *testing.T) {
	convey.Convey("Test validateUpstreamTargets", t, func() {
		convey.Convey("accepts existing names", func() {
			convey.So(func() {
				validateUpstreamTargets(
					&sd_config.UploadConfig{DefaultUpstream: "default"},
					map[string]struct{}{"default": {}},
				)
			}, convey.ShouldNotPanic)
		})
	})
}
