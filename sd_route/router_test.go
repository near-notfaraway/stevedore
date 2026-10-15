package sd_route

import (
	"sync"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestRouterStatsCountNamedRouteMatches(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Name: "canary",
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, true)

	router.Route([]byte{0x01})
	router.Route([]byte{0x01})

	require.Equal(t, []RouteStats{
		{ID: 0, Name: "canary", Matches: 2},
	}, router.Stats().Routes)
}

func TestRouterStatsDisabledByDefault(t *testing.T) {
	router := NewRouter(DropUpstream, nil, false)

	router.Route(nil)

	stats := router.Stats()
	require.Zero(t, stats.DefaultRoutes)
	require.Zero(t, stats.ShortPayloads)
	require.Zero(t, stats.Drops)
}

func TestRouterStatsGenerateNameForLegacyRoute(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, false)

	require.Equal(t, "route-0", router.Stats().Routes[0].Name)
}

func TestRouterCloneSharesRulesAndKeepsIndependentStats(t *testing.T) {
	template := NewRouter("default", []sd_config.RouteConfig{
		{
			Name: "canary",
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, true)
	first := template.Clone()
	second := template.Clone()
	require.Same(t, template.routes[0], first.routes[0])
	require.Same(t, template.routes[0], second.routes[0])

	require.Equal(t, "target", first.Route([]byte{0x01}))
	require.Equal(t, "default", second.Route([]byte{0x02}))

	require.Equal(t, uint64(1), first.Stats().Routes[0].Matches)
	require.Zero(t, first.Stats().DefaultRoutes)
	require.Zero(t, second.Stats().Routes[0].Matches)
	require.Equal(t, uint64(1), second.Stats().DefaultRoutes)
	require.Zero(t, template.Stats().Routes[0].Matches)
	require.Zero(t, template.Stats().DefaultRoutes)
}

func TestRouterCloneKeepsStatsDisabled(t *testing.T) {
	template := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, false)
	clone := template.Clone()

	require.Equal(t, "target", clone.Route([]byte{0x01}))
	require.Equal(t, "default", clone.Route([]byte{0x02}))
	require.Zero(t, clone.Stats().Routes[0].Matches)
	require.Zero(t, clone.Stats().DefaultRoutes)
}

func TestRouterStatsConcurrentWithRouting(t *testing.T) {
	const matches = 1000

	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range matches {
			router.Route([]byte{0x01})
		}
	}()
	go func() {
		defer wg.Done()
		for range matches {
			router.Stats()
		}
	}()
	wg.Wait()

	require.Equal(t, uint64(matches), router.Stats().Routes[0].Matches)
}

func TestNewRouterRejectsDuplicateRouteNames(t *testing.T) {
	require.Panics(t, func() {
		NewRouter("default", []sd_config.RouteConfig{
			{
				Name:       "duplicate",
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
				Upstream:   "first",
			},
			{
				Name:       "duplicate",
				Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x02"}},
				Upstream:   "second",
			},
		}, false)
	})
}

func TestRouterStatsCountDefaultRoutesAndShortPayloads(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "1:2", Operator: "==", Value: "0x01"},
			},
			Upstream: "target",
		},
	}, true)

	router.Route([]byte{0x00, 0x02})
	router.Route([]byte{0x00})

	stats := router.Stats()
	require.Equal(t, uint64(2), stats.DefaultRoutes)
	require.Equal(t, uint64(1), stats.ShortPayloads)
}

func TestRouterStatsCountExplicitDrops(t *testing.T) {
	for _, target := range []string{"", DropUpstream} {
		router := NewRouter(target, []sd_config.RouteConfig{
			{
				Name: "blocked",
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
				},
				Upstream: target,
			},
		}, true)

		require.Empty(t, router.Route([]byte{0x01}))
		require.Empty(t, router.Route([]byte{0x02}))

		stats := router.Stats()
		require.Equal(t, uint64(2), stats.Drops)
		require.Equal(t, uint64(1), stats.Routes[0].Matches)
		require.Equal(t, uint64(1), stats.DefaultRoutes)
	}
}

func TestRouterUsesFirstMatchingRoute(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
			Upstream:   "first",
		},
		{
			Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
			Upstream:   "second",
		},
	}, false)

	require.Equal(t, "first", router.Route([]byte{0x01}))
}

func TestRouterUsesDefaultWithoutMatch(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
			Upstream:   "target",
		},
	}, false)

	require.Equal(t, "default", router.Route([]byte{0x02}))
	require.Equal(t, "default", router.Route(nil))
}

func TestRouterRequiresAllConditionsToMatch(t *testing.T) {
	var router *Router
	require.NotPanics(t, func() {
		router = NewRouter("default", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
					{KeyBytes: "1:2", Operator: "==", Value: "0x02"},
				},
				Upstream: "target",
			},
		}, false)
	})

	require.Equal(t, "target", router.Route([]byte{0x01, 0x02}))
	require.Equal(t, "default", router.Route([]byte{0x01, 0x03}))
	require.Equal(t, "default", router.Route([]byte{0x01}))
}

func TestNewRouterRejectsRouteWithoutConditions(t *testing.T) {
	require.Panics(t, func() {
		NewRouter("default", []sd_config.RouteConfig{
			{
				Upstream: "target",
			},
		}, false)
	})
}

func TestNewRouterConvey(t *testing.T) {
	convey.Convey("Test NewRouter", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("builds routes and normalizes the drop target", func() {
			router := NewRouter(DropUpstream, []sd_config.RouteConfig{config}, false)
			convey.So(router.defaultUpstream, convey.ShouldBeEmpty)
			convey.So(router.routes, convey.ShouldHaveLength, 1)
		})
	})
}

func TestRouterCloneConvey(t *testing.T) {
	convey.Convey("Test Router.Clone", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("shares immutable routes and isolates stats", func() {
			router := NewRouter("default", []sd_config.RouteConfig{config}, true)
			clone := router.Clone()
			convey.So(clone.routes[0], convey.ShouldEqual, router.routes[0])
			clone.Route([]byte{1})
			convey.So(clone.Stats().Routes[0].Matches, convey.ShouldEqual, 1)
			convey.So(router.Stats().Routes[0].Matches, convey.ShouldEqual, 0)
		})
	})
}

func TestRouterStatsConvey(t *testing.T) {
	convey.Convey("Test Router.Stats", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("snapshots atomic counters", func() {
			router := NewRouter("default", []sd_config.RouteConfig{config}, true)
			router.stats.routeMatches[0].Add(2)
			stats := router.Stats()
			convey.So(stats.Routes[0].Name, convey.ShouldEqual, "matched")
			convey.So(stats.Routes[0].Matches, convey.ShouldEqual, 2)
		})
	})
}

func TestRouterRouteConvey(t *testing.T) {
	convey.Convey("Test Router.Route", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("uses the no-stats route path", func() {
			router := NewRouter("default", []sd_config.RouteConfig{config}, false)
			convey.So(router.Route([]byte{1}), convey.ShouldEqual, "target")
			convey.So(router.Route([]byte{2}), convey.ShouldEqual, "default")
		})
	})
}

func TestRouterRouteWithStatsConvey(t *testing.T) {
	convey.Convey("Test Router.routeWithStats", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("records fallback metadata", func() {
			router := NewRouter("", []sd_config.RouteConfig{config}, true)
			convey.So(router.routeWithStats(nil), convey.ShouldBeEmpty)
			stats := router.Stats()
			convey.So(stats.DefaultRoutes, convey.ShouldEqual, 1)
			convey.So(stats.ShortPayloads, convey.ShouldEqual, 1)
			convey.So(stats.Drops, convey.ShouldEqual, 1)
		})
	})
}

func TestNewRouterStatsConvey(t *testing.T) {
	convey.Convey("Test newRouterStats", t, func() {
		convey.Convey("preserves route names and enabled state", func() {
			stats := newRouterStats([]*route{{name: "first"}}, true)
			convey.So(stats.enabled, convey.ShouldBeTrue)
			convey.So(stats.routeNames, convey.ShouldResemble, []string{"first"})
			convey.So(stats.routeMatches, convey.ShouldHaveLength, 1)
		})
	})
}

func TestNewRouteConvey(t *testing.T) {
	convey.Convey("Test newRoute", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("delegates condition construction", func() {
			patches := gomonkey.ApplyFuncReturn(newCondition, condition{
				operator:   sd_util.BytesOpEqual,
				bytesStart: 0,
				bytesEnd:   1,
				bytesValue: []byte{1},
			})
			defer patches.Reset()

			got := newRoute(3, sd_config.RouteConfig{
				Conditions: config.Conditions,
				Upstream:   DropUpstream,
			})
			convey.So(got.name, convey.ShouldEqual, "route-3")
			convey.So(got.upstream, convey.ShouldBeEmpty)
			convey.So(got.conditions, convey.ShouldHaveLength, 1)
		})
	})
}

func TestRouteMatchStatusConvey(t *testing.T) {
	convey.Convey("Test route.matchStatus", t, func() {
		convey.Convey("requires every condition", func() {
			got := &route{conditions: []condition{
				{operator: sd_util.BytesOpEqual, bytesStart: 0, bytesEnd: 1, bytesValue: []byte{1}},
				{operator: sd_util.BytesOpEqual, bytesStart: 1, bytesEnd: 2, bytesValue: []byte{2}},
			}}
			matched, short := got.matchStatus([]byte{1, 2})
			convey.So(matched, convey.ShouldBeTrue)
			convey.So(short, convey.ShouldBeFalse)

			matched, short = got.matchStatus([]byte{1})
			convey.So(matched, convey.ShouldBeFalse)
			convey.So(short, convey.ShouldBeTrue)
		})
	})
}
