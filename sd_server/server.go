package sd_server

import (
	"context"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"sync"
)

const (
	FdHandlerIndexIn = iota
	FdHandlerIndexOut
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
	taskPool    sd_util.TaskPool
	selector    sd_socket.Selector
	fdHandles   sync.Map // map[int][2]func()
	sessionMgr  *sd_session.Manager
	upstreamMgr *sd_upstream.Manager
	mcPool      *sd_socket.MMsgContainerPool
	evChanPool  sync.Pool
}

func NewServer(config *sd_config.Config) *Server {
	selector, err := sd_socket.NewEpoller(config.Server.EventSize, true)
	if err != nil {
		logrus.Panicf("create selector failed %w", err)
	}

	evChanPool := sync.Pool{
		New: func() interface{} {
			return make(chan struct{}, config.Server.EventChanSize)
		},
	}

	return &Server{
		config:      config,
		workers:     make([]*WorkerIns, 0, config.Server.ListenParallel),
		taskPool:    sd_util.NewSimpleTaskPool(config.Server.TaskPoolSize, config.Server.TaskPoolTimeoutSec),
		selector:    selector,
		sessionMgr:  sd_session.NewManager(config.Session, evChanPool, selector),
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

		logrus.Debugf("store fd %d", fd)
		s.fdHandles.Store(fd, [2]func(){func() { ch <- struct{}{} }, nil})
		if err = s.selector.Add(fd, sd_socket.SelectorEventRead); err != nil {
			s.fdHandles.Delete(fd)
			return fmt.Errorf("add listen conn to selector failed: %w", err)
		}

		worker := &WorkerIns{id: i, fd: fd, ch: ch}
		s.workers = append(s.workers, worker)
		go s.uploadWorker(ctx, worker)
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
	logrus.Debug("start polling...")
	return s.selector.Polling(ctx, func(evs []unix.EpollEvent) {
		for i := 0; i < len(evs); i++ {
			fs, ok := s.fdHandles.Load(int(evs[i].Fd))
			if !ok {
				continue
			}
			_fs := fs.([2]func())

			// do read event
			if (evs[i].Events & unix.EPOLLIN) > 0 {
				s.taskPool.Go(_fs[FdHandlerIndexIn])
			}
			// do write event
			if (evs[i].Events & unix.EPOLLOUT) > 0 {
				s.taskPool.Go(_fs[FdHandlerIndexOut])
			}
		}
	})
}
