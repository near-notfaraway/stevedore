package sd_server

import (
	"context"
	"fmt"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
)

type Server struct {
	reactors           []*Reactor            // independent network shards
	upstreamRegistry   *sd_upstream.Registry // shared health checks and peer snapshots
	routeStatsInterval time.Duration
}

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
	s := &Server{
		reactors:         make([]*Reactor, 0, parallel),
		upstreamRegistry: sd_upstream.NewRegistry(config.Upload),
	}
	if config.Log != nil {
		s.configureRouteStats(time.Duration(config.Log.RouteStatsIntervalSec) * time.Second)
	}
	for i := 0; i < parallel; i++ {
		reactor, err := newReactor(i, config, listenSa, s.upstreamRegistry.NewManager())
		if err != nil {
			for _, initialized := range s.reactors {
				initialized.close()
			}
			logrus.Panicf("create reactor %d failed: %v", i, err)
		}
		s.reactors = append(s.reactors, reactor)
	}
	return s
}

func (s *Server) configureRouteStats(interval time.Duration) {
	if interval <= 0 {
		return
	}
	s.routeStatsInterval = interval
	s.upstreamRegistry.EnableRouteStats()
}

func (s *Server) RouteStats() sd_route.Stats {
	return s.upstreamRegistry.RouteStats()
}

func (s *Server) runRouteStatsLogger(ctx context.Context) {
	if s.routeStatsInterval <= 0 {
		return
	}

	ticker := time.NewTicker(s.routeStatsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.logRouteStats()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) logRouteStats() {
	stats := s.RouteStats()
	routes := make(map[string]uint64, len(stats.Routes))
	for _, route := range stats.Routes {
		routes[route.Name] = route.Matches
	}
	logrus.WithFields(logrus.Fields{
		"default_matches": stats.DefaultMatches,
		"short_payloads":  stats.ShortPayloads,
		"drops":           stats.Drops,
		"routes":          routes,
	}).Info("route stats")
}

func (s *Server) ListenAndServe() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.routeStatsInterval > 0 {
		go s.runRouteStatsLogger(ctx)
	}

	errCh := make(chan error, len(s.reactors))
	for _, reactor := range s.reactors {
		go func(r *Reactor) {
			errCh <- r.Serve(ctx)
		}(reactor)
	}

	firstErr := <-errCh
	cancel()
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
