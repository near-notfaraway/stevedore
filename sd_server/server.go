package sd_server

import (
	"context"
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
)

type Server struct {
	reactors         []*Reactor            // independent network shards
	upstreamRegistry *sd_upstream.Registry // shared health checks and peer snapshots
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
	s := &Server{
		reactors:         make([]*Reactor, 0, parallel),
		upstreamRegistry: sd_upstream.NewRegistry(config.Upload),
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

func (s *Server) ListenAndServe() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
