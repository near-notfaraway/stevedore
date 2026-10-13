package sd_server

import (
	"context"
	"fmt"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
)

type Server struct {
	reactors            []*reactor      // independent network shards
	uploadRegistry      *uploadRegistry // shared routing and upstream state
	uploadStatsInterval time.Duration
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
	statsEnabled := config.Log != nil && config.Log.UploadStatsIntervalSec > 0
	s := &Server{
		reactors:       make([]*reactor, 0, parallel),
		uploadRegistry: newUploadRegistry(config.Upload, statsEnabled),
	}
	if statsEnabled {
		s.uploadStatsInterval = time.Duration(config.Log.UploadStatsIntervalSec) * time.Second
	}
	for i := 0; i < parallel; i++ {
		reactor, err := newReactor(i, config, listenSa, s.uploadRegistry.newManager())
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
	if len(s.reactors) == 0 {
		return uploadStats{}
	}

	stats := s.reactors[0].upstreamMgr.stats()
	for _, reactor := range s.reactors[1:] {
		addUploadStats(&stats, reactor.upstreamMgr.stats())
	}
	return stats
}

func (s *Server) ListenAndServe() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.uploadStatsInterval > 0 {
		go s.runUploadStatsLogger(ctx)
	}

	errCh := make(chan error, len(s.reactors))
	for _, r := range s.reactors {
		go func(r *reactor) {
			errCh <- r.serve(ctx)
		}(r)
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
