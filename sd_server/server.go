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

type UploadWorker struct {
	id int           // unique worker id
	fd int           // fd of listened socket
	ch chan struct{} // channel for recv upload event
}

type Server struct {
	config         *sd_config.Config            // global config
	ctx            context.Context              // control context
	workers        []*UploadWorker              // upload workers
	taskPool       sd_util.TaskPool             // task pool for deliver events
	selector       sd_socket.Selector           // poll events from fds
	fdReadHandlers sync.Map                     // map[int]func(): map fd and its read event handler
	sessionMgr     *sd_session.Manager          // manage sessions
	upstreamMgr    *sd_upstream.Manager         // manage upstreams
	mcPool         *sd_socket.MMsgContainerPool // allocate memory for recvmmsg
	evChanPool     sync.Pool                    // allocate memory for event channel
}

func NewServer(config *sd_config.Config) *Server {
	selector, err := sd_socket.NewEpoller(config.Server.EventSize, true)
	if err != nil {
		logrus.Panicf("create selector failed %v", err)
	}

	evChanPool := sync.Pool{
		New: func() interface{} {
			return make(chan struct{}, config.Server.EventChanSize)
		},
	}

	return &Server{
		config:      config,
		workers:     make([]*UploadWorker, 0, config.Server.ListenParallel),
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
		s.fdReadHandlers.Store(fd, func() { ch <- struct{}{} })
		if err = s.selector.Add(fd, sd_socket.SelectorEventRead); err != nil {
			s.fdReadHandlers.Delete(fd)
			return fmt.Errorf("add listen conn to selector failed: %w", err)
		}

		worker := &UploadWorker{id: i, fd: fd, ch: ch}
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
			fs, ok := s.fdReadHandlers.Load(int(evs[i].Fd))
			if !ok {
				continue
			}
			_fs := fs.(func())

			// do read event
			if (evs[i].Events & unix.EPOLLIN) > 0 {
				s.taskPool.Go(_fs)
			}
		}
	})
}
