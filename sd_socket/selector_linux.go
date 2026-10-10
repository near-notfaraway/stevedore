//go:build linux
// +build linux

package sd_socket

import (
	"context"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Epoller struct {
	fd     int
	wakeFD int
	evs    []unix.EpollEvent
	polled []PolledEvent
	et     bool
}

func NewSelector(eventSize int, et bool) (Selector, error) {
	return NewEpoller(eventSize, et)
}

func NewEpoller(eventSize int, et bool) (Selector, error) {
	if eventSize < 1 {
		eventSize = DefaultEventSize
	}

	fd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, os.NewSyscallError("epoll_create1", err)
	}

	wakeFD, err := NewEventFD()
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	s := &Epoller{
		fd:     fd,
		wakeFD: wakeFD,
		evs:    make([]unix.EpollEvent, eventSize),
		polled: make([]PolledEvent, 0, eventSize),
		et:     et,
	}
	if err = s.Add(wakeFD, SelectorEventRead); err != nil {
		_ = unix.Close(wakeFD)
		_ = unix.Close(fd)
		return nil, err
	}
	return s, nil
}

func (s *Epoller) Close() error {
	var firstErr error
	if err := unix.Close(s.wakeFD); err != nil {
		firstErr = os.NewSyscallError("close eventfd", err)
	}
	if err := unix.Close(s.fd); err != nil && firstErr == nil {
		firstErr = os.NewSyscallError("close epoll", err)
	}
	return firstErr
}

func (s *Epoller) Add(fd int, ev SelectorEvent) error {
	return s.control(unix.EPOLL_CTL_ADD, fd, ev)
}

func (s *Epoller) Mod(fd int, ev SelectorEvent) error {
	return s.control(unix.EPOLL_CTL_MOD, fd, ev)
}

func (s *Epoller) control(op, fd int, ev SelectorEvent) error {
	events, err := s.epollEvents(ev)
	if err != nil {
		return err
	}
	e := &unix.EpollEvent{
		Events: events,
		Fd:     int32(fd),
	}
	if err = unix.EpollCtl(s.fd, op, fd, e); err != nil {
		return os.NewSyscallError("epoll_ctl", err)
	}
	return nil
}

func (s *Epoller) epollEvents(ev SelectorEvent) (uint32, error) {
	var events uint32
	if s.et {
		events = unix.EPOLLET
	}
	switch ev {
	case SelectorEventRead:
		events |= unix.EPOLLIN
	case SelectorEventWrite:
		events |= unix.EPOLLOUT
	case SelectorEventReadWrite:
		events |= unix.EPOLLIN | unix.EPOLLOUT
	default:
		return 0, fmt.Errorf("unknown selector event type: %d", ev)
	}
	return events, nil
}

func (s *Epoller) Del(fd int) error {
	if err := unix.EpollCtl(s.fd, unix.EPOLL_CTL_DEL, fd, nil); err != nil {
		return os.NewSyscallError("epoll_ctl del", err)
	}
	return nil
}

func (s *Epoller) Wake() error {
	return NotifyEventFD(s.wakeFD)
}

func (s *Epoller) Polling(ctx context.Context, timeoutMs int, handle func(evs []PolledEvent)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, err := unix.EpollWait(s.fd, s.evs, timeoutMs)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return os.NewSyscallError("epoll_wait", err)
		}

		s.polled = s.polled[:0]
		for i := 0; i < n; i++ {
			event := s.evs[i]
			fd := int(event.Fd)
			if fd == s.wakeFD {
				if err = DrainEventFD(s.wakeFD); err != nil {
					return err
				}
				continue
			}

			var events SelectorEvent
			if event.Events&(unix.EPOLLIN|unix.EPOLLERR|unix.EPOLLHUP) != 0 {
				events |= SelectorEventRead
			}
			if event.Events&unix.EPOLLOUT != 0 {
				events |= SelectorEventWrite
			}
			if events != 0 {
				s.polled = append(s.polled, PolledEvent{FD: fd, Events: events})
			}
		}
		handle(s.polled)
	}
}


func NewEventFD() (int, error) {
	fd, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		return -1, os.NewSyscallError("eventfd", err)
	}
	return fd, nil
}

func NotifyEventFD(fd int) error {
	value := uint64(1)
	buf := (*[8]byte)(unsafe.Pointer(&value))[:]
	for {
		_, err := unix.Write(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN {
			return nil
		}
		if err != nil {
			return os.NewSyscallError("eventfd write", err)
		}
		return nil
	}
}

func DrainEventFD(fd int) error {
	var value uint64
	buf := (*[8]byte)(unsafe.Pointer(&value))[:]
	for {
		_, err := unix.Read(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN {
			return nil
		}
		if err != nil {
			return os.NewSyscallError("eventfd read", err)
		}
		return nil
	}
}
