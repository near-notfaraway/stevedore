package sd_upstream

import (
	"github.com/near-notfaraway/stevedore/sd_config"
)

//------------------------------------------------------------------------------
// Manager: Used to manager routes and upstreams
// - choose upstream according to matched route
//------------------------------------------------------------------------------

type Manager struct {
	defaultUpstream Upstream            // use it when no route match
	upstreams       map[string]Upstream // manage upstreams which decide how to choose peer
	routes          []*Route            // manage routes which  decide how to choose upstream
}

func NewManager(config *sd_config.UploadConfig) *Manager {
	// init upstreams
	upstreams := make(map[string]Upstream)
	for _, upsConfig := range config.Upstreams {
		upstreams[upsConfig.Name] = NewUpstream(upsConfig)
	}

	// init default upstream
	defaultUpstream, ok := upstreams[config.DefaultUpstream]
	if !ok {
		defaultUpstream = nil
	}

	// init routes
	routes := make([]*Route, 0, len(config.Routes))
	for id, routeConfig := range config.Routes {
		route := NewRoute(id, routeConfig)
		routes = append(routes, route)
	}

	return &Manager{
		defaultUpstream: defaultUpstream,
		upstreams:       upstreams,
		routes:          routes,
	}
}

func (m *Manager) RouteUpstream(data []byte) Upstream {
	// iterate over all routes in order
	for _, route := range m.routes {
		if route.Match(data) {
			if v, ok := m.upstreams[route.upstream]; ok {
				return v
			}
		}
	}

	// use default upstream when not match
	return m.defaultUpstream
}
