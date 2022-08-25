package sd_upstream

import "github.com/near-notfaraway/stevedore/sd_config"

type Manager struct {
	upstreams map[string]*Upstream
	routes    []*Route
}

func NewManager(config *sd_config.UploadConfig) *Manager {
	upstreams := make(map[string]*Upstream)
	for _, upsConfig := range config.Upstreams {
		upstreams[upsConfig.Name] = NewUpstream(upsConfig)
	}

	return &Manager{
		upstreams: upstreams,
		routes:    make([]*Route, len(config.Routes)),
	}
}

func (m *Manager) RouteUpstream(data []byte) *Upstream {
	return m.upstreams[DefaultUpstreamName]
}
