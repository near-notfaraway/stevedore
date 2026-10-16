package sd_server

import (
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_diagnosis"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
)

type uploadRegistry struct {
	upstreamRegistry *upstreamRegistry
	router           *sd_route.Router
}

// validateUploadConfig checks routing and upstream definitions without starting runtime resources.
func validateUploadConfig(config *sd_config.UploadConfig, statsEnabled bool) (*sd_route.Router, error) {
	if config == nil {
		return nil, fmt.Errorf("upload config is required")
	}
	upstreamNames := make(map[string]struct{}, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		if upstreamConfig == nil {
			return nil, fmt.Errorf("upstream config is required")
		}
		if upstreamConfig.Name == "" || upstreamConfig.Name == sd_route.DropUpstream {
			return nil, fmt.Errorf("upstream name %q is reserved", upstreamConfig.Name)
		}
		if _, ok := upstreamNames[upstreamConfig.Name]; ok {
			return nil, fmt.Errorf("upstream name %q is duplicated", upstreamConfig.Name)
		}
		upstreamNames[upstreamConfig.Name] = struct{}{}
	}
	if err := validateUpstreamTargets(config, upstreamNames); err != nil {
		return nil, err
	}
	router, err := sd_route.NewRouter(config.DefaultUpstream, config.Routes, statsEnabled)
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}
	for _, upstreamConfig := range config.Upstreams {
		if err := sd_upstream.ValidateConfig(upstreamConfig); err != nil {
			return nil, fmt.Errorf("validate upstream %q: %w", upstreamConfig.Name, err)
		}
	}
	return router, nil
}

// newUploadRegistry validates Upload configuration and builds its shared control plane.
func newUploadRegistry(config *sd_config.UploadConfig, statsEnabled bool) (*uploadRegistry, error) {
	router, err := validateUploadConfig(config, statsEnabled)
	if err != nil {
		return nil, err
	}

	definitions := make([]upstreamDefinition, 0, len(config.Upstreams))
	for _, upstreamConfig := range config.Upstreams {
		group, err := sd_upstream.NewPeerGroup(upstreamConfig)
		if err != nil {
			closeUpstreamDefinitions(definitions)
			return nil, fmt.Errorf("build upstream %q peer group: %w", upstreamConfig.Name, err)
		}
		definition := upstreamDefinition{
			name:             upstreamConfig.Name,
			upstreamType:     upstreamConfig.Type,
			keyExtractor:     upstreamConfig.KeyExtractor,
			invalidKeyPolicy: upstreamConfig.InvalidKeyPolicy,
			group:            group,
		}
		if _, err := definition.newUpstream(); err != nil {
			group.Close()
			closeUpstreamDefinitions(definitions)
			return nil, fmt.Errorf("build upstream %q: %w", upstreamConfig.Name, err)
		}
		definitions = append(definitions, definition)
	}

	upstreamRegistry := &upstreamRegistry{
		definitions:  definitions,
		statsEnabled: statsEnabled,
	}
	return &uploadRegistry{
		upstreamRegistry: upstreamRegistry,
		router:           router,
	}, nil
}

func (r *uploadRegistry) newManager() (*uploadManager, error) {
	upstreamMgr, err := r.upstreamRegistry.newManager()
	if err != nil {
		return nil, err
	}
	return &uploadManager{
		upstreamMgr: upstreamMgr,
		router:      r.router.Clone(),
	}, nil
}

func (r *uploadRegistry) close() {
	closeUpstreamDefinitions(r.upstreamRegistry.definitions)
}

type upstreamRegistry struct {
	definitions  []upstreamDefinition
	statsEnabled bool
}

func (r *upstreamRegistry) newManager() (*sd_upstream.Manager, error) {
	upstreams := make(map[string]sd_upstream.Upstream, len(r.definitions))
	for _, definition := range r.definitions {
		upstream, err := definition.newUpstream()
		if err != nil {
			return nil, fmt.Errorf("build upstream %q: %w", definition.name, err)
		}
		upstreams[definition.name] = upstream
	}
	return sd_upstream.NewManager(upstreams, r.statsEnabled), nil
}

type upstreamDefinition struct {
	name             string
	upstreamType     string
	keyExtractor     *sd_config.KeyExtractorConfig
	invalidKeyPolicy string
	group            *sd_upstream.PeerGroup
}

func (u *upstreamDefinition) newUpstream() (sd_upstream.Upstream, error) {
	return sd_upstream.NewUpstream(&sd_config.UpstreamConfig{
		Name:             u.name,
		Type:             u.upstreamType,
		KeyExtractor:     u.keyExtractor,
		InvalidKeyPolicy: u.invalidKeyPolicy,
	}, u.group)
}

func closeUpstreamDefinitions(definitions []upstreamDefinition) {
	for i := len(definitions) - 1; i >= 0; i-- {
		definitions[i].group.Close()
	}
}

type uploadManager struct {
	upstreamMgr *sd_upstream.Manager
	router      *sd_route.Router
}

func (m *uploadManager) routeUpstream(data []byte) string {
	return m.router.Route(data)
}

func (m *uploadManager) selectPeer(upstreamName string, data []byte) (*sd_upstream.Peer, bool) {
	return m.upstreamMgr.SelectPeer(upstreamName, data)
}

func (m *uploadManager) markPeerDead(upstreamName string, peer *sd_upstream.Peer) {
	m.upstreamMgr.MarkPeerDead(upstreamName, peer)
}

func (m *uploadManager) stats() sd_diagnosis.UploadStats {
	return sd_diagnosis.UploadStats{
		Route:     m.router.Stats(),
		Upstreams: m.upstreamMgr.Stats(),
	}
}

func validateUpstreamTargets(config *sd_config.UploadConfig, upstreamNames map[string]struct{}) error {
	if config.DefaultUpstream != "" && config.DefaultUpstream != sd_route.DropUpstream {
		if _, ok := upstreamNames[config.DefaultUpstream]; !ok {
			return fmt.Errorf("default upstream %q does not exist", config.DefaultUpstream)
		}
	}

	for id, route := range config.Routes {
		if route.Upstream == "" || route.Upstream == sd_route.DropUpstream {
			continue
		}
		if _, ok := upstreamNames[route.Upstream]; !ok {
			return fmt.Errorf("upstream %q for route %d does not exist", route.Upstream, id)
		}
	}
	return nil
}
