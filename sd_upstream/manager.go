package sd_upstream

import "github.com/near-notfaraway/stevedore/sd_config"

type Registry struct {
	configs         []*sd_config.UpstreamConfig // immutable upstream definitions
	groups          map[string]*PeerGroup       // shared peer control planes
	defaultUpstream string                      // default upstream name
	routes          []*Route                    // immutable ordered routes
}

func NewRegistry(config *sd_config.UploadConfig) *Registry {
	groups := make(map[string]*PeerGroup, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		groups[upstreamConfig.Name] = NewPeerGroup(upstreamConfig)
	}

	routes := make([]*Route, 0, len(config.Routes))
	for id, routeConfig := range config.Routes {
		routes = append(routes, NewRoute(id, routeConfig))
	}
	return &Registry{
		configs:         config.Upstreams,
		groups:          groups,
		defaultUpstream: config.DefaultUpstream,
		routes:          routes,
	}
}

func (r *Registry) NewManager() *Manager {
	upstreams := make(map[string]Upstream, len(r.configs))
	for _, config := range r.configs {
		upstreams[config.Name] = NewUpstream(config, r.groups[config.Name])
	}
	return &Manager{
		defaultUpstream: upstreams[r.defaultUpstream],
		upstreams:       upstreams,
		routes:          r.routes,
	}
}

type Manager struct {
	defaultUpstream Upstream            // used when no route matches
	upstreams       map[string]Upstream // reactor-local upstream selectors
	routes          []*Route            // immutable ordered routes
}

func (m *Manager) RouteUpstream(data []byte) Upstream {
	for _, route := range m.routes {
		if route.Match(data) {
			return m.upstreams[route.upstream]
		}
	}
	return m.defaultUpstream
}
