package sd_selector

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"sync"
)

const DefaultEventSize = 1024

type SelectorEvent uint8

const (
	_ SelectorEvent = iota
	SelectorEventRead
	SelectorEventWrite
	SelectorEventReadWrite
)

const (
	FdHandlerIndexIn = iota
	FdHandlerIndexOut
)

type Selector interface {
	Add(fd int, ev SelectorEvent, fs [2]func()) error
	Mod(fd int, ev SelectorEvent, fs [2]func()) error
	Del(fd int) error
	Polling(ctx context.Context) error
}

//------------------------------------------------------------------------------
// Epoller: a safe selector base on epoll
// - register fd with in/out handler
// - run fd handler in go routine pool
//------------------------------------------------------------------------------

type Epoller struct {
	fd         int                 // epoll fd
	eventSize  int                 // max events per poll
	et         bool                // if use edge trigger
	fdLocks    sync.Map            // lock for fd which is registered: map[int32]chan struct{}
	fdHandlers map[int32][2]func() // fd in handler and out handler
	taskPool   TaskPool            // go routine pool for handler
}

func NewEpoller(eventSize int, et bool, taskPool TaskPool) (Selector, error) {
	if eventSize < 1 {
		eventSize = DefaultEventSize
	}

	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, os.NewSyscallError("epoll_create1", err)
	}

	return &Epoller{
		fd:        fd,
		eventSize: eventSize,
		et:        et,
		taskPool:  taskPool,
	}, nil
}

func (s *Epoller) Add(fd int, ev SelectorEvent, fs [2]func()) error {
	_fd := int32(fd)
	fdLock, loaded := s.fdLocks.LoadOrStore(_fd, make(chan struct{}, 1))
	if loaded {
		return fmt.Errorf("the fd %d is already in epoll", fd)
	}
	_fdLock := fdLock.(chan struct{})

	e := &unix.EpollEvent{
		Fd: _fd,
	}
	if s.et {
		e.Events = unix.EPOLLET
	}

	switch ev {
	case SelectorEventRead:
		e.Events |= unix.EPOLLIN
	case SelectorEventWrite:
		e.Events |= unix.EPOLLOUT
	case SelectorEventReadWrite:
		e.Events |= unix.EPOLLIN | unix.EPOLLOUT
	default:
		return fmt.Errorf("unknow epoll event type: %d", ev)
	}

	_fdLock <- struct{}{}
	err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_ADD, fd, e)
	if err != nil {
		s.fdLocks.Delete(_fd)
		return os.NewSyscallError("epoll_ctl add", err)
	}
	s.fdHandlers[_fd] = fs
	<-_fdLock

	return nil
}

func (s *Epoller) Mod(fd int, ev SelectorEvent, fs [2]func()) error {
	_fd := int32(fd)
	fdLock, ok := s.fdLocks.Load(_fd)
	if !ok {
		return fmt.Errorf("the fd %d is not in epoll", fd)
	}
	_fdLock := fdLock.(chan struct{})

	e := &unix.EpollEvent{
		Fd: int32(fd),
	}
	if s.et {
		e.Events = unix.EPOLLET
	}

	switch ev {
	case SelectorEventRead:
		e.Events |= unix.EPOLLIN
	case SelectorEventWrite:
		e.Events |= unix.EPOLLOUT
	case SelectorEventReadWrite:
		e.Events |= unix.EPOLLIN | unix.EPOLLOUT
	default:
		return fmt.Errorf("unknow epoll event type: %d", ev)
	}

	_fdLock <- struct{}{}
	err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_MOD, fd, e)
	if err != nil {
		return os.NewSyscallError("epoll_ctl mod", err)
	}
	s.fdHandlers[_fd] = fs
	<-_fdLock

	return nil
}

func (s *Epoller) Del(fd int) error {
	_fd := int32(fd)
	fdLock, ok := s.fdLocks.Load(_fd)
	if !ok {
		return fmt.Errorf("the fd %d is not in epoll", fd)
	}
	_fdLock := fdLock.(chan struct{})

	_fdLock <- struct{}{}
	err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		return os.NewSyscallError("epoll_ctl del", err)
	}
	delete(s.fdHandlers, _fd)
	s.fdLocks.Delete(_fd)
	<-_fdLock

	return nil
}

func (s *Epoller) Polling(ctx context.Context) error {
	evs := make([]unix.EpollEvent, s.eventSize)
	for {
		select {
		case <-ctx.Done():
			break

		default:
			n, err := unix.EpollWait(s.fd, evs, -1)
			if err != nil && err != unix.EINTR {
				return os.NewSyscallError("epoll_wait", err)
			}

			if n < 0 {
				continue
			}

			for i := 0; i < n; i++ {
				// load fd lock
				fdLock, ok := s.fdLocks.Load(evs[i].Fd)
				if !ok {
					continue
				}
				_fdLock := fdLock.(chan struct{})

				// load fd handlers
				_fdLock <- struct{}{}
				fs := s.fdHandlers[evs[i].Fd]
				<-_fdLock

				// do read event
				if (evs[i].Events & unix.EPOLLIN) > 0 {
					s.taskPool.Go(fs[FdHandlerIndexIn])
				}
				// do write event
				if (evs[i].Events & unix.EPOLLOUT) > 0 {
					s.taskPool.Go(fs[FdHandlerIndexOut])
				}
			}
		}
	}
}
