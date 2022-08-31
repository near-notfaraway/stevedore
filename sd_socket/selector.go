package sd_socket

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

const DefaultEventSize = 1024

type SelectorEvent uint8

const (
	_ SelectorEvent = iota
	SelectorEventRead
	SelectorEventWrite
	SelectorEventReadWrite
)

type Selector interface {
	Add(fd int, ev SelectorEvent) error
	Mod(fd int, ev SelectorEvent) error
	Del(fd int) error
	Polling(ctx context.Context, handle func(evs []unix.EpollEvent)) error
}

//------------------------------------------------------------------------------
// Epoller: a selector base on epoll
//------------------------------------------------------------------------------

type Epoller struct {
	fd  int               // epoll fd
	evs []unix.EpollEvent // max events per poll
	et  bool              // if use edge trigger
}

func NewEpoller(eventSize int, et bool) (Selector, error) {
	if eventSize < 1 {
		eventSize = DefaultEventSize
	}

	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, os.NewSyscallError("epoll_create1", err)
	}

	return &Epoller{
		fd:  fd,
		evs: make([]unix.EpollEvent, eventSize),
		et:  et,
	}, nil
}

func (s *Epoller) Close() error {
	if err := unix.Close(s.fd); err != nil {
		return os.NewSyscallError("close", err)
	}

	return nil
}

func (s *Epoller) Add(fd int, ev SelectorEvent) error {
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

	if err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_ADD, fd, e); err != nil {
		return os.NewSyscallError("epoll_ctl add", err)
	}

	return nil
}

func (s *Epoller) Mod(fd int, ev SelectorEvent) error {
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

	if err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_MOD, fd, e); err != nil {
		return os.NewSyscallError("epoll_ctl mod", err)
	}

	return nil
}

func (s *Epoller) Del(fd int) error {
	if err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_DEL, fd, nil); err != nil {
		return os.NewSyscallError("epoll_ctl del", err)
	}

	return nil
}

func (s *Epoller) Polling(ctx context.Context, handle func(evs []unix.EpollEvent)) error {
	for {
		n, err := unix.EpollWait(s.fd, s.evs, -1)
		if err != nil && err != unix.EINTR {
			return os.NewSyscallError("epoll_wait", err)
		}

		if n < 0 {
			continue
		}

		handle(s.evs[:n])
	}
}
