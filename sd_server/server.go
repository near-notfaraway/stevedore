package sd_server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
)

const uploadRollbackTimeout = 5 * time.Second

type Server struct {
	reactors                 []*reactor      // independent network shards
	uploadRegistry           *uploadRegistry // shared routing and upstream state
	uploadManagers           []*uploadManager
	uploadStatsEnabled       bool
	uploadStatsInterval      time.Duration
	retainedUploadRegistries []*uploadRegistry
	reloadMu                 sync.Mutex
	uploadMu                 sync.RWMutex
}

// NewServer builds the shared upload state and all reactor-local runtime resources.
func NewServer(config *sd_config.Config) *Server {
	listenSa := sd_socket.ResolveUDPSockaddr(config.Server.ListenAddr)
	if listenSa == nil {
		logrus.Panicf("resolve listen addr %s failed", config.Server.ListenAddr)
	}

	parallel := config.Server.ListenParallel
	if parallel < 1 {
		parallel = 1
	}
	if parallel > 1 && !sd_socket.ReusePortShardingSupported() {
		logrus.Warnf("listen parallelism %d is not supported by this platform; using 1", parallel)
		parallel = 1
	}
	statsEnabled := config.Log != nil && config.Log.UploadStatsIntervalSec > 0
	uploadRegistry, err := newUploadRegistry(config.Upload, statsEnabled)
	if err != nil {
		logrus.Panicf("create upload registry failed: %v", err)
	}
	s := &Server{
		reactors:           make([]*reactor, 0, parallel),
		uploadRegistry:     uploadRegistry,
		uploadManagers:     make([]*uploadManager, 0, parallel),
		uploadStatsEnabled: statsEnabled,
	}
	if statsEnabled {
		s.uploadStatsInterval = time.Duration(config.Log.UploadStatsIntervalSec) * time.Second
	}
	for i := 0; i < parallel; i++ {
		manager, err := s.uploadRegistry.newManager()
		if err != nil {
			for _, initialized := range s.reactors {
				initialized.close()
			}
			s.uploadRegistry.close()
			logrus.Panicf("create upload manager %d failed: %v", i, err)
		}
		reactor, err := newReactor(i, config, listenSa, manager)
		if err != nil {
			for _, initialized := range s.reactors {
				initialized.close()
			}
			s.uploadRegistry.close()
			logrus.Panicf("create reactor %d failed: %v", i, err)
		}
		s.reactors = append(s.reactors, reactor)
		s.uploadManagers = append(s.uploadManagers, manager)
	}
	return s
}

// ReloadUpload builds and publishes a new Upload generation across all reactors.
func (s *Server) ReloadUpload(ctx context.Context, config *sd_config.UploadConfig) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	registry, err := newUploadRegistry(config, s.uploadStatsEnabled)
	if err != nil {
		return fmt.Errorf("build upload registry: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			registry.close()
		}
	}()

	managers := make([]*uploadManager, len(s.reactors))
	for i := range managers {
		manager, buildErr := registry.newManager()
		if buildErr != nil {
			return fmt.Errorf("build upload manager %d: %w", i, buildErr)
		}
		managers[i] = manager
	}

	s.uploadMu.Lock()
	oldRegistry := s.uploadRegistry
	oldManagers := s.uploadManagers
	switched, switchErr := updateReactorManagers(ctx, s.reactors, managers, nil)
	if switchErr != nil {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), uploadRollbackTimeout)
		_, rollbackErr := updateReactorManagers(rollbackCtx, s.reactors, oldManagers, switched)
		cancel()
		if rollbackErr != nil {
			s.retainedUploadRegistries = append(s.retainedUploadRegistries, registry)
			committed = true
		}
		s.uploadMu.Unlock()
		if rollbackErr != nil {
			return fmt.Errorf("switch upload managers: %w; rollback failed: %v", switchErr, rollbackErr)
		}
		return fmt.Errorf("switch upload managers: %w", switchErr)
	}
	s.uploadRegistry = registry
	s.uploadManagers = managers
	retainedRegistries := s.retainedUploadRegistries
	s.retainedUploadRegistries = nil
	committed = true
	s.uploadMu.Unlock()

	if oldRegistry != nil {
		oldRegistry.close()
	}
	for _, retained := range retainedRegistries {
		retained.close()
	}
	return nil
}

func (s *Server) closeUploadRegistries() {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	s.uploadMu.Lock()
	registries := make([]*uploadRegistry, 0, 1+len(s.retainedUploadRegistries))
	if s.uploadRegistry != nil {
		registries = append(registries, s.uploadRegistry)
		s.uploadRegistry = nil
	}
	registries = append(registries, s.retainedUploadRegistries...)
	s.retainedUploadRegistries = nil
	s.uploadMu.Unlock()

	for _, registry := range registries {
		registry.close()
	}
}

// updateReactorManagers switches selected reactors concurrently and reports partial success.
func updateReactorManagers(ctx context.Context, reactors []*reactor, managers []*uploadManager,
	selected []bool) ([]bool, error) {
	if len(reactors) != len(managers) {
		return nil, fmt.Errorf("reactor count %d does not match manager count %d", len(reactors), len(managers))
	}
	if selected != nil && len(selected) != len(reactors) {
		return nil, fmt.Errorf("selected count %d does not match reactor count %d", len(selected), len(reactors))
	}

	switched := make([]bool, len(reactors))
	errs := make([]error, len(reactors))
	var wg sync.WaitGroup
	for i, current := range reactors {
		if selected != nil && !selected[i] {
			continue
		}
		wg.Add(1)
		go func(id int, r *reactor) {
			defer wg.Done()
			errs[id] = r.updateUploadManager(ctx, managers[id])
			switched[id] = errs[id] == nil
		}(i, current)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return switched, fmt.Errorf("reactor %d: %w", i, err)
		}
	}
	return switched, nil
}

func (s *Server) runUploadStatsLogger(ctx context.Context) {
	if s.uploadStatsInterval <= 0 {
		return
	}

	ticker := time.NewTicker(s.uploadStatsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.logUploadStats()
		case <-ctx.Done():
			return
		}
	}
}

// logUploadStats aggregates the current generation and emits one structured log entry.
func (s *Server) logUploadStats() {
	stats := s.uploadStats()
	routes := make(map[string]uint64, len(stats.route.Routes))
	for _, route := range stats.route.Routes {
		routes[route.Name] = route.Matches
	}
	upstreams := make(map[string]map[string]uint64, len(stats.upstreams))
	for _, upstream := range stats.upstreams {
		upstreams[upstream.Name] = map[string]uint64{
			"selections":         upstream.Selections,
			"selection_failures": upstream.SelectionFailures,
			"peer_failures":      upstream.PeerFailures,
		}
	}
	logrus.WithFields(logrus.Fields{
		"default_routes": stats.route.DefaultRoutes,
		"short_payloads": stats.route.ShortPayloads,
		"drops":          stats.route.Drops,
		"routes":         routes,
		"upstreams":      upstreams,
	}).Info("upload stats")
}

func (s *Server) uploadStats() uploadStats {
	s.uploadMu.RLock()
	defer s.uploadMu.RUnlock()

	if len(s.uploadManagers) == 0 {
		return uploadStats{}
	}

	stats := s.uploadManagers[0].stats()
	for _, manager := range s.uploadManagers[1:] {
		addUploadStats(&stats, manager.stats())
	}
	return stats
}

// ListenAndServe runs reactors until one stops or the parent context is canceled.
func (s *Server) ListenAndServe(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer s.closeUploadRegistries()

	// start upload stats logger.
	if s.uploadStatsInterval > 0 {
		go s.runUploadStatsLogger(ctx)
	}

	// run reactors concurrently.
	errCh := make(chan error, len(s.reactors))
	for _, r := range s.reactors {
		go func(r *reactor) {
			errCh <- r.serve(ctx)
		}(r)
	}

	// wake selectors immediately on cancellation to reduce shutdown latency and avoid relying on poll timeouts.
	wakeDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		for _, r := range s.reactors {
			_ = r.selector.Wake()
		}
		close(wakeDone)
	}()

	// wait for the first reactor to stop, cancel all reactors.
	firstErr := <-errCh
	cancel()
	<-wakeDone

	// wait for all reactors to stop.
	for i := 1; i < len(s.reactors); i++ {
		if err := <-errCh; firstErr == nil && err != nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("reactor stopped: %w", firstErr)
	}
	return nil
}

func addUploadStats(total *uploadStats, current uploadStats) {
	total.route.DefaultRoutes += current.route.DefaultRoutes
	total.route.ShortPayloads += current.route.ShortPayloads
	total.route.Drops += current.route.Drops
	for i := range total.route.Routes {
		total.route.Routes[i].Matches += current.route.Routes[i].Matches
	}
	for i := range total.upstreams {
		total.upstreams[i].Selections += current.upstreams[i].Selections
		total.upstreams[i].SelectionFailures += current.upstreams[i].SelectionFailures
		total.upstreams[i].PeerFailures += current.upstreams[i].PeerFailures
	}
}
