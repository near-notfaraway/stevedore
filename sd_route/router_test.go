package sd_route

import (
	"sync"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
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
