package sd_server

import (
	"context"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_selector"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"golang.org/x/sys/unix"
	"sync"
)

type WorkerIns struct {
	id int           // unique worker id
	fd int           // fd of listened socket
	ch chan struct{} // channel for recv upload event
}

type Server struct {
	config      *sd_config.Config
	ctx         context.Context
	workers     []*WorkerIns
	selector    sd_selector.Selector
	sessionMgr  *sd_session.Manager
	upstreamMgr *sd_upstream.Manager
	mcPool      *sd_socket.MMsgContainerPool
	evChanPool  sync.Pool
}

func NewServer(config *sd_config.Config) *Server {
	taskPool := sd_selector.NewSimpleTaskPool(config.Server.TaskPoolSize, config.Server.TaskPoolTimeoutSec)
	selector, err := sd_selector.NewEpoller(config.Server.EventSize, true, taskPool)
	if err != nil {
		panic(fmt.Errorf("create selector failed %w", err))
	}

	evChanPool := sync.Pool{
		New: func() interface{} {
			return make(chan struct{}, config.Server.EventChanSize)
		},
	}

	return &Server{
		workers:     make([]*WorkerIns, 0, config.Server.ListenParallel),
		selector:    selector,
		sessionMgr:  sd_session.NewManager(config.Session, evChanPool),
		upstreamMgr: sd_upstream.NewManager(config.Upload),
		mcPool:      sd_socket.NewMMsgContainerPool(config.Server.BatchSize, config.Server.BufSize),
		evChanPool:  evChanPool,
	}
}

func (s *Server) ListenAndServe() error {
	// build context
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx

	// resolve addr
	listenSa := sd_socket.ResolveUDPSockaddr(s.config.Server.ListenAddr)
	if listenSa == nil {
		return fmt.Errorf("resolve listen addr %s failed", s.config.Server.ListenAddr)
	}

	parallel := s.config.Server.ListenParallel
	for i := 0; i < parallel; i++ {
		ch := s.evChanPool.Get().(chan struct{})
		fd, err := sd_socket.UDPBoundSocket(listenSa, true, true, true)
		if err != nil {
			return fmt.Errorf("create listen socket failed: %w", err)
		}

		fs := [2]func(){func() { ch <- struct{}{} }, nil}
		if err = s.selector.Add(fd, sd_selector.SelectorEventRead, fs)
			err != nil {
			return fmt.Errorf("add listen conn to selector failed: %w", err)
		}

		s.workers = append(s.workers, &WorkerIns{id: i, fd: fd, ch: ch})
	}

	// 关闭服务
	defer func() {
		for i := 0; i < parallel; i++ {
			s.evChanPool.Put(s.workers[i].ch)
			_ = unix.Close(s.workers[i].fd)
		}
		cancel()
	}()

	// 开始 polling
	return s.selector.Polling(ctx)
}
