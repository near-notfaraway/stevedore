package sd_upstream

import "github.com/near-notfaraway/stevedore/sd_config"

type Manager struct {
	upstreams map[string]Upstream // decide which peer to use
	routes    []*Route            // decide which upstream to use
}

func NewManager(config *sd_config.UploadConfig) *Manager {
	// init upstreams
	upstreams := make(map[string]Upstream)
	for _, upsConfig := range config.Upstreams {
		upstreams[upsConfig.Name] = NewCHashUpstream(upsConfig)
	}

	// init routes
	routes := make([]*Route, len(config.Routes))
	for id, routeConfig := range config.Routes {
		route := NewRoute(id, routeConfig)
		routes = append(routes, route)
	}

	return &Manager{
		upstreams: upstreams,
		routes:    routes,
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

	return m.upstreams[DefaultUpstreamName]
}
