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
	stats           routerStats
}

type routerStats struct {
	enabled        bool
	defaultMatches atomic.Uint64
	shortPayloads  atomic.Uint64
	drops          atomic.Uint64
}

type Stats struct {
	Routes         []RouteStats
	DefaultMatches uint64
	ShortPayloads  uint64
	Drops          uint64
}

type RouteStats struct {
	ID      int
	Name    string
	Matches uint64
}

func (r *Router) Stats() Stats {
	routes := make([]RouteStats, len(r.routes))
	for i, route := range r.routes {
		routes[i] = RouteStats{
			ID:      i,
			Name:    route.name,
			Matches: route.statMatches.Load(),
		}
	}
	return Stats{
		Routes:         routes,
		DefaultMatches: r.stats.defaultMatches.Load(),
		ShortPayloads:  r.stats.shortPayloads.Load(),
		Drops:          r.stats.drops.Load(),
	}
}

func (r *Router) EnableStats() {
	r.stats.enabled = true
}

func NewRouter(defaultUpstream string, configs []sd_config.RouteConfig) *Router {
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
	if isDropUpstream(defaultUpstream) {
		defaultUpstream = ""
	}
	return &Router{
		defaultUpstream: defaultUpstream,
		routes:          routes,
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
	for _, route := range r.routes {
		matched, short := route.matchStatus(data)
		if matched {
			route.statMatches.Add(1)
			if isDropUpstream(route.upstream) {
				r.stats.drops.Add(1)
			}
			return route.upstream
		}
		shortPayload = shortPayload || short
	}
	r.stats.defaultMatches.Add(1)
	if shortPayload {
		r.stats.shortPayloads.Add(1)
	}
	if isDropUpstream(r.defaultUpstream) {
		r.stats.drops.Add(1)
	}
	return r.defaultUpstream
}

type route struct {
	name        string
	conditions  []condition
	upstream    string
	statMatches atomic.Uint64
}

func newRoute(id int, config sd_config.RouteConfig) *route {
	name := config.Name
	if name == "" {
		name = "route-" + strconv.Itoa(id)
	}
	if isDropUpstream(config.Upstream) {
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

func isDropUpstream(upstream string) bool {
	return upstream == "" || upstream == DropUpstream
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
