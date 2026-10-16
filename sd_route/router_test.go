package sd_route

import (
	"sync"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func mustNewRouter(
	t testing.TB,
	defaultUpstream string,
	configs []sd_config.RouteConfig,
	statsEnabled bool,
) *Router {
	t.Helper()
	router, err := NewRouter(defaultUpstream, configs, statsEnabled)
	require.NoError(t, err)
	return router
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
			router := mustNewRouter(t, DropUpstream, []sd_config.RouteConfig{config}, false)
			convey.So(router.defaultUpstream, convey.ShouldBeEmpty)
			convey.So(router.routes, convey.ShouldHaveLength, 1)
		})
		convey.Convey("rejects duplicate route names", func() {
			router, err := NewRouter("default", []sd_config.RouteConfig{
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

			require.Nil(t, router)
			require.EqualError(t, err, `route name "duplicate" is duplicated`)
		})
		convey.Convey("rejects routes without conditions", func() {
			router, err := NewRouter("default", []sd_config.RouteConfig{
				{
					Upstream: "target",
				},
			}, false)

			require.Nil(t, router)
			require.EqualError(t, err, "conditions are required in route 0")
		})
		convey.Convey("rejects invalid operators", func() {
			router, err := NewRouter("default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "invalid", Value: "0x01"},
					},
					Upstream: "target",
				},
			}, false)

			require.Nil(t, router)
			require.Error(t, err)
		})
		convey.Convey("rejects malformed key byte ranges", func() {
			for _, keyBytes := range []string{"0", "0:1:2"} {
				router, err := NewRouter("default", []sd_config.RouteConfig{
					{
						Conditions: []sd_config.RouteConditionConfig{
							{KeyBytes: keyBytes, Operator: "==", Value: "0x01"},
						},
						Upstream: "target",
					},
				}, false)

				require.Nil(t, router)
				require.Error(t, err)
			}
		})
		convey.Convey("rejects invalid mask semantics", func() {
			tests := []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "&=", Value: "0x80", Mask: "0xc0"},
					},
					Upstream: "target",
				},
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "==", Value: "0xc0", Mask: "0x80"},
					},
					Upstream: "target",
				},
			}

			for _, config := range tests {
				router, err := NewRouter("default", []sd_config.RouteConfig{config}, false)

				require.Nil(t, router)
				require.Error(t, err)
			}
		})
		convey.Convey("rejects invalid condition configs", func() {
			tests := []sd_config.RouteConfig{
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "start:1", Operator: "==", Value: "0x01"}}},
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:end", Operator: "==", Value: "0x01"}}},
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "1:1", Operator: "==", Value: ""}}},
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "invalid"}}},
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:2", Operator: "==", Value: "0x01"}}},
				{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "-1:1", Operator: "==", Value: "0x0001"}}},
			}

			for _, config := range tests {
				router, err := NewRouter("", []sd_config.RouteConfig{config}, false)

				require.Nil(t, router)
				require.Error(t, err)
			}
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
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{config}, true)
			clone := router.Clone()
			convey.So(clone.routes[0], convey.ShouldEqual, router.routes[0])
			clone.Route([]byte{1})
			convey.So(clone.Stats().Routes[0].Matches, convey.ShouldEqual, 1)
			convey.So(router.Stats().Routes[0].Matches, convey.ShouldEqual, 0)
		})
		convey.Convey("shares rules while keeping independent stats", func() {
			template := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("keeps stats disabled", func() {
			template := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{config}, true)
			router.stats.routeMatches[0].Add(2)
			stats := router.Stats()
			convey.So(stats.Routes[0].Name, convey.ShouldEqual, "matched")
			convey.So(stats.Routes[0].Matches, convey.ShouldEqual, 2)
		})
		convey.Convey("counts named route matches", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("stays disabled by default", func() {
			router := mustNewRouter(t, DropUpstream, nil, false)

			router.Route(nil)

			stats := router.Stats()
			require.Zero(t, stats.DefaultRoutes)
			require.Zero(t, stats.ShortPayloads)
			require.Zero(t, stats.Drops)
		})
		convey.Convey("generates names for unnamed routes", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
					},
					Upstream: "target",
				},
			}, false)

			require.Equal(t, "route-0", router.Stats().Routes[0].Name)
		})
		convey.Convey("supports concurrent routing and snapshots", func() {
			const matches = 1000

			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("counts default routes and short payloads", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("counts explicit drops", func() {
			for _, target := range []string{"", DropUpstream} {
				router := mustNewRouter(t, target, []sd_config.RouteConfig{
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
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{config}, false)
			convey.So(router.Route([]byte{1}), convey.ShouldEqual, "target")
			convey.So(router.Route([]byte{2}), convey.ShouldEqual, "default")
		})
		convey.Convey("uses the first matching route", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("uses the default when no route matches", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "0x01"}},
					Upstream:   "target",
				},
			}, false)

			require.Equal(t, "default", router.Route([]byte{0x02}))
			require.Equal(t, "default", router.Route(nil))
		})
		convey.Convey("requires every condition to match", func() {
			var router *Router
			require.NotPanics(t, func() {
				router = mustNewRouter(t, "default", []sd_config.RouteConfig{
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
		})
		convey.Convey("matches masked values", func() {
			router := mustNewRouter(t, "default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: "0:1", Operator: "==", Value: "0x80", Mask: "0xc0"},
					},
					Upstream: "target",
				},
			}, false)

			require.Equal(t, "target", router.Route([]byte{0x8f}))
			require.Equal(t, "default", router.Route([]byte{0xcf}))
		})
		convey.Convey("does not allocate", func() {
			tests := []struct {
				name     string
				operator string
				value    string
				mask     string
				data     byte
			}{
				{name: "equal", operator: "==", value: "0x03", data: 0x03},
				{name: "not equal", operator: "!==", value: "0x02", data: 0x03},
				{name: "or equal", operator: "|=", value: "0x03", data: 0x03},
				{name: "or not equal", operator: "!|=", value: "0x03", data: 0x04},
				{name: "and equal", operator: "&=", value: "0x01", data: 0x03},
				{name: "and not equal", operator: "!&=", value: "0x01", data: 0x02},
				{name: "masked equal", operator: "==", value: "0x80", mask: "0xc0", data: 0x8f},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					router := mustNewRouter(t, "default", []sd_config.RouteConfig{
						{
							Conditions: []sd_config.RouteConditionConfig{
								{
									KeyBytes: "0:1",
									Operator: test.operator,
									Value:    test.value,
									Mask:     test.mask,
								},
							},
							Upstream: "target",
						},
					}, false)
					data := []byte{test.data}
					var upstream string

					allocs := testing.AllocsPerRun(1000, func() {
						upstream = router.Route(data)
					})

					require.Zero(t, allocs)
					require.Equal(t, "target", upstream)
				})
			}
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
			router := mustNewRouter(t, "", []sd_config.RouteConfig{config}, true)
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
			got, err := newRoute(3, sd_config.RouteConfig{
				Conditions: config.Conditions,
				Upstream:   DropUpstream,
			})
			convey.So(err, convey.ShouldBeNil)
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
