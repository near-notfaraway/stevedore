package sd_route

import (
	"strconv"
	"sync/atomic"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

const DropUpstream = "drop"

type Router struct {
	defaultUpstream string
	routes          []*route
	stats           *routerStats
}

func NewRouter(defaultUpstream string, configs []sd_config.RouteConfig, statsEnabled bool) *Router {
	routes := make([]*route, 0, len(configs))
	names := make(map[string]struct{}, len(configs))
	for id, config := range configs {
		route := newRoute(id, config)
		if _, ok := names[route.name]; ok {
			logrus.Panicf("route name %s is duplicated", route.name)
		}
		names[route.name] = struct{}{}
		routes = append(routes, route)
	}
	if defaultUpstream == DropUpstream {
		defaultUpstream = ""
	}
	return &Router{
		defaultUpstream: defaultUpstream,
		routes:          routes,
		stats:           newRouterStats(routes, statsEnabled),
	}
}

func (r *Router) Clone() *Router {
	return &Router{
		defaultUpstream: r.defaultUpstream,
		routes:          r.routes,
		stats:           newRouterStats(r.routes, r.stats.enabled),
	}
}

func (r *Router) Stats() Stats {
	stats := r.stats
	routes := make([]RouteStats, len(stats.routeNames))
	for i, name := range stats.routeNames {
		routes[i] = RouteStats{
			ID:      i,
			Name:    name,
			Matches: stats.routeMatches[i].Load(),
		}
	}
	return Stats{
		Routes:        routes,
		DefaultRoutes: stats.defaultRoutes.Load(),
		ShortPayloads: stats.shortPayloads.Load(),
		Drops:         stats.drops.Load(),
	}
}

func (r *Router) Route(data []byte) string {
	if !r.stats.enabled {
		for _, route := range r.routes {
			matched, _ := route.matchStatus(data)
			if matched {
				return route.upstream
			}
		}
		return r.defaultUpstream
	}

	return r.routeWithStats(data)
}

func (r *Router) routeWithStats(data []byte) string {
	shortPayload := false
	for i, route := range r.routes {
		matched, short := route.matchStatus(data)
		if matched {
			r.stats.routeMatches[i].Add(1)
			if route.upstream == "" {
				r.stats.drops.Add(1)
			}
			return route.upstream
		}
		shortPayload = shortPayload || short
	}
	r.stats.defaultRoutes.Add(1)
	if shortPayload {
		r.stats.shortPayloads.Add(1)
	}
	if r.defaultUpstream == "" {
		r.stats.drops.Add(1)
	}
	return r.defaultUpstream
}

type routerStats struct {
	enabled       bool
	routeNames    []string
	routeMatches  []atomic.Uint64
	defaultRoutes atomic.Uint64
	shortPayloads atomic.Uint64
	drops         atomic.Uint64
}

func newRouterStats(routes []*route, enabled bool) *routerStats {
	names := make([]string, len(routes))
	for i, route := range routes {
		names[i] = route.name
	}
	return &routerStats{
		enabled:      enabled,
		routeNames:   names,
		routeMatches: make([]atomic.Uint64, len(routes)),
	}
}

type Stats struct {
	Routes        []RouteStats
	DefaultRoutes uint64
	ShortPayloads uint64
	Drops         uint64
}

type RouteStats struct {
	ID      int
	Name    string
	Matches uint64
}

type route struct {
	name       string
	conditions []condition
	upstream   string
}

func newRoute(id int, config sd_config.RouteConfig) *route {
	name := config.Name
	if name == "" {
		name = "route-" + strconv.Itoa(id)
	}
	if config.Upstream == DropUpstream {
		config.Upstream = ""
	}

	configs := config.Conditions
	if len(configs) == 0 {
		logrus.Panicf("conditions are required in route %d", id)
	}

	conditions := make([]condition, 0, len(configs))
	for _, config := range configs {
		conditions = append(conditions, newCondition(id, config))
	}
	return &route{
		name:       name,
		conditions: conditions,
		upstream:   config.Upstream,
	}
}

func (r *route) matchStatus(data []byte) (bool, bool) {
	for _, condition := range r.conditions {
		matched, short := condition.matchStatus(data)
		if !matched {
			return false, short
		}
	}
	return true, false
}
