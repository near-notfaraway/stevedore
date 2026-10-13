package sd_server

import (
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
)

type uploadRegistry struct {
	upstreamRegistry *upstreamRegistry
	router           *sd_route.Router
}

type upstreamRegistry struct {
	definitions  []upstreamDefinition
	statsEnabled bool
}

type upstreamDefinition struct {
	name             string
	upstreamType     string
	keyBytes         string
	keyMask          string
	invalidKeyPolicy string
	group            *sd_upstream.PeerGroup
}

type UploadStats struct {
	Route     sd_route.Stats
	Upstreams []sd_upstream.Stats
}

func newUploadRegistry(config *sd_config.UploadConfig, statsEnabled bool) *uploadRegistry {
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

	definitions := make([]upstreamDefinition, 0, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		definitions = append(definitions, upstreamDefinition{
			name:             upstreamConfig.Name,
			upstreamType:     upstreamConfig.Type,
			keyBytes:         upstreamConfig.KeyBytes,
			keyMask:          upstreamConfig.KeyMask,
			invalidKeyPolicy: upstreamConfig.InvalidKeyPolicy,
			group:            sd_upstream.NewPeerGroup(upstreamConfig),
		})
	}

	upstreamRegistry := &upstreamRegistry{
		definitions:  definitions,
		statsEnabled: statsEnabled,
	}
	return &uploadRegistry{
		upstreamRegistry: upstreamRegistry,
		router:           sd_route.NewRouter(config.DefaultUpstream, config.Routes, statsEnabled),
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

func (r *uploadRegistry) newManager() *uploadManager {
	return &uploadManager{
		upstreamManager: r.upstreamRegistry.newManager(),
		router:          r.router.Clone(),
	}
}

func (r *upstreamRegistry) newManager() *sd_upstream.Manager {
	upstreams := make(map[string]sd_upstream.Upstream, len(r.definitions))
	for _, definition := range r.definitions {
		upstreams[definition.name] = definition.newUpstream()
	}
	return sd_upstream.NewManager(upstreams, r.statsEnabled)
}

func (u *upstreamDefinition) newUpstream() sd_upstream.Upstream {
	return sd_upstream.NewUpstream(&sd_config.UpstreamConfig{
		Name:             u.name,
		Type:             u.upstreamType,
		KeyBytes:         u.keyBytes,
		KeyMask:          u.keyMask,
		InvalidKeyPolicy: u.invalidKeyPolicy,
	}, u.group)
}

type uploadManager struct {
	upstreamManager *sd_upstream.Manager
	router          *sd_route.Router
}

func (m *uploadManager) routeUpstream(data []byte) string {
	return m.router.Route(data)
}

func (m *uploadManager) selectPeer(upstreamName string, data []byte) (*sd_upstream.Peer, bool) {
	return m.upstreamManager.SelectPeer(upstreamName, data)
}

func (m *uploadManager) markPeerDead(upstreamName string, peer *sd_upstream.Peer) {
	m.upstreamManager.MarkPeerDead(upstreamName, peer)
}

func (m *uploadManager) stats() UploadStats {
	return UploadStats{
		Route:     m.router.Stats(),
		Upstreams: m.upstreamManager.Stats(),
	}
}
