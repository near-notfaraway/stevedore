package sd_upstream

import (
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
)

type Registry struct {
	configs []*sd_config.UpstreamConfig // immutable upstream definitions
	groups  map[string]*PeerGroup       // shared peer control planes
	router  *sd_route.Router            // immutable payload router
}

func NewRegistry(config *sd_config.UploadConfig) *Registry {
	upstreamNames := make(map[string]struct{}, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		if upstreamConfig.Name == "" || upstreamConfig.Name == sd_route.DropUpstream {
			panic(fmt.Sprintf("upstream name %q is reserved", upstreamConfig.Name))
		}
		if _, ok := upstreamNames[upstreamConfig.Name]; ok {
			panic(fmt.Sprintf("upstream name %q is duplicated", upstreamConfig.Name))
		}
		upstreamNames[upstreamConfig.Name] = struct{}{}
	}
	validateUpstreamTargets(config, upstreamNames)

	groups := make(map[string]*PeerGroup, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		groups[upstreamConfig.Name] = NewPeerGroup(upstreamConfig)
	}

	return &Registry{
		configs: config.Upstreams,
		groups:  groups,
		router:  sd_route.NewRouter(config.DefaultUpstream, config.Routes),
	}
}

func validateUpstreamTargets(config *sd_config.UploadConfig, upstreamNames map[string]struct{}) {
	if config.DefaultUpstream != "" && config.DefaultUpstream != sd_route.DropUpstream {
		if _, ok := upstreamNames[config.DefaultUpstream]; !ok {
			panic(fmt.Sprintf("default upstream %q does not exist", config.DefaultUpstream))
		}
	}

	for id, route := range config.Routes {
		if route.Upstream == "" || route.Upstream == sd_route.DropUpstream {
			continue
		}
		if _, ok := upstreamNames[route.Upstream]; !ok {
			panic(fmt.Sprintf("upstream %q for route %d does not exist", route.Upstream, id))
		}
	}
}

func (r *Registry) NewManager() *Manager {
	upstreams := make(map[string]Upstream, len(r.configs))
	for _, config := range r.configs {
		upstreams[config.Name] = NewUpstream(config, r.groups[config.Name])
	}
	return &Manager{
		upstreams: upstreams,
		router:    r.router,
	}
}

func (r *Registry) RouteStats() sd_route.Stats {
	return r.router.Stats()
}

func (r *Registry) EnableRouteStats() {
	r.router.EnableStats()
}

type Manager struct {
	upstreams map[string]Upstream // reactor-local upstream selectors
	router    *sd_route.Router    // immutable payload router
}

func (m *Manager) RouteUpstream(data []byte) Upstream {
	return m.upstreams[m.router.Route(data)]
}
